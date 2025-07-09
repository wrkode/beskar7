# Beskar7: Cluster API Infrastructure Provider for Immutable Bare Metal

Beskar7 is a Kubernetes operator that implements the Cluster API infrastructure provider contract for managing bare-metal machines using the Redfish API. It allows you to provision and manage the lifecycle of Kubernetes clusters on physical hardware directly through Kubernetes-native APIs.

## Current Status

**Alpha:** This project is currently under active development. Key features are being implemented, and the APIs may change. Not yet suitable for production use.

## 📚 Documentation

Comprehensive documentation is available in the [`docs/`](docs/) directory:

- **[Getting Started](docs/README.md)** - Complete documentation index and navigation
- **[Quick Start Guide](docs/quick-start.md)** - Get up and running quickly
- **[API Reference](docs/api-reference.md)** - Complete API documentation
- **[Hardware Compatibility](docs/hardware-compatibility.md)** - Vendor support matrix
- **[Deployment Best Practices](docs/deployment-best-practices.md)** - Production deployment guidance
- **[Troubleshooting](docs/troubleshooting.md)** - Common issues and solutions

## Architecture Overview

Beskar7 consists of several custom controllers that work together:

*   **`PhysicalHost` Controller:** Manages individual bare-metal hosts discovered via Redfish. It handles Redfish connections, monitors host status (power, health), and performs low-level actions like setting boot devices and powering the host on/off. It exposes the host's state (`Available`, `Provisioning`, `Provisioned`, `Error`, etc.).
*   **`Beskar7Machine` Controller:** Represents the infrastructure for a specific Cluster API `Machine`. It finds an available `PhysicalHost`, claims it, configures its boot (ISO URL, kernel parameters for specific OS families), monitors the host's provisioning progress, and updates the `Machine` object with the `providerID` and readiness status once the host is provisioned.
*   **`Beskar7Cluster` Controller:** Represents the infrastructure for a Cluster API `Cluster`. It is responsible for coordinating cluster-level infrastructure, potentially managing load balancers or setting the `ControlPlaneEndpoint` based on the provisioned control plane `Beskar7Machine` resources.

## Prerequisites

