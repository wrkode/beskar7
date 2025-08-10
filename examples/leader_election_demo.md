# Leader Election Claim Coordination Demo

This document demonstrates how to use the leader election-based claim coordination feature in Beskar7 for handling high-contention scenarios in multi-controller deployments.

## Overview

The leader election claim coordinator provides enhanced coordination for host claims when multiple Beskar7 controller instances are running simultaneously. This is particularly useful in:

- High-availability deployments with multiple controller replicas
- Large clusters with many concurrent machine provisioning requests
- Scenarios where host claim conflicts are frequent

## Configuration

### Enabling Leader Election Claim Coordination

The feature is controlled by command-line flags when starting the manager:

```bash
# Enable leader election for claim coordination
./manager \
  --enable-claim-coordinator-leader-election=true \
  --claim-coordinator-lease-duration=15s \
  --claim-coordinator-renew-deadline=10s \
  --claim-coordinator-retry-period=2s
```

### Configuration Options

| Flag | Default | Description |
|------|---------|-------------|
| `--enable-claim-coordinator-leader-election` | `false` | Enable leader election for claim coordination |
| `--claim-coordinator-lease-duration` | `15s` | Duration that non-leader candidates wait to acquire leadership |
| `--claim-coordinator-renew-deadline` | `10s` | Interval between leader renewal attempts |
| `--claim-coordinator-retry-period` | `2s` | Duration clients wait between acquisition attempts |

## How It Works

### Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                    Multi-Controller Deployment                     │
├─────────────────┬─────────────────┬─────────────────┬───────────────┤
│  Controller A   │  Controller B   │  Controller C   │  Controller N │
│  (Candidate)    │    (Leader)     │  (Candidate)    │ (Candidate)   │
├─────────────────┼─────────────────┼─────────────────┼───────────────┤
│                 │                 │                 │               │
│  Fallback to    │  Process Claims │  Fallback to    │  Fallback to  │
│  Optimistic     │  as Leader      │  Optimistic     │  Optimistic   │
│  Locking        │                 │  Locking        │  Locking      │
│                 │                 │                 │               │
└─────────────────┴─────────────────┴─────────────────┴───────────────┘
                           │
                           ▼
                ┌─────────────────────┐
                │   Kubernetes API    │
                │   Lease Resource    │
                │ "beskar7-claim-     │
                │  coordinator-       │
                │     leader"         │
                └─────────────────────┘
```

### Leader Election Process

1. **Leader Selection**: One controller instance becomes the leader using Kubernetes lease-based leader election
2. **Claim Processing**: The leader processes claims with enhanced coordination
3. **Fallback Behavior**: Non-leader instances fall back to optimistic locking
4. **Leader Transition**: If the leader fails, a new leader is elected automatically

### Claim Priority System

The leader coordinator implements a priority-based claim processing system:

```go
// Priority calculation factors:
// 1. Machine age (older machines get higher priority)
// 2. Hardware requirements complexity (simpler requirements get higher priority)
// 3. Base priority of 100

priority := 100 // Base priority

// Age factor: 1 point per minute
if !machine.CreationTimestamp.IsZero() {
    age := time.Since(machine.CreationTimestamp.Time)
    priority += int(age.Minutes())
}

// Simplicity factor: 50 points for simple requirements
if len(requiredTags) == 0 && len(preferredTags) == 0 {
    priority += 50
}
```

## Deployment Example

### Kubernetes Deployment with Leader Election

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: beskar7-controller
  namespace: beskar7-system
spec:
  replicas: 3  # Multiple replicas for HA
  selector:
    matchLabels:
      control-plane: beskar7-controller-manager
  template:
    metadata:
      labels:
        control-plane: beskar7-controller-manager
    spec:
      containers:
      - name: manager
        image: beskar7:latest
        command:
        - /manager
        args:
        - --leader-elect=true
        - --enable-claim-coordinator-leader-election=true
        - --claim-coordinator-lease-duration=15s
        - --claim-coordinator-renew-deadline=10s
        - --claim-coordinator-retry-period=2s
        env:
        - name: POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
```

