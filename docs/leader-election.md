# Leader Election Configuration

This document describes the leader election configuration for Beskar7 controller manager, which ensures high availability and prevents split-brain scenarios in multi-replica deployments.

## Overview

Leader election is **enabled by default** in Beskar7 and is essential for:

- **High Availability (HA)**: Multiple controller manager replicas can run, but only one is active
- **Split-brain Prevention**: Ensures only one controller processes resources at a time
- **Seamless Failover**: Automatic leader transition when the current leader fails
- **Zero Downtime**: No service interruption during leader transitions

## Configuration Parameters

### Basic Leader Election

Leader election is controlled by the `--leader-elect` flag:

```bash
# Enable leader election (default: true)
--leader-elect=true

# Disable leader election (single replica only)
--leader-elect=false
```

### Advanced Timing Parameters

Fine-tune leader election behavior with these parameters:

```bash
# Lease duration (default: 15s)
--leader-elect-lease-duration=15s

# Renew deadline (default: 10s) 
--leader-elect-renew-deadline=10s

# Retry period (default: 2s)
--leader-elect-retry-period=2s

# Release on cancel (default: true)
--leader-elect-release-on-cancel=true
```

## Deployment Configurations

### Single Replica (Development)

For development or small deployments:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: beskar7-controller-manager
spec:
  replicas: 1
  template:
    spec:
      containers:
      - name: manager
        args:
        - --leader-elect=false  # Can disable for single replica
        - --enable-security-monitoring=true
        - --metrics-bind-address=:8080
        - --health-probe-bind-address=:8081
```

### High Availability (Production)

For production deployments with multiple replicas:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: beskar7-controller-manager
spec:
  replicas: 3  # Multiple replicas for HA
  template:
    spec:
      containers:
      - name: manager
        args:
        - --leader-elect=true
        - --leader-elect-lease-duration=15s
        - --leader-elect-renew-deadline=10s
        - --leader-elect-retry-period=2s
        - --leader-elect-release-on-cancel=true
        - --enable-security-monitoring=true
        - --metrics-bind-address=:8080
        - --health-probe-bind-address=:8081
```

### Fast Failover (Critical Workloads)

For environments requiring faster failover:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: beskar7-controller-manager
spec:
  replicas: 3
  template:
    spec:
      containers:
      - name: manager
        args:
        - --leader-elect=true
        - --leader-elect-lease-duration=10s  # Shorter lease
        - --leader-elect-renew-deadline=6s   # Faster renewal
        - --leader-elect-retry-period=1s     # Quick retries
        - --leader-elect-release-on-cancel=true
```

### Conservative (Network Latency)

For environments with network latency or instability:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: beskar7-controller-manager
spec:
  replicas: 3
  template:
    spec:
      containers:
      - name: manager
        args:
        - --leader-elect=true
        - --leader-elect-lease-duration=30s  # Longer lease
        - --leader-elect-renew-deadline=20s  # More time to renew
        - --leader-elect-retry-period=5s     # Less aggressive retries
        - --leader-elect-release-on-cancel=true
```

## Parameter Guidelines

### Lease Duration
- **Purpose**: How long a leader holds the lease before it expires
- **Default**: 15 seconds
- **Recommendations**:
  - Small/stable clusters: 10-15s
  - Large/unstable networks: 20-30s
  - Critical workloads: 10s

### Renew Deadline
- **Purpose**: How long the leader has to renew the lease before giving up
- **Default**: 10 seconds (must be < lease duration)
- **Formula**: Usually 60-80% of lease duration
- **Recommendations**:
  - Lease 15s → Renew 10s
  - Lease 30s → Renew 20s

### Retry Period
- **Purpose**: How often non-leaders attempt to acquire leadership
- **Default**: 2 seconds
- **Recommendations**:
  - Fast environments: 1-2s
  - Network latency: 3-5s
  - Resource constrained: 5-10s

### Release on Cancel
- **Purpose**: Whether leader voluntarily releases lease on shutdown
- **Default**: true
- **Benefits**: Faster failover during planned shutdowns
- **Caution**: Only safe if process terminates immediately after manager stops

## RBAC Requirements

Leader election requires specific RBAC permissions:

```yaml
# Cluster-wide lease permissions
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: manager-role
rules:
- apiGroups: ["coordination.k8s.io"]
  resources: ["leases"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]

# Namespace-specific lease permissions
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: manager-role
  namespace: beskar7-system
rules:
- apiGroups: ["coordination.k8s.io"]
  resources: ["leases"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
```

## Monitoring Leader Election

### Key Metrics

Monitor these metrics for leader election health:

