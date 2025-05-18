# Error Recovery System

The error recovery system in Beskar7 provides a robust mechanism for handling and recovering from various types of errors that may occur during PhysicalHost operations.

## Overview

The recovery system is designed to:
- Automatically recover from common failure scenarios
- Provide detailed metrics and logging for recovery attempts
- Allow configuration of recovery behavior
- Support composite recovery strategies for complex scenarios

## Recovery Strategies

The system includes several built-in recovery strategies:

### Individual Strategies

1. **PowerStateRecovery**
   - Handles power state related errors
   - Attempts to set the target power state
   - Verifies the state change was successful

2. **BootSourceRecovery**
   - Handles boot source related errors
   - Ejects existing virtual media
   - Sets the specified boot source
   - Verifies the operation was successful

3. **ConnectionRecovery**
   - Handles connection related errors
   - Attempts to re-establish connection
   - Uses exponential backoff for retries

4. **SystemInfoRecovery**
   - Handles system info retrieval errors
   - Attempts to get system information
   - Uses exponential backoff for retries

5. **VirtualMediaRecovery**
   - Handles virtual media related errors
   - Ejects virtual media
   - Verifies connection is still good

### Composite Strategies

The system also includes composite strategies that combine multiple individual strategies:

1. **Provisioning Recovery**
   - Combines PowerState, BootSource, and VirtualMedia recovery
   - Used during host provisioning operations

2. **Discovery Recovery**
   - Combines Connection, SystemInfo, and PowerState recovery
   - Used during host discovery operations

## Configuration

The recovery system can be configured through the `RecoveryConfig` struct:

```go
type RecoveryConfig struct {
    MaxAttempts       int           // Maximum number of recovery attempts
    InitialBackoff    time.Duration // Initial backoff duration
    MaxBackoff        time.Duration // Maximum backoff duration
    BackoffMultiplier float64       // Backoff multiplier
    EnableMetrics     bool          // Enable metrics collection
    EnableLogging     bool          // Enable detailed logging
}
```

Default configuration:
```go
{
    MaxAttempts:       3,
    InitialBackoff:    1 * time.Second,
    MaxBackoff:        5 * time.Minute,
    BackoffMultiplier: 2.0,
    EnableMetrics:     true,
    EnableLogging:     true,
}
```

## Usage

### Basic Usage

```go
// Create a logger
logger := zap.NewProduction()

// Create recovery manager with default config
recoveryManager := recovery.NewRecoveryManager(logger, nil)

// Attempt recovery
err := recoveryManager.AttemptRecovery(ctx, redfishClient, err)
```

### Custom Configuration

```go
// Create custom config
config := &recovery.RecoveryConfig{
    MaxAttempts:       5,
    InitialBackoff:    2 * time.Second,
    MaxBackoff:        10 * time.Minute,
    BackoffMultiplier: 1.5,
    EnableMetrics:     true,
    EnableLogging:     true,
}

// Create recovery manager with custom config
recoveryManager := recovery.NewRecoveryManager(logger, config)
```

### Metrics

The recovery system provides metrics that can be accessed through the `GetMetrics` method:

```go
metrics := recoveryManager.GetMetrics()
fmt.Printf("Total Attempts: %d\n", metrics.TotalAttempts)
fmt.Printf("Successful Recoveries: %d\n", metrics.SuccessfulRecoveries)
fmt.Printf("Failed Recoveries: %d\n", metrics.FailedRecoveries)
fmt.Printf("Total Recovery Duration: %v\n", metrics.RecoveryDuration)
```

## Best Practices

1. **Error Classification**
   - Use specific error types for different failure scenarios
   - Implement `IsApplicable` method to correctly identify applicable strategies

2. **Recovery Strategy Design**
   - Keep strategies focused on specific types of failures
   - Use composite strategies for complex scenarios
   - Include verification steps after recovery attempts

3. **Configuration**
   - Adjust `MaxAttempts` based on the criticality of the operation
   - Configure backoff parameters based on the expected recovery time
   - Enable metrics and logging in production environments

4. **Monitoring**
   - Monitor recovery metrics to identify patterns
   - Set up alerts for high failure rates
   - Use logs to diagnose recovery issues

## Integration with PhysicalHost Controller

The recovery system is integrated into the PhysicalHost controller to handle various failure scenarios:

1. **Discovery Phase**
   - Uses `DiscoveryRecovery` for connection and system info issues
   - Handles temporary network issues and system unavailability

2. **Provisioning Phase**
   - Uses `ProvisioningRecovery` for boot source and power state issues
   - Handles virtual media and power control failures

3. **Deprovisioning Phase**
   - Uses appropriate strategies for cleanup operations
   - Ensures proper host state after deprovisioning

## Error Types

The system handles various error types:

1. **RedfishConnectionError**
   - Connection issues with the Redfish API
   - Authentication failures
   - Network timeouts

2. **PowerStateError**
   - Power state transition failures
   - Power state verification failures

3. **BootSourceError**
   - Boot source configuration failures
   - Virtual media operation failures

4. **DiscoveryError**
   - System info retrieval failures
   - Hardware details collection failures

5. **ProvisioningError**
   - Provisioning step failures
   - State transition failures 