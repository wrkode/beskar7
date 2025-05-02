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
	"errors"
	"fmt"
	"time"

	"github.com/stmcginnis/gofish/redfish"
	infrastructurev1alpha1 "github.com/wrkode/beskar7/api/v1alpha1"
	internalredfish "github.com/wrkode/beskar7/internal/redfish"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	patch "sigs.k8s.io/cluster-api/util/patch"
)

const (
	// PhysicalHostFinalizer allows PhysicalHostReconciler to clean up resources associated with PhysicalHost before removing it from the apiserver.
	PhysicalHostFinalizer = "physicalhost.infrastructure.cluster.x-k8s.io"
)

// PhysicalHostReconciler reconciles a PhysicalHost object
type PhysicalHostReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	// TODO: Inject Redfish client factory for better testing
}

//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=physicalhosts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=physicalhosts/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=physicalhosts/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch // Needed for Redfish credentials

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the PhysicalHost object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.4/pkg/reconcile
func (r *PhysicalHostReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, reterr error) {
	logger := log.FromContext(ctx).WithValues("physicalhost", req.NamespacedName)
	logger.Info("Starting reconciliation")

	// Fetch the PhysicalHost instance
	physicalHost := &infrastructurev1alpha1.PhysicalHost{}
	if err := r.Get(ctx, req.NamespacedName, physicalHost); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Unable to fetch PhysicalHost")
			return ctrl.Result{}, err
		}
		// Object not found, likely deleted after reconcile request.
		// Return and don't requeue
		logger.Info("PhysicalHost resource not found. Ignoring since object must be deleted")
		return ctrl.Result{}, nil
	}

	// Initialize patch helper
	patchHelper, err := patch.NewHelper(physicalHost, r.Client)
	if err != nil {
		logger.Error(err, "Failed to initialize patch helper")
		return ctrl.Result{}, err
	}

	// Always attempt to patch the status on reconcile exit.
	defer func() {
		if err := patchHelper.Patch(ctx, physicalHost); err != nil {
			logger.Error(err, "Failed to patch PhysicalHost status")
			if reterr == nil {
				reterr = err // Return the patching error if no other error occurred
			}
		}
		logger.Info("Finished reconciliation")
	}()

	// Handle deletion reconciliation
	if !physicalHost.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, physicalHost)
	}

	// Ensure the object has a finalizer for cleanup
	if controllerutil.AddFinalizer(physicalHost, PhysicalHostFinalizer) {
		logger.Info("Adding Finalizer")
		// Patch instantly adds the finalizer, no need for r.Update
		// Let the deferred patch handle saving.
		return ctrl.Result{Requeue: true}, nil // Requeue needed after adding finalizer
	}

	logger.Info("Reconciling PhysicalHost create/update")

	// --- Fetch Redfish Credentials ---
	secretName := physicalHost.Spec.RedfishConnection.CredentialsSecretRef
	if secretName == "" {
		logger.Error(nil, "CredentialsSecretRef is not set in PhysicalHost spec")
		// TODO: Update status condition to indicate missing credentials
		return ctrl.Result{}, errors.New("CredentialsSecretRef is not set") // Return error to requeue
	}

	credentialsSecret := &corev1.Secret{}
	secretKey := client.ObjectKey{
		Namespace: physicalHost.Namespace,
		Name:      secretName,
	}
	if err := r.Get(ctx, secretKey, credentialsSecret); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Failed to fetch credentials secret", "SecretName", secretName)
			return ctrl.Result{}, err
		}
		// Secret not found
		logger.Error(err, "Credentials secret not found", "SecretName", secretName)
		// TODO: Update status condition to indicate missing secret
		return ctrl.Result{RequeueAfter: time.Minute}, nil // Requeue after a delay
	}

	usernameBytes, ok := credentialsSecret.Data["username"]
	if !ok {
		logger.Error(nil, "Username not found in credentials secret", "SecretName", secretName)
		// TODO: Update status condition
		return ctrl.Result{}, errors.New("username missing in secret")
	}
	passwordBytes, ok := credentialsSecret.Data["password"]
	if !ok {
		logger.Error(nil, "Password not found in credentials secret", "SecretName", secretName)
		// TODO: Update status condition
		return ctrl.Result{}, errors.New("password missing in secret")
	}

	username := string(usernameBytes)
	password := string(passwordBytes)
	logger.Info("Successfully fetched Redfish credentials")
	// --- End Fetch Redfish Credentials ---

	// --- Connect to Redfish ---
	insecure := false
	if physicalHost.Spec.RedfishConnection.InsecureSkipVerify != nil {
		insecure = *physicalHost.Spec.RedfishConnection.InsecureSkipVerify
	}
	rfClient, err := internalredfish.NewClient(ctx, physicalHost.Spec.RedfishConnection.Address, username, password, insecure)
	if err != nil {
		logger.Error(err, "Failed to create Redfish client")
		physicalHost.Status.Ready = false
		physicalHost.Status.State = "Error"
		physicalHost.Status.ErrorMessage = fmt.Sprintf("Failed to connect: %v", err)
		// No need to requeue immediately, wait for user intervention or next sync
		return ctrl.Result{RequeueAfter: time.Minute * 2}, nil
	}
	defer rfClient.Close(ctx)
	logger.Info("Successfully connected to Redfish endpoint")
	// --- End Connect to Redfish ---

	// --- Reconcile State ---
	// Get current state from Redfish
	systemInfo, rfErr := rfClient.GetSystemInfo(ctx)
	powerState, psErr := rfClient.GetPowerState(ctx)

	// Prioritize updating Status with observed state even if errors occurred
	if systemInfo != nil {
		physicalHost.Status.HardwareDetails = &infrastructurev1alpha1.HardwareDetails{
			Manufacturer: systemInfo.Manufacturer,
			Model:        systemInfo.Model,
			SerialNumber: systemInfo.SerialNumber,
			Status:       systemInfo.Status,
		}
	} else {
		physicalHost.Status.HardwareDetails = nil // Clear if we couldn't fetch
	}
	if psErr == nil {
		physicalHost.Status.ObservedPowerState = powerState
	} // Otherwise, retain the last known state

	// Handle Redfish query errors after attempting to update status
	if rfErr != nil {
		logger.Error(rfErr, "Failed to get system info from Redfish")
		physicalHost.Status.Ready = false
		physicalHost.Status.State = infrastructurev1alpha1.StateError
		physicalHost.Status.ErrorMessage = fmt.Sprintf("Failed to get system info: %v", rfErr)
		return ctrl.Result{RequeueAfter: time.Minute}, nil // Requeue after delay
	}
	if psErr != nil {
		logger.Error(psErr, "Failed to get power state from Redfish")
		physicalHost.Status.Ready = false
		physicalHost.Status.State = infrastructurev1alpha1.StateError
		physicalHost.Status.ErrorMessage = fmt.Sprintf("Failed to get power state: %v", psErr)
		return ctrl.Result{RequeueAfter: time.Minute}, nil // Requeue after delay
	}

	// Determine desired state based on Spec
	if physicalHost.Spec.ConsumerRef == nil {
		// Host is available
		logger.Info("Host is available (no ConsumerRef)")
		physicalHost.Status.State = infrastructurev1alpha1.StateAvailable
		physicalHost.Status.Ready = true
		physicalHost.Status.ErrorMessage = ""
		// TODO: Optionally power off available hosts?
		// TODO: Ensure virtual media is ejected?
		// if err := rfClient.EjectVirtualMedia(ctx); err != nil { ... }
	} else {
		// Host is claimed
		if physicalHost.Spec.BootISOSource == nil || *physicalHost.Spec.BootISOSource == "" {
			// Consumer hasn't specified boot source yet
			logger.Info("Host is claimed but BootISOSource is not set", "consumer", physicalHost.Spec.ConsumerRef.Name)
			physicalHost.Status.State = infrastructurev1alpha1.StateClaimed
			physicalHost.Status.Ready = true // It's ready in the sense that it's claimed and waiting
			physicalHost.Status.ErrorMessage = ""
		} else {
			// Provisioning requested
			logger.Info("Provisioning requested", "consumer", physicalHost.Spec.ConsumerRef.Name, "isoURL", *physicalHost.Spec.BootISOSource)
			physicalHost.Status.State = infrastructurev1alpha1.StateProvisioning
			physicalHost.Status.Ready = false // Not ready until provisioned
			physicalHost.Status.ErrorMessage = ""

			// Set Boot ISO via VirtualMedia
			isoURL := *physicalHost.Spec.BootISOSource
			logger.Info("Attempting to set boot source ISO", "isoURL", isoURL)
			if err := rfClient.SetBootSourceISO(ctx, isoURL); err != nil {
				logger.Error(err, "Failed to set boot source ISO")
				physicalHost.Status.State = infrastructurev1alpha1.StateError
				physicalHost.Status.ErrorMessage = fmt.Sprintf("Failed to set boot source ISO: %v", err)
				return ctrl.Result{}, err // Return error to retry quickly
			}

			// TODO: Handle UserData - how is it passed to the ISO boot?
			// This might require a custom ISO build process or specific OS support (e.g., Kairos matching config label).

			// Power On the host
			logger.Info("Attempting to power on the host")
			if powerState != redfish.OnPowerState {
				if err := rfClient.SetPowerState(ctx, redfish.OnPowerState); err != nil {
					logger.Error(err, "Failed to set power state to On")
					physicalHost.Status.State = infrastructurev1alpha1.StateError
					physicalHost.Status.ErrorMessage = fmt.Sprintf("Failed to power on host: %v", err)
					return ctrl.Result{}, err // Return error to retry quickly
				}
				logger.Info("Host powered on successfully")
				physicalHost.Status.ObservedPowerState = redfish.OnPowerState // Optimistically update status with the constant
			} else {
				logger.Info("Host is already powered on")
			}

			// Provisioning steps initiated successfully
			physicalHost.Status.State = infrastructurev1alpha1.StateProvisioned // Set to Provisioned
			physicalHost.Status.Ready = true                                    // Consider ready once powered on with correct boot source
			logger.Info("Host provisioning initiated successfully, state set to Provisioned")
		}
	}
	// --- End Reconcile State ---

	// TODO: Add Provisioning logic (triggered by Beskar7Machine?)

	// Let the deferred function handle the status patch
	return ctrl.Result{}, nil
}

