# Beskar7 Helm Chart

This Helm chart deploys Beskar7, a Bare Metal Infrastructure Provider for Cluster API.

## Prerequisites

- Kubernetes 1.19+
- Helm 3.2.0+
- Cluster API v1.4.0+
- cert-manager (optional, for webhook support)

## Installation

### Add the Helm repository

```bash
helm repo add beskar7 https://wrkode.github.io/beskar7
helm repo update
```

### Install the chart

```bash
# Install with default values
helm install beskar7 beskar7/beskar7

# Install with custom values
helm install beskar7 beskar7/beskar7 -f values.yaml

# Install in a specific namespace
helm install beskar7 beskar7/beskar7 --namespace beskar7-system --create-namespace
```

## Configuration

The following table lists the configurable parameters of the Beskar7 chart and their default values.

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of Beskar7 controller replicas | `1` |
| `image.repository` | Beskar7 controller image repository | `ghcr.io/wrkode/beskar7` |
| `image.tag` | Beskar7 controller image tag | `v0.2.0` |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `serviceAccount.create` | Create service account | `true` |
| `serviceAccount.name` | Service account name | `""` |
| `serviceAccount.annotations` | Service account annotations | `{}` |
| `rbac.create` | Create RBAC resources | `true` |
| `webhook.enabled` | Enable webhook | `true` |
| `webhook.certManager.enabled` | Enable cert-manager integration | `false` |
| `webhook.failurePolicy` | Webhook failure policy | `Fail` |
| `webhook.port` | Webhook port | `9443` |
| `prometheus.enabled` | Enable Prometheus metrics | `true` |
| `prometheus.serviceMonitor.enabled` | Enable ServiceMonitor | `true` |
| `prometheus.serviceMonitor.interval` | ServiceMonitor scrape interval | `10s` |
| `prometheus.serviceMonitor.scrapeTimeout` | ServiceMonitor scrape timeout | `5s` |
| `controller.redfish.defaultScheme` | Default Redfish scheme | `https` |
| `controller.redfish.defaultPort` | Default Redfish port | `443` |
| `controller.redfish.defaultTimeout` | Default Redfish timeout | `30s` |
| `controller.requeueInterval` | Default requeue interval | `15s` |
| `controller.requeueAfterError` | Requeue interval after error | `5m` |
| `controller.requeueAfterNoHost` | Requeue interval after no host | `1m` |
| `controller.retry.initialInterval` | Initial retry interval | `1s` |
| `controller.retry.maxInterval` | Maximum retry interval | `5m` |
| `controller.retry.multiplier` | Retry multiplier | `2.0` |
| `controller.retry.maxAttempts` | Maximum retry attempts | `5` |
| `controller.retry.maxElapsedTime` | Maximum retry elapsed time | `15m` |
| `controller.boot.defaultEfiPath` | Default EFI path | `\EFI\BOOT\BOOTX64.EFI` |
| `controller.boot.defaultOverrideEnabled` | Default boot override enabled | `Once` |
| `controller.boot.defaultOverrideTarget` | Default boot override target | `UefiTarget` |

## Usage

### Basic Installation

```bash
helm install beskar7 beskar7/beskar7
```

### With Webhook Support

```bash
helm install beskar7 beskar7/beskar7 \
  --set webhook.enabled=true \
  --set webhook.certManager.enabled=true
```

### With Custom Configuration

```bash
helm install beskar7 beskar7/beskar7 \
  --set controller.redfish.defaultTimeout=60s \
  --set controller.requeueInterval=30s
```

### With Prometheus Monitoring

```bash
helm install beskar7 beskar7/beskar7 \
  --set prometheus.enabled=true \
  --set prometheus.serviceMonitor.enabled=true
```

## Uninstalling the Chart

```bash
helm uninstall beskar7
```

## Upgrading the Chart

```bash
helm upgrade beskar7 beskar7/beskar7
```

## Contributing

Please refer to the [Contributing Guide](../../CONTRIBUTING.md) for details on our code of conduct and the process for submitting pull requests.

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](../../LICENSE) file for details. 