### RBAC Requirements

The leader election claim coordinator requires additional RBAC permissions:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: beskar7-leader-election
rules:
- apiGroups:
  - coordination.k8s.io
  resources:
  - leases
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
```

## Monitoring and Metrics

### Available Metrics

The leader election coordinator exposes several Prometheus metrics:

```promql
# Leader election events
beskar7_claim_coordinator_leader_election_total{namespace="beskar7-system",event_type="started_leading"}

# Claim coordination results
beskar7_claim_coordinator_results_total{namespace="default",result_type="success"}

# Processing duration
beskar7_claim_coordinator_processing_duration_seconds{namespace="default",result_type="success"}

# Leadership duration
beskar7_claim_coordinator_leadership_duration_seconds{namespace="beskar7-system",identity="controller-1"}
```

### Example Monitoring Queries

```promql
# Current leader election status
rate(beskar7_claim_coordinator_leader_election_total[5m])

# Claim success rate
rate(beskar7_claim_coordinator_results_total{result_type="success"}[5m]) / 
rate(beskar7_claim_coordinator_results_total[5m])

# Average claim processing time
rate(beskar7_claim_coordinator_processing_duration_seconds_sum[5m]) / 
rate(beskar7_claim_coordinator_processing_duration_seconds_count[5m])
```

## Benefits and Use Cases

### When to Enable Leader Election

✅ **Enable when:**
- Running multiple controller replicas (>1)
- Experiencing frequent claim conflicts
- Managing large numbers of machines (>50)
- Need deterministic claim ordering
- High-availability requirements

❌ **Skip when:**
- Single controller deployment
- Low machine provisioning volume
- Simple, homogeneous hardware pools
- Testing/development environments

### Performance Characteristics

| Scenario | Standard Coordinator | Leader Election Coordinator |
|----------|---------------------|----------------------------|
| Single Controller | Fast, direct | Slightly slower (leader check) |
| Multiple Controllers | Potential conflicts | Coordinated, no conflicts |
| High Contention | Retry storms possible | Ordered processing |
| Leader Failure | No impact | Brief disruption, then recovery |

## Troubleshooting

### Common Issues

1. **Leader Election Not Working**
   ```bash
   # Check lease resource
   kubectl get leases -n beskar7-system beskar7-claim-coordinator-leader
   
   # Check controller logs
   kubectl logs -n beskar7-system deployment/beskar7-controller -c manager
   ```

2. **Claims Still Conflicting**
   - Verify all controllers have leader election enabled
   - Check RBAC permissions for lease resources
   - Ensure unique identity per controller pod

3. **Performance Issues**
   - Adjust lease timings for your environment
   - Monitor leadership transition frequency
   - Consider disabling if not needed

### Log Analysis

Look for these log messages to verify operation:

```
# Leader election events
"Became leader for claim coordination"
"Lost leadership for claim coordination"
"New leader elected for claim coordination"

# Claim processing
"Processing claim as leader"
"Using fallback coordinator (no leader or leader election disabled)"
"Delegating claim to leader"
```

## Migration Guide

### From Standard to Leader Election Coordinator

1. **Update Deployment**: Add leader election flags to existing deployment
2. **Verify RBAC**: Ensure lease permissions are configured
3. **Monitor Metrics**: Watch for leader election activity
4. **Gradual Rollout**: Enable on one replica first, then scale up

### Rollback Procedure

1. Set `--enable-claim-coordinator-leader-election=false`
2. Restart all controller pods
3. Verify normal operation with standard coordinator
4. Clean up lease resources if needed

## Future Enhancements

The leader election coordinator is designed for extensibility:

- **Distributed Claim Queue**: Shared queue for non-leader instances
- **Advanced Priority Algorithms**: Machine learning-based prioritization
- **Cross-Cluster Coordination**: Multi-cluster leader election
- **Custom Leadership Strategies**: Pluggable leader selection algorithms 