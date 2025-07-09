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

package statemachine

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrastructurev1beta1 "github.com/wrkode/beskar7/api/v1beta1"
)

// StateTransition represents a state transition with validation
type StateTransition struct {
	From        string
	To          string
	Condition   StateTransitionCondition
	Description string
}

// StateTransitionCondition is a function that validates if a transition is allowed
type StateTransitionCondition func(host *infrastructurev1beta1.PhysicalHost) error

// PhysicalHostStateMachine manages state transitions for PhysicalHost resources
type PhysicalHostStateMachine struct {
	allowedTransitions map[string][]StateTransition
	logger             logr.Logger
}

// NewPhysicalHostStateMachine creates a new state machine with hardened transitions
func NewPhysicalHostStateMachine(logger logr.Logger) *PhysicalHostStateMachine {
	sm := &PhysicalHostStateMachine{
		allowedTransitions: make(map[string][]StateTransition),
		logger:             logger,
	}
	sm.initializeTransitions()
	return sm
}

// initializeTransitions defines all valid state transitions and their conditions
func (sm *PhysicalHostStateMachine) initializeTransitions() {
	// Define all valid state transitions with validation conditions
	transitions := []StateTransition{
		// Initial enrollment
		{
			From:        infrastructurev1beta1.StateNone,
			To:          infrastructurev1beta1.StateEnrolling,
			Condition:   sm.validateEnrollmentStart,
			Description: "Start enrollment process",
		},
		{
			From:        infrastructurev1beta1.StateEnrolling,
			To:          infrastructurev1beta1.StateAvailable,
			Condition:   sm.validateEnrollmentSuccess,
			Description: "Complete enrollment successfully",
		},
		{
			From:        infrastructurev1beta1.StateEnrolling,
			To:          infrastructurev1beta1.StateError,
			Condition:   sm.validateErrorTransition,
			Description: "Enrollment failed",
		},

		// Claiming and provisioning
		{
			From:        infrastructurev1beta1.StateAvailable,
			To:          infrastructurev1beta1.StateClaimed,
			Condition:   sm.validateClaiming,
			Description: "Host claimed by consumer",
		},
		{
			From:        infrastructurev1beta1.StateClaimed,
			To:          infrastructurev1beta1.StateProvisioning,
			Condition:   sm.validateProvisioningStart,
			Description: "Start provisioning process",
		},
		{
			From:        infrastructurev1beta1.StateProvisioning,
			To:          infrastructurev1beta1.StateProvisioned,
			Condition:   sm.validateProvisioningSuccess,
			Description: "Provisioning completed successfully",
		},

		// Error transitions from any active state
		{
			From:        infrastructurev1beta1.StateAvailable,
			To:          infrastructurev1beta1.StateError,
			Condition:   sm.validateErrorTransition,
			Description: "Available host encountered error",
		},
		{
			From:        infrastructurev1beta1.StateClaimed,
			To:          infrastructurev1beta1.StateError,
			Condition:   sm.validateErrorTransition,
			Description: "Claimed host encountered error",
		},
		{
			From:        infrastructurev1beta1.StateProvisioning,
			To:          infrastructurev1beta1.StateError,
			Condition:   sm.validateErrorTransition,
			Description: "Provisioning failed",
		},

		// Recovery from error
		{
			From:        infrastructurev1beta1.StateError,
			To:          infrastructurev1beta1.StateEnrolling,
			Condition:   sm.validateErrorRecovery,
			Description: "Retry from error state",
		},
		{
			From:        infrastructurev1beta1.StateError,
			To:          infrastructurev1beta1.StateAvailable,
			Condition:   sm.validateDirectErrorRecovery,
			Description: "Direct recovery to available state",
		},

		// Release and cleanup transitions
		{
			From:        infrastructurev1beta1.StateClaimed,
			To:          infrastructurev1beta1.StateAvailable,
			Condition:   sm.validateRelease,
			Description: "Release claimed host",
		},
		{
			From:        infrastructurev1beta1.StateProvisioning,
			To:          infrastructurev1beta1.StateAvailable,
			Condition:   sm.validateRelease,
			Description: "Release provisioning host",
		},
		{
			From:        infrastructurev1beta1.StateProvisioned,
			To:          infrastructurev1beta1.StateAvailable,
			Condition:   sm.validateRelease,
			Description: "Release provisioned host",
		},

		// Deprovisioning (deletion)
		{
			From:        infrastructurev1beta1.StateAvailable,
			To:          infrastructurev1beta1.StateDeprovisioning,
			Condition:   sm.validateDeprovisioning,
			Description: "Start deprovisioning available host",
		},
		{
			From:        infrastructurev1beta1.StateError,
			To:          infrastructurev1beta1.StateDeprovisioning,
			Condition:   sm.validateDeprovisioning,
			Description: "Start deprovisioning error host",
		},
		{
			From:        infrastructurev1beta1.StateProvisioned,
			To:          infrastructurev1beta1.StateDeprovisioning,
			Condition:   sm.validateDeprovisioningWithConsumerCheck,
			Description: "Start deprovisioning provisioned host",
		},

		// Special recovery transitions
		{
			From:        infrastructurev1beta1.StateUnknown,
			To:          infrastructurev1beta1.StateEnrolling,
			Condition:   sm.validateUnknownRecovery,
			Description: "Recover from unknown state",
		},
	}

	// Build the transition map
	for _, transition := range transitions {
		if sm.allowedTransitions[transition.From] == nil {
			sm.allowedTransitions[transition.From] = make([]StateTransition, 0)
		}
		sm.allowedTransitions[transition.From] = append(sm.allowedTransitions[transition.From], transition)
	}
}

