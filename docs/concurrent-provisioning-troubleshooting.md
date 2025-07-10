# Concurrent Provisioning Troubleshooting Guide

This guide provides detailed troubleshooting procedures for Beskar7's concurrent provisioning system, covering common issues, diagnostic procedures, and resolution strategies.

## Quick Diagnostic Script

Start with this comprehensive health check script:

```bash
#!/bin/bash
# Beskar7 Concurrent Provisioning Health Check
# Usage: ./concurrent-provisioning-health-check.sh

set -e

echo "🔍 Beskar7 Concurrent Provisioning Health Check"
echo "============================================="

NAMESPACE=${BESKAR7_NAMESPACE:-beskar7-system}
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() { echo -e "${BLUE}ℹ️  $1${NC}"; }
log_success() { echo -e "${GREEN}✅ $1${NC}"; }
log_warning() { echo -e "${YELLOW}⚠️  $1${NC}"; }
log_error() { echo -e "${RED}❌ $1${NC}"; }

# Function to check if a command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Check prerequisites
echo
log_info "Checking prerequisites..."
if ! command_exists kubectl; then
    log_error "kubectl not found"
    exit 1
fi

if ! kubectl cluster-info >/dev/null 2>&1; then
    log_error "Cannot connect to Kubernetes cluster"
    exit 1
fi

# Check controller deployment
echo
log_info "Checking controller deployment..."
CONTROLLER_STATUS=$(kubectl get deployment beskar7-controller-manager -n $NAMESPACE -o jsonpath='{.status.conditions[?(@.type=="Available")].status}' 2>/dev/null || echo "NotFound")

if [[ "$CONTROLLER_STATUS" == "True" ]]; then
    log_success "Controller deployment is available"
    REPLICAS=$(kubectl get deployment beskar7-controller-manager -n $NAMESPACE -o jsonpath='{.status.replicas}')
    READY_REPLICAS=$(kubectl get deployment beskar7-controller-manager -n $NAMESPACE -o jsonpath='{.status.readyReplicas}')
    echo "  Replicas: $READY_REPLICAS/$REPLICAS ready"
elif [[ "$CONTROLLER_STATUS" == "NotFound" ]]; then
    log_error "Controller deployment not found in namespace $NAMESPACE"
    exit 1
else
    log_error "Controller deployment is not available"
    kubectl get deployment beskar7-controller-manager -n $NAMESPACE
fi

# Check leader election
echo
log_info "Checking leader election..."
LEADER_LEASE=$(kubectl get lease -n $NAMESPACE -o jsonpath='{.items[?(@.metadata.name=="beskar7-controller-manager")].spec.holderIdentity}' 2>/dev/null)
if [[ -n "$LEADER_LEASE" ]]; then
    log_success "Leader election active, leader: $LEADER_LEASE"
else
    log_warning "No leader election lease found"
fi

# Check claim coordinator leader election
CLAIM_LEADER_LEASE=$(kubectl get lease -n $NAMESPACE -o jsonpath='{.items[?(@.metadata.name=="beskar7-claim-coordinator-leader")].spec.holderIdentity}' 2>/dev/null)
if [[ -n "$CLAIM_LEADER_LEASE" ]]; then
    log_success "Claim coordinator leader election active, leader: $CLAIM_LEADER_LEASE"
else
    log_info "Claim coordinator leader election not enabled (using standard coordination)"
fi

# Check host states
echo
log_info "Checking host states..."
TOTAL_HOSTS=$(kubectl get physicalhosts --all-namespaces --no-headers 2>/dev/null | wc -l)
AVAILABLE_HOSTS=$(kubectl get physicalhosts --all-namespaces -o jsonpath='{.items[?(@.status.state=="Available")].metadata.name}' 2>/dev/null | wc -w)
CLAIMED_HOSTS=$(kubectl get physicalhosts --all-namespaces -o jsonpath='{.items[?(@.status.state=="Claimed")].metadata.name}' 2>/dev/null | wc -w)
PROVISIONING_HOSTS=$(kubectl get physicalhosts --all-namespaces -o jsonpath='{.items[?(@.status.state=="Provisioning")].metadata.name}' 2>/dev/null | wc -w)
ERROR_HOSTS=$(kubectl get physicalhosts --all-namespaces -o jsonpath='{.items[?(@.status.state=="Error")].metadata.name}' 2>/dev/null | wc -w)

echo "  Total hosts: $TOTAL_HOSTS"
echo "  Available: $AVAILABLE_HOSTS"
echo "  Claimed: $CLAIMED_HOSTS"
echo "  Provisioning: $PROVISIONING_HOSTS"
echo "  Error: $ERROR_HOSTS"

if [[ $ERROR_HOSTS -gt 0 ]]; then
    log_warning "$ERROR_HOSTS hosts in error state"
fi

if [[ $AVAILABLE_HOSTS -eq 0 && $CLAIMED_HOSTS -gt 0 ]]; then
    log_warning "No available hosts but hosts are claimed (possible resource shortage)"
fi

# Check machine states
echo
log_info "Checking machine states..."
TOTAL_MACHINES=$(kubectl get beskar7machines --all-namespaces --no-headers 2>/dev/null | wc -l)
PENDING_MACHINES=$(kubectl get beskar7machines --all-namespaces -o jsonpath='{.items[?(@.status.phase!="Provisioned")].metadata.name}' 2>/dev/null | wc -w)

echo "  Total machines: $TOTAL_MACHINES"
echo "  Pending provisioning: $PENDING_MACHINES"

if [[ $PENDING_MACHINES -gt 0 && $AVAILABLE_HOSTS -eq 0 ]]; then
    log_error "Machines pending but no available hosts"
fi

# Check recent events
echo
log_info "Checking recent events..."
RECENT_ERRORS=$(kubectl get events --all-namespaces --field-selector type=Warning --sort-by='.lastTimestamp' 2>/dev/null | grep beskar7 | tail -3 | wc -l)
if [[ $RECENT_ERRORS -gt 0 ]]; then
    log_warning "$RECENT_ERRORS recent warning events"
    kubectl get events --all-namespaces --field-selector type=Warning --sort-by='.lastTimestamp' | grep beskar7 | tail -3
fi

# Check metrics availability
echo
log_info "Checking metrics..."
if kubectl get --raw /metrics >/dev/null 2>&1; then
    # Check claim metrics
    CLAIM_ATTEMPTS=$(kubectl get --raw /metrics 2>/dev/null | grep beskar7_host_claim_attempts_total | head -1)
    if [[ -n "$CLAIM_ATTEMPTS" ]]; then
        log_success "Claim metrics available"
        
        # Calculate conflict rate
        TOTAL_ATTEMPTS=$(kubectl get --raw /metrics | grep beskar7_host_claim_attempts_total | awk '{sum+=$2} END {print sum}')
        CONFLICTS=$(kubectl get --raw /metrics | grep 'beskar7_host_claim_attempts_total.*outcome="conflict"' | awk '{sum+=$2} END {print sum}')
        
        if [[ -n "$TOTAL_ATTEMPTS" && -n "$CONFLICTS" && $TOTAL_ATTEMPTS -gt 0 ]]; then
            CONFLICT_RATE=$(( (CONFLICTS * 100) / TOTAL_ATTEMPTS ))
            if [[ $CONFLICT_RATE -gt 15 ]]; then
                log_warning "High claim conflict rate: $CONFLICT_RATE%"
            else
                log_success "Claim conflict rate: $CONFLICT_RATE%"
            fi
        fi
    else
        log_warning "Claim metrics not found"
    fi
    
    # Check queue metrics
    QUEUE_LENGTH=$(kubectl get --raw /metrics 2>/dev/null | grep beskar7_provisioning_queue_length | awk '{print $2}' | head -1)
    if [[ -n "$QUEUE_LENGTH" ]]; then
        log_success "Queue metrics available, length: $QUEUE_LENGTH"
        if [[ $QUEUE_LENGTH -gt 40 ]]; then
            log_warning "Queue length is high: $QUEUE_LENGTH"
        fi
    fi
else
    log_warning "Cannot access metrics endpoint"
fi

# Check controller logs for errors
echo
log_info "Checking recent controller logs..."
ERROR_COUNT=$(kubectl logs -n $NAMESPACE deployment/beskar7-controller-manager --since=10m 2>/dev/null | grep -i error | wc -l)
if [[ $ERROR_COUNT -gt 0 ]]; then
    log_warning "$ERROR_COUNT errors in last 10 minutes"
    echo "Recent errors:"
    kubectl logs -n $NAMESPACE deployment/beskar7-controller-manager --since=10m | grep -i error | tail -3
else
    log_success "No errors in recent logs"
fi

# Resource utilization check
echo
log_info "Checking resource utilization..."
if command_exists kubectl && kubectl top pod -n $NAMESPACE >/dev/null 2>&1; then
    log_success "Resource metrics available"
    kubectl top pod -n $NAMESPACE | grep beskar7-controller
else
    log_info "Resource metrics not available (metrics-server may not be installed)"
fi

# Summary
echo
echo "🎯 Health Check Summary"
echo "======================"

if [[ $ERROR_HOSTS -eq 0 && $RECENT_ERRORS -eq 0 && "$CONTROLLER_STATUS" == "True" ]]; then
    log_success "System appears healthy"
else
    log_warning "Issues detected - see details above"
    echo
    echo "Recommended actions:"
    if [[ $ERROR_HOSTS -gt 0 ]]; then
        echo "  1. Investigate hosts in error state"
    fi
    if [[ $RECENT_ERRORS -gt 0 ]]; then
        echo "  2. Review recent warning events"
    fi
    if [[ "$CONTROLLER_STATUS" != "True" ]]; then
        echo "  3. Check controller deployment status"
    fi
fi

echo
echo "For detailed troubleshooting, see: https://docs.beskar7.io/troubleshooting/concurrent-provisioning"
```

