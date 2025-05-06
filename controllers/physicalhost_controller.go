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
	"time"

	"github.com/go-logr/logr"
	"github.com/stmcginnis/gofish/redfish"
	infrastructurev1alpha1 "github.com/wrkode/beskar7/api/v1alpha1"
	internalredfish "github.com/wrkode/beskar7/internal/redfish"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	conditions "sigs.k8s.io/cluster-api/util/conditions"
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
	// RedfishClientFactory allows overriding the Redfish client creation for testing.
	RedfishClientFactory internalredfish.RedfishClientFactory
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
		// Set the summary condition before patching.
		conditions.SetSummary(physicalHost, conditions.WithConditions(
			infrastructurev1alpha1.RedfishConnectionReadyCondition,
			// Add other conditions that should contribute to the overall Ready state, e.g.:
			// infrastructurev1alpha1.HostAvailableCondition, (if not consumed)
			// infrastructurev1alpha1.HostProvisionedCondition, (if consumed)
		))

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

	// Handle non-deletion reconciliation
	return r.reconcileNormal(ctx, logger, physicalHost)
}

// reconcileNormal handles the logic when the PhysicalHost is not being deleted.
func (r *PhysicalHostReconciler) reconcileNormal(ctx context.Context, logger logr.Logger, physicalHost *infrastructurev1alpha1.PhysicalHost) (ctrl.Result, error) {
	logger.Info("Reconciling PhysicalHost create/update")

	// Ensure the object has a finalizer for cleanup
	if controllerutil.AddFinalizer(physicalHost, PhysicalHostFinalizer) {
		logger.Info("Adding Finalizer")
		// Let the deferred patch handle saving.
		return ctrl.Result{Requeue: true}, nil
	}

	// --- Fetch Redfish Credentials ---
	secretName := physicalHost.Spec.RedfishConnection.CredentialsSecretRef
	if secretName == "" {
		conditions.MarkFalse(physicalHost, infrastructurev1alpha1.RedfishConnectionReadyCondition, infrastructurev1alpha1.MissingCredentialsReason, clusterv1.ConditionSeverityError, "CredentialsSecretRef is not set in Spec")
		return ctrl.Result{}, errors.New("CredentialsSecretRef is not set")
	}
	credentialsSecret := &corev1.Secret{}
	secretKey := client.ObjectKey{Namespace: physicalHost.Namespace, Name: secretName}
	if err := r.Get(ctx, secretKey, credentialsSecret); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Failed to fetch credentials secret", "SecretName", secretName)
			// Assign error to variable first
			errMsg := err.Error()
			conditions.MarkFalse(physicalHost, infrastructurev1alpha1.RedfishConnectionReadyCondition, infrastructurev1alpha1.SecretGetFailedReason, clusterv1.ConditionSeverityWarning, "Failed to get credentials secret: %s", errMsg)
			return ctrl.Result{}, err
		}
		logger.Error(err, "Credentials secret not found", "SecretName", secretName)
		conditions.MarkFalse(physicalHost, infrastructurev1alpha1.RedfishConnectionReadyCondition, infrastructurev1alpha1.SecretNotFoundReason, clusterv1.ConditionSeverityWarning, "Credentials secret %q not found", secretName)
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}
	usernameBytes, okUser := credentialsSecret.Data["username"]
	passwordBytes, okPass := credentialsSecret.Data["password"]
	if !okUser || !okPass {
		errMsg := "Username or password missing in credentials secret data"
		logger.Error(nil, errMsg, "SecretName", secretName)
		conditions.MarkFalse(physicalHost, infrastructurev1alpha1.RedfishConnectionReadyCondition, infrastructurev1alpha1.MissingSecretDataReason, clusterv1.ConditionSeverityError, "Username or password missing in credentials secret data")
		return ctrl.Result{}, errors.New(errMsg)
	}
	username := string(usernameBytes)
	password := string(passwordBytes)
	// --- End Fetch Redfish Credentials ---

	// --- Connect to Redfish ---
	// Use the factory to create the client
	clientFactory := r.RedfishClientFactory
	if clientFactory == nil { // Default to real client if factory not set (should be set in main)
		clientFactory = internalredfish.NewClient
	}
	insecure := physicalHost.Spec.RedfishConnection.InsecureSkipVerify != nil && *physicalHost.Spec.RedfishConnection.InsecureSkipVerify
	rfClient, err := clientFactory(ctx, physicalHost.Spec.RedfishConnection.Address, username, password, insecure)
	if err != nil {
		logger.Error(err, "Failed to create Redfish client")
		conditions.MarkFalse(physicalHost, infrastructurev1alpha1.RedfishConnectionReadyCondition, infrastructurev1alpha1.RedfishConnectionFailedReason, clusterv1.ConditionSeverityError, "Failed to connect: %v", err.Error())
		physicalHost.Status.State = infrastructurev1alpha1.StateError
		return ctrl.Result{RequeueAfter: time.Minute * 2}, nil
	}
	defer rfClient.Close(ctx)
	logger.Info("Successfully connected to Redfish endpoint")
	conditions.MarkTrue(physicalHost, infrastructurev1alpha1.RedfishConnectionReadyCondition)
	// --- End Connect to Redfish ---

	// --- Reconcile State ---
	systemInfo, rfErr := rfClient.GetSystemInfo(ctx)
	powerState, psErr := rfClient.GetPowerState(ctx)

	// Update status with observed info first
	if systemInfo != nil {
		physicalHost.Status.HardwareDetails = &infrastructurev1alpha1.HardwareDetails{
			Manufacturer: systemInfo.Manufacturer,
			Model:        systemInfo.Model,
			SerialNumber: systemInfo.SerialNumber,
			Status:       systemInfo.Status,
		}
	} else {
		physicalHost.Status.HardwareDetails = nil
	}
	if psErr == nil {
		physicalHost.Status.ObservedPowerState = powerState
	}

	// Check for Redfish query errors
	if rfErr != nil {
		logger.Error(rfErr, "Failed to get system info from Redfish")
		conditions.MarkFalse(physicalHost, infrastructurev1alpha1.HostAvailableCondition, infrastructurev1alpha1.RedfishQueryFailedReason, clusterv1.ConditionSeverityWarning, "Failed to get system info: %v", rfErr.Error())
		conditions.MarkFalse(physicalHost, infrastructurev1alpha1.HostProvisionedCondition, infrastructurev1alpha1.RedfishQueryFailedReason, clusterv1.ConditionSeverityWarning, "Failed to get system info: %v", rfErr.Error())
		physicalHost.Status.State = infrastructurev1alpha1.StateError
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}
	if psErr != nil {
		logger.Error(psErr, "Failed to get power state from Redfish")
		conditions.MarkFalse(physicalHost, infrastructurev1alpha1.HostAvailableCondition, infrastructurev1alpha1.RedfishQueryFailedReason, clusterv1.ConditionSeverityWarning, "Failed to get power state: %v", psErr.Error())
		conditions.MarkFalse(physicalHost, infrastructurev1alpha1.HostProvisionedCondition, infrastructurev1alpha1.RedfishQueryFailedReason, clusterv1.ConditionSeverityWarning, "Failed to get power state: %v", psErr.Error())
		physicalHost.Status.State = infrastructurev1alpha1.StateError
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Determine desired state and update conditions
	if physicalHost.Spec.ConsumerRef == nil {
		logger.Info("Host is available (no ConsumerRef)")
		physicalHost.Status.State = infrastructurev1alpha1.StateAvailable
		conditions.MarkTrue(physicalHost, infrastructurev1alpha1.HostAvailableCondition)
		conditions.Delete(physicalHost, infrastructurev1alpha1.HostProvisionedCondition) // No longer provisioned for a consumer
		// TODO: Optionally power off / eject media
	} else {
		conditions.Delete(physicalHost, infrastructurev1alpha1.HostAvailableCondition) // No longer available
		if physicalHost.Spec.BootISOSource == nil || *physicalHost.Spec.BootISOSource == "" {
			logger.Info("Host is claimed but BootISOSource is not set")
			physicalHost.Status.State = infrastructurev1alpha1.StateClaimed
			conditions.MarkFalse(physicalHost, infrastructurev1alpha1.HostProvisionedCondition, infrastructurev1alpha1.WaitingForBootInfoReason, clusterv1.ConditionSeverityInfo, "Waiting for BootISOSource to be set by consumer")
		} else {
			logger.Info("Provisioning requested")
			physicalHost.Status.State = infrastructurev1alpha1.StateProvisioning
			conditions.MarkFalse(physicalHost, infrastructurev1alpha1.HostProvisionedCondition, infrastructurev1alpha1.ProvisioningReason, clusterv1.ConditionSeverityInfo, "Setting boot source and powering on")

			// Set Boot ISO via VirtualMedia
			isoURL := *physicalHost.Spec.BootISOSource
			if err := rfClient.SetBootSourceISO(ctx, isoURL); err != nil {
				logger.Error(err, "Failed to set boot source ISO")
				conditions.MarkFalse(physicalHost, infrastructurev1alpha1.HostProvisionedCondition, infrastructurev1alpha1.SetBootISOFailedReason, clusterv1.ConditionSeverityError, "Failed to set boot source ISO: %v", err.Error())
				physicalHost.Status.State = infrastructurev1alpha1.StateError
				return ctrl.Result{}, err
			}

			// Power On the host
			if powerState != redfish.OnPowerState {
				if err := rfClient.SetPowerState(ctx, redfish.OnPowerState); err != nil {
					logger.Error(err, "Failed to set power state to On")
					conditions.MarkFalse(physicalHost, infrastructurev1alpha1.HostProvisionedCondition, infrastructurev1alpha1.PowerOnFailedReason, clusterv1.ConditionSeverityError, "Failed to power on host: %v", err.Error())
					physicalHost.Status.State = infrastructurev1alpha1.StateError
					return ctrl.Result{}, err
				}
				physicalHost.Status.ObservedPowerState = redfish.OnPowerState // Optimistic update
			}

			// Provisioning steps initiated successfully
			physicalHost.Status.State = infrastructurev1alpha1.StateProvisioned
			conditions.MarkTrue(physicalHost, infrastructurev1alpha1.HostProvisionedCondition)
			logger.Info("Host provisioning initiated successfully, state set to Provisioned")
		}
	}
	// --- End Reconcile State ---

	return ctrl.Result{}, nil
}

