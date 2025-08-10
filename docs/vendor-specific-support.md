# Vendor-Specific Hardware Support

Beskar7 provides automatic vendor-specific support for major server manufacturers, handling hardware quirks and boot parameter mechanisms transparently. This eliminates the need for manual configuration in most cases while providing advanced options for power users.

## Overview

Different server vendors implement Redfish APIs with varying capabilities and quirks. Beskar7 automatically detects your hardware vendor and applies the appropriate configuration methods, ensuring reliable provisioning across diverse hardware environments.

### Automatic Vendor Detection

Beskar7 automatically detects hardware vendors based on the system manufacturer reported via Redfish:

| Manufacturer Field | Detected Vendor | Boot Method |
|-------------------|-----------------|-------------|
| "Dell Inc." | Dell | BIOS `KernelArgs` attribute |
| "HPE" | HPE | UEFI Target Boot Override |
| "Lenovo" | Lenovo | UEFI with BIOS fallback |
| "Supermicro" | Supermicro | UEFI (BIOS attribute fallback may be required) |
| Others | Generic | UEFI with fallback support |

## Supported Hardware

### Dell Technologies

**Automatically Handled:**
- PowerEdge R750, R650, R350 series
- iDRAC 9 systems
- BIOS attribute method using `KernelArgs`

**What works automatically:**
- RemoteConfig mode (kernel parameter injection)
- PreBakedISO mode
- Virtual media mounting
- Power management

**Example PhysicalHost:**
```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: PhysicalHost
metadata:
  name: dell-server
  namespace: default
spec:
  redfishConnection:
    address: "https://idrac.example.com"
    credentialsSecretRef: "dell-credentials"
    insecureSkipVerify: false
```

### HPE (Hewlett Packard Enterprise)

**Automatically Handled:**
- ProLiant DL380, DL360, ML350 Gen10+ series
- iLO 5 systems
- UEFI Target Boot Override method

**What works automatically:**
- RemoteConfig mode (excellent compatibility)
- PreBakedISO mode
- Virtual media mounting
- Power management

### Lenovo

**Automatically Handled:**
- ThinkSystem SR650, SR630, SR950 series
- XCC (eXtended Configuration and Control) systems
- UEFI method with BIOS attribute fallback

**What works automatically:**
- RemoteConfig mode with intelligent fallback
- PreBakedISO mode
- Virtual media mounting
- Power management

### Supermicro

**Automatically Handled:**
- X12, X11, H12 series
- Modern BMC implementations
- UEFI target override (BIOS attribute override available via annotation)

**What works automatically:**
- RemoteConfig mode (force BIOS attribute via annotation if UEFI override fails)
- PreBakedISO mode (recommended for older systems)
- Virtual media mounting
- Power management

## Manual Configuration (Advanced)

For special cases or debugging, you can override the automatic vendor detection using annotations.

### Available Annotations

| Annotation | Purpose | Values |
|------------|---------|---------|
| `beskar7.infrastructure.cluster.x-k8s.io/boot-parameter-mechanism` | Override boot method | `bios-attribute`, `uefi-target`, `unsupported` |
| `beskar7.infrastructure.cluster.x-k8s.io/bios-kernel-arg-attribute` | Specify BIOS attribute name | `KernelArgs`, `CustomBootArgs`, etc. |

### Common Override Scenarios

#### Force BIOS Attribute Method

Useful for systems that auto-detect as UEFI-capable but work better with BIOS attributes:

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: PhysicalHost
metadata:
  name: custom-server
  annotations:
    beskar7.infrastructure.cluster.x-k8s.io/boot-parameter-mechanism: "bios-attribute"
    beskar7.infrastructure.cluster.x-k8s.io/bios-kernel-arg-attribute: "KernelArgs"
spec:
  redfishConnection:
    address: "https://bmc.example.com"
    credentialsSecretRef: "server-credentials"
```

#### Force UEFI Method

For systems incorrectly detected as requiring BIOS attributes:

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: PhysicalHost
metadata:
  name: uefi-server
  annotations:
    beskar7.infrastructure.cluster.x-k8s.io/boot-parameter-mechanism: "uefi-target"
spec:
  redfishConnection:
    address: "https://bmc.example.com"
    credentialsSecretRef: "server-credentials"
```

#### Disable Boot Parameter Setting

For problematic systems, disable RemoteConfig and use PreBakedISO only:

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: PhysicalHost
metadata:
  name: problematic-server
  annotations:
    beskar7.infrastructure.cluster.x-k8s.io/boot-parameter-mechanism: "unsupported"
spec:
  redfishConnection:
    address: "https://bmc.example.com"
    credentialsSecretRef: "server-credentials"
```

#### Custom BIOS Attribute Name

For vendors using non-standard BIOS attribute names:

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: PhysicalHost
metadata:
  name: custom-bios-server
  annotations:
    beskar7.infrastructure.cluster.x-k8s.io/boot-parameter-mechanism: "bios-attribute"
    beskar7.infrastructure.cluster.x-k8s.io/bios-kernel-arg-attribute: "BootParameters"
spec:
  redfishConnection:
    address: "https://bmc.example.com"
    credentialsSecretRef: "server-credentials"
```