```promql
# Current leader (should always be 1)
sum(up{job="beskar7-controller"}) by (instance)

# Leader election attempts
rate(leader_election_slowpath_total[5m])

# Leader election errors  
rate(leader_election_error_total[5m])
```

### Health Checks

The controller manager provides health endpoints:

```bash
# Check if this instance is the leader
curl http://pod-ip:8081/readyz

# General health check
curl http://pod-ip:8081/healthz
```

### Log Analysis

Monitor logs for leader election events:

```bash
# View leader election logs
kubectl logs deployment/beskar7-controller-manager -n beskar7-system | grep leader

# Common log patterns:
# "successfully acquired lease" - became leader
# "stopped leading" - lost leadership
# "attempting to acquire leader lease" - trying to become leader
```

## Troubleshooting

### Split Brain Detection

**Symptoms:**
- Multiple controllers processing the same resources
- Conflicting resource updates
- Duplicate provisioning attempts

**Diagnosis:**
```bash
# Check active leaders
kubectl get lease -n beskar7-system

# Verify RBAC permissions
kubectl auth can-i create leases --as=system:serviceaccount:beskar7-system:controller-manager -n beskar7-system

# Check controller logs
kubectl logs deployment/beskar7-controller-manager -n beskar7-system | grep "leader"
```

**Resolution:**
1. Verify RBAC permissions are correct
2. Check network connectivity between replicas
3. Restart all controller manager pods
4. Verify lease duration settings

### Leader Election Failures

**Symptoms:**
- Controllers not starting
- Frequent leader changes
- "unable to acquire lease" errors

**Common Causes:**
1. **RBAC Issues**: Missing lease permissions
2. **Network Issues**: Connectivity problems between pods
3. **Resource Constraints**: Insufficient CPU/memory
4. **Clock Skew**: Time differences between nodes

**Solutions:**

#### RBAC Issues
```bash
# Check current permissions
kubectl describe clusterrole manager-role | grep coordination

# Apply correct RBAC
kubectl apply -f config/rbac/
```

#### Network Issues
```bash
# Test pod-to-pod connectivity
kubectl exec deployment/beskar7-controller-manager -n beskar7-system -- nslookup kubernetes.default

# Check network policies
kubectl get networkpolicy -n beskar7-system
```

#### Resource Constraints
```bash
# Check resource usage
kubectl top pod -n beskar7-system

# Increase resources if needed
kubectl patch deployment beskar7-controller-manager -n beskar7-system \
  --patch '{"spec":{"template":{"spec":{"containers":[{"name":"manager","resources":{"limits":{"memory":"1Gi"}}}]}}}}'
```

### Stuck Leadership

**Symptoms:**
- Leader lease exists but pod is not running
- New pods cannot acquire leadership
- Controllers not processing resources

**Resolution:**
```bash
# Delete stuck lease (emergency only)
kubectl delete lease beskar7-controller-manager -n beskar7-system

# Restart all pods
kubectl rollout restart deployment/controller-manager -n beskar7-system
```

## Best Practices

### Production Deployments

1. **Always enable leader election** for multi-replica deployments
2. **Use 3+ replicas** for true high availability
3. **Configure pod disruption budgets** to prevent all replicas being terminated
4. **Monitor leader election metrics** for health
5. **Test failover scenarios** regularly

### Parameter Tuning

1. **Start with defaults** and adjust based on observed behavior
2. **Conservative settings** for unstable networks
3. **Aggressive settings** for fast failover requirements
4. **Test parameter changes** in staging first

### Resource Management

1. **Adequate resources** for leader election overhead
2. **Network policies** that allow lease coordination
3. **Node affinity** to spread replicas across failure domains
4. **Priority classes** for critical controller pods

### Security Considerations

1. **Minimal RBAC** permissions for lease management
2. **Namespace isolation** for lease resources
3. **Network policies** restricting lease access
4. **Audit logging** for lease operations

## Integration with Cluster API

Beskar7 leader election is compatible with:

- **CAPI Controller Manager**: Can run alongside other controllers
- **Multi-tenancy**: Namespace-scoped lease management
- **Cluster Autoscaler**: Works with dynamic node scaling
- **Service Mesh**: Compatible with Istio/Linkerd

## Migration Guide

### Upgrading from Single to Multi-Replica

1. **Enable leader election** in current deployment
2. **Scale to multiple replicas** gradually
3. **Verify lease creation** and leadership
4. **Test failover** by terminating leader pod

### Changing Timing Parameters

1. **Update deployment** with new parameters
2. **Rolling update** preserves availability
3. **Monitor metrics** for impact assessment
4. **Rollback** if issues detected

This comprehensive leader election configuration ensures reliable, highly available Beskar7 deployments while providing flexibility for different operational requirements. 