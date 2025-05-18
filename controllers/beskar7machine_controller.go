/*
Copyright 2024 The Beskar7 Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	infrastructurev1alpha1 "github.com/wrkode/beskar7/api/v1alpha1"
	"github.com/wrkode/beskar7/internal/config"
	internalredfish "github.com/wrkode/beskar7/internal/redfish"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	conditions "sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// Beskar7MachineFinalizer allows Beskar7MachineReconciler to clean up resources associated with Beskar7Machine before removing it from the apiserver.
	Beskar7MachineFinalizer = "beskar7machine.infrastructure.cluster.x-k8s.io"

	// ProviderIDPrefix is the prefix used for ProviderID
	ProviderIDPrefix = "b7://"
)

// Beskar7MachineReconciler reconciles a Beskar7Machine object
type Beskar7MachineReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	// RedfishClientFactory allows overriding the Redfish client creation for testing.
	RedfishClientFactory internalredfish.RedfishClientFactory
	// Config holds the controller configuration
	Config *config.Config
}

//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=beskar7machines,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=beskar7machines/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=beskar7machines/finalizers,verbs=update
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch // Needed to interact with Machine object
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=physicalhosts,verbs=get;list;watch;patch // Needed to find and claim/patch PhysicalHost
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch // Needed for UserData

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Beskar7MachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	logger := log.FromContext(ctx).WithValues("beskar7machine", req.NamespacedName)
	logger.Info("Starting reconciliation")

	// Fetch the Beskar7Machine instance.
	b7machine := &infrastructurev1alpha1.Beskar7Machine{}
	if err := r.Get(ctx, req.NamespacedName, b7machine); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Unable to fetch Beskar7Machine")
			return ctrl.Result{}, err
		}
		logger.Info("Beskar7Machine resource not found. Ignoring since object must be deleted")
		return ctrl.Result{}, nil
	}

	// Fetch the Machine instance.
	machine, err := util.GetOwnerMachine(ctx, r.Client, b7machine.ObjectMeta)
	if err != nil {
		logger.Error(err, "Failed to get owner Machine")
		return ctrl.Result{}, err
	}
	if machine == nil {
		logger.Info("Waiting for Machine Controller to set OwnerRef on Beskar7Machine")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil // Requeue after a short delay
	}

	logger = logger.WithValues("machine", machine.Name)

	// Initialize patch helper
	patchHelper, err := patch.NewHelper(b7machine, r.Client)
	if err != nil {
		logger.Error(err, "Failed to init patch helper")
		return ctrl.Result{}, err
	}

	// Always attempt to patch the Beskar7Machine object and its status on reconciliation exit.
	defer func() {
		// Set the summary condition based on InfrastructureReadyCondition
		conditions.SetSummary(b7machine, conditions.WithConditions(infrastructurev1alpha1.InfrastructureReadyCondition))

		if err := patchHelper.Patch(ctx, b7machine); err != nil {
			logger.Error(err, "Failed to patch Beskar7Machine")
			if reterr == nil {
				reterr = err
			}
		}
		logger.Info("Finished reconciliation")
	}()

	// Handle deletion reconciliation
	if !b7machine.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, logger, b7machine)
	}

	// Handle non-deletion reconciliation
	return r.reconcileNormal(ctx, logger, b7machine, machine)
}

// reconcileNormal handles the logic when the Beskar7Machine is not being deleted.
func (r *Beskar7MachineReconciler) reconcileNormal(ctx context.Context, logger logr.Logger, b7machine *infrastructurev1alpha1.Beskar7Machine, machine *clusterv1.Machine) (ctrl.Result, error) {
	logger.Info("Reconciling Beskar7Machine create/update")

	// If the Beskar7Machine doesn't have our finalizer, add it.
	if controllerutil.AddFinalizer(b7machine, Beskar7MachineFinalizer) {
		logger.Info("Adding finalizer")
		return ctrl.Result{Requeue: true}, nil
	}

	// TODO: Check if paused

	// Find or retrieve the associated PhysicalHost.
	physicalHost, result, err := r.findAndClaimOrGetAssociatedHost(ctx, logger, b7machine)
	if err != nil {
		logger.Error(err, "Failed to find, claim, or get associated PhysicalHost")
		conditions.MarkFalse(b7machine, infrastructurev1alpha1.PhysicalHostAssociatedCondition, infrastructurev1alpha1.PhysicalHostAssociationFailedReason, clusterv1.ConditionSeverityWarning, "Failed to associate with PhysicalHost: %v", err.Error())
		return ctrl.Result{}, err
	}

	// ---> Set Condition True if host is found/claimed, even if requeuing <---
	if physicalHost != nil {
		logger.Info("Successfully associated with PhysicalHost", "physicalhost", physicalHost.Name)
		conditions.MarkTrue(b7machine, infrastructurev1alpha1.PhysicalHostAssociatedCondition)
	} else {
		// No host found yet
		conditions.MarkFalse(b7machine, infrastructurev1alpha1.PhysicalHostAssociatedCondition, infrastructurev1alpha1.WaitingForPhysicalHostReason, clusterv1.ConditionSeverityInfo, "No available PhysicalHost found")
		// If result is also zero here, it's an unexpected state, but the later check handles it.
	}

	if !result.IsZero() {
		logger.Info("Requeuing requested by findAndClaimOrGetAssociatedHost")
		// Condition is already set based on whether physicalHost was nil or not above
		return result, nil
	}
	if physicalHost == nil {
		// Should not happen if result is zero and err is nil, but check defensively.
		logger.Info("No associated or available PhysicalHost found (unexpected state after check), requeuing")
		conditions.MarkFalse(b7machine, infrastructurev1alpha1.PhysicalHostAssociatedCondition, infrastructurev1alpha1.WaitingForPhysicalHostReason, clusterv1.ConditionSeverityInfo, "No available PhysicalHost found")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	logger = logger.WithValues("physicalhost", physicalHost.Name)
	// logger.Info("Successfully associated with PhysicalHost") // Moved up
	// conditions.MarkTrue(b7machine, infrastructurev1alpha1.PhysicalHostAssociatedCondition) // Moved up

	// Reconcile based on PhysicalHost status
	switch physicalHost.Status.State {
	case infrastructurev1alpha1.StateProvisioned:
		logger.Info("Associated PhysicalHost is Provisioned")
		currentProviderID := providerID(physicalHost.Namespace, physicalHost.Name)
		if b7machine.Spec.ProviderID == nil || *b7machine.Spec.ProviderID != currentProviderID {
			logger.Info("Setting ProviderID", "ProviderID", currentProviderID)
			b7machine.Spec.ProviderID = &currentProviderID
		}
		conditions.MarkTrue(b7machine, infrastructurev1alpha1.InfrastructureReadyCondition)
		phase := "Provisioned"
		b7machine.Status.Phase = &phase
		logger.Info("Beskar7Machine infrastructure is Ready")

	case infrastructurev1alpha1.StateProvisioning:
		logger.Info("Waiting for associated PhysicalHost to finish provisioning")
		conditions.MarkFalse(b7machine, infrastructurev1alpha1.InfrastructureReadyCondition, infrastructurev1alpha1.PhysicalHostNotReadyReason, clusterv1.ConditionSeverityInfo, "PhysicalHost %q is still provisioning", physicalHost.Name)
		phase := "Provisioning"
		b7machine.Status.Phase = &phase
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil

	case infrastructurev1alpha1.StateAvailable, infrastructurev1alpha1.StateClaimed, infrastructurev1alpha1.StateEnrolling:
		logger.Info("Waiting for associated PhysicalHost to start/complete provisioning", "hostState", physicalHost.Status.State)
		conditions.MarkFalse(b7machine, infrastructurev1alpha1.InfrastructureReadyCondition, infrastructurev1alpha1.PhysicalHostNotReadyReason, clusterv1.ConditionSeverityInfo, "PhysicalHost %q is not yet provisioned (state: %s)", physicalHost.Name, physicalHost.Status.State)
		phase := "Associating"
		b7machine.Status.Phase = &phase
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil

	case infrastructurev1alpha1.StateError:
		errMsg := fmt.Sprintf("PhysicalHost %q is in error state: %s", physicalHost.Name, physicalHost.Status.ErrorMessage)
		logger.Error(nil, errMsg)
		conditions.MarkFalse(b7machine, infrastructurev1alpha1.InfrastructureReadyCondition, infrastructurev1alpha1.PhysicalHostErrorReason, clusterv1.ConditionSeverityError, "%s", errMsg)
		phase := "Failed"
		b7machine.Status.Phase = &phase
		return ctrl.Result{}, nil // No automatic requeue

	default:
		logger.Info("Associated PhysicalHost is in unknown or intermediate state", "hostState", physicalHost.Status.State)
		conditions.MarkFalse(b7machine, infrastructurev1alpha1.InfrastructureReadyCondition, infrastructurev1alpha1.PhysicalHostNotReadyReason, clusterv1.ConditionSeverityInfo, "PhysicalHost %q is in state: %s", physicalHost.Name, physicalHost.Status.State)
		phase := "Pending"
		b7machine.Status.Phase = &phase
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

// reconcileDelete handles the logic when the Beskar7Machine is marked for deletion.
func (r *Beskar7MachineReconciler) reconcileDelete(ctx context.Context, logger logr.Logger, b7machine *infrastructurev1alpha1.Beskar7Machine) (ctrl.Result, error) {
	logger.Info("Reconciling Beskar7Machine deletion")

	// Mark conditions False
	conditions.MarkFalse(b7machine, infrastructurev1alpha1.InfrastructureReadyCondition, clusterv1.DeletingReason, clusterv1.ConditionSeverityInfo, "Beskar7Machine is being deleted")
	conditions.MarkFalse(b7machine, infrastructurev1alpha1.PhysicalHostAssociatedCondition, clusterv1.DeletingReason, clusterv1.ConditionSeverityInfo, "Beskar7Machine is being deleted")

	// Get the associated PhysicalHost to release it
	var physicalHost *infrastructurev1alpha1.PhysicalHost
	if b7machine.Spec.ProviderID != nil && *b7machine.Spec.ProviderID != "" {
		ns, name, err := parseProviderID(*b7machine.Spec.ProviderID)
		if err != nil {
			logger.Error(err, "Failed to parse ProviderID during deletion, unable to release host", "ProviderID", *b7machine.Spec.ProviderID)
			// Cannot release the host, but proceed with finalizer removal as the ProviderID is invalid
		} else if ns != b7machine.Namespace {
			logger.Error(err, "ProviderID namespace mismatch during deletion, unable to release host", "ProviderID", *b7machine.Spec.ProviderID, "MachineNamespace", b7machine.Namespace)
			// Cannot release the host, proceed with finalizer removal
		} else {
			logger.Info("Finding associated PhysicalHost to release", "PhysicalHostName", name)
			foundHost := &infrastructurev1alpha1.PhysicalHost{}
			if err := r.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, foundHost); err != nil {
				if client.IgnoreNotFound(err) != nil {
					logger.Error(err, "Failed to get PhysicalHost for release", "PhysicalHostName", name)
					conditions.MarkFalse(b7machine, infrastructurev1alpha1.PhysicalHostAssociatedCondition, infrastructurev1alpha1.ReleasePhysicalHostFailedReason, clusterv1.ConditionSeverityWarning, "Failed to get PhysicalHost %s for release: %v", name, err.Error())
					return ctrl.Result{}, err // Requeue
				}
				// Host not found, nothing to release.
				logger.Info("Associated PhysicalHost not found, nothing to release", "PhysicalHostName", name)
			} else {
				// Host found, store it for release
				physicalHost = foundHost
			}
		}
	} else {
		logger.Info("No ProviderID set, assuming no PhysicalHost was associated.")
		// If no provider ID, maybe try listing hosts claimed by this machine?
		// For now, assume nothing to release if ProviderID is missing.
	}

	// Release the host if found
	if physicalHost != nil {
		logger.Info("Releasing associated PhysicalHost", "PhysicalHost", physicalHost.Name)
		// Check if we are the consumer before releasing
		if physicalHost.Spec.ConsumerRef != nil &&
			physicalHost.Spec.ConsumerRef.Name == b7machine.Name &&
			physicalHost.Spec.ConsumerRef.Namespace == b7machine.Namespace {

			// --- Try using Update instead of Patch for simplicity in test ---
			originalHost := physicalHost.DeepCopy() // Keep for potential revert or logging
			logger.Info("Attempting to release host via client.Update", "PhysicalHost", physicalHost.Name)
			physicalHost.Spec.ConsumerRef = nil
			physicalHost.Spec.BootISOSource = nil

			if err := r.Update(ctx, physicalHost); err != nil {
				logger.Error(err, "Failed to Update PhysicalHost for release", "PhysicalHost", physicalHost.Name)
				conditions.MarkFalse(b7machine, infrastructurev1alpha1.PhysicalHostAssociatedCondition, infrastructurev1alpha1.ReleasePhysicalHostFailedReason, clusterv1.ConditionSeverityWarning, "Failed to update PhysicalHost %s for release: %v", originalHost.Name, err.Error())
				// Attempt to revert local change before returning error?
				// physicalHost.Spec = originalHost.Spec
				return ctrl.Result{}, err // Requeue to retry update
			}
			// --- End Update attempt ---

			/* --- Original Patch Logic ---
			originalHostToPatch := physicalHost.DeepCopy()
			physicalHost.Spec.ConsumerRef = nil
			physicalHost.Spec.BootISOSource = nil
			// No need to manage UserDataSecretRef here, PhysicalHost controller should handle it if needed during its own delete/deprovision.

			hostPatchHelper, err := patch.NewHelper(originalHostToPatch, r.Client)
			if err != nil {
				logger.Error(err, "Failed to init patch helper for releasing PhysicalHost", "PhysicalHost", physicalHost.Name)
				conditions.MarkFalse(b7machine, infrastructurev1alpha1.PhysicalHostAssociatedCondition, infrastructurev1alpha1.ReleasePhysicalHostFailedReason, clusterv1.ConditionSeverityWarning, "Failed to init patch helper for release: %v", err.Error())
				return ctrl.Result{}, err
			}
			if err := hostPatchHelper.Patch(ctx, physicalHost); err != nil {
				logger.Error(err, "Failed to patch PhysicalHost for release", "PhysicalHost", physicalHost.Name)
				conditions.MarkFalse(b7machine, infrastructurev1alpha1.PhysicalHostAssociatedCondition, infrastructurev1alpha1.ReleasePhysicalHostFailedReason, clusterv1.ConditionSeverityWarning, "Failed to patch PhysicalHost %s for release: %v", physicalHost.Name, err.Error())
				return ctrl.Result{}, err // Requeue to retry patch
			}
			--- End Original Patch Logic --- */

			logger.Info("Successfully released PhysicalHost", "PhysicalHost", physicalHost.Name)
			// TODO: Maybe wait briefly or requeue to allow PhysicalHost controller to react?
		} else {
			logger.Info("PhysicalHost already released or claimed by another resource", "PhysicalHost", physicalHost.Name)
		}
	}

	// Beskar7Machine is being deleted, remove the finalizer.
	if controllerutil.RemoveFinalizer(b7machine, Beskar7MachineFinalizer) {
		logger.Info("Removing finalizer")
		// Patching is handled by the deferred patch function in Reconcile.
	}

	return ctrl.Result{}, nil
}

