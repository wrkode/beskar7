//go:build integration

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

package emulation

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"
)

// VendorType represents different hardware vendors
type VendorType string

const (
	VendorDell       VendorType = "Dell Inc."
	VendorHPE        VendorType = "HPE"
	VendorLenovo     VendorType = "Lenovo"
	VendorSupermicro VendorType = "Supermicro"
	VendorGeneric    VendorType = "Generic"
)

// PowerState represents the current power state
type PowerState string

const (
	PowerStateOff              PowerState = "Off"
	PowerStateOn               PowerState = "On"
	PowerStateForceOff         PowerState = "ForceOff"
	PowerStateForceRestart     PowerState = "ForceRestart"
	PowerStateGracefulShutdown PowerState = "GracefulShutdown"
	PowerStateGracefulRestart  PowerState = "GracefulRestart"
)

// BootSourceOverrideTarget represents boot override targets
type BootSourceOverrideTarget string

const (
	BootSourceNone       BootSourceOverrideTarget = "None"
	BootSourcePxe        BootSourceOverrideTarget = "Pxe"
	BootSourceHdd        BootSourceOverrideTarget = "Hdd"
	BootSourceCd         BootSourceOverrideTarget = "Cd"
	BootSourceUefiTarget BootSourceOverrideTarget = "UefiTarget"
)

// MockRedfishServer represents a mock Redfish BMC server
type MockRedfishServer struct {
	server         *httptest.Server
	vendor         VendorType
	mu             sync.RWMutex
	powerState     PowerState
	bootSource     BootSourceOverrideTarget
	bootParameters []string
	virtualMedia   []VirtualMedia
	biosAttributes map[string]interface{}
	systemInfo     SystemInfo
	failures       FailureConfig
	requestLog     []RequestLog
	authEnabled    bool
	credentials    map[string]string // username:password
}

// SystemInfo represents basic system information
type SystemInfo struct {
	Manufacturer   string
	Model          string
	SerialNumber   string
	ProcessorCount int
	MemoryGB       int
	PowerState     PowerState
	Health         string
	UUID           string
}

// VirtualMedia represents virtual media configuration
type VirtualMedia struct {
	ID             string
	ImageURL       string
	Inserted       bool
	WriteProtected bool
	ConnectedVia   string
}

// FailureConfig configures various failure scenarios
type FailureConfig struct {
	NetworkErrors   bool
	AuthFailures    bool
	SlowResponses   bool
	PartialFailures bool
	VendorQuirks    bool
	PowerFailures   bool
	MediaFailures   bool
}

// RequestLog tracks all requests for debugging
type RequestLog struct {
	Timestamp time.Time
	Method    string
	URL       string
	Body      string
	Response  int
}

// NewMockRedfishServer creates a new mock Redfish server
func NewMockRedfishServer(vendor VendorType) *MockRedfishServer {
	mrs := &MockRedfishServer{
		vendor:         vendor,
		powerState:     PowerStateOn,
		bootSource:     BootSourceNone,
		bootParameters: make([]string, 0),
		virtualMedia:   make([]VirtualMedia, 2), // CD and USB
		biosAttributes: make(map[string]interface{}),
		failures:       FailureConfig{},
		requestLog:     make([]RequestLog, 0),
		authEnabled:    true,
		credentials:    map[string]string{"admin": "password123"},
	}

	// Initialize virtual media slots
	mrs.virtualMedia[0] = VirtualMedia{
		ID:             "CD",
		Inserted:       false,
		WriteProtected: true,
		ConnectedVia:   "URI",
	}
	mrs.virtualMedia[1] = VirtualMedia{
		ID:             "USB",
		Inserted:       false,
		WriteProtected: false,
		ConnectedVia:   "URI",
	}

	// Set vendor-specific system info
	mrs.initializeSystemInfo()

	// Initialize BIOS attributes based on vendor
	mrs.initializeBIOSAttributes()

	// Create HTTP server
	mrs.server = httptest.NewTLSServer(http.HandlerFunc(mrs.handleRequest))

	return mrs
}

