# API Reference

This document provides comprehensive reference documentation for all Beskar7 Custom Resource Definitions (CRDs).

## API Groups and Versions

Beskar7 defines resources in the `infrastructure.cluster.x-k8s.io` API group with version `v1beta1`.

**API Group:** `infrastructure.cluster.x-k8s.io`  
**Version:** `v1beta1`  
**Categories:** `cluster-api`

## Resource Overview

| Resource | Kind | Short Name | Purpose |
|----------|------|------------|---------|
| PhysicalHost | `PhysicalHost` | `ph` | Represents a physical server manageable via Redfish |
| Beskar7Machine | `Beskar7Machine` | - | Infrastructure provider for CAPI Machine resources |
| Beskar7Cluster | `Beskar7Cluster` | - | Infrastructure provider for CAPI Cluster resources |
| Beskar7MachineTemplate | `Beskar7MachineTemplate` | - | Template for creating Beskar7Machine resources |

## PhysicalHost

Represents a physical server that can be managed via Redfish BMC.

### API Definition

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: PhysicalHost
```

### Specification Fields

#### spec.redfishConnection

**Type:** `RedfishConnection` (required)

Connection details for the Redfish BMC endpoint.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `address` | string | Yes | URL of the Redfish service (e.g., `https://192.168.1.100`) |
| `credentialsSecretRef` | string | Yes | Name of the Secret containing Redfish credentials |
| `insecureSkipVerify` | boolean | No | Skip TLS certificate verification (default: false) |

**Validation:**
- `address` must match pattern: `^(https?://)[a-zA-Z0-9.-]+(:[0-9]+)?(/.*)?$`
- `credentialsSecretRef` minimum length: 1

#### spec.consumerRef

**Type:** `ObjectReference` (optional)

Reference to the Beskar7Machine using this host. Set automatically by the Beskar7Machine controller.

#### spec.bootIsoSource

**Type:** `string` (optional)

URL of the ISO image for provisioning. Set by the consuming Beskar7Machine controller.

#### spec.userDataSecretRef

**Type:** `ObjectReference` (optional)

Reference to a Secret containing cloud-init user data.

**Status:** Currently accepted but not yet integrated into the provisioning process. This field is validated and can be set, but user data injection is pending implementation. Future versions will integrate this with OS-specific provisioning methods (cloud-init for Kairos, Ignition for Flatcar, Combustion for LeapMicro).

### Status Fields

#### status.ready

**Type:** `boolean`

Indicates if the host is ready and enrolled.

#### status.state

**Type:** `string`

Current provisioning state. Possible values:

| State | Description |
|-------|-------------|
| `""` (empty) | Initial state before reconciliation |
| `Enrolling` | Controller establishing connection |
| `Available` | Host ready to be claimed |
| `Claimed` | Host reserved by a consumer |
| `Provisioning` | Host being configured |
| `Provisioned` | Host successfully configured |
| `Deprovisioning` | Host being cleaned up |
| `Error` | Host in error state |
| `Unknown` | State could not be determined |

#### status.observedPowerState

**Type:** `string`

Last observed power state from Redfish endpoint.

#### status.errorMessage

**Type:** `string`

Details about any error encountered.

#### status.hardwareDetails

**Type:** `HardwareDetails`

Information about the physical hardware.

| Field | Type | Description |
|-------|------|-------------|
| `manufacturer` | string | Hardware manufacturer |
| `model` | string | Hardware model |
| `serialNumber` | string | Hardware serial number |
| `status.health` | string | Health status |
| `status.healthRollup` | string | Overall health status |
| `status.state` | string | Hardware state |

#### status.addresses

**Type:** `[]MachineAddress`

Network addresses associated with the host.

#### status.conditions

**Type:** `[]Condition`

Current service state conditions.

### Example

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: PhysicalHost
metadata:
  name: server-01
  namespace: default
  labels:
    topology.kubernetes.io/zone: "rack-1"
spec:
  redfishConnection:
    address: "https://192.168.1.100"
    credentialsSecretRef: "bmc-credentials"
    insecureSkipVerify: false
