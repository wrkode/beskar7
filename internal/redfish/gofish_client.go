package redfish

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/common"
	"github.com/stmcginnis/gofish/redfish"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// goFishClient implements the Client interface using the gofish library.
type gofishClient struct {
	gofishClient *gofish.APIClient
	apiEndpoint  string // Store the original endpoint address
}

var log = logf.Log.WithName("redfish-client")

// NewClient creates a new Redfish client.
func NewClient(ctx context.Context, address, username, password string, insecure bool) (Client, error) {
	log.Info("Creating new Redfish client", "address", address, "insecure", insecure)

	// Ensure address has a scheme
	if !strings.HasPrefix(address, "http://") && !strings.HasPrefix(address, "https://") {
		address = "https://" + address // Default to https
	}

	config := gofish.ClientConfig{
		Endpoint: address,
		Username: username,
		Password: password,
		Insecure: insecure,
	}

	c, err := gofish.ConnectContext(ctx, config)
	if err != nil {
		log.Error(err, "Failed to connect to Redfish endpoint", "address", address)
		return nil, fmt.Errorf("failed to connect to Redfish endpoint %s: %w", address, err)
	}

	log.Info("Successfully connected to Redfish endpoint", "address", address)
	return &gofishClient{
		gofishClient: c,
		apiEndpoint:  address, // Use the processed address
	}, nil
}

// Close disconnects the client.
func (c *gofishClient) Close(ctx context.Context) {
	if c.gofishClient != nil {
		log.Info("Disconnecting Redfish client", "address", c.apiEndpoint)
		c.gofishClient.Logout()
		c.gofishClient = nil
	}
}

// getSystemService retrieves the first ComputerSystem instance.
// Helper function to avoid repetition.
func (c *gofishClient) getSystemService(ctx context.Context) (*redfish.ComputerSystem, error) {
	if c.gofishClient == nil {
		return nil, fmt.Errorf("Redfish client is not connected")
	}
	service := c.gofishClient.Service
	systems, err := service.Systems()
	if err != nil {
		log.Error(err, "Failed to retrieve systems from Redfish service root")
		return nil, fmt.Errorf("failed to retrieve systems: %w", err)
	}
	if len(systems) == 0 {
		log.Error(nil, "No systems found on Redfish endpoint")
		return nil, fmt.Errorf("no systems found")
	}
	if len(systems) > 1 {
		log.Info("Multiple systems found, using the first one", "systemID", systems[0].ID)
	}
	return systems[0], nil
}

// GetSystemInfo retrieves basic system details.
func (c *gofishClient) GetSystemInfo(ctx context.Context) (*SystemInfo, error) {
	system, err := c.getSystemService(ctx)
	if err != nil {
		return nil, err
	}

	info := &SystemInfo{
		Manufacturer: system.Manufacturer,
		Model:        system.Model,
		SerialNumber: system.SerialNumber,
		Status:       system.Status,
	}
	log.Info("Retrieved system info", "Manufacturer", info.Manufacturer, "Model", info.Model, "SerialNumber", info.SerialNumber, "Status", info.Status.State)
	return info, nil
}

// GetPowerState retrieves the current power state of the system.
func (c *gofishClient) GetPowerState(ctx context.Context) (redfish.PowerState, error) {
	system, err := c.getSystemService(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get system for power state check: %w", err)
	}
	log.Info("Retrieved power state", "state", system.PowerState)
	return system.PowerState, nil
}

// SetPowerState sets the desired power state of the system.
func (c *gofishClient) SetPowerState(ctx context.Context, state redfish.PowerState) error {
	system, err := c.getSystemService(ctx)
	if err != nil {
		return fmt.Errorf("failed to get system for setting power state: %w", err)
	}

	var resetType redfish.ResetType
	switch state {
	case redfish.OnPowerState:
		resetType = redfish.OnResetType
	case redfish.OffPowerState:
		resetType = redfish.ForceOffResetType // Or OffResetType?
	default:
		// Try direct conversion if it matches a ResetType
		switch redfish.ResetType(state) {
		case redfish.OnResetType, redfish.ForceOffResetType, redfish.GracefulShutdownResetType, redfish.GracefulRestartResetType, redfish.ForceRestartResetType, redfish.NmiResetType, redfish.ForceOnResetType, redfish.PushPowerButtonResetType, redfish.PowerCycleResetType:
			resetType = redfish.ResetType(state)
		default:
			return fmt.Errorf("unsupported power state or reset type for SetPowerState: %s", state)
		}
	}

	log.Info("Attempting to set power state", "desiredState", state, "resetType", resetType)
	err = system.Reset(resetType) // Use Reset method with correct ResetType
	if err != nil {
		log.Error(err, "Failed to set power state", "desiredState", state)
		return fmt.Errorf("failed to set power state to %s: %w", state, err)
	}
	log.Info("Successfully requested power state change", "desiredState", state)
	return nil
}

