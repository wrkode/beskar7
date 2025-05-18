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

	"github.com/stmcginnis/gofish/redfish"
	infrastructurev1alpha1 "github.com/wrkode/beskar7/api/v1alpha1"
	"github.com/wrkode/beskar7/internal/config"
	beskarerrors "github.com/wrkode/beskar7/internal/errors"
	"github.com/wrkode/beskar7/internal/recovery"
	internalredfish "github.com/wrkode/beskar7/internal/redfish"
	"github.com/wrkode/beskar7/internal/retry"
	"github.com/wrkode/beskar7/internal/statemachine"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
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
	// StateMachine manages the state transitions for PhysicalHost
	StateMachine statemachine.StateMachine
	// RecoveryManager handles error recovery strategies
	RecoveryManager *recovery.RecoveryManager
	// Logger for the controller
	Logger *zap.Logger
	// Config holds the controller configuration
	Config *config.Config
}

//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=physicalhosts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=physicalhosts/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=physicalhosts/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch // Needed for Redfish credentials

// NewPhysicalHostReconciler creates a new PhysicalHostReconciler
func NewPhysicalHostReconciler(client client.Client, scheme *runtime.Scheme, redfishClientFactory internalredfish.RedfishClientFactory) *PhysicalHostReconciler {
	logger, err := zap.NewProduction()
	if err != nil {
		// If we can't create a production logger, use a development logger
		logger, _ = zap.NewDevelopment()
	}
	return &PhysicalHostReconciler{
		Client:               client,
		Scheme:               scheme,
		RedfishClientFactory: redfishClientFactory,
		StateMachine:         statemachine.NewPhysicalHostStateMachine(),
		RecoveryManager:      recovery.NewRecoveryManager(logger, nil),
		Logger:               logger,
	}
}

// mapStateMachineStateToPhysicalHostState maps a state machine state to a PhysicalHost state
func mapStateMachineStateToPhysicalHostState(state statemachine.State) infrastructurev1alpha1.PhysicalHostProvisioningState {
	switch statemachine.PhysicalHostState(state) {
	case statemachine.PhysicalHostStateInitial:
		return infrastructurev1alpha1.StateNone
	case statemachine.PhysicalHostStateDiscovering:
		return infrastructurev1alpha1.StateEnrolling
	case statemachine.PhysicalHostStateAvailable:
		return infrastructurev1alpha1.StateAvailable
	case statemachine.PhysicalHostStateClaimed:
		return infrastructurev1alpha1.StateClaimed
	case statemachine.PhysicalHostStateProvisioning:
		return infrastructurev1alpha1.StateProvisioning
	case statemachine.PhysicalHostStateProvisioned:
		return infrastructurev1alpha1.StateProvisioned
	case statemachine.PhysicalHostStateError:
		return infrastructurev1alpha1.StateError
	case statemachine.PhysicalHostStateDeprovisioning:
		return infrastructurev1alpha1.StateDeprovisioning
	default:
		return infrastructurev1alpha1.StateUnknown
	}
}

// mapPhysicalHostStateToStateMachineState maps a PhysicalHost state to a state machine state
func mapPhysicalHostStateToStateMachineState(state infrastructurev1alpha1.PhysicalHostProvisioningState) statemachine.State {
	switch state {
	case infrastructurev1alpha1.StateNone:
		return statemachine.ConvertState(statemachine.PhysicalHostStateInitial)
	case infrastructurev1alpha1.StateEnrolling:
		return statemachine.ConvertState(statemachine.PhysicalHostStateDiscovering)
	case infrastructurev1alpha1.StateAvailable:
		return statemachine.ConvertState(statemachine.PhysicalHostStateAvailable)
	case infrastructurev1alpha1.StateClaimed:
		return statemachine.ConvertState(statemachine.PhysicalHostStateClaimed)
	case infrastructurev1alpha1.StateProvisioning:
		return statemachine.ConvertState(statemachine.PhysicalHostStateProvisioning)
	case infrastructurev1alpha1.StateProvisioned:
		return statemachine.ConvertState(statemachine.PhysicalHostStateProvisioned)
	case infrastructurev1alpha1.StateError:
		return statemachine.ConvertState(statemachine.PhysicalHostStateError)
	case infrastructurev1alpha1.StateDeprovisioning:
		return statemachine.ConvertState(statemachine.PhysicalHostStateDeprovisioning)
	default:
		return statemachine.ConvertState(statemachine.PhysicalHostStateError)
	}
}

