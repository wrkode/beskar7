# Feature Flags

Beskar7 includes a feature flag system that allows enabling or disabling experimental features. This document describes how to use and configure feature flags.

## Available Feature Flags

The following feature flags are currently available:

| Feature Flag | Description | Default |
|--------------|-------------|---------|
| `EnableAdvancedRecovery` | Enables advanced error recovery mechanisms | `false` |
| `EnableMetricsExport` | Enables detailed metrics export | `false` |
| `EnableCustomBootSource` | Enables custom boot source configuration | `false` |
| `EnableVendorSpecificFeatures` | Enables vendor-specific features | `false` |

## Configuration Methods

### Environment Variables

Feature flags can be configured using environment variables. The format is:

```
BESKAR7_FEATURE_<FEATURE_NAME>
```

For example:
```bash
export BESKAR7_FEATURE_ENABLEADVANCEDRECOVERY=true
export BESKAR7_FEATURE_ENABLEMETRICSEXPORT=false
```

Valid values are:
- `true` or `1` to enable the feature
- `false` or `0` to disable the feature

### Configuration File

Feature flags can also be configured in the controller configuration:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: beskar7-config
  namespace: beskar7-system
data:
  feature-flags: |
    EnableAdvancedRecovery: true
    EnableMetricsExport: false
    EnableCustomBootSource: true
    EnableVendorSpecificFeatures: false
```

## Using Feature Flags in Code

To check if a feature is enabled in your code:

```go
import "github.com/wrkode/beskar7/internal/features"

// Check if a feature is enabled
if features.IsEnabled(features.EnableAdvancedRecovery) {
    // Use advanced recovery features
}
```

## Adding New Feature Flags

To add a new feature flag:

1. Add the feature flag constant in `internal/features/flags.go`:
```go
const (
    // EnableNewFeature enables the new feature
    EnableNewFeature Feature = "EnableNewFeature"
)
```

2. Register the feature flag with its default state in `RegisterDefaultFeatures()`:
```go
func RegisterDefaultFeatures() {
    RegisterFeature(EnableNewFeature, false)
}
```

3. Update this documentation to include the new feature flag.

## Best Practices

1. **Default to Disabled**: New features should be disabled by default until they are stable.
2. **Documentation**: Always document new feature flags and their purpose.
3. **Testing**: Include tests for both enabled and disabled states of feature flags.
4. **Deprecation**: When a feature is stable, consider removing the feature flag and making it always enabled.
5. **Monitoring**: Monitor the usage of feature flags to understand their adoption.

## Security Considerations

1. Feature flags should not be used to control security features.
2. Sensitive features should not be controlled by feature flags.
3. Feature flag states should be logged for audit purposes.

## Troubleshooting

If a feature flag is not working as expected:

1. Verify the environment variable is set correctly
2. Check the controller logs for feature flag initialization
3. Ensure the feature flag is properly registered
4. Verify the feature flag is being checked correctly in the code

## Future Improvements

1. Add a web interface for managing feature flags
2. Implement feature flag analytics
3. Add support for feature flag targeting (e.g., by namespace or cluster)
4. Implement feature flag versioning
5. Add support for feature flag dependencies 