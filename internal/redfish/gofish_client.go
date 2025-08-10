package redfish

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/stmcginnis/gofish"
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
	log := logf.Log.WithName("redfish-client")
	log.Info("Creating new Redfish client", "rawAddress", address, "username", username, "insecure", insecure)

	// Parse and validate the address URL
	parsedURL, err := url.Parse(address)
	if err != nil {
		log.Error(err, "Failed to parse provided Redfish address", "rawAddress", address)
		// If parsing fails, try adding https and parse again
		parsedURL, err = url.Parse("https://" + address)
		if err != nil {
			log.Error(err, "Failed to parse Redfish address even after adding https scheme", "address", address)
			return nil, fmt.Errorf("invalid Redfish address format: %s: %w", address, err)
		}
	}

	// Ensure scheme is present
	if parsedURL.Scheme == "" {
		parsedURL.Scheme = "https" // Default to https
		log.Info("Defaulted address scheme to https", "processedAddress", parsedURL.String())
	}

	// Use the validated and cleaned URL string
	endpointURL := parsedURL.String()

	config := gofish.ClientConfig{
		Endpoint:  endpointURL, // Use the processed URL
		Username:  username,
		Password:  password,
		Insecure:  insecure,
		BasicAuth: true,
		// Cannot pass httpClient directly, gofish creates its own based on these settings.
	}

	// Log the final config before connecting
	log.Info("Attempting gofish.ConnectContext with config",
		"Endpoint", config.Endpoint,
		"Username", config.Username,
		"PasswordProvided", (config.Password != ""),
		"Insecure", config.Insecure,
		"BasicAuth", config.BasicAuth)

	c, err := gofish.ConnectContext(ctx, config) // gofish uses the config fields internally
	if err != nil {
		log.Error(err, "Failed to connect to Redfish endpoint", "address", endpointURL) // Log the processed URL
		return nil, fmt.Errorf("failed to connect to Redfish endpoint %s: %w", endpointURL, err)
	}

	log.Info("Successfully connected to Redfish endpoint", "address", endpointURL)

	return &gofishClient{
		gofishClient: c,
		apiEndpoint:  endpointURL, // Store the processed URL
	}, nil
}

