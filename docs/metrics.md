# Monitoring and Metrics

Beskar7 provides comprehensive monitoring and observability through Prometheus metrics. This document describes the available metrics, how to set up monitoring, and how to interpret the data.

## Overview

The Beskar7 controllers expose metrics on the `/metrics` endpoint (default port 8080) that can be scraped by Prometheus. These metrics provide insights into:

- Controller reconciliation performance
- PhysicalHost provisioning success/failure rates
- Infrastructure resource states and availability
- Error rates and types
- Boot configuration success rates
- Failure domain discovery

## Metric Categories

### Controller Performance Metrics

These metrics track the overall performance and health of the Beskar7 controllers.

#### `beskar7_controller_reconciliation_total`
**Type:** Counter  
**Labels:** `controller`, `outcome`, `namespace`  
**Description:** Total number of reconciliation attempts by controller and outcome.

**Outcomes:**
- `success` - Reconciliation completed successfully
- `error` - Reconciliation failed with an error
- `requeue` - Reconciliation was requeued for later processing
- `not_found` - Resource was not found (likely deleted)

#### `beskar7_controller_reconciliation_duration_seconds`
**Type:** Histogram  
**Labels:** `controller`, `outcome`, `namespace`  
**Description:** Time taken to complete reconciliation operations.

#### `beskar7_controller_errors_total`
**Type:** Counter  
**Labels:** `controller`, `error_type`, `namespace`  
**Description:** Total number of errors encountered by controller type.

**Error Types:**
- `transient` - Temporary errors that may resolve automatically
- `permanent` - Persistent errors requiring intervention
- `validation` - Input validation errors
- `connection` - Network/connectivity errors
- `timeout` - Operation timeout errors
- `unknown` - Unclassified errors

#### `beskar7_controller_requeue_total`
**Type:** Counter  
**Labels:** `controller`, `reason`, `namespace`  
**Description:** Total number of reconciliation requeues by reason.

### PhysicalHost Metrics

These metrics track the state and health of physical hosts managed by Beskar7.

#### `beskar7_controller_physicalhost_states_total`
**Type:** Gauge  
**Labels:** `state`, `namespace`  
**Description:** Number of PhysicalHosts in each state.

**States:**
- `available` - Host is available for provisioning
- `claimed` - Host has been claimed by a machine
- `provisioning` - Host is being provisioned
- `provisioned` - Host has been successfully provisioned
- `deprovisioning` - Host is being cleaned up
- `error` - Host is in an error state

#### `beskar7_controller_physicalhost_provisioning_total`
**Type:** Counter  
**Labels:** `outcome`, `namespace`, `error_type`  
**Description:** Total number of PhysicalHost provisioning attempts.

#### `beskar7_controller_physicalhost_power_operations_total`
**Type:** Counter  
**Labels:** `operation`, `outcome`, `namespace`  
**Description:** Total number of power operations performed on PhysicalHosts.

**Operations:**
- `power_on` - Power on operation
- `power_off` - Power off operation
- `power_reset` - Power reset operation

#### `beskar7_controller_physicalhost_redfish_connections_total`
**Type:** Counter  
**Labels:** `outcome`, `namespace`, `error_type`  
**Description:** Total number of Redfish connection attempts.

#### `beskar7_controller_physicalhost_availability`
**Type:** Gauge  
**Labels:** `namespace`  
**Description:** Availability ratio of PhysicalHosts (available/total).

#### `beskar7_controller_physicalhost_consumer_mappings_total`
**Type:** Gauge  
**Labels:** `consumer_type`, `namespace`  
**Description:** Number of PhysicalHosts mapped to consumers.

### Beskar7Machine Metrics

These metrics track machine provisioning and lifecycle management.

#### `beskar7_controller_beskar7machine_states_total`
**Type:** Gauge  
**Labels:** `phase`, `namespace`  
**Description:** Number of Beskar7Machines in each phase.

#### `beskar7_controller_beskar7machine_provisioning_duration_seconds`
**Type:** Histogram  
**Labels:** `outcome`, `namespace`  
**Description:** Time taken to provision a Beskar7Machine from creation to ready.

**Buckets:** 30s, 60s, 2m, 5m, 10m, 20m, 30m, 1h

#### `beskar7_controller_beskar7machine_boot_configurations_total`
**Type:** Counter  
**Labels:** `mode`, `os_family`, `outcome`, `namespace`  
**Description:** Total number of boot configuration attempts.

### Beskar7Cluster Metrics

These metrics track cluster-level operations and failure domain discovery.

#### `beskar7_controller_beskar7cluster_states_total`
**Type:** Gauge  
**Labels:** `ready`, `namespace`  
**Description:** Number of Beskar7Clusters in each readiness state.

