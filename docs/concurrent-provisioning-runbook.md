# Concurrent Provisioning Operations Runbook

This runbook provides step-by-step procedures for operating Beskar7's concurrent provisioning system, including routine maintenance, incident response, and optimization tasks.

## Daily Operations

### Morning Health Check

Execute this daily health check to ensure system wellness:

```bash
#!/bin/bash
# daily-health-check.sh
# Run this every morning to check concurrent provisioning health

echo "🌅 Daily Beskar7 Concurrent Provisioning Health Check"
echo "Date: $(date)"
echo "=================================================="

# Check controller deployment health
echo "1. Controller Deployment Status:"
kubectl get deployment beskar7-controller-manager -n beskar7-system
kubectl get pods -n beskar7-system -l app=beskar7-controller-manager

# Check leader election status
echo -e "\n2. Leader Election Status:"
CURRENT_LEADER=$(kubectl get lease beskar7-controller-manager -n beskar7-system -o jsonpath='{.spec.holderIdentity}' 2>/dev/null || echo "No leader")
echo "Current leader: $CURRENT_LEADER"

CLAIM_LEADER=$(kubectl get lease beskar7-claim-coordinator-leader -n beskar7-system -o jsonpath='{.spec.holderIdentity}' 2>/dev/null || echo "Disabled")
echo "Claim coordinator leader: $CLAIM_LEADER"

# Check host pool status
echo -e "\n3. Host Pool Status:"
TOTAL_HOSTS=$(kubectl get physicalhosts --all-namespaces --no-headers | wc -l)
AVAILABLE_HOSTS=$(kubectl get physicalhosts --all-namespaces -o jsonpath='{.items[?(@.status.state=="Available")].metadata.name}' | wc -w)
ERROR_HOSTS=$(kubectl get physicalhosts --all-namespaces -o jsonpath='{.items[?(@.status.state=="Error")].metadata.name}' | wc -w)

echo "Total hosts: $TOTAL_HOSTS"
echo "Available hosts: $AVAILABLE_HOSTS"
echo "Error hosts: $ERROR_HOSTS"

if [[ $ERROR_HOSTS -gt 0 ]]; then
    echo "⚠️  Hosts in error state need attention"
    kubectl get physicalhosts --all-namespaces -o custom-columns=NAME:.metadata.name,NAMESPACE:.metadata.namespace,STATE:.status.state | grep Error
fi

# Check provisioning queue metrics
echo -e "\n4. Provisioning Queue Metrics:"
if kubectl get --raw /metrics >/dev/null 2>&1; then
    QUEUE_LENGTH=$(kubectl get --raw /metrics | grep beskar7_provisioning_queue_length | awk '{print $2}' | head -1)
    echo "Current queue length: ${QUEUE_LENGTH:-0}"
    
    # Conflict rate calculation
    TOTAL_ATTEMPTS=$(kubectl get --raw /metrics | grep beskar7_host_claim_attempts_total | awk '{sum+=$2} END {print sum}')
    CONFLICTS=$(kubectl get --raw /metrics | grep 'beskar7_host_claim_attempts_total.*outcome="conflict"' | awk '{sum+=$2} END {print sum}')
    
    if [[ $TOTAL_ATTEMPTS -gt 0 && -n $CONFLICTS ]]; then
        CONFLICT_RATE=$(( (CONFLICTS * 100) / TOTAL_ATTEMPTS ))
        echo "Overall conflict rate: $CONFLICT_RATE%"
        
        if [[ $CONFLICT_RATE -gt 15 ]]; then
            echo "⚠️  High conflict rate detected"
        fi
    fi
else
    echo "⚠️  Metrics endpoint not accessible"
fi

# Check recent errors
echo -e "\n5. Recent Errors (last 24h):"
ERROR_COUNT=$(kubectl get events --all-namespaces --field-selector type=Warning --sort-by='.lastTimestamp' | grep beskar7 | wc -l)
echo "Warning events: $ERROR_COUNT"

if [[ $ERROR_COUNT -gt 0 ]]; then
    echo "Recent warning events:"
    kubectl get events --all-namespaces --field-selector type=Warning --sort-by='.lastTimestamp' | grep beskar7 | tail -5
fi

# Summary and recommendations
echo -e "\n📋 Daily Summary:"
if [[ $ERROR_HOSTS -eq 0 && $ERROR_COUNT -lt 5 && ${CONFLICT_RATE:-0} -lt 15 ]]; then
    echo "✅ System is healthy"
else
    echo "⚠️  Issues detected requiring attention:"
    [[ $ERROR_HOSTS -gt 0 ]] && echo "  - $ERROR_HOSTS hosts in error state"
    [[ $ERROR_COUNT -ge 5 ]] && echo "  - High number of warning events"
    [[ ${CONFLICT_RATE:-0} -ge 15 ]] && echo "  - High claim conflict rate"
fi

echo "Next check: $(date -d 'tomorrow' '+%Y-%m-%d 09:00')"
```

