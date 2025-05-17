package statemachine

import (
	"context"
	"sync"
)

// machine implements the StateMachine interface
type machine struct {
	currentState State
	transitions  map[State]map[Event]Transition
	mu           sync.RWMutex
}

// NewMachine creates a new state machine with the given initial state
func NewMachine(initialState State) StateMachine {
	return &machine{
		currentState: initialState,
		transitions:  make(map[State]map[Event]Transition),
	}
}

// CurrentState returns the current state of the machine
func (m *machine) CurrentState() State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentState
}

// CanTransition checks if a transition is allowed from the current state
func (m *machine) CanTransition(ctx context.Context, event Event) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stateTransitions, exists := m.transitions[m.currentState]
	if !exists {
		return false
	}

	_, exists = stateTransitions[event]
	return exists
}

// Transition performs a state transition if allowed
func (m *machine) Transition(ctx context.Context, event Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	stateTransitions, exists := m.transitions[m.currentState]
	if !exists {
		return NewStateMachineError(m.currentState, event, "no transitions defined for current state")
	}

	transition, exists := stateTransitions[event]
	if !exists {
		return NewStateMachineError(m.currentState, event, "no transition defined for event")
	}

	// Execute the transition action if defined
	if transition.Action != nil {
		if err := transition.Action(ctx); err != nil {
			return err
		}
	}

	// Update the current state
	m.currentState = transition.TargetState
	return nil
}

// AddTransition adds a new transition rule to the state machine
func (m *machine) AddTransition(fromState State, event Event, transition Transition) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.transitions[fromState]; !exists {
		m.transitions[fromState] = make(map[Event]Transition)
	}
	m.transitions[fromState][event] = transition
}

// GetTransitions returns all possible transitions from the current state
func (m *machine) GetTransitions() map[Event]Transition {
	m.mu.RLock()
	defer m.mu.RUnlock()

	transitions := m.transitions[m.currentState]
	if transitions == nil {
		return make(map[Event]Transition)
	}

	// Create a copy to prevent external modification
	result := make(map[Event]Transition, len(transitions))
	for event, transition := range transitions {
		result[event] = transition
	}
	return result
}