## Issue Categories

### 1. Host Claim Conflicts

#### Symptoms
- High conflict rate in metrics: `beskar7_host_claim_attempts_total{outcome="conflict"}`
- Log messages: "optimistic lock conflict" or "resource version conflict"
- Machines taking excessive time to provision
- Multiple machines competing for same hosts

#### Diagnostic Commands

```bash
# Check current conflict rate
kubectl get --raw /metrics | grep beskar7_host_claim_attempts_total | grep conflict

# Find machines with claim conflicts
kubectl get events --field-selector reason=ClaimConflict --sort-by='.lastTimestamp'

# Check deterministic selection patterns
kubectl logs -n beskar7-system deployment/beskar7-controller-manager | grep "selecting host" | tail -20

# Analyze host selection distribution
kubectl get physicalhosts -o custom-columns=NAME:.metadata.name,STATE:.status.state,CONSUMER:.spec.consumerRef.name | sort -k3
```

#### Resolution Steps

**Immediate Actions:**
```bash
# 1. Check host pool adequacy
AVAILABLE=$(kubectl get physicalhosts -o jsonpath='{.items[?(@.status.state=="Available")].metadata.name}' | wc -w)
PENDING=$(kubectl get beskar7machines -o jsonpath='{.items[?(@.status.phase!="Provisioned")].metadata.name}' | wc -w)
echo "Available hosts: $AVAILABLE, Pending machines: $PENDING"

# 2. If insufficient hosts, scale up
kubectl get machinedeployment worker-nodes -o yaml | sed 's/replicas: [0-9]*/replicas: '$((PENDING + 5))'/' | kubectl apply -f -

# 3. Enable leader election coordination for high contention
kubectl patch deployment beskar7-controller-manager -n beskar7-system \
  --patch '{"spec":{"template":{"spec":{"containers":[{"name":"manager","args":["--enable-claim-coordinator-leader-election=true"]}]}}}}'
```

