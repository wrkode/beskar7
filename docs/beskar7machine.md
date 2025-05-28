# Beskar7Machine

The Beskar7Machine custom resource represents a machine in a Beskar7-managed cluster. It provides the infrastructure configuration for individual machines in the cluster.

## API Version

`infrastructure.cluster.x-k8s.io/v1alpha1`

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
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: Beskar7Machine
metadata:
  name: example-machine
  namespace: default
  labels:
    cluster.x-k8s.io/cluster-name: example-cluster
spec:
  imageURL: "https://example.com/images/machine-image.iso"
  osFamily: "talos"
  configURL: "https://example.com/configs/machine-config.yaml"
  providerID: "beskar7://machine-123"
  provisioningMode: "RemoteConfig"
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