// reconcileDelete handles the cleanup when a PhysicalHost is marked for deletion.
func (r *PhysicalHostReconciler) reconcileDelete(ctx context.Context, physicalHost *infrastructurev1alpha1.PhysicalHost) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("physicalhost", physicalHost.Name)
	logger.Info("Reconciling PhysicalHost deletion")

	// TODO: This delete logic needs refinement with conditions
	// Mark related conditions as deleting/false
	conditions.MarkFalse(physicalHost, infrastructurev1alpha1.HostAvailableCondition, clusterv1.DeletingReason, clusterv1.ConditionSeverityInfo, "Host is being deleted")
	conditions.MarkFalse(physicalHost, infrastructurev1alpha1.HostProvisionedCondition, clusterv1.DeletingReason, clusterv1.ConditionSeverityInfo, "Host is being deleted")

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
		needsPatch = true // Patch handled by defer in main Reconcile
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
		// Use the factory to create the client
		clientFactory := r.RedfishClientFactory
		if clientFactory == nil {
			clientFactory = internalredfish.NewClient
		}
		insecure := physicalHost.Spec.RedfishConnection.InsecureSkipVerify != nil && *physicalHost.Spec.RedfishConnection.InsecureSkipVerify
		rfClient, err := clientFactory(ctx, physicalHost.Spec.RedfishConnection.Address, username, password, insecure)
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
		// Patching handled by defer in main Reconcile
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PhysicalHostReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1alpha1.PhysicalHost{}).
		// TODO: Add Watches for Secrets or Beskar7Machines if needed
		Complete(r)
}