// getRedfishClient creates a Redfish client using the credentials from the secret.
func (r *PhysicalHostReconciler) getRedfishClient(ctx context.Context, physicalHost *infrastructurev1alpha1.PhysicalHost) (internalredfish.Client, error) {
	secretName := physicalHost.Spec.RedfishConnection.CredentialsSecretRef
	if secretName == "" {
		return nil, beskarerrors.NewCredentialsError("", "CredentialsSecretRef is not set", nil)
	}

	credentialsSecret := &corev1.Secret{}
	secretKey := client.ObjectKey{Namespace: physicalHost.Namespace, Name: secretName}
	if err := r.Get(ctx, secretKey, credentialsSecret); err != nil {
		return nil, beskarerrors.NewCredentialsError(secretName, "failed to get credentials secret", err)
	}

	usernameBytes, okUser := credentialsSecret.Data["username"]
	passwordBytes, okPass := credentialsSecret.Data["password"]
	if !okUser || !okPass {
		return nil, beskarerrors.NewCredentialsError(secretName, "username or password missing in credentials secret data", nil)
	}

	username := string(usernameBytes)
	password := string(passwordBytes)

	// Create Redfish client
	clientFactory := r.RedfishClientFactory
	if clientFactory == nil {
		clientFactory = internalredfish.NewClient
	}

	insecure := physicalHost.Spec.RedfishConnection.InsecureSkipVerify != nil && *physicalHost.Spec.RedfishConnection.InsecureSkipVerify
	client, err := clientFactory(ctx, physicalHost.Spec.RedfishConnection.Address, username, password, insecure)
	if err != nil {
		return nil, beskarerrors.NewRedfishConnectionError(physicalHost.Spec.RedfishConnection.Address, "failed to create Redfish client", err)
	}

	return client, nil
}

// discoverHost attempts to discover the host via Redfish
func (r *PhysicalHostReconciler) discoverHost(ctx context.Context, physicalHost *infrastructurev1alpha1.PhysicalHost, redfishClient internalredfish.Client) error {
	config := &retry.Config{
		InitialInterval: 2 * time.Second,
		MaxInterval:     1 * time.Minute,
		Multiplier:      2.0,
		MaxAttempts:     3,
		MaxElapsedTime:  5 * time.Minute,
	}

	var systemInfo *internalredfish.SystemInfo
	var powerState redfish.PowerState
	var err error

	// Retry getting system info
	err = retry.WithContext(ctx, config, func() error {
		systemInfo, err = redfishClient.GetSystemInfo(ctx)
		if err != nil {
			if beskarerrors.IsRetryableError(err) {
				return beskarerrors.NewRetryableError(err)
			}
			// Try to recover from the error
			if recoveryErr := r.RecoveryManager.AttemptRecovery(ctx, redfishClient, err); recoveryErr != nil {
				return beskarerrors.NewDiscoveryError(physicalHost.Name, "failed to get system info", err)
			}
			// If recovery was successful, retry the operation
			systemInfo, err = redfishClient.GetSystemInfo(ctx)
			if err != nil {
				return beskarerrors.NewDiscoveryError(physicalHost.Name, "failed to get system info after recovery", err)
			}
		}
		return nil
	})
	if err != nil {
		return beskarerrors.NewDiscoveryError(physicalHost.Name, "failed to get system info after retries", err)
	}

	// Retry getting power state
	err = retry.WithContext(ctx, config, func() error {
		powerState, err = redfishClient.GetPowerState(ctx)
		if err != nil {
			if beskarerrors.IsRetryableError(err) {
				return beskarerrors.NewRetryableError(err)
			}
			// Try to recover from the error
			if recoveryErr := r.RecoveryManager.AttemptRecovery(ctx, redfishClient, err); recoveryErr != nil {
				return beskarerrors.NewPowerStateError("unknown", "unknown", "failed to get power state", err)
			}
			// If recovery was successful, retry the operation
			powerState, err = redfishClient.GetPowerState(ctx)
			if err != nil {
				return beskarerrors.NewPowerStateError("unknown", "unknown", "failed to get power state after recovery", err)
			}
		}
		return nil
	})
	if err != nil {
		return beskarerrors.NewPowerStateError("unknown", "unknown", "failed to get power state after retries", err)
	}

	// Update status with discovered info
	physicalHost.Status.HardwareDetails = &infrastructurev1alpha1.HardwareDetails{
		Manufacturer: systemInfo.Manufacturer,
		Model:        systemInfo.Model,
		SerialNumber: systemInfo.SerialNumber,
		Status:       systemInfo.Status,
	}
	physicalHost.Status.ObservedPowerState = powerState
	physicalHost.Status.Ready = true

	return nil
}