// NewClientWithHTTPClient creates a new Redfish client using a custom HTTP client.
// Note: The underlying gofish library does not currently accept a custom http.Client.
// This function preserves API compatibility for tests and callers that wish to provide
// a client (e.g., to disable TLS verification). The provided httpClient is ignored,
// and the 'insecure' parameter is used to control TLS verification instead.
func NewClientWithHTTPClient(
	ctx context.Context,
	address, username, password string,
	insecure bool,
	_ *http.Client,
) (Client, error) {
	return NewClient(ctx, address, username, password, insecure)
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
// It tries finding the manager via the system's ManagedBy links first, then falls back
// to searching all managers from the service root.
func (c *gofishClient) findFirstVirtualMedia(ctx context.Context) (*redfish.VirtualMedia, error) {
	if c.gofishClient == nil {
		return nil, fmt.Errorf("Redfish client is not connected")
	}
	log := logf.FromContext(ctx)

	// Helper function to search virtual media within a specific manager
	searchManagerVM := func(mgr *redfish.Manager) (*redfish.VirtualMedia, error) {
		virtualMedia, err := mgr.VirtualMedia()
		if err != nil {
			log.Error(err, "Failed to retrieve virtual media collection", "manager", mgr.ID)
			// Don't return error, just skip this manager
			return nil, nil
		}
		for _, vm := range virtualMedia {
			for _, mediaType := range vm.MediaTypes {
				if mediaType == redfish.CDMediaType || mediaType == redfish.DVDMediaType {
					log.Info("Found suitable virtual media device via manager", "vmID", vm.ID, "managerID", mgr.ID)
					return vm, nil
				}
			}
		}
		return nil, nil // Not found in this manager
	}

	// Attempt 1: Via System.ManagedBy
	log.V(1).Info("Attempting to find manager via System.ManagedBy")
	system, err := c.getSystemService(ctx)
	if err != nil {
		log.Error(err, "Failed to get system when searching for virtual media manager")
		// Fallback to searching all managers if getting system fails
	} else {
		mgrLinks, err := system.ManagedBy()
		if err != nil {
			log.Error(err, "Failed to get ManagedBy links from system", "systemID", system.ID)
		} else if len(mgrLinks) == 0 {
			log.Info("System reported no ManagedBy links", "systemID", system.ID)
		} else {
			for _, link := range mgrLinks {
				mgr, err := redfish.GetManager(c.gofishClient, link.ODataID)
				if err != nil {
					log.Error(err, "Failed to get manager from ManagedBy link", "link", link.ODataID)
					continue
				}
				vm, _ := searchManagerVM(mgr) // Error already logged in helper
				if vm != nil {
					return vm, nil // Found it!
				}
			}
		}
	}

	// Attempt 2: Via ServiceRoot.Managers
	log.Info("Falling back to searching all managers from service root for virtual media")
	managers, err := c.gofishClient.Service.Managers()
	if err != nil {
		log.Error(err, "Failed to retrieve managers from service root")
		return nil, fmt.Errorf("failed to get managers from service root: %w", err)
	}
	if len(managers) == 0 {
		log.Error(nil, "No managers found at service root")
		return nil, fmt.Errorf("no managers found at service root")
	}

	for _, mgr := range managers {
		vm, _ := searchManagerVM(mgr) // Error already logged in helper
		if vm != nil {
			return vm, nil // Found it!
		}
	}

	log.Error(nil, "No suitable virtual media device (CD/DVD) found after checking all managers.")
	return nil, fmt.Errorf("no suitable virtual media (CD/DVD) found")
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
// This implementation uses vendor-specific methods when possible, with fallback
// to the standard UEFI method.
func (c *gofishClient) SetBootParameters(ctx context.Context, params []string) error {
	return c.SetBootParametersWithAnnotations(ctx, params, nil)
}

// SetBootParametersWithAnnotations configures kernel command line parameters for the next boot
// with vendor-specific support based on annotations.
func (c *gofishClient) SetBootParametersWithAnnotations(ctx context.Context, params []string, annotations map[string]string) error {
	log := logf.FromContext(ctx)
	log.Info("Attempting to set boot parameters with vendor-specific support", "Params", params)

	// Create vendor-specific boot manager
	vendorBootMgr := NewVendorSpecificBootManager(c)

	// Try vendor-specific boot parameter setting first
	// This will automatically detect the vendor and use the appropriate method
	err := vendorBootMgr.SetBootParametersWithVendorSupport(ctx, params, annotations)
	if err != nil {
		log.Error(err, "Vendor-specific boot parameter setting failed, trying fallback")

		// Fallback to the original UEFI method
		return c.setBootParametersUEFI(ctx, params)
	}

	log.Info("Successfully set boot parameters using vendor-specific method")
	return nil
}

// setBootParametersUEFI is the original UEFI boot parameter implementation
func (c *gofishClient) setBootParametersUEFI(ctx context.Context, params []string) error {
	log := logf.FromContext(ctx)
	log.V(1).Info("Attempting boot parameter setting via UefiTargetBootSourceOverride")

	system, err := c.getSystemService(ctx)
	if err != nil {
		return fmt.Errorf("failed to get system to set boot parameters: %w", err)
	}

	var uefiBootSettings redfish.Boot
	if len(params) == 0 {
		// Clear parameters: Disable override and set target to None.
		uefiBootSettings = redfish.Boot{
			BootSourceOverrideEnabled:    redfish.DisabledBootSourceOverrideEnabled,
			BootSourceOverrideTarget:     redfish.NoneBootSourceOverrideTarget,
			UefiTargetBootSourceOverride: "",
		}
	} else {
		// Default EFI bootloader path. This is a guess and might need to be configurable.
		efiBootloaderPath := "\\EFI\\BOOT\\BOOTX64.EFI"
		fullBootString := efiBootloaderPath + " " + strings.Join(params, " ")
		uefiBootSettings = redfish.Boot{
			BootSourceOverrideTarget:     redfish.UefiTargetBootSourceOverrideTarget,
			BootSourceOverrideEnabled:    redfish.OnceBootSourceOverrideEnabled,
			UefiTargetBootSourceOverride: fullBootString,
		}
	}

	log.Info("Applying boot settings via UefiTargetBootSourceOverride", "Settings", uefiBootSettings)
	uerr := system.SetBoot(uefiBootSettings)
	if uerr == nil {
		log.Info("Successfully applied boot settings via UefiTargetBootSourceOverride.")
		return nil // Success!
	}

	// UEFI Target method failed, log details and consider alternatives.
	log.Error(uerr, "Failed to set boot settings via UefiTargetBootSourceOverride", "Settings", uefiBootSettings)
	return fmt.Errorf("failed to set boot parameters using UefiTargetBootSourceOverride: %w", uerr)
}

// setBootParametersViaBootOptions attempts to set boot parameters using UEFI BootOptions/BootNext,
// by locating a suitable BootOption (e.g., the virtual CD/DVD) and setting BootNext for a one-time boot.
// Note: Kernel parameters are generally not supported via BootOptions; this is a fallback to select
// a proper boot source when UefiTargetBootSourceOverride is not reliable.
func (c *gofishClient) setBootParametersViaBootOptions(ctx context.Context) error {
	log := logf.FromContext(ctx)
	log.V(1).Info("Attempting to set boot via BootOptions/BootNext")

	system, err := c.getSystemService(ctx)
	if err != nil {
		return fmt.Errorf("failed to get system for boot options: %w", err)
	}

	// Fetch available BootOptions
	bootOptions, err := system.BootOptions()
	if err != nil {
		log.Error(err, "Failed to list BootOptions")
		return fmt.Errorf("failed to list boot options: %w", err)
	}
	if len(bootOptions) == 0 {
		return fmt.Errorf("no BootOptions available on system")
	}

	// Heuristic: prefer a CD/DVD or virtual media BootOption; fallback to first option.
	var ref string
	for _, bo := range bootOptions {
		if bo == nil {
			continue
		}
		// Many implementations encode media type in reference or description; be conservative
		if strings.Contains(strings.ToLower(bo.BootOptionReference), "cd") ||
			strings.Contains(strings.ToLower(bo.BootOptionReference), "dvd") ||
			strings.Contains(strings.ToLower(bo.BootOptionReference), "virtual") {
			ref = bo.BootOptionReference
			break
		}
	}
	if ref == "" {
		// Fallback to first option
		ref = bootOptions[0].BootOptionReference
	}

	// Set one-time boot using BootNext
	current := system.Boot
	current.BootSourceOverrideTarget = redfish.UefiBootNextBootSourceOverrideTarget
	current.BootSourceOverrideEnabled = redfish.OnceBootSourceOverrideEnabled
	current.BootNext = ref

	log.Info("Applying boot via BootNext", "BootNext", ref)
	if err := system.SetBoot(current); err != nil {
		log.Error(err, "Failed to set BootNext via SetBoot")
		return fmt.Errorf("failed to set BootNext via SetBoot: %w", err)
	}

	log.Info("Successfully set one-time boot via BootNext", "BootNext", ref)
	return nil
}

// GetNetworkAddresses retrieves network interface addresses from the system.
func (c *gofishClient) GetNetworkAddresses(ctx context.Context) ([]NetworkAddress, error) {
	log := logf.FromContext(ctx)
	log.Info("Attempting to retrieve network addresses from Redfish")

	system, err := c.getSystemService(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get system for network address discovery: %w", err)
	}

	var addresses []NetworkAddress

	// Try to get EthernetInterfaces first (more common and reliable)
	ethernetInterfaces, err := system.EthernetInterfaces()
	if err != nil {
		log.Error(err, "Failed to retrieve ethernet interfaces")
		// Don't return error yet, try NetworkInterfaces as fallback
	} else {
		log.Info("Found ethernet interfaces", "count", len(ethernetInterfaces))
		for _, ethIntf := range ethernetInterfaces {
			interfaceAddresses := c.extractAddressesFromEthernetInterface(ctx, ethIntf)
			addresses = append(addresses, interfaceAddresses...)
		}
	}

	// If we didn't get addresses from EthernetInterfaces, try NetworkInterfaces
	if len(addresses) == 0 {
		log.Info("No addresses found via EthernetInterfaces, trying NetworkInterfaces fallback")
		networkInterfaces, err := system.NetworkInterfaces()
		if err != nil {
			log.Error(err, "Failed to retrieve network interfaces")
			return nil, fmt.Errorf("failed to retrieve both ethernet and network interfaces: %w", err)
		}
		log.Info("Found network interfaces", "count", len(networkInterfaces))
		for _, netIntf := range networkInterfaces {
			interfaceAddresses := c.extractAddressesFromNetworkInterface(ctx, netIntf)
			addresses = append(addresses, interfaceAddresses...)
		}
	}

	log.Info("Successfully retrieved network addresses", "totalAddresses", len(addresses))
	return addresses, nil
}

// extractAddressesFromEthernetInterface extracts network addresses from an EthernetInterface.
func (c *gofishClient) extractAddressesFromEthernetInterface(ctx context.Context, ethIntf *redfish.EthernetInterface) []NetworkAddress {
	log := logf.FromContext(ctx)
	var addresses []NetworkAddress

	// Extract IPv4 addresses
	for _, ipv4 := range ethIntf.IPv4Addresses {
		if ipv4.Address != "" {
			address := NetworkAddress{
				Type:          IPv4AddressType,
				Address:       ipv4.Address,
				Gateway:       ipv4.Gateway,
				InterfaceName: ethIntf.Name,
				MACAddress:    ethIntf.MACAddress,
			}
			addresses = append(addresses, address)
			log.V(1).Info("Found IPv4 address", "interface", ethIntf.Name, "address", ipv4.Address, "gateway", ipv4.Gateway)
		}
	}

	// Extract IPv6 addresses
	for _, ipv6 := range ethIntf.IPv6Addresses {
		if ipv6.Address != "" {
			address := NetworkAddress{
				Type:          IPv6AddressType,
				Address:       ipv6.Address,
				Gateway:       ethIntf.IPv6DefaultGateway,
				InterfaceName: ethIntf.Name,
				MACAddress:    ethIntf.MACAddress,
			}
			addresses = append(addresses, address)
			log.V(1).Info("Found IPv6 address", "interface", ethIntf.Name, "address", ipv6.Address, "gateway", ethIntf.IPv6DefaultGateway)
		}
	}

	return addresses
}

// extractAddressesFromNetworkInterface extracts network addresses from a NetworkInterface.
func (c *gofishClient) extractAddressesFromNetworkInterface(ctx context.Context, netIntf *redfish.NetworkInterface) []NetworkAddress {
	log := logf.FromContext(ctx)
	var addresses []NetworkAddress

	// NetworkInterface doesn't directly contain IP addresses like EthernetInterface
	// We need to check if it has associated ports or device functions that might contain address info
	// For now, just log that we found the interface but can't extract addresses
	log.V(1).Info("Found NetworkInterface but cannot extract addresses directly", "interface", netIntf.Name)

	// TODO: If needed, implement logic to traverse NetworkPorts or NetworkDeviceFunctions
	// associated with this NetworkInterface to find IP address information

	return addresses
}
