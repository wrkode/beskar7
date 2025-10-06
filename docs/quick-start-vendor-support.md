# Quick Start: Vendor-Specific Hardware Support

This guide helps you quickly get started with Beskar7's automatic vendor-specific hardware support.

## TL;DR - It Just Works! 

Beskar7 now automatically detects your hardware vendor and handles boot parameter quirks. **No configuration needed for Dell, HPE, Lenovo, or Supermicro systems.**

## 30-Second Setup

### For Any Supported Hardware

1. **Create PhysicalHost** (no special configuration needed):
   ```yaml
   apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
   kind: PhysicalHost
   metadata:
     name: my-server
   spec:
     redfishConnection:
       address: "https://your-bmc-ip"
       credentialsSecretRef: "bmc-credentials"
   ```

2. **Create Beskar7Machine**:
   ```yaml
   apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
   kind: Beskar7Machine
   metadata:
     name: worker-node
   spec:
     provisioningMode: "RemoteConfig"  # Now works on Dell!
     imageURL: "https://github.com/kairos-io/kairos/releases/download/v2.8.1/kairos-alpine-v2.8.1-amd64.iso"
     configURL: "https://your-config-server/config.yaml"
     osFamily: "kairos"
   ```

3. **Done!** Beskar7 automatically:
   - Detects vendor (Dell/HPE/Lenovo/Supermicro)
   - Uses appropriate boot parameter method
   - Handles vendor-specific quirks

## What's New

### **Dell Systems Now Work Automatically**
- **Before:** Manual BIOS configuration required
- **Now:** Automatic detection and BIOS attribute handling

### **All Vendors Supported**
- **Dell:** BIOS `KernelArgs` attribute (automatic)
- **HPE:** UEFI Target Boot Override (automatic)  
- **Lenovo:** UEFI with BIOS fallback (automatic)
- **Supermicro:** UEFI with multiple fallbacks (automatic)

### **Zero Configuration Required**
- Works out of the box
- No vendor-specific annotations needed
- Intelligent fallback mechanisms

## Vendor-Specific Examples

### Dell PowerEdge Servers
```yaml
# This now works automatically - no special config needed!
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: PhysicalHost
metadata:
  name: dell-r750
spec:
  redfishConnection:
    address: "https://idrac.example.com"
    credentialsSecretRef: "dell-idrac-creds"
```

### HPE ProLiant Servers
```yaml
# Works automatically with excellent compatibility
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: PhysicalHost
metadata:
  name: hpe-dl380
spec:
  redfishConnection:
    address: "https://ilo.example.com"
    credentialsSecretRef: "hpe-ilo-creds"
```

### Lenovo ThinkSystem Servers
```yaml
# Automatic UEFI with BIOS fallback
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: PhysicalHost
metadata:
  name: lenovo-sr650
spec:
  redfishConnection:
    address: "https://xcc.example.com"
    credentialsSecretRef: "lenovo-xcc-creds"
```

### Supermicro Servers
```yaml
# Multiple fallback mechanisms for reliability
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: PhysicalHost
metadata:
  name: supermicro-x12
spec:
  redfishConnection:
    address: "https://bmc.example.com"
    credentialsSecretRef: "supermicro-bmc-creds"
```

## Advanced Overrides (Rarely Needed)

### Force Specific Boot Method
Only use if automatic detection fails:

```yaml
metadata:
  annotations:
    # Force BIOS attribute method
    beskar7.infrastructure.cluster.x-k8s.io/boot-parameter-mechanism: "bios-attribute"
    beskar7.infrastructure.cluster.x-k8s.io/bios-kernel-arg-attribute: "KernelArgs"
```

### Disable RemoteConfig
For problematic hardware, fall back to PreBakedISO:

```yaml
metadata:
  annotations:
    # Disable boot parameters, use PreBakedISO only
    beskar7.infrastructure.cluster.x-k8s.io/boot-parameter-mechanism: "unsupported"
```

## Quick Troubleshooting

### Check Vendor Detection
```bash
kubectl get physicalhost my-server -o jsonpath='{.status.hardwareDetails.manufacturer}'
# Should show: "Dell Inc.", "HPE", "Lenovo", "Supermicro", etc.
```

### Check Controller Logs
```bash
kubectl logs -n beskar7-system deployment/beskar7-controller-manager | grep vendor
```

Look for messages like:
```
INFO    Attempting to set boot parameters with vendor-specific support
INFO    Successfully set boot parameters using vendor-specific method
```

### If RemoteConfig Still Fails
1. Check BMC credentials and network connectivity
2. Try annotation overrides (see above)
3. Fall back to PreBakedISO mode
4. Check [detailed troubleshooting guide](vendor-specific-support.md#troubleshooting)

## Migration from Manual Workarounds

### If You Were Using Manual Dell Workarounds

**Remove these** (no longer needed):
```yaml
# DELETE THESE - handled automatically now
metadata:
  annotations:
    infrastructure.cluster.x-k8s.io/skip-boot-params: "true"
    # Any custom Dell BIOS annotations
```

**Keep it simple:**
```yaml
# This is all you need now
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: PhysicalHost
metadata:
  name: dell-server
spec:
  redfishConnection:
    address: "https://idrac.example.com"
    credentialsSecretRef: "dell-credentials"
```

## Complete Working Example

Here's a complete working example that provisions a Kubernetes node on any supported hardware:

```yaml
---
apiVersion: v1
kind: Secret
metadata:
  name: server-credentials
  namespace: default
type: Opaque
stringData:
  username: "admin"
  password: "your-bmc-password"

---
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: PhysicalHost
metadata:
  name: worker-server
  namespace: default
spec:
  redfishConnection:
    address: "https://your-bmc-ip"
    credentialsSecretRef: "server-credentials"
    insecureSkipVerify: true  # Only for testing

---
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: Beskar7Machine
metadata:
  name: worker-node
  namespace: default
spec:
  provisioningMode: "RemoteConfig"
  imageURL: "https://github.com/kairos-io/kairos/releases/download/v2.8.1/kairos-alpine-v2.8.1-amd64.iso"
  configURL: "https://your-config-server/worker-config.yaml"
  osFamily: "kairos"
```

## What's Next?

- [Vendor-Specific Support Guide](vendor-specific-support.md) - Detailed documentation
- [Hardware Compatibility Matrix](hardware-compatibility.md) - Vendor support status
- [Beskar7Machine Configuration](beskar7machine.md) - Machine configuration options 