# Beskar7Machine

The `Beskar7Machine` resource represents a machine in the Beskar7 infrastructure provider.

## API Version

`infrastructure.cluster.x-k8s.io/v1beta1`

## Kind

`Beskar7Machine`

## Namespaced

Yes

## Specification

### Required Fields

#### imageURL
- **imageURL** (string, required): URL of the machine image to use

#### osFamily
- **osFamily** (string, required): The operating system family to use. Must be one of:
  - `kairos`
  - `talos`
  - `flatcar`
  - `LeapMicro`

### Optional Fields

#### configURL
- **configURL** (string, optional): URL of the configuration to use for the machine

#### providerID
- **providerID** (string, optional): Provider-specific identifier for the machine

#### provisioningMode
- **provisioningMode** (string, optional): The mode to use for provisioning. Must be one of:
  - `RemoteConfig`
  - `PreBakedISO`

## Status

### addresses
Array of machine addresses:
- **address** (string, required): The address value
- **type** (string, required): The type of address. Must be one of:
  - `Hostname`
  - `ExternalIP`
  - `InternalIP`
  - `ExternalDNS`
  - `InternalDNS`

### conditions
Array of conditions representing the latest available observations of the object's state:
- **lastTransitionTime** (string, required): Last time the condition transitioned
- **message** (string, required): Human-readable message indicating details about the transition
- **reason** (string, required): One-word CamelCase reason for the condition's last transition
- **severity** (string): Severity level of the condition
- **status** (string, required): Status of the condition
- **type** (string, required): Type of the condition

### failureMessage
- **failureMessage** (string): Error message describing any failure

### failureReason
- **failureReason** (string): Reason for any failure

### phase
- **phase** (string): Current phase of the machine

### ready
- **ready** (boolean): Indicates if the machine is ready

## Additional Printer Columns

- **Cluster**: Cluster to which this Beskar7Machine belongs
- **State**: Current state of the Beskar7Machine
- **Ready**: Machine ready status
- **ProviderID**: Provider ID
- **Machine**: Machine object which owns this Beskar7Machine
- **Age**: Creation timestamp

## Example

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: Beskar7Machine
metadata:
  name: my-machine
  namespace: default
spec:
  providerID: beskar7://default/my-host
  imageURL: http://example.com/image.qcow2
  configURL: http://example.com/config.yaml
  osFamily: ubuntu
  provisioningMode: image
status:
  addresses:
    - type: "InternalIP"
      address: "192.168.1.100"
    - type: "Hostname"
      address: "example-machine"
  conditions:
    - type: "Ready"
      status: "True"
      lastTransitionTime: "2024-01-01T00:00:00Z"
      reason: "MachineReady"
      message: "Machine is ready"
  phase: "Running"
  ready: true
```

## Fields

| Field | Type | Description |
|-------|------|-------------|
| `spec.providerID` | `string` | The unique identifier as specified by the cloud provider. |
| `spec.imageURL` | `string` | The URL of the OS image to use for the machine. |
| `spec.configURL` | `string` | The URL of the configuration to use for the machine. |
| `spec.osFamily` | `string` | The operating system family to use for the machine. |
| `spec.provisioningMode` | `string` | The mode to use for provisioning the machine. |
| `status.ready` | `bool` | Indicates that the machine is ready. |
| `status.addresses` | `[]MachineAddress` | The associated addresses for the machine. |
| `status.phase` | `string` | The current phase of machine actuation. |
| `status.failureReason` | `string` | A succinct value suitable for machine interpretation in case of terminal problems. |
| `status.failureMessage` | `string` | A more verbose string suitable for logging and human consumption in case of terminal problems. |
| `status.conditions` | `Conditions` | Current service state of the Beskar7Machine. | 