// findFirstVirtualMedia finds the first available virtual media device (CD or DVD type).
func (c *gofishClient) findFirstVirtualMedia(ctx context.Context) (*redfish.VirtualMedia, error) {
	if c.gofishClient == nil {
		return nil, fmt.Errorf("Redfish client is not connected")
	}
	// Find the manager associated with the system
	system, err := c.getSystemService(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get system to find manager: %w", err)
	}
	// Call the ManagedBy() method to get manager links
	mgrLinks, err := system.ManagedBy()
	if err != nil {
		return nil, fmt.Errorf("failed to get managers for system %s: %w", system.ID, err)
	}
	if len(mgrLinks) == 0 {
		return nil, fmt.Errorf("system %s has no manager links", system.ID)
	}

	// Assume the first manager is the relevant one
	mgr, err := redfish.GetManager(c.gofishClient, mgrLinks[0].ODataID) // Use ODataID from the link
	if err != nil {
		return nil, fmt.Errorf("failed to get manager %s: %w", mgrLinks[0].ODataID, err)
	}

	// Get Virtual Media collection
	virtualMedia, err := mgr.VirtualMedia()
	if err != nil {
		log.Error(err, "Failed to retrieve virtual media collection", "manager", mgr.ID)
		return nil, fmt.Errorf("failed to get virtual media for manager %s: %w", mgr.ID, err)
	}

	// Find the first CD/DVD type virtual media
	for _, vm := range virtualMedia {
		for _, mediaType := range vm.MediaTypes {
			if mediaType == redfish.CDMediaType || mediaType == redfish.DVDMediaType {
				log.Info("Found suitable virtual media device", "vmID", vm.ID)
				return vm, nil
			}
		}
	}

	log.Error(nil, "No suitable virtual media device (CD/DVD) found", "manager", mgr.ID)
	return nil, fmt.Errorf("no suitable virtual media (CD/DVD) found for manager %s", mgr.ID)
}

// SetBootSourceISO configures the system to boot from a given ISO URL via VirtualMedia.
func (c *gofishClient) SetBootSourceISO(ctx context.Context, isoURL string) error {
	vm, err := c.findFirstVirtualMedia(ctx)
	if err != nil {
		return fmt.Errorf("failed to find virtual media device: %w", err)
	}

	log.Info("Attempting to insert virtual media", "vmID", vm.ID, "isoURL", isoURL)
	err = vm.InsertMedia(isoURL, true, false) // Insert, make it bootable
	if err != nil {
		// Check if media is already inserted - some BMCs might return an error
		if vm.MediaTypes != nil && vm.Image != "" {
			log.Info("Virtual media possibly already inserted", "vmID", vm.ID, "currentImage", vm.Image)
			if vm.Image == isoURL {
				log.Info("Correct ISO already inserted.")
				// Still need to ensure boot order
			} else {
				log.Info("Different media inserted, attempting eject first", "vmID", vm.ID)
				if ejectErr := vm.EjectMedia(); ejectErr != nil {
					log.Error(ejectErr, "Failed to eject existing media before inserting new ISO", "vmID", vm.ID)
					// Proceeding with insert might still work or fail cleanly
				}
				err = vm.InsertMedia(isoURL, true, false) // Retry insert
				if err != nil {
					log.Error(err, "Failed to insert virtual media after eject attempt", "vmID", vm.ID, "isoURL", isoURL)
					return fmt.Errorf("failed to insert virtual media %s: %w", isoURL, err)
				}
			}
		} else {
			log.Error(err, "Failed to insert virtual media", "vmID", vm.ID, "isoURL", isoURL)
			return fmt.Errorf("failed to insert virtual media %s: %w", isoURL, err)
		}
	}

	// Set boot order to boot from CD/DVD once
	system, err := c.getSystemService(ctx)
	if err != nil {
		return fmt.Errorf("failed to get system to set boot order: %w", err)
	}

	boot := redfish.Boot{
		BootSourceOverrideTarget:  redfish.CdBootSourceOverrideTarget,    // Target CD/DVD
		BootSourceOverrideEnabled: redfish.OnceBootSourceOverrideEnabled, // Boot from it once
		// BootSourceOverrideMode indicates UEFI or BIOS. Assuming UEFI is default or desired.
		// BootSourceOverrideMode: redfish.UEFIBootSourceOverrideMode,
	}
	log.Info("Attempting to set boot source override", "target", boot.BootSourceOverrideTarget, "enabled", boot.BootSourceOverrideEnabled)
	err = system.SetBoot(boot)
	if err != nil {
		log.Error(err, "Failed to set boot source override")
		return fmt.Errorf("failed to set boot source override: %w", err)
	}

	log.Info("Successfully set boot source to virtual media ISO", "vmID", vm.ID, "isoURL", isoURL)
	return nil
}

