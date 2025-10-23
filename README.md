# Beskar7: Cluster API Infrastructure Provider for Immutable Bare Metal

Beskar7 is a Kubernetes operator that implements the Cluster API infrastructure provider contract for managing bare-metal machines using the Redfish API. It allows you to provision and manage the lifecycle of Kubernetes clusters on physical hardware directly through Kubernetes-native APIs.

## **Automatic Vendor-Specific Hardware Support**

Beskar7 now automatically detects and handles vendor-specific hardware quirks! **Dell, HPE, Lenovo, and Supermicro systems** with zero configuration. (until bugs are found :D ) (**automatic detection is still experimental**)

- **Dell PowerEdge:** Automatic BIOS attribute handling (testing advised following microcode upgrades)
- **HPE ProLiant:** UEFI Target Boot Override
- **Lenovo ThinkSystem:** UEFI with intelligent BIOS fallback
- **Supermicro:** UEFI and BIOS-attribute methods (fallback behavior depends on BMC)

**[Quick Start Guide →](docs/quick-start-vendor-support.md)** | **[Detailed Documentation →](docs/vendor-specific-support.md)**

## Current Status

**Alpha:** This project is currently under active development. Key features are being implemented, and the APIs may change. Not yet suitable for production use.

### Supported Features

- **Provisioning Modes**: RemoteConfig, PreBakedISO, PXE, iPXE
- **OS Families**: Kairos (recommended), Flatcar, openSUSE Leap Micro
- **Boot Modes**: UEFI (recommended), Legacy BIOS
- **Vendor Support**: Dell, HPE, Lenovo, Supermicro (automatic detection experimental)
- ✅ **Hardware Management**: Power control, boot configuration, status monitoring
- ✅ **Cluster API Integration**: Full CAPI provider implementation

To prepare for real hardware testing, ensure you configure reconciliation timeouts via flags or env (see `docs/state-management.md`) and follow the testing instructions below.

## Documentation

Comprehensive documentation is available in the [`docs/`](docs/) directory:

- **[Getting Started](docs/README.md)** - Complete documentation index and navigation
- **[Quick Start Guide](docs/quick-start.md)** - Get up and running quickly
- **[API Reference](docs/api-reference.md)** - Complete API documentation
- **[Hardware Compatibility](docs/hardware-compatibility.md)** - Vendor support matrix
- **[PXE/iPXE Setup Guide](examples/pxe-ipxe-prerequisites.md)** - Infrastructure requirements for network boot
- **[Deployment Best Practices](docs/deployment-best-practices.md)** - Production deployment guidance
- **[Troubleshooting](docs/troubleshooting.md)** - Common issues and solutions

## Architecture Overview

Beskar7 consists of several custom controllers that work together:

*   **`PhysicalHost` Controller:** Manages individual bare-metal hosts discovered via Redfish. It handles Redfish connections, monitors host status (power, health), and performs low-level actions like setting boot devices and powering the host on/off. It exposes the host's state (`Available`, `Provisioning`, `Provisioned`, `Error`, etc.).
*   **`Beskar7Machine` Controller:** Represents the infrastructure for a specific Cluster API `Machine`. It finds an available `PhysicalHost`, claims it, configures its boot (ISO URL, kernel parameters for specific OS families), monitors the host's provisioning progress, and updates the `Machine` object with the `providerID` and readiness status once the host is provisioned.
*   **`Beskar7Cluster` Controller:** Represents the infrastructure for a Cluster API `Cluster`. It is responsible for coordinating cluster-level infrastructure, potentially managing load balancers or setting the `ControlPlaneEndpoint` based on the provisioned control plane `Beskar7Machine` resources.

## Prerequisites

### Development Prerequisites