// ValidateTransition validates if a state transition is allowed
func (sm *PhysicalHostStateMachine) ValidateTransition(host *infrastructurev1beta1.PhysicalHost, newState string) error {
	currentState := host.Status.State

	// Handle empty current state as StateNone
	if currentState == "" {
		currentState = infrastructurev1beta1.StateNone
	}

	// Check if this is actually a transition
	if currentState == newState {
		// Same state is always valid (idempotent)
		return nil
	}

	// Find allowed transitions from current state
	allowedTransitions, exists := sm.allowedTransitions[currentState]
	if !exists {
		return fmt.Errorf("no transitions defined from state %s", currentState)
	}

	// Find the specific transition
	for _, transition := range allowedTransitions {
		if transition.To == newState {
			// Validate the transition condition
			if err := transition.Condition(host); err != nil {
				return fmt.Errorf("transition from %s to %s not allowed: %w", currentState, newState, err)
			}
			sm.logger.V(1).Info("State transition validated",
				"from", currentState,
				"to", newState,
				"description", transition.Description,
				"host", host.Name)
			return nil
		}
	}

	return fmt.Errorf("transition from %s to %s is not allowed", currentState, newState)
}

// TransitionTo attempts to transition the host to a new state with validation
func (sm *PhysicalHostStateMachine) TransitionTo(ctx context.Context, host *infrastructurev1beta1.PhysicalHost, newState string, reason string) error {
	// Validate the transition first
	if err := sm.ValidateTransition(host, newState); err != nil {
		return fmt.Errorf("state transition validation failed: %w", err)
	}

	previousState := host.Status.State
	if previousState == "" {
		previousState = infrastructurev1beta1.StateNone
	}

	// Perform the transition
	host.Status.State = newState

	// Log the transition
	sm.logger.Info("State transition executed",
		"host", host.Name,
		"from", previousState,
		"to", newState,
		"reason", reason)

	return nil
}

// GetValidTransitions returns all valid transitions from the current state
func (sm *PhysicalHostStateMachine) GetValidTransitions(currentState string) []string {
	if currentState == "" {
		currentState = infrastructurev1beta1.StateNone
	}

	transitions, exists := sm.allowedTransitions[currentState]
	if !exists {
		return []string{}
	}

	states := make([]string, len(transitions))
	for i, transition := range transitions {
		states[i] = transition.To
	}
	return states
}

// IsStateValid checks if a state is a valid PhysicalHost state
func (sm *PhysicalHostStateMachine) IsStateValid(state string) bool {
	validStates := []string{
		infrastructurev1beta1.StateNone,
		infrastructurev1beta1.StateEnrolling,
		infrastructurev1beta1.StateAvailable,
		infrastructurev1beta1.StateClaimed,
		infrastructurev1beta1.StateProvisioning,
		infrastructurev1beta1.StateProvisioned,
		infrastructurev1beta1.StateDeprovisioning,
		infrastructurev1beta1.StateError,
		infrastructurev1beta1.StateUnknown,
	}

	for _, validState := range validStates {
		if state == validState {
			return true
		}
	}
	return false
}