### Weekly Performance Review

```bash
#!/bin/bash
# weekly-performance-review.sh
# Comprehensive weekly performance analysis

echo "📊 Weekly Concurrent Provisioning Performance Review"
echo "Week ending: $(date)"
echo "================================================="

# Calculate key metrics over the past week
echo "1. Performance Metrics (7-day period):"

# Host claim performance
echo "Host Claim Performance:"
TOTAL_CLAIMS=$(kubectl get --raw /metrics | grep beskar7_host_claim_attempts_total | awk '{sum+=$2} END {print sum}')
SUCCESSFUL_CLAIMS=$(kubectl get --raw /metrics | grep 'beskar7_host_claim_attempts_total.*outcome="success"' | awk '{sum+=$2} END {print sum}')
FAILED_CLAIMS=$(kubectl get --raw /metrics | grep 'beskar7_host_claim_attempts_total.*outcome="failure"' | awk '{sum+=$2} END {print sum}')

echo "  Total claims: $TOTAL_CLAIMS"
echo "  Successful: $SUCCESSFUL_CLAIMS"
echo "  Failed: $FAILED_CLAIMS"

if [[ $TOTAL_CLAIMS -gt 0 ]]; then
    SUCCESS_RATE=$(( (SUCCESSFUL_CLAIMS * 100) / TOTAL_CLAIMS ))
    echo "  Success rate: $SUCCESS_RATE%"
fi

# Queue performance
echo -e "\nQueue Performance:"
MAX_QUEUE_SIZE=$(kubectl get configmap beskar7-config -n beskar7-system -o jsonpath='{.data.provisioning\.maxQueueSize}')
CURRENT_QUEUE=$(kubectl get --raw /metrics | grep beskar7_provisioning_queue_length | awk '{print $2}' | head -1)
echo "  Current queue length: $CURRENT_QUEUE"
echo "  Maximum queue size: $MAX_QUEUE_SIZE"
echo "  Utilization: $(( (CURRENT_QUEUE * 100) / MAX_QUEUE_SIZE ))%"

# BMC performance
echo -e "\nBMC Performance:"
BMC_COOLDOWNS=$(kubectl get --raw /metrics | grep beskar7_bmc_cooldown_waits_total | awk '{sum+=$2} END {print sum}')
echo "  Total BMC cooldown events: $BMC_COOLDOWNS"

# Resource utilization
echo -e "\n2. Resource Utilization:"
kubectl top pod -n beskar7-system --no-headers | grep beskar7-controller

# Trends and recommendations
echo -e "\n3. Trends and Recommendations:"

# Check for increasing conflict rate
RECENT_CONFLICT_RATE=${CONFLICT_RATE:-0}
if [[ $RECENT_CONFLICT_RATE -gt 10 ]]; then
    echo "📈 Trend: Increasing conflict rate"
    echo "   Recommendation: Consider enabling leader election coordination"
fi

# Check for queue pressure
QUEUE_UTIL=$(( (CURRENT_QUEUE * 100) / MAX_QUEUE_SIZE ))
if [[ $QUEUE_UTIL -gt 70 ]]; then
    echo "📈 Trend: High queue utilization"
    echo "   Recommendation: Increase maxQueueSize or maxConcurrentOps"
fi

# Check for BMC performance issues
if [[ $BMC_COOLDOWNS -gt 100 ]]; then
    echo "📈 Trend: Frequent BMC cooldowns"
    echo "   Recommendation: Increase bmcCooldownPeriod or check BMC health"
fi

echo -e "\n✅ Weekly review complete"
```

## Incident Response Procedures

### P1: Critical System Failure

**Symptoms:** All provisioning stopped, controllers not responding, massive error rates