**Long-term Solutions:**
```yaml
# Add diverse host tagging
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: PhysicalHost
metadata:
  name: host-with-varied-tags
spec:
  tags: ["worker", "gpu", "high-memory", "zone-a"]  # More specific tagging
  redfishConnection:
    address: "redfish.example.com"
    credentialsSecretRef: "bmc-credentials"
```

### 2. Provisioning Queue Issues

#### Symptoms
- Queue length metrics showing high values: `beskar7_provisioning_queue_length`
- BMC cooldown events: `beskar7_bmc_cooldown_waits_total`
- Operations timing out
- New provisioning requests being rejected

#### Diagnostic Commands

```bash
# Check queue status
kubectl get --raw /metrics | grep beskar7_provisioning_queue

# Check BMC operation times
kubectl get --raw /metrics | grep beskar7_bmc_operation_duration

# Find slow BMCs
kubectl logs -n beskar7-system deployment/beskar7-controller-manager | grep "BMC operation" | grep -E "(timeout|slow)"

# Check queue configuration
kubectl get configmap beskar7-config -n beskar7-system -o jsonpath='{.data}'
```

#### Resolution Steps

**Queue Capacity Issues:**
```bash
# Increase queue size
kubectl patch configmap beskar7-config -n beskar7-system \
  --patch '{"data":{"provisioning.maxQueueSize":"100"}}'

# Add more workers
kubectl patch configmap beskar7-config -n beskar7-system \
  --patch '{"data":{"provisioning.workerCount":"8"}}'

# Restart controllers to apply changes
kubectl rollout restart deployment/beskar7-controller-manager -n beskar7-system
```

