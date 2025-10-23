# PXE/iPXE Quick Start Guide

Quick reference for testing PXE/iPXE provisioning with Beskar7.

## 1. Quick Test (5 minutes)

### Prerequisites
- Beskar7 controller running
- Physical server with BMC
- Basic PXE server (or use Docker)

### Start PXE Server (Docker)
```bash
docker run -d --name pxe-test \
  -p 69:69/udp \
  -p 8080:80 \
  netbootxyz/netbootxyz
```

### Deploy Test
```bash
# 1. Edit pxe-simple-test.yaml with your BMC IP and credentials
vi examples/pxe-simple-test.yaml

# 2. Apply
kubectl apply -f examples/pxe-simple-test.yaml

# 3. Watch (in separate terminals)
watch kubectl get physicalhost,beskar7machine -n pxe-test
kubectl logs -n beskar7-system -l control-plane=controller-manager -f
```

### Expected Timeline
- **0-30s**: PhysicalHost enrolls, becomes Available
- **30-60s**: Beskar7Machine claims host
- **60-90s**: BMC configured for PXE boot
- **90s+**: Machine powers on, PXE boots

## 2. Verify BMC Configuration

```bash
# Check that Beskar7 set PXE boot
curl -k -u admin:password \
  https://YOUR_BMC_IP/redfish/v1/Systems/System.Embedded.1 | \
  jq '.Boot.BootSourceOverrideTarget'

# Should return: "Pxe"
```

## 3. What Beskar7 Does

When you create a Beskar7Machine with `provisioningMode: "PXE"`:

1. ✅ Claims an available PhysicalHost
2. ✅ Connects to BMC via Redfish
3. ✅ Sets boot override: `BootSourceOverrideTarget = "Pxe"`
4. ✅ Ensures one-time boot: `BootSourceOverrideEnabled = "Once"`
5. ✅ Powers on the machine (if needed)
6. ⚠️ **You provide**: PXE infrastructure (DHCP, TFTP, OS images)

## 4. Troubleshooting Commands

```bash
# PhysicalHost status
kubectl describe physicalhost -n pxe-test

# Beskar7Machine status
kubectl describe beskar7machine -n pxe-test

# Controller logs (last 50 lines)
kubectl logs -n beskar7-system -l control-plane=controller-manager --tail=50

# Filter for PXE-related logs
kubectl logs -n beskar7-system -l control-plane=controller-manager | grep -i "pxe\|boot"

# Check for errors
kubectl logs -n beskar7-system -l control-plane=controller-manager | grep -i error
```

## 5. Common Issues

| Problem | Check | Solution |
|---------|-------|----------|
| PhysicalHost stuck in Enrolling | BMC connectivity | Test: `curl -k https://BMC_IP/redfish/v1` |
| "PXE boot configuration failed" | BMC capability | Check: Boot.BootSourceOverrideTarget@Redfish.AllowableValues |
| Machine doesn't PXE boot | Network config | Verify data network, DHCP server |
| "ensure BMC supports PXE" | BIOS settings | Enable network boot in BIOS/UEFI |

## 6. Success Indicators

✅ **PhysicalHost**
```bash
kubectl get physicalhost -n pxe-test
# STATE: Claimed → Provisioning
# READY: true
```

✅ **Beskar7Machine**
```bash
kubectl get beskar7machine -n pxe-test
# CONDITIONS: PhysicalHostAssociated=True
```

✅ **Controller Logs**
```
Configuring boot for PXE mode
Successfully configured PXE boot
ensure PXE server is properly configured
```

✅ **BMC API**
```json
{
  "BootSourceOverrideTarget": "Pxe",
  "BootSourceOverrideEnabled": "Once"
}
```

✅ **Physical Console**
- Machine powers on
- Shows PXE boot messages
- DHCP request visible
- Begins network boot

## 7. Cleanup

```bash
kubectl delete -f examples/pxe-simple-test.yaml
docker stop pxe-test && docker rm pxe-test
```

## 8. Next Steps

Once basic PXE boot works:

1. **Setup proper PXE infrastructure** - See `PXE_TESTING_GUIDE.md`
2. **Test iPXE mode** - Uncomment iPXE section in `pxe-simple-test.yaml`
3. **Deploy full cluster** - Use `pxe-provisioning-example.yaml`
4. **Automate** - Integrate with your provisioning workflow

## 9. Full Examples

For complete cluster deployments:
- **PXE Cluster**: `pxe-provisioning-example.yaml`
- **iPXE Cluster**: `ipxe-provisioning-example.yaml`
- **Detailed Guide**: `PXE_TESTING_GUIDE.md`

## 10. Key Differences: PXE vs iPXE

| Feature | PXE | iPXE |
|---------|-----|------|
| Protocol | TFTP (slow) | HTTP (fast) |
| Scripting | Limited | Advanced |
| Setup | Traditional | Modern |
| Boot Files | pxelinux.0 | ipxe.efi |
| Config | DHCP options | iPXE scripts |
| **Beskar7 Config** | Same | Same |

**Note**: From Beskar7's perspective, both modes are identical - it just configures the BMC to boot from network. The difference is in your PXE infrastructure.

---

## Quick Reference: Provisioning Modes

```yaml
# RemoteConfig - Boot ISO with config URL
provisioningMode: "RemoteConfig"
configURL: "https://server/config.yaml"
imageURL: "https://server/kairos.iso"

# PreBakedISO - Boot pre-configured ISO
provisioningMode: "PreBakedISO"
imageURL: "https://server/custom-kairos.iso"

# PXE - Network boot via PXE/TFTP
provisioningMode: "PXE"
imageURL: "http://pxe-server/reference.iso"  # Reference only

# iPXE - Network boot via iPXE/HTTP
provisioningMode: "iPXE"
imageURL: "http://ipxe-server/boot.ipxe"  # Script URL
```

---

**For detailed setup instructions, see: `PXE_TESTING_GUIDE.md`**