// checkProvisioningStatus checks if the host has been successfully provisioned
func (r *PhysicalHostReconciler) checkProvisioningStatus(ctx context.Context, physicalHost *infrastructurev1alpha1.PhysicalHost, redfishClient internalredfish.Client) error {
	config := &retry.Config{
		InitialInterval: 2 * time.Second,
		MaxInterval:     1 * time.Minute,
		Multiplier:      2.0,
		MaxAttempts:     3,
		MaxElapsedTime:  5 * time.Minute,
	}

	// Check if boot source is set
	if physicalHost.Spec.BootISOSource == nil || *physicalHost.Spec.BootISOSource == "" {
		return beskarerrors.NewBootSourceError("", "BootISOSource is not set", nil)
	}

	// Retry setting boot source
	err := retry.WithContext(ctx, config, func() error {
		err := redfishClient.SetBootSourceISO(ctx, *physicalHost.Spec.BootISOSource)
		if err != nil {
			if beskarerrors.IsRetryableError(err) {
				return beskarerrors.NewRetryableError(err)
			}
			// Try to recover from the error
			if recoveryErr := r.RecoveryManager.AttemptRecovery(ctx, redfishClient, err); recoveryErr != nil {
				return beskarerrors.NewBootSourceError(*physicalHost.Spec.BootISOSource, "failed to set boot source", err)
			}
			// If recovery was successful, retry the operation
			err = redfishClient.SetBootSourceISO(ctx, *physicalHost.Spec.BootISOSource)
			if err != nil {
				return beskarerrors.NewBootSourceError(*physicalHost.Spec.BootISOSource, "failed to set boot source after recovery", err)
			}
		}
		return nil
	})
	if err != nil {
		return beskarerrors.NewBootSourceError(*physicalHost.Spec.BootISOSource, "failed to set boot source after retries", err)
	}

	// Retry getting power state
	var powerState redfish.PowerState
	err = retry.WithContext(ctx, config, func() error {
		powerState, err = redfishClient.GetPowerState(ctx)
		if err != nil {
			if beskarerrors.IsRetryableError(err) {
				return beskarerrors.NewRetryableError(err)
			}
			// Try to recover from the error
			if recoveryErr := r.RecoveryManager.AttemptRecovery(ctx, redfishClient, err); recoveryErr != nil {
				return beskarerrors.NewPowerStateError("unknown", "unknown", "failed to get power state", err)
			}
			// If recovery was successful, retry the operation
			powerState, err = redfishClient.GetPowerState(ctx)
			if err != nil {
				return beskarerrors.NewPowerStateError("unknown", "unknown", "failed to get power state after recovery", err)
			}
		}
		return nil
	})
	if err != nil {
		return beskarerrors.NewPowerStateError("unknown", "unknown", "failed to get power state after retries", err)
	}

	if powerState != redfish.OnPowerState {
		// Retry setting power state
		err = retry.WithContext(ctx, config, func() error {
			err := redfishClient.SetPowerState(ctx, redfish.OnPowerState)
			if err != nil {
				if beskarerrors.IsRetryableError(err) {
					return beskarerrors.NewRetryableError(err)
				}
				// Try to recover from the error
				if recoveryErr := r.RecoveryManager.AttemptRecovery(ctx, redfishClient, err); recoveryErr != nil {
					return beskarerrors.NewPowerStateError(string(powerState), string(redfish.OnPowerState), "failed to set power state", err)
				}
				// If recovery was successful, retry the operation
				err = redfishClient.SetPowerState(ctx, redfish.OnPowerState)
				if err != nil {
					return beskarerrors.NewPowerStateError(string(powerState), string(redfish.OnPowerState), "failed to set power state after recovery", err)
				}
			}
			return nil
		})
		if err != nil {
			return beskarerrors.NewPowerStateError(string(powerState), string(redfish.OnPowerState), "failed to set power state after retries", err)
		}
		physicalHost.Status.ObservedPowerState = redfish.OnPowerState
	}

	return nil
}

