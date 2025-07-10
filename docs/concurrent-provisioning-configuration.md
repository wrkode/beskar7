# Concurrent Provisioning Configuration Guide

This guide provides detailed configuration instructions for Beskar7's concurrent provisioning system, including deployment scenarios, performance tuning, and operational parameters.

## Configuration Overview

Beskar7's concurrent provisioning system offers two coordination modes with extensive configuration options:

1. **Standard Optimistic Locking** (Default)
2. **Leader Election Coordination** (High Contention)

## Deployment Configurations

### Development Environment

Small-scale development with minimal resource requirements:

```yaml
# dev-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: beskar7-controller-manager
  namespace: beskar7-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: beskar7-controller-manager
  template:
    metadata:
      labels:
        app: beskar7-controller-manager
    spec:
      containers:
      - name: manager
        image: beskar7/controller:latest
        args:
        - --leader-elect=false  # Single replica, no leader election needed
        - --max-concurrent-reconciles=3
        - --metrics-bind-address=:8080
        - --health-probe-bind-address=:8081
        resources:
          requests:
            memory: "128Mi"
            cpu: "100m"
          limits:
            memory: "256Mi"
            cpu: "200m"
        env:
        - name: BESKAR7_LOG_LEVEL
          value: "debug"  # More verbose logging for development
        ports:
        - containerPort: 8080
          name: metrics
        - containerPort: 8081
          name: health
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8081
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8081
          initialDelaySeconds: 5
          periodSeconds: 5
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: beskar7-config
  namespace: beskar7-system
data:
  # Conservative settings for development
  provisioning.maxConcurrentOps: "2"
  provisioning.maxQueueSize: "10"
  provisioning.operationTimeout: "10m"
  provisioning.bmcCooldownPeriod: "15s"
  provisioning.workerCount: "2"
  
  # Host selection configuration
  hostSelection.algorithm: "deterministic"
  hostSelection.retryLimit: "3"
  hostSelection.backoffMultiplier: "1.5"
  
  # Logging and debugging
  log.level: "debug"
  log.stacktraceLevel: "error"
```

### Staging Environment

Medium-scale staging with realistic production-like settings:

```yaml
# staging-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: beskar7-controller-manager
  namespace: beskar7-system
spec:
  replicas: 2
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 1
      maxSurge: 1
  selector:
    matchLabels:
      app: beskar7-controller-manager
  template:
    metadata:
      labels:
        app: beskar7-controller-manager
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8080"
        prometheus.io/path: "/metrics"
    spec:
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchLabels:
                  app: beskar7-controller-manager
              topologyKey: kubernetes.io/hostname
      containers:
      - name: manager
        image: beskar7/controller:latest
        args:
        - --leader-elect=true
        - --leader-elect-lease-duration=15s
        - --leader-elect-renew-deadline=10s
        - --leader-elect-retry-period=2s
        - --max-concurrent-reconciles=8
        - --metrics-bind-address=:8080
        - --health-probe-bind-address=:8081
        resources:
          requests:
            memory: "256Mi"
            cpu: "200m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        env:
        - name: BESKAR7_LOG_LEVEL
          value: "info"
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        - name: POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        ports:
        - containerPort: 8080
          name: metrics
        - containerPort: 8081
          name: health
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8081
          initialDelaySeconds: 30
          periodSeconds: 20
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8081
          initialDelaySeconds: 5
          periodSeconds: 10
        securityContext:
          allowPrivilegeEscalation: false
          runAsNonRoot: true
          runAsUser: 1000
          capabilities:
            drop:
            - ALL
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: beskar7-config
  namespace: beskar7-system
data:
  # Balanced settings for staging
  provisioning.maxConcurrentOps: "5"
  provisioning.maxQueueSize: "30"
  provisioning.operationTimeout: "8m"
  provisioning.bmcCooldownPeriod: "10s"
  provisioning.workerCount: "4"
  provisioning.retryAttempts: "3"
  
  # Host selection configuration
  hostSelection.algorithm: "deterministic"
  hostSelection.retryLimit: "5"
  hostSelection.backoffMultiplier: "2.0"
  
  # Performance tuning
  reconcile.syncPeriod: "30s"
  reconcile.timeout: "5m"
  
  # Monitoring
  metrics.interval: "30s"
  log.level: "info"
```

### Production Environment

High-availability production deployment with advanced coordination:

```yaml
# production-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: beskar7-controller-manager
  namespace: beskar7-system
  labels:
    app: beskar7-controller-manager
    version: v1.0.0
spec:
  replicas: 3
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 1
      maxSurge: 1
  selector:
    matchLabels:
      app: beskar7-controller-manager
  template:
    metadata:
      labels:
        app: beskar7-controller-manager
        version: v1.0.0
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8080"
        prometheus.io/path: "/metrics"
    spec:
      serviceAccountName: beskar7-controller-manager
      affinity:
        podAntiAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
          - labelSelector:
              matchLabels:
                app: beskar7-controller-manager
            topologyKey: kubernetes.io/hostname
        nodeAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            preference:
              matchExpressions:
              - key: node-role.kubernetes.io/master
                operator: DoesNotExist
      tolerations:
      - key: node-role.kubernetes.io/master
        operator: Exists
        effect: NoSchedule
      containers:
      - name: manager
        image: beskar7/controller:v1.0.0
        imagePullPolicy: IfNotPresent
        args:
        # Standard leader election
        - --leader-elect=true
        - --leader-elect-lease-duration=15s
        - --leader-elect-renew-deadline=10s
        - --leader-elect-retry-period=2s
        - --leader-elect-resource-namespace=beskar7-system
        
        # Claim coordinator leader election (advanced coordination)
        - --enable-claim-coordinator-leader-election=true
        - --claim-coordinator-lease-duration=15s
        - --claim-coordinator-renew-deadline=10s
        - --claim-coordinator-retry-period=2s
        
        # Performance settings
        - --max-concurrent-reconciles=12
        - --kube-api-qps=20
        - --kube-api-burst=30
        
        # Observability
        - --metrics-bind-address=:8080
        - --health-probe-bind-address=:8081
        - --enable-pprof=true
        - --pprof-bind-address=:8082
        
        resources:
          requests:
            memory: "512Mi"
            cpu: "300m"
          limits:
            memory: "1Gi"
            cpu: "1000m"
        env:
        - name: BESKAR7_LOG_LEVEL
          value: "info"
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        - name: POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: POD_IP
          valueFrom:
            fieldRef:
              fieldPath: status.podIP
        - name: NODE_NAME
          valueFrom:
            fieldRef:
              fieldPath: spec.nodeName
        ports:
        - containerPort: 8080
          name: metrics
          protocol: TCP
        - containerPort: 8081
          name: health
          protocol: TCP
        - containerPort: 8082
          name: pprof
          protocol: TCP
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8081
          initialDelaySeconds: 30
          periodSeconds: 20
          timeoutSeconds: 5
          failureThreshold: 3
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8081
          initialDelaySeconds: 5
          periodSeconds: 10
          timeoutSeconds: 5
          failureThreshold: 3
        securityContext:
          allowPrivilegeEscalation: false
          runAsNonRoot: true
          runAsUser: 1000
          runAsGroup: 2000
          capabilities:
            drop:
            - ALL
          readOnlyRootFilesystem: true
        volumeMounts:
        - name: tmp
          mountPath: /tmp
        - name: cache
          mountPath: /home/controller/.cache
      volumes:
      - name: tmp
        emptyDir: {}
      - name: cache
        emptyDir: {}
      terminationGracePeriodSeconds: 30
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        runAsGroup: 2000
        fsGroup: 2000
        seccompProfile:
          type: RuntimeDefault
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: beskar7-config
  namespace: beskar7-system
data:
  # High-performance production settings
  provisioning.maxConcurrentOps: "10"
  provisioning.maxQueueSize: "75"
  provisioning.operationTimeout: "5m"
  provisioning.bmcCooldownPeriod: "8s"
  provisioning.workerCount: "8"
  provisioning.retryAttempts: "5"
  
  # Host selection optimization
  hostSelection.algorithm: "deterministic"
  hostSelection.retryLimit: "5"
  hostSelection.backoffMultiplier: "1.5"
  hostSelection.maxBackoffDelay: "30s"
  
  # BMC-specific tuning
  bmc.connectionTimeout: "30s"
  bmc.requestTimeout: "60s"
  bmc.maxRetries: "3"
  bmc.retryDelay: "5s"
  
  # Performance optimization
  reconcile.syncPeriod: "10s"
  reconcile.timeout: "3m"
  reconcile.maxConcurrentReconciles: "12"
  
  # Leader election tuning
  leaderElection.leaseDuration: "15s"
  leaderElection.renewDeadline: "10s"
  leaderElection.retryPeriod: "2s"
  
  # Monitoring and observability
  metrics.interval: "15s"
  metrics.enableProfiling: "true"
  log.level: "info"
  log.format: "json"
  log.timeEncoding: "iso8601"
---
# Pod Disruption Budget for high availability
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: beskar7-controller-manager
  namespace: beskar7-system
spec:
  minAvailable: 2
  selector:
    matchLabels:
      app: beskar7-controller-manager
```