**BMC Performance Issues:**
```bash
# Increase BMC cooldown for problematic BMCs
kubectl patch configmap beskar7-config -n beskar7-system \
  --patch '{"data":{"provisioning.bmcCooldownPeriod":"30s"}}'

# Reduce concurrent operations
kubectl patch configmap beskar7-config -n beskar7-system \
  --patch '{"data":{"provisioning.maxConcurrentOps":"3"}}'
```

### 3. Leader Election Problems

#### Symptoms
- Frequent leadership changes: `beskar7_claim_coordinator_leader_election_total`
- Log messages about leadership transitions
- Inconsistent claim processing performance
- Split-brain behavior

#### Diagnostic Commands

```bash
# Check leader election lease
kubectl get lease -n beskar7-system beskar7-claim-coordinator-leader -o yaml

# Monitor leadership transitions
kubectl get events --field-selector involvedObject.name=beskar7-claim-coordinator-leader

# Check leader election configuration
kubectl get deployment beskar7-controller-manager -n beskar7-system -o jsonpath='{.spec.template.spec.containers[0].args}'

# Watch leadership changes
kubectl get lease -n beskar7-system beskar7-claim-coordinator-leader -w
```

#### Resolution Steps

**Unstable Leadership:**
```yaml
# More conservative leader election settings
apiVersion: apps/v1
kind: Deployment
metadata:
  name: beskar7-controller-manager
spec:
  template:
    spec:
      containers:
      - name: manager
        args:
        - --claim-coordinator-lease-duration=30s    # Longer lease
        - --claim-coordinator-renew-deadline=20s    # More time to renew
        - --claim-coordinator-retry-period=5s       # Less aggressive retries
```

**Network Issues:**
```bash
# Check pod-to-pod connectivity
kubectl exec -n beskar7-system deployment/beskar7-controller-manager -- nslookup kubernetes.default

# Test API server connectivity
kubectl exec -n beskar7-system deployment/beskar7-controller-manager -- wget -O- --timeout=5 https://kubernetes.default/api/v1

# Check for network policies blocking coordination
kubectl get networkpolicy -n beskar7-system
```

### 4. Performance Degradation

#### Symptoms
- High claim duration: `beskar7_host_claim_duration_seconds` P95 > 5s
- Controller pod CPU/memory usage high
- Slow reconciliation loops
- Delayed state transitions

#### Diagnostic Commands

```bash
# Check resource utilization
kubectl top pod -n beskar7-system

# Check reconciliation performance
kubectl logs -n beskar7-system deployment/beskar7-controller-manager | grep "Reconcile" | tail -10

# Analyze claim durations
kubectl get --raw /metrics | grep beskar7_host_claim_duration_seconds_bucket

# Check for resource constraints
kubectl describe pod -n beskar7-system -l app=beskar7-controller-manager
```

#### Resolution Steps

**Resource Scaling:**
```yaml
# Increase controller resources
apiVersion: apps/v1
kind: Deployment
metadata:
  name: beskar7-controller-manager
spec:
  template:
    spec:
      containers:
      - name: manager
        resources:
          requests:
            memory: "512Mi"
            cpu: "300m"
          limits:
            memory: "2Gi"
            cpu: "1000m"
```

**Concurrency Tuning:**
```bash
# Reduce concurrent reconciles if overloaded
kubectl patch deployment beskar7-controller-manager -n beskar7-system \
  --patch '{"spec":{"template":{"spec":{"containers":[{"name":"manager","args":["--max-concurrent-reconciles=5"]}]}}}}'
```

### 5. BMC Connectivity Issues

#### Symptoms
- BMC operation timeouts
- Authentication failures
- Network connectivity errors
- Inconsistent Redfish responses

#### Diagnostic Commands

```bash
# Test BMC connectivity from controller pod
kubectl exec -n beskar7-system deployment/beskar7-controller-manager -- curl -k https://your-bmc.example.com/redfish/v1

# Check BMC credentials
kubectl get secret -n beskar7-system bmc-credentials -o jsonpath='{.data}'

# Find BMC-related errors
kubectl logs -n beskar7-system deployment/beskar7-controller-manager | grep -i "redfish\|bmc" | grep -i error

# Check host connection status
kubectl get physicalhosts -o custom-columns=NAME:.metadata.name,STATE:.status.state,BMC:.spec.redfishConnection.address
```