// deprovisionHost handles the deprovisioning of a PhysicalHost.
func (r *PhysicalHostReconciler) deprovisionHost(ctx context.Context, physicalHost *infrastructurev1alpha1.PhysicalHost, redfishClient internalredfish.Client) error {
	log := log.FromContext(ctx)

	// Eject Virtual Media
	log.Info("Attempting to eject virtual media during delete")
	if err := redfishClient.EjectVirtualMedia(ctx); err != nil {
		log.Error(err, "Failed to eject virtual media during delete")
		return beskarerrors.NewProvisioningError(physicalHost.Name, "eject_virtual_media", "failed to eject virtual media", err)
	}

	// Power Off the host
	log.Info("Attempting to power off host during delete")
	powerState, err := redfishClient.GetPowerState(ctx)
	if err != nil {
		log.Error(err, "Failed to get power state before power off attempt")
		return beskarerrors.NewPowerStateError("unknown", "Off", "failed to get power state for power off", err)
	}

	if powerState != "Off" {
		if err := redfishClient.SetPowerState(ctx, "Off"); err != nil {
			log.Error(err, "Failed to power off host during delete")
			return beskarerrors.NewPowerStateError(string(powerState), "Off", "failed to power off host", err)
		}
	} else {
		log.Info("Host already powered off")
	}

	return nil
}

// recoverFromError attempts to recover from an error state
func (r *PhysicalHostReconciler) recoverFromError(ctx context.Context, physicalHost *infrastructurev1alpha1.PhysicalHost) error {
	config := &retry.Config{
		InitialInterval: 5 * time.Second,
		MaxInterval:     5 * time.Minute,
		Multiplier:      2.0,
		MaxAttempts:     5,
		MaxElapsedTime:  15 * time.Minute,
	}

	// Try to recover from the error
	err := retry.WithContext(ctx, config, func() error {
		// For now, just try to rediscover the host
		redfishClient, err := r.getRedfishClient(ctx, physicalHost)
		if err != nil {
			if beskarerrors.IsRetryableError(err) {
				return beskarerrors.NewRetryableError(err)
			}
			return beskarerrors.NewRedfishConnectionError(physicalHost.Spec.RedfishConnection.Address, "failed to get Redfish client during recovery", err)
		}

		if err := r.discoverHost(ctx, physicalHost, redfishClient); err != nil {
			if beskarerrors.IsRetryableError(err) {
				return beskarerrors.NewRetryableError(err)
			}
			return beskarerrors.NewDiscoveryError(physicalHost.Name, "failed to rediscover host during recovery", err)
		}

		// If successful, transition back to available state
		if err := r.StateMachine.Transition(ctx, statemachine.ConvertEvent(statemachine.PhysicalHostEventDiscoverySucceeded)); err != nil {
			if beskarerrors.IsRetryableError(err) {
				return beskarerrors.NewRetryableError(err)
			}
			return beskarerrors.NewStateTransitionError(string(physicalHost.Status.State), string(infrastructurev1alpha1.StateAvailable), "failed to transition to available state during recovery", err)
		}
		physicalHost.Status.State = mapStateMachineStateToPhysicalHostState(statemachine.ConvertState(statemachine.PhysicalHostStateAvailable))
		return nil
	})

	return err
}

// updateStateTransition updates the state transition information in the PhysicalHost status
func (r *PhysicalHostReconciler) updateStateTransition(physicalHost *infrastructurev1alpha1.PhysicalHost, newState infrastructurev1alpha1.PhysicalHostProvisioningState, reason string) {
	now := metav1.Now()
	physicalHost.Status.LastStateTransitionTime = &now
	physicalHost.Status.LastStateTransitionReason = reason
	physicalHost.Status.State = newState

	// Clear error details if transitioning out of error state
	if physicalHost.Status.State != infrastructurev1alpha1.StateError {
		physicalHost.Status.ErrorDetails = nil
		physicalHost.Status.ErrorMessage = ""
	}

	// Clear progress if transitioning to a non-progress state
	if newState != infrastructurev1alpha1.StateProvisioning &&
		newState != infrastructurev1alpha1.StateDeprovisioning {
		physicalHost.Status.Progress = nil
	}
}

