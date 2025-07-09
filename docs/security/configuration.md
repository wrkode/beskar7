# Security Configuration Guide

This guide provides detailed configuration options for Beskar7's security features.

## Security Policy Configuration

The security policy is defined in a ConfigMap and controls all security behavior.

### Location

```bash
kubectl get configmap beskar7-security-policy -n beskar7-system -o yaml
```

### Full Configuration Reference

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: beskar7-security-policy
  namespace: beskar7-system
data:
  policy.yaml: |
    security:
      # TLS Security Configuration
      tls:
        # Enforce certificate validation (recommended: true for production)
        enforce_certificate_validation: true
        
        # Allow insecureSkipVerify (recommended: false for production)
        allow_insecure_skip_verify: false
        
        # Minimum TLS version (options: "1.0", "1.1", "1.2", "1.3")
        min_tls_version: "1.2"
        
        # Certificate expiry warning thresholds (days)
        expiry_warning_days: 30
        expiry_critical_days: 7
        
        # Trusted certificate authorities
        trusted_cas:
          - system  # Use system CA bundle
          # Add custom CAs:
          # - custom-ca-secret
        
        # Certificate validation requirements
        require_hostname_verification: true
        require_valid_certificate_chain: true
        
      # RBAC Security Configuration
      rbac:
        # Enforce principle of least privilege
        enforce_least_privilege: true
        
        # Prohibited permissions (never allow)
        prohibited_permissions:
          - apiGroups: ["*"]
            resources: ["*"]
            verbs: ["*"]
          - apiGroups: ["rbac.authorization.k8s.io"]
            resources: ["*"]
            verbs: ["*"]
          - verbs: ["impersonate"]
        
        # Maximum allowed permissions
        max_allowed_permissions:
          beskar7-controller:
            - apiGroups: ["infrastructure.cluster.x-k8s.io"]
              resources: ["physicalhosts", "beskar7machines", "beskar7clusters"]
              verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
```

## TLS Configuration

### Basic TLS Setup

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
    # Basic TLS settings
    insecureSkipVerify: false  # Always validate certificates
```

### Custom CA Certificate

For private CAs or self-signed certificates:

```yaml
# 1. Create CA certificate secret
apiVersion: v1
kind: Secret
metadata:
  name: custom-ca-cert
  namespace: beskar7-system
type: Opaque
data:
  ca.crt: LS0tLS1CRUdJTi... # Base64 encoded CA certificate

---
# 2. Reference in PhysicalHost
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

### TLS Version Control

Configure minimum TLS version in security policy:

```yaml
security:
  tls:
    min_tls_version: "1.2"  # Options: "1.0", "1.1", "1.2", "1.3"
```

### Certificate Expiry Monitoring

Configure certificate expiry warnings:

```yaml
security:
  tls:
    expiry_warning_days: 30   # Warning threshold
    expiry_critical_days: 7   # Critical threshold
```

## Credential Configuration

### Password Policy

Configure password requirements:

```yaml
security:
  credentials:
    password_policy:
      min_length: 12                    # Minimum password length
      require_uppercase: true           # Require uppercase letters
      require_lowercase: true           # Require lowercase letters
      require_numbers: true             # Require numbers
      require_special_chars: true       # Require special characters
      prohibited_common_passwords: true # Block common passwords
```

### Credential Rotation

Configure rotation policies:

```yaml
security:
  credentials:
    rotation_policy:
      max_age_days: 90           # Maximum credential age
      warning_age_days: 60       # Warning threshold
      force_rotation_age_days: 365  # Force rotation threshold
```

### Credential Storage

Enforce secure storage:

```yaml
security:
  credentials:
    storage_policy:
      require_encryption_at_rest: true
      require_kubernetes_secrets: true
      prohibit_plaintext_storage: true
```

### Creating Secure Credentials

```bash
# Generate strong password
PASSWORD=$(openssl rand -base64 32)

# Create credential secret
kubectl create secret generic bmc-credentials \
  --from-literal=username=admin \
  --from-literal=password="$PASSWORD" \
  --namespace=default
