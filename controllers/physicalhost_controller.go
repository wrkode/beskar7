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
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"github.com/stmcginnis/gofish/redfish"
	infrastructurev1beta1 "github.com/wrkode/beskar7/api/v1beta1"
	internalredfish "github.com/wrkode/beskar7/internal/redfish"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	conditions "sigs.k8s.io/cluster-api/util/conditions"
	patch "sigs.k8s.io/cluster-api/util/patch"

	internalmetrics "github.com/wrkode/beskar7/internal/metrics"
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
	startTime := time.Now()
	logger := log.FromContext(ctx).WithValues("physicalhost", req.NamespacedName)
	logger.Info("Starting reconciliation")

	// Initialize outcome tracking for metrics
	outcome := internalmetrics.ReconciliationOutcomeSuccess
	var errorType internalmetrics.ErrorType

	// Record reconciliation attempt and duration at the end
	defer func() {
		duration := time.Since(startTime)
		internalmetrics.RecordReconciliation("physicalhost", req.Namespace, outcome, duration)

		// Record errors if any occurred
		if reterr != nil {
			internalmetrics.RecordError("physicalhost", req.Namespace, errorType)
		}

		// Record requeue if needed
		if result.Requeue || result.RequeueAfter > 0 {
			reason := "general"
			if result.RequeueAfter > 0 {
				reason = "timed_requeue"
			}
			internalmetrics.RecordRequeue("physicalhost", req.Namespace, reason)
		}
	}()

	// Fetch the PhysicalHost instance
	physicalHost := &infrastructurev1beta1.PhysicalHost{}
	if err := r.Get(ctx, req.NamespacedName, physicalHost); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Unable to fetch PhysicalHost")
			outcome = internalmetrics.ReconciliationOutcomeError
			errorType = internalmetrics.ErrorTypeUnknown
			reterr = err
			return ctrl.Result{}, err
		}
		// Object not found, likely deleted after reconcile request.
		// Return and don't requeue
		logger.Info("PhysicalHost resource not found. Ignoring since object must be deleted")
		outcome = internalmetrics.ReconciliationOutcomeNotFound
		return ctrl.Result{}, nil
	}

	// Check if the PhysicalHost is paused
	if isPaused(physicalHost) {
		logger.Info("PhysicalHost reconciliation is paused")
		return ctrl.Result{}, nil
	}

	// Initialize patch helper
	patchHelper, err := patch.NewHelper(physicalHost, r.Client)
	if err != nil {
		logger.Error(err, "Failed to initialize patch helper")
		outcome = internalmetrics.ReconciliationOutcomeError
		errorType = internalmetrics.ErrorTypeUnknown
		reterr = err
		return ctrl.Result{}, err
	}

	// Track previous state for metrics
	previousState := physicalHost.Status.State

	// Always attempt to patch the status on reconcile exit.
	defer func() {
		// Set the summary condition before patching.
		conditions.SetSummary(physicalHost, conditions.WithConditions(
			infrastructurev1beta1.RedfishConnectionReadyCondition,
			// Add other conditions that should contribute to the overall Ready state, e.g.:
			// infrastructurev1beta1.HostAvailableCondition, (if not consumed)
			// infrastructurev1beta1.HostProvisionedCondition, (if consumed)
		))

		// Update state metrics if state changed
		currentState := physicalHost.Status.State
		if previousState != currentState {
			// Decrement previous state count
			if previousState != "" {
				internalmetrics.RecordPhysicalHostState(string(previousState), physicalHost.Namespace, -1)
			}
			// Increment new state count
			if currentState != "" {
				internalmetrics.RecordPhysicalHostState(string(currentState), physicalHost.Namespace, 1)
			}
		}

		if err := patchHelper.Patch(ctx, physicalHost); err != nil {
			logger.Error(err, "Failed to patch PhysicalHost status")
			outcome = internalmetrics.ReconciliationOutcomeError
			errorType = internalmetrics.ErrorTypeUnknown
			if reterr == nil {
				reterr = err // Return the patching error if no other error occurred
			}
		}
		logger.Info("Finished reconciliation")
	}()

	// Handle deletion reconciliation
	if !physicalHost.ObjectMeta.DeletionTimestamp.IsZero() {
		result, reterr = r.reconcileDelete(ctx, physicalHost)
		if reterr != nil {
			outcome = internalmetrics.ReconciliationOutcomeError
			errorType = internalmetrics.ErrorTypeUnknown
		}
		return result, reterr
	}

	// Handle non-deletion reconciliation
	result, reterr = r.reconcileNormal(ctx, logger, physicalHost)
	if reterr != nil {
		outcome = internalmetrics.ReconciliationOutcomeError
		// Try to determine error type
		if apierrors.IsTimeout(reterr) {
			errorType = internalmetrics.ErrorTypeTimeout
		} else if apierrors.IsNotFound(reterr) || apierrors.IsConflict(reterr) {
			errorType = internalmetrics.ErrorTypeTransient
		} else {
			errorType = internalmetrics.ErrorTypeUnknown
		}
	} else if result.Requeue || result.RequeueAfter > 0 {
		outcome = internalmetrics.ReconciliationOutcomeRequeue
	}

	return result, reterr
}

