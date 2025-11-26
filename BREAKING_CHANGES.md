# Breaking Changes in Beskar7

## Version 1.0 - Major Architecture Simplification

**Date:** November 2025

### Overview

Beskar7 has undergone a fundamental architectural simplification, moving from a VirtualMedia-based provisioning approach to an iPXE + inspection workflow. This eliminates vendor-specific complexity and provides more reliable bare-metal provisioning.

---

## Architecture Changes

### Before (v0.x)
```
Beskar7 → Redfish (VirtualMedia + Boot Params + Vendor Quirks) → ISO Boot → OS
```

### After (v1.0)
```
Beskar7 → Redfish (Power + PXE flag) → iPXE → Inspection (Alpine) → Report → Kexec → OS
```

---

## Removed Features

### 1. VirtualMedia Support
**Status:** REMOVED

**Rationale:** VirtualMedia implementations vary significantly across vendors, leading to:
- Inconsistent reliability
- Complex vendor-specific workarounds
- Difficult troubleshooting
- Poor scalability

**Migration Path:** Use iPXE + inspection workflow instead.

### 2. Vendor-Specific Workarounds
**Status:** REMOVED

The following vendor-specific code has been removed:
- BIOS attribute manipulation (Dell `KernelArgs`)
- Boot parameter injection via `UefiTargetBootSourceOverride`
- Vendor detection and configuration (`internal/redfish/vendor.go`)
- BIOS management (`internal/redfish/bios_manager.go`)

**Rationale:** With power-only Redfish usage, vendor quirks are no longer relevant.

### 3. Provisioning Modes
**Status:** REMOVED

The following provisioning modes have been removed:
- `RemoteConfig` - Used VirtualMedia + kernel parameters
- `PreBakedISO` - Used VirtualMedia
- `PXE` (legacy) - Replaced with iPXE-only workflow

**Migration Path:** All provisioning now uses iPXE + inspection workflow.

### 4. OS Family Configuration
**Status:** REMOVED

The `osFamily` field has been removed from `Beskar7MachineSpec`.

**Rationale:** The inspection phase and final OS image determine the OS, not Beskar7.

---

## API Changes

### Beskar7Machine Spec

**Removed Fields:**
```yaml
imageURL: ""           # No longer needed (was for ISO)
configURL: ""          # No longer needed (was for RemoteConfig)
osFamily: ""           # No longer needed (OS determined by target image)
provisioningMode: ""   # Only iPXE now
bootMode: ""           # UEFI only, no legacy support
```

**New Fields:**
```yaml
inspectionImageURL: "http://boot-server/beskar7-inspector/boot"  # iPXE boot script
targetImageURL: "http://boot-server/kairos/latest.tar.gz"        # Final OS for kexec
configurationURL: "http://config-server/config.yaml"             # Optional config
hardwareRequirements:                                             # Optional validation
  minCPUCores: 4
  minMemoryGB: 16
  minDiskGB: 100
```

### PhysicalHost Spec

**Removed Fields:**
```yaml
bootISOSource: ""      # No longer needed
userDataSecretRef: ""  # Removed (not implemented)
```

**New States:**
```yaml
# Old states (removed):
- StateClaimed
- StateProvisioning
- StateProvisioned
- StateDeprovisioning

# New states:
- StateInUse        # Host is claimed
- StateInspecting   # Inspection image running
- StateReady        # Inspection complete, ready for OS
```

### PhysicalHost Status

**New Fields:**
```yaml
inspectionReport:       # Hardware details from inspection
  timestamp: "..."
  cpus:
    count: 2
    cores: 16
    model: "Intel Xeon"
  memory:
    totalGB: 64
  disks:
    - device: "/dev/sda"
      sizeGB: 500
      type: "SSD"
  nics:
    - interface: "eth0"
      macAddress: "00:11:22:33:44:55"
inspectionPhase: "Complete"      # Pending, InProgress, Complete, Failed, Timeout
inspectionTimestamp: "..."       # When inspection started
```

---

## Redfish Client Changes

### Simplified Interface

The Redfish client interface has been dramatically simplified:

**Before:**
```go
type Client interface {
    Close(ctx context.Context)
    GetSystemInfo(ctx context.Context) (*SystemInfo, error)
    GetPowerState(ctx context.Context) (redfish.PowerState, error)
    SetPowerState(ctx context.Context, state redfish.PowerState) error
    SetBootSourceISO(ctx context.Context, isoURL string) error      // REMOVED
    SetBootSourcePXE(ctx context.Context) error
    EjectVirtualMedia(ctx context.Context) error                    // REMOVED
    SetBootParameters(ctx context.Context, params []string) error   // REMOVED
    SetBootParametersWithAnnotations(...)                           // REMOVED
    GetNetworkAddresses(ctx context.Context) ([]NetworkAddress, error)
}
```