// reconcileDelete handles the cleanup when a PhysicalHost is marked for deletion.
func (r *PhysicalHostReconciler) reconcileDelete(ctx context.Context, physicalHost *infrastructurev1alpha1.PhysicalHost) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("physicalhost", physicalHost.Name)
	logger.Info("Reconciling PhysicalHost deletion")

	// Check if host is still provisioned or in use - we should only cleanup if unowned
	if physicalHost.Spec.ConsumerRef != nil {
		logger.Info("PhysicalHost still has ConsumerRef, waiting for it to be cleared before cleaning up", "ConsumerRef", physicalHost.Spec.ConsumerRef)
		// Requeue, maybe the Beskar7Machine delete hasn't finished releasing yet.
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
	// Check if state indicates it might still be in use (e.g., Provisioned, Provisioning)
	// Allow cleanup from Available, Error, Deprovisioning, Unknown, Enrolled states.
	isProvisioningOrProvisioned := physicalHost.Status.State == infrastructurev1alpha1.StateProvisioning || physicalHost.Status.State == infrastructurev1alpha1.StateProvisioned
	if isProvisioningOrProvisioned {
		logger.Info("PhysicalHost state is still Provisioning/Provisioned, requeuing before cleanup", "State", physicalHost.Status.State)
		// This might happen if ConsumerRef was cleared manually without going through B7Machine deletion.
		// We might want to force deprovisioning steps anyway, TBD.
		// For now, let's wait.
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Set state to Deprovisioning
	needsPatch := false
	if physicalHost.Status.State != infrastructurev1alpha1.StateDeprovisioning {
		logger.Info("Setting state to Deprovisioning")
		physicalHost.Status.State = infrastructurev1alpha1.StateDeprovisioning
		physicalHost.Status.Ready = false
		needsPatch = true
	}

	// --- Connect to Redfish for cleanup ---
	// Need credentials again for delete path
	secretName := physicalHost.Spec.RedfishConnection.CredentialsSecretRef
	username := ""
	password := ""
	if secretName != "" {
		credentialsSecret := &corev1.Secret{}
		secretKey := client.ObjectKey{Namespace: physicalHost.Namespace, Name: secretName}
		if err := r.Get(ctx, secretKey, credentialsSecret); err != nil {
			if client.IgnoreNotFound(err) != nil {
				logger.Error(err, "Failed to fetch credentials secret during delete", "SecretName", secretName)
				// Proceed without credentials? Might be okay for finalizer removal, but cleanup will fail.
			} else {
				logger.Info("Credentials secret not found during delete", "SecretName", secretName)
			}
		} else {
			if userBytes, ok := credentialsSecret.Data["username"]; ok {
				username = string(userBytes)
			}
			if passBytes, ok := credentialsSecret.Data["password"]; ok {
				password = string(passBytes)
			}
		}
	}
	if username == "" || password == "" {
		logger.Info("Missing credentials, skipping Redfish cleanup operations.")
	} else {
		insecure := false
		if physicalHost.Spec.RedfishConnection.InsecureSkipVerify != nil {
			insecure = *physicalHost.Spec.RedfishConnection.InsecureSkipVerify
		}
		rfClient, err := internalredfish.NewClient(ctx, physicalHost.Spec.RedfishConnection.Address, username, password, insecure)
		if err != nil {
			logger.Error(err, "Failed to create Redfish client during delete, skipping cleanup")
			// Proceed with finalizer removal, maybe log event?
		} else {
			defer rfClient.Close(ctx)
			logger.Info("Connected to Redfish for cleanup")

			// Eject Virtual Media
			logger.Info("Attempting to eject virtual media during delete")
			if err := rfClient.EjectVirtualMedia(ctx); err != nil {
				// Log error but continue cleanup
				logger.Error(err, "Failed to eject virtual media during delete")
			}

			// Power Off the host
			logger.Info("Attempting to power off host during delete")
			powerState, psErr := rfClient.GetPowerState(ctx)
			if (psErr == nil && powerState != redfish.OffPowerState) || psErr != nil {
				if psErr != nil {
					logger.Error(psErr, "Failed to get power state before power off attempt")
				}
				if err := rfClient.SetPowerState(ctx, redfish.OffPowerState); err != nil {
					// Log error but continue cleanup
					logger.Error(err, "Failed to power off host during delete")
				}
			} else {
				logger.Info("Host already powered off")
			}
			logger.Info("Redfish cleanup steps attempted")
		}
	}
	// --- End Redfish Connection ---

	// Patch status if changed
	if needsPatch {
		patchHelper, err := patch.NewHelper(physicalHost, r.Client) // Need a fresh helper or use the original?
		if err != nil {
			logger.Error(err, "Failed to initialize patch helper for deprovisioning state update")
			return ctrl.Result{}, err
		}
		if err := patchHelper.Patch(ctx, physicalHost); err != nil {
			logger.Error(err, "Failed to patch PhysicalHost status to Deprovisioning")
			return ctrl.Result{}, err
		}
	}

	// Cleanup finished (or skipped), remove the finalizer
	logger.Info("Removing finalizer")
	if controllerutil.RemoveFinalizer(physicalHost, PhysicalHostFinalizer) {
		logger.Info("Finalizer flag set for removal by controllerutil")
		// The actual patch happens in the main Reconcile loop's defer block.
		// We just need to ensure controllerutil.RemoveFinalizer modified the object.
		// Re-patching here might cause conflicts with the deferred patch.
	}

	logger.Info("Finished deletion reconciliation")
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PhysicalHostReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1alpha1.PhysicalHost{}).
		// TODO: Add Watches for Secrets or Beskar7Machines if needed
		Complete(r)
}