status:
  ready: true
  state: "Available"
  observedPowerState: "On"
  hardwareDetails:
    manufacturer: "Dell Inc."
    model: "PowerEdge R750"
    serialNumber: "ABC123DEF"
    status:
      health: "OK"
      healthRollup: "OK"
      state: "Enabled"
  addresses:
  - type: InternalIP
    address: "192.168.1.100"
  conditions:
  - type: RedfishConnectionReady
    status: "True"
    reason: "Connected"
    message: "Successfully connected to Redfish endpoint"
```

## Beskar7Machine

Infrastructure provider resource for CAPI Machine objects.

### API Definition

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: Beskar7Machine
```

### Specification Fields

#### spec.providerID

**Type:** `string` (optional)

Unique identifier set by the infrastructure provider.

#### spec.imageURL

**Type:** `string` (required)

URL of the OS image to use for the machine.

**Validation:**
- Must match pattern for valid image files: `.iso`, `.img`, `.qcow2`, `.vmdk`, `.raw`, `.vhd`, `.vhdx`, `.ova`, `.ovf`
- Supports compressed formats: `.gz`, `.bz2`, `.xz`, `.zip`, `.tar`, `.tgz`, `.tbz2`, `.txz`
- Must use supported schemes: `http`, `https`, `ftp`, `file`

#### spec.osFamily

**Type:** `string` (required)

Operating system family to use.

**Valid Values:**
- `kairos` - Kairos cloud-native OS (recommended)
- `flatcar` - Flatcar Container Linux
- `LeapMicro` - openSUSE Leap Micro

**Note:** Only the OS families listed above are currently supported with full RemoteConfig provisioning capabilities. Each OS family has specific kernel parameter requirements for configuration URL passing.

#### spec.configURL

**Type:** `string` (optional)

URL of the configuration file for the machine.

**Validation:**
- Must match pattern for configuration files: `.yaml`, `.yml`, `.json`, `.toml`, `.conf`, `.cfg`, `.ini`, `.properties`
- Must use supported schemes: `http`, `https`, `file`

#### spec.provisioningMode

**Type:** `string` (optional, default: "RemoteConfig")

Mode to use for provisioning the machine.

**Valid Values:**
- `RemoteConfig` - Boot generic ISO with configuration URL
- `PreBakedISO` - Boot pre-configured ISO. Use this when the OS config is embedded into the ISO for `kairos`, `talos`, `flatcar`, or `LeapMicro`.
- `PXE` - PXE boot (future)
- `iPXE` - iPXE boot (future)

**Cross-field Validation:**
- `configURL` is required when `provisioningMode` is `RemoteConfig`
- `configURL` should not be set when `provisioningMode` is `PreBakedISO`

### Status Fields

#### status.ready

**Type:** `boolean`

Indicates if the machine is ready.

#### status.phase

**Type:** `string`

Current phase of the machine lifecycle.

#### status.failureReason

**Type:** `string`

Reason for any terminal failure.

#### status.failureMessage

**Type:** `string`

Detailed message about any terminal failure.

#### status.addresses

**Type:** `[]MachineAddress`

Network addresses for the machine.

#### status.conditions

**Type:** `[]Condition`

Current service state conditions.

**Standard Conditions:**
- `InfrastructureReady` - Infrastructure is ready
- `PhysicalHostAssociated` - Associated with a PhysicalHost

### Example

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: Beskar7Machine
metadata:
  name: control-plane-01
  namespace: default
  labels:
    cluster.x-k8s.io/cluster-name: "production-cluster"
    cluster.x-k8s.io/control-plane: ""
spec:
  imageURL: "https://releases.kairos.io/v2.8.1/kairos-alpine-v2.8.1-amd64.iso"
  osFamily: "kairos"
  provisioningMode: "RemoteConfig"
  configURL: "https://config.example.com/control-plane.yaml"
status:
  ready: true
  phase: "Running"
  addresses:
  - type: InternalIP
    address: "10.0.1.10"
  - type: ExternalIP
    address: "203.0.113.10"
  conditions:
  - type: InfrastructureReady
    status: "True"
    reason: "ProvisioningComplete"
    message: "Infrastructure is ready"
  - type: PhysicalHostAssociated
    status: "True"
    reason: "HostClaimed"
    message: "Successfully associated with PhysicalHost server-01"