#### Resolution Steps

**Connectivity Fixes:**
```bash
# Test network connectivity
kubectl exec -n beskar7-system deployment/beskar7-controller-manager -- ping -c 3 your-bmc.example.com

# Check DNS resolution
kubectl exec -n beskar7-system deployment/beskar7-controller-manager -- nslookup your-bmc.example.com

# Verify TLS connectivity
kubectl exec -n beskar7-system deployment/beskar7-controller-manager -- openssl s_client -connect your-bmc.example.com:443 -servername your-bmc.example.com
```

**Credential Issues:**
```bash
# Verify credential format
kubectl get secret bmc-credentials -o jsonpath='{.data.username}' | base64 -d
kubectl get secret bmc-credentials -o jsonpath='{.data.password}' | base64 -d

# Test authentication manually
curl -k -u "$(kubectl get secret bmc-credentials -o jsonpath='{.data.username}' | base64 -d):$(kubectl get secret bmc-credentials -o jsonpath='{.data.password}' | base64 -d)" https://your-bmc.example.com/redfish/v1/Systems
```

## Advanced Debugging Procedures

### Claim Flow Tracing

To trace a specific machine through the entire claim process:

```bash
#!/bin/bash
MACHINE_NAME="worker-01"
NAMESPACE="default"

echo "Tracing claim flow for $MACHINE_NAME"

# 1. Check machine status
kubectl get beskar7machine $MACHINE_NAME -n $NAMESPACE -o yaml

# 2. Find related events
kubectl get events --field-selector involvedObject.name=$MACHINE_NAME -n $NAMESPACE --sort-by='.lastTimestamp'

# 3. Trace through controller logs
kubectl logs -n beskar7-system deployment/beskar7-controller-manager | grep "$MACHINE_NAME" | sort

# 4. Check host assignment
HOST_NAME=$(kubectl get beskar7machine $MACHINE_NAME -n $NAMESPACE -o jsonpath='{.status.hostRef.name}')
if [[ -n "$HOST_NAME" ]]; then
    echo "Assigned to host: $HOST_NAME"
    kubectl get physicalhost $HOST_NAME -n $NAMESPACE -o yaml
fi

# 5. Check for conflicts
kubectl get events --field-selector reason=ClaimConflict,involvedObject.name=$MACHINE_NAME -n $NAMESPACE
```

### Performance Profiling

To identify performance bottlenecks:

```bash
#!/bin/bash
echo "Performance profiling for concurrent provisioning"

# 1. Collect baseline metrics
kubectl get --raw /metrics | grep beskar7_ > baseline_metrics.txt
echo "Baseline metrics collected"

# 2. Generate load
echo "Generating test load..."
for i in {1..10}; do
    kubectl apply -f - <<EOF
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: Beskar7Machine
metadata:
  name: test-machine-$i
  namespace: default
spec:
  imageURL: "http://example.com/test.iso"
  osFamily: "kairos"
EOF
done

# 3. Wait and collect metrics
sleep 60
kubectl get --raw /metrics | grep beskar7_ > load_metrics.txt
echo "Load metrics collected"

# 4. Analyze differences
echo "Analyzing performance impact..."
echo "Claim attempts during load:"
grep beskar7_host_claim_attempts_total load_metrics.txt | awk '{sum+=$2} END {print sum}'

echo "Average claim duration:"
grep beskar7_host_claim_duration_seconds_sum load_metrics.txt

# 5. Cleanup
for i in {1..10}; do
    kubectl delete beskar7machine test-machine-$i -n default --ignore-not-found
done
```

### Memory Leak Detection

To detect potential memory leaks in the controller:

```bash
#!/bin/bash
echo "Memory leak detection for beskar7 controller"

# Monitor memory usage over time
for i in {1..10}; do
    MEMORY=$(kubectl top pod -n beskar7-system --no-headers | grep beskar7-controller | awk '{print $3}')
    TIMESTAMP=$(date)
    echo "$TIMESTAMP: $MEMORY"
    sleep 30
done

# Check for goroutine leaks
kubectl exec -n beskar7-system deployment/beskar7-controller-manager -- wget -O- http://localhost:8080/debug/pprof/goroutine?debug=1 | grep "goroutine" | wc -l
```

