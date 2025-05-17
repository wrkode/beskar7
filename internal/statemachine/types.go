package statemachine

import (
	"context"
	"fmt"
)

// State represents a state in the state machine
type State string

// Event represents an event that can trigger a state transition
type Event string

// Transition represents a state transition with an optional action
type Transition struct {
	TargetState State
	Action      func(context.Context) error
}

// StateMachine defines the interface for a state machine
type StateMachine interface {
	// CurrentState returns the current state of the machine
	CurrentState() State

	// CanTransition checks if a transition is allowed from the current state
	CanTransition(ctx context.Context, event Event) bool

	// Transition performs a state transition if allowed
	Transition(ctx context.Context, event Event) error

	// AddTransition adds a new transition rule to the state machine
	AddTransition(fromState State, event Event, transition Transition)

	// GetTransitions returns all possible transitions from the current state
	GetTransitions() map[Event]Transition
}

// StateMachineError represents an error that occurred during a state transition
type StateMachineError struct {
	CurrentState State
	Event        Event
	Message      string
}

func (e *StateMachineError) Error() string {
	return fmt.Sprintf("state machine error: cannot transition from state %q with event %q: %s", e.CurrentState, e.Event, e.Message)
}

// NewStateMachineError creates a new StateMachineError
func NewStateMachineError(currentState State, event Event, message string) *StateMachineError {
	return &StateMachineError{
		CurrentState: currentState,
		Event:        event,
		Message:      message,
	}
}

// ConvertState converts a state to the base State type
func ConvertState[T ~string](state T) State {
	return State(state)
}

// ConvertEvent converts an event to the base Event type
func ConvertEvent[T ~string](event T) Event {
	return Event(event)
}
