# State Management and Recovery

Beskar7 implements a hardened state machine for PhysicalHost resources that ensures reliable state transitions, automatic recovery from stuck states, and protection against race conditions.

## Overview

The state management system provides:
- **Validated State Transitions**: All state changes are validated before execution
- **Automatic Recovery**: Detects and recovers from stuck or inconsistent states
- **Race Condition Protection**: Prevents concurrent modifications from causing invalid states
- **Comprehensive Logging**: Events and logs for troubleshooting state issues

## PhysicalHost State Machine

### State Diagram

```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│    None     │───▶│ Enrolling   │───▶│ Available   │───▶│   Claimed   │
└─────────────┘    └─────────────┘    └─────────────┘    └─────────────┘
       │                   │                   │                   │
       │                   │                   │                   ▼
       │                   │                   │          ┌─────────────┐
       │                   │                   │          │Provisioning │
       │                   │                   │          └─────────────┘
       │                   │                   │                   │
       │                   │                   │                   ▼
       │                   │                   │          ┌─────────────┐
       │                   │                   │          │ Provisioned │
       │                   │                   │          └─────────────┘
       │                   │                   │                   │
       │                   │                   │                   ▼
       └──────────────────▶┌─────────────────────────────────────────┐
                           │                Error                    │
                           └─────────────────────────────────────────┘
                                               │
                                               ▼
                                    ┌─────────────┐
                                    │Deprovisioning│
                                    └─────────────┘
```

### State Definitions

| State | Description | Valid Transitions |
|-------|-------------|-------------------|
| `None` | Initial state before enrollment | `Enrolling` |
| `Enrolling` | Connecting to Redfish endpoint | `Available`, `Error` |
| `Available` | Ready to be claimed | `Claimed`, `Error`, `Deprovisioning` |
| `Claimed` | Reserved by a consumer | `Provisioning`, `Available`, `Error` |
| `Provisioning` | Being configured | `Provisioned`, `Available`, `Error` |
| `Provisioned` | Successfully configured | `Available`, `Deprovisioning` |
| `Error` | Error state requiring intervention | `Enrolling`, `Available`, `Deprovisioning` |
| `Deprovisioning` | Being cleaned up | *(final state)* |

### Transition Requirements

#### Enrolling → Available
- Redfish connection successful
- Hardware details retrieved

#### Available → Claimed  
- `ConsumerRef` must be set
- Host must not be in error state

#### Claimed → Provisioning
- `ConsumerRef` must be set
- `BootISOSource` must be set

#### Provisioning → Provisioned
- Boot configuration successful
- Host powered on

#### Any State → Error
- Always allowed (for error handling)

#### Error → Recovery States
- `Error → Enrolling`: Redfish connection info present
- `Error → Available`: Host not claimed, connection healthy

## Automatic Recovery

### Stuck State Detection

The system automatically detects hosts stuck in transitional states:

- **Detection Timeout**: 15 minutes (configurable)
- **Check Frequency**: Every reconciliation cycle
- **Recovery Actions**: Automatic state transition or error reporting

### Recovery Strategies

#### Enrolling State (Stuck)
- Retry Redfish connection
- Transition to `Error` if connection fails repeatedly

#### Provisioning State (Stuck)  
- Check boot configuration status
- Retry provisioning steps
- Transition to `Error` if provisioning fails

#### Error State Recovery
- Attempt transition to `Available` if host is healthy
- Retry `Enrolling` if connection issues resolved

### Configuration

Recovery behavior can be configured via environment variables:

```yaml
env:
  - name: RECONCILE_TIMEOUT
    value: "30m"  # Maximum reconciliation time
  - name: STUCK_STATE_TIMEOUT  
    value: "15m"  # Time before considering state stuck
  - name: MAX_RETRIES
    value: "3"    # Maximum retry attempts
```

## Troubleshooting

### Common Issues

#### Host Stuck in Enrolling State

**Symptoms:**
- Host remains in `Enrolling` state for > 15 minutes
- No progress in Redfish connection

**Diagnosis:**
```bash
# Check host status
kubectl get physicalhost <host-name> -o yaml

# Check events
kubectl describe physicalhost <host-name>

# Check controller logs
kubectl logs -n beskar7-system deployment/beskar7-controller-manager
```

**Resolution:**
1. Verify Redfish credentials in secret
2. Check network connectivity to BMC
3. Validate BMC address format
4. Check for InsecureSkipVerify setting if using self-signed certificates

#### Host Stuck in Provisioning State