## Emergency Procedures

### Split-Brain Recovery

If multiple controllers are processing the same resources:

```bash
#!/bin/bash
echo "🚨 Emergency split-brain recovery procedure"

# 1. Identify the issue
kubectl get lease -n beskar7-system
kubectl get deployment beskar7-controller-manager -n beskar7-system

# 2. Stop all controllers
kubectl scale deployment beskar7-controller-manager -n beskar7-system --replicas=0

# 3. Wait for graceful shutdown
sleep 30

# 4. Clean up leader election leases
kubectl delete lease beskar7-controller-manager -n beskar7-system --ignore-not-found
kubectl delete lease beskar7-claim-coordinator-leader -n beskar7-system --ignore-not-found

# 5. Start single controller first
kubectl scale deployment beskar7-controller-manager -n beskar7-system --replicas=1

# 6. Wait for stability
sleep 60
kubectl get lease -n beskar7-system

# 7. Scale back up gradually
kubectl scale deployment beskar7-controller-manager -n beskar7-system --replicas=3

echo "✅ Split-brain recovery completed"
```

### Queue Drain and Reset

If the provisioning queue becomes corrupted:

```bash
#!/bin/bash
echo "🔄 Emergency queue drain and reset"

# 1. Get current queue status
kubectl get --raw /metrics | grep beskar7_provisioning_queue_length

# 2. Stop all workers
kubectl patch configmap beskar7-config -n beskar7-system \
  --patch '{"data":{"provisioning.maxConcurrentOps":"0"}}'

# 3. Restart controllers to pick up new config
kubectl rollout restart deployment/beskar7-controller-manager -n beskar7-system
kubectl rollout status deployment/beskar7-controller-manager -n beskar7-system

# 4. Wait for queue to drain
echo "Waiting for queue to drain..."
while true; do
    QUEUE_LENGTH=$(kubectl get --raw /metrics | grep beskar7_provisioning_queue_length | awk '{print $2}' | head -1)
    if [[ "$QUEUE_LENGTH" == "0" ]]; then
        break
    fi
    echo "Queue length: $QUEUE_LENGTH"
    sleep 10
done

# 5. Reset queue configuration
kubectl patch configmap beskar7-config -n beskar7-system \
  --patch '{"data":{"provisioning.maxConcurrentOps":"5"}}'

# 6. Restart controllers again
kubectl rollout restart deployment/beskar7-controller-manager -n beskar7-system

echo "✅ Queue reset completed"
```

### Host State Recovery

If hosts become stuck in invalid states:

```bash
#!/bin/bash
echo "🔧 Host state recovery procedure"

# 1. Find hosts in error state
ERROR_HOSTS=$(kubectl get physicalhosts --all-namespaces -o jsonpath='{.items[?(@.status.state=="Error")].metadata.name}')

if [[ -z "$ERROR_HOSTS" ]]; then
    echo "No hosts in error state"
    exit 0
fi

echo "Hosts in error state: $ERROR_HOSTS"

# 2. For each error host, attempt recovery
for host in $ERROR_HOSTS; do
    echo "Recovering host: $host"
    
    # Get host namespace
    NAMESPACE=$(kubectl get physicalhost $host --all-namespaces -o jsonpath='{.items[0].metadata.namespace}')
    
    # Clear consumer reference if present
    kubectl patch physicalhost $host -n $NAMESPACE --type='json' \
      -p='[{"op": "remove", "path": "/spec/consumerRef"}]' || true
    
    # Reset state to enrolling to trigger re-enrollment
    kubectl patch physicalhost $host -n $NAMESPACE --type='json' \
      -p='[{"op": "replace", "path": "/status/state", "value": "Enrolling"}]'
    
    echo "Reset $host to Enrolling state"
done

echo "✅ Host state recovery initiated"
echo "Monitor host states with: kubectl get physicalhosts --all-namespaces -w"
```

## Monitoring and Alerting Setup

### Prometheus Rules

