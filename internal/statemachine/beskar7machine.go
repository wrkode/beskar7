package statemachine

// Beskar7MachineState represents the possible states of a Beskar7Machine
type Beskar7MachineState State

const (
	// Initial state when the Beskar7Machine is first created
	Beskar7MachineStateInitial Beskar7MachineState = "Initial"

	// State when the Beskar7Machine is waiting for a PhysicalHost
	Beskar7MachineStateWaitingForHost Beskar7MachineState = "WaitingForHost"

	// State when the Beskar7Machine has found a PhysicalHost
	Beskar7MachineStateHostFound Beskar7MachineState = "HostFound"

	// State when the Beskar7Machine is configuring the PhysicalHost
	Beskar7MachineStateConfiguringHost Beskar7MachineState = "ConfiguringHost"

	// State when the Beskar7Machine is waiting for the PhysicalHost to be provisioned
	Beskar7MachineStateWaitingForProvisioning Beskar7MachineState = "WaitingForProvisioning"

	// State when the Beskar7Machine is provisioned and ready
	Beskar7MachineStateProvisioned Beskar7MachineState = "Provisioned"

	// State when the Beskar7Machine is in an error state
	Beskar7MachineStateError Beskar7MachineState = "Error"

	// State when the Beskar7Machine is being deleted
	Beskar7MachineStateDeleting Beskar7MachineState = "Deleting"
)

// Beskar7MachineEvent represents the possible events that can trigger state transitions
type Beskar7MachineEvent Event

const (
	// Event when the machine is created
	Beskar7MachineEventCreate Beskar7MachineEvent = "Create"

	// Event when a host is found
	Beskar7MachineEventHostFound Beskar7MachineEvent = "HostFound"

	// Event when host configuration starts
	Beskar7MachineEventStartHostConfig Beskar7MachineEvent = "StartHostConfig"

	// Event when host configuration completes
	Beskar7MachineEventHostConfigCompleted Beskar7MachineEvent = "HostConfigCompleted"

	// Event when provisioning starts
	Beskar7MachineEventStartProvisioning Beskar7MachineEvent = "StartProvisioning"

	// Event when provisioning completes
	Beskar7MachineEventProvisioningCompleted Beskar7MachineEvent = "ProvisioningCompleted"

	// Event when an error occurs
	Beskar7MachineEventError Beskar7MachineEvent = "Error"

	// Event when deletion starts
	Beskar7MachineEventStartDeletion Beskar7MachineEvent = "StartDeletion"

	// Event when deletion completes
	Beskar7MachineEventDeletionCompleted Beskar7MachineEvent = "DeletionCompleted"
)

// NewBeskar7MachineStateMachine creates a new state machine for Beskar7Machine
func NewBeskar7MachineStateMachine() StateMachine {
	machine := NewMachine(State(Beskar7MachineStateInitial))

	// Define transitions
	transitions := map[Beskar7MachineState]map[Beskar7MachineEvent]Transition{
		Beskar7MachineStateInitial: {
			Beskar7MachineEventCreate: {
				TargetState: State(Beskar7MachineStateWaitingForHost),
			},
		},
		Beskar7MachineStateWaitingForHost: {
			Beskar7MachineEventHostFound: {
				TargetState: State(Beskar7MachineStateHostFound),
			},
			Beskar7MachineEventError: {
				TargetState: State(Beskar7MachineStateError),
			},
		},
		Beskar7MachineStateHostFound: {
			Beskar7MachineEventStartHostConfig: {
				TargetState: State(Beskar7MachineStateConfiguringHost),
			},
			Beskar7MachineEventError: {
				TargetState: State(Beskar7MachineStateError),
			},
		},
		Beskar7MachineStateConfiguringHost: {
			Beskar7MachineEventHostConfigCompleted: {
				TargetState: State(Beskar7MachineStateWaitingForProvisioning),
			},
			Beskar7MachineEventError: {
				TargetState: State(Beskar7MachineStateError),
			},
		},
		Beskar7MachineStateWaitingForProvisioning: {
			Beskar7MachineEventStartProvisioning: {
				TargetState: State(Beskar7MachineStateWaitingForProvisioning),
			},
			Beskar7MachineEventProvisioningCompleted: {
				TargetState: State(Beskar7MachineStateProvisioned),
			},
			Beskar7MachineEventError: {
				TargetState: State(Beskar7MachineStateError),
			},
		},
		Beskar7MachineStateProvisioned: {
			Beskar7MachineEventStartDeletion: {
				TargetState: State(Beskar7MachineStateDeleting),
			},
			Beskar7MachineEventError: {
				TargetState: State(Beskar7MachineStateError),
			},
		},
		Beskar7MachineStateError: {
			Beskar7MachineEventCreate: {
				TargetState: State(Beskar7MachineStateWaitingForHost),
			},
		},
		Beskar7MachineStateDeleting: {
			Beskar7MachineEventDeletionCompleted: {
				TargetState: State(Beskar7MachineStateInitial),
			},
			Beskar7MachineEventError: {
				TargetState: State(Beskar7MachineStateError),
			},
		},
	}

	// Add all transitions to the state machine
	for fromState, eventTransitions := range transitions {
		for event, transition := range eventTransitions {
			machine.AddTransition(State(fromState), Event(event), transition)
		}
	}

	return machine
}
