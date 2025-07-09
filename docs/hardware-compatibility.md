# Hardware Compatibility Matrix

This document provides information about hardware vendors, BMC implementations, and their compatibility with Beskar7.

## Overview

Beskar7 works with any server that implements the Redfish API standard. However, due to vendor-specific implementations and quirks, some features may work better on certain platforms than others.

## Compatibility Status

| Status | Description |
|--------|-------------|
| ✅ **Tested** | Fully tested and confirmed working |
| ⚠️ **Partial** | Works with known limitations |
| ❓ **Untested** | Should work based on Redfish standard compliance but not verified |
| ❌ **Not Supported** | Known issues prevent proper operation |

## Vendor Support Matrix

### Dell Technologies

| Model/Series | BMC Version | Redfish Version | RemoteConfig | PreBakedISO | Virtual Media | Power Management | Notes |
|--------------|-------------|-----------------|--------------|-------------|---------------|------------------|-------|
| PowerEdge R750 | iDRAC9 6.x | 1.11+ | ✅ | ✅ | ✅ | ✅ | **Auto-detects** and uses `KernelArgs` BIOS attribute |
| PowerEdge R650 | iDRAC9 5.x+ | 1.9+ | ✅ | ✅ | ✅ | ✅ | **Auto-detects** Dell systems and uses BIOS attributes |
| PowerEdge R350 | iDRAC9 4.x+ | 1.8+ | ❓ | ✅ | ✅ | ✅ | Not extensively tested |

**Dell-Specific Notes:**
- Dell BMCs may require setting BIOS attributes instead of using `UefiTargetBootSourceOverride`
- Virtual media mounting is reliable
- Consider using iDRAC licenses for advanced features

### HPE (Hewlett Packard Enterprise)

| Model/Series | BMC Version | Redfish Version | RemoteConfig | PreBakedISO | Virtual Media | Power Management | Notes |
|--------------|-------------|-----------------|--------------|-------------|---------------|------------------|-------|
| ProLiant DL380 Gen10+ | iLO 5 2.x+ | 1.6+ | ✅ | ✅ | ✅ | ✅ | Good Redfish compliance |
| ProLiant DL360 Gen10+ | iLO 5 2.x+ | 1.6+ | ✅ | ✅ | ✅ | ✅ | UefiTargetBootSourceOverride works well |
| ProLiant ML350 Gen10+ | iLO 5 1.x+ | 1.4+ | ⚠️ | ✅ | ✅ | ✅ | Older firmware may have issues |

**HPE-Specific Notes:**
- iLO 5 generally provides excellent Redfish compliance
- Virtual media and boot override mechanisms work reliably
- Ensure iLO firmware is up to date for best results

### Supermicro

| Model/Series | BMC Version | Redfish Version | RemoteConfig | PreBakedISO | Virtual Media | Power Management | Notes |
|--------------|-------------|-----------------|--------------|-------------|---------------|------------------|-------|
| X12 Series | BMC 1.x+ | 1.8+ | ⚠️ | ✅ | ✅ | ✅ | Variable Redfish implementation quality |
| X11 Series | IPMI 3.x+ | 1.4+ | ❌ | ✅ | ⚠️ | ✅ | Limited Redfish support |
| H12 Series | BMC 2.x+ | 1.9+ | ❓ | ✅ | ✅ | ✅ | AMD-based, not extensively tested |

**Supermicro-Specific Notes:**
- Redfish implementation varies significantly across product lines
- Newer X12+ series have better compatibility
- Virtual media may require specific configuration

### Lenovo

| Model/Series | BMC Version | Redfish Version | RemoteConfig | PreBakedISO | Virtual Media | Power Management | Notes |
|--------------|-------------|-----------------|--------------|-------------|---------------|------------------|-------|
| ThinkSystem SR650 V2 | XCC 2.x+ | 1.8+ | ✅ | ✅ | ✅ | ✅ | Good overall compatibility |
| ThinkSystem SR630 V2 | XCC 2.x+ | 1.8+ | ✅ | ✅ | ✅ | ✅ | Similar to SR650 V2 |
| ThinkSystem SR950 | XCC 1.x+ | 1.6+ | ⚠️ | ✅ | ✅ | ✅ | Some boot parameter limitations |

**Lenovo-Specific Notes:**
- XCC (eXtended Configuration and Control) provides good Redfish support
- Boot parameter injection generally works well
- Virtual media mounting is reliable

### Generic/Whitebox

| Type | BMC/BIOS | Redfish Version | RemoteConfig | PreBakedISO | Virtual Media | Power Management | Notes |
|------|----------|-----------------|--------------|-------------|---------------|------------------|-------|
| AMI MegaRAC | Various | 1.4+ | ⚠️ | ✅ | ⚠️ | ✅ | Implementation varies by OEM |
| Aspeed AST2500/2600 | OpenBMC | 1.6+ | ❓ | ✅ | ✅ | ✅ | Open source BMC stack |
| Intel Server Board | RMM4/BMC | 1.3+ | ❌ | ✅ | ⚠️ | ✅ | Limited Redfish feature set |