```yaml
# concurrent-provisioning-alerts.yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: beskar7-concurrent-provisioning
  namespace: beskar7-system
spec:
  groups:
  - name: beskar7.concurrent.provisioning
    interval: 30s
    rules:
    # Recording rules for better performance
    - record: beskar7:host_claim_conflict_rate_5m
      expr: |
        (
          rate(beskar7_host_claim_attempts_total{outcome="conflict"}[5m]) /
          rate(beskar7_host_claim_attempts_total[5m])
        ) * 100
        
    - record: beskar7:provisioning_queue_utilization
      expr: |
        beskar7_provisioning_queue_length / 50 * 100
        
    - record: beskar7:bmc_cooldown_rate_5m
      expr: |
        rate(beskar7_bmc_cooldown_waits_total[5m])
        
    # Alerting rules
    - alert: HighClaimConflictRate
      expr: beskar7:host_claim_conflict_rate_5m > 15
      for: 3m
      labels:
        severity: warning
        component: concurrent-provisioning
      annotations:
        summary: "High host claim conflict rate"
        description: "{{ $value }}% of host claims are conflicting in namespace {{ $labels.namespace }}"
        runbook_url: "https://docs.beskar7.io/runbooks/claim-conflicts"
        
    - alert: ProvisioningQueueNearFull
      expr: beskar7:provisioning_queue_utilization > 80
      for: 2m
      labels:
        severity: warning
        component: concurrent-provisioning
      annotations:
        summary: "Provisioning queue utilization high"
        description: "Queue is {{ $value }}% full, may start rejecting requests"
        
    - alert: ProvisioningQueueFull
      expr: beskar7:provisioning_queue_utilization >= 95
      for: 1m
      labels:
        severity: critical
        component: concurrent-provisioning
      annotations:
        summary: "Provisioning queue critical"
        description: "Queue is {{ $value }}% full, rejecting new requests"
        
    - alert: BMCOverload
      expr: beskar7:bmc_cooldown_rate_5m > 0.3
      for: 5m
      labels:
        severity: warning
        component: concurrent-provisioning
      annotations:
        summary: "BMC experiencing high load"
        description: "BMC {{ $labels.bmc_address }} cooldown rate: {{ $value }}/sec"
        
    - alert: LeaderElectionInstability
      expr: rate(beskar7_claim_coordinator_leader_election_total[10m]) > 0.05
      for: 5m
      labels:
        severity: warning
        component: concurrent-provisioning
      annotations:
        summary: "Frequent leader election changes"
        description: "Leadership changing {{ $value }} times per minute"
        
    - alert: ConcurrentProvisioningDown
      expr: up{job="beskar7-controller-manager"} == 0
      for: 1m
      labels:
        severity: critical
        component: concurrent-provisioning
      annotations:
        summary: "Beskar7 controller manager down"
        description: "No healthy beskar7-controller-manager instances"
```

### Grafana Dashboard JSON

```json
{
  "dashboard": {
    "id": null,
    "title": "Beskar7 Concurrent Provisioning",
    "tags": ["beskar7", "concurrent-provisioning"],
    "timezone": "browser",
    "panels": [
      {
        "title": "Claim Success Rate",
        "type": "stat",
        "targets": [
          {
            "expr": "rate(beskar7_host_claim_attempts_total{outcome=\"success\"}[5m]) / rate(beskar7_host_claim_attempts_total[5m]) * 100",
            "legendFormat": "Success Rate %"
          }
        ],
        "fieldConfig": {
          "defaults": {
            "unit": "percent",
            "thresholds": {
              "steps": [
                {"color": "red", "value": 0},
                {"color": "yellow", "value": 85},
                {"color": "green", "value": 95}
              ]
            }
          }
        }
      },
      {
        "title": "Provisioning Queue Length",
        "type": "graph",
        "targets": [
          {
            "expr": "beskar7_provisioning_queue_length",
            "legendFormat": "Queue Length"
          },
          {
            "expr": "beskar7_provisioning_queue_processing_count",
            "legendFormat": "Processing"
          }
        ]
      },
      {
        "title": "Claim Conflicts by Reason",
        "type": "graph",
        "targets": [
          {
            "expr": "sum by (conflict_reason) (rate(beskar7_host_claim_attempts_total{outcome=\"conflict\"}[5m]))",
            "legendFormat": "{{ conflict_reason }}"
          }
        ]
      }
    ]
  }
}
```

## Prevention Best Practices

### 1. Proactive Monitoring

Set up comprehensive monitoring before issues occur:

```bash
#!/bin/bash
# Daily health check script (run via cron)

LOG_FILE="/var/log/beskar7-health-check.log"
DATE=$(date)

echo "$DATE: Starting daily health check" >> $LOG_FILE

# Check for accumulating conflicts
CONFLICT_RATE=$(kubectl get --raw /metrics | grep beskar7_host_claim_attempts_total | awk '/conflict/ {conflicts+=$2} /total/ {total+=$2} END {if(total>0) print (conflicts/total)*100; else print 0}')

if (( $(echo "$CONFLICT_RATE > 10" | bc -l) )); then
    echo "$DATE: WARNING - High conflict rate: $CONFLICT_RATE%" >> $LOG_FILE
    # Send alert notification
    curl -X POST "$SLACK_WEBHOOK_URL" -H 'Content-type: application/json' \
      --data '{"text":"⚠️ Beskar7 high conflict rate: '$CONFLICT_RATE'%"}'
fi

# Check queue utilization
QUEUE_UTIL=$(kubectl get --raw /metrics | grep beskar7_provisioning_queue_length | awk '{print ($2/50)*100}')
if (( $(echo "$QUEUE_UTIL > 70" | bc -l) )); then
    echo "$DATE: WARNING - High queue utilization: $QUEUE_UTIL%" >> $LOG_FILE
fi

echo "$DATE: Health check completed" >> $LOG_FILE
```

### 2. Capacity Planning

Regular capacity assessment:

```bash
#!/bin/bash
# Weekly capacity planning report

echo "=== Beskar7 Capacity Planning Report ==="
echo "Date: $(date)"

# Host pool analysis
TOTAL_HOSTS=$(kubectl get physicalhosts --all-namespaces --no-headers | wc -l)
AVAILABLE_HOSTS=$(kubectl get physicalhosts --all-namespaces -o jsonpath='{.items[?(@.status.state=="Available")].metadata.name}' | wc -w)
UTILIZATION=$(( (TOTAL_HOSTS - AVAILABLE_HOSTS) * 100 / TOTAL_HOSTS ))

echo "Host Pool Status:"
echo "  Total hosts: $TOTAL_HOSTS"
echo "  Available: $AVAILABLE_HOSTS"
echo "  Utilization: $UTILIZATION%"

if [[ $UTILIZATION -gt 80 ]]; then
    echo "  ⚠️  WARNING: High utilization, consider adding more hosts"
fi

# Controller resource analysis
echo
echo "Controller Resource Usage:"
kubectl top pod -n beskar7-system | grep beskar7-controller

# Queue capacity analysis
echo
echo "Queue Metrics (last 24h max):"
QUEUE_MAX=$(kubectl get --raw /metrics | grep beskar7_provisioning_queue_length | awk '{print $2}' | sort -n | tail -1)
echo "  Maximum queue length: $QUEUE_MAX"

if [[ $QUEUE_MAX -gt 40 ]]; then
    echo "  ⚠️  WARNING: Queue approaching capacity, consider increasing maxQueueSize"
fi
```

### 3. Configuration Validation

Validate configuration changes before applying:

```bash
#!/bin/bash
# Configuration validation script

validate_config() {
    local CONFIG_FILE=$1
    
    echo "Validating configuration: $CONFIG_FILE"
    
    # Check required fields
    if ! grep -q "provisioning.maxConcurrentOps" "$CONFIG_FILE"; then
        echo "❌ Missing: provisioning.maxConcurrentOps"
        return 1
    fi
    
    # Validate numeric values
    MAX_CONCURRENT=$(grep "provisioning.maxConcurrentOps" "$CONFIG_FILE" | cut -d'"' -f4)
    if [[ ! "$MAX_CONCURRENT" =~ ^[0-9]+$ ]] || [[ $MAX_CONCURRENT -lt 1 ]] || [[ $MAX_CONCURRENT -gt 20 ]]; then
        echo "❌ Invalid maxConcurrentOps: $MAX_CONCURRENT (should be 1-20)"
        return 1
    fi
    
    # Validate cooldown period
    COOLDOWN=$(grep "provisioning.bmcCooldownPeriod" "$CONFIG_FILE" | cut -d'"' -f4)
    if [[ ! "$COOLDOWN" =~ ^[0-9]+s$ ]]; then
        echo "❌ Invalid bmcCooldownPeriod format: $COOLDOWN (should be like '10s')"
        return 1
    fi
    
    echo "✅ Configuration validation passed"
    return 0
}

# Usage
validate_config "new-config.yaml"
```

This comprehensive troubleshooting guide provides the necessary tools and procedures to diagnose, resolve, and prevent concurrent provisioning issues in Beskar7. Regular use of these procedures will help maintain a healthy and efficient provisioning system. 