# Beskar7 Security Configuration Examples

This directory contains security configuration examples for different deployment environments and security profiles.

## Available Examples

### Production Configuration (`production.yaml`)

A comprehensive production-ready security configuration with:

- **Strict TLS enforcement** with certificate validation
- **Minimal RBAC permissions** following principle of least privilege
- **Strong password policies** and credential rotation
- **Network policies** with default deny and selective allow rules
- **Container security hardening** with read-only filesystems and dropped capabilities
- **Comprehensive monitoring** with frequent security scans and alerting
- **Compliance features** for CIS Kubernetes, NIST, and SOC 2
- **High availability** configuration with pod anti-affinity

Use this configuration for production environments where security is paramount.

```bash
kubectl apply -f production.yaml
```

### Development Configuration (`development.yaml`)

A development-friendly configuration with relaxed security settings:

- **Relaxed TLS validation** allowing self-signed certificates
- **Broader RBAC permissions** for easier development and debugging
- **Relaxed password policies** with shorter, simpler passwords
- **Minimal network restrictions** allowing more communication
- **Debug-friendly container settings** with root access and writable filesystems
- **Less frequent monitoring** to reduce noise during development
- **Enhanced logging** for troubleshooting

Use this configuration for development and testing environments.

```bash
kubectl apply -f development.yaml
```

## Configuration Comparison

| Feature | Production | Development |
|---------|------------|-------------|
| TLS Validation | Strict | Relaxed |
| Password Policy | Strong (16+ chars) | Simple (8+ chars) |
| RBAC Permissions | Minimal | Broad |
| Container Security | Hardened | Debug-friendly |
| Monitoring Frequency | 30 minutes | 4 hours |
| Network Policies | Restrictive | Permissive |
| Resource Limits | Enforced | Optional |
| Audit Logging | Enabled | Disabled |

## Using These Examples

### Production Deployment

1. **Review the configuration** to ensure it meets your security requirements
2. **Update CA certificates** if using custom certificate authorities
3. **Customize resource limits** based on your cluster capacity
4. **Configure monitoring** to integrate with your alerting system
5. **Apply the configuration**:

```bash
# Apply production security configuration
kubectl apply -f examples/security/production.yaml

# Verify deployment
kubectl get deployment controller-manager -n beskar7-system
kubectl get events -n beskar7-system
```

### Development Deployment

1. **Apply the development configuration**:

```bash
# Apply development security configuration
kubectl apply -f examples/security/development.yaml

# Enable debug logging
kubectl patch deployment controller-manager -n beskar7-system \
  --patch '{"spec":{"template":{"spec":{"containers":[{"name":"manager","args":["--leader-elect=false","--v=2"]}]}}}}'
```

2. **Create test resources**:

```bash
# Create development PhysicalHost
kubectl apply -f - <<EOF
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: PhysicalHost
metadata:
  name: dev-test-server
spec:
  redfishConnection:
    address: "https://192.168.1.100"
    credentialsSecretRef:
      name: dev-bmc-credentials
    insecureSkipVerify: true
EOF
```

## Security Validation

### Check Security Status

```bash
# Check security events
kubectl get events -n beskar7-system --field-selector type=Warning

# View security policy
kubectl get configmap beskar7-security-policy -n beskar7-system -o yaml

# Check RBAC permissions
kubectl auth can-i --list --as=system:serviceaccount:beskar7-system:controller-manager
```

### Validate TLS Configuration

```bash
# Test certificate validation
openssl s_client -connect your-bmc.example.com:443 -verify_return_error

# Check PhysicalHost TLS settings
kubectl get physicalhost -o yaml | grep -A5 redfishConnection
```

### Monitor Security Metrics

```bash
# View security metrics
kubectl port-forward -n beskar7-system deployment/controller-manager 8080:8080
curl http://localhost:8080/metrics | grep beskar7_security
```

## Customization

### Environment-Specific Customization

Create your own configuration by copying and modifying one of the examples:

```bash
# Copy production config for customization
cp production.yaml my-environment.yaml

# Edit security policy
vim my-environment.yaml
```

### Common Customizations

#### Custom CA Certificate

```yaml
# Add your organization's CA certificate
apiVersion: v1
kind: Secret
metadata:
  name: corporate-ca-cert
  namespace: beskar7-system
data:
  ca.crt: <base64-encoded-ca-certificate>
```

#### Adjusted Password Policy

```yaml
# Customize password requirements
security:
  credentials:
    password_policy:
      min_length: 20              # Longer passwords
      require_special_chars: true
      prohibited_patterns:        # Block specific patterns
        - "company"
        - "beskar"
```

#### Custom Network Policies

```yaml
# Allow specific BMC subnets
egress:
- to:
  - ipBlock:
      cidr: 10.100.0.0/16  # BMC network
  ports:
  - protocol: TCP
    port: 443
```

## Troubleshooting

### Common Issues

1. **Certificate validation failures**: Check CA certificates and hostname verification
2. **RBAC permission denied**: Verify ClusterRole permissions match requirements
3. **Network connectivity issues**: Review network policies and firewall rules
4. **Resource limit violations**: Adjust resource limits based on actual usage

### Debug Commands

```bash
# Check controller logs
kubectl logs deployment/controller-manager -n beskar7-system --tail=100

# Test webhook validation
kubectl create -f test-physicalhost.yaml --dry-run=server

# Verify security monitoring
kubectl get events -n beskar7-system | grep security
```

## Migration Between Configurations

### From Development to Production

1. **Test in staging** with production configuration
2. **Update secrets** with strong passwords
3. **Configure certificates** for production BMCs
4. **Apply production configuration** during maintenance window
5. **Monitor for issues** and validate functionality

### From Production to Development

1. **Create development namespace** to avoid conflicts
2. **Apply development configuration** in separate namespace
3. **Update PhysicalHost resources** to point to development BMCs
4. **Use development credentials** with relaxed policies

## Security Best Practices

1. **Start with production configuration** and relax only as needed
2. **Use separate configurations** for different environments
3. **Regularly rotate credentials** according to your security policy
4. **Monitor security events** and respond to alerts promptly
5. **Keep configurations in version control** with proper access controls
6. **Test configuration changes** in non-production environments first

## Additional Resources

- [Security Documentation](../../docs/security/README.md)
- [Configuration Guide](../../docs/security/configuration.md)
- [Troubleshooting Guide](../../docs/security/troubleshooting.md)
- [Best Practices](../../docs/security/README.md#best-practices) 