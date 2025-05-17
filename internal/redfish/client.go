package redfish

import (
	"context"

	"github.com/stmcginnis/gofish/common"
	"github.com/stmcginnis/gofish/redfish"
)

// Power state constants
const (
	OnPowerState  = "On"
	OffPowerState = "Off"
)

// Client defines the interface for interacting with a Redfish API.
type Client interface {
	// GetSystemInfo retrieves basic system details.
	GetSystemInfo(ctx context.Context) (*SystemInfo, error)

	// GetPowerState retrieves the current power state of the system.
	GetPowerState(ctx context.Context) (redfish.PowerState, error)

	// SetPowerState sets the desired power state of the system.
	SetPowerState(ctx context.Context, state redfish.PowerState) error

	// SetBootSourceISO configures the system to boot from a given ISO URL via VirtualMedia.
	// It finds an available virtual media device, inserts the ISO, and sets it as the boot target.
	SetBootSourceISO(ctx context.Context, isoURL string) error

	// EjectVirtualMedia ejects any inserted virtual media.
	EjectVirtualMedia(ctx context.Context) error

	// SetBootParameters configures kernel command line parameters for the next boot.
	// These are typically applied when booting from an ISO via VirtualMedia.
	// The parameters are for one-time boot.
	SetBootParameters(ctx context.Context, params []string) error

	// Close closes the connection to the Redfish service.
	Close(ctx context.Context)
}

// RedfishClientFactory defines the signature for a function that creates a Redfish client.
// It is defined here to be shared between PhysicalHost and Beskar7Machine controllers.
type RedfishClientFactory func(ctx context.Context, address, username, password string, insecure bool) (Client, error)

// SystemInfo holds basic hardware details retrieved from Redfish.
type SystemInfo struct {
	Manufacturer string
	Model        string
	SerialNumber string
	Status       common.Status
}

// RedfishClientFactory type definition should be here ...

// (The gofishClient struct and its methods remain in gofish_client.go, not here)
// Remove the MockClient and NewMockClient that were just added if they are here.

// EjectVirtualMedia ejects any inserted virtual media.
// TODO: Implement the actual Redfish calls to eject media

// SetBootParameters configures kernel command line parameters for the next boot.
// TODO: Implement the actual Redfish calls to set UEFI boot options.

// Close disconnects from the Redfish service.
