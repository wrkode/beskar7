# State Machine Package

This package provides a generic state machine implementation that can be used to manage state transitions in the Beskar7 project. It includes specific implementations for `PhysicalHost` and `Beskar7Machine` state machines.

## Overview

The state machine package provides:

1. A generic state machine implementation
2. Type-safe state and event definitions
3. Support for transition actions
4. Thread-safe operations
5. Comprehensive test coverage
6. Test helpers for easy testing

## Usage

### Basic Usage

```go
// Create a new state machine with an initial state
machine := NewMachine(State("Initial"))

// Add a transition
machine.AddTransition(State("Initial"), Event("Event1"), Transition{
    TargetState: State("State1"),
    Action: func(ctx context.Context) error {
        // Perform some action
        return nil
    },
})

// Perform a transition
err := machine.Transition(ctx, Event("Event1"))
```

### Using the PhysicalHost State Machine

```go
// Create a new PhysicalHost state machine
machine := NewPhysicalHostStateMachine()

// Start discovery
err := machine.Transition(ctx, Event(PhysicalHostEventStartDiscovery))

// Handle discovery success
err = machine.Transition(ctx, Event(PhysicalHostEventDiscoverySucceeded))
```

### Using the Beskar7Machine State Machine

```go
// Create a new Beskar7Machine state machine
machine := NewBeskar7MachineStateMachine()

// Create a new machine
err := machine.Transition(ctx, Event(Beskar7MachineEventCreate))

// Handle host found
err = machine.Transition(ctx, Event(Beskar7MachineEventHostFound))
```

## States and Events

### PhysicalHost States

- `Initial`: Initial state when the PhysicalHost is first created
- `Discovering`: State when the PhysicalHost is being discovered
- `Available`: State when the PhysicalHost is available for use
- `Claimed`: State when the PhysicalHost has been claimed by a Beskar7Machine
- `Provisioning`: State when the PhysicalHost is being provisioned
- `Provisioned`: State when the PhysicalHost has been provisioned
- `Error`: State when the PhysicalHost is in an error state
- `Deprovisioning`: State when the PhysicalHost is being deprovisioned

### PhysicalHost Events

- `StartDiscovery`: Event when discovery starts
- `DiscoverySucceeded`: Event when discovery succeeds
- `DiscoveryFailed`: Event when discovery fails
- `Claim`: Event when the host is claimed
- `StartProvisioning`: Event when provisioning starts
- `ProvisioningSucceeded`: Event when provisioning succeeds
- `ProvisioningFailed`: Event when provisioning fails
- `Error`: Event when an error occurs
- `StartDeprovisioning`: Event when deprovisioning starts
- `DeprovisioningCompleted`: Event when deprovisioning completes
- `Release`: Event when the host is released

### Beskar7Machine States

- `Initial`: Initial state when the Beskar7Machine is first created
- `WaitingForHost`: State when the Beskar7Machine is waiting for a PhysicalHost
- `HostFound`: State when the Beskar7Machine has found a PhysicalHost
- `ConfiguringHost`: State when the Beskar7Machine is configuring the PhysicalHost
- `WaitingForProvisioning`: State when the Beskar7Machine is waiting for the PhysicalHost to be provisioned
- `Provisioned`: State when the Beskar7Machine is provisioned and ready
- `Error`: State when the Beskar7Machine is in an error state
- `Deleting`: State when the Beskar7Machine is being deleted

### Beskar7Machine Events

- `Create`: Event when the machine is created
- `HostFound`: Event when a host is found
- `StartHostConfig`: Event when host configuration starts
- `HostConfigCompleted`: Event when host configuration completes
- `StartProvisioning`: Event when provisioning starts
- `ProvisioningCompleted`: Event when provisioning completes
- `Error`: Event when an error occurs
- `StartDeletion`: Event when deletion starts
- `DeletionCompleted`: Event when deletion completes

## Testing

The package includes test helpers to make testing state machines easier:

```go
// Test a state transition
AssertStateTransition(t, machine, ctx, Event("Event1"), State("State1"))

// Test an invalid transition
AssertInvalidTransition(t, machine, ctx, Event("InvalidEvent"))

// Test action execution
action := NewTestAction()
AssertActionExecuted(t, action, 1)
AssertActionNotExecuted(t, action)
```

## Error Handling

The state machine returns errors in the following cases:

1. When a transition is not allowed from the current state
2. When a transition action fails
3. When no transitions are defined for the current state

Errors are wrapped in a `StateMachineError` type that includes:
- The current state
- The event that triggered the error
- A descriptive message 