```

## Beskar7Cluster

Infrastructure provider resource for CAPI Cluster objects.

### API Definition

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: Beskar7Cluster
```

### Specification Fields

#### spec.controlPlaneEndpoint

**Type:** `APIEndpoint` (optional)

Endpoint for the cluster's control plane API.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `host` | string | Yes | Hostname or IP address |
| `port` | int32 | Yes | Port number (1-65535) |

**Validation:**
- `host` must be valid IP address or hostname
- `port` must be between 1 and 65535

### Status Fields

#### status.ready

**Type:** `boolean`

Indicates if the cluster infrastructure is ready.

#### status.controlPlaneEndpoint

**Type:** `APIEndpoint`

Discovered or configured control plane endpoint.

#### status.failureDomains

**Type:** `FailureDomains`

Map of failure domain information discovered from PhysicalHost labels.

#### status.conditions

**Type:** `[]Condition`

Current service state conditions.

**Standard Conditions:**
- `ControlPlaneEndpointReady` - Control plane endpoint is available

### Example

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: Beskar7Cluster
metadata:
  name: production-cluster
  namespace: default
spec:
  controlPlaneEndpoint:
    host: "10.0.1.10"
    port: 6443
status:
  ready: true
  controlPlaneEndpoint:
    host: "10.0.1.10"
    port: 6443
  failureDomains:
    rack-1:
      controlPlane: true
      attributes:
        zone: "rack-1"
    rack-2:
      controlPlane: true
      attributes:
        zone: "rack-2"
  conditions:
  - type: ControlPlaneEndpointReady
    status: "True"
    reason: "EndpointDiscovered"
    message: "Control plane endpoint is ready"
```

## Beskar7MachineTemplate

Template for creating Beskar7Machine resources, used by CAPI MachineDeployment and other templating resources.

### API Definition

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: Beskar7MachineTemplate
```

### Specification Fields

#### spec.template

**Type:** `Beskar7MachineTemplateResource` (required)

Template for creating Beskar7Machine resources.

#### spec.template.spec

**Type:** `Beskar7MachineSpec` (required)

Specification for the Beskar7Machine (same fields as Beskar7Machine.spec).

### Example

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: Beskar7MachineTemplate
metadata:
  name: worker-template
  namespace: default
spec:
  template:
    spec:
      imageURL: "https://releases.kairos.io/v2.8.1/kairos-alpine-v2.8.1-amd64.iso"
      osFamily: "kairos"
      provisioningMode: "RemoteConfig"
      configURL: "https://config.example.com/worker.yaml"
```

## Common Types

### ObjectReference

Standard Kubernetes object reference.

| Field | Type | Description |
|-------|------|-------------|
| `apiVersion` | string | API version of the referenced object |
| `kind` | string | Kind of the referenced object |
| `name` | string | Name of the referenced object |
| `namespace` | string | Namespace of the referenced object |
| `uid` | string | UID of the referenced object |

### MachineAddress

Network address information.

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Type of address (InternalIP, ExternalIP, etc.) |
| `address` | string | The address value |

**Address Types:**
- `Hostname` - DNS hostname
- `ExternalIP` - Public IP address
- `InternalIP` - Private IP address
- `ExternalDNS` - External DNS name
- `InternalDNS` - Internal DNS name

### Condition

Standard Kubernetes condition type.

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Type of condition |
| `status` | string | Status of condition (True, False, Unknown) |
| `reason` | string | Machine-readable reason for condition |
| `message` | string | Human-readable message |
| `lastTransitionTime` | string | Time when condition last changed |
| `severity` | string | Severity of the condition |

## Usage Patterns

### Basic PhysicalHost Setup

1. **Create BMC credentials secret:**
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: bmc-credentials
  namespace: default
type: Opaque
stringData:
  username: "admin"
  password: "password123"
```

