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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	infrastructurev1beta1 "github.com/wrkode/beskar7/api/v1beta1"
	"github.com/wrkode/beskar7/internal/redfish"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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
	// If nil, internalredfish.NewClient will be used by default in getRedfishClientForHost.
	RedfishClientFactory redfish.RedfishClientFactory
	Log                  logr.Logger
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
	log := r.Log.WithValues("beskar7machine", req.NamespacedName)
	log.Info("Starting reconciliation")

	// Fetch the Beskar7Machine instance.
	b7machine := &infrastructurev1beta1.Beskar7Machine{}
	err := r.Get(ctx, req.NamespacedName, b7machine)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Beskar7Machine resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Unable to fetch Beskar7Machine")
		return ctrl.Result{}, err
	}

	// Check if the Beskar7Machine is paused
	if isPaused(b7machine) {
		log.Info("Beskar7Machine reconciliation is paused")
		return ctrl.Result{}, nil
	}

	// Fetch the Machine instance.
	machine, err := util.GetOwnerMachine(ctx, r.Client, b7machine.ObjectMeta)
	if err != nil {
		log.Error(err, "Failed to get owner Machine")
		return ctrl.Result{}, err
	}
	if machine == nil {
		log.Info("Waiting for Machine Controller to set OwnerRef on Beskar7Machine")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil // Requeue after a short delay
	}

	log = log.WithValues("machine", machine.Name)

	// Get the owner cluster to check if it's paused
	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, machine.ObjectMeta)
	if err != nil {
		log.Error(err, "Failed to get cluster from machine metadata")
		return ctrl.Result{}, err
	}

	// Check if the owner cluster is paused
	if isClusterPaused(cluster) {
		log.Info("Beskar7Machine reconciliation is paused because owner cluster is paused")
		return ctrl.Result{}, nil
	}

	// Initialize patch helper
	patchHelper, err := patch.NewHelper(b7machine, r.Client)
	if err != nil {
		log.Error(err, "Failed to init patch helper")
		return ctrl.Result{}, err
	}

	// Always attempt to patch the Beskar7Machine object and its status on reconciliation exit.
	defer func() {
		// Set the summary condition based on InfrastructureReadyCondition
		conditions.SetSummary(b7machine, conditions.WithConditions(infrastructurev1beta1.InfrastructureReadyCondition))

		if err := patchHelper.Patch(ctx, b7machine); err != nil {
			log.Error(err, "Failed to patch Beskar7Machine")
			if reterr == nil {
				reterr = err
			}
		}
		log.Info("Finished reconciliation")
	}()

	// Handle deletion reconciliation
	if !b7machine.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, log, b7machine)
	}

	// Handle non-deletion reconciliation
	return r.reconcileNormal(ctx, log, b7machine, machine)
}

