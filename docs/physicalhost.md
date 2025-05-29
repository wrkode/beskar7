# PhysicalHost

The `PhysicalHost` resource represents a physical host in the Beskar7 infrastructure provider.

## API Version

`infrastructure.cluster.x-k8s.io/v1beta1`

## Kind

`PhysicalHost`

## Short Name

`ph`

## Namespaced

Yes

## Specification

### Required Fields

#### redfishConnection
- **address** (string, required): URL of the Redfish API endpoint (e.g., https://192.168.1.100)
- **credentialsSecretRef** (string, required): Reference to a Secret containing username and password for Redfish authentication
- **insecureSkipVerify** (boolean, optional): Whether to skip TLS certificate verification

### Optional Fields

#### consumerRef
Reference to the Beskar7Machine that is using this host. Contains standard Kubernetes object reference fields:
- **apiVersion** (string)
- **kind** (string)
- **name** (string)
- **namespace** (string)
- **fieldPath** (string)
- **resourceVersion** (string)
- **uid** (string)

#### bootIsoSource
- **bootIsoSource** (string, optional): URL of the ISO image to use for provisioning. Set by the consumer (Beskar7Machine controller) to trigger provisioning.

#### userDataSecretRef
- **name** (string, optional): Reference to a secret containing cloud-init user data

## Status

### ready
- **ready** (boolean, default: false): Indicates if the host is ready and enrolled

### state
- **state** (string): Current provisioning state, which can be one of:
  - `""` (empty string) - StateNone: Default state before reconciliation
  - `"Enrolling"` - StateEnrolling: Controller is trying to establish connection
  - `"Available"` - StateAvailable: Host is ready to be claimed
  - `"Claimed"` - StateClaimed: Host is reserved by a consumer
  - `"Provisioning"` - StateProvisioning: Host is being configured
  - `"Provisioned"` - StateProvisioned: Host has been successfully configured
  - `"Deprovisioning"` - StateDeprovisioning: Host is being cleaned up
  - `"Error"` - StateError: Host is in an error state
  - `"Unknown"` - StateUnknown: Host state could not be determined

### observedPowerState
- **observedPowerState** (string): Last observed power state from Redfish endpoint

### errorMessage
- **errorMessage** (string): Details on the last error encountered

### hardwareDetails
- **manufacturer** (string): Manufacturer of the physical host
- **model** (string): Model of the physical host
- **serialNumber** (string): Serial number of the physical host
- **status** (object):
  - **Health** (string): Health status of the host
  - **HealthRollup** (string): Overall health status
  - **State** (string): Current state of the host

### conditions
Array of conditions representing the latest available observations of the object's state.

## Additional Printer Columns

- **State**: Current state of the Physical Host
- **Ready**: Whether the host is ready
- **Age**: Creation timestamp

## Example

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: PhysicalHost
metadata:
  name: my-host
  namespace: default
spec:
  redfishConnection:
    address: "https://192.168.1.100"
    credentialsSecretRef: "redfish-credentials"
    insecureSkipVerify: false
  bootIsoSource: "http://example.com/boot.iso"
  userDataSecretRef:
    name: "user-data"
status:
  ready: true
  state: "Available"
  observedPowerState: "On"
  hardwareDetails:
    manufacturer: "Example Corp"
    model: "Server-123"
    serialNumber: "SN123456"
    status:
      Health: "OK"
      HealthRollup: "OK"
      State: "Enabled"
  addresses:
    - type: InternalIP
      address: 192.168.1.100
```

## Fields

| Field | Type | Description |
|-------|------|-------------|
| `spec.redfishConnection` | `RedfishConnection` | The Redfish connection configuration. |
| `spec.bootIsoSource` | `string` | The URL of the ISO image to use for provisioning. |
| `spec.userDataSecretRef` | `ObjectReference` | Reference to a secret containing cloud-init user data. |
| `status.ready` | `boolean` | Indicates if the host is ready and enrolled. |
| `status.state` | `string` | The current state of the host. |
| `status.observedPowerState` | `string` | The last observed power state from the Redfish endpoint. |
| `status.hardwareDetails` | `HardwareDetails` | Details about the hardware of the physical host. |
| `status.errorMessage` | `string` | Error message if the host is in an error state. |
| `status.addresses` | `[]MachineAddress` | The associated addresses for the host. | 