### Large Scale Production

For environments with 100+ hosts and high concurrency requirements:

```yaml
# large-scale-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: beskar7-controller-manager
  namespace: beskar7-system
spec:
  replicas: 5  # Higher replica count for better distribution
  template:
    spec:
      containers:
      - name: manager
        args:
        # Enhanced leader election settings
        - --enable-claim-coordinator-leader-election=true
        - --claim-coordinator-lease-duration=20s     # Longer for stability
        - --claim-coordinator-renew-deadline=15s
        - --claim-coordinator-retry-period=3s
        
        # Higher concurrency
        - --max-concurrent-reconciles=20
        - --kube-api-qps=50
        - --kube-api-burst=100
        
        resources:
          requests:
            memory: "1Gi"
            cpu: "500m"
          limits:
            memory: "2Gi"
            cpu: "2000m"
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: beskar7-config
  namespace: beskar7-system
data:
  # Aggressive settings for large scale
  provisioning.maxConcurrentOps: "20"
  provisioning.maxQueueSize: "150"
  provisioning.operationTimeout: "4m"
  provisioning.bmcCooldownPeriod: "5s"   # Shorter cooldown
  provisioning.workerCount: "15"
  
  # Enhanced retries
  provisioning.retryAttempts: "7"
  provisioning.exponentialBackoff: "true"
  provisioning.maxRetryDelay: "60s"
```

## Configuration Parameters Reference

### Core Provisioning Settings

| Parameter | Default | Range | Description | Production Recommendation |
|-----------|---------|--------|-------------|---------------------------|
| `provisioning.maxConcurrentOps` | `5` | `1-50` | Maximum concurrent BMC operations | Start with 8-12, tune based on BMC performance |
| `provisioning.maxQueueSize` | `50` | `10-500` | Maximum queued operations | 2-3x peak hourly demand |
| `provisioning.operationTimeout` | `5m` | `1m-30m` | Individual operation timeout | 3-8m based on BMC response times |
| `provisioning.bmcCooldownPeriod` | `10s` | `1s-60s` | Delay between BMC operations | 5-15s, vendor-dependent |
| `provisioning.workerCount` | `3` | `1-20` | Number of queue worker goroutines | 1.5-2x maxConcurrentOps |

### Leader Election Coordination

| Parameter | Default | Range | Description | Use Case |
|-----------|---------|--------|-------------|----------|
| `--enable-claim-coordinator-leader-election` | `false` | boolean | Enable advanced coordination | Conflict rate >10% or >50 hosts |
| `--claim-coordinator-lease-duration` | `15s` | `5s-60s` | Leader lease duration | 15-30s for production |
| `--claim-coordinator-renew-deadline` | `10s` | `3s-45s` | Lease renewal deadline | 60-80% of lease duration |
| `--claim-coordinator-retry-period` | `2s` | `1s-10s` | Leadership acquisition retry | 1-5s based on urgency |

### Host Selection Algorithm

| Parameter | Default | Options | Description | Recommendation |
|-----------|---------|---------|-------------|----------------|
| `hostSelection.algorithm` | `deterministic` | `deterministic`, `round-robin`, `priority` | Host selection method | `deterministic` for consistency |
| `hostSelection.retryLimit` | `3` | `1-10` | Max retries for host selection | 3-5 for production |
| `hostSelection.backoffMultiplier` | `1.5` | `1.0-3.0` | Exponential backoff multiplier | 1.5-2.0 for balanced retry |
| `hostSelection.maxBackoffDelay` | `30s` | `5s-300s` | Maximum retry delay | 30-60s to prevent long delays |

### BMC Connection Settings

| Parameter | Default | Range | Description | Tuning Guide |
|-----------|---------|--------|-------------|--------------|
| `bmc.connectionTimeout` | `30s` | `5s-120s` | TCP connection timeout | 15-45s based on network |
| `bmc.requestTimeout` | `60s` | `10s-300s` | HTTP request timeout | 30-120s based on BMC vendor |
| `bmc.maxRetries` | `3` | `1-10` | Maximum request retries | 3-5 for reliability |
| `bmc.retryDelay` | `5s` | `1s-30s` | Delay between retries | 2-10s progressive delay |

