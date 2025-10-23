# Beskar7 Examples

This directory contains practical examples for deploying and managing Kubernetes clusters with Beskar7.

## Quick Start Examples

### 🚀 [Minimal Test Cluster](minimal-test-cluster.yaml)

A simple, single-node example perfect for:
- Testing Beskar7 functionality
- Development and debugging
- Quick proof of concept

**Resources included:**
- 1 PhysicalHost
- 1 Beskar7Cluster
- 1 Beskar7Machine
- BMC credentials secret

**Use case:** Testing basic provisioning flow

### 🏗️ [Complete Cluster](complete-cluster.yaml)

A production-ready, multi-node cluster example featuring:
- 1 Control plane node
- 2 Worker nodes
- Full Cluster API integration
- High availability ready

**Resources included:**
- 3 PhysicalHosts
- 1 Beskar7Cluster
- Cluster API resources (Cluster, KubeadmControlPlane, MachineDeployment)
- Machine templates
- Bootstrap configurations

**Documentation:** [complete-cluster.md](complete-cluster.md)

**Use case:** Production deployments, learning CAPI integration

## Advanced Examples

### 🔒 [Security Examples](security/)

Production-hardened configurations with:
- RBAC policies
- Network policies
- Security best practices

See [security/README.md](security/README.md) for details.

### ⚡ [Leader Election Demo](leader_election_demo.md)

Demonstrates Beskar7's leader election mechanism for high availability controller deployments.

## Getting Started

### Prerequisites

Before using these examples, ensure you have:

1. **Beskar7 installed:**
   ```bash
   kubectl apply -f https://github.com/wrkode/beskar7/releases/download/v0.3.0/beskar7-manifests-v0.3.0.yaml
   ```

2. **cert-manager installed:**
   ```bash
   kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.16.2/cert-manager.yaml
   ```

3. **For complete cluster example, Cluster API:**
   ```bash
   clusterctl init
   ```

### Basic Usage

1. **Choose an example** based on your needs
2. **Edit the manifest** to match your environment:
   - Update BMC IP addresses
   - Update credentials
   - Update network configuration
3. **Apply the manifest:**
   ```bash
   kubectl apply -f examples/minimal-test-cluster.yaml
   ```
4. **Monitor progress:**
   ```bash
   kubectl get physicalhost,beskar7machine,beskar7cluster -w
   ```

## Customization Guide

### BMC Configuration

Update these fields in PhysicalHost resources:

```yaml
spec:
  redfishConnection:
    address: "https://YOUR_BMC_IP"
    credentialsSecretRef: "your-credentials-secret"
    insecureSkipVerify: true  # Use false in production
```

### OS Selection

Beskar7 supports the following immutable Linux distributions:

**Kairos (Recommended):**
```yaml
spec:
  osFamily: "kairos"
  imageURL: "https://github.com/kairos-io/kairos/releases/download/v2.8.1/kairos-alpine-v2.8.1-amd64.iso"
```

**Flatcar:**
```yaml
spec:
  osFamily: "flatcar"
  imageURL: "https://stable.release.flatcar-linux.net/amd64-usr/current/flatcar_production_iso_image.iso"
```

**openSUSE Leap Micro:**
```yaml
spec:
  osFamily: "LeapMicro"
  imageURL: "https://download.opensuse.org/distribution/leap-micro/5.5/appliances/openSUSE-Leap-Micro.x86_64-Default.iso"
```

### Provisioning Modes

**RemoteConfig Mode** (fetch config from URL):
```yaml
spec:
  provisioningMode: "RemoteConfig"
  configURL: "https://your-server.com/config.yaml"
```

**PreBakedISO Mode** (config baked into ISO):
```yaml
spec:
  provisioningMode: "PreBakedISO"
  imageURL: "https://your-server.com/custom-image.iso"
```

**PXE Mode** (network boot via PXE):
```yaml
spec:
  provisioningMode: "PXE"
  osFamily: "flatcar"
  imageURL: "http://pxe-server/flatcar.iso"  # For reference
```

**iPXE Mode** (network boot via iPXE):
```yaml
spec:
  provisioningMode: "iPXE"
  osFamily: "kairos"
  imageURL: "http://ipxe-server/boot.ipxe"
```

## Example Files

### Basic Examples

- **`minimal-test-cluster.yaml`** - Minimal single-node cluster for testing
- **`complete-cluster.yaml`** - Full multi-node cluster with HA control plane
- **`pxe-simple-test.yaml`** - Simple PXE/iPXE testing without full cluster

### Network Boot Examples

- **`pxe-simple-test.yaml`** - Simple PXE/iPXE test without full cluster
- **`pxe-provisioning-example.yaml`** - Complete PXE-based cluster deployment
- **`ipxe-provisioning-example.yaml`** - Complete iPXE-based cluster deployment

### Network Boot Documentation

- **`pxe-ipxe-prerequisites.md`** - ⚠️ **Start here!** Complete infrastructure requirements
- **`PXE_QUICK_START.md`** - 5-minute quick start guide
- **`PXE_TESTING_GUIDE.md`** - Comprehensive testing and troubleshooting guide

## Example Workflows

### Testing Workflow

1. Start with `minimal-test-cluster.yaml`
2. Verify PhysicalHost detection
3. Monitor provisioning progress
4. Test machine lifecycle (create, update, delete)

### Production Workflow

1. Review `complete-cluster.yaml`
2. Customize for your environment
3. Deploy control plane first
4. Scale workers incrementally
5. Implement monitoring and alerting

## Troubleshooting

### PhysicalHost Issues

```bash
# Check PhysicalHost status
kubectl describe physicalhost <name>

# View controller logs
kubectl logs -n beskar7-system deployment/controller-manager -c manager -f
```

### Machine Provisioning Issues

```bash
# Check Beskar7Machine status
kubectl describe beskar7machine <name>

# Check events
kubectl get events --sort-by='.lastTimestamp'
```

### Common Problems

| Problem | Solution |
|---------|----------|
| PhysicalHost stuck in "Initializing" | Check BMC credentials and network connectivity |
| Machine not claiming host | Verify cluster labels match between resources |
| Boot failures | Check ISO URL accessibility and BIOS settings |
| Network issues | Verify control plane endpoint and network config |

See the [Troubleshooting Guide](../docs/troubleshooting.md) for detailed solutions.

## Additional Resources

- **[Quick Start Guide](../docs/quick-start.md)** - Getting started with Beskar7
- **[API Reference](../docs/api-reference.md)** - Complete API documentation
- **[Architecture](../docs/architecture.md)** - Understanding Beskar7 design
- **[Vendor Support](../docs/vendor-specific-support.md)** - Vendor-specific configurations
- **[Best Practices](../docs/deployment-best-practices.md)** - Production deployment guide

## Contributing

Have a great example to share? Please:
1. Create a new example file
2. Add documentation (markdown file)
3. Update this README
4. Submit a pull request

Examples should be:
- ✅ Well-documented
- ✅ Production-ready (or clearly marked as dev/test)
- ✅ Following security best practices
- ✅ Including cleanup instructions

## License

All examples are provided under the Apache License 2.0, same as the Beskar7 project.

