# Beskar7MachineTemplate

The Beskar7MachineTemplate custom resource defines a template for creating Beskar7Machine resources. It provides a way to define reusable machine configurations that are validated and enforced by admission webhooks.

## API Version

`infrastructure.cluster.x-k8s.io/v1beta1`

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
- **Validation**: Must be a valid URL with supported schemes (http, https, file, ftp)
- **Supported formats**: .iso, .img, .qcow2, .vmdk, .raw, .vhd, .vhdx, .ova, .ovf and compressed variants

##### spec.osFamily
- **osFamily** (string, required): The operating system family to use. Must be one of:
  - `kairos` - Kairos cloud-native OS
  - `talos` - Talos Linux
  - `flatcar` - Flatcar Container Linux
  - `LeapMicro` - openSUSE Leap Micro
  - `ubuntu` - Ubuntu Server
  - `rhel` - Red Hat Enterprise Linux
  - `centos` - CentOS
  - `fedora` - Fedora Server
  - `debian` - Debian
  - `opensuse` - openSUSE

### Optional Fields

##### spec.configURL
- **configURL** (string, optional): URL of the configuration to use for the machine
- **Validation**: Must be a valid URL pointing to a configuration file (.yaml, .yml, .json, .toml, .conf, .cfg, .ini, .properties)
- **Required for**: RemoteConfig provisioning mode
- **Forbidden for**: PreBakedISO provisioning mode

##### spec.provisioningMode
- **provisioningMode** (string, optional, default: "RemoteConfig"): The mode to use for provisioning. Must be one of:
  - `RemoteConfig` - Boot generic ISO with configuration URL (requires configURL)
  - `PreBakedISO` - Boot pre-configured ISO (configURL not allowed)
  - `PXE` - PXE boot (future implementation)
  - `iPXE` - iPXE boot (future implementation)

## Webhook Validation

Beskar7MachineTemplate resources are validated by admission webhooks that enforce:

### Creation Validation
- **Template validation**: Reuses all Beskar7Machine validation logic
- **ProviderID restriction**: `providerID` cannot be set in templates (it's managed by the controller)
- **Cross-field validation**: Ensures configURL is provided when required and forbidden when not allowed

### Update Validation (Immutability)
Templates are **immutable after creation** to ensure consistency for machines created from the template:

- `imageURL` cannot be changed
- `osFamily` cannot be changed 
- `provisioningMode` cannot be changed
- `configURL` cannot be changed

**Rationale**: Changing template specifications could affect existing machines or cause inconsistencies in machine provisioning.

### Defaulting
- Applies the same defaulting logic as Beskar7Machine webhooks
- Ensures consistent behavior across all machine-related resources

## Example

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
      configURL: "https://config.example.com/worker.yaml"
      provisioningMode: "RemoteConfig"
```

## Common Use Cases

### 1. Control Plane Template
```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: Beskar7MachineTemplate
metadata:
  name: control-plane-template
  namespace: cluster-system
spec:
  template:
    spec:
      imageURL: "https://releases.kairos.io/v2.8.1/kairos-alpine-v2.8.1-amd64.iso"
      osFamily: "kairos"
      configURL: "https://config.example.com/control-plane.yaml"
      provisioningMode: "RemoteConfig"
```

### 2. Worker Node Template  
```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: Beskar7MachineTemplate
metadata:
  name: worker-template
  namespace: cluster-system
spec:
  template:
    spec:
      imageURL: "https://releases.kairos.io/v2.8.1/kairos-alpine-v2.8.1-amd64.iso"
      osFamily: "kairos"
      configURL: "https://config.example.com/worker.yaml" 
      provisioningMode: "RemoteConfig"
```

### 3. Pre-baked ISO Template
```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: Beskar7MachineTemplate
metadata:
  name: prebaked-template
  namespace: cluster-system
spec:
  template:
    spec:
      imageURL: "https://storage.example.com/custom-worker-v1.0.iso"
      osFamily: "ubuntu"
      provisioningMode: "PreBakedISO"
      # Note: configURL is not allowed for PreBakedISO mode
```

## Integration with Cluster API

Beskar7MachineTemplate is typically used by Cluster API resources:

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: KubeadmControlPlane
metadata:
  name: my-cluster-control-plane
spec:
  machineTemplate:
    infrastructureRef:
      apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
      kind: Beskar7MachineTemplate
      name: control-plane-template
      namespace: cluster-system
  # ... other KubeadmControlPlane fields
---
apiVersion: cluster.x-k8s.io/v1beta1
kind: MachineDeployment
metadata:
  name: my-cluster-workers
spec:
  template:
    spec:
      infrastructureRef:
        apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
        kind: Beskar7MachineTemplate
        name: worker-template
        namespace: cluster-system
  # ... other MachineDeployment fields
```

## Troubleshooting

### Common Validation Errors

1. **Invalid imageURL**: 
   ```
   Error: "imageURL should point to a valid image file"
   ```
   Ensure the URL points to a supported image format.

2. **Unsupported osFamily**:
   ```
   Error: "osFamily 'windows' is not supported"
   ```
   Use one of the supported OS families listed above.

3. **Immutability violation**:
   ```
   Error: "imageURL is immutable in machine templates"
   ```
   Template specifications cannot be changed after creation. Create a new template instead.

4. **ProviderID in template**:
   ```
   Error: "providerID should not be set in machine templates"
   ```
   Remove the providerID field from the template specification.

## Controller Behavior

The Beskar7MachineTemplate controller provides automated lifecycle management for template resources:

### Finalizer Management
- Automatically adds the `beskar7machinetemplate.infrastructure.cluster.x-k8s.io` finalizer on creation
- Manages proper cleanup when templates are deleted

### Reference Tracking
- **Deletion Protection**: Templates cannot be deleted while machines still reference them
- **Automatic Detection**: Controller watches Beskar7Machine resources for owner references
- **Graceful Cleanup**: Prevents orphaned machines by blocking template deletion until all references are removed

### Template Validation
- **Runtime Validation**: Controller performs additional validation beyond webhook checks
- **Integrity Checks**: Ensures ImageURL and OSFamily are properly specified
- **Error Reporting**: Validation failures are logged and reported through controller status

### Event Watching
The controller automatically responds to:
- **Machine Creation/Deletion**: Updates reference tracking when machines are created/destroyed
- **Template Changes**: Immediately validates and processes template modifications
- **Cleanup Events**: Triggers reference checks when machines or templates are deleted

### Metrics Integration
- **Reconciliation Metrics**: Tracks successful/failed reconciliations with timing data
- **Error Tracking**: Records error types and frequencies for monitoring
- **Performance Monitoring**: Provides duration metrics for operations

### Pause Support
Templates support the standard Cluster API pause annotation:
```yaml
metadata:
  annotations:
    cluster.x-k8s.io/paused: "true"
```
When paused, the controller skips reconciliation until the annotation is removed.

### Best Practices

- **Version your templates**: Use descriptive names with versions (e.g., `worker-template-v1.2`)
- **Test before deployment**: Validate templates in development environments
- **Create new templates for changes**: Since templates are immutable, create new versions instead of trying to modify existing ones
- **Use appropriate provisioning modes**: Choose RemoteConfig for flexibility or PreBakedISO for speed
- **Validate URLs**: Ensure imageURL and configURL are accessible from your cluster nodes
- **Monitor references**: Use `kubectl describe` to check which machines reference a template before deletion
- **Clean deletion**: Ensure all machines using a template are deleted before removing the template 