// EjectVirtualMedia ejects any inserted virtual media.
func (c *gofishClient) EjectVirtualMedia(ctx context.Context) error {
	vm, err := c.findFirstVirtualMedia(ctx)
	if err != nil {
		// If no suitable VM device is found, maybe that's okay for eject?
		if strings.Contains(err.Error(), "no suitable virtual media") {
			log.Info("No suitable virtual media device found to eject from.")
			return nil
		}
		return fmt.Errorf("failed to find virtual media device for eject: %w", err)
	}

	// Check if media is inserted by looking at the Image field
	if vm.Image == "" {
		log.Info("No virtual media currently inserted.", "vmID", vm.ID)
		return nil
	}

	log.Info("Attempting to eject virtual media", "vmID", vm.ID, "currentImage", vm.Image)
	err = vm.EjectMedia()
	if err != nil {
		log.Error(err, "Failed to eject virtual media", "vmID", vm.ID)
		return fmt.Errorf("failed to eject virtual media on %s: %w", vm.ID, err)
	}

	log.Info("Successfully ejected virtual media", "vmID", vm.ID)
	return nil
}

// SetBootParameters configures kernel command line parameters for the next boot.
func (c *gofishClient) SetBootParameters(ctx context.Context, params []string) error {
	log := logf.FromContext(ctx)
	log.Info("Attempting to set boot parameters", "Params", params)

	system, err := c.getSystemService(ctx)
	if err != nil {
		return fmt.Errorf("failed to get system to set boot parameters: %w", err)
	}

	var bootSettings redfish.Boot

	if len(params) == 0 {
		log.Info("Clearing one-time UEFI boot parameters via UefiTargetBootSourceOverride by disabling override.")
		bootSettings = redfish.Boot{
			BootSourceOverrideEnabled:    redfish.DisabledBootSourceOverrideEnabled,
			BootSourceOverrideTarget:     redfish.NoneBootSourceOverrideTarget,
			UefiTargetBootSourceOverride: "", // Explicitly try to clear it
		}
	} else {
		// Default EFI bootloader path. This is a common path but might need to be configurable
		// or discovered for different ISOs/OSes. Examples:
		// - "\\EFI\\BOOT\\BOOTX64.EFI"
		// - "/efi/boot/bootx64.efi"
		// - Path to shimx64.efi for Secure Boot systems, then grubx64.efi
		efiBootloaderPath := "\\EFI\\BOOT\\BOOTX64.EFI"

		fullBootString := efiBootloaderPath + " " + strings.Join(params, " ")
		log.Info("Attempting to set UEFI boot target with parameters", "UefiTarget", fullBootString)

		bootSettings = redfish.Boot{
			BootSourceOverrideTarget:     redfish.UefiTargetBootSourceOverrideTarget,
			BootSourceOverrideEnabled:    redfish.OnceBootSourceOverrideEnabled,
			UefiTargetBootSourceOverride: fullBootString,
		}
	}

	log.Info("Applying boot settings via UefiTargetBootSourceOverride", "Settings", bootSettings)
	err = system.SetBoot(bootSettings)
	if err != nil {
		// If this fails, it could be due to several reasons:
		// 1. The BMC does not support UefiTargetBootSourceOverride.
		// 2. The BMC does not allow appending parameters to the UefiTargetBootSourceOverride string.
		// 3. The provided efiBootloaderPath is incorrect for the target ISO.
		// 4. A transient communication error with the BMC.
		//
		// Setting kernel parameters via Redfish is highly vendor-dependent.
		// Another approach involves setting specific BIOS attributes, but these are not standardized
		// and would require vendor-specific knowledge and likely configuration by the user.
		// For now, we rely on UefiTargetBootSourceOverride and log failures clearly.
		// Users may need to use the "PreBakedISO" provisioningMode if this method is unreliable for their hardware.

		// Attempt to get more detailed error information if it's a common.Error
		var redfishError *common.Error
		if errors.As(err, &redfishError) {
			log.Error(redfishError, "Failed to set boot settings via UefiTargetBootSourceOverride (Redfish error)",
				"Settings", bootSettings)
		} else {
			log.Error(err, "Failed to set boot settings via UefiTargetBootSourceOverride (non-Redfish error)",
				"Settings", bootSettings)
		}
		return fmt.Errorf("failed to set boot settings on system using UefiTargetBootSourceOverride: %w", err)
	}

	log.Info("Successfully applied boot settings via UefiTargetBootSourceOverride.")
	return nil
}