```bash
#!/bin/bash
# p1-critical-response.sh
# Immediate response for critical system failures

echo "🚨 P1 CRITICAL: Concurrent Provisioning System Failure"
echo "Incident ID: BESKAR7-$(date +%Y%m%d-%H%M%S)"
echo "================================================="

# Step 1: Immediate assessment
echo "1. IMMEDIATE ASSESSMENT"
echo "Time: $(date)"

# Check controller status
CONTROLLER_STATUS=$(kubectl get deployment beskar7-controller-manager -n beskar7-system -o jsonpath='{.status.conditions[?(@.type=="Available")].status}' 2>/dev/null || echo "ERROR")
echo "Controller status: $CONTROLLER_STATUS"

# Check running pods
RUNNING_PODS=$(kubectl get pods -n beskar7-system -l app=beskar7-controller-manager --field-selector=status.phase=Running --no-headers | wc -l)
TOTAL_PODS=$(kubectl get pods -n beskar7-system -l app=beskar7-controller-manager --no-headers | wc -l)
echo "Running pods: $RUNNING_PODS/$TOTAL_PODS"

# Check API connectivity
if kubectl cluster-info >/dev/null 2>&1; then
    echo "✅ Kubernetes API accessible"
else
    echo "❌ Kubernetes API not accessible"
    exit 1
fi

# Step 2: Emergency stabilization
echo -e "\n2. EMERGENCY STABILIZATION"

if [[ "$CONTROLLER_STATUS" != "True" || $RUNNING_PODS -eq 0 ]]; then
    echo "🔄 Attempting controller restart..."
    
    # Scale down to 0
    kubectl scale deployment beskar7-controller-manager -n beskar7-system --replicas=0
    
    # Wait for graceful shutdown
    sleep 30
    
    # Scale back up to 1 (safe mode)
    kubectl scale deployment beskar7-controller-manager -n beskar7-system --replicas=1
    
    echo "⏳ Waiting for controller to stabilize..."
    kubectl wait --for=condition=available deployment/beskar7-controller-manager -n beskar7-system --timeout=300s
    
    if [[ $? -eq 0 ]]; then
        echo "✅ Controller restarted successfully"
    else
        echo "❌ Controller restart failed - escalating to engineering team"
        exit 1
    fi
fi

# Step 3: Verify basic functionality
echo -e "\n3. FUNCTIONALITY VERIFICATION"

# Check metrics endpoint
if kubectl get --raw /metrics >/dev/null 2>&1; then
    echo "✅ Metrics endpoint accessible"
else
    echo "❌ Metrics endpoint not accessible"
fi

# Check leader election
LEADER=$(kubectl get lease beskar7-controller-manager -n beskar7-system -o jsonpath='{.spec.holderIdentity}' 2>/dev/null || echo "NONE")
echo "Current leader: $LEADER"

# Step 4: Monitor recovery
echo -e "\n4. MONITORING RECOVERY"
echo "📊 Monitoring system recovery for 5 minutes..."

for i in {1..5}; do
    sleep 60
    PODS_READY=$(kubectl get pods -n beskar7-system -l app=beskar7-controller-manager --field-selector=status.phase=Running --no-headers | wc -l)
    QUEUE_LENGTH=$(kubectl get --raw /metrics 2>/dev/null | grep beskar7_provisioning_queue_length | awk '{print $2}' | head -1 || echo "N/A")
    echo "Minute $i: Pods ready: $PODS_READY, Queue length: $QUEUE_LENGTH"
done

echo -e "\n✅ P1 INCIDENT RESPONSE COMPLETE"
echo "📞 If issues persist, escalate to engineering on-call"
echo "📝 Document incident details in incident management system"
```

### P2: Performance Degradation

**Symptoms:** High latency, increasing conflict rates, queue backup

