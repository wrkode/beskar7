# Quick Start Guide

This guide provides the steps to get the Beskar7 controller manager built, deployed, and ready to manage `PhysicalHost` resources.

## Prerequisites

*   [Go](https://golang.org/dl/) (version 1.21 or later recommended)
*   [Docker](https://docs.docker.com/get-docker/) (for building the manager image)
*   `docker buildx` configured for multi-arch builds (if needed, e.g., Mac M1/M2 building for amd64):
    ```bash
    docker buildx create --use
    ```
*   [controller-gen](https://book.kubebuilder.io/reference/controller-gen.html) (`make install-controller-gen`)
*   [kustomize](https://kubectl.docs.kubernetes.io/installation/kustomize/) (v4 or later)
*   A running Kubernetes cluster (e.g., kind, minikube, or a remote cluster) with `kubectl` configured.
*   Access to a container registry (like ghcr.io, Docker Hub, etc.) where the manager image can be pushed.
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

## Getting Started Steps

1.  **Clone the repository:**
    ```bash
    git clone https://github.com/wrkode/beskar7.git
    cd beskar7 
    ```

2.  **Install Development Tools:**
    ```bash
    make install-controller-gen
    ```

3.  **Build and Push Container Image:**
    You need to push the manager image to a container registry accessible by your Kubernetes cluster.
    ```bash
    # Login to your container registry (e.g., GitHub Container Registry)
    # export CR_PAT=YOUR_GITHUB_PAT # Use a PAT with write:packages scope for GHCR
    # echo $CR_PAT | docker login ghcr.io -u YOUR_GITHUB_USERNAME --password-stdin

    # Build and push the image 
    # (Uses values from Makefile: ghcr.io/wrkode/beskar7:v0.1.0-dev by default)
    # This builds for linux/amd64 by default due to Makefile configuration.
    make docker-build docker-push 
    ```
    *(Note: If using a different registry/repo/tag, override Makefile variables: `make docker-push IMG=my-registry/my-repo:my-tag`)*

4.  **Generate Code & Manifests (If you made code changes):**
    Run this after code changes, especially to API types or RBAC markers.
    ```bash
    make manifests
    ```

5.  **Run Tests (Optional but Recommended):**
    ```bash
    make test
    ```

## Installation / Deployment

1.  **Ensure prerequisites are met:** `kubectl` configured for your target cluster, the manager image pushed to an accessible registry, and **cert-manager installed** (see above).
2.  **Install CRDs:**
    ```bash
    make install
    ```
3.  **Deploy the Controller Manager:**
    This will deploy the controller using the image defined in the Makefile (`ghcr.io/wrkode/beskar7:v0.1.0-dev` by default).
    ```bash
    make deploy
    ```
    *(Note: If you pushed the image to a different location than specified in the Makefile, ensure the `IMG` variable was set correctly during the `make deploy` step, or modify the deployment manifests manually/via kustomize before applying).*

4.  **Verify Deployment:**
    Check that the controller manager pod is running in the `beskar7-system` namespace:
    ```bash
    kubectl get pods -n beskar7-system -l control-plane=controller-manager
    ```

## Basic Usage

See the main [README.md](../README.md#usage-examples) or the specific examples in the `docs` directory for creating `PhysicalHost` and `Beskar7Machine` resources. 