// reconcileNormal handles the logic when the PhysicalHost is not being deleted.
func (r *PhysicalHostReconciler) reconcileNormal(ctx context.Context, logger logr.Logger, physicalHost *infrastructurev1beta1.PhysicalHost) (ctrl.Result, error) {
	logger = logger.WithValues(
		"currentState", physicalHost.Status.State,
		"consumerRef", physicalHost.Spec.ConsumerRef,
		"bootIsoSource", physicalHost.Spec.BootISOSource,
	)
	logger.Info("Starting normal reconciliation")

	// Ensure the object has a finalizer for cleanup
	if controllerutil.AddFinalizer(physicalHost, PhysicalHostFinalizer) {
		logger.Info("Adding Finalizer")
		return ctrl.Result{Requeue: true}, nil
	}

	// --- Fetch Redfish Credentials ---
	secretName := physicalHost.Spec.RedfishConnection.CredentialsSecretRef
	if secretName == "" {
		// This is a permanent error, validated by the webhook. No need to requeue.
		logger.Info("Missing credentials reference, setting terminal condition")
		conditions.MarkFalse(physicalHost, infrastructurev1beta1.RedfishConnectionReadyCondition, infrastructurev1beta1.MissingCredentialsReason, clusterv1.ConditionSeverityError, "CredentialsSecretRef is not set in Spec")
		internalmetrics.RecordRedfishConnection(physicalHost.Namespace, internalmetrics.ProvisioningOutcomeFailed, internalmetrics.ErrorTypeValidation)
		return ctrl.Result{}, nil
	}

	credentialsSecret := &corev1.Secret{}
	secretKey := client.ObjectKey{Namespace: physicalHost.Namespace, Name: secretName}
	if err := r.Get(ctx, secretKey, credentialsSecret); err != nil {
		if apierrors.IsNotFound(err) {
			// Transient error: Secret might be created later. Requeue with backoff.
			logger.Info("Credentials secret not found, waiting for it to be created")
			conditions.MarkFalse(physicalHost, infrastructurev1beta1.RedfishConnectionReadyCondition, infrastructurev1beta1.SecretNotFoundReason, clusterv1.ConditionSeverityWarning, "Credentials secret %q not found, waiting.", secretName)
			internalmetrics.RecordRedfishConnection(physicalHost.Namespace, internalmetrics.ProvisioningOutcomeFailed, internalmetrics.ErrorTypeTransient)
			return ctrl.Result{}, err // Requeue with exponential backoff
		}
		// Other transient Get error
		logger.Error(err, "Failed to fetch credentials secret")
		conditions.MarkFalse(physicalHost, infrastructurev1beta1.RedfishConnectionReadyCondition, infrastructurev1beta1.SecretGetFailedReason, clusterv1.ConditionSeverityWarning, "Failed to get credentials secret: %s", err.Error())
		internalmetrics.RecordRedfishConnection(physicalHost.Namespace, internalmetrics.ProvisioningOutcomeFailed, internalmetrics.ErrorTypeTransient)
		return ctrl.Result{}, err
	}

	usernameBytes, okUser := credentialsSecret.Data["username"]
	passwordBytes, okPass := credentialsSecret.Data["password"]
	if !okUser || !okPass {
		// This is a permanent error. The secret content is invalid.
		logger.Info("Username or password missing in credentials secret, setting terminal condition")
		conditions.MarkFalse(physicalHost, infrastructurev1beta1.RedfishConnectionReadyCondition, infrastructurev1beta1.MissingSecretDataReason, clusterv1.ConditionSeverityError, "Username or password missing in credentials secret data")
		internalmetrics.RecordRedfishConnection(physicalHost.Namespace, internalmetrics.ProvisioningOutcomeFailed, internalmetrics.ErrorTypeValidation)
		return ctrl.Result{}, nil
	}
	username := string(usernameBytes)
	password := string(passwordBytes)
	// --- End Fetch Redfish Credentials ---

	// --- Connect to Redfish ---
	clientFactory := r.RedfishClientFactory
	if clientFactory == nil {
		clientFactory = internalredfish.NewClient
	}
	insecure := physicalHost.Spec.RedfishConnection.InsecureSkipVerify != nil && *physicalHost.Spec.RedfishConnection.InsecureSkipVerify
	rfClient, err := clientFactory(ctx, physicalHost.Spec.RedfishConnection.Address, username, password, insecure)
	if err != nil {
		// Transient error: Redfish endpoint might be temporarily unavailable. Requeue with backoff.
		logger.Error(err, "Failed to create Redfish client")
		conditions.MarkFalse(physicalHost, infrastructurev1beta1.RedfishConnectionReadyCondition, infrastructurev1beta1.RedfishConnectionFailedReason, clusterv1.ConditionSeverityWarning, "Failed to connect to Redfish: %v", err)
		physicalHost.Status.State = infrastructurev1beta1.StateError
		internalmetrics.RecordRedfishConnection(physicalHost.Namespace, internalmetrics.ProvisioningOutcomeFailed, internalmetrics.ErrorTypeConnection)
		return ctrl.Result{}, err
	}
	defer rfClient.Close(ctx)
	logger.Info("Successfully connected to Redfish endpoint")
	conditions.MarkTrue(physicalHost, infrastructurev1beta1.RedfishConnectionReadyCondition)
	internalmetrics.RecordRedfishConnection(physicalHost.Namespace, internalmetrics.ProvisioningOutcomeSuccess, "")
	// --- End Connect to Redfish ---

	// --- Reconcile State ---
	systemInfo, rfErr := rfClient.GetSystemInfo(ctx)
	powerState, psErr := rfClient.GetPowerState(ctx)

	// Update status with observed info first
	if systemInfo != nil {
		physicalHost.Status.HardwareDetails = infrastructurev1beta1.HardwareDetails{
			Manufacturer: systemInfo.Manufacturer,
			Model:        systemInfo.Model,
			SerialNumber: systemInfo.SerialNumber,
			Status: infrastructurev1beta1.HardwareStatus{
				Health:       string(systemInfo.Status.Health),
				HealthRollup: string(systemInfo.Status.HealthRollup),
				State:        string(systemInfo.Status.State),
			},
		}
		logger.Info("Updated hardware details", "manufacturer", systemInfo.Manufacturer, "model", systemInfo.Model, "serialNumber", systemInfo.SerialNumber, "status", systemInfo.Status.State)
	} else {
		physicalHost.Status.HardwareDetails = infrastructurev1beta1.HardwareDetails{}
		logger.Info("No hardware details available")
	}
	if psErr == nil {
		physicalHost.Status.ObservedPowerState = string(powerState)
		logger.Info("Updated power state", "powerState", powerState)
	}

	// Check for Redfish query errors - treat as transient
	if rfErr != nil {
		logger.Error(rfErr, "Failed to get system info from Redfish")
		conditions.MarkFalse(physicalHost, infrastructurev1beta1.HostAvailableCondition, infrastructurev1beta1.RedfishQueryFailedReason, clusterv1.ConditionSeverityWarning, "Failed to get system info: %v", rfErr)
		physicalHost.Status.State = infrastructurev1beta1.StateError
		internalmetrics.RecordRedfishQuery(physicalHost.Namespace, internalmetrics.ProvisioningOutcomeFailed, internalmetrics.ErrorTypeQuery)
		return ctrl.Result{}, rfErr // Requeue with backoff
	}
	if psErr != nil {
		logger.Error(psErr, "Failed to get power state from Redfish")
		conditions.MarkFalse(physicalHost, infrastructurev1beta1.HostAvailableCondition, infrastructurev1beta1.RedfishQueryFailedReason, clusterv1.ConditionSeverityWarning, "Failed to get power state: %v", psErr)
		physicalHost.Status.State = infrastructurev1beta1.StateError
		internalmetrics.RecordRedfishQuery(physicalHost.Namespace, internalmetrics.ProvisioningOutcomeFailed, internalmetrics.ErrorTypeQuery)
		return ctrl.Result{}, psErr // Requeue with backoff
	}

	// --- Address Detection ---
	// Attempt to detect network addresses from the Redfish endpoint
	// This is best effort - address detection failures should not prevent normal reconciliation
	addresses, addrErr := r.detectNetworkAddresses(ctx, logger, rfClient)
	if addrErr != nil {
		logger.V(1).Info("Failed to detect network addresses (non-fatal)", "error", addrErr)
		// Don't treat address detection failure as critical
		internalmetrics.RecordNetworkAddress(physicalHost.Namespace, internalmetrics.ProvisioningOutcomeFailed, internalmetrics.ErrorTypeAddress)
	} else if len(addresses) > 0 {
		physicalHost.Status.Addresses = addresses
		logger.Info("Updated network addresses", "addressCount", len(addresses))
		for _, addr := range addresses {
			logger.V(1).Info("Detected address", "type", addr.Type, "address", addr.Address)
		}
		internalmetrics.RecordNetworkAddress(physicalHost.Namespace, internalmetrics.ProvisioningOutcomeSuccess, "")
	} else {
		logger.V(1).Info("No network addresses detected from Redfish")
		internalmetrics.RecordNetworkAddress(physicalHost.Namespace, internalmetrics.ProvisioningOutcomeSuccess, "")
	}
	// --- End Address Detection ---

	// Determine desired state and update conditions
	if physicalHost.Spec.ConsumerRef == nil {
		// Host is being released or is available
		previousState := physicalHost.Status.State

		// If transitioning from a provisioned state, ensure host is powered off
		if previousState == infrastructurev1beta1.StateProvisioned || previousState == infrastructurev1beta1.StateProvisioning {
			logger.Info("Host being released from provisioned state, ensuring power off",
				"previousState", previousState, "currentPowerState", powerState)

			if powerState == redfish.OnPowerState {
				logger.Info("Powering off released host")
				if err := rfClient.SetPowerState(ctx, redfish.OffPowerState); err != nil {
					logger.Error(err, "Failed to power off released host")
					conditions.MarkFalse(physicalHost, infrastructurev1beta1.HostAvailableCondition,
						infrastructurev1beta1.PowerOffFailedReason, clusterv1.ConditionSeverityWarning,
						"Failed to power off released host: %v", err)
					internalmetrics.RecordPowerOperation(physicalHost.Namespace, internalmetrics.ProvisioningOutcomeFailed, internalmetrics.ErrorTypePower)
					// Don't return error - allow state transition but mark condition
				} else {
					logger.Info("Successfully powered off released host")
					physicalHost.Status.ObservedPowerState = string(redfish.OffPowerState)
					internalmetrics.RecordPowerOperation(physicalHost.Namespace, internalmetrics.ProvisioningOutcomeSuccess, "")
				}
			} else {
				logger.Info("Host already powered off")
			}

			// Eject any virtual media when releasing host
			logger.Info("Ejecting virtual media from released host")
			if err := rfClient.EjectVirtualMedia(ctx); err != nil {
				logger.Error(err, "Failed to eject virtual media from released host")
				// Don't fail the transition, just log
				internalmetrics.RecordVirtualMediaOperation(physicalHost.Namespace, internalmetrics.ProvisioningOutcomeFailed, internalmetrics.ErrorTypeVirtualMedia)
			} else {
				logger.Info("Successfully ejected virtual media from released host")
				internalmetrics.RecordVirtualMediaOperation(physicalHost.Namespace, internalmetrics.ProvisioningOutcomeSuccess, "")
			}
		}

		logger.Info("Host is available (no ConsumerRef)", "previousState", previousState, "newState", infrastructurev1beta1.StateAvailable)
		physicalHost.Status.State = infrastructurev1beta1.StateAvailable
		conditions.MarkTrue(physicalHost, infrastructurev1beta1.HostAvailableCondition)
		conditions.Delete(physicalHost, infrastructurev1beta1.HostProvisionedCondition)
		internalmetrics.RecordPhysicalHostState(string(infrastructurev1beta1.StateAvailable), physicalHost.Namespace, 1)
	} else {
		conditions.Delete(physicalHost, infrastructurev1beta1.HostAvailableCondition)
		if physicalHost.Spec.BootISOSource == nil || *physicalHost.Spec.BootISOSource == "" {
			logger.Info("Host is claimed but BootISOSource is not set", "previousState", physicalHost.Status.State, "newState", infrastructurev1beta1.StateClaimed)
			physicalHost.Status.State = infrastructurev1beta1.StateClaimed
			conditions.MarkFalse(physicalHost, infrastructurev1beta1.HostProvisionedCondition, infrastructurev1beta1.WaitingForBootInfoReason, clusterv1.ConditionSeverityInfo, "Waiting for BootISOSource to be set by consumer")
			internalmetrics.RecordPhysicalHostState(string(infrastructurev1beta1.StateClaimed), physicalHost.Namespace, 1)
		} else {
			logger.Info("Provisioning requested", "previousState", physicalHost.Status.State, "newState", infrastructurev1beta1.StateProvisioning)
			physicalHost.Status.State = infrastructurev1beta1.StateProvisioning
			conditions.MarkFalse(physicalHost, infrastructurev1beta1.HostProvisionedCondition, infrastructurev1beta1.ProvisioningReason, clusterv1.ConditionSeverityInfo, "Setting boot source and powering on")
			internalmetrics.RecordPhysicalHostState(string(infrastructurev1beta1.StateProvisioning), physicalHost.Namespace, 1)

			isoURL := *physicalHost.Spec.BootISOSource
			if err := rfClient.SetBootSourceISO(ctx, isoURL); err != nil {
				logger.Error(err, "Failed to set boot source ISO")
				conditions.MarkFalse(physicalHost, infrastructurev1beta1.HostProvisionedCondition, infrastructurev1beta1.SetBootISOFailedReason, clusterv1.ConditionSeverityError, "Failed to set boot source ISO: %v", err)
				physicalHost.Status.State = infrastructurev1beta1.StateError
				internalmetrics.RecordBootOperation(physicalHost.Namespace, internalmetrics.ProvisioningOutcomeFailed, internalmetrics.ErrorTypeBoot)
				return ctrl.Result{}, err
			}
			logger.Info("Successfully set boot source ISO", "isoURL", isoURL)
			internalmetrics.RecordBootOperation(physicalHost.Namespace, internalmetrics.ProvisioningOutcomeSuccess, "")

			// Enhanced power management with verification
			if powerState != redfish.OnPowerState {
				logger.Info("Attempting to power on host", "currentPowerState", powerState)
				if err := rfClient.SetPowerState(ctx, redfish.OnPowerState); err != nil {
					logger.Error(err, "Failed to set power state to On")
					conditions.MarkFalse(physicalHost, infrastructurev1beta1.HostProvisionedCondition, infrastructurev1beta1.PowerOnFailedReason, clusterv1.ConditionSeverityError, "Failed to power on host: %v", err)
					physicalHost.Status.State = infrastructurev1beta1.StateError
					internalmetrics.RecordPowerOperation(physicalHost.Namespace, internalmetrics.ProvisioningOutcomeFailed, internalmetrics.ErrorTypePower)
					return ctrl.Result{}, err
				}

				// Update observed power state optimistically
				physicalHost.Status.ObservedPowerState = string(redfish.OnPowerState)
				logger.Info("Successfully requested power on - host should be booting")

				// For power operations, we don't immediately verify since it takes time
				// The next reconciliation will pick up the actual power state
			} else {
				logger.Info("Host already powered on")
			}

			logger.Info("Host provisioning initiated successfully", "newState", infrastructurev1beta1.StateProvisioned)
			physicalHost.Status.State = infrastructurev1beta1.StateProvisioned
			conditions.MarkTrue(physicalHost, infrastructurev1beta1.HostProvisionedCondition)
			internalmetrics.RecordPhysicalHostState(string(infrastructurev1beta1.StateProvisioned), physicalHost.Namespace, 1)
		}
	}
	// --- End Reconcile State ---

	return ctrl.Result{}, nil
}