```bash
#!/bin/bash
# p2-performance-degradation.sh
# Response for performance degradation incidents

echo "⚠️  P2 PERFORMANCE: Concurrent Provisioning Degradation"
echo "Incident ID: BESKAR7-PERF-$(date +%Y%m%d-%H%M%S)"
echo "==============================================="

# Step 1: Performance assessment
echo "1. PERFORMANCE ASSESSMENT"

# Check conflict rate
TOTAL_ATTEMPTS=$(kubectl get --raw /metrics | grep beskar7_host_claim_attempts_total | awk '{sum+=$2} END {print sum}')
CONFLICTS=$(kubectl get --raw /metrics | grep 'beskar7_host_claim_attempts_total.*outcome="conflict"' | awk '{sum+=$2} END {print sum}')

if [[ $TOTAL_ATTEMPTS -gt 0 && -n $CONFLICTS ]]; then
    CONFLICT_RATE=$(( (CONFLICTS * 100) / TOTAL_ATTEMPTS ))
    echo "Current conflict rate: $CONFLICT_RATE%"
    
    if [[ $CONFLICT_RATE -gt 20 ]]; then
        echo "🚨 CRITICAL: Very high conflict rate"
        SEVERITY="HIGH"
    elif [[ $CONFLICT_RATE -gt 10 ]]; then
        echo "⚠️  WARNING: High conflict rate"
        SEVERITY="MEDIUM"
    fi
fi

# Check queue status
QUEUE_LENGTH=$(kubectl get --raw /metrics | grep beskar7_provisioning_queue_length | awk '{print $2}' | head -1)
MAX_QUEUE=$(kubectl get configmap beskar7-config -n beskar7-system -o jsonpath='{.data.provisioning\.maxQueueSize}')
QUEUE_UTIL=$(( (QUEUE_LENGTH * 100) / MAX_QUEUE ))

echo "Queue utilization: $QUEUE_UTIL% ($QUEUE_LENGTH/$MAX_QUEUE)"

if [[ $QUEUE_UTIL -gt 90 ]]; then
    echo "🚨 CRITICAL: Queue near capacity"
    SEVERITY="HIGH"
elif [[ $QUEUE_UTIL -gt 70 ]]; then
    echo "⚠️  WARNING: High queue utilization"
    SEVERITY=${SEVERITY:-MEDIUM}
fi

# Step 2: Immediate mitigation
echo -e "\n2. IMMEDIATE MITIGATION"

if [[ "$SEVERITY" == "HIGH" ]]; then
    echo "🔧 Applying emergency performance tuning..."
    
    # Increase queue capacity immediately
    kubectl patch configmap beskar7-config -n beskar7-system \
      --patch '{"data":{"provisioning.maxQueueSize":"'$((MAX_QUEUE * 2))'"}}'
    
    # Reduce concurrency to prevent overload
    kubectl patch configmap beskar7-config -n beskar7-system \
      --patch '{"data":{"provisioning.maxConcurrentOps":"3"}}'
    
    # Enable leader election if not already enabled
    LEADER_ELECTION=$(kubectl get deployment beskar7-controller-manager -n beskar7-system -o jsonpath='{.spec.template.spec.containers[0].args}' | grep "enable-claim-coordinator-leader-election=true" || echo "")
    
    if [[ -z "$LEADER_ELECTION" ]]; then
        echo "🔧 Enabling leader election coordination..."
        kubectl patch deployment beskar7-controller-manager -n beskar7-system \
          --patch '{"spec":{"template":{"spec":{"containers":[{"name":"manager","args":["--enable-claim-coordinator-leader-election=true"]}]}}}}'
    fi
    
    # Restart controllers to apply changes
    kubectl rollout restart deployment/beskar7-controller-manager -n beskar7-system
    
    echo "⏳ Waiting for changes to take effect..."
    kubectl rollout status deployment/beskar7-controller-manager -n beskar7-system --timeout=300s
fi

# Step 3: Monitor improvement
echo -e "\n3. MONITORING IMPROVEMENT"

for i in {1..10}; do
    sleep 60
    
    # Recalculate metrics
    NEW_CONFLICTS=$(kubectl get --raw /metrics | grep 'beskar7_host_claim_attempts_total.*outcome="conflict"' | awk '{sum+=$2} END {print sum}')
    NEW_QUEUE=$(kubectl get --raw /metrics | grep beskar7_provisioning_queue_length | awk '{print $2}' | head -1)
    
    # Calculate rates
    CONFLICT_DELTA=$((NEW_CONFLICTS - CONFLICTS))
    
    echo "Minute $i: Queue: $NEW_QUEUE, New conflicts: $CONFLICT_DELTA"
    
    # Check if improvement is occurring
    if [[ $i -eq 5 ]]; then
        if [[ $NEW_QUEUE -lt $QUEUE_LENGTH && $CONFLICT_DELTA -lt 5 ]]; then
            echo "✅ Performance improvement detected"
            break
        else
            echo "⚠️  No improvement yet, continuing monitoring..."
        fi
    fi
done

echo -e "\n✅ P2 PERFORMANCE RESPONSE COMPLETE"
echo "📊 Continue monitoring metrics for sustained improvement"
```

### P3: Resource Shortage

**Symptoms:** No available hosts, machines stuck in pending