**Symptoms:**
- Host remains in `Provisioning` state for > 15 minutes
- Boot configuration appears incomplete

**Diagnosis:**
```bash
# Check provisioning status
kubectl get physicalhost <host-name> -o jsonpath='{.status}'

# Check associated machine
kubectl get beskar7machine <machine-name> -o yaml
```

**Resolution:**
1. Verify boot ISO URL is accessible
2. Check host power state via BMC
3. Validate boot configuration in BMC
4. Check for hardware compatibility issues

#### State Consistency Errors

**Symptoms:**
- Events showing "StateInconsistent"
- Frequent state transitions

**Diagnosis:**
```bash
# Check for inconsistent fields
kubectl get physicalhost <host-name> -o yaml | grep -E "(consumerRef|bootIsoSource|state)"
```

**Resolution:**
1. Ensure `ConsumerRef` alignment with state
2. Verify `BootISOSource` is set only when needed
3. Check for manual modifications to spec fields

### Manual Recovery

If automatic recovery fails, manual intervention may be required:

#### Force State Reset
```bash
# Clear consumer reference to release host
kubectl patch physicalhost <host-name> --type='merge' -p='{"spec":{"consumerRef":null}}'

# Clear boot ISO source  
kubectl patch physicalhost <host-name> --type='merge' -p='{"spec":{"bootIsoSource":null}}'
```

#### Reset to Available State
The controller will automatically transition to `Available` when spec fields are cleared.

#### Emergency Cleanup
```bash
# Remove finalizer to force deletion (USE WITH CAUTION)
kubectl patch physicalhost <host-name> --type='merge' -p='{"metadata":{"finalizers":[]}}'
```

## Monitoring and Observability

### Metrics

The state machine exposes metrics for monitoring:

- `beskar7_physicalhost_state_transitions_total`: Total state transitions
- `beskar7_physicalhost_stuck_states_total`: Number of stuck state detections
- `beskar7_physicalhost_recovery_attempts_total`: Recovery attempts
- `beskar7_physicalhost_state_duration_seconds`: Time spent in each state

### Events

State changes generate Kubernetes events:

```bash
# View state-related events
kubectl get events --field-selector involvedObject.kind=PhysicalHost
```

### Logging

State machine operations are logged with structured logging:

```json
{
  "level": "info",
  "msg": "State transition validated",
  "physicalhost": "default/host-1",
  "from": "Available", 
  "to": "Claimed",
  "reason": "ConsumerAssigned"
}
```

## Best Practices

### For Operations Teams

1. **Monitor State Metrics**: Set up alerts for stuck states and failed recoveries
2. **Regular Health Checks**: Monitor hosts in transitional states
3. **Event Monitoring**: Set up log aggregation for state transition events
4. **Backup Recovery**: Document manual recovery procedures

### For Developers

1. **State Validation**: Always use the state machine for transitions
2. **Error Handling**: Implement proper error transitions
3. **Testing**: Test state transitions in development environments
4. **Documentation**: Document any custom state logic

### For End Users

1. **Avoid Manual Edits**: Don't manually edit PhysicalHost status fields
2. **Use Proper Workflow**: Follow claiming → provisioning → release workflow  
3. **Monitor Events**: Check events if hosts appear stuck
4. **Report Issues**: Report persistent state issues to operations team

## State Machine API

### Using State Machine in Controllers

```go
// Initialize state machine
stateMachine := statemachine.NewPhysicalHostStateMachine(logger)
stateGuard := statemachine.NewStateTransitionGuard(client, logger)

// Validate transition
if err := stateMachine.ValidateTransition(host, newState); err != nil {
    return fmt.Errorf("invalid transition: %w", err)
}

// Safe transition with retries
err := stateGuard.SafeStateTransition(ctx, host, stateMachine, newState, reason, maxRetries)
```

### State Consistency Validation

```go
// Validate current state consistency
if err := stateMachine.ValidateStateConsistency(host); err != nil {
    logger.Error(err, "State consistency validation failed")
    // Handle inconsistency...
}
```

## Security Considerations

- **RBAC**: State transitions respect Kubernetes RBAC policies
- **Validation**: All transitions are validated before execution
- **Audit Trail**: State changes are logged and recorded as events
- **Race Protection**: Optimistic locking prevents concurrent modification issues

## Version Compatibility

This state management system is compatible with:
- Kubernetes 1.25+
- Cluster API v1.5+
- Beskar7 v1.0+

State machine behavior is backward compatible, but new validation rules may reject previously invalid configurations. 