package redfish

import (
	"testing"

	"github.com/stmcginnis/gofish/common"
)

func TestVendorDetector_DetectVendor(t *testing.T) {
	detector := NewVendorDetector()

	tests := []struct {
		name         string
		sysInfo      *SystemInfo
		expectedType VendorType
	}{
		{
			name: "Dell system",
			sysInfo: &SystemInfo{
				Manufacturer: "Dell Inc.",
				Model:        "PowerEdge R740",
				SerialNumber: "1234567",
				Status:       common.Status{State: common.EnabledState},
			},
			expectedType: VendorDell,
		},
		{
			name: "HPE system",
			sysInfo: &SystemInfo{
				Manufacturer: "HPE",
				Model:        "ProLiant DL380 Gen10",
				SerialNumber: "1234567",
				Status:       common.Status{State: common.EnabledState},
			},
			expectedType: VendorHPE,
		},
		{
			name: "Lenovo system",
			sysInfo: &SystemInfo{
				Manufacturer: "Lenovo",
				Model:        "ThinkSystem SR650",
				SerialNumber: "1234567",
				Status:       common.Status{State: common.EnabledState},
			},
			expectedType: VendorLenovo,
		},
		{
			name: "Supermicro system",
			sysInfo: &SystemInfo{
				Manufacturer: "Supermicro",
				Model:        "SYS-6028R-TRT",
				SerialNumber: "1234567",
				Status:       common.Status{State: common.EnabledState},
			},
			expectedType: VendorSupermicro,
		},
		{
			name: "Generic system",
			sysInfo: &SystemInfo{
				Manufacturer: "ACME Corp",
				Model:        "Server 2000",
				SerialNumber: "1234567",
				Status:       common.Status{State: common.EnabledState},
			},
			expectedType: VendorGeneric,
		},
		{
			name:         "Nil system info",
			sysInfo:      nil,
			expectedType: VendorUnknown,
		},
		{
			name: "Empty manufacturer",
			sysInfo: &SystemInfo{
				Manufacturer: "",
				Model:        "Server",
				SerialNumber: "1234567",
				Status:       common.Status{State: common.EnabledState},
			},
			expectedType: VendorUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.DetectVendor(tt.sysInfo)
			if result != tt.expectedType {
				t.Errorf("DetectVendor() = %v, want %v", result, tt.expectedType)
			}
		})
	}
}

func TestVendorDetector_GetVendorConfig(t *testing.T) {
	detector := NewVendorDetector()

	// Test Dell config
	dellConfig := detector.GetVendorConfig(VendorDell)
	if dellConfig.Type != VendorDell {
		t.Errorf("Dell config type = %v, want %v", dellConfig.Type, VendorDell)
	}
	if dellConfig.BootParameterMechanism != MechanismBIOSAttribute {
		t.Errorf("Dell boot mechanism = %v, want %v", dellConfig.BootParameterMechanism, MechanismBIOSAttribute)
	}
	if dellConfig.BIOSKernelArgAttribute != "KernelArgs" {
		t.Errorf("Dell BIOS attribute = %v, want %v", dellConfig.BIOSKernelArgAttribute, "KernelArgs")
	}

	// Test HPE config
	hpeConfig := detector.GetVendorConfig(VendorHPE)
	if hpeConfig.Type != VendorHPE {
		t.Errorf("HPE config type = %v, want %v", hpeConfig.Type, VendorHPE)
	}
	if hpeConfig.BootParameterMechanism != MechanismUEFITarget {
		t.Errorf("HPE boot mechanism = %v, want %v", hpeConfig.BootParameterMechanism, MechanismUEFITarget)
	}
	if hpeConfig.SupportsUEFIBootParams != true {
		t.Errorf("HPE UEFI support = %v, want %v", hpeConfig.SupportsUEFIBootParams, true)
	}

	// Test unknown vendor returns generic
	unknownConfig := detector.GetVendorConfig(VendorUnknown)
	if unknownConfig.Type != VendorGeneric {
		t.Errorf("Unknown vendor config type = %v, want %v", unknownConfig.Type, VendorGeneric)
	}
}

func TestAnnotationOverrides(t *testing.T) {
	client := &gofishClient{}
	bootMgr := NewVendorSpecificBootManager(client)

	baseConfig := VendorConfig{
		Type:                   VendorGeneric,
		BootParameterMechanism: MechanismUEFITarget,
	}

	annotations := map[string]string{
		"beskar7.infrastructure.cluster.x-k8s.io/bios-kernel-arg-attribute": "CustomKernelArgs",
	}

	result := bootMgr.processAnnotationOverrides(baseConfig, annotations)

	if result.BIOSKernelArgAttribute != "CustomKernelArgs" {
		t.Errorf("BIOS kernel arg attribute = %v, want %v", result.BIOSKernelArgAttribute, "CustomKernelArgs")
	}
	if result.BootParameterMechanism != MechanismBIOSAttribute {
		t.Errorf("Boot parameter mechanism = %v, want %v", result.BootParameterMechanism, MechanismBIOSAttribute)
	}
	if result.RequiresBIOSAttributes != true {
		t.Errorf("Requires BIOS attributes = %v, want %v", result.RequiresBIOSAttributes, true)
	}
}

func TestVendorSpecificBootManager_Creation(t *testing.T) {
	client := &gofishClient{}
	bootMgr := NewVendorSpecificBootManager(client)

	if bootMgr == nil {
		t.Error("NewVendorSpecificBootManager() returned nil")
	}

	if bootMgr.client != client {
		t.Error("VendorSpecificBootManager client not set correctly")
	}

	if bootMgr.detector == nil {
		t.Error("VendorSpecificBootManager detector not initialized")
	}

	if bootMgr.biosMgr == nil {
		t.Error("VendorSpecificBootManager biosMgr not initialized")
	}
}