```bash
#!/bin/bash
# p3-resource-shortage.sh
# Response for resource shortage incidents

echo "🔍 P3 RESOURCE: Host Pool Shortage"
echo "Incident ID: BESKAR7-RES-$(date +%Y%m%d-%H%M%S)"
echo "=================================="

# Step 1: Resource assessment
echo "1. RESOURCE ASSESSMENT"

TOTAL_HOSTS=$(kubectl get physicalhosts --all-namespaces --no-headers | wc -l)
AVAILABLE_HOSTS=$(kubectl get physicalhosts --all-namespaces -o jsonpath='{.items[?(@.status.state=="Available")].metadata.name}' | wc -w)
CLAIMED_HOSTS=$(kubectl get physicalhosts --all-namespaces -o jsonpath='{.items[?(@.status.state=="Claimed")].metadata.name}' | wc -w)
ERROR_HOSTS=$(kubectl get physicalhosts --all-namespaces -o jsonpath='{.items[?(@.status.state=="Error")].metadata.name}' | wc -w)
PENDING_MACHINES=$(kubectl get beskar7machines --all-namespaces -o jsonpath='{.items[?(@.status.phase!="Provisioned")].metadata.name}' | wc -w)

echo "Host Pool Status:"
echo "  Total hosts: $TOTAL_HOSTS"
echo "  Available: $AVAILABLE_HOSTS"
echo "  Claimed: $CLAIMED_HOSTS"
echo "  Error: $ERROR_HOSTS"
echo "  Pending machines: $PENDING_MACHINES"

# Step 2: Recovery opportunities
echo -e "\n2. RECOVERY OPPORTUNITIES"

if [[ $ERROR_HOSTS -gt 0 ]]; then
    echo "🔧 Found $ERROR_HOSTS hosts in error state - attempting recovery"
    
    # List error hosts for investigation
    kubectl get physicalhosts --all-namespaces -o custom-columns=NAME:.metadata.name,NAMESPACE:.metadata.namespace,STATE:.status.state,ERROR:.status.errorMessage | grep Error
    
    # Attempt to reset hosts in error state
    ERROR_HOST_LIST=$(kubectl get physicalhosts --all-namespaces -o jsonpath='{.items[?(@.status.state=="Error")].metadata.name}')
    for host in $ERROR_HOST_LIST; do
        echo "🔄 Attempting to recover host: $host"
        
        # Get host namespace
        NAMESPACE=$(kubectl get physicalhost $host --all-namespaces -o jsonpath='{.items[0].metadata.namespace}')
        
        # Clear consumer reference and reset state
        kubectl patch physicalhost $host -n $NAMESPACE --type='json' \
          -p='[{"op": "remove", "path": "/spec/consumerRef"}]' 2>/dev/null || true
        kubectl patch physicalhost $host -n $NAMESPACE --type='json' \
          -p='[{"op": "replace", "path": "/status/state", "value": "Enrolling"}]'
        
        echo "  Reset $host to Enrolling state"
    done
fi

# Check for stuck claims (hosts claimed for >1 hour)
echo -e "\n🔍 Checking for stuck claims..."
STUCK_CLAIMS=$(kubectl get physicalhosts --all-namespaces -o json | jq -r '.items[] | select(.status.state=="Claimed" and .metadata.annotations."beskar7.io/claimed-at"?) | select((now - (.metadata.annotations."beskar7.io/claimed-at" | fromdateiso8601)) > 3600) | .metadata.name' 2>/dev/null || echo "")

if [[ -n "$STUCK_CLAIMS" ]]; then
    echo "Found stuck claims: $STUCK_CLAIMS"
    for host in $STUCK_CLAIMS; do
        echo "🔄 Releasing stuck claim on host: $host"
        
        NAMESPACE=$(kubectl get physicalhost $host --all-namespaces -o jsonpath='{.items[0].metadata.namespace}')
        kubectl patch physicalhost $host -n $NAMESPACE --type='json' \
          -p='[{"op": "remove", "path": "/spec/consumerRef"}]'
        kubectl patch physicalhost $host -n $NAMESPACE --type='json' \
          -p='[{"op": "replace", "path": "/status/state", "value": "Available"}]'
    done
fi

# Step 3: Capacity planning recommendations
echo -e "\n3. CAPACITY PLANNING"

UTILIZATION=$(( ((TOTAL_HOSTS - AVAILABLE_HOSTS) * 100) / TOTAL_HOSTS ))
echo "Current utilization: $UTILIZATION%"

if [[ $UTILIZATION -gt 90 ]]; then
    RECOMMENDED_HOSTS=$(( (PENDING_MACHINES + TOTAL_HOSTS) * 120 / 100 ))
    echo "🚨 CRITICAL: Very high utilization"
    echo "   Recommendation: Add $((RECOMMENDED_HOSTS - TOTAL_HOSTS)) more hosts"
elif [[ $UTILIZATION -gt 80 ]]; then
    RECOMMENDED_HOSTS=$(( (PENDING_MACHINES + TOTAL_HOSTS) * 110 / 100 ))
    echo "⚠️  WARNING: High utilization"
    echo "   Recommendation: Add $((RECOMMENDED_HOSTS - TOTAL_HOSTS)) more hosts"
fi

# Step 4: Monitor recovery
echo -e "\n4. MONITORING RECOVERY"

echo "📊 Monitoring host recovery for 5 minutes..."
for i in {1..5}; do
    sleep 60
    NEW_AVAILABLE=$(kubectl get physicalhosts --all-namespaces -o jsonpath='{.items[?(@.status.state=="Available")].metadata.name}' | wc -w)
    NEW_PENDING=$(kubectl get beskar7machines --all-namespaces -o jsonpath='{.items[?(@.status.phase!="Provisioned")].metadata.name}' | wc -w)
    
    echo "Minute $i: Available hosts: $NEW_AVAILABLE, Pending machines: $NEW_PENDING"
    
    if [[ $NEW_AVAILABLE -gt $AVAILABLE_HOSTS ]]; then
        RECOVERED_HOSTS=$((NEW_AVAILABLE - AVAILABLE_HOSTS))
        echo "✅ Recovered $RECOVERED_HOSTS hosts"
    fi
done

echo -e "\n✅ P3 RESOURCE RESPONSE COMPLETE"
```

## Maintenance Procedures

### Configuration Updates

