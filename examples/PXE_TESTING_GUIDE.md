# PXE/iPXE Testing Guide for Beskar7

This guide provides step-by-step instructions for manually testing PXE and iPXE provisioning with Beskar7.

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Infrastructure Setup](#infrastructure-setup)
3. [Testing PXE Mode](#testing-pxe-mode)
4. [Testing iPXE Mode](#testing-ipxe-mode)
5. [Verification Steps](#verification-steps)
6. [Troubleshooting](#troubleshooting)

---

## Prerequisites

### Hardware Requirements

- **Physical servers** with:
  - Redfish-compliant BMC (iDRAC, iLO, BMC, etc.)
  - Network boot support (PXE/UEFI network boot)
  - Network connectivity to PXE/iPXE server
  - At least 2 network interfaces (1 for BMC, 1 for OS)

### Software Requirements

- **Kubernetes cluster** with Cluster API installed
- **Beskar7 controller** deployed
- **Network infrastructure**:
  - DHCP server (for PXE/iPXE)
  - TFTP server (for PXE) or HTTP server (for iPXE)
  - Web server for hosting OS images and configurations

### Network Configuration

```
Physical Network Layout:
┌─────────────────────────────────────┐
│   Management Network (BMC)          │
│   Subnet: 192.168.1.0/24            │
│   - BMC IPs: .10, .11, .12          │
└─────────────────────────────────────┘
           │
┌─────────────────────────────────────┐
│   Data Network (OS/PXE)             │
│   Subnet: 10.0.0.0/24               │
│   - DHCP Server: 10.0.0.1           │
│   - PXE/TFTP: 10.0.0.2              │
│   - HTTP Server: 10.0.0.3           │
└─────────────────────────────────────┘
```

---

## Infrastructure Setup

### Option 1: Quick Test with Docker (Development)

Use this for quick testing without full infrastructure:

```bash
# Run a simple PXE server using Docker
docker run -d --name pxe-server \
  -p 69:69/udp \
  -p 8080:80 \
  -v /path/to/pxe/files:/var/lib/tftpboot \
  netbootxyz/netbootxyz

# This provides both TFTP and HTTP serving
# Access http://localhost:8080 for the web interface
```

### Option 2: Production-Like PXE Setup

#### 1. DHCP Server Configuration

Edit `/etc/dhcp/dhcpd.conf`:

```conf
# PXE Boot Configuration
subnet 10.0.0.0 netmask 255.255.255.0 {
  range 10.0.0.100 10.0.0.200;
  option routers 10.0.0.1;
  option domain-name-servers 8.8.8.8;
  
  # PXE boot options
  next-server 10.0.0.2;  # TFTP server
  
  # BIOS clients
  if exists user-class and option user-class = "iPXE" {
    filename "http://10.0.0.3/boot.ipxe";
  } elsif option arch = 00:00 {
    filename "pxelinux.0";
  }
  # UEFI clients
  elsif option arch = 00:07 or option arch = 00:09 {
    filename "bootx64.efi";
  }
}
```

#### 2. TFTP Server Setup

```bash
# Install TFTP server
sudo apt-get install tftpd-hpa

# Configure TFTP
sudo mkdir -p /var/lib/tftpboot
sudo chown -R tftp:tftp /var/lib/tftpboot

# Download PXE bootloader files
cd /var/lib/tftpboot
wget http://boot.ipxe.org/ipxe.efi -O bootx64.efi

# Start TFTP service
sudo systemctl enable tftpd-hpa
sudo systemctl start tftpd-hpa
```

#### 3. HTTP Server for iPXE

```bash
# Install nginx
sudo apt-get install nginx

# Create iPXE boot directory
sudo mkdir -p /var/www/html/ipxe

# Create a simple iPXE boot script
cat <<'EOF' | sudo tee /var/www/html/ipxe/boot.ipxe
#!ipxe

# iPXE Boot Script for Beskar7 Testing
echo Starting iPXE boot for Beskar7...

# Set variables
set base-url http://10.0.0.3/images

# Display menu
:menu
menu iPXE Boot Menu
item --key f flatcar Boot Flatcar Container Linux
item --key k kairos Boot Kairos
choose --default flatcar --timeout 5000 target && goto ${target}

:flatcar
echo Booting Flatcar Container Linux...
kernel ${base-url}/flatcar/flatcar_production_pxe.vmlinuz
initrd ${base-url}/flatcar/flatcar_production_pxe_image.cpio.gz
imgargs flatcar_production_pxe.vmlinuz flatcar.first_boot=1 flatcar.autologin
boot

:kairos
echo Booting Kairos...
kernel ${base-url}/kairos/kernel
initrd ${base-url}/kairos/initrd
imgargs kernel config_url=http://10.0.0.3/configs/kairos-config.yaml
boot
EOF

# Start nginx
sudo systemctl enable nginx
sudo systemctl start nginx
```

### Option 3: iPXE Server Setup

#### Using Netboot.xyz (Recommended for Testing)

```bash
# Deploy netboot.xyz which provides a comprehensive iPXE menu
docker run -d \
  --name=netbootxyz \
  -e PUID=1000 \
  -e PGID=1000 \
  -p 3000:3000 \
  -p 69:69/udp \
  -p 8080:80 \
  -v /path/to/config:/config \
  -v /path/to/assets:/assets \
  --restart unless-stopped \
  ghcr.io/netbootxyz/netbootxyz
```

---

## Testing PXE Mode

### Step 1: Prepare the Environment

```bash
# Ensure Beskar7 is running
kubectl get pods -n beskar7-system

# Create the test namespace
kubectl create namespace pxe-test
```

### Step 2: Deploy the Example

```bash
# Apply the PXE example
kubectl apply -f examples/pxe-provisioning-example.yaml

# Watch the resources
watch kubectl get physicalhost,beskar7machine,machine -n pxe-cluster
```

### Step 3: Monitor PXE Boot

```bash
# Check PhysicalHost status
kubectl describe physicalhost pxe-control-plane-01 -n pxe-cluster

# Watch Beskar7Machine events
kubectl describe beskar7machine -n pxe-cluster

# Check controller logs
kubectl logs -n beskar7-system -l control-plane=controller-manager -f
```

### Step 4: Verify BMC Configuration

What Beskar7 does when you apply the example:

1. **Detects available PhysicalHost** in `Available` state
2. **Claims the host** for the Beskar7Machine
3. **Connects to BMC** via Redfish
4. **Configures boot source** to PXE:
   ```go
   // Sets boot override to PXE
   BootSourceOverrideTarget: "Pxe"
   BootSourceOverrideEnabled: "Once"
   ```
5. **Powers on the machine** (if needed)

### Step 5: Expected Behavior

```bash
# PhysicalHost should transition through states:
# None → Enrolling → Available → Claimed → Provisioning → Provisioned

# Check state transitions
kubectl get physicalhost -n pxe-cluster -w

# Expected output:
# NAME                      STATE          READY
# pxe-control-plane-01      Available      true
# pxe-control-plane-01      Claimed        true
# pxe-control-plane-01      Provisioning   true
# (Machine should PXE boot here)
# pxe-control-plane-01      Provisioned    true
```

### Step 6: Verify Network Boot

Check on your PXE server logs:

```bash
# TFTP server logs
sudo journalctl -u tftpd-hpa -f

# Expected to see:
# TFTP request for bootx64.efi from 10.0.0.100
# File sent successfully

# DHCP server logs
sudo journalctl -u isc-dhcp-server -f

# Expected to see:
# DHCP request from MAC address
# Offered IP 10.0.0.100
# PXE boot options sent
```

---

## Testing iPXE Mode

### Step 1: Deploy iPXE Example

```bash
# Apply the iPXE example
kubectl apply -f examples/ipxe-provisioning-example.yaml

# Watch resources
watch kubectl get physicalhost,beskar7machine -n ipxe-cluster
```

### Step 2: Monitor iPXE Boot

```bash
# Check detailed status
kubectl describe beskar7machine -n ipxe-cluster

# View controller logs with filtering
kubectl logs -n beskar7-system -l control-plane=controller-manager -f | grep -i "ipxe\|pxe\|boot"
```

### Step 3: Verify iPXE Script Execution

What happens during iPXE boot:

1. **BMC configured** for network boot (same as PXE)
2. **Machine PXE boots** and chains to iPXE
3. **iPXE loads** from HTTP server
4. **iPXE script executes** (boot.ipxe)
5. **OS installation** proceeds per script

Monitor HTTP server access logs:

```bash
# Nginx access logs
sudo tail -f /var/log/nginx/access.log

# Expected entries:
# GET /ipxe/boot.ipxe
# GET /images/flatcar/flatcar_production_pxe.vmlinuz
# GET /images/flatcar/flatcar_production_pxe_image.cpio.gz
```

---

## Verification Steps

### 1. Verify Beskar7 Configuration

```bash
# Check that Beskar7 set PXE boot correctly
kubectl get beskar7machine -n pxe-cluster -o yaml | grep -A5 "provisioningMode"

# Expected:
#   provisioningMode: PXE (or iPXE)
#   bootMode: UEFI
```

### 2. Check BMC via Redfish API Directly

```bash
# Query the BMC to verify boot configuration
# (Replace with your BMC IP and credentials)
curl -k -u admin:password \
  https://192.168.1.10/redfish/v1/Systems/System.Embedded.1 | jq '.Boot'

# Expected JSON response:
# {
#   "BootSourceOverrideEnabled": "Once",
#   "BootSourceOverrideTarget": "Pxe",
#   "BootSourceOverrideMode": "UEFI"
# }
```

### 3. Verify PhysicalHost Conditions

```bash
# Check all conditions
kubectl get physicalhost pxe-control-plane-01 -n pxe-cluster -o json | \
  jq '.status.conditions'

# Expected conditions:
# - RedfishConnectionReady: True
# - HostAvailable: True (or False if claimed)
```

### 4. Check Machine Addresses

```bash
# After successful boot, machine should report addresses
kubectl get machine -n pxe-cluster -o yaml | grep -A10 "addresses:"

# Expected:
# addresses:
# - address: 10.0.0.100
#   type: InternalIP
```

---

## Troubleshooting

### Issue: BMC Not Booting from Network

**Symptoms:**
- Machine powers on but doesn't PXE boot
- PhysicalHost stuck in "Provisioning" state

**Checks:**

```bash
# 1. Verify BMC supports PXE
curl -k -u admin:password \
  https://192.168.1.10/redfish/v1/Systems/System.Embedded.1 | \
  jq '.Boot.BootSourceOverrideTarget@Redfish.AllowableValues'

# Should include "Pxe" in the list

# 2. Check controller logs for errors
kubectl logs -n beskar7-system -l control-plane=controller-manager --tail=100 | grep -i error

# 3. Verify network connectivity
# The host's data network interface should reach DHCP/PXE server
```

**Solutions:**
- Enable PXE boot in BIOS/UEFI settings
- Check network cable on data interface
- Verify VLAN configuration matches
- Try Legacy boot mode if UEFI fails

### Issue: DHCP Not Responding

**Symptoms:**
- Machine PXE boots but gets no IP
- DHCP timeout errors on console

**Checks:**

```bash
# 1. Test DHCP server
sudo nmap --script broadcast-dhcp-discover -e eth0

# 2. Check DHCP server logs
sudo journalctl -u isc-dhcp-server -n 50

# 3. Verify network configuration
ip addr show
ip route show
```

**Solutions:**
- Ensure DHCP server is running
- Check firewall rules (port 67/68)
- Verify network interface configuration
- Check for DHCP relay if on different subnet

### Issue: TFTP Timeout

**Symptoms:**
- Gets IP via DHCP but can't download bootloader
- "TFTP timeout" on console

**Checks:**

```bash
# 1. Test TFTP manually
tftp 10.0.0.2
> get bootx64.efi
> quit

# 2. Check TFTP server status
sudo systemctl status tftpd-hpa
sudo netstat -ulnp | grep :69

# 3. Verify firewall
sudo ufw status | grep 69
```

**Solutions:**
- Restart TFTP server
- Check file permissions in /var/lib/tftpboot
- Open UDP port 69 in firewall
- Verify next-server IP in DHCP config

### Issue: iPXE Script Not Loading

**Symptoms:**
- iPXE loads but script fails
- HTTP 404 errors

**Checks:**

```bash
# 1. Test HTTP server
curl http://10.0.0.3/ipxe/boot.ipxe

# 2. Check nginx logs
sudo tail -f /var/log/nginx/error.log

# 3. Verify file exists
ls -la /var/www/html/ipxe/
```

**Solutions:**
- Fix file path in boot.ipxe
- Check nginx configuration
- Verify file permissions (644)
- Test URLs from another machine

### Issue: Beskar7 Controller Errors

**Symptoms:**
- PhysicalHost not transitioning states
- Error conditions on resources

**Debug Steps:**

```bash
# 1. Get detailed controller logs
kubectl logs -n beskar7-system deployment/beskar7-controller-manager --all-containers=true --tail=200

# 2. Check for Redfish connection errors
kubectl logs -n beskar7-system deployment/beskar7-controller-manager | grep -i "redfish\|connection"

# 3. Verify BMC credentials
kubectl get secret bmc-credentials -n pxe-cluster -o jsonpath='{.data.username}' | base64 -d
kubectl get secret bmc-credentials -n pxe-cluster -o jsonpath='{.data.password}' | base64 -d

# 4. Test BMC access manually
curl -k -u admin:password https://192.168.1.10/redfish/v1/Systems
```

### Common Error Messages

| Error | Cause | Solution |
|-------|-------|----------|
| "PXE boot configuration failed" | BMC doesn't support PXE | Check hardware compatibility, try Legacy mode |
| "Failed to set boot source" | Redfish API error | Verify BMC firmware version, check permissions |
| "ensure BMC supports PXE boot" | Missing PXE capability | Enable in BIOS/UEFI settings |
| "network boot infrastructure is available" | No DHCP/PXE server | Setup PXE infrastructure first |

---

## Validation Checklist

Use this checklist to verify your setup:

- [ ] BMC accessible via Redfish API
- [ ] DHCP server responding on data network
- [ ] TFTP server (PXE) or HTTP server (iPXE) accessible
- [ ] Boot files available and accessible
- [ ] Beskar7 controller running
- [ ] PhysicalHosts in Available state
- [ ] Secrets created with correct credentials
- [ ] Network connectivity between all components
- [ ] Firewall rules allow required ports
- [ ] BMC supports network boot

---

## Advanced Testing

### Test with Multiple Provisioning Modes

```bash
# Create different machines with different modes
kubectl apply -f - <<EOF
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: Beskar7Machine
metadata:
  name: test-pxe
  namespace: default
spec:
  provisioningMode: "PXE"
  osFamily: "flatcar"
  imageURL: "http://pxe-server/flatcar.iso"
---
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: Beskar7Machine
metadata:
  name: test-ipxe
  namespace: default
spec:
  provisioningMode: "iPXE"
  osFamily: "kairos"
  imageURL: "http://ipxe-server/boot.ipxe"
---
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: Beskar7Machine
metadata:
  name: test-iso
  namespace: default
spec:
  provisioningMode: "PreBakedISO"
  osFamily: "kairos"
  imageURL: "https://github.com/kairos-io/kairos/releases/download/v2.8.1/kairos-alpine-v2.8.1-amd64.iso"
EOF

# Compare how each mode behaves
watch kubectl get beskar7machine
```

### Monitor Redfish Interactions

```bash
# Enable verbose logging (if supported by your Beskar7 deployment)
kubectl set env deployment/beskar7-controller-manager -n beskar7-system LOG_LEVEL=debug

# Watch detailed Redfish calls
kubectl logs -n beskar7-system -l control-plane=controller-manager -f | grep -E "SetBootSource|SetPowerState|GetPowerState"
```

---

## Clean Up

After testing:

```bash
# Delete test clusters
kubectl delete -f examples/pxe-provisioning-example.yaml
kubectl delete -f examples/ipxe-provisioning-example.yaml

# Verify PhysicalHosts return to Available
kubectl get physicalhost --all-namespaces

# Clean up namespaces
kubectl delete namespace pxe-cluster
kubectl delete namespace ipxe-cluster
```

---

## Next Steps

Once PXE/iPXE boot is working:

1. **Configure OS Installation**: Ensure your PXE/iPXE scripts properly install and configure the OS
2. **Test Cluster Formation**: Verify nodes join the Kubernetes cluster
3. **Automate**: Create automation for PXE server configuration
4. **Scale**: Test with multiple nodes provisioning simultaneously
5. **Production**: Move to production-grade PXE infrastructure

---

## Additional Resources

- [Beskar7 Documentation](../docs/README.md)
- [PXE Boot Specification](https://www.intel.com/content/www/us/en/download/19098/preboot-execution-environment-pxe-specification.html)
- [iPXE Documentation](https://ipxe.org/docs)
- [Netboot.xyz](https://netboot.xyz/)
- [Redfish API Documentation](https://www.dmtf.org/standards/redfish)