// GetURL returns the server URL
func (mrs *MockRedfishServer) GetURL() string {
	return mrs.server.URL
}

// Close shuts down the mock server
func (mrs *MockRedfishServer) Close() {
	mrs.server.Close()
}

// SetFailureMode enables/disables failure scenarios
func (mrs *MockRedfishServer) SetFailureMode(config FailureConfig) {
	mrs.mu.Lock()
	defer mrs.mu.Unlock()
	mrs.failures = config
}

// GetRequestLog returns the request log for debugging
func (mrs *MockRedfishServer) GetRequestLog() []RequestLog {
	mrs.mu.RLock()
	defer mrs.mu.RUnlock()
	logCopy := make([]RequestLog, len(mrs.requestLog))
	copy(logCopy, mrs.requestLog)
	return logCopy
}

// SetCredentials configures authentication
func (mrs *MockRedfishServer) SetCredentials(username, password string) {
	mrs.mu.Lock()
	defer mrs.mu.Unlock()
	mrs.credentials = map[string]string{username: password}
}

// DisableAuth disables authentication (for testing)
func (mrs *MockRedfishServer) DisableAuth() {
	mrs.mu.Lock()
	defer mrs.mu.Unlock()
	mrs.authEnabled = false
}

// initializeSystemInfo sets vendor-specific system information
func (mrs *MockRedfishServer) initializeSystemInfo() {
	switch mrs.vendor {
	case VendorDell:
		mrs.systemInfo = SystemInfo{
			Manufacturer:   "Dell Inc.",
			Model:          "PowerEdge R750",
			SerialNumber:   "DELL123456789",
			ProcessorCount: 2,
			MemoryGB:       128,
			PowerState:     PowerStateOn,
			Health:         "OK",
			UUID:           "4c4c4544-0033-3310-8051-b4c04f4d3132",
		}
	case VendorHPE:
		mrs.systemInfo = SystemInfo{
			Manufacturer:   "HPE",
			Model:          "ProLiant DL380 Gen10",
			SerialNumber:   "HPE987654321",
			ProcessorCount: 2,
			MemoryGB:       64,
			PowerState:     PowerStateOn,
			Health:         "OK",
			UUID:           "30373237-3132-584d-5131-333032584d51",
		}
	case VendorLenovo:
		mrs.systemInfo = SystemInfo{
			Manufacturer:   "Lenovo",
			Model:          "ThinkSystem SR650",
			SerialNumber:   "LEN555666777",
			ProcessorCount: 2,
			MemoryGB:       96,
			PowerState:     PowerStateOn,
			Health:         "OK",
			UUID:           "01234567-89ab-cdef-0123-456789abcdef",
		}
	case VendorSupermicro:
		mrs.systemInfo = SystemInfo{
			Manufacturer:   "Supermicro",
			Model:          "X12DPi-NT6",
			SerialNumber:   "SMC111222333",
			ProcessorCount: 2,
			MemoryGB:       256,
			PowerState:     PowerStateOn,
			Health:         "OK",
			UUID:           "fedcba98-7654-3210-fedc-ba9876543210",
		}
	default:
		mrs.systemInfo = SystemInfo{
			Manufacturer:   "Generic Manufacturer",
			Model:          "Generic Server",
			SerialNumber:   "GEN000111222",
			ProcessorCount: 1,
			MemoryGB:       32,
			PowerState:     PowerStateOn,
			Health:         "OK",
			UUID:           "12345678-1234-5678-1234-123456789012",
		}
	}
}

