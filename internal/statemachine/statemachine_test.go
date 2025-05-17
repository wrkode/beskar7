package statemachine

import (
	"context"
	"errors"
	"testing"

	. "github.com/onsi/gomega"
)

func TestPhysicalHostStateMachine(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create a new PhysicalHost state machine
	machine := NewPhysicalHostStateMachine()

	// Test initial state
	g.Expect(machine.CurrentState()).To(Equal(State(PhysicalHostStateInitial)))

	// Test valid transitions
	AssertStateTransition(t, machine, ctx, Event(PhysicalHostEventStartDiscovery), State(PhysicalHostStateDiscovering))
	AssertStateTransition(t, machine, ctx, Event(PhysicalHostEventDiscoverySucceeded), State(PhysicalHostStateAvailable))
	AssertStateTransition(t, machine, ctx, Event(PhysicalHostEventClaim), State(PhysicalHostStateClaimed))
	AssertStateTransition(t, machine, ctx, Event(PhysicalHostEventStartProvisioning), State(PhysicalHostStateProvisioning))
	AssertStateTransition(t, machine, ctx, Event(PhysicalHostEventProvisioningSucceeded), State(PhysicalHostStateProvisioned))
	AssertStateTransition(t, machine, ctx, Event(PhysicalHostEventStartDeprovisioning), State(PhysicalHostStateDeprovisioning))
	AssertStateTransition(t, machine, ctx, Event(PhysicalHostEventDeprovisioningCompleted), State(PhysicalHostStateAvailable))

	// Test error state transition
	AssertStateTransition(t, machine, ctx, Event(PhysicalHostEventError), State(PhysicalHostStateError))

	// Test recovery from error state
	AssertStateTransition(t, machine, ctx, Event(PhysicalHostEventStartDiscovery), State(PhysicalHostStateDiscovering))

	// Test invalid transition
	AssertInvalidTransition(t, machine, ctx, Event(PhysicalHostEventClaim))
}

func TestBeskar7MachineStateMachine(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create a new Beskar7Machine state machine
	machine := NewBeskar7MachineStateMachine()

	// Test initial state
	g.Expect(machine.CurrentState()).To(Equal(State(Beskar7MachineStateInitial)))

	// Test valid transitions
	AssertStateTransition(t, machine, ctx, Event(Beskar7MachineEventCreate), State(Beskar7MachineStateWaitingForHost))
	AssertStateTransition(t, machine, ctx, Event(Beskar7MachineEventHostFound), State(Beskar7MachineStateHostFound))
	AssertStateTransition(t, machine, ctx, Event(Beskar7MachineEventStartHostConfig), State(Beskar7MachineStateConfiguringHost))
	AssertStateTransition(t, machine, ctx, Event(Beskar7MachineEventHostConfigCompleted), State(Beskar7MachineStateWaitingForProvisioning))
	AssertStateTransition(t, machine, ctx, Event(Beskar7MachineEventStartProvisioning), State(Beskar7MachineStateWaitingForProvisioning))
	AssertStateTransition(t, machine, ctx, Event(Beskar7MachineEventProvisioningCompleted), State(Beskar7MachineStateProvisioned))
	AssertStateTransition(t, machine, ctx, Event(Beskar7MachineEventStartDeletion), State(Beskar7MachineStateDeleting))
	AssertStateTransition(t, machine, ctx, Event(Beskar7MachineEventDeletionCompleted), State(Beskar7MachineStateInitial))

	// Test error state transition
	AssertStateTransition(t, machine, ctx, Event(Beskar7MachineEventError), State(Beskar7MachineStateError))

	// Test recovery from error state
	AssertStateTransition(t, machine, ctx, Event(Beskar7MachineEventCreate), State(Beskar7MachineStateWaitingForHost))

	// Test invalid transition
	AssertInvalidTransition(t, machine, ctx, Event(Beskar7MachineEventHostFound))
}

func TestStateMachineWithActions(t *testing.T) {
	ctx := context.Background()

	// Create a new state machine
	machine := NewMachine(State("Initial"))

	// Create test actions
	action1 := NewTestAction()
	action2 := NewTestAction()

	// Add transitions with actions
	machine.AddTransition(State("Initial"), Event("Event1"), Transition{
		TargetState: State("State1"),
		Action:      action1.Execute,
	})

	machine.AddTransition(State("State1"), Event("Event2"), Transition{
		TargetState: State("State2"),
		Action:      action2.Execute,
	})

	// Test successful transitions with actions
	AssertStateTransition(t, machine, ctx, Event("Event1"), State("State1"))
	AssertActionExecuted(t, action1, 1)
	AssertActionNotExecuted(t, action2)

	AssertStateTransition(t, machine, ctx, Event("Event2"), State("State2"))
	AssertActionExecuted(t, action1, 1)
	AssertActionExecuted(t, action2, 1)

	// Test transition with failing action
	action1.SetError(errors.New("action failed"))
	AssertInvalidTransition(t, machine, ctx, Event("Event1"))
	AssertActionExecuted(t, action1, 2)
	AssertActionExecuted(t, action2, 1)
}