### Performance Tuning

| Parameter | Default | Range | Description | Scaling Guide |
|-----------|---------|--------|-------------|---------------|
| `reconcile.syncPeriod` | `30s` | `5s-300s` | Full reconciliation interval | 10-60s based on cluster size |
| `reconcile.timeout` | `5m` | `1m-30m` | Reconciliation timeout | 3-10m for complex operations |
| `--max-concurrent-reconciles` | `10` | `1-50` | Concurrent reconcile operations | 1 per 5-10 hosts |
| `--kube-api-qps` | `20` | `5-100` | Kubernetes API requests/sec | Scale with cluster operations |
| `--kube-api-burst` | `30` | `10-200` | Kubernetes API burst limit | 1.5x QPS |

## Environment-Specific Configurations

### Network Latency Optimization

For environments with high network latency to BMCs:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: beskar7-config-latency-optimized
data:
  # Increased timeouts for high latency
  bmc.connectionTimeout: "60s"
  bmc.requestTimeout: "180s"
  provisioning.operationTimeout: "15m"
  
  # Longer cooldowns to account for slower responses
  provisioning.bmcCooldownPeriod: "20s"
  
  # More conservative retries
  bmc.maxRetries: "5"
  bmc.retryDelay: "10s"
  
  # Reduced concurrency to prevent overload
  provisioning.maxConcurrentOps: "3"
```

### High-Throughput Optimization

For environments requiring maximum provisioning throughput:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: beskar7-config-high-throughput
data:
  # Aggressive concurrency settings
  provisioning.maxConcurrentOps: "15"
  provisioning.workerCount: "20"
  
  # Large queue capacity
  provisioning.maxQueueSize: "200"
  
  # Reduced timeouts for faster cycling
  provisioning.operationTimeout: "3m"
  bmc.requestTimeout: "45s"
  
  # Minimal cooldowns
  provisioning.bmcCooldownPeriod: "3s"
  
  # Fast retries
  hostSelection.backoffMultiplier: "1.2"
  bmc.retryDelay: "2s"
```

### Vendor-Specific Optimizations

#### Dell iDRAC Optimization

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: beskar7-config-dell-idrac
data:
  # Dell iDRAC performs well with moderate concurrency
  provisioning.maxConcurrentOps: "8"
  provisioning.bmcCooldownPeriod: "8s"
  
  # iDRAC can handle longer operations
  bmc.requestTimeout: "120s"
  provisioning.operationTimeout: "8m"
  
  # iDRAC-specific retry strategy
  bmc.maxRetries: "4"
  bmc.retryDelay: "5s"
```

#### HPE iLO Optimization

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: beskar7-config-hpe-ilo
data:
  # HPE iLO is generally fast and reliable
  provisioning.maxConcurrentOps: "12"
  provisioning.bmcCooldownPeriod: "5s"
  
  # iLO typically responds quickly
  bmc.requestTimeout: "60s"
  provisioning.operationTimeout: "5m"
  
  # iLO rarely needs many retries
  bmc.maxRetries: "3"
  bmc.retryDelay: "3s"
```

#### Supermicro BMC Optimization

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: beskar7-config-supermicro
data:
  # Supermicro BMCs can be slower
  provisioning.maxConcurrentOps: "4"
  provisioning.bmcCooldownPeriod: "15s"
  
  # Allow more time for operations
  bmc.requestTimeout: "90s"
  provisioning.operationTimeout: "10m"
  
  # More retries may be needed
  bmc.maxRetries: "5"
  bmc.retryDelay: "8s"
```

## Configuration Validation

### Pre-deployment Validation Script

```bash
#!/bin/bash
# validate-beskar7-config.sh
# Validates Beskar7 configuration before deployment

CONFIG_FILE=${1:-"beskar7-config.yaml"}
ERRORS=0

echo "🔍 Validating Beskar7 configuration: $CONFIG_FILE"

validate_numeric_range() {
    local param=$1
    local value=$2
    local min=$3
    local max=$4
    
    if [[ ! "$value" =~ ^[0-9]+$ ]]; then
        echo "❌ $param: '$value' is not a valid number"
        ((ERRORS++))
        return 1
    fi
    
    if [[ $value -lt $min || $value -gt $max ]]; then
        echo "❌ $param: $value is outside valid range [$min-$max]"
        ((ERRORS++))
        return 1
    fi
    
    echo "✅ $param: $value"
    return 0
}