## Provisioning Modes

### RemoteConfig Mode (Recommended)

Automatically configures the system to fetch configuration from a URL during boot:

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: Beskar7Machine
metadata:
  name: worker-node
spec:
  provisioningMode: "RemoteConfig"
  imageURL: "https://releases.example.com/kairos-v2.4.iso"
  configURL: "https://config.example.com/worker-config.yaml"
  osFamily: "kairos"
```

**How it works:**
1. Beskar7 detects your hardware vendor
2. Applies appropriate boot parameter mechanism
3. Injects kernel parameters like `config_url=https://config.example.com/worker-config.yaml`
4. System boots and fetches configuration automatically

### PreBakedISO Mode (Fallback)

Uses pre-configured ISO images with embedded configuration:

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: Beskar7Machine
metadata:
  name: worker-node
spec:
  provisioningMode: "PreBakedISO"
  imageURL: "https://custom-isos.example.com/worker-configured.iso"
  osFamily: "kairos"
```

## Troubleshooting

### Check Vendor Detection

View the PhysicalHost status to see detected vendor information:

```bash
kubectl get physicalhost my-server -o yaml
```

Look for vendor information in the hardware details:

```yaml
status:
  hardwareDetails:
    manufacturer: "Dell Inc."
    model: "PowerEdge R750"
    # ... vendor automatically detected as Dell
```

### Boot Parameter Issues

If RemoteConfig isn't working:

1. **Check logs:**
   ```bash
kubectl logs -n beskar7-system deployment/beskar7-controller-manager
   ```

2. **Look for vendor-specific messages:**
   ```
   INFO    Attempting to set boot parameters with vendor-specific support
   INFO    Successfully set boot parameters using vendor-specific method
   ```

3. **Try manual override:**
   Add annotations to force a specific method

### Common Issues

#### Dell Systems

**Problem:** RemoteConfig fails with "boot parameter setting failed"
**Solution:** Now handled automatically - Beskar7 detects Dell systems and uses BIOS attributes

#### HPE Systems

**Problem:** Authentication failures
**Solution:** Ensure iLO user has proper privileges:
- Login privilege
- Remote Console privilege
- Configure Manager privilege

#### Supermicro Systems

**Problem:** Inconsistent behavior
**Solution:** Update BMC firmware or use PreBakedISO mode

#### Generic/Unknown Vendors

**Problem:** Boot parameters not working
**Solution:** Try manual annotation overrides or use PreBakedISO mode

## Best Practices

### 1. Let Automatic Detection Work

Start without annotations and let Beskar7 detect your hardware:

```yaml
# Good - Let Beskar7 auto-detect
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: PhysicalHost
metadata:
  name: my-server
spec:
  redfishConnection:
    address: "https://bmc.example.com"
    credentialsSecretRef: "credentials"
```

### 2. Use Annotations Only When Needed

Only add annotations if automatic detection doesn't work:

```yaml
# Only when needed
metadata:
  annotations:
    beskar7.infrastructure.cluster.x-k8s.io/boot-parameter-mechanism: "bios-attribute"
```

### 3. Test with Known-Good Hardware

Start with well-supported hardware (Dell R750, HPE DL380 Gen10) to validate your setup.

### 4. Monitor Logs

Watch controller logs during provisioning to understand what's happening:

```bash
kubectl logs -f -n beskar7-system deployment/beskar7-controller-manager
```

### 5. Fallback to PreBakedISO

If RemoteConfig continues to fail, use PreBakedISO mode as a reliable fallback.

## Migration from Manual Configuration

If you were previously using manual workarounds for Dell systems:

### Before (Manual BIOS Configuration)
```yaml
# Old way - manual intervention required
metadata:
  annotations:
    infrastructure.cluster.x-k8s.io/skip-boot-params: "true"
# Manual BIOS configuration via iDRAC required
```

### After (Automatic)
```yaml
# New way - works automatically
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: PhysicalHost
metadata:
  name: dell-server
spec:
  redfishConnection:
    address: "https://idrac.example.com"
    credentialsSecretRef: "dell-credentials"
# No manual configuration needed!
```

## Extending Support

To add support for new vendors or hardware:

1. Hardware vendors should ensure their Redfish implementation follows standards
2. Report compatibility issues with hardware details and Redfish endpoint information
3. Contribute test results and vendor-specific workarounds

See [Contributing Hardware Test Results](hardware-compatibility.md#contributing-hardware-test-results) for details.

## Related Documentation

- [Hardware Compatibility Matrix](hardware-compatibility.md) - Detailed vendor support status
- [Beskar7Machine Configuration](beskar7machine.md) - Machine provisioning options
- [Troubleshooting Guide](advanced-usage.md#troubleshooting) - General troubleshooting steps
- [API Reference](api-reference.md) - Complete API documentation 