2. **Create PhysicalHost:**
```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: PhysicalHost
metadata:
  name: server-01
  namespace: default
spec:
  redfishConnection:
    address: "https://192.168.1.100"
    credentialsSecretRef: "bmc-credentials"
```

### Machine Provisioning Patterns

#### RemoteConfig Pattern

Used when you have a generic OS installer ISO and want to provide configuration via URL:

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: Beskar7Machine
metadata:
  name: worker-01
spec:
  imageURL: "https://releases.kairos.io/v2.8.1/kairos-alpine-v2.8.1-amd64.iso"
  osFamily: "kairos"
  provisioningMode: "RemoteConfig"
  configURL: "https://config.example.com/worker.yaml"
```

#### PreBakedISO Pattern

Used when you have a pre-configured ISO with all settings embedded:

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: Beskar7Machine
metadata:
  name: worker-02
spec:
  imageURL: "https://storage.example.com/custom-kairos-worker.iso"
  osFamily: "kairos"
  provisioningMode: "PreBakedISO"
```

### Cluster API Integration

Beskar7 resources are typically created by Cluster API controllers, not directly by users:

```yaml
# CAPI Cluster - creates Beskar7Cluster
apiVersion: cluster.x-k8s.io/v1beta1
kind: Cluster
metadata:
  name: production-cluster
spec:
  infrastructureRef:
    apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
    kind: Beskar7Cluster
    name: production-cluster

---
# CAPI Machine - creates Beskar7Machine
apiVersion: cluster.x-k8s.io/v1beta1
kind: Machine
metadata:
  name: control-plane-01
spec:
  infrastructureRef:
    apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
    kind: Beskar7Machine
    name: control-plane-01
```

## Validation and Constraints

### Admission Webhooks

All Beskar7 resources are protected by comprehensive admission webhooks that provide:

#### Validating Webhooks
- **Field validation**: URL formats, enum values, cross-field constraints
- **Security validation**: TLS certificate validation, credential strength requirements
- **Business logic validation**: Redfish connectivity, resource dependencies
- **Immutability enforcement**: Critical fields that cannot be changed after creation

#### Mutating Webhooks (Defaulting)
- **Automatic field defaulting**: Sets sensible defaults for optional fields
- **Consistent behavior**: Ensures all resources have complete specifications
- **Version compatibility**: Maintains compatibility across API versions

### Field Validation

All resources include comprehensive field validation via OpenAPI schemas and admission webhooks:

- **URL validation** for `imageURL` and `configURL` (with format and accessibility checks)
- **Enum validation** for `osFamily` and `provisioningMode` 
- **Pattern validation** for Redfish addresses and BMC endpoints
- **Cross-field validation** for provisioning mode constraints and dependencies
- **Security validation** for TLS settings and credential requirements

### Resource Constraints

#### PhysicalHost Constraints
- Must have unique Redfish addresses within a namespace
- Redfish connection parameters are validated on creation/update
- Credentials secret must exist and contain valid username/password
- TLS configuration is validated for production environments

#### Beskar7Machine Constraints  
- Can only claim available PhysicalHost resources
- ConfigURL is mandatory for RemoteConfig mode but forbidden for PreBakedISO mode
- ImageURL must point to supported image formats (.iso, .img, .qcow2, etc.)
- ProviderID is managed by controllers and cannot be set manually

#### Beskar7MachineTemplate Constraints
- **Immutable after creation**: imageURL, osFamily, provisioningMode, configURL cannot be changed
- ProviderID cannot be set in templates (managed by controllers)
- Inherits all validation rules from Beskar7Machine specifications
- Template changes require creating new template versions

### Status Transitions

Resources follow predictable state transitions:

**PhysicalHost States:**
```
None → Enrolling → Available → Claimed → Provisioning → Provisioned
                     ↓           ↓
                   Error ← → Deprovisioning → Available
```

**Beskar7Machine Conditions:**
```
PhysicalHostAssociated: False → True (when host is claimed)
InfrastructureReady: False → True (when provisioning completes)
```

This API reference provides comprehensive information for working with Beskar7 resources. For additional examples and usage scenarios, see the other documentation files. 