```bash
#!/bin/bash
# update-concurrent-provisioning-config.sh
# Safe procedure for updating concurrent provisioning configuration

NEW_CONFIG_FILE=${1:-"new-config.yaml"}

echo "🔧 Updating Concurrent Provisioning Configuration"
echo "New config file: $NEW_CONFIG_FILE"
echo "=============================================="

# Step 1: Backup current configuration
BACKUP_FILE="beskar7-config-backup-$(date +%Y%m%d-%H%M%S).yaml"
kubectl get configmap beskar7-config -n beskar7-system -o yaml > "$BACKUP_FILE"
echo "✅ Current configuration backed up to: $BACKUP_FILE"

# Step 2: Validate new configuration
echo "🔍 Validating new configuration..."
if ./validate-beskar7-config.sh "$NEW_CONFIG_FILE"; then
    echo "✅ Configuration validation passed"
else
    echo "❌ Configuration validation failed - aborting"
    exit 1
fi

# Step 3: Show configuration diff
echo "📋 Configuration changes:"
kubectl diff -f "$NEW_CONFIG_FILE" || true

echo "❓ Proceed with configuration update? (y/N)"
read -r CONFIRM
if [[ "$CONFIRM" != "y" && "$CONFIRM" != "Y" ]]; then
    echo "❌ Update cancelled"
    exit 1
fi

# Step 4: Apply configuration gradually
echo "🔄 Applying configuration update..."

# Apply new config
kubectl apply -f "$NEW_CONFIG_FILE"

# Step 5: Restart controllers if needed
echo "🔄 Restarting controllers to pick up new configuration..."
kubectl rollout restart deployment/beskar7-controller-manager -n beskar7-system
kubectl rollout status deployment/beskar7-controller-manager -n beskar7-system --timeout=300s

# Step 6: Verify configuration took effect
echo "🔍 Verifying configuration update..."
sleep 30

# Check if queue configuration changed
NEW_MAX_QUEUE=$(kubectl get configmap beskar7-config -n beskar7-system -o jsonpath='{.data.provisioning\.maxQueueSize}')
NEW_MAX_CONCURRENT=$(kubectl get configmap beskar7-config -n beskar7-system -o jsonpath='{.data.provisioning\.maxConcurrentOps}')

echo "New configuration active:"
echo "  Max queue size: $NEW_MAX_QUEUE"
echo "  Max concurrent ops: $NEW_MAX_CONCURRENT"

# Step 7: Monitor for issues
echo "📊 Monitoring system stability for 5 minutes..."
for i in {1..5}; do
    sleep 60
    
    PODS_READY=$(kubectl get pods -n beskar7-system -l app=beskar7-controller-manager --field-selector=status.phase=Running --no-headers | wc -l)
    QUEUE_LENGTH=$(kubectl get --raw /metrics 2>/dev/null | grep beskar7_provisioning_queue_length | awk '{print $2}' | head -1 || echo "N/A")
    
    echo "Minute $i: Pods ready: $PODS_READY, Queue length: $QUEUE_LENGTH"
    
    if [[ $PODS_READY -eq 0 ]]; then
        echo "❌ Controllers not ready - rolling back"
        kubectl apply -f "$BACKUP_FILE"
        kubectl rollout restart deployment/beskar7-controller-manager -n beskar7-system
        exit 1
    fi
done

echo "✅ Configuration update completed successfully"
echo "📁 Backup file: $BACKUP_FILE"
```

### Host Pool Expansion

