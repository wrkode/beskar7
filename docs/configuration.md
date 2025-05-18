# Configuration

Beskar7 supports configuration via environment variables and environment-specific `.env` files.

## Environment Variables

All configuration keys are prefixed with `BESKAR7_`. For example:

- `BESKAR7_REDFISH_SCHEME` – Default URL scheme for Redfish endpoints
- `BESKAR7_REDFISH_PORT` – Default port for Redfish endpoints
- `BESKAR7_REDFISH_TIMEOUT` – Default timeout for Redfish operations
- `BESKAR7_CONTROLLER_REQUEUE_INTERVAL` – Default interval for requeuing reconciliation
- `BESKAR7_CONTROLLER_REQUEUE_AFTER_ERROR` – Interval for requeuing after an error
- `BESKAR7_CONTROLLER_REQUEUE_AFTER_NO_HOST` – Interval for requeuing when no host is found
- `BESKAR7_RETRY_INITIAL_INTERVAL` – Initial retry interval
- `BESKAR7_RETRY_MAX_INTERVAL` – Maximum retry interval
- `BESKAR7_RETRY_MULTIPLIER` – Factor to multiply the interval by for each retry
- `BESKAR7_RETRY_MAX_ATTEMPTS` – Maximum number of retry attempts
- `BESKAR7_RETRY_MAX_ELAPSED_TIME` – Maximum total time to retry
- `BESKAR7_BOOT_DEFAULT_EFI_PATH` – Default path to the EFI bootloader
- `BESKAR7_BOOT_DEFAULT_OVERRIDE_ENABLED` – Default boot source override setting
- `BESKAR7_BOOT_DEFAULT_OVERRIDE_TARGET` – Default boot source override target

## Environment-Specific Configuration

Beskar7 supports environment-specific configuration via `.env` files and environment variables.

### How it works

- Set the environment with `BESKAR7_ENVIRONMENT` (e.g., `development`, `staging`, `production`).
- Place a `.env` file in `config/environments/<environment>/.env`.
- You can override any config key in the `.env` file (see example below).
- Environment variables with the prefix `BESKAR7_<ENV>_` (e.g., `BESKAR7_DEVELOPMENT_REDFISH_PORT`) will also override config.

### Example

```sh
export BESKAR7_ENVIRONMENT=staging
export BESKAR7_STAGING_REDFISH_PORT=8443
```

The controller will load:
- Defaults
- Values from `config/environments/staging/.env`
- Any `BESKAR7_STAGING_*` environment variables

### Precedence

1. `BESKAR7_<ENV>_<KEY>` environment variables
2. `.env` file in `config/environments/<environment>/`
3. Top-level `BESKAR7_*` environment variables
4. Built-in defaults

## Usage

In your code, load the configuration using:

```go
import "github.com/wrkode/beskar7/internal/config"

cfg, err := config.LoadConfig()
if err != nil {
    // Handle error
}
```

## Best Practices

- Use environment variables for dynamic configuration.
- Use `.env` files for static, environment-specific overrides.
- Always validate configuration values before use.

## Validation Rules

- All durations must be valid Go duration strings (e.g., `30s`, `5m`).
- All numeric values must be valid numbers.
- All paths must be valid for the operating system.

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