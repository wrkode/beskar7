# Beskar7: Bare-Metal Provisioning for Immutable Kubernetes

Beskar7 is a Kubernetes operator that implements the Cluster API infrastructure provider contract for managing bare-metal machines. It uses a simple, reliable approach: **Redfish for power management + iPXE for network boot + Hardware inspection**.

## Why Beskar7?

- **Simple** - No complex VirtualMedia or vendor-specific workarounds
- **Reliable** - Uses only universally-supported Redfish features (power management)
- **Vendor Agnostic** - Works with any Redfish-compliant BMC
- **Hardware Discovery** - Real hardware specs collected during inspection
- **Production Ready** - Clean architecture, minimal dependencies

## How It Works

```
┌─────────────┐
│   Beskar7   │  1. Claims physical host
│  Controller │  2. Sets PXE boot via Redfish
└──────┬──────┘  3. Powers on server
       │
       ▼
┌─────────────┐
│   Redfish   │  Simple power management
│     BMC     │  (No vendor quirks!)
└──────┬──────┘
       │
       ▼
┌─────────────┐
│    iPXE     │  4. Network boots inspection image
│   Boot      │  (for the moment - You provide DHCP + HTTP server)
└──────┬──────┘
       │
       ▼
┌─────────────┐
│  Inspector  │  5. Boots Alpine Linux
│  (Alpine)   │  6. Collects CPU, RAM, disks, NICs
└──────┬──────┘  7. Reports back to Beskar7
       │          8. Downloads target OS
       ▼          9. Kexecs into final OS
┌─────────────┐
│   Beskar7   │ 10. Validates hardware requirements
│  Controller │ 11. Marks machine ready
└─────────────┘
```

## Current Status

**Version:** 1.0 (Major Architecture Simplification)

**Status:** Alpha - Under active development

**Breaking Changes:** v1.0 is NOT compatible with v0.x. See [`BREAKING_CHANGES.md`](BREAKING_CHANGES.md) for migration guide.

## Key Features

- **Power Management** - Simple on/off via Redfish
- **iPXE Provisioning** - Network boot for any OS
- **Hardware Inspection** - Real CPU, RAM, disk, NIC discovery
- **Hardware Validation** - Enforce minimum requirements
- **Cluster API Integration** - Full CAPI provider implementation
- **Vendor Agnostic** - No vendor-specific code

## Quick Start

### Prerequisites

