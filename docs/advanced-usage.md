# Advanced Usage

This document covers more advanced configuration options and usage scenarios for Beskar7.

## Provisioning Modes

Beskar7 supports two primary provisioning modes controlled by the `spec.provisioningMode` field on the `Beskar7Machine` resource:

*   **`RemoteConfig` (Default if `spec.configURL` is set):**
    *   Requires `spec.osFamily`, `spec.imageURL` (pointing to a generic OS installer ISO), and `spec.configURL` (pointing to an OS configuration file like Kairos YAML, Flatcar Ignition JSON, or openSUSE Combustion script).
    *   Beskar7 attempts to configure the BMC to boot the generic ISO and inject kernel parameters (`config_url=...` for Kairos, `flatcar.ignition.config.url=...` for Flatcar, `combustion.path=...` for LeapMicro) so the booting OS can fetch its configuration from the specified `configURL`.
    *   **Reliability:** This mode depends heavily on the target BMC's Redfish implementation supporting the `UefiTargetBootSourceOverride` method for setting boot parameters. Success may vary across hardware vendors.
    *   **Supported OS Families:** Currently only `kairos`, `flatcar`, and `LeapMicro` are supported with RemoteConfig mode.
*   **`PreBakedISO` (Default if `spec.configURL` is *not* set):**
    *   Requires `spec.osFamily` and `spec.imageURL`.
    *   The `spec.imageURL` must point to an ISO image that has *already* been customized (using the target OS's native tooling, e.g., `kairos-agent build iso ...`) to include all necessary configuration for an unattended installation.
    *   Beskar7 simply instructs the BMC to boot from this provided ISO without injecting any extra parameters.
    *   The user is responsible for ensuring the pre-baked ISO is bootable (BIOS/UEFI) and self-sufficient.

## Vendor-Specific Boot Configuration

Currently, the `RemoteConfig` mode relies on the somewhat standard `UefiTargetBootSourceOverride` Redfish mechanism. If this fails for specific hardware, future versions of Beskar7 might support vendor-specific methods, potentially configured via annotations on the `PhysicalHost` resource.

For detailed information about vendor-specific behavior and compatibility, see the **[Hardware Compatibility Matrix](./hardware-compatibility.md)**.

**Example (Conceptual):**

If a Dell BMC requires setting the `KernelArgs` BIOS attribute instead of using `UefiTargetBootSourceOverride`, a user might apply an annotation like this:

```yaml
# physicalhost.yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
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

## Failure Domains

Cluster API utilizes failure domains (often corresponding to availability zones, racks, etc.) for workload scheduling and resilience.

*   **Discovery:** The `Beskar7Cluster` controller attempts to discover available failure domains by listing `PhysicalHost` resources in the same namespace.
*   **Labeling:** It looks for the standard Kubernetes label `topology.kubernetes.io/zone` on the `PhysicalHost` resources.
*   **Status:** Unique zone values found are populated into the `Beskar7Cluster`'s `status.failureDomains` map.

To use failure domains, ensure your `PhysicalHost` resources are labeled appropriately:

```yaml
# physicalhost-rack1.yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: PhysicalHost
metadata:
  name: server-rack1-01
  namespace: default
  labels:
    topology.kubernetes.io/zone: "rack-1" # Assign zone label
spec:
  # ... rest of spec ...
```

## Redfish Client Configuration

*(To be added: Details on configuring timeouts or other Redfish client parameters, if such configuration options are implemented in the future.)*

For additional advanced topics, see:

*   **[Hardware Compatibility Matrix](./hardware-compatibility.md)** - Vendor-specific configurations and limitations
*   **[Troubleshooting Guide](./troubleshooting.md)** - Hardware/BMC interaction issues
*   **[Deployment Best Practices](./deployment-best-practices.md)** - Production configuration scenarios
*   **[API Reference](./api-reference.md)** - Complete field documentation and validation rules 