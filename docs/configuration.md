# Beskar7 Configuration

This document describes the configuration options available in Beskar7.

## Environment Variables

Beskar7 can be configured using environment variables. All configuration variables are prefixed with `BESKAR7_`.

### Redfish Configuration

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `BESKAR7_REDFISH_SCHEME` | Default URL scheme for Redfish endpoints | `https` | `https` |
| `BESKAR7_REDFISH_PORT` | Default port for Redfish endpoints | `443` | `443` |
| `BESKAR7_REDFISH_TIMEOUT` | Default timeout for Redfish operations | `30s` | `1m` |

### Controller Configuration

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `BESKAR7_CONTROLLER_REQUEUE_INTERVAL` | Default interval for requeuing reconciliation | `15s` | `30s` |
| `BESKAR7_CONTROLLER_REQUEUE_AFTER_ERROR` | Interval for requeuing after an error | `5m` | `10m` |
| `BESKAR7_CONTROLLER_REQUEUE_AFTER_NO_HOST` | Interval for requeuing when no host is found | `1m` | `2m` |

### Retry Configuration

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `BESKAR7_RETRY_INITIAL_INTERVAL` | Initial retry interval | `1s` | `2s` |
| `BESKAR7_RETRY_MAX_INTERVAL` | Maximum retry interval | `5m` | `10m` |
| `BESKAR7_RETRY_MULTIPLIER` | Factor to multiply the interval by for each retry | `2.0` | `1.5` |
| `BESKAR7_RETRY_MAX_ATTEMPTS` | Maximum number of retry attempts | `5` | `10` |
| `BESKAR7_RETRY_MAX_ELAPSED_TIME` | Maximum total time to retry | `15m` | `30m` |

### Boot Configuration

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `BESKAR7_BOOT_DEFAULT_EFI_PATH` | Default path to the EFI bootloader | `\EFI\BOOT\BOOTX64.EFI` | `/EFI/BOOT/BOOTX64.EFI` |
| `BESKAR7_BOOT_DEFAULT_OVERRIDE_ENABLED` | Default boot source override setting | `Once` | `Continuous` |
| `BESKAR7_BOOT_DEFAULT_OVERRIDE_TARGET` | Default boot source override target | `UefiTarget` | `Pxe` |

## Usage

### Setting Environment Variables

You can set these environment variables in your deployment configuration:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: beskar7-controller-manager
spec:
  template:
    spec:
      containers:
      - name: manager
        env:
        - name: BESKAR7_REDFISH_TIMEOUT
          value: "1m"
        - name: BESKAR7_CONTROLLER_REQUEUE_INTERVAL
          value: "30s"
```

### Duration Format

Time-based configuration values (timeouts, intervals) should be specified using Go's duration format:
- `s` for seconds (e.g., `30s`)
- `m` for minutes (e.g., `5m`)
- `h` for hours (e.g., `1h`)

### Best Practices

1. **Timeouts**: Set appropriate timeouts based on your network conditions and BMC response times.
2. **Retry Configuration**: Adjust retry parameters based on your environment's reliability:
   - Increase `BESKAR7_RETRY_MAX_ATTEMPTS` for less reliable networks
   - Decrease `BESKAR7_RETRY_MULTIPLIER` for more aggressive retries
3. **Boot Configuration**: Customize boot parameters based on your hardware:
   - Adjust `BESKAR7_BOOT_DEFAULT_EFI_PATH` for different EFI bootloader locations
   - Modify `BESKAR7_BOOT_DEFAULT_OVERRIDE_TARGET` for different boot methods

## Configuration in Code

You can also configure Beskar7 programmatically using the `config` package:

```go
import "github.com/wrkode/beskar7/internal/config"

// Load default configuration
cfg := config.DefaultConfig()

// Modify configuration
cfg.Redfish.DefaultTimeout = 1 * time.Minute
cfg.Controller.RequeueInterval = 30 * time.Second

// Use configuration in your code
```

## Validation

The configuration system validates all input values:
- Duration values must be valid Go duration strings
- Numeric values must be valid numbers
- String values are checked for non-empty values

Invalid values will be logged and the default value will be used instead. 