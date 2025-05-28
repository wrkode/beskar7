# Beskar7MachineTemplate

The Beskar7MachineTemplate custom resource defines a template for creating Beskar7Machine resources. It provides a way to define reusable machine configurations.

## API Version

`infrastructure.cluster.x-k8s.io/v1alpha1`

## Kind

`Beskar7MachineTemplate`

## Short Name

`b7mt`

## Namespaced

Yes

## Categories

- cluster-api

## Specification

### Required Fields

#### template
- **spec** (object, required): The template specification for the machine

##### spec.imageURL
- **imageURL** (string, required): URL of the machine image to use

##### spec.osFamily
- **osFamily** (string, required): The operating system family to use. Must be one of:
  - `kairos`
  - `talos`
  - `flatcar`
  - `LeapMicro`

### Optional Fields

##### spec.configURL
- **configURL** (string, optional): URL of the configuration to use for the machine

##### spec.providerID
- **providerID** (string, optional): Provider-specific identifier for the machine

##### spec.provisioningMode
- **provisioningMode** (string, optional): The mode to use for provisioning. Must be one of:
  - `RemoteConfig`
  - `PreBakedISO`

## Example

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: Beskar7MachineTemplate
metadata:
  name: example-template
  namespace: default
spec:
  template:
    spec:
      imageURL: "https://example.com/images/machine-image.iso"
      osFamily: "talos"
      configURL: "https://example.com/configs/machine-config.yaml"
      providerID: "beskar7://machine-123"
      provisioningMode: "RemoteConfig"
``` 