// reconcileDelete handles the cleanup when a PhysicalHost is marked for deletion.
func (r *PhysicalHostReconciler) reconcileDelete(ctx context.Context, physicalHost *infrastructurev1beta1.PhysicalHost) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("physicalhost", physicalHost.Name)
	logger.Info("Reconciling PhysicalHost deletion")

	// Mark overall HostProvisioned and HostAvailable conditions as False because the host is being deleted.
	conditions.MarkFalse(physicalHost, infrastructurev1beta1.HostProvisionedCondition, clusterv1.DeletingReason, clusterv1.ConditionSeverityInfo, "PhysicalHost is being deleted")
	conditions.MarkFalse(physicalHost, infrastructurev1beta1.HostAvailableCondition, clusterv1.DeletingReason, clusterv1.ConditionSeverityInfo, "PhysicalHost is being deleted")

	// Check if host is still provisioned or in use - we should only cleanup if unowned
	if physicalHost.Spec.ConsumerRef != nil {
		logger.Info("PhysicalHost still has ConsumerRef, waiting for it to be cleared before cleaning up", "ConsumerRef", physicalHost.Spec.ConsumerRef)
		conditions.MarkFalse(physicalHost, infrastructurev1beta1.HostProvisionedCondition, "WaitingForConsumerRelease", clusterv1.ConditionSeverityInfo, "Host is claimed by %s, waiting for release.", physicalHost.Spec.ConsumerRef.Name)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Set state to Deprovisioning and update condition
	if physicalHost.Status.State != infrastructurev1beta1.StateDeprovisioning {
		logger.Info("Setting state to Deprovisioning")
		physicalHost.Status.State = infrastructurev1beta1.StateDeprovisioning
		physicalHost.Status.Ready = false
		conditions.MarkFalse(physicalHost, infrastructurev1beta1.HostProvisionedCondition, infrastructurev1beta1.DeprovisioningReason, clusterv1.ConditionSeverityInfo, "Host deprovisioning started.")
		// No immediate patch, defer func in Reconcile will handle it.
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
		// If we can't connect, we can't confirm deprovisioning, but we should still allow finalizer removal.
		conditions.MarkFalse(physicalHost, infrastructurev1beta1.HostProvisionedCondition, infrastructurev1beta1.MissingCredentialsReason, clusterv1.ConditionSeverityWarning, "Missing Redfish credentials, cannot perform deprovisioning operations.")
		internalmetrics.RecordRedfishConnection(physicalHost.Namespace, internalmetrics.ProvisioningOutcomeFailed, internalmetrics.ErrorTypeConnection)
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
			conditions.MarkFalse(physicalHost, infrastructurev1beta1.HostProvisionedCondition, infrastructurev1beta1.RedfishConnectionFailedReason, clusterv1.ConditionSeverityError, "Failed to connect to Redfish for deprovisioning: %v", err)
			internalmetrics.RecordRedfishConnection(physicalHost.Namespace, internalmetrics.ProvisioningOutcomeFailed, internalmetrics.ErrorTypeConnection)
		} else {
			defer rfClient.Close(ctx)
			logger.Info("Connected to Redfish for cleanup")

			// Eject Virtual Media
			logger.Info("Attempting to eject virtual media during delete")
			if err := rfClient.EjectVirtualMedia(ctx); err != nil {
				logger.Error(err, "Failed to eject virtual media during delete")
				conditions.MarkFalse(physicalHost, infrastructurev1beta1.HostProvisionedCondition, infrastructurev1beta1.EjectMediaFailedReason, clusterv1.ConditionSeverityWarning, "Failed to eject virtual media: %v", err)
				// Log error but continue cleanup
				internalmetrics.RecordVirtualMediaOperation(physicalHost.Namespace, internalmetrics.ProvisioningOutcomeFailed, internalmetrics.ErrorTypeVirtualMedia)
			} else {
				// Optionally, mark a positive condition or clear the EjectMediaFailedReason if it was previously set.
			}

			// Power Off the host
			logger.Info("Attempting to power off host during delete")
			powerState, psErr := rfClient.GetPowerState(ctx)
			if psErr != nil {
				logger.Error(psErr, "Failed to get power state before power off attempt")
				conditions.MarkFalse(physicalHost, infrastructurev1beta1.HostProvisionedCondition, infrastructurev1beta1.RedfishQueryFailedReason, clusterv1.ConditionSeverityWarning, "Failed to get power state for power off: %v", psErr)
				// Continue with cleanup even if we can't check power state
			} else if powerState != redfish.OffPowerState {
				logger.Info("Host is powered on, attempting graceful power off", "currentPowerState", powerState)
				if err := rfClient.SetPowerState(ctx, redfish.OffPowerState); err != nil {
					logger.Error(err, "Failed to power off host during delete")
					conditions.MarkFalse(physicalHost, infrastructurev1beta1.HostProvisionedCondition, infrastructurev1beta1.PowerOffFailedReason, clusterv1.ConditionSeverityError, "Failed to power off host: %v", err)
					// Log error but continue cleanup - we don't want to block finalizer removal
					internalmetrics.RecordPowerOperation(physicalHost.Namespace, internalmetrics.ProvisioningOutcomeFailed, internalmetrics.ErrorTypePower)
				} else {
					logger.Info("Successfully requested power off during deletion")
					physicalHost.Status.ObservedPowerState = string(redfish.OffPowerState)
					// Note: We don't verify power state change here since this is cleanup
					// and we want to allow finalizer removal even if power off is slow
				}
			} else {
				logger.Info("Host already powered off")
			}
			logger.Info("Redfish cleanup steps attempted")
			internalmetrics.RecordDeprovisioningOperation(physicalHost.Namespace, internalmetrics.ProvisioningOutcomeSuccess, "")
		}
	}
	// --- End Redfish Connection ---

	// Cleanup finished (or skipped), remove the finalizer
	logger.Info("Removing finalizer")
	if controllerutil.RemoveFinalizer(physicalHost, PhysicalHostFinalizer) {
		logger.Info("Finalizer flag set for removal by controllerutil")
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PhysicalHostReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1beta1.PhysicalHost{}).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.SecretToPhysicalHosts),
		).
		Complete(r)
}

