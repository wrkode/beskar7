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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
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
		return ctrl.Result{}, err
	}
	if !result.IsZero() {
		logger.Info("Requeuing requested by findAndClaimOrGetAssociatedHost")
		return result, nil
	}
	if physicalHost == nil {
		// Should not happen if result is zero and err is nil, but check defensively.
		logger.Info("No associated or available PhysicalHost found, requeuing")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	logger = logger.WithValues("physicalhost", physicalHost.Name)
	logger.Info("Successfully associated with PhysicalHost")

	// Reconcile based on PhysicalHost status
	switch physicalHost.Status.State {
	case infrastructurev1alpha1.StateProvisioned:
		logger.Info("Associated PhysicalHost is Provisioned")
		// Set ProviderID if not already set
		currentProviderID := providerID(physicalHost.Namespace, physicalHost.Name)
		if b7machine.Spec.ProviderID == nil || *b7machine.Spec.ProviderID != currentProviderID {
			logger.Info("Setting ProviderID", "ProviderID", currentProviderID)
			b7machine.Spec.ProviderID = &currentProviderID
		}
		// Set machine status
		b7machine.Status.Ready = true
		phase := "Provisioned" // TODO: Use constants for phases
		b7machine.Status.Phase = &phase
		// TODO: Set addresses from physicalHost.Status?
		logger.Info("Beskar7Machine is Ready")

	case infrastructurev1alpha1.StateProvisioning:
		logger.Info("Waiting for associated PhysicalHost to finish provisioning")
		phase := "Provisioning"
		b7machine.Status.Phase = &phase
		b7machine.Status.Ready = false
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil

	case infrastructurev1alpha1.StateAvailable, infrastructurev1alpha1.StateClaimed, infrastructurev1alpha1.StateEnrolling:
		logger.Info("Waiting for associated PhysicalHost to start/complete provisioning", "hostState", physicalHost.Status.State)
		phase := "Associating" // Or Pending? Claimed?
		b7machine.Status.Phase = &phase
		b7machine.Status.Ready = false
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil

	case infrastructurev1alpha1.StateError:
		logger.Error(nil, "Associated PhysicalHost is in error state", "errorMessage", physicalHost.Status.ErrorMessage)
		// TODO: Maybe set a condition on Beskar7Machine?
		phase := "Failed"
		b7machine.Status.Phase = &phase
		b7machine.Status.Ready = false
		// No automatic requeue, needs intervention or PhysicalHost to recover.
		return ctrl.Result{}, nil

	default:
		logger.Info("Associated PhysicalHost is in unknown or intermediate state", "hostState", physicalHost.Status.State)
		phase := "Pending"
		b7machine.Status.Phase = &phase
		b7machine.Status.Ready = false
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

// reconcileDelete handles the logic when the Beskar7Machine is marked for deletion.
func (r *Beskar7MachineReconciler) reconcileDelete(ctx context.Context, logger logr.Logger, b7machine *infrastructurev1alpha1.Beskar7Machine) (ctrl.Result, error) {
	logger.Info("Reconciling Beskar7Machine deletion")

	// TODO: Implement cleanup logic:
	// 1. Find the associated PhysicalHost (using ProviderID or maybe labels/annotations?)
	// 2. If found, release the host by clearing its ConsumerRef and BootISOSource.
	//    (This will trigger the PhysicalHost controller to deprovision/power off/eject media).
	// 3. Wait for the PhysicalHost controller to confirm cleanup?
	logger.Info("Deletion cleanup logic not yet implemented")

	// Beskar7Machine is deleted, remove the finalizer.
	if controllerutil.RemoveFinalizer(b7machine, Beskar7MachineFinalizer) {
		logger.Info("Removing finalizer")
		// Patching is handled by the deferred patch function.
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
		originalHost := availableHost.DeepCopy()

		availableHost.Spec.ConsumerRef = &corev1.ObjectReference{
			Kind:       b7machine.Kind,
			APIVersion: b7machine.APIVersion,
			Name:       b7machine.Name,
			Namespace:  b7machine.Namespace,
			UID:        b7machine.UID,
		}
		isoURL := b7machine.Spec.Image.URL
		availableHost.Spec.BootISOSource = &isoURL
		// TODO: Set UserDataSecretRef on PhysicalHost?

		hostPatchHelper, err := patch.NewHelper(originalHost, r.Client)
		if err != nil {
			logger.Error(err, "Failed to init patch helper for PhysicalHost", "PhysicalHost", availableHost.Name)
			return nil, ctrl.Result{}, err
		}
		if err := hostPatchHelper.Patch(ctx, availableHost); err != nil {
			logger.Error(err, "Failed to patch PhysicalHost to claim it", "PhysicalHost", availableHost.Name)
			return nil, ctrl.Result{}, err
		}

		logger.Info("Successfully claimed PhysicalHost by setting ConsumerRef and BootISOSource", "PhysicalHost", availableHost.Name)
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
