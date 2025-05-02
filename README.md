# Beskar7: Cluster API Infrastructure Provider for Bare Metal

Beskar7 is a Kubernetes operator that implements the Cluster API infrastructure provider contract for managing bare-metal machines using the Redfish API. It allows you to provision and manage the lifecycle of Kubernetes clusters on physical hardware directly through Kubernetes-native APIs.

## Current Status

**Under Development:** This project is currently under active development. Key features are being implemented, and the APIs may change. Not yet suitable for production use.

## Architecture Overview

Beskar7 consists of several custom controllers that work together:

*   **`PhysicalHost` Controller:** Manages individual bare-metal hosts discovered via Redfish. It handles Redfish connections, monitors host status (power, health), and performs low-level actions like setting boot devices and powering the host on/off. It exposes the host's state (`Available`, `Provisioning`, `Provisioned`, `Error`, etc.).
*   **`Beskar7Machine` Controller:** Represents the infrastructure for a specific Cluster API `Machine`. It finds an available `PhysicalHost`, claims it, provides the boot ISO URL (potentially customized with user data), monitors the host's provisioning progress, and updates the `Machine` object with the `providerID` and readiness status once the host is provisioned.
*   **`Beskar7Cluster` Controller:** Represents the infrastructure for a Cluster API `Cluster`. It is responsible for coordinating cluster-level infrastructure, potentially managing load balancers or setting the `ControlPlaneEndpoint` based on the provisioned control plane `Beskar7Machine` resources.

## Prerequisites

*   [Go](https://golang.org/dl/) (version 1.21 or later recommended)
*   [Docker](https://docs.docker.com/get-docker/) (for envtest)
*   [controller-gen](https://book.kubebuilder.io/reference/controller-gen.html) (`make install-controller-gen`)
*   [kustomize](https://kubectl.docs.kubernetes.io/installation/kustomize/) (v3 or later)

## Getting Started

1.  **Clone the repository:**
    ```bash
    git clone <repository-url>
    cd projectbeskar/beskar7
    ```

2.  **Install Development Tools:**
    ```bash
    make install-controller-gen
    ```

3.  **Generate Code & Manifests:**
    ```bash
    make manifests
    # Optional, usually included in manifests: make generate
    ```

4.  **Build the Manager:**
    ```bash
    make build
    ```
    The manager binary will be located at `bin/manager`.

5.  **Run Tests:**
    ```bash
    make test # Runs unit/integration tests using envtest
    # OR individually: go test ./... -v -ginkgo.v
    ```

*(Instructions for running the controller locally or deploying to a cluster will be added later).*

## Future Work / Roadmap

The following key areas are planned or in progress:

*   [x] Basic `PhysicalHost` reconciliation (Redfish connection, status update).
*   [x] Basic `Beskar7Machine` reconciliation (Host claiming, status monitoring based on host).
*   [x] `Beskar7Machine` deletion/finalizer handling (releasing the `PhysicalHost`).
*   [x] BDD Testing setup (`envtest`, Ginkgo/Gomega).
*   [ ] Implement `PhysicalHost` Deprovisioning (Power off, eject media on delete).
*   [ ] Handle UserData (Secret reading, **ISO customization placeholder**).
*   [ ] Implement `Beskar7Cluster` Reconciliation (ControlPlaneEndpoint, status).
*   [ ] Refine Status Reporting (CAPI Conditions).
*   [ ] Comprehensive BDD Tests for all controllers.
*   [ ] Real-world testing with physical hardware and Redfish implementations.
*   [ ] Documentation (Deployment, Usage examples).

## Contributing

Contributions are welcome! Please refer to the contribution guidelines (to be added). 