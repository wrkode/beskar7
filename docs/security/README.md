# Beskar7 Security Guide

Beskar7 provides comprehensive security features to ensure secure bare-metal infrastructure provisioning. This guide covers all security capabilities, configuration options, and best practices.

## Overview

Beskar7's security framework includes:

- **TLS Certificate Validation**: Comprehensive certificate verification for BMC communications
- **RBAC Security**: Principle of least privilege with granular permissions
- **Credential Security**: Secure handling, validation, and rotation of BMC credentials
- **Security Monitoring**: Real-time security scanning and threat detection
- **Network Security**: Network policies and secure communication protocols
- **Container Security**: Hardened container configurations and security contexts

## Quick Start

### Enable Security Features

Security monitoring is enabled by default. To configure:

```bash
# Enable security monitoring (default)
kubectl apply -f config/default

# Disable security monitoring
kubectl patch deployment controller-manager -n beskar7-system \
  --patch '{"spec":{"template":{"spec":{"containers":[{"name":"manager","args":["--leader-elect","--enable-security-monitoring=false"]}]}}}}'
```

### Check Security Status

```bash
# View security events
kubectl get events -n beskar7-system --field-selector type=Warning

# Check security policy
kubectl get configmap beskar7-security-policy -n beskar7-system -o yaml

# View network policies
kubectl get networkpolicy -n beskar7-system
```

## Security Features

### 1. TLS Certificate Validation

Beskar7 validates TLS certificates for all BMC connections to prevent man-in-the-middle attacks.

#### Features:
- ✅ Certificate chain validation
- ✅ Hostname verification
- ✅ Expiry date checking
- ✅ Self-signed certificate detection
- ✅ Key usage validation
- ✅ Certificate authority verification

#### Configuration:

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: PhysicalHost
metadata:
  name: server-01
spec:
  redfishConnection:
    address: "https://bmc.example.com"
    credentialsSecretRef:
      name: bmc-credentials
    # Security options
    insecureSkipVerify: false  # Enforce certificate validation
    # Optional: Custom CA certificate
    # caCertificateSecretRef:
    #   name: custom-ca-cert
```

#### Certificate Warnings:

The system provides warnings for:
- Certificates expiring within 30 days (WARNING)
- Certificates expiring within 7 days (CRITICAL)
- Self-signed certificates (INFO)
- Invalid certificate chains (ERROR)

### 2. RBAC Security

Beskar7 follows the principle of least privilege with minimal required permissions.

#### Default Permissions:

```yaml
# Infrastructure resources - Full access for our CRDs
- apiGroups: ["infrastructure.cluster.x-k8s.io"]
  resources: ["physicalhosts", "beskar7machines", "beskar7clusters"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]

# Status updates
- apiGroups: ["infrastructure.cluster.x-k8s.io"]
  resources: ["physicalhosts/status", "beskar7machines/status", "beskar7clusters/status"]
  verbs: ["get", "update", "patch"]

# Cluster API resources - Read-only
- apiGroups: ["cluster.x-k8s.io"]
  resources: ["clusters", "machines"]
  verbs: ["get", "list", "watch"]

# Secrets - Read-only for credentials
- apiGroups: [""]
  resources: ["secrets"]
  verbs: ["get", "list", "watch"]

# Events - Create/update for logging
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
```

#### Security Validation:

The RBAC validator checks for:
- ❌ Wildcard permissions (`*,*,*`)
- ❌ Overly broad verb permissions
- ❌ Impersonation capabilities
- ❌ Cluster-admin equivalent permissions
- ✅ Principle of least privilege compliance

### 3. Credential Security

Secure handling of BMC credentials with validation and rotation tracking.

#### Credential Requirements:

**Password Policy:**
- Minimum 12 characters
- Must include uppercase, lowercase, numbers, and special characters
- Prohibited common passwords
- Regular rotation (recommended: 90 days)

**Secret Format:**
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: bmc-credentials
type: Opaque
data:
  username: <base64-encoded-username>
  password: <base64-encoded-password>
```

#### Security Validation:

The system validates:
- ✅ Required fields (username, password)
- ✅ Password strength and complexity
- ✅ Credential age and rotation needs
- ✅ Secure storage in Kubernetes secrets
- ❌ Plaintext credential storage

### 4. Security Monitoring

Real-time security monitoring with automated threat detection.

#### Monitoring Capabilities:

- **TLS Security**: Certificate validation and expiry tracking
- **RBAC Security**: Permission analysis and violation detection
- **Credential Security**: Secret age tracking and rotation monitoring
- **Configuration Drift**: Security policy compliance checking

#### Security Reports:

```json
{
  "timestamp": "2024-01-15T10:30:00Z",
  "total_issues": 5,
  "critical_issues": 1,
  "high_issues": 2,
  "medium_issues": 2,
  "low_issues": 0,
  "tls_findings": [...],
  "rbac_findings": [...],
  "secret_findings": [...],
  "recommendations": [...]
}
```

#### Alerting:

Critical and high-severity issues generate Kubernetes events:

```bash
kubectl get events -n beskar7-system --field-selector type=Warning,reason=TLSSecurityIssue
kubectl get events -n beskar7-system --field-selector type=Warning,reason=RBACSecurityIssue
kubectl get events -n beskar7-system --field-selector type=Warning,reason=SecretSecurityIssue
```

### 5. Network Security

Network policies enforce secure communication patterns.

#### Network Policies:

- **Default Deny**: All traffic denied by default
- **Selective Allow**: Explicit rules for required communication
- **Monitoring Integration**: Metrics scraping allowed from monitoring systems
- **API Access**: Controlled access to Kubernetes API server

