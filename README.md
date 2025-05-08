# Beskar7: Cluster API Infrastructure Provider for Immutable Bare Metal

Beskar7 is a Kubernetes operator that implements the Cluster API infrastructure provider contract for managing bare-metal machines using the Redfish API. It allows you to provision and manage the lifecycle of Kubernetes clusters on physical hardware directly through Kubernetes-native APIs.

## Current Status

**Alpha:** This project is currently under active development. Key features are being implemented, and the APIs may change. Not yet suitable for production use.

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

    # Build and push the image (uses values from Makefile: ghcr.io/wrkode/beskar7:v0.1.0-dev)
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

### Using Pre-built Manifests (Recommended)

1.  **Ensure prerequisites are met:** `kubectl` configured for your target cluster.
2.  **Install CRDs:**
    ```bash
    make install
    ```
3.  **Deploy the Controller Manager:**
    This will deploy the controller using the image defined in the Makefile (`ghcr.io/wrkode/beskar7:v0.1.0-dev` by default).
    ```bash
    make deploy
    ```
    *(Note: If you pushed the image to a different location, you MUST either override `IMG` in the Makefile and re-run `make manifests` before `make deploy`, or manually edit the deployment manifest in `config/manager/manager.yaml` before running `make deploy`.)*

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
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
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

### 3. Create a `Beskar7Machine` (Pre-Baked ISO Mode)

This example assumes you have an ISO image (`http://example.com/my-kairos-prebaked.iso`) that has Kairos OS and its configuration already embedded using Kairos's own tooling.

**Important Note for Pre-Baked ISO Mode:**
When using `provisioningMode: "PreBakedISO"`, you are responsible for ensuring the ISO specified in `imageURL` is:
1.  **Self-sufficient:** It must contain all necessary OS installation files and configuration for a complete, unattended installation.
2.  **Bootable:** It must be bootable in the desired firmware mode (Legacy BIOS or UEFI) of your target hardware. Beskar7 will set the virtual media as the boot target, but the ISO itself must handle the rest.

<details>
<summary>Example: `b7machine-prebaked.yaml`</summary>

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
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
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
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

*(This README will be updated as more features are implemented, including CAPI `Machine` and `Cluster` examples.)*

## Future Work / Roadmap

The following key areas are planned or in progress:

*   [x] Basic `PhysicalHost` reconciliation (Redfish connection, status update).
*   [x] Basic `Beskar7Machine` reconciliation (Host claiming, status monitoring based on host).
*   [x] `Beskar7Machine` deletion/finalizer handling (releasing the `PhysicalHost`).
*   [x] BDD Testing setup (`envtest`, Ginkgo/Gomega).
*   [x] Basic UserData handling (`Beskar7Machine` spec changes for OS-specific remote config).
*   [x] Implement `PhysicalHost` Deprovisioning (Power off, eject media on delete).
*   [x] Initial `SetBootParameters` implementation in Redfish client (UEFI target attempt).
*   [x] Basic `Beskar7Cluster` reconciliation (handles finalizer and `ControlPlaneEndpointReady` based on spec).
*   [x] Refine Status Reporting (CAPI Conditions for Beskar7Machine, PhysicalHost, Beskar7Cluster types and basic `Status.Ready` logic).
*   [ ] **`SetBootParameters` Full Implementation:** Robustly handle setting boot parameters via Redfish across various BMCs, investigating `UefiTargetBootSourceOverride`, BIOS attributes, and other vendor-specific mechanisms. This is crucial for reliable "RemoteConfig" provisioning.
*   [ ] **`Beskar7Cluster` Enhancements:**
    *   Derive `ControlPlaneEndpoint` in `Status` from control plane `Beskar7Machine`s (this will first require `Beskar7MachineStatus` to include IP address information).
    *   Implement `FailureDomains` reporting in `Beskar7ClusterStatus` if applicable to the target bare-metal environments.
    *   Add comprehensive tests for `Beskar7ClusterReconciler`.
*   [ ] **Testing & Validation:**
    *   Comprehensive BDD Tests for all controllers and provisioning modes (especially "RemoteConfig" error cases and different OS families).
    *   Real-world testing with a variety of physical hardware and Redfish implementations.
*   [ ] **Documentation:** Advanced Usage, Troubleshooting, Contribution Guidelines.

## Contributing

Contributions are welcome! Please refer to the contribution guidelines (to be added).

## Some testing notes

```
export KUBEBUILDER_ASSETS=$(/Users/wrizzo/go/bin/setup-envtest use 1.29.x -p path)
go clean -testcache && go test ./controllers/... -v -ginkgo.v
```