// updateErrorDetails updates the error details in the PhysicalHost status
func (r *PhysicalHostReconciler) updateErrorDetails(physicalHost *infrastructurev1alpha1.PhysicalHost, errType, code, message string) {
	now := metav1.Now()
	if physicalHost.Status.ErrorDetails == nil {
		physicalHost.Status.ErrorDetails = &infrastructurev1alpha1.ErrorDetails{
			Type:            errType,
			Code:            code,
			Message:         message,
			LastAttemptTime: &now,
			RetryCount:      1,
		}
	} else {
		physicalHost.Status.ErrorDetails.LastAttemptTime = &now
		physicalHost.Status.ErrorDetails.RetryCount++
		physicalHost.Status.ErrorDetails.Message = message
	}
	physicalHost.Status.ErrorMessage = message
}

// updateOperationProgress updates the operation progress in the PhysicalHost status
func (r *PhysicalHostReconciler) updateOperationProgress(physicalHost *infrastructurev1alpha1.PhysicalHost, operation, currentStep string, currentStepNumber, totalSteps int32) {
	now := metav1.Now()
	if physicalHost.Status.Progress == nil {
		physicalHost.Status.Progress = &infrastructurev1alpha1.OperationProgress{
			Operation:         operation,
			CurrentStep:       currentStep,
			CurrentStepNumber: currentStepNumber,
			TotalSteps:        totalSteps,
			StartTime:         &now,
			LastUpdateTime:    &now,
		}
	} else {
		physicalHost.Status.Progress.CurrentStep = currentStep
		physicalHost.Status.Progress.CurrentStepNumber = currentStepNumber
		physicalHost.Status.Progress.LastUpdateTime = &now
	}
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *PhysicalHostReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the PhysicalHost instance
	physicalHost := &infrastructurev1alpha1.PhysicalHost{}
	if err := r.Get(ctx, req.NamespacedName, physicalHost); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Initialize the patch helper
	patchHelper, err := patch.NewHelper(physicalHost, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Always attempt to Patch the PhysicalHost object and status after each reconciliation.
	defer func() {
		if err := patchHelper.Patch(ctx, physicalHost); err != nil {
			log.Error(err, "failed to patch PhysicalHost")
		}
	}()

	// Handle deletion reconciliation loop.
	if !physicalHost.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, physicalHost)
	}

	// Handle normal reconciliation loop.
	return r.reconcileNormal(ctx, physicalHost)
}