*   [Go](https://golang.org/dl/) (version 1.21 or later recommended)
*   [Docker](https://docs.docker.com/get-docker/) (for envtest)
*   [controller-gen](https://book.kubebuilder.io/reference/controller-gen.html) (`make install-controller-gen`)
*   [kustomize](https://kubectl.docs.kubernetes.io/installation/kustomize/) (v4 or later for `make deploy`)
*   A running Kubernetes cluster (e.g., kind, minikube, or a remote cluster) with `kubectl` configured.
*   Kubernetes 1.19+
*   Helm 3.2.0+
*   Cluster API v1.4.0+
*   **cert-manager (required, for webhook support and TLS certificates)**

### Install cert-manager (Required)

Beskar7 requires cert-manager to be installed in your cluster to manage webhook TLS certificates. Install cert-manager and its CRDs before deploying Beskar7:

```bash
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.12.0/cert-manager.crds.yaml
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.12.0/cert-manager.yaml
```

Wait for all cert-manager pods to be running:

```bash
kubectl get pods -n cert-manager
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

    # Build and push the image (uses values from Makefile: ghcr.io/wrkode/beskar7/beskar7:v0.2.0)
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

#### Add the Helm repository

```bash
helm repo add beskar7 https://wrkode.github.io/beskar7
helm repo update
```

#### Install the chart

```bash
# Install with default values
helm install beskar7 beskar7/beskar7

# Install with custom values
helm install beskar7 beskar7/beskar7 -f values.yaml

# Install in a specific namespace
helm install beskar7 beskar7/beskar7 --namespace beskar7-system --create-namespace
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

1.  **Download the release manifest** (e.g., `beskar7-manifests-v0.2.0.yaml`) from the [GitHub Releases page](https://github.com/wrkode/beskar7/releases).
2.  **Apply the manifest to your cluster:**
    ```bash
    kubectl apply -f beskar7-manifests-v0.2.0.yaml
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

### 3. Create a `Beskar7Machine` (Pre-Baked ISO Mode)

This example assumes you have an ISO image (`http://example.com/my-kairos-prebaked.iso`) that has Kairos OS and its configuration already embedded using Kairos's own tooling.

**Important Note for Pre-Baked ISO Mode:**
When using `provisioningMode: "PreBakedISO"`, you are responsible for ensuring the ISO specified in `imageURL` is:
1.  **Self-sufficient:** It must contain all necessary OS installation files and configuration for a complete, unattended installation.
2.  **Bootable:** It must be bootable in the desired firmware mode (Legacy BIOS or UEFI) of your target hardware. Beskar7 will set the virtual media as the boot target, but the ISO itself must handle the rest.

<details>
<summary>Example: `b7machine-prebaked.yaml`</summary>

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: Beskar7Machine
metadata:
  name: node-01
  namespace: default
  labels:
    cluster.x-k8s.io/cluster-name: "my-cluster" # CAPI Machine owner should set this
    cluster.x-k8s.io/role: "control-plane"      # Example role
spec:
  osFamily: "kairos"
  imageURL: "http://example.com/my-kairos-prebaked.iso" # URL to your pre-baked ISO
  provisioningMode: "PreBakedISO"
  # providerID will be set by the controller
```
</details>

```bash
kubectl apply -f b7machine-prebaked.yaml
```

### 4. Create a `Beskar7Machine` (Remote Config Mode - Kairos Example)

This example uses a generic Kairos installer ISO and provides a URL to a Kairos configuration file served over HTTPS.

<details>
<summary>Example: `b7machine-kairos-remote.yaml`</summary>

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: Beskar7Machine
metadata:
  name: node-02
  namespace: default
  labels:
    cluster.x-k8s.io/cluster-name: "my-cluster"
    cluster.x-k8s.io/role: "worker"
spec:
  osFamily: "kairos"
  imageURL: "https://github.com/kairos-io/kairos/releases/download/v2.8.1/kairos-alpine-v2.8.1-amd64.iso" # Generic Kairos ISO
  provisioningMode: "RemoteConfig"
  configURL: "https://your-server.com/path/to/kairos-config.yaml" # URL to your Kairos config file
  # providerID will be set by the controller
```
</details>

```bash
kubectl apply -f b7machine-kairos-remote.yaml
```

**Note on `configURL` for RemoteConfig:**
*   Ensure the URL is accessible from the bare-metal server during its boot process.
*   For Kairos, the parameter `config_url=<ConfigURL>` will be passed to the kernel.
*   For Talos, `talos.config=<ConfigURL>`.
*   For Flatcar, `flatcar.ignition.config.url=<ConfigURL>`.
*   For openSUSE Leap Micro, `combustion.path=<ConfigURL>`.

## Hardware Compatibility

Beskar7 supports any Redfish-compliant BMC. Tested vendors include:

- **Dell Technologies** (iDRAC9) - Excellent support with minor RemoteConfig limitations
- **HPE** (iLO 5) - Excellent Redfish compliance and feature support  
- **Lenovo** (XCC) - Good overall compatibility with reliable boot parameter injection
- **Supermicro** (BMC) - Variable support, newer X12+ series recommended

For detailed compatibility information, hardware-specific workarounds, and testing procedures, see the **[Hardware Compatibility Matrix](docs/hardware-compatibility.md)**.

## Production Deployment

For production deployments, review:

- **[Deployment Best Practices](docs/deployment-best-practices.md)** - Security, scaling, and operational guidance
- **[Troubleshooting Guide](docs/troubleshooting.md)** - Common issues and vendor-specific solutions
- **[Metrics Documentation](docs/metrics.md)** - Monitoring and observability setup

## Contributing

For information about contributing to Beskar7, see the `CONTRIBUTING.md` file and the issue tracker at https://github.com/wrkode/beskar7/issues.

## Project Status and Roadmap

For detailed information about the project's current status and future plans, please see our [ROADMAP.md](ROADMAP.md) file.

## Contributing

Contributions are welcome! Please refer to the contribution guidelines (to be added).

## Running Tests

Before running the tests, you need to download the required CRDs:

```bash
# Download test CRDs
./hack/download-test-crds.sh

# Run the tests
export KUBEBUILDER_ASSETS=$(go run sigs.k8s.io/controller-runtime/tools/setup-envtest@latest use 1.31.x -p path)
go test ./controllers/... -v -ginkgo.v
```

The test setup is designed to be portable and work across different systems. All required CRDs are downloaded locally and referenced from the repository, ensuring consistent test behavior across different environments.

## Uninstalling

### Using Helm

```bash
helm uninstall beskar7
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
helm upgrade beskar7 beskar7/beskar7
```

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.