```

## RBAC Configuration

### Service Account Setup

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: controller-manager
  namespace: beskar7-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: manager-role
rules:
# Minimal required permissions
- apiGroups: ["infrastructure.cluster.x-k8s.io"]
  resources: ["physicalhosts", "beskar7machines", "beskar7clusters"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["infrastructure.cluster.x-k8s.io"]
  resources: ["physicalhosts/status", "beskar7machines/status", "beskar7clusters/status"]
  verbs: ["get", "update", "patch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: manager-rolebinding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: manager-role
subjects:
- kind: ServiceAccount
  name: controller-manager
  namespace: beskar7-system
```

### RBAC Validation

Enable RBAC security validation:

```yaml
security:
  rbac:
    enforce_least_privilege: true
    
    # Define prohibited permissions
    prohibited_permissions:
      - apiGroups: ["*"]
        resources: ["*"]
        verbs: ["*"]
      - verbs: ["impersonate"]
```

## Network Security Configuration

### Network Policies

#### Manager Network Policy

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: beskar7-manager-policy
  namespace: beskar7-system
spec:
  podSelector:
    matchLabels:
      control-plane: controller-manager
  policyTypes:
  - Ingress
  - Egress
  
  ingress:
  # Webhook traffic from API server
  - from:
    - namespaceSelector: {}
    ports:
    - protocol: TCP
      port: 9443
  
  # Metrics scraping
  - from:
    - namespaceSelector:
        matchLabels:
          name: monitoring
    ports:
    - protocol: TCP
      port: 8080
  
  egress:
  # DNS resolution
  - to: []
    ports:
    - protocol: UDP
      port: 53
  
  # Kubernetes API
  - to: []
    ports:
    - protocol: TCP
      port: 443
  
  # BMC communication
  - to: []
    ports:
    - protocol: TCP
      port: 443
    - protocol: TCP
      port: 8443
```

#### Default Deny Policy

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: default-deny-all
  namespace: beskar7-system
spec:
  podSelector: {}
  policyTypes:
  - Ingress
  - Egress
```

### BMC Network Configuration

Configure BMC network access:

```yaml
security:
  network:
    # Default to secure communication
    default_tls: true
    
    # Prohibited protocols
    prohibited_protocols:
      - http
      - telnet
      - ftp
    
    # Required encryption for BMC
    bmc_encryption: true
```

## Container Security Configuration

### Security Context

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: controller-manager
spec:
  template:
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 65532
        runAsGroup: 65532
        fsGroup: 65532
        seccompProfile:
          type: RuntimeDefault
      
      containers:
      - name: manager
        securityContext:
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
          runAsNonRoot: true
          runAsUser: 65532
          capabilities:
            drop: ["ALL"]
          seccompProfile:
            type: RuntimeDefault
```

### Resource Limits

```yaml
security:
  container:
    resource_limits:
      enforce_resource_limits: true
      default_cpu_limit: "500m"
      default_memory_limit: "512Mi"
      max_cpu_limit: "2000m"
      max_memory_limit: "4Gi"
```

## Monitoring Configuration

### Security Monitoring

Enable/disable security monitoring:

```yaml
# Via command line arguments
args:
- --enable-security-monitoring=true
- --security-scan-interval=1h

# Via security policy
security:
  monitoring:
    security_monitoring:
      enabled: true
      scan_interval: "1h"
      alert_on_critical: true
      alert_on_high: true
```

### Metrics Configuration

Configure security metrics:

```yaml
security:
  monitoring:
    metrics:
      expose_security_metrics: true
      alert_on_policy_violations: true
      alert_on_certificate_expiry: true
      alert_on_credential_age: true
```

### Audit Logging

Enable audit logging:

```yaml
security:
  monitoring:
    audit_logging:
      enabled: true
      log_failed_authentications: true
      log_privilege_escalations: true
      log_secret_access: true
      log_rbac_changes: true