```bash
#!/bin/bash
# expand-host-pool.sh
# Procedure for adding new hosts to the pool

echo "📈 Host Pool Expansion Procedure"
echo "==============================="

# Step 1: Pre-expansion assessment
echo "1. PRE-EXPANSION ASSESSMENT"

CURRENT_HOSTS=$(kubectl get physicalhosts --all-namespaces --no-headers | wc -l)
AVAILABLE_HOSTS=$(kubectl get physicalhosts --all-namespaces -o jsonpath='{.items[?(@.status.state=="Available")].metadata.name}' | wc -w)
PENDING_MACHINES=$(kubectl get beskar7machines --all-namespaces -o jsonpath='{.items[?(@.status.phase!="Provisioned")].metadata.name}' | wc -w)

echo "Current state:"
echo "  Total hosts: $CURRENT_HOSTS"
echo "  Available hosts: $AVAILABLE_HOSTS"
echo "  Pending machines: $PENDING_MACHINES"

UTILIZATION=$(( ((CURRENT_HOSTS - AVAILABLE_HOSTS) * 100) / CURRENT_HOSTS ))
echo "  Utilization: $UTILIZATION%"

# Calculate recommended expansion
if [[ $PENDING_MACHINES -gt $AVAILABLE_HOSTS ]]; then
    SHORTAGE=$((PENDING_MACHINES - AVAILABLE_HOSTS))
    RECOMMENDED_ADDITION=$((SHORTAGE + (SHORTAGE * 20 / 100)))  # 20% buffer
    echo "  Recommended addition: $RECOMMENDED_ADDITION hosts"
fi

# Step 2: Host validation checklist
echo -e "\n2. NEW HOST VALIDATION CHECKLIST"
echo "Before adding new hosts, ensure:"
echo "  ☐ BMC network connectivity verified"
echo "  ☐ BMC credentials configured and tested"
echo "  ☐ Redfish API functional"
echo "  ☐ Host hardware compatible"
echo "  ☐ Network boot configuration ready"

echo "❓ Have all validation steps been completed? (y/N)"
read -r VALIDATED
if [[ "$VALIDATED" != "y" && "$VALIDATED" != "Y" ]]; then
    echo "❌ Complete validation before proceeding"
    exit 1
fi

# Step 3: Add hosts gradually
echo -e "\n3. GRADUAL HOST ADDITION"
echo "📁 Host definition files should be in ./new-hosts/ directory"

if [[ ! -d "new-hosts" ]]; then
    echo "❌ ./new-hosts/ directory not found"
    echo "   Create directory and place PhysicalHost YAML files there"
    exit 1
fi

HOST_FILES=(./new-hosts/*.yaml)
TOTAL_NEW_HOSTS=${#HOST_FILES[@]}

echo "Found $TOTAL_NEW_HOSTS host definition files"
echo "❓ Proceed with adding these hosts? (y/N)"
read -r PROCEED
if [[ "$PROCEED" != "y" && "$PROCEED" != "Y" ]]; then
    echo "❌ Host addition cancelled"
    exit 1
fi

# Add hosts in batches of 5
BATCH_SIZE=5
for ((i=0; i<TOTAL_NEW_HOSTS; i+=BATCH_SIZE)); do
    BATCH_END=$((i + BATCH_SIZE - 1))
    if [[ $BATCH_END -ge $TOTAL_NEW_HOSTS ]]; then
        BATCH_END=$((TOTAL_NEW_HOSTS - 1))
    fi
    
    echo "📦 Adding batch $((i/BATCH_SIZE + 1)): hosts $((i+1))-$((BATCH_END+1))"
    
    # Apply batch
    for ((j=i; j<=BATCH_END; j++)); do
        if [[ -f "${HOST_FILES[$j]}" ]]; then
            echo "  Adding: ${HOST_FILES[$j]}"
            kubectl apply -f "${HOST_FILES[$j]}"
        fi
    done
    
    # Wait for hosts to enroll
    echo "  ⏳ Waiting for hosts to enroll..."
    sleep 120
    
    # Check enrollment status
    NEW_AVAILABLE=$(kubectl get physicalhosts --all-namespaces -o jsonpath='{.items[?(@.status.state=="Available")].metadata.name}' | wc -w)
    echo "  Available hosts now: $NEW_AVAILABLE"
done

# Step 4: Verify expansion
echo -e "\n4. EXPANSION VERIFICATION"

FINAL_HOSTS=$(kubectl get physicalhosts --all-namespaces --no-headers | wc -l)
FINAL_AVAILABLE=$(kubectl get physicalhosts --all-namespaces -o jsonpath='{.items[?(@.status.state=="Available")].metadata.name}' | wc -w)

echo "Expansion results:"
echo "  Total hosts: $CURRENT_HOSTS → $FINAL_HOSTS (+$((FINAL_HOSTS - CURRENT_HOSTS)))"
echo "  Available hosts: $AVAILABLE_HOSTS → $FINAL_AVAILABLE (+$((FINAL_AVAILABLE - AVAILABLE_HOSTS)))"

NEW_UTILIZATION=$(( ((FINAL_HOSTS - FINAL_AVAILABLE) * 100) / FINAL_HOSTS ))
echo "  New utilization: $NEW_UTILIZATION%"

if [[ $NEW_UTILIZATION -lt 80 ]]; then
    echo "✅ Host pool expansion successful"
else
    echo "⚠️  Utilization still high, consider adding more hosts"
fi

echo -e "\n✅ HOST POOL EXPANSION COMPLETE"
```

### System Upgrade