1. **Kubernetes Cluster** - v1.31+ with kubectl configured
2. **Cluster API** - v1.10+ installed ([install guide](#install-cluster-api))
3. **cert-manager** - For webhook certificates ([install guide](#install-cert-manager))
4. **iPXE Infrastructure** - DHCP + HTTP server ([setup guide](docs/ipxe-setup.md))
5. **Inspection Image** - beskar7-inspector deployed ([inspector repo](https://github.com/wrkode/beskar7-inspector))

### Install Cluster API (Required)

```bash
# Install clusterctl
curl -L https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.10.0/clusterctl-linux-amd64 -o clusterctl
chmod +x clusterctl
sudo mv clusterctl /usr/local/bin/

# Initialize Cluster API
clusterctl init
```

### Install cert-manager (Required)

```bash
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.16.2/cert-manager.yaml
kubectl wait --for=condition=Available --timeout=300s deployment/cert-manager -n cert-manager
```

### Install Beskar7

**Option 1: Using Helm (Recommended)**

```bash
# Add Helm repository
helm repo add beskar7 https://wrkode.github.io/beskar7
helm repo update

# Install
helm install beskar7 beskar7/beskar7 --namespace beskar7-system --create-namespace

# Wait for deployment
kubectl wait --for=condition=available --timeout=600s deployment/beskar7-controller-manager -n beskar7-system
```

**Option 2: Using Release Manifests**

```bash
# Download and apply release manifest
kubectl apply -f https://github.com/wrkode/beskar7/releases/download/v1.0.0/beskar7-manifests-v1.0.0.yaml
```

## Usage

### 1. Create BMC Credentials Secret

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: bmc-credentials
  namespace: default
stringData:
  username: "admin"
  password: "your-bmc-password"
```

```bash
kubectl apply -f bmc-credentials.yaml
```

### 2. Register Physical Hosts

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: PhysicalHost
metadata:
  name: server-01
  namespace: default
spec:
  redfishConnection:
    address: "https://192.168.1.100"  # BMC IP address
    credentialsSecretRef: "bmc-credentials"
```

```bash
kubectl apply -f physicalhost.yaml

# Check status
kubectl get physicalhost server-01 -o wide
```

### 3. Create a Machine

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: Beskar7Machine
metadata:
  name: worker-01
  namespace: default
spec:
  # iPXE boot script URL (boots inspection image)
  inspectionImageURL: "http://boot-server/beskar7-inspector/boot"
  
  # Final OS image URL (for kexec after inspection)
  targetImageURL: "http://boot-server/kairos/v2.8.1.tar.gz"
  
  # Optional: OS configuration
  configurationURL: "http://config-server/worker-config.yaml"
  
  # Optional: Hardware requirements
  hardwareRequirements:
    minCPUCores: 4
    minMemoryGB: 16
    minDiskGB: 100
```

```bash
kubectl apply -f beskar7machine.yaml

# Monitor provisioning
kubectl get beskar7machine worker-01 -o wide
kubectl describe beskar7machine worker-01
```

### 4. Monitor Inspection

```bash
# Check PhysicalHost for inspection report
kubectl get physicalhost server-01 -o jsonpath='{.status.inspectionReport}' | jq

# Example output:
# {
#   "cpus": {
#     "count": 2,
#     "cores": 16,
#     "model": "Intel Xeon E5-2640"
#   },
#   "memory": {
#     "totalGB": 64
#   },
#   "disks": [
#     {
#       "device": "/dev/sda",
#       "sizeGB": 500,
#       "type": "SSD"
#     }
#   ]
# }
```

## Complete Example

See [`examples/simple-cluster.yaml`](examples/simple-cluster.yaml) for a complete working example with:
- BMC credentials
- Physical host registration  
- Control plane machine
- Worker machines
- Hardware requirements

## Architecture

### Components

**PhysicalHost Controller**
- Connects to BMC via Redfish
- Monitors power state
- Manages basic hardware info
- Tracks inspection reports

**Beskar7Machine Controller**
- Claims available PhysicalHost
- Triggers inspection boot via iPXE
- Validates hardware requirements
- Monitors provisioning progress
- Reports to Cluster API

**Beskar7Cluster Controller**
- Manages cluster-level infrastructure
- Sets control plane endpoint
- Coordinates machine lifecycle

### States

**PhysicalHost States:**
- `Enrolling` - Connecting to BMC
- `Available` - Ready to be claimed
- `InUse` - Claimed by a machine
- `Inspecting` - Running inspection image
- `Ready` - Inspection complete, validated
- `Error` - Something went wrong

**Inspection Phases:**
- `Pending` - Not started
- `Booting` - Inspection image booting
- `InProgress` - Collecting hardware info
- `Complete` - Report submitted
- `Failed` - Inspection error
- `Timeout` - Took too long (>10 minutes)

## Infrastructure Requirements

### iPXE Setup

You must provide iPXE infrastructure. See [`docs/ipxe-setup.md`](docs/ipxe-setup.md) for complete guide.

**Required Services:**
1. **DHCP Server** - Configured to chainload iPXE
2. **HTTP Server** - Serves iPXE boot scripts
3. **Boot Server** - Hosts inspection and OS images

**Quick Setup Example:**

```bash
# DHCP configuration (dnsmasq)
dhcp-match=set:efi-x86_64,option:client-arch,7
dhcp-boot=tag:efi-x86_64,ipxe.efi

# HTTP server (nginx)
server {
    listen 80;
    server_name boot-server;
    root /var/www/boot;
    
    location /beskar7-inspector/ {
        # Serve iPXE boot scripts
    }
    
    location /images/ {
        # Serve OS images
    }
}
```

### Inspection Image

Deploy the beskar7-inspector image. See [beskar7-inspector repository](https://github.com/wrkode/beskar7-inspector) for:
- Alpine Linux-based inspection image
- Hardware detection scripts
- Kexec boot scripts
- Configuration guide

## Hardware Compatibility

Beskar7 works with **any Redfish-compliant BMC** because it only uses:
- Power management (On/Off/Reset)
- PXE boot flag setting
- System information queries

**Tested Vendors:**
- Dell (iDRAC)
- HPE (iLO)
- Lenovo (XCC)
- Supermicro
- Generic Redfish BMCs

**No vendor-specific code needed!**

## Supported Operating Systems

Any OS that can be deployed via network boot:
- **Kairos** (recommended) - Cloud-native, immutable
- **Flatcar** - Container-optimized
- **Talos** - Kubernetes-optimized
- **Ubuntu** - Traditional Linux
- **RHEL/Rocky/Alma** - Enterprise Linux
- **Custom** - Build your own image

The inspection image uses Alpine Linux, and the final OS is determined by your `targetImageURL`.

## Documentation

- **[Breaking Changes](BREAKING_CHANGES.md)** - v1.0 migration guide
- **[iPXE Setup Guide](docs/ipxe-setup.md)** - Infrastructure requirements
- **[API Reference](docs/api-reference.md)** - Complete API documentation
- **[Examples](examples/)** - Working configuration examples
- **[Troubleshooting](docs/troubleshooting.md)** - Common issues and solutions

## Development

### Build from Source

```bash
# Clone repository
git clone https://github.com/wrkode/beskar7.git
cd beskar7

# Build
make build

# Run tests
make test

# Build and push container
make docker-build docker-push IMG=your-registry/beskar7:tag

# Deploy to cluster
make deploy IMG=your-registry/beskar7:tag
```

### Project Structure

```
beskar7/
├── api/v1beta1/              # CRD types
│   ├── beskar7machine_types.go
│   ├── physicalhost_types.go
│   └── beskar7cluster_types.go
├── controllers/              # Controllers
│   ├── beskar7machine_controller.go
│   ├── physicalhost_controller.go
│   └── beskar7cluster_controller.go
├── internal/
│   ├── redfish/             # Redfish client (simplified)
│   └── coordination/        # Host claiming logic
├── docs/                    # Documentation
├── examples/                # Example YAMLs
└── config/                  # Kustomize manifests
```

## Comparison with v0.x

### What Changed?

**v0.x (VirtualMedia):**
- Complex vendor-specific workarounds
- ISO mounting via VirtualMedia
- BIOS attribute manipulation
- Boot parameter injection
- 2,250 lines of code

**v1.0 (iPXE + Inspection):**
- Simple power management only
- Network boot via iPXE
- Hardware discovery via inspection
- No vendor-specific code
- 950 lines of code (58% reduction!)

### Why the Change?

1. **VirtualMedia is unreliable** - Implementations vary wildly across vendors
2. **Vendor quirks are complex** - Hard to maintain and debug
3. **Network boot is universal** - Works the same everywhere
4. **Hardware discovery is better** - Get real specs, not guesses

See [`BREAKING_CHANGES.md`](BREAKING_CHANGES.md) for complete details.

## Troubleshooting

### PhysicalHost Stuck in Enrolling

**Symptoms:** Host doesn't transition to Available

**Checks:**
```bash
# Check controller logs
kubectl logs -n beskar7-system deployment/beskar7-controller-manager -f

# Check PhysicalHost status
kubectl describe physicalhost <name>

# Test Redfish connectivity
curl -k -u username:password https://BMC_IP/redfish/v1/
```

**Common Causes:**
- BMC not reachable from controller pod
- Invalid credentials
- Firewall blocking Redfish port (443)

### Inspection Timeout

**Symptoms:** InspectionPhase shows "Timeout"

**Checks:**
```bash
# Check PhysicalHost inspection status
kubectl get physicalhost <name> -o jsonpath='{.status.inspectionPhase}'

# Check iPXE infrastructure
curl http://boot-server/beskar7-inspector/boot
```

**Common Causes:**
- iPXE infrastructure not configured
- Inspection image URL incorrect
- Server can't reach boot server
- Network boot disabled in BIOS

### Hardware Validation Failed

**Symptoms:** Machine stuck with validation error

**Checks:**
```bash
# Check inspection report
kubectl get physicalhost <name> -o jsonpath='{.status.inspectionReport}' | jq

# Check requirements
kubectl get beskar7machine <name> -o jsonpath='{.spec.hardwareRequirements}' | jq
```

**Solution:** Adjust `hardwareRequirements` or use different hardware.

## Contributing

Contributions are welcome! Please:
1. Open an issue to discuss major changes
2. Follow existing code style
3. Add tests for new features
4. Update documentation

## License

Apache License 2.0 - See [LICENSE](LICENSE) file for details.

## Support

- **Issues:** https://github.com/wrkode/beskar7/issues
- **Discussions:** https://github.com/wrkode/beskar7/discussions
- **Documentation:** https://github.com/wrkode/beskar7/tree/main/docs

## Acknowledgments

This project was inspired by and learns from:
- [Tinkerbell](https://tinkerbell.org/) - Network boot provisioning
- [Metal³](https://metal3.io/) - Kubernetes bare-metal
- [Cluster API](https://cluster-api.sigs.k8s.io/) - Kubernetes cluster lifecycle

---

**Beskar7** - Simple, reliable bare-metal provisioning for Kubernetes.