// initializeBIOSAttributes sets vendor-specific BIOS attributes
func (mrs *MockRedfishServer) initializeBIOSAttributes() {
	switch mrs.vendor {
	case VendorDell:
		mrs.biosAttributes["KernelArgs"] = ""
		mrs.biosAttributes["BootMode"] = "Uefi"
		mrs.biosAttributes["SecureBoot"] = "Enabled"
	case VendorHPE:
		mrs.biosAttributes["BootOrderPolicy"] = "AttemptOnce"
		mrs.biosAttributes["UefiOptimizedBoot"] = "Enabled"
		mrs.biosAttributes["SecureBootStatus"] = "Enabled"
	case VendorLenovo:
		mrs.biosAttributes["SystemBootSequence"] = "UEFI First"
		mrs.biosAttributes["SecureBootEnable"] = "Enabled"
	case VendorSupermicro:
		mrs.biosAttributes["BootFeature"] = "UEFI"
		mrs.biosAttributes["QuietBoot"] = "Enabled"
	}
}

// handleRequest is the main HTTP request handler
func (mrs *MockRedfishServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	// Log the request
	mrs.logRequest(r)

	// Simulate slow responses if configured
	if mrs.failures.SlowResponses {
		time.Sleep(5 * time.Second)
	}

	// Simulate network errors for non-root requests so client can still connect
	if mrs.failures.NetworkErrors && r.URL.Path != "/redfish/v1/" {
		http.Error(w, "Network Error", http.StatusInternalServerError)
		return
	}

	// Handle authentication
	if mrs.authEnabled && !mrs.authenticate(r) {
		w.Header().Set("WWW-Authenticate", `Basic realm="Redfish"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Set common headers
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("OData-Version", "4.0")

	// Route the request
	switch {
	case r.URL.Path == "/redfish/v1/" && r.Method == http.MethodGet:
		mrs.handleServiceRoot(w, r)
	case r.URL.Path == "/redfish/v1/Systems" && r.Method == http.MethodGet:
		mrs.handleSystemsCollection(w, r)
	case strings.HasPrefix(r.URL.Path, "/redfish/v1/Systems/") && r.Method == http.MethodGet:
		mrs.handleSystemGet(w, r)
	case r.URL.Path == "/redfish/v1/Systems/1" && (r.Method == http.MethodPatch || r.Method == http.MethodPost):
		// Accept Boot Set requests
		w.WriteHeader(http.StatusNoContent)
	case strings.HasPrefix(r.URL.Path, "/redfish/v1/Systems/") && strings.HasSuffix(r.URL.Path, "/Actions/ComputerSystem.Reset") && r.Method == http.MethodPost:
		mrs.handleSystemReset(w, r)
	case r.URL.Path == "/redfish/v1/Managers" && r.Method == http.MethodGet:
		mrs.handleManagersCollection(w, r)
	case strings.HasPrefix(r.URL.Path, "/redfish/v1/Managers/") && strings.HasSuffix(r.URL.Path, "/VirtualMedia") && r.Method == http.MethodGet:
		mrs.handleManagerVirtualMediaCollection(w, r)
	case strings.HasPrefix(r.URL.Path, "/redfish/v1/Managers/") && strings.Contains(r.URL.Path, "/VirtualMedia/") && strings.Contains(r.URL.Path, "/Actions/"):
		// Accept any VirtualMedia actions
		w.WriteHeader(http.StatusNoContent)
	case strings.HasPrefix(r.URL.Path, "/redfish/v1/Managers/") && strings.Contains(r.URL.Path, "/VirtualMedia/") && r.Method == http.MethodGet:
		mrs.handleVirtualMediaGet(w, r)
	case strings.HasPrefix(r.URL.Path, "/redfish/v1/Managers/") && r.Method == http.MethodGet:
		mrs.handleManagerGet(w, r)
	case strings.Contains(r.URL.Path, "VirtualMedia"):
		// Basic virtual media endpoints are handled via SetBootSourceISO in client tests
		w.WriteHeader(http.StatusNotFound)
	case strings.Contains(r.URL.Path, "Bios"):
		// BIOS attribute GET/PATCH can be added if tests require deeper emulation
		w.WriteHeader(http.StatusNotFound)
	default:
		http.NotFound(w, r)
	}
}

// authenticate validates basic authentication
func (mrs *MockRedfishServer) authenticate(r *http.Request) bool {
	if mrs.failures.AuthFailures {
		return false
	}

	username, password, ok := r.BasicAuth()
	if !ok {
		return false
	}

	mrs.mu.RLock()
	defer mrs.mu.RUnlock()

	expectedPassword, exists := mrs.credentials[username]
	return exists && expectedPassword == password
}

// logRequest logs the HTTP request for debugging
func (mrs *MockRedfishServer) logRequest(r *http.Request) {
	mrs.mu.Lock()
	defer mrs.mu.Unlock()

	body := ""
	if r.Body != nil {
		// Note: In real implementation, you'd need to handle body reading properly
		// This is simplified for the example
	}

	mrs.requestLog = append(mrs.requestLog, RequestLog{
		Timestamp: time.Now(),
		Method:    r.Method,
		URL:       r.URL.String(),
		Body:      body,
		Response:  200, // Will be updated if different
	})
}

// handleServiceRoot handles /redfish/v1/
func (mrs *MockRedfishServer) handleServiceRoot(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"@odata.type":    "#ServiceRoot.v1_5_0.ServiceRoot",
		"@odata.id":      "/redfish/v1/",
		"Id":             "RootService",
		"Name":           "Root Service",
		"RedfishVersion": "1.6.1",
		"UUID":           mrs.systemInfo.UUID,
		"Systems": map[string]string{
			"@odata.id": "/redfish/v1/Systems",
		},
		"Managers": map[string]string{
			"@odata.id": "/redfish/v1/Managers",
		},
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		// best-effort in tests: respond with 500 if encoding fails
		http.Error(w, "encode error", http.StatusInternalServerError)
		return
	}
}

// handleSystemsCollection handles /redfish/v1/Systems
func (mrs *MockRedfishServer) handleSystemsCollection(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"@odata.type":         "#ComputerSystemCollection.ComputerSystemCollection",
		"@odata.id":           "/redfish/v1/Systems",
		"Name":                "Computer System Collection",
		"Members@odata.count": 1,
		"Members": []map[string]string{
			{"@odata.id": "/redfish/v1/Systems/1"},
		},
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "encode error", http.StatusInternalServerError)
		return
	}
}

// handleManagersCollection handles /redfish/v1/Managers
func (mrs *MockRedfishServer) handleManagersCollection(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"@odata.type":         "#ManagerCollection.ManagerCollection",
		"@odata.id":           "/redfish/v1/Managers",
		"Name":                "Manager Collection",
		"Members@odata.count": 1,
		"Members": []map[string]string{
			{"@odata.id": "/redfish/v1/Managers/1"},
		},
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "encode error", http.StatusInternalServerError)
		return
	}
}

// handleManagerGet handles GET /redfish/v1/Managers/1
func (mrs *MockRedfishServer) handleManagerGet(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"@odata.type": "#Manager.v1_9_0.Manager",
		"@odata.id":   "/redfish/v1/Managers/1",
		"Id":          "1",
		"Name":        "BMCManager",
		"Actions": map[string]interface{}{
			"#Manager.Reset": map[string]string{
				"target": "/redfish/v1/Managers/1/Actions/Manager.Reset",
			},
		},
		"VirtualMedia": map[string]string{
			"@odata.id": "/redfish/v1/Managers/1/VirtualMedia",
		},
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "encode error", http.StatusInternalServerError)
		return
	}
}

// handleManagerVirtualMediaCollection handles GET /redfish/v1/Managers/1/VirtualMedia
func (mrs *MockRedfishServer) handleManagerVirtualMediaCollection(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"@odata.type":         "#VirtualMediaCollection.VirtualMediaCollection",
		"@odata.id":           "/redfish/v1/Managers/1/VirtualMedia",
		"Name":                "Virtual Media Collection",
		"Members@odata.count": 1,
		"Members": []map[string]string{
			{"@odata.id": "/redfish/v1/Managers/1/VirtualMedia/1"},
		},
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "encode error", http.StatusInternalServerError)
		return
	}
}

// handleVirtualMediaGet handles GET /redfish/v1/Managers/1/VirtualMedia/1
func (mrs *MockRedfishServer) handleVirtualMediaGet(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"@odata.type":    "#VirtualMedia.v1_5_0.VirtualMedia",
		"@odata.id":      "/redfish/v1/Managers/1/VirtualMedia/1",
		"Id":             "1",
		"Name":           "Virtual CD",
		"MediaTypes":     []string{"CD", "DVD"},
		"Image":          "",
		"Inserted":       false,
		"WriteProtected": true,
		"ConnectedVia":   "URI",
		"Actions": map[string]map[string]string{
			"#VirtualMedia.InsertMedia": {"target": "/redfish/v1/Managers/1/VirtualMedia/1/Actions/VirtualMedia.InsertMedia"},
			"#VirtualMedia.EjectMedia":  {"target": "/redfish/v1/Managers/1/VirtualMedia/1/Actions/VirtualMedia.EjectMedia"},
		},
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "encode error", http.StatusInternalServerError)
		return
	}
}

// handleSystemGet handles GET /redfish/v1/Systems/1
func (mrs *MockRedfishServer) handleSystemGet(w http.ResponseWriter, r *http.Request) {
	mrs.mu.RLock()
	defer mrs.mu.RUnlock()

	response := map[string]interface{}{
		"@odata.type":  "#ComputerSystem.v1_10_0.ComputerSystem",
		"@odata.id":    "/redfish/v1/Systems/1",
		"Id":           "1",
		"Name":         "System",
		"SystemType":   "Physical",
		"Manufacturer": mrs.systemInfo.Manufacturer,
		"Model":        mrs.systemInfo.Model,
		"SerialNumber": mrs.systemInfo.SerialNumber,
		"UUID":         mrs.systemInfo.UUID,
		"ProcessorSummary": map[string]interface{}{
			"Count": mrs.systemInfo.ProcessorCount,
		},
		"MemorySummary": map[string]interface{}{
			"TotalSystemMemoryGiB": mrs.systemInfo.MemoryGB,
		},
		"PowerState": mrs.powerState,
		"Status": map[string]string{
			"State":  "Enabled",
			"Health": mrs.systemInfo.Health,
		},
		"Boot": map[string]interface{}{
			"BootSourceOverrideTarget":  mrs.bootSource,
			"BootSourceOverrideEnabled": "Once",
		},
		"Actions": map[string]interface{}{
			"#ComputerSystem.Reset": map[string]string{
				"target": "/redfish/v1/Systems/1/Actions/ComputerSystem.Reset",
			},
		},
		"Bios": map[string]string{
			"@odata.id": "/redfish/v1/Systems/1/Bios",
		},
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "encode error", http.StatusInternalServerError)
		return
	}
}

// handleSystemReset handles system power actions
func (mrs *MockRedfishServer) handleSystemReset(w http.ResponseWriter, r *http.Request) {
	if mrs.failures.PowerFailures {
		http.Error(w, "Power operation failed", http.StatusInternalServerError)
		return
	}

	var resetRequest struct {
		ResetType string `json:"ResetType"`
	}

	if err := json.NewDecoder(r.Body).Decode(&resetRequest); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	mrs.mu.Lock()
	defer mrs.mu.Unlock()

	switch resetRequest.ResetType {
	case "On":
		mrs.powerState = PowerStateOn
	case "ForceOff":
		mrs.powerState = PowerStateOff
	case "GracefulShutdown":
		mrs.powerState = PowerStateOff
	case "ForceRestart":
		mrs.powerState = PowerStateOn
	default:
		http.Error(w, "Invalid ResetType", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Additional handler methods would be implemented here...
// handleManagerRequest, handleVirtualMediaRequest, handleBIOSRequest, etc.