**After:**
```go
type Client interface {
    Close(ctx context.Context)
    GetSystemInfo(ctx context.Context) (*SystemInfo, error)
    GetPowerState(ctx context.Context) (redfish.PowerState, error)
    SetPowerState(ctx context.Context, state redfish.PowerState) error
    SetBootSourcePXE(ctx context.Context) error                     // iPXE only
    Reset(ctx context.Context) error                                // NEW: for troubleshooting
    GetNetworkAddresses(ctx context.Context) ([]NetworkAddress, error)
}
```

---

## New Requirements

### 1. iPXE Infrastructure

You must provide:
- **DHCP Server:** Configure to chainload iPXE
- **HTTP Server:** Serve iPXE boot scripts and inspection images
- **Boot Server:** Host the beskar7-inspector image and final OS images
- **DNS:** (Optional) For friendly names

See [`docs/ipxe-setup.md`](docs/ipxe-setup.md) for detailed setup instructions.

### 2. Beskar7-Inspector Image

The inspection phase requires the separate `beskar7-inspector` repository:
- Alpine Linux-based inspection image
- Collects hardware information
- Reports back to Beskar7 API
- Performs kexec into final OS

Repository: `https://github.com/wrkode/beskar7-inspector` (separate repo)

---

## Migration Guide

### For Existing Users

**Step 1: Understand the Breaking Changes**
- VirtualMedia is completely removed
- All provisioning now uses iPXE
- API fields have changed significantly

**Step 2: Set Up iPXE Infrastructure**
Follow the guide at [`docs/ipxe-setup.md`](docs/ipxe-setup.md):
1. Configure DHCP for iPXE
2. Set up HTTP server for boot scripts
3. Host inspection and target OS images
4. Configure DNS (optional)

**Step 3: Update Your Manifests**

**Old (v0.x):**
```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: Beskar7Machine
metadata:
  name: worker-01
spec:
  imageURL: "https://releases.example.com/kairos.iso"
  configURL: "https://config.example.com/worker.yaml"
  osFamily: "kairos"
  provisioningMode: "RemoteConfig"
  bootMode: "UEFI"
```

**New (v1.0):**
```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: Beskar7Machine
metadata:
  name: worker-01
spec:
  inspectionImageURL: "http://boot-server/beskar7-inspector/boot"
  targetImageURL: "http://boot-server/kairos/latest.tar.gz"
  configurationURL: "http://config-server/worker.yaml"
  hardwareRequirements:
    minCPUCores: 4
    minMemoryGB: 8
```

**Step 4: Deploy New Beskar7 Version**
```bash
# Uninstall old version
helm uninstall beskar7 -n beskar7-system

# Install new version (v1.0+)
helm install beskar7 beskar7/beskar7 -n beskar7-system --create-namespace
```

**Step 5: Update CRDs**
```bash
kubectl apply -f https://github.com/wrkode/beskar7/releases/download/v1.0.0/crds.yaml
```

---

## Compatibility

### Not Compatible

The v1.0 release is NOT compatible with v0.x releases. This is a complete architectural rewrite.

### Upgrade Path

There is NO in-place upgrade path. You must:
1. Delete all v0.x resources
2. Set up new iPXE infrastructure
3. Install v1.0 with new manifests

---

## Benefits of New Architecture

### 1. Simplified Codebase
- **Before:** 3000+ lines of vendor-specific code
- **After:** ~1500 lines of clean, generic code
- **Reduction:** 50% less code to maintain

### 2. Better Reliability
- No vendor-specific quirks
- Network boot is universal
- Easier troubleshooting

### 3. Real Hardware Discovery
- Inspection phase collects actual hardware specs
- No guessing about CPU, RAM, disks
- Validation before provisioning

### 4. Vendor Agnostic
- Works with any Redfish-compliant BMC
- Only uses universally-supported Redfish features
- Minimal vendor differences

### 5. Scalability
- Network boot scales better than VirtualMedia
- HTTP serving is faster than BMC mounting
- Can provision many hosts simultaneously

---

## Support

For questions or issues:
- **Issues:** https://github.com/wrkode/beskar7/issues
- **Discussions:** https://github.com/wrkode/beskar7/discussions
- **Documentation:** [`docs/`](docs/)

---

## Acknowledgments

This refactoring was necessary to make Beskar7 production-ready and maintainable. While it introduces breaking changes, the new architecture is significantly more reliable and easier to understand.

Thank you for your patience during this transition.