// findAndClaimOrGetAssociatedHost tries to find an available PhysicalHost and claim it,
// or returns the PhysicalHost already associated with the Beskar7Machine.
func (r *Beskar7MachineReconciler) findAndClaimOrGetAssociatedHost(ctx context.Context, logger logr.Logger, b7machine *infrastructurev1alpha1.Beskar7Machine) (*infrastructurev1alpha1.PhysicalHost, ctrl.Result, error) {
	logger.Info("Attempting to find associated or available PhysicalHost")

	// First, check if ProviderID is set and try to get that host
	if b7machine.Spec.ProviderID != nil && *b7machine.Spec.ProviderID != "" {
		ns, name, err := parseProviderID(*b7machine.Spec.ProviderID)
		if err != nil {
			logger.Error(err, "Failed to parse ProviderID", "ProviderID", *b7machine.Spec.ProviderID)
			// TODO: Set error status on b7machine?
			return nil, ctrl.Result{}, err // Return error, stop reconciliation
		}
		if ns != b7machine.Namespace {
			err := errors.Errorf("ProviderID %q has different namespace than Beskar7Machine %q", *b7machine.Spec.ProviderID, b7machine.Namespace)
			logger.Error(err, "Namespace mismatch")
			return nil, ctrl.Result{}, err
		}

		logger.Info("ProviderID is set, fetching associated PhysicalHost", "PhysicalHostName", name)
		foundHost := &infrastructurev1alpha1.PhysicalHost{}
		if err := r.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, foundHost); err != nil {
			if client.IgnoreNotFound(err) != nil {
				logger.Error(err, "Failed to get PhysicalHost by ProviderID", "PhysicalHostName", name)
				return nil, ctrl.Result{}, err
			}
			// Host not found, maybe it was deleted?
			logger.Error(err, "PhysicalHost specified in ProviderID not found")
			// Clear the provider ID and let the logic try to find a new host
			b7machine.Spec.ProviderID = nil
			return nil, ctrl.Result{Requeue: true}, nil
		}

		// Verify the host is actually claimed by this machine, or is in a state indicating it was (e.g., Provisioned)
		if foundHost.Spec.ConsumerRef == nil || foundHost.Spec.ConsumerRef.Name != b7machine.Name || foundHost.Spec.ConsumerRef.Namespace != b7machine.Namespace {
			if foundHost.Status.State != infrastructurev1alpha1.StateProvisioned && foundHost.Status.State != infrastructurev1alpha1.StateProvisioning {
				logger.Error(nil, "PhysicalHost specified in ProviderID is not referencing this Beskar7Machine", "PhysicalHostName", name, "ConsumerRef", foundHost.Spec.ConsumerRef)
				// Clear the provider ID and let the logic try to find a new host
				b7machine.Spec.ProviderID = nil
				return nil, ctrl.Result{Requeue: true}, nil
			}
			logger.Info("PhysicalHost specified in ProviderID does not have matching ConsumerRef, but assuming association due to Provisioned/Provisioning state", "PhysicalHostName", name)
		}
		logger.Info("Found associated PhysicalHost via ProviderID", "PhysicalHostName", name)
		return foundHost, ctrl.Result{}, nil
	}

	// ProviderID not set, list hosts to find an associated or available one
	logger.Info("ProviderID not set, searching for associated or available PhysicalHost")
	phList := &infrastructurev1alpha1.PhysicalHostList{}
	listOpts := []client.ListOption{client.InNamespace(b7machine.Namespace)}
	// TODO: Add label selector if using labels for matching?
	if err := r.List(ctx, phList, listOpts...); err != nil {
		logger.Error(err, "Failed to list PhysicalHosts")
		return nil, ctrl.Result{}, err
	}

	var associatedHost *infrastructurev1alpha1.PhysicalHost
	var availableHost *infrastructurev1alpha1.PhysicalHost

	for i := range phList.Items {
		host := &phList.Items[i]

		// Check if this host is already claimed by us
		if host.Spec.ConsumerRef != nil && host.Spec.ConsumerRef.Name == b7machine.Name && host.Spec.ConsumerRef.Namespace == b7machine.Namespace {
			associatedHost = host
			logger.Info("Found PhysicalHost already associated via ConsumerRef", "PhysicalHost", associatedHost.Name)
			break // Found our host, no need to look further
		}

		// Check if this host is available (and remember the first one)
		if availableHost == nil && host.Spec.ConsumerRef == nil && host.Status.State == infrastructurev1alpha1.StateAvailable {
			availableHost = host
			logger.Info("Found potentially available PhysicalHost", "PhysicalHost", availableHost.Name)
			// Continue searching in case we find one already associated
		}
	}

	// If we found a host already associated, return it
	if associatedHost != nil {
		return associatedHost, ctrl.Result{}, nil
	}

	// If no associated host, but found an available one, claim it
	if availableHost != nil {
		logger.Info("Claiming available PhysicalHost", "PhysicalHost", availableHost.Name)
		// Set ConsumerRef first
		originalHost := availableHost.DeepCopy()
		availableHost.Spec.ConsumerRef = &corev1.ObjectReference{
			Kind:       b7machine.Kind,
			APIVersion: b7machine.APIVersion,
			Name:       b7machine.Name,
			Namespace:  b7machine.Namespace,
			UID:        b7machine.UID,
		}
		// Also set the BootISOSource on the PhysicalHost spec when claiming
		// This is what the PhysicalHostReconciler will look for.
		isoURLForPHSpec := b7machine.Spec.ImageURL // Default to ImageURL
		availableHost.Spec.BootISOSource = &isoURLForPHSpec

		// Patch the ConsumerRef and BootISOSource update immediately
		hostPatchHelper, err := patch.NewHelper(originalHost, r.Client)
		if err != nil {
			logger.Error(err, "Failed to init patch helper for PhysicalHost", "PhysicalHost", availableHost.Name)
			return nil, ctrl.Result{}, err
		}
		if err := hostPatchHelper.Patch(ctx, availableHost); err != nil {
			logger.Error(err, "Failed to patch PhysicalHost to set ConsumerRef and BootISOSource", "PhysicalHost", availableHost.Name)
			return nil, ctrl.Result{}, err
		}
		logger.Info("Successfully patched PhysicalHost with ConsumerRef and BootISOSource in Spec")

		// Determine provisioning mode and configure boot on the BMC
		provisioningMode := b7machine.Spec.ProvisioningMode
		if provisioningMode == "" {
			if b7machine.Spec.ConfigURL != "" {
				provisioningMode = "RemoteConfig"
			} else {
				provisioningMode = "PreBakedISO"
			}
			logger.Info("ProvisioningMode defaulted", "Mode", provisioningMode)
		}

		// Get Redfish client for the claimed host
		rfClient, err := r.getRedfishClientForHost(ctx, logger, availableHost)
		if err != nil {
			logger.Error(err, "Failed to get Redfish client for host provisioning", "PhysicalHost", availableHost.Name)
			// TODO: Should we set a condition on b7machine here? Or just let it requeue?
			return nil, ctrl.Result{}, err // Requeue and try again
		}
		defer rfClient.Close(ctx)

		switch provisioningMode {
		case "RemoteConfig":
			logger.Info("Configuring boot for RemoteConfig mode")
			if b7machine.Spec.ConfigURL == "" {
				err := errors.New("ConfigURL must be set when ProvisioningMode is RemoteConfig")
				logger.Error(err, "Invalid spec")
				return nil, ctrl.Result{}, err
			}

			var kernelParams []string
			switch b7machine.Spec.OSFamily {
			case "kairos":
				kernelParams = []string{fmt.Sprintf("config_url=%s", b7machine.Spec.ConfigURL)}
			case "talos":
				kernelParams = []string{fmt.Sprintf("talos.config=%s", b7machine.Spec.ConfigURL)}
			case "flatcar":
				kernelParams = []string{fmt.Sprintf("flatcar.ignition.config.url=%s", b7machine.Spec.ConfigURL)}
			case "LeapMicro":
				kernelParams = []string{fmt.Sprintf("combustion.path=%s", b7machine.Spec.ConfigURL)}
			default:
				err := errors.Errorf("unsupported OSFamily for RemoteConfig: %s", b7machine.Spec.OSFamily)
				logger.Error(err, "Invalid spec")
				return nil, ctrl.Result{}, err
			}
			logger.Info("Calculated kernel parameters", "Params", kernelParams)

			// TODO: Replace with actual Redfish client calls
			logger.Info("TODO: Call rfClient.SetBootParameters", "Params", kernelParams)
			if err := rfClient.SetBootParameters(ctx, kernelParams); err != nil {
				logger.Error(err, "Failed to set boot parameters for RemoteConfig", "Params", kernelParams)
				return nil, ctrl.Result{}, err // Requeue
			}
			logger.Info("TODO: Call rfClient.SetBootSourceISO", "URL", b7machine.Spec.ImageURL)
			if err := rfClient.SetBootSourceISO(ctx, b7machine.Spec.ImageURL); err != nil {
				logger.Error(err, "Failed to set boot source ISO for RemoteConfig", "ImageURL", b7machine.Spec.ImageURL)
				return nil, ctrl.Result{}, err // Requeue
			}

		case "PreBakedISO":
			logger.Info("Configuring boot for PreBakedISO mode")
			// TODO: Replace with actual Redfish client calls
			logger.Info("TODO: Call rfClient.SetBootParameters with nil")
			if err := rfClient.SetBootParameters(ctx, nil); err != nil { // Clear any existing boot params
				logger.Error(err, "Failed to clear boot parameters for PreBakedISO")
				return nil, ctrl.Result{}, err // Requeue
			}
			logger.Info("TODO: Call rfClient.SetBootSourceISO", "URL", b7machine.Spec.ImageURL)
			if err := rfClient.SetBootSourceISO(ctx, b7machine.Spec.ImageURL); err != nil {
				logger.Error(err, "Failed to set boot source ISO for PreBakedISO", "ImageURL", b7machine.Spec.ImageURL)
				return nil, ctrl.Result{}, err // Requeue
			}

		default:
			err := errors.Errorf("invalid ProvisioningMode: %s", provisioningMode)
			logger.Error(err, "Invalid spec")
			return nil, ctrl.Result{}, err
		}

		logger.Info("Successfully claimed PhysicalHost and initiated boot configuration on BMC", "PhysicalHost", availableHost.Name)
		// Return the claimed host, but controller should requeue to wait for PhysicalHost reconcile
		return availableHost, ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	// No associated or available host found
	logger.Info("No associated or available PhysicalHost found, requeuing")
	// TODO: Set status condition?
	return nil, ctrl.Result{RequeueAfter: time.Minute}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Beskar7MachineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1alpha1.Beskar7Machine{}).
		// TODO: Add watches for CAPI Machine, PhysicalHost?
		// Watches(&source.Kind{Type: &infrastructurev1alpha1.PhysicalHost{}}, handler.EnqueueRequestsFromMapFunc(r.PhysicalHostToBeskar7Machine))?
		Complete(r)
}