// ValidateStateConsistency validates the overall consistency of the host state
func (sm *PhysicalHostStateMachine) ValidateStateConsistency(host *infrastructurev1beta1.PhysicalHost) error {
	state := host.Status.State
	if state == "" {
		state = infrastructurev1beta1.StateNone
	}

	// Validate state against spec fields
	switch state {
	case infrastructurev1beta1.StateAvailable:
		if host.Spec.ConsumerRef != nil {
			return fmt.Errorf("host in Available state cannot have ConsumerRef set")
		}
		if host.Spec.BootISOSource != nil && *host.Spec.BootISOSource != "" {
			return fmt.Errorf("host in Available state cannot have BootISOSource set")
		}

	case infrastructurev1beta1.StateClaimed:
		if host.Spec.ConsumerRef == nil {
			return fmt.Errorf("host in Claimed state must have ConsumerRef set")
		}
		// BootISOSource is optional in Claimed state

	case infrastructurev1beta1.StateProvisioning, infrastructurev1beta1.StateProvisioned:
		if host.Spec.ConsumerRef == nil {
			return fmt.Errorf("host in %s state must have ConsumerRef set", state)
		}
		if host.Spec.BootISOSource == nil || *host.Spec.BootISOSource == "" {
			return fmt.Errorf("host in %s state must have BootISOSource set", state)
		}

	case infrastructurev1beta1.StateDeprovisioning:
		// Deprovisioning hosts should not have ConsumerRef
		if host.Spec.ConsumerRef != nil {
			return fmt.Errorf("host in Deprovisioning state should not have ConsumerRef set")
		}
	}

	return nil
}

// State transition condition validators

func (sm *PhysicalHostStateMachine) validateEnrollmentStart(host *infrastructurev1beta1.PhysicalHost) error {
	// Enrollment can start if we have required Redfish connection info
	if host.Spec.RedfishConnection.Address == "" {
		return fmt.Errorf("cannot start enrollment: missing Redfish address")
	}
	if host.Spec.RedfishConnection.CredentialsSecretRef == "" {
		return fmt.Errorf("cannot start enrollment: missing credentials secret reference")
	}
	return nil
}

func (sm *PhysicalHostStateMachine) validateEnrollmentSuccess(host *infrastructurev1beta1.PhysicalHost) error {
	// Enrollment succeeds when we have hardware details and no consumer
	if host.Spec.ConsumerRef != nil {
		return fmt.Errorf("cannot complete enrollment: host is already claimed")
	}
	// Hardware details are populated during enrollment - we don't enforce them here
	// as they might be updated after the state transition
	return nil
}

func (sm *PhysicalHostStateMachine) validateClaiming(host *infrastructurev1beta1.PhysicalHost) error {
	if host.Spec.ConsumerRef == nil {
		return fmt.Errorf("cannot claim host: no ConsumerRef set")
	}
	return nil
}

func (sm *PhysicalHostStateMachine) validateProvisioningStart(host *infrastructurev1beta1.PhysicalHost) error {
	if host.Spec.ConsumerRef == nil {
		return fmt.Errorf("cannot start provisioning: no ConsumerRef set")
	}
	if host.Spec.BootISOSource == nil || *host.Spec.BootISOSource == "" {
		return fmt.Errorf("cannot start provisioning: no BootISOSource set")
	}
	return nil
}

func (sm *PhysicalHostStateMachine) validateProvisioningSuccess(host *infrastructurev1beta1.PhysicalHost) error {
	if host.Spec.ConsumerRef == nil {
		return fmt.Errorf("cannot complete provisioning: no ConsumerRef set")
	}
	if host.Spec.BootISOSource == nil || *host.Spec.BootISOSource == "" {
		return fmt.Errorf("cannot complete provisioning: no BootISOSource set")
	}
	return nil
}

func (sm *PhysicalHostStateMachine) validateRelease(host *infrastructurev1beta1.PhysicalHost) error {
	// Release is valid when ConsumerRef is cleared
	if host.Spec.ConsumerRef != nil {
		return fmt.Errorf("cannot release host: ConsumerRef still set")
	}
	return nil
}

