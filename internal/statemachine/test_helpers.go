package statemachine

import (
	"context"
	"sync"
	"testing"

	. "github.com/onsi/gomega"
)

// TestAction is a helper type for testing state machine actions
type TestAction struct {
	mu    sync.Mutex
	calls int
	err   error
}

// NewTestAction creates a new TestAction
func NewTestAction() *TestAction {
	return &TestAction{}
}

// Execute increments the call counter and returns the configured error
func (a *TestAction) Execute(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls++
	return a.err
}

// SetError sets the error to be returned by Execute
func (a *TestAction) SetError(err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.err = err
}

// Calls returns the number of times Execute was called
func (a *TestAction) Calls() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.calls
}

// Reset resets the call counter and error
func (a *TestAction) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls = 0
	a.err = nil
}

// AssertStateTransition is a helper function to test state transitions
func AssertStateTransition(t *testing.T, machine StateMachine, ctx context.Context, event Event, expectedState State) {
	g := NewWithT(t)
	err := machine.Transition(ctx, event)
	g.Expect(err).To(BeNil())
	g.Expect(machine.CurrentState()).To(Equal(expectedState))
}

// AssertInvalidTransition is a helper function to test invalid state transitions
func AssertInvalidTransition(t *testing.T, machine StateMachine, ctx context.Context, event Event) {
	g := NewWithT(t)
	err := machine.Transition(ctx, event)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err).To(BeAssignableToTypeOf(&StateMachineError{}))
}

// AssertActionExecuted is a helper function to test if an action was executed
func AssertActionExecuted(t *testing.T, action *TestAction, expectedCalls int) {
	g := NewWithT(t)
	g.Expect(action.Calls()).To(Equal(expectedCalls))
}

// AssertActionNotExecuted is a helper function to test if an action was not executed
func AssertActionNotExecuted(t *testing.T, action *TestAction) {
	g := NewWithT(t)
	g.Expect(action.Calls()).To(Equal(0))
}
