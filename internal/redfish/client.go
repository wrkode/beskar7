package redfish

import (
	"context"

	"github.com/stmcginnis/gofish/common"
	"github.com/stmcginnis/gofish/redfish"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
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

	// GetNetworkAddresses retrieves network interface addresses from the system.
	GetNetworkAddresses(ctx context.Context) ([]NetworkAddress, error)

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

// NetworkAddress represents a network address discovered from Redfish.
type NetworkAddress struct {
	// Type indicates the address type (IPv4 or IPv6)
	Type NetworkAddressType
	// Address is the IP address string
	Address string
	// Gateway is the gateway address if available
	Gateway string
	// InterfaceName is the name of the network interface
	InterfaceName string
	// MACAddress is the MAC address of the interface
	MACAddress string
}

// NetworkAddressType represents the type of network address.
type NetworkAddressType string

const (
	// IPv4AddressType represents an IPv4 address
	IPv4AddressType NetworkAddressType = "IPv4"
	// IPv6AddressType represents an IPv6 address
	IPv6AddressType NetworkAddressType = "IPv6"
)

// ConvertToMachineAddresses converts NetworkAddress slices to Cluster API MachineAddress format.
func ConvertToMachineAddresses(networkAddresses []NetworkAddress) []clusterv1.MachineAddress {
	var machineAddresses []clusterv1.MachineAddress

	for _, netAddr := range networkAddresses {
		// Determine the machine address type based on the network address
		var addrType clusterv1.MachineAddressType
		if netAddr.Type == IPv4AddressType {
			// Classify IPv4 addresses as Internal or External based on RFC 1918 private ranges
			if isPrivateIPv4(netAddr.Address) {
				addrType = clusterv1.MachineInternalIP
			} else {
				addrType = clusterv1.MachineExternalIP
			}
		} else if netAddr.Type == IPv6AddressType {
			// Classify IPv6 addresses as Internal or External based on RFC 4193 ULA and link-local
			if isPrivateIPv6(netAddr.Address) {
				addrType = clusterv1.MachineInternalIP
			} else {
				addrType = clusterv1.MachineExternalIP
			}
		} else {
			// Default to InternalIP for unknown types
			addrType = clusterv1.MachineInternalIP
		}

		machineAddr := clusterv1.MachineAddress{
			Type:    addrType,
			Address: netAddr.Address,
		}
		machineAddresses = append(machineAddresses, machineAddr)
	}

	return machineAddresses
}

// isPrivateIPv4 checks if an IPv4 address is in a private range (RFC 1918).
func isPrivateIPv4(ip string) bool {
	// Common private IPv4 ranges:
	// 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16
	// Also consider loopback (127.0.0.0/8) and link-local (169.254.0.0/16) as internal
	if len(ip) >= 3 && ip[:3] == "10." {
		return true
	}
	if len(ip) >= 8 && ip[:8] == "192.168." {
		return true
	}
	if len(ip) >= 4 && ip[:4] == "127." {
		return true
	}
	if len(ip) >= 7 && ip[:7] == "169.254" {
		return true
	}
	// Handle 172.16.0.0/12 range (172.16.x.x to 172.31.x.x)
	if len(ip) >= 7 && ip[:4] == "172." {
		// Extract the second octet to check if it's in range 16-31
		if len(ip) > 7 {
			secondOctet := ""
			for i := 4; i < len(ip); i++ {
				if ip[i] == '.' {
					break
				}
				secondOctet += string(ip[i])
			}
			// Simple range check for second octet (16-31)
			if secondOctet == "16" || secondOctet == "17" || secondOctet == "18" || secondOctet == "19" ||
				secondOctet == "20" || secondOctet == "21" || secondOctet == "22" || secondOctet == "23" ||
				secondOctet == "24" || secondOctet == "25" || secondOctet == "26" || secondOctet == "27" ||
				secondOctet == "28" || secondOctet == "29" || secondOctet == "30" || secondOctet == "31" {
				return true
			}
		}
	}
	return false
}

// isPrivateIPv6 checks if an IPv6 address is in a private range.
func isPrivateIPv6(ip string) bool {
	// Common private IPv6 ranges:
	// fc00::/7 (Unique Local Addresses), fe80::/10 (Link-local), ::1/128 (loopback)
	if len(ip) >= 2 {
		prefix := ip[:2]
		// ULA starts with fc or fd
		if prefix == "fc" || prefix == "fd" {
			return true
		}
		// Link-local starts with fe8, fe9, fea, feb
		if len(ip) >= 3 && (ip[:3] == "fe8" || ip[:3] == "fe9" || ip[:3] == "fea" || ip[:3] == "feb") {
			return true
		}
	}
	// Loopback
	if ip == "::1" {
		return true
	}
	// For other IPv6 addresses, classify as external if they appear to be global unicast
	// Global unicast typically starts with 2 or 3
	if len(ip) >= 1 && (ip[0] == '2' || ip[0] == '3') {
		return false // External/public
	}
	// Default to internal for safety/unknown ranges
	return true
}