func (sm *PhysicalHostStateMachine) validateDeprovisioning(host *infrastructurev1beta1.PhysicalHost) error {
	// Deprovisioning requires deletion timestamp
	if host.DeletionTimestamp == nil {
		return fmt.Errorf("cannot start deprovisioning: host not marked for deletion")
	}
	// Must not have consumer
	if host.Spec.ConsumerRef != nil {
		return fmt.Errorf("cannot start deprovisioning: host still has ConsumerRef")
	}
	return nil
}

func (sm *PhysicalHostStateMachine) validateDeprovisioningWithConsumerCheck(host *infrastructurev1beta1.PhysicalHost) error {
	// Deprovisioning requires deletion timestamp
	if host.DeletionTimestamp == nil {
		return fmt.Errorf("cannot start deprovisioning: host not marked for deletion")
	}
	// For provisioned hosts, we'll wait for consumer to be cleared first
	return nil
}

func (sm *PhysicalHostStateMachine) validateErrorTransition(host *infrastructurev1beta1.PhysicalHost) error {
	// Error transitions are always allowed - any state can enter error
	return nil
}

func (sm *PhysicalHostStateMachine) validateErrorRecovery(host *infrastructurev1beta1.PhysicalHost) error {
	// Recovery to enrollment is allowed if we have basic connection info
	if host.Spec.RedfishConnection.Address == "" {
		return fmt.Errorf("cannot recover: missing Redfish address")
	}
	if host.Spec.RedfishConnection.CredentialsSecretRef == "" {
		return fmt.Errorf("cannot recover: missing credentials secret reference")
	}
	return nil
}

func (sm *PhysicalHostStateMachine) validateDirectErrorRecovery(host *infrastructurev1beta1.PhysicalHost) error {
	// Direct recovery to Available is allowed if host is not claimed
	if host.Spec.ConsumerRef != nil {
		return fmt.Errorf("cannot recover directly to Available: host is claimed")
	}
	return nil
}

func (sm *PhysicalHostStateMachine) validateUnknownRecovery(host *infrastructurev1beta1.PhysicalHost) error {
	// Recovery from Unknown always goes through enrollment
	return sm.validateEnrollmentStart(host)
}

// StateTransitionGuard provides additional safety checks for concurrent operations
type StateTransitionGuard struct {
	client client.Client
	logger logr.Logger
}

// NewStateTransitionGuard creates a new transition guard
func NewStateTransitionGuard(client client.Client, logger logr.Logger) *StateTransitionGuard {
	return &StateTransitionGuard{
		client: client,
		logger: logger,
	}
}

// SafeStateTransition performs a state transition with optimistic locking protection
func (g *StateTransitionGuard) SafeStateTransition(
	ctx context.Context,
	host *infrastructurev1beta1.PhysicalHost,
	stateMachine *PhysicalHostStateMachine,
	newState string,
	reason string,
	maxRetries int,
) error {
	for retry := 0; retry < maxRetries; retry++ {
		// Get the latest version of the host
		latest := &infrastructurev1beta1.PhysicalHost{}
		key := client.ObjectKeyFromObject(host)
		if err := g.client.Get(ctx, key, latest); err != nil {
			return fmt.Errorf("failed to get latest host state: %w", err)
		}

		// Check if someone else already transitioned to our target state
		if latest.Status.State == newState {
			g.logger.V(1).Info("State already transitioned by another controller",
				"host", host.Name,
				"targetState", newState,
				"currentState", latest.Status.State)
			// Update our local copy and return success
			host.Status = latest.Status
			return nil
		}

		// Validate the transition on the latest version
		if err := stateMachine.ValidateTransition(latest, newState); err != nil {
			return fmt.Errorf("state transition validation failed on latest version: %w", err)
		}

		// Attempt the transition
		previousState := latest.Status.State
		latest.Status.State = newState
		latest.Status.ErrorMessage = "" // Clear error message on successful transition

		// Try to update
		if err := g.client.Status().Update(ctx, latest); err != nil {
			if errors.IsConflict(err) && retry < maxRetries-1 {
				g.logger.V(1).Info("Optimistic lock conflict, retrying state transition",
					"host", host.Name,
					"retry", retry+1,
					"maxRetries", maxRetries)
				time.Sleep(time.Duration(retry+1) * 100 * time.Millisecond) // Exponential backoff
				continue
			}
			return fmt.Errorf("failed to update host state after %d retries: %w", retry+1, err)
		}

		// Success
		g.logger.Info("Safe state transition completed",
			"host", host.Name,
			"from", previousState,
			"to", newState,
			"reason", reason,
			"retries", retry)

		// Update our local copy
		host.Status = latest.Status
		return nil
	}

	return fmt.Errorf("failed to complete state transition after %d retries", maxRetries)
}

