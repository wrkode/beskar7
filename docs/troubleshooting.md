# Troubleshooting

This document provides guidance on troubleshooting common issues encountered when using Beskar7.

## Controller Logs

The first place to look for issues is the logs of the Beskar7 controller manager pod, usually running in the `beskar7-system` namespace.

```bash
# List the controller manager pods
kubectl get pods -n beskar7-system -l control-plane=controller-manager

# View the logs
kubectl logs -n beskar7-system -f <pod-name> -c manager
```

Increase verbosity by editing the manager Deployment (`config/manager/manager.yaml` or via `kubectl edit deployment -n beskar7-system controller-manager`) and adding a `-v=X` argument (e.g., `-v=5`) to the manager container's args list, then restart the pod.

## Webhook and Certificate Issues

### Controller Fails with Missing TLS Certificate

*   **Error:** `open /tmp/k8s-webhook-server/serving-certs/tls.crt: no such file or directory`
    *   **Cause:** The controller-manager cannot find the webhook TLS certificate. This is almost always because cert-manager is not installed, not running, or the certificate/secret is missing.
    *   **Troubleshooting:**
        1. **Ensure cert-manager is installed and running:**
            ```bash
            kubectl get pods -n cert-manager
            ```
            All pods should be `Running`.
        2. **Check for the certificate and secret:**
            ```bash
            kubectl get certificate -n beskar7-system
            kubectl get secret -n beskar7-system
            ```
            You should see a certificate (e.g., `beskar7-serving-cert`) and a secret (e.g., `beskar7-webhook-server-cert`).
        3. **If missing, re-apply the certificate manifest:**
            ```bash
            kubectl apply -f config/certmanager/certificate.yaml
            ```
        4. **Check cert-manager logs for errors:**
            ```bash
            kubectl logs -n cert-manager -l app=cert-manager
            ```
        5. **If you see errors about the namespace being terminated, ensure the `beskar7-system` namespace is `Active` and not stuck in `Terminating`.

*   **Error:** `the server could not find the requested resource (post certificates.cert-manager.io)`
    *   **Cause:** cert-manager CRDs are not installed.
    *   **Troubleshooting:**
        1. Install cert-manager CRDs:
            ```bash
            kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.12.0/cert-manager.crds.yaml
            ```
        2. Install cert-manager:
            ```bash
            kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.12.0/cert-manager.yaml
            ```
        3. Wait for all cert-manager pods to be running:
            ```bash
            kubectl get pods -n cert-manager
            ```

## Common Issues & Solutions

### `PhysicalHost` Reconciliation Errors

*   **Error: "Failed to connect to Redfish endpoint ... dial tcp ... i/o timeout"**
    *   **Cause:** Network connectivity issue between the controller manager pod and the BMC IP address.
    *   **Troubleshooting:**
        *   Verify the BMC IP address in the `PhysicalHost.spec.redfishConnection.address` is correct.
        *   Check network routes and firewalls between your Kubernetes nodes and the BMC network.
        *   Verify the Redfish service is enabled and running on the BMC (usually port 443 for HTTPS).
        *   Try pinging or connecting (`nc -vz <bmc_ip> 443`) from a Kubernetes node.
        *   Try connecting from a debug pod within the cluster.

*   **Error: "Failed to connect to Redfish endpoint ... unable to execute request, no target provided"**
    *   **Cause:** Often indicates an issue within the underlying HTTP client or `gofish` library when parsing or preparing the request for the specified endpoint URL, even if basic connectivity exists.
    *   **Troubleshooting:**
        *   Ensure the `address` in `PhysicalHost.spec.redfishConnection` is correctly formatted (e.g., `https://1.2.3.4` or just `1.2.3.4`).
        *   Check the version of the `gofish` library being used (`go.mod`) and consider updating.
        *   Verify DNS resolution within the controller pod if using hostnames.

*   **Error: "Failed to connect to Redfish endpoint ... authentication failed"** (or similar relating to auth)
    *   **Cause:** Incorrect username or password in the referenced credentials Secret.
    *   **Troubleshooting:**
        *   Verify the `credentialsSecretRef` in `PhysicalHost.spec.redfishConnection` points to the correct Secret name.
        *   Check the content of the Secret (`kubectl get secret <secret-name> -o yaml`) ensures the `username` and `password` keys exist and contain the correct base64-encoded credentials.
        *   Verify the user account is enabled and has appropriate privileges on the BMC.

*   **Error: "Failed to connect to Redfish endpoint ... x509: certificate signed by unknown authority"** (or similar TLS errors)
    *   **Cause:** The BMC is using a self-signed or untrusted TLS certificate, and Beskar7 is configured to verify certificates.
    *   **Troubleshooting:**
        *   **Recommended:** Configure the BMC with a certificate signed by a trusted CA.
        *   **Less Secure:** Set `insecureSkipVerify: true` in the `PhysicalHost.spec.redfishConnection` block. Use this with caution.

*   **`PhysicalHost` Stuck in `Enrolling` or `Error` State:**
    *   Check controller logs for connection or query errors as described above.
    *   Verify Redfish service health on the BMC itself.

### `Beskar7Machine` Reconciliation Errors

*   **Machine Stuck Waiting for `PhysicalHost`:**
    *   **Log:** `No associated or available PhysicalHost found, requeuing`
    *   **Cause:** No `PhysicalHost` resources in the `Available` state exist in the same namespace as the `Beskar7Machine`.
    *   **Troubleshooting:**
        *   Ensure `PhysicalHost` resources for your hardware exist.
        *   Check the status of existing `PhysicalHost`s (`kubectl get physicalhost -o wide`). Are they `Available`? If not, check their logs/conditions to see why.
        *   Ensure `PhysicalHost`s are in the same namespace as the `Beskar7Machine`.