#### `beskar7_controller_beskar7cluster_failure_domains_total`
**Type:** Gauge  
**Labels:** `cluster`, `namespace`  
**Description:** Number of failure domains discovered per cluster.

#### `beskar7_controller_beskar7cluster_failure_domain_discovery_total`
**Type:** Counter  
**Labels:** `outcome`, `namespace`  
**Description:** Total number of failure domain discovery operations.

## Setting Up Monitoring

### Prerequisites

- Prometheus server
- Grafana (optional, for dashboards)
- ServiceMonitor CRD (if using Prometheus Operator)

### Prometheus Configuration

Add the following scrape configuration to your Prometheus config:

```yaml
scrape_configs:
  - job_name: 'beskar7-controller'
    static_configs:
      - targets: ['<beskar7-controller-service>:8080']
    metrics_path: /metrics
    scrape_interval: 30s
```

### ServiceMonitor (Prometheus Operator)

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: beskar7-controller
  namespace: beskar7-system
spec:
  selector:
    matchLabels:
      control-plane: controller-manager
  endpoints:
  - port: metrics
    interval: 30s
    path: /metrics
```

### Grafana Dashboard

Create dashboards to visualize:

1. **Controller Health Dashboard**
   - Reconciliation rate and duration
   - Error rates by type
   - Requeue patterns

2. **Infrastructure Dashboard**
   - PhysicalHost state distribution
   - Host availability trends
   - Provisioning success rates

3. **Performance Dashboard**
   - Machine provisioning times
   - Boot configuration success rates
   - Failure domain discovery

## Key Metrics for Alerting

### Critical Alerts

```yaml
# High error rate
- alert: Beskar7HighErrorRate
  expr: rate(beskar7_controller_errors_total[5m]) > 0.1
  for: 2m
  annotations:
    summary: "High error rate in Beskar7 controllers"

# Controller not reconciling
- alert: Beskar7NoReconciliation
  expr: rate(beskar7_controller_reconciliation_total[10m]) == 0
  for: 5m
  annotations:
    summary: "Beskar7 controller not performing reconciliations"

# Low host availability
- alert: Beskar7LowHostAvailability
  expr: beskar7_controller_physicalhost_availability < 0.2
  for: 5m
  annotations:
    summary: "Low PhysicalHost availability"
```

### Warning Alerts

```yaml
# Slow provisioning
- alert: Beskar7SlowProvisioning
  expr: histogram_quantile(0.95, rate(beskar7_controller_beskar7machine_provisioning_duration_seconds_bucket[10m])) > 1800
  for: 10m
  annotations:
    summary: "Slow machine provisioning times"

# High requeue rate
- alert: Beskar7HighRequeueRate
  expr: rate(beskar7_controller_requeue_total[5m]) > 0.5
  for: 5m
  annotations:
    summary: "High reconciliation requeue rate"
```

## Troubleshooting with Metrics

### High Error Rates

1. Check `beskar7_controller_errors_total` by `error_type`
2. Look for patterns in `controller` and `namespace` labels
3. Correlate with `beskar7_controller_physicalhost_redfish_connections_total` for connectivity issues

### Slow Provisioning

1. Examine `beskar7_controller_beskar7machine_provisioning_duration_seconds` percentiles
2. Check for high error rates in boot configurations
3. Monitor power operation success rates

### Resource Exhaustion

1. Monitor `beskar7_controller_physicalhost_availability`
2. Check distribution of `beskar7_controller_physicalhost_states_total`
3. Look for stuck hosts in `provisioning` or `error` states

### Cluster Issues

1. Monitor `beskar7_controller_beskar7cluster_failure_domain_discovery_total` for discovery failures
2. Check if failure domains are being properly detected
3. Verify cluster readiness states

## Metric Retention and Storage

- **Retention Period:** Configure based on your operational needs (30-90 days typical)
- **Storage:** Size storage based on metric cardinality and retention period
- **Backup:** Include metrics in your backup strategy for historical analysis

## Integration with Other Tools

### With Cluster API

Beskar7 metrics complement Cluster API metrics to provide full cluster lifecycle visibility:

- Correlate machine provisioning times with cluster scaling events
- Monitor infrastructure readiness during cluster creation
- Track failure domain availability for cluster placement decisions

### With Hardware Monitoring

Combine with BMC/hardware metrics:

- Correlate Beskar7 host states with hardware health metrics
- Monitor power consumption during provisioning operations
- Track hardware failures that impact host availability

## Best Practices

1. **Alert Tuning:** Start with conservative thresholds and adjust based on observed patterns
2. **Dashboard Organization:** Group metrics by operational concern (health, performance, capacity)
3. **Metric Labels:** Use consistent labeling across your monitoring stack
4. **Historical Analysis:** Retain metrics long enough for trend analysis and capacity planning
5. **Documentation:** Keep runbooks that reference specific metrics for troubleshooting procedures 