validate_duration() {
    local param=$1
    local value=$2
    
    if [[ ! "$value" =~ ^[0-9]+[smh]$ ]]; then
        echo "❌ $param: '$value' is not a valid duration (e.g., '30s', '5m', '1h')"
        ((ERRORS++))
        return 1
    fi
    
    echo "✅ $param: $value"
    return 0
}

# Extract values from config
MAX_CONCURRENT=$(yq eval '.data."provisioning.maxConcurrentOps"' "$CONFIG_FILE" 2>/dev/null || echo "")
MAX_QUEUE_SIZE=$(yq eval '.data."provisioning.maxQueueSize"' "$CONFIG_FILE" 2>/dev/null || echo "")
OPERATION_TIMEOUT=$(yq eval '.data."provisioning.operationTimeout"' "$CONFIG_FILE" 2>/dev/null || echo "")
BMC_COOLDOWN=$(yq eval '.data."provisioning.bmcCooldownPeriod"' "$CONFIG_FILE" 2>/dev/null || echo "")
WORKER_COUNT=$(yq eval '.data."provisioning.workerCount"' "$CONFIG_FILE" 2>/dev/null || echo "")

# Validate core parameters
if [[ -n "$MAX_CONCURRENT" ]]; then
    validate_numeric_range "provisioning.maxConcurrentOps" "$MAX_CONCURRENT" 1 50
fi

if [[ -n "$MAX_QUEUE_SIZE" ]]; then
    validate_numeric_range "provisioning.maxQueueSize" "$MAX_QUEUE_SIZE" 10 500
fi

if [[ -n "$WORKER_COUNT" ]]; then
    validate_numeric_range "provisioning.workerCount" "$WORKER_COUNT" 1 20
fi

if [[ -n "$OPERATION_TIMEOUT" ]]; then
    validate_duration "provisioning.operationTimeout" "$OPERATION_TIMEOUT"
fi

if [[ -n "$BMC_COOLDOWN" ]]; then
    validate_duration "provisioning.bmcCooldownPeriod" "$BMC_COOLDOWN"
fi

# Validate relationships between parameters
if [[ -n "$MAX_CONCURRENT" && -n "$WORKER_COUNT" ]]; then
    if [[ $WORKER_COUNT -lt $MAX_CONCURRENT ]]; then
        echo "⚠️  Warning: workerCount ($WORKER_COUNT) is less than maxConcurrentOps ($MAX_CONCURRENT)"
        echo "   Recommendation: Set workerCount to at least 1.5x maxConcurrentOps"
    fi
fi

if [[ -n "$MAX_QUEUE_SIZE" && -n "$MAX_CONCURRENT" ]]; then
    RATIO=$((MAX_QUEUE_SIZE / MAX_CONCURRENT))
    if [[ $RATIO -lt 5 ]]; then
        echo "⚠️  Warning: Queue size to concurrency ratio is low ($RATIO)"
        echo "   Recommendation: Set maxQueueSize to at least 5x maxConcurrentOps"
    fi
fi

# Summary
echo
if [[ $ERRORS -eq 0 ]]; then
    echo "✅ Configuration validation passed"
    exit 0
else
    echo "❌ Configuration validation failed with $ERRORS errors"
    exit 1
fi
```

### Runtime Configuration Monitoring

Monitor configuration effectiveness in production:

```bash
#!/bin/bash
# monitor-config-effectiveness.sh
# Monitors the effectiveness of current configuration

echo "📊 Beskar7 Configuration Effectiveness Report"
echo "============================================="

# Queue utilization analysis
QUEUE_LENGTH=$(kubectl get --raw /metrics | grep beskar7_provisioning_queue_length | awk '{print $2}' | head -1)
MAX_QUEUE_SIZE=$(kubectl get configmap beskar7-config -n beskar7-system -o jsonpath='{.data.provisioning\.maxQueueSize}')
QUEUE_UTIL=$(( (QUEUE_LENGTH * 100) / MAX_QUEUE_SIZE ))

echo "Queue Utilization: $QUEUE_UTIL% ($QUEUE_LENGTH/$MAX_QUEUE_SIZE)"

if [[ $QUEUE_UTIL -gt 80 ]]; then
    echo "🚨 Action Required: Queue utilization high, consider increasing maxQueueSize"
elif [[ $QUEUE_UTIL -lt 20 ]]; then
    echo "💡 Optimization: Queue utilization low, could reduce maxQueueSize"
