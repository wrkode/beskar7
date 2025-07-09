package redfish

import (
	"context"
	"fmt"
	"strings"

	"github.com/stmcginnis/gofish/redfish"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// VendorType represents different hardware vendors
type VendorType string

const (
	VendorUnknown    VendorType = "unknown"
	VendorDell       VendorType = "dell"
	VendorHPE        VendorType = "hpe"
	VendorLenovo     VendorType = "lenovo"
	VendorSupermicro VendorType = "supermicro"
	VendorGeneric    VendorType = "generic"
)

// VendorConfig contains vendor-specific configuration
type VendorConfig struct {
	Type                   VendorType
	SupportsUEFIBootParams bool
	BIOSKernelArgAttribute string
	BootParameterMechanism BootParameterMechanism
	RequiresBIOSAttributes bool
}

// BootParameterMechanism defines how to set boot parameters
type BootParameterMechanism string

const (
	MechanismUEFITarget    BootParameterMechanism = "uefi_target"
	MechanismBIOSAttribute BootParameterMechanism = "bios_attribute"
	MechanismBootOptions   BootParameterMechanism = "boot_options"
	MechanismUnsupported   BootParameterMechanism = "unsupported"
)

// VendorDetector handles vendor detection and configuration
type VendorDetector struct {
	vendorConfigs map[VendorType]VendorConfig
}

// NewVendorDetector creates a new vendor detector with default configurations
func NewVendorDetector() *VendorDetector {
	return &VendorDetector{
		vendorConfigs: map[VendorType]VendorConfig{
			VendorDell: {
				Type:                   VendorDell,
				SupportsUEFIBootParams: false, // Dell prefers BIOS attributes
				BIOSKernelArgAttribute: "KernelArgs",
				BootParameterMechanism: MechanismBIOSAttribute,
				RequiresBIOSAttributes: true,
			},
			VendorHPE: {
				Type:                   VendorHPE,
				SupportsUEFIBootParams: true, // HPE iLO has good UEFI support
				BIOSKernelArgAttribute: "",
				BootParameterMechanism: MechanismUEFITarget,
				RequiresBIOSAttributes: false,
			},
			VendorLenovo: {
				Type:                   VendorLenovo,
				SupportsUEFIBootParams: true, // Lenovo XCC supports UEFI
				BIOSKernelArgAttribute: "",
				BootParameterMechanism: MechanismUEFITarget,
				RequiresBIOSAttributes: false,
			},
			VendorSupermicro: {
				Type:                   VendorSupermicro,
				SupportsUEFIBootParams: false, // Variable support, conservative approach
				BIOSKernelArgAttribute: "BootArgs",
				BootParameterMechanism: MechanismBIOSAttribute,
				RequiresBIOSAttributes: true,
			},
			VendorGeneric: {
				Type:                   VendorGeneric,
				SupportsUEFIBootParams: true, // Try UEFI first for generic
				BIOSKernelArgAttribute: "",
				BootParameterMechanism: MechanismUEFITarget,
				RequiresBIOSAttributes: false,
			},
		},
	}
}

// DetectVendor detects the vendor from system information
func (vd *VendorDetector) DetectVendor(sysInfo *SystemInfo) VendorType {
	if sysInfo == nil || sysInfo.Manufacturer == "" {
		return VendorUnknown
	}

	manufacturer := strings.ToLower(strings.TrimSpace(sysInfo.Manufacturer))

	// Dell detection
	if strings.Contains(manufacturer, "dell") {
		return VendorDell
	}

	// HPE detection
	if strings.Contains(manufacturer, "hpe") || strings.Contains(manufacturer, "hewlett") ||
		strings.Contains(manufacturer, "hp enterprise") {
		return VendorHPE
	}

	// Lenovo detection
	if strings.Contains(manufacturer, "lenovo") {
		return VendorLenovo
	}

	// Supermicro detection
	if strings.Contains(manufacturer, "supermicro") || strings.Contains(manufacturer, "super micro") {
		return VendorSupermicro
	}

	// If no specific vendor detected, use generic
	return VendorGeneric
}

// GetVendorConfig returns the configuration for a specific vendor
func (vd *VendorDetector) GetVendorConfig(vendor VendorType) VendorConfig {
	if config, exists := vd.vendorConfigs[vendor]; exists {
		return config
	}
	return vd.vendorConfigs[VendorGeneric]
}

// OverrideVendorConfig allows overriding vendor config (for annotations)
func (vd *VendorDetector) OverrideVendorConfig(vendor VendorType, overrides VendorConfig) {
	vd.vendorConfigs[vendor] = overrides
}

// BIOSAttributeManager handles BIOS attribute operations
type BIOSAttributeManager interface {
	// GetBIOSAttributes retrieves current BIOS attributes
	GetBIOSAttributes(ctx context.Context, attributeNames []string) (map[string]interface{}, error)

	// SetBIOSAttribute sets a single BIOS attribute
	SetBIOSAttribute(ctx context.Context, attributeName string, value interface{}) error

	// SetBIOSAttributes sets multiple BIOS attributes
	SetBIOSAttributes(ctx context.Context, attributes map[string]interface{}) error

	// ScheduleBIOSSettingsApply schedules the BIOS settings to be applied on next boot
	ScheduleBIOSSettingsApply(ctx context.Context) error
}

// VendorSpecificBootManager handles vendor-specific boot parameter setting
type VendorSpecificBootManager struct {
	client   *gofishClient
	detector *VendorDetector
	biosMgr  BIOSAttributeManager
}

// NewVendorSpecificBootManager creates a new vendor-specific boot manager
func NewVendorSpecificBootManager(client *gofishClient) *VendorSpecificBootManager {
	detector := NewVendorDetector()
	return &VendorSpecificBootManager{
		client:   client,
		detector: detector,
		biosMgr:  NewBIOSAttributeManager(client),
	}
}

// SetBootParametersWithVendorSupport sets boot parameters using vendor-specific methods
func (vbm *VendorSpecificBootManager) SetBootParametersWithVendorSupport(ctx context.Context, params []string, annotations map[string]string) error {
	// Get system info to detect vendor
	sysInfo, err := vbm.client.GetSystemInfo(ctx)
	if err != nil {
		return fmt.Errorf("failed to get system info for vendor detection: %w", err)
	}

	// Detect vendor
	vendor := vbm.detector.DetectVendor(sysInfo)
	config := vbm.detector.GetVendorConfig(vendor)

	// Check for annotation overrides
	if annotations != nil {
		config = vbm.processAnnotationOverrides(config, annotations)
	}

	log := logf.FromContext(ctx)
	log.Info("Setting boot parameters with vendor-specific support",
		"vendor", vendor,
		"mechanism", config.BootParameterMechanism,
		"params", params)

	// Try vendor-specific mechanism first
	switch config.BootParameterMechanism {
	case MechanismUEFITarget:
		return vbm.setBootParametersUEFI(ctx, params)
	case MechanismBIOSAttribute:
		return vbm.setBootParametersBIOS(ctx, params, config.BIOSKernelArgAttribute)
	case MechanismBootOptions:
		return vbm.setBootParametersBootOptions(ctx, params)
	case MechanismUnsupported:
		return fmt.Errorf("boot parameter setting is not supported for vendor %s", vendor)
	default:
		// Fallback to UEFI if unknown mechanism
		log.Info("Unknown boot parameter mechanism, falling back to UEFI", "mechanism", config.BootParameterMechanism)
		return vbm.setBootParametersUEFI(ctx, params)
	}
}

// processAnnotationOverrides processes annotation-based configuration overrides
func (vbm *VendorSpecificBootManager) processAnnotationOverrides(config VendorConfig, annotations map[string]string) VendorConfig {
	const annotationPrefix = "beskar7.infrastructure.cluster.x-k8s.io/"

	// Override BIOS kernel arg attribute
	if attr, exists := annotations[annotationPrefix+"bios-kernel-arg-attribute"]; exists && attr != "" {
		config.BIOSKernelArgAttribute = attr
		config.BootParameterMechanism = MechanismBIOSAttribute
		config.RequiresBIOSAttributes = true
	}

	// Override boot parameter mechanism
	if mechanism, exists := annotations[annotationPrefix+"boot-parameter-mechanism"]; exists {
		switch mechanism {
		case "uefi-target":
			config.BootParameterMechanism = MechanismUEFITarget
		case "bios-attribute":
			config.BootParameterMechanism = MechanismBIOSAttribute
		case "boot-options":
			config.BootParameterMechanism = MechanismBootOptions
		case "unsupported":
			config.BootParameterMechanism = MechanismUnsupported
		}
	}

	return config
}

// setBootParametersUEFI sets boot parameters using UEFI target method
func (vbm *VendorSpecificBootManager) setBootParametersUEFI(ctx context.Context, params []string) error {
	// This is the existing implementation from SetBootParameters
	log := logf.FromContext(ctx)
	log.V(1).Info("Setting boot parameters via UefiTargetBootSourceOverride")

	system, err := vbm.client.getSystemService(ctx)
	if err != nil {
		return fmt.Errorf("failed to get system to set boot parameters: %w", err)
	}

	var uefiBootSettings redfish.Boot
	if len(params) == 0 {
		uefiBootSettings = redfish.Boot{
			BootSourceOverrideEnabled:    redfish.DisabledBootSourceOverrideEnabled,
			BootSourceOverrideTarget:     redfish.NoneBootSourceOverrideTarget,
			UefiTargetBootSourceOverride: "",
		}
	} else {
		efiBootloaderPath := "\\EFI\\BOOT\\BOOTX64.EFI"
		fullBootString := efiBootloaderPath + " " + strings.Join(params, " ")
		uefiBootSettings = redfish.Boot{
			BootSourceOverrideTarget:     redfish.UefiTargetBootSourceOverrideTarget,
			BootSourceOverrideEnabled:    redfish.OnceBootSourceOverrideEnabled,
			UefiTargetBootSourceOverride: fullBootString,
		}
	}

	log.Info("Applying boot settings via UefiTargetBootSourceOverride", "Settings", uefiBootSettings)
	err = system.SetBoot(uefiBootSettings)
	if err != nil {
		log.Error(err, "Failed to set boot settings via UefiTargetBootSourceOverride", "Settings", uefiBootSettings)
		return fmt.Errorf("failed to set boot parameters using UefiTargetBootSourceOverride: %w", err)
	}

	log.Info("Successfully applied boot settings via UefiTargetBootSourceOverride")
	return nil
}

// setBootParametersBIOS sets boot parameters using BIOS attributes
func (vbm *VendorSpecificBootManager) setBootParametersBIOS(ctx context.Context, params []string, attributeName string) error {
	log := logf.FromContext(ctx)
	log.Info("Setting boot parameters via BIOS attribute", "attribute", attributeName, "params", params)

	if attributeName == "" {
		return fmt.Errorf("BIOS kernel arg attribute name not specified")
	}

	var attributeValue string
	if len(params) == 0 {
		// Clear the attribute
		attributeValue = ""
	} else {
		// Join parameters with spaces
		attributeValue = strings.Join(params, " ")
	}

	// Set the BIOS attribute
	err := vbm.biosMgr.SetBIOSAttribute(ctx, attributeName, attributeValue)
	if err != nil {
		log.Error(err, "Failed to set BIOS attribute", "attribute", attributeName, "value", attributeValue)
		return fmt.Errorf("failed to set BIOS attribute %s: %w", attributeName, err)
	}

	// Schedule BIOS settings to be applied
	err = vbm.biosMgr.ScheduleBIOSSettingsApply(ctx)
	if err != nil {
		log.Error(err, "Failed to schedule BIOS settings apply")
		return fmt.Errorf("failed to schedule BIOS settings apply: %w", err)
	}

	log.Info("Successfully set boot parameters via BIOS attribute", "attribute", attributeName, "value", attributeValue)
	return nil
}

// setBootParametersBootOptions sets boot parameters using boot options (future implementation)
func (vbm *VendorSpecificBootManager) setBootParametersBootOptions(ctx context.Context, params []string) error {
	log := logf.FromContext(ctx)
	log.Info("Boot parameter setting via boot options not yet implemented", "params", params)

	// TODO: Implement boot options mechanism
	// This could be used for vendors that support modifying boot entries directly
	return fmt.Errorf("boot parameter setting via boot options is not yet implemented")
}