#### BMC Communication:

```yaml
# Allowed BMC ports
- protocol: TCP
  port: 443   # HTTPS/Redfish
- protocol: TCP
  port: 8443  # Alternative HTTPS
```

### 6. Container Security

Hardened container configuration following security best practices.

#### Security Context:

```yaml
securityContext:
  runAsNonRoot: true
  runAsUser: 65532
  runAsGroup: 65532
  readOnlyRootFilesystem: true
  allowPrivilegeEscalation: false
  capabilities:
    drop: ["ALL"]
  seccompProfile:
    type: RuntimeDefault
```

#### Resource Limits:

```yaml
resources:
  limits:
    cpu: 500m
    memory: 512Mi
    ephemeral-storage: 1Gi
  requests:
    cpu: 100m
    memory: 128Mi
    ephemeral-storage: 100Mi
```

## Security Configuration

### Environment-Specific Settings

#### Production Environment:

```yaml
security:
  tls:
    enforce_certificate_validation: true
    allow_insecure_skip_verify: false
    min_tls_version: "1.2"
  
  rbac:
    enforce_least_privilege: true
  
  credentials:
    password_policy:
      min_length: 12
      require_complexity: true
    rotation_policy:
      max_age_days: 90
  
  monitoring:
    security_monitoring:
      enabled: true
      scan_interval: "1h"
      alert_on_critical: true
```

#### Development Environment:

```yaml
security:
  exceptions:
    development:
      allow_insecure_skip_verify: true
      allow_self_signed_certificates: true
      reduced_password_requirements: true
```

### Custom CA Certificates

For environments with custom certificate authorities:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: custom-ca-cert
  namespace: beskar7-system
type: Opaque
data:
  ca.crt: <base64-encoded-ca-certificate>
---
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: PhysicalHost
metadata:
  name: server-01
spec:
  redfishConnection:
    address: "https://bmc.internal.com"
    credentialsSecretRef:
      name: bmc-credentials
    caCertificateSecretRef:
      name: custom-ca-cert
```

## Security Metrics

Beskar7 exposes security metrics for monitoring:

### Prometheus Metrics:

```
# TLS certificate metrics
beskar7_tls_certificate_expiry_days
beskar7_tls_certificate_valid
beskar7_tls_insecure_skip_verify_total

# RBAC security metrics
beskar7_rbac_overly_broad_permissions_total
beskar7_rbac_security_violations_total

# Credential security metrics
beskar7_credential_age_days
beskar7_credential_rotation_required_total

# Security scan metrics
beskar7_security_scan_issues_total
beskar7_security_scan_duration_seconds
```

### Grafana Dashboard:

Use the provided Grafana dashboard to visualize security metrics:

```bash
kubectl apply -f examples/monitoring/grafana-dashboard-security.yaml
```

## Compliance

Beskar7 security features support compliance with:

- **CIS Kubernetes Benchmark**: Container and cluster security
- **NIST Cybersecurity Framework**: Risk management and security controls
- **SOC 2 Type II**: Security and availability controls
- **ISO 27001**: Information security management

### Compliance Reports:

Generate compliance reports:

```bash
# Run compliance scan
kubectl create job beskar7-compliance-scan --from=cronjob/beskar7-security-monitor

# View compliance report
kubectl logs job/beskar7-compliance-scan
```

## Troubleshooting

### Common Security Issues:

#### TLS Certificate Problems:

```bash
# Check certificate validation errors
kubectl get events -n beskar7-system --field-selector reason=TLSSecurityIssue

# Validate certificate manually
openssl s_client -connect bmc.example.com:443 -verify_return_error
```

#### RBAC Permission Issues:

```bash
# Check RBAC violations
kubectl get events -n beskar7-system --field-selector reason=RBACSecurityIssue

# Validate current permissions
kubectl auth can-i --list --as=system:serviceaccount:beskar7-system:controller-manager
```

#### Credential Problems:

```bash
# Check credential validation
kubectl get events -n beskar7-system --field-selector reason=SecretSecurityIssue

# Validate secret format
kubectl get secret bmc-credentials -o yaml
```

### Security Logs:

Enable detailed security logging:

```bash
# Increase log level for security components
kubectl patch deployment controller-manager -n beskar7-system \
  --patch '{"spec":{"template":{"spec":{"containers":[{"name":"manager","args":["--leader-elect","--v=2"]}]}}}}'
```

## Best Practices

### 1. Certificate Management:
- Use proper CA-signed certificates for production
- Implement certificate rotation procedures
- Monitor certificate expiry dates
- Validate certificate chains regularly

### 2. Credential Management:
- Rotate credentials every 90 days
- Use strong, unique passwords
- Store credentials securely in Kubernetes secrets
- Implement credential rotation automation

### 3. RBAC Security:
- Follow principle of least privilege
- Regularly audit permissions
- Avoid wildcard permissions
- Use namespace-scoped roles when possible

### 4. Monitoring:
- Enable security monitoring in production
- Set up alerting for critical issues
- Review security reports regularly
- Investigate security events promptly

### 5. Network Security:
- Implement network policies
- Use TLS for all communications
- Restrict network access to BMCs
- Monitor network traffic patterns

## Security Contacts

For security-related issues:

- **Security Issues**: Open an issue with the `security` label
- **Vulnerability Reports**: Email security@beskar7.io
- **Security Questions**: Join the `#security` channel in Slack

## Related Documentation

- [Installation Guide](../installation.md)
- [Configuration Reference](../configuration.md)
- [Troubleshooting Guide](../troubleshooting.md)
- [API Reference](../api.md) 