else
    echo "✅ Queue utilization optimal"
fi

# Conflict rate analysis
TOTAL_ATTEMPTS=$(kubectl get --raw /metrics | grep beskar7_host_claim_attempts_total | awk '{sum+=$2} END {print sum}')
CONFLICTS=$(kubectl get --raw /metrics | grep 'beskar7_host_claim_attempts_total.*outcome="conflict"' | awk '{sum+=$2} END {print sum}')

if [[ $TOTAL_ATTEMPTS -gt 0 && -n $CONFLICTS ]]; then
    CONFLICT_RATE=$(( (CONFLICTS * 100) / TOTAL_ATTEMPTS ))
    echo "Conflict Rate: $CONFLICT_RATE%"
    
    if [[ $CONFLICT_RATE -gt 15 ]]; then
        echo "🚨 Action Required: High conflict rate, consider enabling leader election coordination"
    elif [[ $CONFLICT_RATE -lt 5 ]]; then
        echo "✅ Conflict rate optimal"
    else
        echo "⚠️  Monitoring: Moderate conflict rate, monitor trends"
    fi
fi

# BMC performance analysis
BMC_COOLDOWNS=$(kubectl get --raw /metrics | grep beskar7_bmc_cooldown_waits_total | awk '{sum+=$2} END {print sum}')
echo "BMC Cooldown Events: $BMC_COOLDOWNS"

if [[ $BMC_COOLDOWNS -gt 100 ]]; then
    echo "🚨 Action Required: Frequent BMC cooldowns, consider increasing bmcCooldownPeriod"
fi

# Performance recommendations
echo
echo "📈 Performance Recommendations:"

# Check if leader election is enabled for high contention
LEADER_ELECTION=$(kubectl get deployment beskar7-controller-manager -n beskar7-system -o jsonpath='{.spec.template.spec.containers[0].args}' | grep -o "enable-claim-coordinator-leader-election=true" || echo "disabled")

if [[ $CONFLICT_RATE -gt 10 && "$LEADER_ELECTION" == "disabled" ]]; then
    echo "  • Enable leader election coordination for better conflict resolution"
fi

if [[ $QUEUE_UTIL -gt 70 ]]; then
    echo "  • Increase maxQueueSize to handle higher load"
fi

if [[ $BMC_COOLDOWNS -gt 50 ]]; then
    echo "  • Increase bmcCooldownPeriod to reduce BMC load"
fi
```

## Configuration Migration

### Upgrading Configuration

When upgrading from basic to advanced configuration:

```bash
#!/bin/bash
# migrate-to-advanced-config.sh
# Safely migrates from basic to advanced concurrent provisioning configuration

echo "🔄 Migrating to advanced concurrent provisioning configuration"

# Backup current configuration
kubectl get configmap beskar7-config -n beskar7-system -o yaml > beskar7-config-backup-$(date +%Y%m%d-%H%M%S).yaml
echo "✅ Current configuration backed up"

# Apply new configuration gradually
echo "📝 Applying advanced configuration..."

# Stage 1: Increase queue capacity first
kubectl patch configmap beskar7-config -n beskar7-system \
  --patch '{"data":{"provisioning.maxQueueSize":"75"}}'

# Stage 2: Enable leader election coordination
kubectl patch deployment beskar7-controller-manager -n beskar7-system \
  --patch '{"spec":{"template":{"spec":{"containers":[{"name":"manager","args":["--enable-claim-coordinator-leader-election=true"]}]}}}}'

# Stage 3: Optimize performance settings
kubectl patch configmap beskar7-config -n beskar7-system \
  --patch '{"data":{"provisioning.maxConcurrentOps":"8","provisioning.workerCount":"10"}}'

echo "✅ Advanced configuration applied"
echo "🔍 Monitor metrics for 10 minutes to ensure stability"

# Monitor for 10 minutes
for i in {1..10}; do
    sleep 60
    QUEUE_LENGTH=$(kubectl get --raw /metrics | grep beskar7_provisioning_queue_length | awk '{print $2}' | head -1)
    CONFLICTS=$(kubectl get --raw /metrics | grep 'beskar7_host_claim_attempts_total.*outcome="conflict"' | awk '{sum+=$2} END {print sum}')
    echo "Minute $i: Queue=$QUEUE_LENGTH, Conflicts=$CONFLICTS"
done

echo "✅ Migration monitoring completed"
```

This comprehensive configuration guide provides the foundation for optimal concurrent provisioning performance across different environments and use cases. 