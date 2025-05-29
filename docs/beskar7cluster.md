# Beskar7Cluster

The `Beskar7Cluster` resource represents a cluster in the Beskar7 infrastructure provider.

## API Version

`infrastructure.cluster.x-k8s.io/v1beta1`

## Kind

`Beskar7Cluster`

## Short Name

`b7c`

## Namespaced

Yes

## Categories

- cluster-api

## Specification

### Required Fields

#### controlPlaneEndpoint
- **host** (string, required): The hostname or IP address of the control plane endpoint
- **port** (integer, required): The port number of the control plane endpoint

### Optional Fields

#### failureDomains
Map of failure domains with their attributes:
- **attributes** (map[string]string): Key-value pairs of domain attributes
- **controlPlane** (boolean): Whether this domain can host control plane nodes

## Status

### ready
- **ready** (boolean): Indicates if the cluster infrastructure is ready for bootstrapping

### failureDomains
Map of failure domains with their current status:
- **attributes** (map[string]string): Key-value pairs of domain attributes
- **controlPlane** (boolean): Whether this domain can host control plane nodes

## Additional Printer Columns

- **Cluster**: Cluster to which this Beskar7Cluster belongs
- **Endpoint**: Control Plane Endpoint Host
- **Ready**: Cluster infrastructure is ready for bootstrapping
- **Age**: Creation timestamp

## Example

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: Beskar7Cluster
metadata:
  name: my-cluster
  namespace: default
spec:
  controlPlaneEndpoint:
    host: 192.168.1.100
    port: 6443
```

## Fields

| Field | Type | Description |
|-------|------|-------------|
| `spec.controlPlaneEndpoint` | `APIEndpoint` | The endpoint used to communicate with the control plane. |
| `status.ready` | `bool` | Indicates that the cluster is ready. |
| `status.controlPlaneEndpoint` | `APIEndpoint` | The endpoint used to communicate with the control plane. |
| `status.failureDomains` | `FailureDomains` | A list of failure domain objects synced from the infrastructure provider. |
| `status.conditions` | `Conditions` | Current service state of the Beskar7Cluster. |

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: Beskar7Cluster
metadata:
  name: example-cluster
  namespace: default
  labels:
    cluster.x-k8s.io/cluster-name: example-cluster
spec:
  controlPlaneEndpoint:
    host: "192.168.1.100"
    port: 6443
  failureDomains:
    rack-1:
      attributes:
        rack: "1"
        row: "A"
      controlPlane: true
    rack-2:
      attributes:
        rack: "2"
        row: "A"
      controlPlane: false
status:
  ready: true
  failureDomains:
    rack-1:
      attributes:
        rack: "1"
        row: "A"
      controlPlane: true
    rack-2:
      attributes:
        rack: "2"
        row: "A"
      controlPlane: false
``` 