```bash
#!/bin/bash
# upgrade-concurrent-provisioning.sh
# Procedure for upgrading the concurrent provisioning system

NEW_VERSION=${1:-"latest"}

echo "⬆️  Concurrent Provisioning System Upgrade"
echo "New version: $NEW_VERSION"
echo "========================================"

# Step 1: Pre-upgrade checks
echo "1. PRE-UPGRADE CHECKS"

# Check current version
CURRENT_VERSION=$(kubectl get deployment beskar7-controller-manager -n beskar7-system -o jsonpath='{.spec.template.spec.containers[0].image}' | cut -d':' -f2)
echo "Current version: $CURRENT_VERSION"
echo "Target version: $NEW_VERSION"

# Check system health
READY_PODS=$(kubectl get pods -n beskar7-system -l app=beskar7-controller-manager --field-selector=status.phase=Running --no-headers | wc -l)
TOTAL_PODS=$(kubectl get pods -n beskar7-system -l app=beskar7-controller-manager --no-headers | wc -l)

if [[ $READY_PODS -ne $TOTAL_PODS ]]; then
    echo "❌ System not healthy - $READY_PODS/$TOTAL_PODS pods ready"
    exit 1
fi

# Check for pending operations
QUEUE_LENGTH=$(kubectl get --raw /metrics | grep beskar7_provisioning_queue_length | awk '{print $2}' | head -1)
if [[ $QUEUE_LENGTH -gt 10 ]]; then
    echo "⚠️  Warning: $QUEUE_LENGTH operations in queue"
    echo "❓ Proceed with upgrade anyway? (y/N)"
    read -r PROCEED
    if [[ "$PROCEED" != "y" && "$PROCEED" != "Y" ]]; then
        echo "❌ Upgrade cancelled"
        exit 1
    fi
fi

# Step 2: Backup current state
echo -e "\n2. BACKUP CURRENT STATE"

BACKUP_DIR="upgrade-backup-$(date +%Y%m%d-%H%M%S)"
mkdir -p "$BACKUP_DIR"

kubectl get deployment beskar7-controller-manager -n beskar7-system -o yaml > "$BACKUP_DIR/deployment.yaml"
kubectl get configmap beskar7-config -n beskar7-system -o yaml > "$BACKUP_DIR/config.yaml"
kubectl get physicalhosts --all-namespaces -o yaml > "$BACKUP_DIR/physicalhosts.yaml"

echo "✅ Backup created in: $BACKUP_DIR"

# Step 3: Rolling upgrade
echo -e "\n3. ROLLING UPGRADE"

# Update image
kubectl set image deployment/beskar7-controller-manager -n beskar7-system manager=beskar7/controller:$NEW_VERSION

# Monitor rollout
echo "📊 Monitoring rollout progress..."
kubectl rollout status deployment/beskar7-controller-manager -n beskar7-system --timeout=600s

if [[ $? -ne 0 ]]; then
    echo "❌ Rollout failed - initiating rollback"
    kubectl rollout undo deployment/beskar7-controller-manager -n beskar7-system
    kubectl rollout status deployment/beskar7-controller-manager -n beskar7-system --timeout=300s
    exit 1
fi

# Step 4: Post-upgrade verification
echo -e "\n4. POST-UPGRADE VERIFICATION"

# Wait for system to stabilize
sleep 60

# Check pod health
NEW_READY_PODS=$(kubectl get pods -n beskar7-system -l app=beskar7-controller-manager --field-selector=status.phase=Running --no-headers | wc -l)
echo "Pods ready after upgrade: $NEW_READY_PODS"

# Verify metrics endpoint
if kubectl get --raw /metrics >/dev/null 2>&1; then
    echo "✅ Metrics endpoint accessible"
else
    echo "❌ Metrics endpoint not accessible"
fi

# Check leader election
NEW_LEADER=$(kubectl get lease beskar7-controller-manager -n beskar7-system -o jsonpath='{.spec.holderIdentity}' 2>/dev/null || echo "NONE")
echo "New leader: $NEW_LEADER"

# Verify basic functionality
echo "🧪 Testing basic functionality..."
TEST_QUEUE=$(kubectl get --raw /metrics | grep beskar7_provisioning_queue_length | awk '{print $2}' | head -1)
echo "Queue operational: ${TEST_QUEUE:-N/A}"

# Step 5: Monitor stability
echo -e "\n5. STABILITY MONITORING"

echo "📊 Monitoring system stability for 10 minutes..."
for i in {1..10}; do
    sleep 60
    
    STABLE_PODS=$(kubectl get pods -n beskar7-system -l app=beskar7-controller-manager --field-selector=status.phase=Running --no-headers | wc -l)
    ERROR_COUNT=$(kubectl get events -n beskar7-system --field-selector type=Warning --sort-by='.lastTimestamp' | grep beskar7 | wc -l)
    
    echo "Minute $i: Stable pods: $STABLE_PODS, Recent errors: $ERROR_COUNT"
    
    if [[ $STABLE_PODS -eq 0 ]]; then
        echo "❌ System unstable - initiating rollback"
        kubectl rollout undo deployment/beskar7-controller-manager -n beskar7-system
        exit 1
    fi
done

echo -e "\n✅ UPGRADE COMPLETED SUCCESSFULLY"
echo "📁 Backup directory: $BACKUP_DIR"
echo "🏷️  New version: $NEW_VERSION"
```

This comprehensive runbook provides the necessary procedures for day-to-day operations, incident response, and maintenance of the concurrent provisioning system. Regular use of these procedures will ensure reliable operation and quick resolution of issues. 