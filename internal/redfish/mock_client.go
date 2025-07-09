package redfish

import (
	"context"
	"fmt"
	"sync"

	"github.com/stmcginnis/gofish/common"
	"github.com/stmcginnis/gofish/redfish"
)

// MockClient provides a mock implementation of the Client interface for testing.
type MockClient struct {
	mu sync.Mutex // Protect access to mock state

	// Mockable fields
	SystemInfo      *SystemInfo
	PowerState      redfish.PowerState
	ShouldFail      map[string]error // Map method name to error to simulate failures
	InsertedISO     string
	BootSourceIsISO bool

	// Network address fields
	NetworkAddresses        []NetworkAddress
	GetNetworkAddressesFunc func(ctx context.Context) ([]NetworkAddress, error)

	// Counters (optional, for verification)
	CloseCalled         bool
	GetSystemInfoCalled bool
	GetPowerStateCalled bool
	SetPowerStateCalled bool
	SetBootSourceCalled bool
	EjectMediaCalled    bool
	// Add fields for SetBootParameters
	SetBootParametersFunc     func(ctx context.Context, params []string) error
	SetBootParametersCalled   bool
	StoredBootParams          []string // To store the parameters for assertion
	GetNetworkAddressesCalled bool

	// New fields for SetBootParametersWithAnnotations
	SetBootParametersWithAnnotationsFunc   func(ctx context.Context, params []string, annotations map[string]string) error
	SetBootParametersWithAnnotationsCalled bool
}

// NewMockClient creates a new mock client with default values.
func NewMockClient() *MockClient {
	return &MockClient{
		SystemInfo: &SystemInfo{
			Manufacturer: "MockInc",
			Model:        "MockSystem",
			SerialNumber: "MOCK12345",
			Status:       common.Status{State: common.EnabledState},
		},
		PowerState: redfish.OffPowerState,
		ShouldFail: make(map[string]error),
		// Initialize new fields
		StoredBootParams: nil,
	}
}

// failIfNeeded checks if a method call should fail based on the ShouldFail map.
func (m *MockClient) failIfNeeded(methodName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err, ok := m.ShouldFail[methodName]; ok {
		return err
	}
	return nil
}

// GetSystemInfo mock implementation.
func (m *MockClient) GetSystemInfo(ctx context.Context) (*SystemInfo, error) {
	m.mu.Lock()
	m.GetSystemInfoCalled = true
	m.mu.Unlock()
	if err := m.failIfNeeded("GetSystemInfo"); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.SystemInfo, nil
}

// GetPowerState mock implementation.
func (m *MockClient) GetPowerState(ctx context.Context) (redfish.PowerState, error) {
	m.mu.Lock()
	m.GetPowerStateCalled = true
	m.mu.Unlock()
	if err := m.failIfNeeded("GetPowerState"); err != nil {
		return "", err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.PowerState, nil
}

// SetPowerState mock implementation.
func (m *MockClient) SetPowerState(ctx context.Context, state redfish.PowerState) error {
	m.mu.Lock()
	m.SetPowerStateCalled = true
	m.mu.Unlock()
	if err := m.failIfNeeded("SetPowerState"); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PowerState = state
	return nil
}

// SetBootSourceISO mock implementation.
func (m *MockClient) SetBootSourceISO(ctx context.Context, isoURL string) error {
	m.mu.Lock()
	m.SetBootSourceCalled = true
	m.mu.Unlock()
	if err := m.failIfNeeded("SetBootSourceISO"); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.InsertedISO != "" && m.InsertedISO != isoURL {
		return fmt.Errorf("mock error: different media already inserted (%s)", m.InsertedISO)
	}
	m.InsertedISO = isoURL
	m.BootSourceIsISO = true
	return nil
}

// EjectVirtualMedia mock implementation.
func (m *MockClient) EjectVirtualMedia(ctx context.Context) error {
	m.mu.Lock()
	m.EjectMediaCalled = true
	m.mu.Unlock()
	if err := m.failIfNeeded("EjectVirtualMedia"); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.InsertedISO = ""
	m.BootSourceIsISO = false
	return nil
}

// SetBootParameters mock implementation.
func (m *MockClient) SetBootParameters(ctx context.Context, params []string) error {
	m.mu.Lock()
	m.SetBootParametersCalled = true
	// Store a copy of the params. If params is nil, StoredBootParams will be nil.
	if params != nil {
		m.StoredBootParams = make([]string, len(params))
		copy(m.StoredBootParams, params)
	} else {
		m.StoredBootParams = nil
	}
	m.mu.Unlock()

	if err := m.failIfNeeded("SetBootParameters"); err != nil {
		return err
	}
	if m.SetBootParametersFunc != nil {
		return m.SetBootParametersFunc(ctx, params)
	}
	return nil
}

// SetBootParametersWithAnnotations mock implementation.
func (m *MockClient) SetBootParametersWithAnnotations(ctx context.Context, params []string, annotations map[string]string) error {
	m.mu.Lock()
	m.SetBootParametersWithAnnotationsCalled = true
	m.mu.Unlock()
	if err := m.failIfNeeded("SetBootParametersWithAnnotations"); err != nil {
		return err
	}
	if m.SetBootParametersWithAnnotationsFunc != nil {
		return m.SetBootParametersWithAnnotationsFunc(ctx, params, annotations)
	}
	return nil
}

// GetNetworkAddresses mock implementation.
func (m *MockClient) GetNetworkAddresses(ctx context.Context) ([]NetworkAddress, error) {
	m.mu.Lock()
	m.GetNetworkAddressesCalled = true
	m.mu.Unlock()

	if err := m.failIfNeeded("GetNetworkAddresses"); err != nil {
		return nil, err
	}
	if m.GetNetworkAddressesFunc != nil {
		return m.GetNetworkAddressesFunc(ctx)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	// Return a copy of the stored network addresses
	addresses := make([]NetworkAddress, len(m.NetworkAddresses))
	copy(addresses, m.NetworkAddresses)
	return addresses, nil
}

// Close mock implementation.
func (m *MockClient) Close(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CloseCalled = true
}