// StateRecoveryManager handles stuck states and provides recovery mechanisms
type StateRecoveryManager struct {
	client client.Client
	logger logr.Logger
}

// NewStateRecoveryManager creates a new recovery manager
func NewStateRecoveryManager(client client.Client, logger logr.Logger) *StateRecoveryManager {
	return &StateRecoveryManager{
		client: client,
		logger: logger,
	}
}

// DetectStuckState detects if a host is stuck in a state for too long
func (rm *StateRecoveryManager) DetectStuckState(host *infrastructurev1beta1.PhysicalHost, timeout time.Duration) bool {
	// Check if the host has been in the current state for too long
	now := time.Now()
	var stateTransitionTime *metav1.Time

	// Try to find the most recent condition transition time as a proxy for state transition
	var mostRecentTime *metav1.Time
	for _, condition := range host.Status.Conditions {
		if mostRecentTime == nil || condition.LastTransitionTime.After(mostRecentTime.Time) {
			mostRecentTime = &condition.LastTransitionTime
		}
	}

	if mostRecentTime != nil {
		stateTransitionTime = mostRecentTime
	} else {
		// If we don't have any condition transition time, use creation time
		stateTransitionTime = &host.CreationTimestamp
	}

	timeInState := now.Sub(stateTransitionTime.Time)
	isStuck := timeInState > timeout

	if isStuck {
		rm.logger.Info("Detected stuck state",
			"host", host.Name,
			"state", host.Status.State,
			"timeInState", timeInState,
			"timeout", timeout)
	}

	return isStuck
}

// RecoverStuckState attempts to recover a host from a stuck state
func (rm *StateRecoveryManager) RecoverStuckState(
	ctx context.Context,
	host *infrastructurev1beta1.PhysicalHost,
	stateMachine *PhysicalHostStateMachine,
) error {
	currentState := host.Status.State
	if currentState == "" {
		currentState = infrastructurev1beta1.StateNone
	}

	rm.logger.Info("Attempting recovery from stuck state",
		"host", host.Name,
		"currentState", currentState)

	// Define recovery strategies for each state
	switch currentState {
	case infrastructurev1beta1.StateEnrolling:
		// Retry enrollment by transitioning back to enrolling
		return rm.retryOperation(ctx, host, stateMachine, infrastructurev1beta1.StateEnrolling, "stuck enrollment recovery")

	case infrastructurev1beta1.StateProvisioning:
		// Check if we should retry or transition to error
		if host.Spec.ConsumerRef == nil {
			// Consumer was removed, transition to available
			return rm.transitionToState(ctx, host, stateMachine, infrastructurev1beta1.StateAvailable, "consumer removed during provisioning")
		}
		// Retry provisioning
		return rm.retryOperation(ctx, host, stateMachine, infrastructurev1beta1.StateProvisioning, "stuck provisioning recovery")

	case infrastructurev1beta1.StateDeprovisioning:
		// Deprovisioning should always eventually succeed
		return rm.retryOperation(ctx, host, stateMachine, infrastructurev1beta1.StateDeprovisioning, "stuck deprovisioning recovery")

	default:
		// For other states, transition to error state for investigation
		return rm.transitionToState(ctx, host, stateMachine, infrastructurev1beta1.StateError, fmt.Sprintf("stuck in state %s", currentState))
	}
}

func (rm *StateRecoveryManager) retryOperation(
	ctx context.Context,
	host *infrastructurev1beta1.PhysicalHost,
	stateMachine *PhysicalHostStateMachine,
	retryState string,
	reason string,
) error {
	guard := NewStateTransitionGuard(rm.client, rm.logger)
	return guard.SafeStateTransition(ctx, host, stateMachine, retryState, reason, 3)
}

func (rm *StateRecoveryManager) transitionToState(
	ctx context.Context,
	host *infrastructurev1beta1.PhysicalHost,
	stateMachine *PhysicalHostStateMachine,
	newState string,
	reason string,
) error {
	guard := NewStateTransitionGuard(rm.client, rm.logger)
	return guard.SafeStateTransition(ctx, host, stateMachine, newState, reason, 3)
}
