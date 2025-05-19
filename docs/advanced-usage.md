# Advanced Usage

This document covers more advanced configuration options and usage scenarios for Beskar7.

## Provisioning Modes

Beskar7 supports two primary provisioning modes controlled by the `spec.provisioningMode` field on the `Beskar7Machine` resource:

*   **`RemoteConfig` (Default if `spec.configURL` is set):**
    *   Requires `spec.osFamily`, `spec.imageURL` (pointing to a generic OS installer ISO), and `spec.configURL` (pointing to an OS configuration file like Kairos YAML, Flatcar Ignition JSON, Talos MachineConfig YAML, or Combustion script).
    *   Beskar7 attempts to configure the BMC to boot the generic ISO and inject kernel parameters (`config_url=...`, `flatcar.ignition.config.url=...`, etc.) so the booting OS can fetch its configuration from the specified `configURL`.
    *   **Reliability:** This mode depends heavily on the target BMC's Redfish implementation supporting the `UefiTargetBootSourceOverride` method for setting boot parameters. Success may vary across hardware vendors.
*   **`PreBakedISO` (Default if `spec.configURL` is *not* set):**
    *   Requires `spec.osFamily` and `spec.imageURL`.
    *   The `spec.imageURL` must point to an ISO image that has *already* been customized (using the target OS's native tooling, e.g., `kairos-agent build iso ...`) to include all necessary configuration for an unattended installation.
    *   Beskar7 simply instructs the BMC to boot from this provided ISO without injecting any extra parameters.
    *   The user is responsible for ensuring the pre-baked ISO is bootable (BIOS/UEFI) and self-sufficient.

## Vendor-Specific Boot Configuration (Future Work)

Currently, the `RemoteConfig` mode relies on the somewhat standard `UefiTargetBootSourceOverride` Redfish mechanism. If this fails for specific hardware, future versions of Beskar7 might support vendor-specific methods, potentially configured via annotations on the `PhysicalHost` resource.

**Example (Conceptual):**

If a Dell BMC requires setting the `KernelArgs` BIOS attribute instead of using `UefiTargetBootSourceOverride`, a user might apply an annotation like this:

```yaml
# physicalhost.yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: PhysicalHost
metadata:
  name: dell-server-01
  namespace: default
  annotations:
    # Tell Beskar7 to use the 'KernelArgs' BIOS attribute for boot params
    beskar7.infrastructure.cluster.x-k8s.io/bios-kernel-arg-attribute: "KernelArgs"
spec:
  # ... rest of spec ...
```

*Note: This annotation and the logic to handle it are **not yet implemented**.* It represents a potential future direction for handling hardware incompatibility with the default boot parameter method.

## IP Address Handling

Beskar7 provides enhanced IP address management through the `Beskar7Machine` status. Each machine's IP addresses are categorized and tracked:

```yaml
status:
  ipAddresses:
    internalIPs: ["192.168.1.10", "10.0.0.1"]  # Internal network IPs
    externalIPs: ["1.1.1.1", "2.2.2.2"]        # External/public IPs
    preferredIP: "192.168.1.10"                 # Preferred IP for this machine
```

The IP addresses are automatically populated from the CAPI Machine's addresses, with the following behavior:
- Internal IPs are preferred over external IPs
- The first internal IP is set as the preferred IP
- If no internal IPs are available, the first external IP is used as the preferred IP
- The original CAPI Machine addresses are preserved for compatibility

## Control Plane Endpoint

The control plane endpoint is automatically derived from the control plane machines in the following order:

1. Use the preferred IP from the Beskar7Machine status if available
2. Fall back to the first internal IP from the Beskar7Machine status
3. Use the first external IP from the Beskar7Machine status if no internal IPs are available
4. Finally, fall back to the original CAPI Machine addresses if no IPs are found in the Beskar7Machine status

The endpoint is always set to port 6443 (the default Kubernetes API server port).

Example of a derived endpoint:
```yaml
status:
  controlPlaneEndpoint:
    host: "192.168.1.10"  # Derived from the preferred IP
    port: 6443
```

## Failure Domains

Cluster API utilizes failure domains (often corresponding to availability zones, racks, etc.) for workload scheduling and resilience.

*   **Discovery:** The `Beskar7Cluster` controller attempts to discover available failure domains by listing `PhysicalHost` resources in the same namespace.
*   **Labeling:** By default, it looks for the standard Kubernetes label `topology.kubernetes.io/zone` on the `PhysicalHost` resources. This can be customized using the `spec.failureDomainLabels` field in the `Beskar7Cluster` resource to specify multiple labels in order of preference.
*   **Status:** Unique zone values found are populated into the `Beskar7Cluster`'s `status.failureDomains` map.

To use failure domains, ensure your `PhysicalHost` resources are labeled appropriately:

```yaml
# physicalhost-rack1.yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: PhysicalHost
metadata:
  name: server-rack1-01
  namespace: default
  labels:
    topology.kubernetes.io/zone: "rack-1" # Default zone label
spec:
  # ... rest of spec ...
```

To use custom labels for failure domains:

```yaml
# beskar7cluster.yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: Beskar7Cluster
metadata:
  name: my-cluster
  namespace: default
spec:
  failureDomainLabels:  # Multiple labels in order of preference
    - "custom.zone"     # Primary label
    - "backup.zone"     # Fallback label
  # ... rest of spec ...

---
# physicalhost-rack1.yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: PhysicalHost
metadata:
  name: server-rack1-01
  namespace: default
  labels:
    custom.zone: "rack-1"    # Primary label
    backup.zone: "rack-1-b"  # Fallback label
spec:
  # ... rest of spec ...
```

Note: When using custom failure domain labels:
- Labels are checked in order, and the first matching label is used
- Label names must follow Kubernetes label format (lowercase alphanumeric with dots and hyphens)
- A maximum of 5 labels can be specified
- At least one label must be specified
- Ensure all `PhysicalHost` resources in the cluster use consistent labeling

## Redfish Client Configuration

The Redfish client can be configured through environment variables and environment-specific configuration files. For detailed configuration options, see [Configuration](configuration.md).

Key configuration options include:

* `BESKAR7_REDFISH_SCHEME` - URL scheme for Redfish endpoints (default: https)
* `BESKAR7_REDFISH_PORT` - Port for Redfish endpoints
* `BESKAR7_REDFISH_TIMEOUT` - Timeout for Redfish operations
* `BESKAR7_RETRY_*` - Various retry parameters for Redfish operations

Example configuration:
```sh
export BESKAR7_REDFISH_TIMEOUT=30s
export BESKAR7_RETRY_MAX_ATTEMPTS=5
export BESKAR7_RETRY_INITIAL_INTERVAL=1s
```

For more configuration options and environment-specific settings, refer to the [Configuration](configuration.md) documentation. 