*   [Go](https://golang.org/dl/) (version 1.25 or later required)
*   [Docker](https://docs.docker.com/get-docker/) (for envtest)
*   [controller-gen](https://book.kubebuilder.io/reference/controller-gen.html) (`make install-controller-gen`)
*   [kustomize](https://kubectl.docs.kubernetes.io/installation/kustomize/) (v4 or later for `make deploy`)

### Runtime Prerequisites

*   A running Kubernetes cluster (e.g., kind, minikube, or a remote cluster) with `kubectl` configured
*   Kubernetes 1.31+
*   Helm 3.2.0+ (for Helm installation method)
*   **Cluster API v1.4.0+ (REQUIRED)** - See installation below
*   **cert-manager (REQUIRED)** - See installation below

### Install Cluster API (Required)

**⚠️ IMPORTANT:** Beskar7 is a Cluster API Infrastructure Provider and requires Cluster API core components to be installed first.

**Option 1: Using clusterctl (Recommended)**
```bash
# Install clusterctl
curl -L https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.10.0/clusterctl-linux-amd64 -o clusterctl
chmod +x clusterctl
sudo mv clusterctl /usr/local/bin/

# Initialize Cluster API
clusterctl init
```

**Option 2: Manual Installation**
```bash
# Install CAPI core components
kubectl apply -f https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.10.0/cluster-api-components.yaml

# Install bootstrap provider (kubeadm)
kubectl apply -f https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.10.0/bootstrap-components.yaml

# Install control plane provider (kubeadm)
kubectl apply -f https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.10.0/control-plane-components.yaml

# Wait for components to be ready
kubectl wait --for=condition=Available --timeout=300s deployment/capi-controller-manager -n capi-system
kubectl wait --for=condition=Available --timeout=300s deployment/capi-kubeadm-bootstrap-controller-manager -n capi-kubeadm-bootstrap-system
kubectl wait --for=condition=Available --timeout=300s deployment/capi-kubeadm-control-plane-controller-manager -n capi-kubeadm-control-plane-system
```

Verify CAPI installation:
```bash
kubectl get pods -n capi-system
kubectl get pods -n capi-kubeadm-bootstrap-system
kubectl get pods -n capi-kubeadm-control-plane-system
```

### Install cert-manager (Required)

Beskar7 requires cert-manager to be installed in your cluster to manage webhook TLS certificates. Install cert-manager and its CRDs before deploying Beskar7:

```bash
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.16.2/cert-manager.crds.yaml
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.16.2/cert-manager.yaml
```

Wait for all cert-manager pods to be running:

```bash
kubectl wait --for=condition=Available --timeout=300s deployment/cert-manager -n cert-manager
kubectl wait --for=condition=Available --timeout=300s deployment/cert-manager-webhook -n cert-manager
kubectl wait --for=condition=Available --timeout=300s deployment/cert-manager-cainjector -n cert-manager
```

## Getting Started

1.  **Clone the repository:**
    ```bash
    git clone https://github.com/wrkode/beskar7.git
    cd beskar7 
    ```

2.  **Install Development Tools:**
    ```bash
    make install-controller-gen
    ```

3.  **Build and Push Container Image (Required for deployment):**
    You need to push the manager image to a container registry accessible by your Kubernetes cluster.
    ```bash
    # Login to GitHub Container Registry (or your chosen registry)
    # export CR_PAT=YOUR_GITHUB_PAT # Use a PAT with write:packages scope
    # echo $CR_PAT | docker login ghcr.io -u USERNAME --password-stdin

    # Build and push the image (uses values from Makefile: ghcr.io/wrkode/beskar7/beskar7:${VERSION})
    make docker-build docker-push 
    ```
    *(Note: If using a different registry/repo/tag, override Makefile variables: `make docker-push IMG=my-registry/my-repo:my-tag`)*

4.  **Generate Code & Manifests (If you made code changes):**
    ```bash
    make manifests
    ```

5.  **Build the Manager (Local Binary - Optional):**
    ```bash
    make build
    ```

6.  **Run Tests:**
    ```bash
    make test
    ```

## Installation / Deployment

### Using Helm

> **📝 Note for Repository Maintainers**: The Helm chart repository is automatically published to GitHub Pages via the `helm-publish.yml` workflow when a new tag is pushed. However, GitHub Pages must be manually enabled in the repository settings:
> 1. Go to **Settings → Pages**
> 2. Under **Build and deployment**, set **Source** to "Deploy from a branch"
> 3. Select branch: **gh-pages**, folder: **/ (root)**
> 4. Click **Save**
> 
> After enabling, wait 1-2 minutes for GitHub Pages to deploy. The workflow will then automatically update the Helm repository on every new release.

#### Add the Helm repository

```bash
helm repo add beskar7 https://wrkode.github.io/beskar7
helm repo update
```

#### Install the chart

```bash
# Install with default values
helm install beskar7 beskar7/beskar7 --namespace beskar7-system --create-namespace

# Install with custom values
helm install beskar7 beskar7/beskar7 -f values.yaml --namespace beskar7-system --create-namespace

# Wait for deployment to be ready
kubectl wait --for=condition=available --timeout=600s deployment/beskar7-controller-manager -n beskar7-system
```

### Manual Deployment using Kustomize:

This provides more control if you need to customize the deployment.

1.  **Build and push the manager image** as described in "Getting Started" step 3.
2.  **Install CRDs:**
    ```bash
    make install
    ```
3.  **Apply Base Manifests using Kustomize:**
    Navigate to the directory containing the checked-out code.
    ```bash
    # Apply the default configuration (ensure IMG in Makefile is correct or customize)
    kustomize build config/default | kubectl apply -f -
    ```
    *Alternatively, create your own Kustomize overlay pointing to `config/default` and set the image there.* 

### Deploying from a Release Manifest Bundle:

Each GitHub release will include a `beskar7-manifests-$(VERSION).yaml` file. This bundle contains all necessary CRDs, RBAC, and the Deployment for the controller manager, pre-configured with the correct image for that release.

**Important:** You must install cert-manager and its CRDs before applying the Beskar7 manifest bundle. See the "Install cert-manager (Required)" section above.

1.  **Download the release manifest** (e.g., `beskar7-manifests-${VERSION}.yaml`) from the [GitHub Releases page](https://github.com/wrkode/beskar7/releases).
2.  **Apply the manifest to your cluster:**
    ```bash
    kubectl apply -f beskar7-manifests-${VERSION}.yaml
    ```
    This will create the `beskar7-system` namespace and all required Beskar7 components.

## Usage Examples

### 1. Create Redfish Credentials Secret

First, create a Kubernetes Secret containing the username and password for your Redfish BMC.

<details>
<summary>Example: `redfish-credentials-secret.yaml`</summary>

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-bmc-credentials
  namespace: default # Or your target namespace
stringData:
  username: "your-bmc-username"
  password: "your-bmc-password"
```
</details>

```bash
kubectl apply -f redfish-credentials-secret.yaml
```

### 2. Create a `PhysicalHost` Resource

This resource tells Beskar7 about a physical server it can manage.

<details>
<summary>Example: `physicalhost.yaml`</summary>

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: PhysicalHost
metadata:
  name: server-01
  namespace: default
spec:
  redfishConnection:
    address: "https://192.168.1.123" # Replace with your BMC IP/hostname
    credentialsSecretRef: "my-bmc-credentials"
    # insecureSkipVerify: true # Optional: use for self-signed certs, not recommended for production
```
</details>

```bash
kubectl apply -f physicalhost.yaml
```

After a short while, the `PhysicalHost` should transition to an `Available` state if the connection is successful:
```bash
kubectl get physicalhost server-01 -o wide
```

If the PhysicalHost doesn't reach `Available` state, see the **[Troubleshooting Guide](docs/troubleshooting.md)** for common issues and solutions.

### 3. Create a `Beskar7Machine` (Multiple Provisioning Modes)

Beskar7 supports four provisioning modes. Choose the one that fits your infrastructure:

#### Mode 1: Pre-Baked ISO (Self-Contained)

Use this when you have an ISO with OS and configuration pre-built.

<details>
<summary>Example: `b7machine-prebaked.yaml`</summary>

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: Beskar7Machine
metadata:
  name: node-01
  namespace: default
spec:
  osFamily: "kairos"  # kairos, flatcar, or LeapMicro
  imageURL: "http://example.com/my-kairos-prebaked.iso"
  provisioningMode: "PreBakedISO"
  bootMode: "UEFI"  # UEFI (recommended) or Legacy
```
</details>

#### Mode 2: Remote Config (Dynamic Configuration)

Use this with a generic ISO and external configuration URL.

<details>
<summary>Example: `b7machine-remoteconfig.yaml`</summary>

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: Beskar7Machine
metadata:
  name: node-02
  namespace: default
spec:
  osFamily: "kairos"  # kairos, flatcar, or LeapMicro
  imageURL: "https://github.com/kairos-io/kairos/releases/download/v2.8.1/kairos-alpine-v2.8.1-amd64.iso"
  provisioningMode: "RemoteConfig"
  configURL: "https://your-server.com/kairos-config.yaml"  # Required for RemoteConfig
  bootMode: "UEFI"
```
</details>

**Configuration URL Parameters by OS:**
- **Kairos**: `config_url=<ConfigURL>`
- **Flatcar**: `flatcar.ignition.config.url=<ConfigURL>`
- **Leap Micro**: `combustion.path=<ConfigURL>`

#### Mode 3: PXE Boot (Traditional Network Boot)

Use this with existing PXE infrastructure.

<details>
<summary>Example: `b7machine-pxe.yaml`</summary>

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: Beskar7Machine
metadata:
  name: node-03
  namespace: default
spec:
  osFamily: "flatcar"
  imageURL: "http://pxe-server.example.com/flatcar.iso"  # Reference
  provisioningMode: "PXE"
  bootMode: "UEFI"
```
</details>

**Prerequisites:** DHCP, TFTP, and PXE infrastructure must be configured. See [PXE Setup Guide](examples/pxe-ipxe-prerequisites.md).

#### Mode 4: iPXE Boot (Modern Network Boot)

Use this with iPXE infrastructure for faster, HTTP-based boot.

<details>
<summary>Example: `b7machine-ipxe.yaml`</summary>

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: Beskar7Machine
metadata:
  name: node-04
  namespace: default
spec:
  osFamily: "kairos"
  imageURL: "http://ipxe-server.example.com/boot.ipxe"  # iPXE script URL
  provisioningMode: "iPXE"
  bootMode: "UEFI"
```
</details>

**Prerequisites:** DHCP and HTTP server with iPXE scripts. See [iPXE Setup Guide](examples/pxe-ipxe-prerequisites.md).

### 4. Complete Examples

For complete cluster deployments with multiple nodes, see the [`examples/`](examples/) directory:

- **[Minimal Test Cluster](examples/minimal-test-cluster.yaml)** - Single-node testing
- **[Complete Cluster](examples/complete-cluster.yaml)** - Multi-node with HA
- **[PXE Provisioning](examples/pxe-provisioning-example.yaml)** - Full PXE cluster
- **[iPXE Provisioning](examples/ipxe-provisioning-example.yaml)** - Full iPXE cluster
- **[Examples README](examples/README.md)** - Overview of all examples

## Hardware Compatibility

### Supported Hardware Vendors

Beskar7 supports any Redfish-compliant BMC with **automatic vendor detection**:

- **Dell Technologies** (iDRAC9+) - Full support with automatic BIOS attribute handling
- **HPE** (iLO 5+) - Full support with UEFI Target Boot Override  
- **Lenovo** (XCC) - Full support with intelligent BIOS fallback
- **Supermicro** (BMC) - Good support, X12+ series recommended

### Supported Operating Systems

Beskar7 supports the following immutable OS families:

| OS Family | Provisioning Modes | Status |
|-----------|-------------------|--------|
| **Kairos** (Alpine, Ubuntu) | All modes | ✅ Recommended |
| **Flatcar Container Linux** | All modes | ✅ Fully supported |
| **openSUSE Leap Micro** | All modes | ✅ Fully supported |

**Note:** Traditional Linux distributions (Ubuntu, RHEL, CentOS, etc.) are not currently supported. Only immutable, cloud-native OS families with built-in provisioning mechanisms are supported.

For detailed compatibility information, hardware-specific workarounds, and testing procedures, see the **[Hardware Compatibility Matrix](docs/hardware-compatibility.md)**.

## Production Deployment

For production deployments, review:

- **[Deployment Best Practices](docs/deployment-best-practices.md)** - Security, scaling, and operational guidance
- **[Troubleshooting Guide](docs/troubleshooting.md)** - Common issues and vendor-specific solutions
- **[Metrics Documentation](docs/metrics.md)** - Monitoring and observability setup

## Contributing

Contributions are welcome! For information about contributing to Beskar7, see the issue tracker at https://github.com/wrkode/beskar7/issues.

## Project Status and Roadmap

For detailed information about the project's current status and future plans, please see the **[GitHub Issues](https://github.com/wrkode/beskar7/issues)** and **[GitHub Projects](https://github.com/wrkode/beskar7/projects)** pages.

## Running Tests

Before running the tests, you need to download the required CRDs and set up envtest assets. Unit tests and controller tests run without real hardware; integration and emulation tests can be run with build tags.

```bash
# Download test CRDs (once)
./hack/download-test-crds.sh

# Set up envtest assets (Kubernetes API server binaries)
export KUBEBUILDER_ASSETS=$(go run sigs.k8s.io/controller-runtime/tools/setup-envtest@latest use 1.31.x -p path)

# Run unit and controller tests
go test ./controllers/... -v -ginkgo.v --timeout=10m
go test ./internal/... -v --timeout=10m

# Run integration tests (envtest) — excludes emulation unless tagged
go test -tags=integration ./test/integration/... -v --timeout=30m

# Optional: Run hardware emulation tests (no real hardware required)
# Note: The emulation tests use a mock Redfish server and skip TLS verification.
go test -tags=integration ./test/emulation/... -v --timeout=30m
```

The test setup is designed to be portable and work across different systems. All required CRDs are downloaded locally and referenced from the repository, ensuring consistent test behavior across different environments.

## Uninstalling

### Using Helm

```bash
helm uninstall beskar7 --namespace beskar7-system
```

### Manual Uninstallation

1. **Undeploy the controller**

   ```bash
   make undeploy
   ```

2. **Uninstall CRDs**

   ```bash
   make uninstall
   ```

## Upgrading

### Using Helm

```bash
helm upgrade beskar7 beskar7/beskar7 --namespace beskar7-system
```

## Quick Reference

### Provisioning Modes

| Mode | Use Case | Infrastructure Required |
|------|----------|------------------------|
| **PreBakedISO** | Pre-configured ISO | HTTP/HTTPS server for ISO hosting |
| **RemoteConfig** | Generic ISO + config URL | HTTP/HTTPS server for ISO and config |
| **PXE** | Traditional network boot | DHCP, TFTP, PXE infrastructure |
| **iPXE** | Modern network boot | DHCP, HTTP, iPXE infrastructure |

### Supported OS Families

- **kairos** - Cloud-native, immutable OS (recommended)
- **flatcar** - Container-optimized Linux
- **LeapMicro** - openSUSE Leap Micro

### Key Resources

- **PhysicalHost** - Represents a bare-metal server
- **Beskar7Machine** - CAPI Machine infrastructure
- **Beskar7Cluster** - CAPI Cluster infrastructure
- **Beskar7MachineTemplate** - Template for machine configs

### Getting Help

- 📖 [Complete Documentation](docs/README.md)
- 🐛 [Issue Tracker](https://github.com/wrkode/beskar7/issues)
- 💬 [Discussions](https://github.com/wrkode/beskar7/discussions)
- 📚 [Examples](examples/)

## Recent Updates

### v0.3.4-alpha (October 23, 2025)

- ✅ **PXE/iPXE Support**: Full network boot provisioning modes
- ✅ **Boot Mode Control**: UEFI and Legacy BIOS support
- ✅ **Enhanced Testing**: All skipped tests fixed and passing
- ✅ **Documentation**: Complete alignment with implementation
- ✅ **OS Support**: Focused on proven immutable OS families (kairos, flatcar, LeapMicro)
- ✅ **Hardware Matching**: Label-based host selection implemented

See [CHANGELOG.md](CHANGELOG.md) for complete release notes and breaking changes.

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.