// reconcileNormal handles the logic when the Beskar7Machine is not being deleted.
func (r *Beskar7MachineReconciler) reconcileNormal(ctx context.Context, logger logr.Logger, b7machine *infrastructurev1beta1.Beskar7Machine, machine *clusterv1.Machine) (ctrl.Result, error) {
	logger.Info("Reconciling Beskar7Machine create/update")

	// If the Beskar7Machine doesn't have our finalizer, add it.
	if controllerutil.AddFinalizer(b7machine, Beskar7MachineFinalizer) {
		logger.Info("Adding finalizer")
		return ctrl.Result{Requeue: true}, nil
	}

	// Find or retrieve the associated PhysicalHost.
	physicalHost, result, err := r.findAndClaimOrGetAssociatedHost(ctx, logger, b7machine)
	if err != nil {
		// Transient error during host association
		logger.Error(err, "Failed to find, claim, or get associated PhysicalHost")
		conditions.MarkFalse(b7machine, infrastructurev1beta1.PhysicalHostAssociatedCondition, infrastructurev1beta1.PhysicalHostAssociationFailedReason, clusterv1.ConditionSeverityWarning, "Failed to associate with PhysicalHost: %v", err)
		return result, err // Requeue with backoff
	}

	if physicalHost != nil {
		logger.Info("Successfully associated with PhysicalHost", "physicalhost", physicalHost.Name)
		conditions.MarkTrue(b7machine, infrastructurev1beta1.PhysicalHostAssociatedCondition)
	} else {
		// No host found yet, this is a transient condition
		logger.Info("No available or associated PhysicalHost found, requeuing")
		conditions.MarkFalse(b7machine, infrastructurev1beta1.PhysicalHostAssociatedCondition, infrastructurev1beta1.WaitingForPhysicalHostReason, clusterv1.ConditionSeverityInfo, "No available PhysicalHost found")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	if !result.IsZero() {
		logger.Info("Requeuing requested by findAndClaimOrGetAssociatedHost")
		return result, nil
	}

	logger = logger.WithValues("physicalhost", physicalHost.Name)

	// Reconcile based on PhysicalHost status
	switch physicalHost.Status.State {
	case infrastructurev1beta1.StateProvisioned:
		logger.Info("Associated PhysicalHost is Provisioned")
		currentProviderID := providerID(physicalHost.Namespace, physicalHost.Name)
		if b7machine.Spec.ProviderID == nil || *b7machine.Spec.ProviderID != currentProviderID {
			logger.Info("Setting ProviderID", "ProviderID", currentProviderID)
			b7machine.Spec.ProviderID = &currentProviderID
		}

		// Copy addresses from PhysicalHost to Beskar7Machine
		if len(physicalHost.Status.Addresses) > 0 {
			b7machine.Status.Addresses = physicalHost.Status.Addresses
			logger.Info("Copied network addresses from PhysicalHost", "addressCount", len(physicalHost.Status.Addresses))
			for _, addr := range physicalHost.Status.Addresses {
				logger.V(1).Info("Copied address", "type", addr.Type, "address", addr.Address)
			}
		} else {
			logger.V(1).Info("No addresses available on PhysicalHost to copy")
		}

		conditions.MarkTrue(b7machine, infrastructurev1beta1.InfrastructureReadyCondition)
		b7machine.Status.Ready = true
		phase := "Provisioned"
		b7machine.Status.Phase = &phase
		logger.Info("Beskar7Machine infrastructure is Ready")
		return ctrl.Result{}, nil

	case infrastructurev1beta1.StateProvisioning:
		logger.Info("Waiting for associated PhysicalHost to finish provisioning")
		conditions.MarkFalse(b7machine, infrastructurev1beta1.InfrastructureReadyCondition, infrastructurev1beta1.PhysicalHostNotReadyReason, clusterv1.ConditionSeverityInfo, "PhysicalHost %q is still provisioning", physicalHost.Name)
		phase := "Provisioning"
		b7machine.Status.Phase = &phase
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil

	case infrastructurev1beta1.StateAvailable, infrastructurev1beta1.StateClaimed, infrastructurev1beta1.StateEnrolling:
		logger.Info("Waiting for associated PhysicalHost to start/complete provisioning", "hostState", physicalHost.Status.State)
		conditions.MarkFalse(b7machine, infrastructurev1beta1.InfrastructureReadyCondition, infrastructurev1beta1.PhysicalHostNotReadyReason, clusterv1.ConditionSeverityInfo, "PhysicalHost %q is not yet provisioned (state: %s)", physicalHost.Name, physicalHost.Status.State)
		phase := "Associating"
		b7machine.Status.Phase = &phase
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil

	case infrastructurev1beta1.StateError:
		// Permanent error: the associated PhysicalHost is in a terminal error state.
		logger.Error(nil, "Associated PhysicalHost is in error state", "errorMessage", physicalHost.Status.ErrorMessage)
		conditions.MarkFalse(b7machine, infrastructurev1beta1.InfrastructureReadyCondition, infrastructurev1beta1.PhysicalHostErrorReason, clusterv1.ConditionSeverityError, "PhysicalHost %q in error state: %s", physicalHost.Name, physicalHost.Status.ErrorMessage)
		phase := "Failed"
		b7machine.Status.Phase = &phase
		b7machine.Status.Ready = false
		return ctrl.Result{}, nil // Stop reconciliation

	default:
		logger.Info("Associated PhysicalHost is in unknown or intermediate state", "hostState", physicalHost.Status.State)
		conditions.MarkFalse(b7machine, infrastructurev1beta1.InfrastructureReadyCondition, infrastructurev1beta1.PhysicalHostNotReadyReason, clusterv1.ConditionSeverityInfo, "PhysicalHost %q is in state: %s", physicalHost.Name, physicalHost.Status.State)
		phase := "Pending"
		b7machine.Status.Phase = &phase
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
}

// reconcileDelete handles the logic when the Beskar7Machine is marked for deletion.
func (r *Beskar7MachineReconciler) reconcileDelete(ctx context.Context, logger logr.Logger, b7machine *infrastructurev1beta1.Beskar7Machine) (ctrl.Result, error) {
	logger.Info("Reconciling Beskar7Machine deletion")

	// Mark conditions False
	conditions.MarkFalse(b7machine, infrastructurev1beta1.InfrastructureReadyCondition, clusterv1.DeletingReason, clusterv1.ConditionSeverityInfo, "Beskar7Machine is being deleted")
	conditions.MarkFalse(b7machine, infrastructurev1beta1.PhysicalHostAssociatedCondition, clusterv1.DeletingReason, clusterv1.ConditionSeverityInfo, "Beskar7Machine is being deleted")

	// Get the associated PhysicalHost to release it
	var physicalHost *infrastructurev1beta1.PhysicalHost
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
			foundHost := &infrastructurev1beta1.PhysicalHost{}
			if err := r.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, foundHost); err != nil {
				if client.IgnoreNotFound(err) != nil {
					logger.Error(err, "Failed to get PhysicalHost for release", "PhysicalHostName", name)
					conditions.MarkFalse(b7machine, infrastructurev1beta1.PhysicalHostAssociatedCondition, infrastructurev1beta1.ReleasePhysicalHostFailedReason, clusterv1.ConditionSeverityWarning, "Failed to get PhysicalHost %s for release: %v", name, err.Error())
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
				conditions.MarkFalse(b7machine, infrastructurev1beta1.PhysicalHostAssociatedCondition, infrastructurev1beta1.ReleasePhysicalHostFailedReason, clusterv1.ConditionSeverityWarning, "Failed to update PhysicalHost %s for release: %v", originalHost.Name, err.Error())
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
				conditions.MarkFalse(b7machine, infrastructurev1beta1.PhysicalHostAssociatedCondition, infrastructurev1beta1.ReleasePhysicalHostFailedReason, clusterv1.ConditionSeverityWarning, "Failed to init patch helper for release: %v", err.Error())
				return ctrl.Result{}, err
			}
			if err := hostPatchHelper.Patch(ctx, physicalHost); err != nil {
				logger.Error(err, "Failed to patch PhysicalHost for release", "PhysicalHost", physicalHost.Name)
				conditions.MarkFalse(b7machine, infrastructurev1beta1.PhysicalHostAssociatedCondition, infrastructurev1beta1.ReleasePhysicalHostFailedReason, clusterv1.ConditionSeverityWarning, "Failed to patch PhysicalHost %s for release: %v", physicalHost.Name, err.Error())
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
func (r *Beskar7MachineReconciler) findAndClaimOrGetAssociatedHost(ctx context.Context, logger logr.Logger, b7machine *infrastructurev1beta1.Beskar7Machine) (*infrastructurev1beta1.PhysicalHost, ctrl.Result, error) {
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
		foundHost := &infrastructurev1beta1.PhysicalHost{}
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
			if foundHost.Status.State != infrastructurev1beta1.StateProvisioned && foundHost.Status.State != infrastructurev1beta1.StateProvisioning {
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
	phList := &infrastructurev1beta1.PhysicalHostList{}
	listOpts := []client.ListOption{client.InNamespace(b7machine.Namespace)}
	// TODO: Add label selector if using labels for matching?
	if err := r.List(ctx, phList, listOpts...); err != nil {
		logger.Error(err, "Failed to list PhysicalHosts")
		return nil, ctrl.Result{}, err
	}

	var associatedHost *infrastructurev1beta1.PhysicalHost
	var availableHost *infrastructurev1beta1.PhysicalHost

	for i := range phList.Items {
		host := &phList.Items[i]

		// Check if this host is already claimed by us
		if host.Spec.ConsumerRef != nil && host.Spec.ConsumerRef.Name == b7machine.Name && host.Spec.ConsumerRef.Namespace == b7machine.Namespace {
			associatedHost = host
			logger.Info("Found PhysicalHost already associated via ConsumerRef", "PhysicalHost", associatedHost.Name)
			break // Found our host, no need to look further
		}

		// Check if this host is available (and remember the first one)
		if availableHost == nil && host.Spec.ConsumerRef == nil && host.Status.State == infrastructurev1beta1.StateAvailable {
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

			if err := rfClient.SetBootParameters(ctx, kernelParams); err != nil {
				logger.Error(err, "Failed to set boot parameters for RemoteConfig", "Params", kernelParams)
				return nil, ctrl.Result{}, err // Requeue
			}
			if err := rfClient.SetBootSourceISO(ctx, b7machine.Spec.ImageURL); err != nil {
				logger.Error(err, "Failed to set boot source ISO for RemoteConfig", "ImageURL", b7machine.Spec.ImageURL)
				return nil, ctrl.Result{}, err // Requeue
			}

		case "PreBakedISO":
			logger.Info("Configuring boot for PreBakedISO mode")
			if err := rfClient.SetBootParameters(ctx, nil); err != nil { // Clear any existing boot params
				logger.Error(err, "Failed to clear boot parameters for PreBakedISO")
				return nil, ctrl.Result{}, err // Requeue
			}
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
func (r *Beskar7MachineReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1beta1.Beskar7Machine{}).
		Watches(&infrastructurev1beta1.PhysicalHost{}, handler.EnqueueRequestsFromMapFunc(r.PhysicalHostToBeskar7Machine)).
		Watches(&clusterv1.Machine{}, handler.EnqueueRequestsFromMapFunc(r.MachineToBeskar7Machine)).
		Complete(r)
}

// PhysicalHostToBeskar7Machine maps PhysicalHost events to Beskar7Machine reconcile requests.
// When a PhysicalHost changes, find any Beskar7Machine that references it and trigger reconciliation.
func (r *Beskar7MachineReconciler) PhysicalHostToBeskar7Machine(ctx context.Context, obj client.Object) []reconcile.Request {
	physicalHost, ok := obj.(*infrastructurev1beta1.PhysicalHost)
	if !ok {
		return nil
	}

	// Find Beskar7Machines that reference this PhysicalHost via ConsumerRef
	var requests []reconcile.Request

	// Check if this PhysicalHost has a ConsumerRef that points to a Beskar7Machine
	if physicalHost.Spec.ConsumerRef != nil {
		// The ConsumerRef should point to a Beskar7Machine
		if physicalHost.Spec.ConsumerRef.Kind == "Beskar7Machine" &&
			physicalHost.Spec.ConsumerRef.APIVersion == "infrastructure.cluster.x-k8s.io/v1beta1" {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: physicalHost.Spec.ConsumerRef.Namespace,
					Name:      physicalHost.Spec.ConsumerRef.Name,
				},
			})
		}
	}

	// Also find Beskar7Machines that might have already claimed this host
	// by listing all Beskar7Machines and checking their status or spec for references
	b7machineList := &infrastructurev1beta1.Beskar7MachineList{}
	if err := r.List(ctx, b7machineList, client.InNamespace(physicalHost.Namespace)); err != nil {
		// Log error but don't fail the mapping
		ctrl.LoggerFrom(ctx).Error(err, "Failed to list Beskar7Machines for PhysicalHost mapping", "PhysicalHost", physicalHost.Name)
		return requests
	}

	// Check if any Beskar7Machine has this PhysicalHost in its ProviderID or status
	for _, b7machine := range b7machineList.Items {
		shouldReconcile := false

		// Check if the ProviderID matches this PhysicalHost
		if b7machine.Spec.ProviderID != nil {
			namespace, name, err := parseProviderID(*b7machine.Spec.ProviderID)
			if err == nil && namespace == physicalHost.Namespace && name == physicalHost.Name {
				shouldReconcile = true
			}
		}

		if shouldReconcile {
			// Avoid duplicates
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: b7machine.Namespace,
					Name:      b7machine.Name,
				},
			}

			// Check if we already have this request
			found := false
			for _, existingReq := range requests {
				if existingReq == req {
					found = true
					break
				}
			}
			if !found {
				requests = append(requests, req)
			}
		}
	}

	ctrl.LoggerFrom(ctx).V(1).Info("PhysicalHost change mapped to Beskar7Machine reconcile requests",
		"PhysicalHost", physicalHost.Name, "requests", len(requests))
	return requests
}

// MachineToBeskar7Machine maps Machine events to Beskar7Machine reconcile requests.
// When a CAPI Machine changes, find the corresponding Beskar7Machine and trigger reconciliation.
func (r *Beskar7MachineReconciler) MachineToBeskar7Machine(ctx context.Context, obj client.Object) []reconcile.Request {
	machine, ok := obj.(*clusterv1.Machine)
	if !ok {
		return nil
	}

	// Find the Beskar7Machine that is owned by this Machine
	// The Beskar7Machine should have the Machine as its owner reference
	b7machineList := &infrastructurev1beta1.Beskar7MachineList{}
	if err := r.List(ctx, b7machineList, client.InNamespace(machine.Namespace)); err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "Failed to list Beskar7Machines for Machine mapping", "Machine", machine.Name)
		return nil
	}

	var requests []reconcile.Request
	for _, b7machine := range b7machineList.Items {
		// Check if this Beskar7Machine is owned by the Machine
		for _, ownerRef := range b7machine.OwnerReferences {
			if ownerRef.Kind == "Machine" &&
				ownerRef.APIVersion == "cluster.x-k8s.io/v1beta1" &&
				ownerRef.Name == machine.Name &&
				ownerRef.UID == machine.UID {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: b7machine.Namespace,
						Name:      b7machine.Name,
					},
				})
				break // Found the match, no need to check other owner refs
			}
		}
	}

	ctrl.LoggerFrom(ctx).V(1).Info("Machine change mapped to Beskar7Machine reconcile requests",
		"Machine", machine.Name, "requests", len(requests))
	return requests
}

// getRedfishClientForHost retrieves credentials and establishes a connection
// to the Redfish endpoint specified in the PhysicalHost spec.
func (r *Beskar7MachineReconciler) getRedfishClientForHost(ctx context.Context, logger logr.Logger, physicalHost *infrastructurev1beta1.PhysicalHost) (redfish.Client, error) {
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

	// --- Connect to Redfish ---
	clientFactory := r.RedfishClientFactory
	if clientFactory == nil {
		log.Info("RedfishClientFactory not provided, using default internalredfish.NewClient")
		clientFactory = redfish.NewClient
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