*   **Machine Stuck Claiming Host / Configuring Boot:**
    *   **Log:** `Failed to get Redfish client for host provisioning...`, `Failed to set boot parameters...`, `Failed to set boot source ISO...`
    *   **Cause:** Errors occurred during the second phase of reconciliation where the `Beskar7MachineController` connects to the claimed `PhysicalHost`'s BMC to configure boot settings.
    *   **Troubleshooting:** Check the specific error message. It often relates back to Redfish connectivity, authentication (same checks as for `PhysicalHost`), or BMC capability issues.

*   **RemoteConfig Fails - `SetBootParameters` Error:**
    *   **Log:** `Failed to set boot settings via UefiTargetBootSourceOverride...`
    *   **Cause:** As noted in Advanced Usage, setting kernel parameters via Redfish (`UefiTargetBootSourceOverride`) is vendor-dependent and may not be supported or may require specific EFI paths.
    *   **Troubleshooting:**
        *   Check BMC documentation for Redfish boot override capabilities.
        *   Inspect the target ISO to verify the EFI bootloader path (`\EFI\BOOT\BOOTX64.EFI` is a guess).
        *   Consider using the `PreBakedISO` mode as an alternative if `RemoteConfig` is unreliable for your hardware.
        *   Check for more detailed Redfish error messages in the logs (if available via `common.Error`).

*   **Virtual Media / ISO Boot Issues:**
    *   **Log:** `Failed to insert virtual media...`, `Failed to set boot source override...`
    *   **Cause:** Problems with the BMC's virtual media service.
    *   **Troubleshooting:**
        *   Verify the `imageURL` in the `Beskar7MachineSpec` is correct and accessible from the network where the BMC resides.
        *   Check BMC logs/status for virtual media errors.
        *   Ensure the BMC supports mounting ISOs from the specified URL type (e.g., HTTP, HTTPS, NFS - Beskar7 currently assumes HTTP/S via `gofish`).

## Webhook Validation Errors

### Resource Creation/Update Rejected by Webhooks

*   **Error: `admission webhook denied the request`**
    *   **Cause:** Resource specification violates validation rules enforced by admission webhooks.
    *   **Common validation failures:**

#### PhysicalHost Validation Errors

*   **Error: `insecureSkipVerify=true is not allowed in production environments`**
    *   **Cause:** TLS certificate validation is disabled in non-development environment
    *   **Solution:** Configure proper TLS certificates or add development annotations

*   **Error: `invalid Redfish address format`**
    *   **Cause:** Address doesn't match expected format (IP or FQDN with optional scheme)
    *   **Solution:** Use format like `https://192.168.1.100` or `192.168.1.100`

*   **Error: `credentialsSecretRef is required`**
    *   **Cause:** Missing reference to credentials secret
    *   **Solution:** Create credentials secret and reference it properly

#### Beskar7Machine Validation Errors

*   **Error: `invalid URL format for imageURL`**
    *   **Cause:** ImageURL doesn't point to supported image format
    *   **Solution:** Use URLs ending in .iso, .img, .qcow2, etc.

*   **Error: `osFamily 'xyz' is not supported`**
    *   **Cause:** Unsupported operating system family specified
    *   **Solution:** Use supported OS families: kairos, talos, flatcar, ubuntu, etc.

*   **Error: `configURL is required for RemoteConfig mode`**
    *   **Cause:** Missing configuration URL for RemoteConfig provisioning
    *   **Solution:** Provide valid configURL or change to PreBakedISO mode

*   **Error: `configURL is not allowed for PreBakedISO mode`**
    *   **Cause:** Configuration URL specified for pre-baked ISO mode
    *   **Solution:** Remove configURL or change to RemoteConfig mode

#### Beskar7MachineTemplate Validation Errors

*   **Error: `providerID should not be set in machine templates`**
    *   **Cause:** ProviderID is managed by controllers and forbidden in templates
    *   **Solution:** Remove providerID from template specification

*   **Error: `imageURL is immutable in machine templates`**
    *   **Cause:** Attempting to modify immutable field after creation
    *   **Solution:** Create new template version instead of modifying existing one

*   **Error: `template validation failed`**
    *   **Cause:** Template spec violates Beskar7Machine validation rules
    *   **Solution:** Fix template spec according to Beskar7Machine requirements

### Webhook Service Connectivity Issues

*   **Error: `failed calling webhook ... connection refused`**
    *   **Cause:** Kubernetes API server cannot reach webhook service
    *   **Troubleshooting:**
        1. Check webhook service exists: `kubectl get svc -n beskar7-system beskar7-webhook-service`
        2. Check webhook configuration: `kubectl get validatingwebhookconfiguration,mutatingwebhookconfiguration`
        3. Verify webhook pods are running: `kubectl get pods -n beskar7-system`
        4. Check webhook endpoints: `kubectl get endpoints -n beskar7-system`

### General Tips

*   **Check CRD Status:** `kubectl get crd physicalhosts.infrastructure.cluster.x-k8s.io -o yaml`, etc. Ensure they are established.
*   **Check Resource Status:** Use `kubectl get physicalhost <n> -o yaml` and `kubectl get beskar7machine <n> -o yaml` to inspect the full status, including conditions and reported states.
*   **Check BMC:** Log in directly to the BMC (Web UI, SSH) to verify its status, Redfish service state, virtual media status, and power state.
*   **Validate Before Apply:** Use `kubectl apply --dry-run=server` to validate resources before creation
*   **Check Webhook Logs:** View webhook validation details in controller manager logs with increased verbosity (-v=5) 