// getRedfishClientForHost retrieves credentials and establishes a connection
// to the Redfish endpoint specified in the PhysicalHost spec.
func (r *Beskar7MachineReconciler) getRedfishClientForHost(ctx context.Context, logger logr.Logger, physicalHost *infrastructurev1alpha1.PhysicalHost) (internalredfish.Client, error) {
	log := logger.WithValues("physicalhost", physicalHost.Name)

	// --- Fetch Redfish Credentials ---
	secretName := physicalHost.Spec.RedfishConnection.CredentialsSecretRef
	if secretName == "" {
		// Should ideally be validated by webhook, but double-check
		err := errors.New("PhysicalHost CredentialsSecretRef is not set")
		log.Error(err, "Missing CredentialsSecretRef")
		return nil, err
	}
	credentialsSecret := &corev1.Secret{}
	secretKey := client.ObjectKey{Namespace: physicalHost.Namespace, Name: secretName}
	if err := r.Get(ctx, secretKey, credentialsSecret); err != nil {
		if client.IgnoreNotFound(err) != nil {
			log.Error(err, "Failed to fetch credentials secret", "SecretName", secretName)
		} else {
			log.Error(err, "Credentials secret not found", "SecretName", secretName)
		}
		return nil, fmt.Errorf("failed to get credentials secret %s: %w", secretName, err)
	}
	usernameBytes, okUser := credentialsSecret.Data["username"]
	passwordBytes, okPass := credentialsSecret.Data["password"]
	if !okUser || !okPass {
		err := errors.New("username or password missing in credentials secret data")
		log.Error(err, "Invalid credentials secret format", "SecretName", secretName)
		return nil, err
	}
	username := string(usernameBytes)
	password := string(passwordBytes)
	// --- End Fetch Redfish Credentials ---

	// --- Read Vendor-Specific Configuration (Example: Annotation for BIOS Attribute) ---
	// Check for an annotation on the PhysicalHost that might specify a vendor-specific
	// BIOS attribute to use for setting kernel parameters if the standard UEFI method fails.
	var biosKernelArgAttribute string
	if physicalHost.Annotations != nil {
		biosKernelArgAttribute = physicalHost.Annotations["beskar7.infrastructure.cluster.x-k8s.io/bios-kernel-arg-attribute"]
	}
	if biosKernelArgAttribute != "" {
		log.Info("Found annotation for BIOS kernel argument attribute", "attributeName", biosKernelArgAttribute)
		// TODO: Pass this attribute name to the Redfish client implementation somehow.
		// This might involve modifying the RedfishClientFactory signature or having a
		// dedicated method on the client interface like `SetVendorOptions(...)`.
		// For now, we just log its presence.
	} else {
		log.V(1).Info("No specific BIOS kernel argument attribute annotation found.")
	}
	// --- End Vendor-Specific Configuration ---

	// --- Connect to Redfish ---
	clientFactory := r.RedfishClientFactory
	if clientFactory == nil {
		log.Info("RedfishClientFactory not provided, using default internalredfish.NewClient")
		clientFactory = internalredfish.NewClient
	}

	insecure := physicalHost.Spec.RedfishConnection.InsecureSkipVerify != nil && *physicalHost.Spec.RedfishConnection.InsecureSkipVerify
	rfClient, err := clientFactory(ctx, physicalHost.Spec.RedfishConnection.Address, username, password, insecure)
	if err != nil {
		log.Error(err, "Failed to create Redfish client")
		return nil, fmt.Errorf("failed to create redfish client for %s: %w", physicalHost.Spec.RedfishConnection.Address, err)
	}
	log.Info("Successfully connected to Redfish endpoint")
	// --- End Connect to Redfish ---

	return rfClient, nil
}

// providerID returns the Beskar7 providerID for the given PhysicalHost.
func providerID(namespace, name string) string {
	return fmt.Sprintf("%s%s/%s", ProviderIDPrefix, namespace, name)
}

// parseProviderID extracts the namespace and name from the providerID.
// Returns error if the providerID is invalid.
func parseProviderID(providerID string) (string, string, error) {
	if !strings.HasPrefix(providerID, ProviderIDPrefix) {
		return "", "", errors.Errorf("invalid providerID %q: missing prefix %q", providerID, ProviderIDPrefix)
	}
	parts := strings.SplitN(strings.TrimPrefix(providerID, ProviderIDPrefix), "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", errors.Errorf("invalid providerID %q: expected format %s<namespace>/<name>", providerID, ProviderIDPrefix)
	}
	return parts[0], parts[1], nil
}
