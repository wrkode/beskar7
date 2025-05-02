package redfish

import (
	"context"

	"github.com/stmcginnis/gofish/common"
	"github.com/stmcginnis/gofish/redfish"
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

	// Close closes the connection to the Redfish service.
	Close(ctx context.Context)
}

// SystemInfo holds basic hardware details retrieved from Redfish.
type SystemInfo struct {
	Manufacturer string
	Model        string
	SerialNumber string
	Status       common.Status
}
