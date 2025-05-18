package errors

import (
	"fmt"
)

// PhysicalHostError represents a base error type for PhysicalHost operations
type PhysicalHostError struct {
	Operation string
	Reason    string
	Err       error
}

// Error implements the error interface
func (e *PhysicalHostError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s failed: %s: %v", e.Operation, e.Reason, e.Err)
	}
	return fmt.Sprintf("%s failed: %s", e.Operation, e.Reason)
}

// Unwrap returns the wrapped error
func (e *PhysicalHostError) Unwrap() error {
	return e.Err
}

// IsRetryable returns whether this error should be retried
func (e *PhysicalHostError) IsRetryable() bool {
	// Check if the wrapped error is retryable
	if e.Err != nil {
		if retryable, ok := e.Err.(interface{ IsRetryable() bool }); ok {
			return retryable.IsRetryable()
		}
	}
	return false
}

// NewPhysicalHostError creates a new PhysicalHostError
func NewPhysicalHostError(operation, reason string, err error) *PhysicalHostError {
	return &PhysicalHostError{
		Operation: operation,
		Reason:    reason,
		Err:       err,
	}
}

// RedfishConnectionError represents errors related to Redfish connectivity
type RedfishConnectionError struct {
	*PhysicalHostError
	Address string
}

// NewRedfishConnectionError creates a new RedfishConnectionError
func NewRedfishConnectionError(address, reason string, err error) *RedfishConnectionError {
	return &RedfishConnectionError{
		PhysicalHostError: NewPhysicalHostError("RedfishConnection", reason, err),
		Address:           address,
	}
}

// DiscoveryError represents errors that occur during host discovery
type DiscoveryError struct {
	*PhysicalHostError
	HostID string
}

// NewDiscoveryError creates a new DiscoveryError
func NewDiscoveryError(hostID, reason string, err error) *DiscoveryError {
	return &DiscoveryError{
		PhysicalHostError: NewPhysicalHostError("Discovery", reason, err),
		HostID:            hostID,
	}
}

// ProvisioningError represents errors that occur during host provisioning
type ProvisioningError struct {
	*PhysicalHostError
	HostID string
	Step   string
}

// NewProvisioningError creates a new ProvisioningError
func NewProvisioningError(hostID, step, reason string, err error) *ProvisioningError {
	return &ProvisioningError{
		PhysicalHostError: NewPhysicalHostError("Provisioning", reason, err),
		HostID:            hostID,
		Step:              step,
	}
}

// StateTransitionError represents errors that occur during state transitions
type StateTransitionError struct {
	*PhysicalHostError
	FromState string
	ToState   string
}

// NewStateTransitionError creates a new StateTransitionError
func NewStateTransitionError(fromState, toState, reason string, err error) *StateTransitionError {
	return &StateTransitionError{
		PhysicalHostError: NewPhysicalHostError("StateTransition", reason, err),
		FromState:         fromState,
		ToState:           toState,
	}
}

// CredentialsError represents errors related to credentials
type CredentialsError struct {
	*PhysicalHostError
	SecretName string
}

// NewCredentialsError creates a new CredentialsError
func NewCredentialsError(secretName, reason string, err error) *CredentialsError {
	return &CredentialsError{
		PhysicalHostError: NewPhysicalHostError("Credentials", reason, err),
		SecretName:        secretName,
	}
}

// PowerStateError represents errors related to power state operations
type PowerStateError struct {
	*PhysicalHostError
	CurrentState string
	TargetState  string
}

// NewPowerStateError creates a new PowerStateError
func NewPowerStateError(currentState, targetState, reason string, err error) *PowerStateError {
	return &PowerStateError{
		PhysicalHostError: NewPhysicalHostError("PowerState", reason, err),
		CurrentState:      currentState,
		TargetState:       targetState,
	}
}

// BootSourceError represents errors related to boot source operations
type BootSourceError struct {
	*PhysicalHostError
	BootSource string
}

// NewBootSourceError creates a new BootSourceError
func NewBootSourceError(bootSource, reason string, err error) *BootSourceError {
	return &BootSourceError{
		PhysicalHostError: NewPhysicalHostError("BootSource", reason, err),
		BootSource:        bootSource,
	}
}
