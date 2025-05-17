package statemachine

// PhysicalHostState represents the possible states of a PhysicalHost
type PhysicalHostState State

const (
	// Initial state when the PhysicalHost is first created
	PhysicalHostStateInitial PhysicalHostState = "Initial"

	// State when the PhysicalHost is being discovered
	PhysicalHostStateDiscovering PhysicalHostState = "Discovering"

	// State when the PhysicalHost is available for use
	PhysicalHostStateAvailable PhysicalHostState = "Available"

	// State when the PhysicalHost has been claimed by a Beskar7Machine
	PhysicalHostStateClaimed PhysicalHostState = "Claimed"

	// State when the PhysicalHost is being provisioned
	PhysicalHostStateProvisioning PhysicalHostState = "Provisioning"

	// State when the PhysicalHost has been provisioned
	PhysicalHostStateProvisioned PhysicalHostState = "Provisioned"

	// State when the PhysicalHost is in an error state
	PhysicalHostStateError PhysicalHostState = "Error"

	// State when the PhysicalHost is being deprovisioned
	PhysicalHostStateDeprovisioning PhysicalHostState = "Deprovisioning"
)

// PhysicalHostEvent represents the possible events that can trigger state transitions
type PhysicalHostEvent Event

const (
	// Event when discovery starts
	PhysicalHostEventStartDiscovery PhysicalHostEvent = "StartDiscovery"

	// Event when discovery succeeds
	PhysicalHostEventDiscoverySucceeded PhysicalHostEvent = "DiscoverySucceeded"

	// Event when discovery fails
	PhysicalHostEventDiscoveryFailed PhysicalHostEvent = "DiscoveryFailed"

	// Event when the host is claimed
	PhysicalHostEventClaim PhysicalHostEvent = "Claim"

	// Event when provisioning starts
	PhysicalHostEventStartProvisioning PhysicalHostEvent = "StartProvisioning"

	// Event when provisioning succeeds
	PhysicalHostEventProvisioningSucceeded PhysicalHostEvent = "ProvisioningSucceeded"

	// Event when provisioning fails
	PhysicalHostEventProvisioningFailed PhysicalHostEvent = "ProvisioningFailed"

	// Event when an error occurs
	PhysicalHostEventError PhysicalHostEvent = "Error"

	// Event when deprovisioning starts
	PhysicalHostEventStartDeprovisioning PhysicalHostEvent = "StartDeprovisioning"

	// Event when deprovisioning completes
	PhysicalHostEventDeprovisioningCompleted PhysicalHostEvent = "DeprovisioningCompleted"

	// Event when the host is released
	PhysicalHostEventRelease PhysicalHostEvent = "Release"
)

// NewPhysicalHostStateMachine creates a new state machine for PhysicalHost
func NewPhysicalHostStateMachine() StateMachine {
	machine := NewMachine(ConvertState(PhysicalHostStateInitial))

	// Define transitions
	transitions := map[PhysicalHostState]map[PhysicalHostEvent]Transition{
		PhysicalHostStateInitial: {
			PhysicalHostEventStartDiscovery: {
				TargetState: ConvertState(PhysicalHostStateDiscovering),
			},
			PhysicalHostEventStartDeprovisioning: {
				TargetState: ConvertState(PhysicalHostStateDeprovisioning),
			},
		},
		PhysicalHostStateDiscovering: {
			PhysicalHostEventDiscoverySucceeded: {
				TargetState: ConvertState(PhysicalHostStateAvailable),
			},
			PhysicalHostEventDiscoveryFailed: {
				TargetState: ConvertState(PhysicalHostStateError),
			},
		},
		PhysicalHostStateAvailable: {
			PhysicalHostEventClaim: {
				TargetState: ConvertState(PhysicalHostStateClaimed),
			},
			PhysicalHostEventError: {
				TargetState: ConvertState(PhysicalHostStateError),
			},
		},
		PhysicalHostStateClaimed: {
			PhysicalHostEventStartProvisioning: {
				TargetState: ConvertState(PhysicalHostStateProvisioning),
			},
			PhysicalHostEventError: {
				TargetState: ConvertState(PhysicalHostStateError),
			},
		},
		PhysicalHostStateProvisioning: {
			PhysicalHostEventProvisioningSucceeded: {
				TargetState: ConvertState(PhysicalHostStateProvisioned),
			},
			PhysicalHostEventProvisioningFailed: {
				TargetState: ConvertState(PhysicalHostStateError),
			},
		},
		PhysicalHostStateProvisioned: {
			PhysicalHostEventStartDeprovisioning: {
				TargetState: ConvertState(PhysicalHostStateDeprovisioning),
			},
			PhysicalHostEventError: {
				TargetState: ConvertState(PhysicalHostStateError),
			},
		},
		PhysicalHostStateError: {
			PhysicalHostEventStartDiscovery: {
				TargetState: ConvertState(PhysicalHostStateDiscovering),
			},
		},
		PhysicalHostStateDeprovisioning: {
			PhysicalHostEventDeprovisioningCompleted: {
				TargetState: ConvertState(PhysicalHostStateAvailable),
			},
			PhysicalHostEventError: {
				TargetState: ConvertState(PhysicalHostStateError),
			},
		},
	}

	// Add all transitions to the state machine
	for fromState, eventTransitions := range transitions {
		for event, transition := range eventTransitions {
			machine.AddTransition(ConvertState(fromState), ConvertEvent(event), transition)
		}
	}

	return machine
}
