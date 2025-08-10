package redfish

import (
	"context"
	"fmt"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// gofishBIOSAttributeManager implements BIOSAttributeManager using gofish
type gofishBIOSAttributeManager struct {
	client *gofishClient
}

// NewBIOSAttributeManager creates a new BIOS attribute manager
func NewBIOSAttributeManager(client *gofishClient) BIOSAttributeManager {
	return &gofishBIOSAttributeManager{
		client: client,
	}
}

// GetBIOSAttributes retrieves current BIOS attributes
func (bam *gofishBIOSAttributeManager) GetBIOSAttributes(ctx context.Context, attributeNames []string) (map[string]interface{}, error) {
	log := logf.FromContext(ctx)
	log.Info("Getting BIOS attributes", "attributes", attributeNames)

	system, err := bam.client.getSystemService(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get system for BIOS attributes: %w", err)
	}

	bios, err := system.Bios()
	if err != nil {
		return nil, fmt.Errorf("failed to get BIOS resource: %w", err)
	}

	// Get current attributes
	attributes := make(map[string]interface{})

	// If no specific attributes requested, get all
	if len(attributeNames) == 0 {
		for name, value := range bios.Attributes {
			attributes[name] = value
		}
	} else {
		// Get only requested attributes
		for _, name := range attributeNames {
			if value, exists := bios.Attributes[name]; exists {
				attributes[name] = value
			} else {
				log.V(1).Info("BIOS attribute not found", "attribute", name)
			}
		}
	}

	log.Info("Retrieved BIOS attributes", "count", len(attributes))
	return attributes, nil
}

// SetBIOSAttribute sets a single BIOS attribute
func (bam *gofishBIOSAttributeManager) SetBIOSAttribute(ctx context.Context, attributeName string, value interface{}) error {
	attributes := map[string]interface{}{
		attributeName: value,
	}
	return bam.SetBIOSAttributes(ctx, attributes)
}

// SetBIOSAttributes sets multiple BIOS attributes
func (bam *gofishBIOSAttributeManager) SetBIOSAttributes(ctx context.Context, attributes map[string]interface{}) error {
	log := logf.FromContext(ctx)
	log.Info("Setting BIOS attributes", "attributes", attributes)

	system, err := bam.client.getSystemService(ctx)
	if err != nil {
		return fmt.Errorf("failed to get system for BIOS attributes: %w", err)
	}

	bios, err := system.Bios()
	if err != nil {
		return fmt.Errorf("failed to get BIOS resource: %w", err)
	}

	// Convert our map to the format expected by gofish
	settingsAttrs := make(map[string]interface{})
	for k, v := range attributes {
		settingsAttrs[k] = v
	}

	// Use gofish's built-in BIOS attribute update method
	err = bios.UpdateBiosAttributes(settingsAttrs)
	if err != nil {
		log.Error(err, "Failed to update BIOS attributes", "attributes", attributes)
		return fmt.Errorf("failed to update BIOS attributes: %w", err)
	}

	log.Info("Successfully set BIOS attributes", "attributes", attributes)
	return nil
}

// ScheduleBIOSSettingsApply schedules the BIOS settings to be applied on next boot
func (bam *gofishBIOSAttributeManager) ScheduleBIOSSettingsApply(ctx context.Context) error {
	log := logf.FromContext(ctx)
	log.Info("Scheduling BIOS settings to be applied on next boot")

	// Detect vendor to determine if explicit job creation is needed
	sysInfo, err := bam.client.GetSystemInfo(ctx)
	if err != nil {
		// Non-fatal: log and assume default behavior
		log.V(1).Info("Failed to get system info for BIOS scheduling; assuming default behavior", "error", err)
		return nil
	}

	vendor := NewVendorDetector().DetectVendor(sysInfo)
	switch vendor {
	case VendorDell:
		// Dell iDRAC supports ApplyTime via the Redfish settings object. Use OnReset apply time
		// so changes apply on the next reset/power cycle.
		system, err := bam.client.getSystemService(ctx)
		if err != nil {
			return fmt.Errorf("failed to get system for BIOS job scheduling: %w", err)
		}
		bios, err := system.Bios()
		if err != nil {
			return fmt.Errorf("failed to get BIOS resource for job scheduling: %w", err)
		}
		// No attributes to update at this point; we rely on SetBIOSAttributes having been called just before.
		// To be explicit, we can call UpdateBiosAttributesApplyAt with an empty map and OnReset apply time which
		// will be a no-op for attributes but still set apply timing where supported.
		if err := bios.UpdateBiosAttributesApplyAt(map[string]interface{}{}, "OnReset"); err != nil {
			log.V(1).Info("ApplyTime setting not supported or failed; relying on implicit job", "error", err)
		} else {
			log.Info("Requested BIOS settings apply on next reset for Dell iDRAC")
		}
	default:
		// Most vendors automatically schedule settings on next boot.
		log.V(1).Info("Vendor does not require explicit BIOS job scheduling", "vendor", vendor)
	}

	log.Info("BIOS settings scheduled (vendor-specific handling applied where needed)")
	return nil
}