// reconcileNormal handles the normal reconciliation loop for PhysicalHost.
func (r *PhysicalHostReconciler) reconcileNormal(ctx context.Context, physicalHost *infrastructurev1alpha1.PhysicalHost) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// If the PhysicalHost doesn't have our finalizer, add it.
	if !controllerutil.ContainsFinalizer(physicalHost, PhysicalHostFinalizer) {
		controllerutil.AddFinalizer(physicalHost, PhysicalHostFinalizer)
	}

	// Get the current state from the PhysicalHost status
	currentState := physicalHost.Status.State
	if currentState == "" {
		currentState = infrastructurev1alpha1.StateNone
	}

	// Map PhysicalHost state to state machine state
	stateMachineState := mapPhysicalHostStateToStateMachineState(currentState)

	// Check if we can transition to the next state
	switch statemachine.PhysicalHostState(stateMachineState) {
	case statemachine.PhysicalHostStateInitial:
		// Start discovery process
		if err := r.StateMachine.Transition(ctx, statemachine.ConvertEvent(statemachine.PhysicalHostEventStartDiscovery)); err != nil {
			log.Error(err, "failed to start discovery")
			r.updateErrorDetails(physicalHost, "StateTransitionError", "DISCOVERY_START_FAILED", err.Error())
			return ctrl.Result{}, beskarerrors.NewStateTransitionError(string(currentState), string(infrastructurev1alpha1.StateEnrolling), "failed to start discovery", err)
		}
		r.updateStateTransition(physicalHost, infrastructurev1alpha1.StateEnrolling, "Starting host discovery")
		r.updateOperationProgress(physicalHost, "Discovery", "Connecting to Redfish", 1, 3)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil

	case statemachine.PhysicalHostStateDiscovering:
		// Try to discover the host
		redfishClient, err := r.getRedfishClient(ctx, physicalHost)
		if err != nil {
			log.Error(err, "failed to get Redfish client")
			r.updateErrorDetails(physicalHost, "RedfishConnectionError", "CLIENT_CREATION_FAILED", err.Error())
			return ctrl.Result{}, err
		}

		if err := r.discoverHost(ctx, physicalHost, redfishClient); err != nil {
			log.Error(err, "failed to discover host")
			r.updateErrorDetails(physicalHost, "DiscoveryError", "HOST_DISCOVERY_FAILED", err.Error())
			return ctrl.Result{}, err
		}

		if err := r.StateMachine.Transition(ctx, statemachine.ConvertEvent(statemachine.PhysicalHostEventDiscoverySucceeded)); err != nil {
			log.Error(err, "failed to transition to available state")
			r.updateErrorDetails(physicalHost, "StateTransitionError", "DISCOVERY_SUCCESS_TRANSITION_FAILED", err.Error())
			return ctrl.Result{}, err
		}
		r.updateStateTransition(physicalHost, infrastructurev1alpha1.StateAvailable, "Host discovery completed successfully")
		conditions.MarkTrue(physicalHost, infrastructurev1alpha1.HostAvailableCondition)
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil

	case statemachine.PhysicalHostStateAvailable:
		// Check if host is claimed
		if physicalHost.Spec.ConsumerRef != nil {
			if err := r.StateMachine.Transition(ctx, statemachine.ConvertEvent(statemachine.PhysicalHostEventClaim)); err != nil {
				log.Error(err, "failed to transition to claimed state")
				r.updateErrorDetails(physicalHost, "StateTransitionError", "CLAIM_TRANSITION_FAILED", err.Error())
				return ctrl.Result{}, err
			}
			r.updateStateTransition(physicalHost, infrastructurev1alpha1.StateClaimed, "Host claimed by consumer")
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil

	case statemachine.PhysicalHostStateClaimed:
		// Check if we need to start provisioning
		if physicalHost.Spec.BootISOSource != nil && *physicalHost.Spec.BootISOSource != "" {
			if err := r.StateMachine.Transition(ctx, statemachine.ConvertEvent(statemachine.PhysicalHostEventStartProvisioning)); err != nil {
				log.Error(err, "failed to transition to provisioning state")
				r.updateErrorDetails(physicalHost, "StateTransitionError", "PROVISIONING_START_FAILED", err.Error())
				return ctrl.Result{}, err
			}
			r.updateStateTransition(physicalHost, infrastructurev1alpha1.StateProvisioning, "Starting host provisioning")
			r.updateOperationProgress(physicalHost, "Provisioning", "Setting boot source", 1, 4)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil

	case statemachine.PhysicalHostStateProvisioning:
		// Check provisioning status
		redfishClient, err := r.getRedfishClient(ctx, physicalHost)
		if err != nil {
			log.Error(err, "failed to get Redfish client")
			r.updateErrorDetails(physicalHost, "RedfishConnectionError", "CLIENT_CREATION_FAILED", err.Error())
			return ctrl.Result{}, err
		}

		// Update progress
		r.updateOperationProgress(physicalHost, "Provisioning", "Checking power state", 2, 4)

		if err := r.checkProvisioningStatus(ctx, physicalHost, redfishClient); err != nil {
			log.Error(err, "failed to check provisioning status")
			r.updateErrorDetails(physicalHost, "ProvisioningError", "PROVISIONING_CHECK_FAILED", err.Error())
			return ctrl.Result{}, err
		}

		// Update progress
		r.updateOperationProgress(physicalHost, "Provisioning", "Setting boot source", 3, 4)

		if err := r.StateMachine.Transition(ctx, statemachine.ConvertEvent(statemachine.PhysicalHostEventProvisioningSucceeded)); err != nil {
			log.Error(err, "failed to transition to provisioned state")
			r.updateErrorDetails(physicalHost, "StateTransitionError", "PROVISIONING_SUCCESS_TRANSITION_FAILED", err.Error())
			return ctrl.Result{}, err
		}

		r.updateStateTransition(physicalHost, infrastructurev1alpha1.StateProvisioned, "Host provisioning completed successfully")
		conditions.MarkTrue(physicalHost, infrastructurev1alpha1.HostProvisionedCondition)
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil

	case statemachine.PhysicalHostStateProvisioned:
		// Host is provisioned and ready
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil

	case statemachine.PhysicalHostStateError:
		// Attempt to recover from error state
		if err := r.recoverFromError(ctx, physicalHost); err != nil {
			log.Error(err, "failed to recover from error state")
			r.updateErrorDetails(physicalHost, "RecoveryError", "RECOVERY_FAILED", err.Error())
			return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
		}
		r.updateStateTransition(physicalHost, infrastructurev1alpha1.StateAvailable, "Recovered from error state")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil

	default:
		log.Error(nil, "unknown state", "state", currentState)
		r.updateErrorDetails(physicalHost, "StateError", "UNKNOWN_STATE", "Unknown state encountered")
		return ctrl.Result{}, beskarerrors.New("unknown state")
	}
}

// reconcileDelete handles the deletion of a PhysicalHost.
func (r *PhysicalHostReconciler) reconcileDelete(ctx context.Context, physicalHost *infrastructurev1alpha1.PhysicalHost) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Get the current state from the PhysicalHost status
	currentState := physicalHost.Status.State
	if currentState == "" {
		currentState = infrastructurev1alpha1.StateNone
	}

	// Map PhysicalHost state to state machine state
	stateMachineState := mapPhysicalHostStateToStateMachineState(currentState)

	// Check if we can transition to the next state
	switch statemachine.PhysicalHostState(stateMachineState) {
	case statemachine.PhysicalHostStateProvisioned:
		// Start deprovisioning
		if err := r.StateMachine.Transition(ctx, statemachine.ConvertEvent(statemachine.PhysicalHostEventStartDeprovisioning)); err != nil {
			log.Error(err, "failed to transition to deprovisioning state")
			r.updateErrorDetails(physicalHost, "StateTransitionError", "DEPROVISIONING_START_FAILED", err.Error())
			return ctrl.Result{}, beskarerrors.NewStateTransitionError(string(currentState), string(infrastructurev1alpha1.StateDeprovisioning), "failed to start deprovisioning", err)
		}
		r.updateStateTransition(physicalHost, infrastructurev1alpha1.StateDeprovisioning, "Starting host deprovisioning")
		r.updateOperationProgress(physicalHost, "Deprovisioning", "Ejecting virtual media", 1, 3)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil

	case statemachine.PhysicalHostStateDeprovisioning:
		// Check deprovisioning status
		redfishClient, err := r.getRedfishClient(ctx, physicalHost)
		if err != nil {
			log.Error(err, "failed to get Redfish client")
			r.updateErrorDetails(physicalHost, "RedfishConnectionError", "CLIENT_CREATION_FAILED", err.Error())
			return ctrl.Result{}, beskarerrors.NewRedfishConnectionError(physicalHost.Spec.RedfishConnection.Address, "failed to get Redfish client during deprovisioning", err)
		}

		// Update progress
		r.updateOperationProgress(physicalHost, "Deprovisioning", "Powering off host", 2, 3)

		if err := r.deprovisionHost(ctx, physicalHost, redfishClient); err != nil {
			log.Error(err, "failed to deprovision host")
			r.updateErrorDetails(physicalHost, "DeprovisioningError", "DEPROVISIONING_FAILED", err.Error())
			return ctrl.Result{RequeueAfter: 5 * time.Minute}, err
		}

		// Update progress
		r.updateOperationProgress(physicalHost, "Deprovisioning", "Cleaning up", 3, 3)

		if err := r.StateMachine.Transition(ctx, statemachine.ConvertEvent(statemachine.PhysicalHostEventDeprovisioningCompleted)); err != nil {
			log.Error(err, "failed to transition to available state")
			r.updateErrorDetails(physicalHost, "StateTransitionError", "DEPROVISIONING_COMPLETION_FAILED", err.Error())
			return ctrl.Result{}, beskarerrors.NewStateTransitionError(string(currentState), string(infrastructurev1alpha1.StateAvailable), "failed to complete deprovisioning", err)
		}

		r.updateStateTransition(physicalHost, infrastructurev1alpha1.StateAvailable, "Host deprovisioning completed successfully")
		conditions.MarkTrue(physicalHost, infrastructurev1alpha1.HostProvisionedCondition)

		// Remove finalizer
		controllerutil.RemoveFinalizer(physicalHost, PhysicalHostFinalizer)
		return ctrl.Result{}, nil

	default:
		// If we're not in a state that needs deprovisioning, just remove the finalizer
		controllerutil.RemoveFinalizer(physicalHost, PhysicalHostFinalizer)
		return ctrl.Result{}, nil
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *PhysicalHostReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1alpha1.PhysicalHost{}).
		// TODO: Add Watches for Secrets or Beskar7Machines if needed
		Complete(r)
}
