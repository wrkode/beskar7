# Introduction to Beskar7

Welcome to Beskar7, a Cluster API (CAPI) Infrastructure Provider designed for managing bare-metal Kubernetes clusters using the Redfish standard.

## What is Beskar7?

Beskar7 acts as a bridge between the declarative Kubernetes API and the physical hardware managed via Redfish BMCs (Baseboard Management Controllers). It allows you to:

*   Define your bare-metal infrastructure (`PhysicalHost` resources) within Kubernetes.
*   Provision Kubernetes nodes onto these physical hosts using `Beskar7Machine` resources, which integrate with CAPI `Machine` objects.
*   Leverage Kubernetes-native OSes like Kairos, Talos, Flatcar, and openSUSE MicroOS using their specific provisioning methods (Remote Configuration via kernel parameters or Pre-Baked ISOs).
*   Orchestrate cluster-level infrastructure using `Beskar7Cluster` resources.

## Why Beskar7?

Managing bare-metal infrastructure traditionally involves manual steps or separate automation tools. Beskar7 brings this management into the Kubernetes ecosystem, enabling consistent, declarative lifecycle management using familiar CAPI workflows.

## Target Audience

This project is primarily aimed at platform administrators, SREs, and DevOps teams who manage Kubernetes clusters running directly on physical hardware and utilize servers with Redfish-compliant BMCs.

## Current Status

Beskar7 is currently in the **Alpha** stage. Core functionality is being developed, and APIs or implementation details may change. It is not yet recommended for production deployments.

## Next Steps

*   **[Quick Start](./quick-start.md):** Get Beskar7 built and deployed.
*   **[Architecture](./architecture.md):** Understand the components and how they interact. 