## Feature Compatibility Details

### RemoteConfig Mode Support

**Requirements for RemoteConfig:**
- Redfish API version 1.6+
- Support for `UefiTargetBootSourceOverride` or vendor-specific BIOS attribute setting
- Ability to pass kernel parameters to the bootloader

**Known Working Implementations:**
- ✅ **HPE iLO 5** - Automatic detection and `UefiTargetBootSourceOverride` usage
- ✅ **Dell iDRAC** - Automatic detection and BIOS attribute (`KernelArgs`) usage  
- ✅ **Lenovo XCC** - Automatic detection with boot parameter injection
- ✅ **Supermicro BMC** - Automatic detection with fallback mechanisms

**Vendor-Specific Features:**
- **Automatic vendor detection** based on system manufacturer
- **Intelligent fallback mechanisms** when primary methods fail
- **Annotation-based overrides** for custom configurations
- **BIOS attribute management** for Dell and other vendors requiring it

**Resolved Issues:**
- ✅ Dell kernel parameter injection now works automatically via BIOS attributes
- ✅ Vendor-specific quirks handled transparently 
- ✅ Manual configuration reduced through automatic detection

### Virtual Media Support

**Requirements:**
- HTTP/HTTPS virtual media mounting
- CD/DVD virtual media device emulation
- Boot priority override capabilities

**Vendor Notes:**
- **Dell:** Excellent virtual media support, reliable mounting
- **HPE:** Very good support, handles large ISOs well
- **Lenovo:** Good support with proper licensing
- **Supermicro:** Variable support, may require specific BMC settings

### Power Management

All tested vendors support basic power operations:
- Power On/Off
- Power Status queries
- Graceful shutdown (where supported by OS)

## Operating System Support Matrix

### Supported OS Families

| OS Family | Version Range | RemoteConfig | PreBakedISO | Notes |
|-----------|---------------|--------------|-------------|-------|
| **Kairos** | v2.4+ | ✅ | ✅ | Excellent support, cloud-init compatible |
| **Talos** | v1.4+ | ✅ | ✅ | Native machine config support |
| **Flatcar** | 3400+ | ✅ | ✅ | Ignition-based configuration |
| **openSUSE LeapMicro** | 5.3+ | ✅ | ✅ | Combustion script support |
| **Ubuntu** | 20.04+ | ⚠️ | ✅ | Limited RemoteConfig testing |
| **RHEL/CentOS** | 8+ | ⚠️ | ✅ | Kickstart-based, limited RemoteConfig |
| **Fedora** | 35+ | ⚠️ | ✅ | Cloud-init compatible |
| **Debian** | 11+ | ⚠️ | ✅ | Preseed-based configuration |
| **openSUSE** | 15.4+ | ⚠️ | ✅ | AutoYaST configuration |

### OS-Specific Configuration

**Kairos:**
- Uses `config_url=<URL>` kernel parameter
- Supports cloud-init and Kairos-specific configuration
- Excellent unattended installation support

**Talos:**
- Uses `talos.config=<URL>` kernel parameter
- Machine configuration via YAML
- Built for Kubernetes, minimal attack surface

**Flatcar:**
- Uses `flatcar.ignition.config.url=<URL>` kernel parameter
- Ignition-based configuration
- Container-optimized Linux

## Troubleshooting by Vendor

### Dell Troubleshooting

**Common Issues:**
1. **RemoteConfig fails with kernel parameter errors**
   - **Solution:** ✅ **AUTOMATICALLY HANDLED** - Beskar7 now automatically detects Dell systems and uses BIOS attribute setting
   - **Manual Override:** Use annotation `beskar7.infrastructure.cluster.x-k8s.io/bios-kernel-arg-attribute: "KernelArgs"` if needed

2. **Virtual media mounting timeouts**
   - **Solution:** Ensure ISO URLs are accessible from BMC network
   - **Check:** iDRAC network configuration and DNS resolution

3. **Power operations fail**
   - **Solution:** Verify iDRAC licensing and user permissions
   - **Check:** User has "Configure Manager" privileges

### HPE Troubleshooting

**Common Issues:**
1. **Redfish authentication failures**
   - **Solution:** Ensure user account has proper role assignments
   - **Check:** User has "Login" and "Remote Console" privileges

2. **Boot override not persisting**
   - **Solution:** Use one-time boot override instead of permanent
   - **Check:** iLO firmware version compatibility

### Supermicro Troubleshooting

**Common Issues:**
1. **Inconsistent Redfish behavior**
   - **Solution:** Update BMC firmware to latest version
   - **Workaround:** Use PreBakedISO mode for better reliability

2. **Virtual media compatibility issues**
   - **Solution:** Configure BMC virtual media settings
   - **Check:** Enable virtual media in BMC configuration

## Testing Hardware

To test your hardware compatibility with Beskar7:

### 1. Basic Redfish Connectivity Test

```bash
# Test basic Redfish endpoint
curl -k -u username:password https://BMC_IP/redfish/v1/

# Test system information
curl -k -u username:password https://BMC_IP/redfish/v1/Systems/
```

### 2. Virtual Media Test

```bash
# Check virtual media managers
curl -k -u username:password https://BMC_IP/redfish/v1/Managers/
curl -k -u username:password https://BMC_IP/redfish/v1/Managers/1/VirtualMedia/
```

### 3. Boot Override Test

```bash
# Check boot options
curl -k -u username:password https://BMC_IP/redfish/v1/Systems/1/
```

### 4. Deploy Test PhysicalHost

Create a test PhysicalHost resource and monitor its progression through states:

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: PhysicalHost
metadata:
  name: compatibility-test
  namespace: default
spec:
  redfishConnection:
    address: "https://YOUR_BMC_IP"
    credentialsSecretRef: "test-credentials"
    insecureSkipVerify: true  # For testing only
```

Monitor the resource:

```bash
kubectl get physicalhost compatibility-test -o wide
kubectl describe physicalhost compatibility-test
```

## Reporting Compatibility Issues

When reporting hardware compatibility issues, please include:

1. **Hardware Information:**
   - Vendor and model
   - BMC/BIOS version
   - Firmware versions

2. **Redfish Information:**
   - Redfish version support
   - Available endpoints (`/redfish/v1/odata`)

3. **Error Details:**
   - Controller logs
   - Redfish API responses
   - Error conditions observed

4. **Configuration:**
   - PhysicalHost resource definition
   - Beskar7Machine resource definition
   - Network configuration

Submit issues to: https://github.com/wrkode/beskar7/issues

## Advanced Configuration: Vendor-Specific Annotations

Beskar7 now supports annotation-based overrides for vendor-specific behavior. These annotations allow you to customize how boot parameters are set for specific hardware.

### Available Annotations

| Annotation | Description | Example Value |
|------------|-------------|---------------|
| `beskar7.infrastructure.cluster.x-k8s.io/boot-parameter-mechanism` | Override the boot parameter method | `bios-attribute`, `uefi-target`, `unsupported` |
| `beskar7.infrastructure.cluster.x-k8s.io/bios-kernel-arg-attribute` | Specify BIOS attribute name for kernel args | `KernelArgs`, `CustomBootArgs` |

### Example Usage

**Force BIOS attribute method for Dell systems:**
```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: PhysicalHost
metadata:
  name: dell-server
  annotations:
    beskar7.infrastructure.cluster.x-k8s.io/boot-parameter-mechanism: "bios-attribute"
    beskar7.infrastructure.cluster.x-k8s.io/bios-kernel-arg-attribute: "KernelArgs"
spec:
  # ... rest of spec
```

**Force UEFI method for systems that auto-detect as requiring BIOS attributes:**
```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: PhysicalHost
metadata:
  name: custom-server
  annotations:
    beskar7.infrastructure.cluster.x-k8s.io/boot-parameter-mechanism: "uefi-target"
spec:
  # ... rest of spec
```

**Disable boot parameter setting entirely:**
```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: PhysicalHost
metadata:
  name: problematic-server
  annotations:
    beskar7.infrastructure.cluster.x-k8s.io/boot-parameter-mechanism: "unsupported"
spec:
  # ... rest of spec (will use PreBakedISO mode only)
```

### Automatic Vendor Detection

Beskar7 automatically detects hardware vendors based on the system manufacturer field in Redfish and applies appropriate configurations:

- **Dell Inc.** → Uses BIOS attribute method with `KernelArgs` attribute
- **HPE** → Uses UEFI target boot source override method
- **Lenovo** → Uses UEFI method with fallback to BIOS attributes
- **Supermicro** → Uses UEFI method with multiple fallback mechanisms
- **Others** → Uses generic UEFI method with fallback support

Annotations override this automatic detection when specified.

## Vendor-Specific Support Status

### ✅ Completed Features

- **Dell:** ✅ BIOS attribute configuration automation (`KernelArgs` support)
- **HPE:** ✅ `UefiTargetBootSourceOverride` optimization
- **Lenovo:** ✅ XCC-specific boot parameter detection
- **Supermicro:** ✅ Multi-mechanism fallback support
- **All Vendors:** ✅ Automatic vendor detection and method selection

### 🔄 Planned Enhancements

- **Dell:** Advanced iDRAC job management for BIOS settings
- **HPE:** Extended iLO 6 feature integration  
- **Supermicro:** Enhanced BMC version detection and workarounds
- **Lenovo:** XCC licensing detection and advanced features
- **All Vendors:** Expanded annotation-based configuration options

## Contributing Hardware Test Results

We welcome community contributions for hardware compatibility testing. Please submit test results including:

- Hardware specifications
- Test configurations used
- Success/failure results for each feature
- Any workarounds discovered

This helps improve compatibility and guides future development priorities. 