```

## Environment-Specific Configurations

### Production Environment

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
      min_length: 16
      require_complexity: true
    rotation_policy:
      max_age_days: 90
  
  monitoring:
    security_monitoring:
      enabled: true
      scan_interval: "30m"
      alert_on_critical: true
      alert_on_high: true
  
  compliance:
    standards:
      cis_kubernetes: true
      nist_cybersecurity: true
      soc2: true
```

### Development Environment

```yaml
security:
  # Enable development exceptions
  exceptions:
    development:
      allow_insecure_skip_verify: true
      allow_self_signed_certificates: true
      reduced_password_requirements: true
      extended_credential_rotation: true
  
  # Relaxed monitoring
  monitoring:
    security_monitoring:
      enabled: true
      scan_interval: "4h"
      alert_on_critical: true
      alert_on_high: false
```

### Testing Environment

```yaml
security:
  exceptions:
    testing:
      allow_test_credentials: true
      allow_mock_certificates: true
      relaxed_rbac_validation: true
  
  monitoring:
    security_monitoring:
      enabled: false
```

## Advanced Configuration

### Custom Security Validators

Extend security validation with custom validators:

```yaml
security:
  custom_validators:
    - name: "corporate-policy"
      type: "rbac"
      config:
        max_permissions_per_role: 10
        require_justification: true
    
    - name: "certificate-pinning"
      type: "tls"
      config:
        pinned_certificates:
          - fingerprint: "sha256:..."
            hosts: ["bmc.internal.com"]
```

### Integration with External Systems

#### LDAP/Active Directory

```yaml
security:
  authentication:
    ldap:
      enabled: true
      server: "ldap.corporate.com"
      base_dn: "dc=corporate,dc=com"
      user_filter: "(uid=%s)"
      tls_config:
        min_version: "1.2"
        certificate_validation: true
```

#### External Secret Management

```yaml
security:
  credentials:
    external_secret_manager:
      enabled: true
      provider: "vault"  # Options: vault, aws-secrets, azure-keyvault
      config:
        vault_address: "https://vault.corporate.com"
        vault_role: "beskar7"
        secret_path: "secret/beskar7/bmc-credentials"
```

## Troubleshooting Configuration

### Debug Mode

Enable debug logging for security components:

```yaml
# In deployment args
args:
- --v=2  # Verbose logging
- --security-debug=true

# Environment variables
env:
- name: BESKAR7_SECURITY_DEBUG
  value: "true"
```

### Configuration Validation

Validate security configuration:

```bash
# Validate security policy
kubectl get configmap beskar7-security-policy -n beskar7-system -o yaml | yq eval '.data."policy.yaml"'

# Check current security status
kubectl get events -n beskar7-system --field-selector type=Warning

# Test RBAC permissions
kubectl auth can-i --list --as=system:serviceaccount:beskar7-system:controller-manager
```

### Common Configuration Issues

1. **Certificate Validation Failures**
   ```yaml
   # Fix: Add custom CA or disable validation for testing
   insecureSkipVerify: true  # Only for development!
   ```

2. **RBAC Permission Denied**
   ```bash
   # Fix: Check and update ClusterRole permissions
   kubectl get clusterrole manager-role -o yaml
   ```

3. **Network Policy Blocking Traffic**
   ```bash
   # Fix: Review and update network policies
   kubectl get networkpolicy -n beskar7-system
   ```

4. **Resource Limits Too Restrictive**
   ```yaml
   # Fix: Increase resource limits
   resources:
     limits:
       memory: "1Gi"  # Increase from default
   ```

## Configuration Best Practices

1. **Start Secure**: Begin with restrictive settings and relax as needed
2. **Environment Separation**: Use different configurations for dev/test/prod
3. **Regular Updates**: Review and update security configurations regularly
4. **Validate Changes**: Test configuration changes in non-production first
5. **Monitor Impact**: Watch for security events after configuration changes
6. **Document Exceptions**: Clearly document any security exceptions and their justification

## Configuration Examples

See the `examples/security/` directory for complete configuration examples:

- `examples/security/production.yaml` - Production security configuration
- `examples/security/development.yaml` - Development configuration
- `examples/security/high-security.yaml` - High-security environment configuration 