// SecretToPhysicalHosts maps a Secret event to reconcile requests for any PhysicalHost
// that references the Secret.
func (r *PhysicalHostReconciler) SecretToPhysicalHosts(ctx context.Context, obj client.Object) []reconcile.Request {
	log := log.FromContext(ctx).WithValues("mapping", "SecretToPhysicalHosts")
	secret, ok := obj.(*corev1.Secret)
	if !ok {
		log.Error(errors.New("unexpected type"), "Expected a Secret but got a %T", obj)
		return nil
	}

	phList := &infrastructurev1beta1.PhysicalHostList{}
	if err := r.List(ctx, phList, client.InNamespace(secret.Namespace)); err != nil {
		log.Error(err, "failed to list PhysicalHosts in namespace", "namespace", secret.Namespace)
		return nil
	}

	var requests []reconcile.Request
	for _, ph := range phList.Items {
		if ph.Spec.RedfishConnection.CredentialsSecretRef == secret.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      ph.Name,
					Namespace: ph.Namespace,
				},
			})
		}
	}
	if len(requests) > 0 {
		log.Info("Triggering reconciliation for PhysicalHosts due to secret change", "secret", secret.Name, "count", len(requests))
	}
	return requests
}

// detectNetworkAddresses attempts to retrieve network addresses from the Redfish endpoint.
func (r *PhysicalHostReconciler) detectNetworkAddresses(ctx context.Context, logger logr.Logger, rfClient internalredfish.Client) ([]clusterv1.MachineAddress, error) {
	logger.V(1).Info("Attempting to detect network addresses from Redfish")

	// Get network addresses from the Redfish client
	networkAddresses, err := rfClient.GetNetworkAddresses(ctx)
	if err != nil {
		logger.V(1).Info("Failed to retrieve network addresses from Redfish", "error", err)
		return nil, err
	}

	// Convert to Cluster API MachineAddress format
	machineAddresses := internalredfish.ConvertToMachineAddresses(networkAddresses)

	logger.V(1).Info("Successfully converted network addresses",
		"networkAddressCount", len(networkAddresses),
		"machineAddressCount", len(machineAddresses))

	return machineAddresses, nil
}
