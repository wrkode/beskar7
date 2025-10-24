# PXE/iPXE Infrastructure Prerequisites

This document details all infrastructure components required to support PXE and iPXE provisioning with Beskar7.

## Table of Contents

1. [Overview](#overview)
2. [Network Infrastructure](#network-infrastructure)
3. [DHCP Server Requirements](#dhcp-server-requirements)
4. [TFTP Server Requirements (PXE)](#tftp-server-requirements-pxe)
5. [HTTP Server Requirements (iPXE)](#http-server-requirements-ipxe)
6. [OS Image Hosting](#os-image-hosting)
7. [DNS Requirements](#dns-requirements)
8. [Firewall Configuration](#firewall-configuration)
9. [Hardware Requirements](#hardware-requirements)
10. [Optional Components](#optional-components)
11. [Architecture Reference](#architecture-reference)
12. [Validation Checklist](#validation-checklist)

---

## Overview

Beskar7 configures the BMC to boot from network, but the actual network boot infrastructure must be provided separately. This document outlines all required components.

### What Beskar7 Provides

- ✅ BMC configuration via Redfish API
- ✅ Setting boot override to PXE
- ✅ Power management
- ✅ Host lifecycle orchestration

### What You Must Provide

- ⚠️ DHCP server with PXE boot options
- ⚠️ TFTP server (for PXE) or HTTP server (for iPXE)
- ⚠️ Boot loader files
- ⚠️ OS installation images
- ⚠️ Configuration management (kickstart, cloud-init, ignition, etc.)
- ⚠️ Network infrastructure

---

## Network Infrastructure

### Network Segments

You need at least two network segments:

```
┌─────────────────────────────────────────────────┐
│ Management Network (Out-of-Band)               │
│ Purpose: BMC/Redfish access                    │
│ Example: 192.168.1.0/24                        │
│ Devices:                                        │
│  - BMCs (iDRAC, iLO, etc.)                     │
│  - Beskar7 controller                          │
│  - Management workstations                      │
└─────────────────────────────────────────────────┘
                    │
┌─────────────────────────────────────────────────┐
│ Data Network (In-Band)                         │
│ Purpose: OS networking, PXE boot              │
│ Example: 10.0.0.0/16                           │
│ Devices:                                        │
│  - Server data NICs                            │
│  - DHCP server                                 │
│  - PXE/TFTP server                            │
│  - HTTP/iPXE server                           │
│  - Kubernetes pod/service networks            │
└─────────────────────────────────────────────────┘
```

### Network Requirements

| Component | Requirement | Notes |
|-----------|-------------|-------|
| **Management Network** | Routable to BMCs | Can be isolated from data network |
| **Data Network** | Layer 2 for PXE, Layer 3 capable | Must support DHCP broadcast |
| **VLANs** | Optional but recommended | Separate management, provisioning, production |
| **MTU** | 1500+ bytes | 9000 (jumbo frames) recommended for iPXE |
| **Bandwidth** | 1 Gbps minimum | 10 Gbps recommended for multiple simultaneous boots |

### Network Topology Options

#### Option 1: Flat Network (Simple)
```
[Servers] ──┬── [DHCP/PXE Server]
            │
            ├── [Beskar7 Controller]
            │
            └── [Management Network] ── [BMCs]
```

#### Option 2: Segregated Networks (Recommended)
```
[Server BMCs] ── [Management VLAN 10] ── [Beskar7 Controller]
                                         
[Server NICs] ── [Provisioning VLAN 20] ── [DHCP/PXE/iPXE]
              │
              └─ [Production VLAN 30] ── [Kubernetes Services]
```

#### Option 3: Enterprise (Best Practice)
```
                    [Core Switch]
                         │
        ┌────────────────┼────────────────┐
        │                │                │
   [Management]    [Provisioning]   [Production]
     VLAN 10         VLAN 20          VLAN 30
        │                │                │
     [BMCs]         [PXE/DHCP]      [K8s Cluster]
                         │
                    [Servers]
```

---

## DHCP Server Requirements

### Minimum Requirements

| Specification | Value |
|--------------|-------|
| **Software** | ISC DHCP, dnsmasq, or Windows DHCP |
| **RAM** | 512 MB minimum |
| **CPU** | 1 core |
| **Storage** | 1 GB for logs |
| **Network** | Connected to provisioning network |
| **Ports** | UDP 67, 68 (DHCP) |

### Configuration Requirements

Your DHCP server must provide:

1. **IP Address Assignment**
   - DHCP pool with sufficient addresses
   - Static reservations for infrastructure servers (optional)
   - Appropriate lease times (shorter during provisioning)

2. **PXE Boot Options**
   - Next-server (TFTP server IP)
   - Boot filename (based on client architecture)
   - Option 66 (TFTP server)
   - Option 67 (boot filename)

3. **iPXE Chainloading**
   - Detect iPXE clients
   - Provide iPXE script URL
   - Option 175 (iPXE-specific options)

### ISC DHCP Configuration Example

```conf
# /etc/dhcp/dhcpd.conf

# Global settings
default-lease-time 600;
max-lease-time 7200;
authoritative;

# Subnet configuration
subnet 10.0.0.0 netmask 255.255.0.0 {
  range 10.0.100.0 10.0.199.255;
  option routers 10.0.0.1;
  option domain-name-servers 10.0.0.10, 8.8.8.8;
  option domain-name "cluster.local";
  
  # PXE boot configuration
  next-server 10.0.0.2;  # TFTP server IP
  
  # Architecture-specific boot files
  if option arch = 00:00 {
    # BIOS x86
    filename "pxelinux.0";
  } elsif option arch = 00:07 {
    # UEFI x64
    filename "bootx64.efi";
  } elsif option arch = 00:09 {
    # UEFI x64 HTTP
    filename "http://10.0.0.3/ipxe/bootx64.efi";
  }
  
  # iPXE chainloading
  if exists user-class and option user-class = "iPXE" {
    filename "http://10.0.0.3/ipxe/boot.ipxe";
  }
}

# Static reservations (optional)
host control-plane-01 {
  hardware ethernet 00:11:22:33:44:55;
  fixed-address 10.0.1.10;
}
```

### dnsmasq Configuration Example

```conf
# /etc/dnsmasq.conf

# DHCP settings
interface=eth0
dhcp-range=10.0.100.0,10.0.199.255,24h
dhcp-option=option:router,10.0.0.1
dhcp-option=option:dns-server,10.0.0.10,8.8.8.8

# Enable TFTP
enable-tftp
tftp-root=/var/lib/tftpboot

# PXE boot options
dhcp-boot=tag:!ipxe,bootx64.efi
dhcp-boot=tag:ipxe,http://10.0.0.3/ipxe/boot.ipxe

# iPXE detection
dhcp-userclass=set:ipxe,iPXE

# Architecture detection
dhcp-match=set:efi-x86_64,option:client-arch,7
dhcp-match=set:efi-x86_64,option:client-arch,9
dhcp-match=set:bios,option:client-arch,0
```

### Validation Commands

```bash
# Check DHCP server status
sudo systemctl status isc-dhcp-server  # or dnsmasq

# Monitor DHCP requests (tcpdump)
sudo tcpdump -i eth0 -n port 67 and port 68

# Test DHCP from client
sudo dhclient -v eth0

# Check DHCP leases
cat /var/lib/dhcp/dhcpd.leases

# Verify DHCP options
nmap --script broadcast-dhcp-discover -e eth0
```

---

## TFTP Server Requirements (PXE)

Required for traditional PXE boot (not needed if using pure iPXE).

### Minimum Requirements

| Specification | Value |
|--------------|-------|
| **Software** | tftpd-hpa, atftpd, or dnsmasq |
| **RAM** | 256 MB |
| **CPU** | 1 core |
| **Storage** | 10 GB for boot files |
| **Network** | Connected to provisioning network |
| **Ports** | UDP 69 (TFTP) |

### Installation

```bash
# Ubuntu/Debian
sudo apt-get install tftpd-hpa

# RHEL/CentOS
sudo yum install tftp-server

# Configure
sudo systemctl enable tftpd-hpa
sudo systemctl start tftpd-hpa
```

### Directory Structure

```
/var/lib/tftpboot/
├── pxelinux.0              # BIOS boot loader
├── bootx64.efi             # UEFI boot loader
├── ldlinux.c32             # Required for pxelinux
├── libcom32.c32
├── libutil.c32
├── menu.c32
├── vesamenu.c32
├── pxelinux.cfg/
│   ├── default             # Default menu
│   └── 01-aa-bb-cc-dd-ee-ff  # MAC-specific configs
├── images/
│   ├── flatcar/
│   │   ├── flatcar_production_pxe.vmlinuz
│   │   └── flatcar_production_pxe_image.cpio.gz
│   ├── kairos/
│   │   ├── kernel
│   │   └── initrd
│   └── opensuse/
│       ├── linux
│       └── initrd
└── ipxe/
    └── ipxe.efi            # iPXE chainload (optional)
```

### Download Boot Files

```bash
# Create directory
sudo mkdir -p /var/lib/tftpboot/{pxelinux.cfg,images,ipxe}
cd /var/lib/tftpboot

# Download syslinux/pxelinux (for BIOS)
wget https://mirrors.kernel.org/archlinux/iso/latest/arch/boot/syslinux/lpxelinux.0 -O pxelinux.0
wget https://mirrors.kernel.org/archlinux/iso/latest/arch/boot/syslinux/ldlinux.c32
wget https://mirrors.kernel.org/archlinux/iso/latest/arch/boot/syslinux/libcom32.c32
wget https://mirrors.kernel.org/archlinux/iso/latest/arch/boot/syslinux/libutil.c32
wget https://mirrors.kernel.org/archlinux/iso/latest/arch/boot/syslinux/menu.c32

# Download iPXE (for UEFI)
wget http://boot.ipxe.org/ipxe.efi -O bootx64.efi

# Set permissions
sudo chown -R tftp:tftp /var/lib/tftpboot
sudo chmod -R 755 /var/lib/tftpboot
```

### PXE Menu Example

```conf
# /var/lib/tftpboot/pxelinux.cfg/default

DEFAULT menu.c32
PROMPT 0
TIMEOUT 50
ONTIMEOUT local

MENU TITLE Beskar7 PXE Boot Menu

LABEL local
  MENU LABEL Boot from local disk
  LOCALBOOT 0

LABEL flatcar
  MENU LABEL Flatcar Container Linux
  KERNEL images/flatcar/flatcar_production_pxe.vmlinuz
  APPEND initrd=images/flatcar/flatcar_production_pxe_image.cpio.gz flatcar.first_boot=1

LABEL kairos
  MENU LABEL Kairos Cloud Native OS
  KERNEL images/kairos/kernel
  APPEND initrd=images/kairos/initrd config_url=http://10.0.0.3/configs/kairos.yaml
```

### Validation Commands

```bash
# Test TFTP access
tftp 10.0.0.2
> get pxelinux.0
> quit

# Check TFTP server status
sudo systemctl status tftpd-hpa

# Monitor TFTP requests
sudo tcpdump -i eth0 -n port 69

# Test from remote host
tftp -v 10.0.0.2 -c get pxelinux.0

# Check file permissions
ls -la /var/lib/tftpboot
```

---

## HTTP Server Requirements (iPXE)

Required for iPXE boot and recommended for faster boot times.

### Minimum Requirements

| Specification | Value |
|--------------|-------|
| **Software** | nginx, Apache httpd, or Caddy |
| **RAM** | 512 MB minimum, 2 GB recommended |
| **CPU** | 2 cores |
| **Storage** | 50 GB+ for OS images |
| **Network** | Connected to provisioning network |
| **Ports** | TCP 80 (HTTP), TCP 443 (HTTPS optional) |

### Nginx Installation

```bash
# Ubuntu/Debian
sudo apt-get install nginx

# RHEL/CentOS
sudo yum install nginx

# Enable and start
sudo systemctl enable nginx
sudo systemctl start nginx
```

### Directory Structure

```
/var/www/html/
├── ipxe/
│   ├── boot.ipxe           # Main iPXE script
│   ├── menus/
│   │   ├── main.ipxe
│   │   ├── flatcar.ipxe
│   │   └── kairos.ipxe
│   └── bootx64.efi         # iPXE UEFI loader
├── images/
│   ├── flatcar/
│   │   ├── flatcar_production_pxe.vmlinuz
│   │   ├── flatcar_production_pxe_image.cpio.gz
│   │   └── flatcar_production.iso
│   ├── kairos/
│   │   ├── kernel
│   │   ├── initrd
│   │   └── kairos-alpine-v2.8.1-amd64.iso
│   └── leap-micro/
│       ├── linux
│       ├── initrd
│       └── leap-micro.iso
├── configs/
│   ├── flatcar/
│   │   ├── ignition.json
│   │   └── control-plane.ign
│   ├── kairos/
│   │   ├── config.yaml
│   │   └── worker.yaml
│   └── leap-micro/
│       └── combustion.sh
└── scripts/
    ├── post-install.sh
    └── kubernetes-join.sh
```

### Nginx Configuration

```nginx
# /etc/nginx/sites-available/pxe-server

server {
    listen 80;
    server_name pxe-server.example.com;
    
    root /var/www/html;
    
    # Enable directory listing (useful for debugging)
    autoindex on;
    
    # Optimize for large file transfers
    sendfile on;
    tcp_nopush on;
    tcp_nodelay on;
    keepalive_timeout 65;
    types_hash_max_size 2048;
    client_max_body_size 4G;
    
    # iPXE scripts
    location /ipxe/ {
        default_type text/plain;
        add_header Content-Type text/plain;
        
        # CORS headers (if needed)
        add_header Access-Control-Allow-Origin *;
    }
    
    # OS images
    location /images/ {
        # Enable range requests for resumable downloads
        add_header Accept-Ranges bytes;
    }
    
    # Configuration files
    location /configs/ {
        # Restrict access (optional)
        # allow 10.0.0.0/16;
        # deny all;
    }
    
    # Logging
    access_log /var/log/nginx/pxe-access.log;
    error_log /var/log/nginx/pxe-error.log;
}
```

### iPXE Boot Script Example

```ipxe
#!ipxe
# /var/www/html/ipxe/boot.ipxe

# Set variables
set base-url http://${next-server}/images
set config-url http://${next-server}/configs

# Display network info
echo Network configuration:
echo IP: ${ip}
echo Gateway: ${gateway}
echo DNS: ${dns}
echo Next-server: ${next-server}

# Set timeout
set menu-timeout 30000
set submenu-timeout ${menu-timeout}

# Main menu
:start
menu iPXE Boot Menu for Beskar7
item --gap -- Operating Systems:
item --key f flatcar    Flatcar Container Linux
item --key k kairos     Kairos Cloud Native OS
item --key l leap       openSUSE Leap Micro
item --gap -- Advanced:
item --key s shell      iPXE Shell
item --key r reboot     Reboot
item --key x exit       Exit to BIOS
choose --timeout ${menu-timeout} --default flatcar target && goto ${target}

:flatcar
echo Booting Flatcar Container Linux...
set os flatcar
kernel ${base-url}/flatcar/flatcar_production_pxe.vmlinuz
initrd ${base-url}/flatcar/flatcar_production_pxe_image.cpio.gz
imgargs flatcar_production_pxe.vmlinuz flatcar.first_boot=1 flatcar.config.url=${config-url}/flatcar/ignition.json
boot || goto failed

:kairos
echo Booting Kairos...
set os kairos
kernel ${base-url}/kairos/kernel
initrd ${base-url}/kairos/initrd
imgargs kernel config_url=${config-url}/kairos/config.yaml
boot || goto failed

:leap
echo Booting openSUSE Leap Micro...
set os leap-micro
kernel ${base-url}/leap-micro/linux
initrd ${base-url}/leap-micro/initrd
imgargs linux combustion.path=${config-url}/leap-micro/combustion.sh
boot || goto failed

:shell
echo Type 'exit' to return to menu
shell
goto start

:reboot
reboot

:exit
exit

:failed
echo Boot failed, returning to menu...
sleep 3
goto start
```

### Validation Commands

```bash
# Test HTTP server
curl -I http://10.0.0.3/ipxe/boot.ipxe

# Download a test file
wget http://10.0.0.3/images/flatcar/flatcar_production_pxe.vmlinuz -O /tmp/test

# Check server status
sudo systemctl status nginx

# Monitor access logs
sudo tail -f /var/log/nginx/pxe-access.log

# Test iPXE script syntax
ipxe boot.ipxe  # If iPXE is installed locally

# Check disk usage
df -h /var/www/html

# Verify permissions
ls -la /var/www/html/ipxe/
```

---

## OS Image Hosting

### Storage Requirements

| OS | Image Size | Extracted Size | Recommended Space |
|----|-----------|----------------|-------------------|
| **Flatcar** | ~500 MB | ~1 GB | 2 GB |
| **Kairos** | ~1.5 GB | ~3 GB | 5 GB |
| **openSUSE Leap Micro** | ~800 MB | ~2 GB | 3 GB |

**Total Recommended**: 50-100 GB for multiple versions and OS families.

### Obtaining OS Images

#### Flatcar Container Linux

```bash
cd /var/www/html/images/flatcar

# Download stable channel
wget https://stable.release.flatcar-linux.net/amd64-usr/current/flatcar_production_pxe.vmlinuz
wget https://stable.release.flatcar-linux.net/amd64-usr/current/flatcar_production_pxe_image.cpio.gz
wget https://stable.release.flatcar-linux.net/amd64-usr/current/flatcar_production_iso_image.iso

# Verify checksums
wget https://stable.release.flatcar-linux.net/amd64-usr/current/flatcar_production_pxe.vmlinuz.sig
gpg --verify flatcar_production_pxe.vmlinuz.sig
```

#### Kairos

```bash
cd /var/www/html/images/kairos

# Download latest Kairos Alpine
KAIROS_VERSION="v2.8.1"
wget https://github.com/kairos-io/kairos/releases/download/${KAIROS_VERSION}/kairos-alpine-${KAIROS_VERSION}-amd64.iso

# Extract kernel and initrd from ISO
sudo mount -o loop kairos-alpine-${KAIROS_VERSION}-amd64.iso /mnt
sudo cp /mnt/boot/kernel ./kernel
sudo cp /mnt/boot/initrd ./initrd
sudo umount /mnt

# Verify
file kernel initrd
```

#### openSUSE Leap Micro

```bash
cd /var/www/html/images/leap-micro

# Download openSUSE Leap Micro
wget https://download.opensuse.org/distribution/leap-micro/5.5/appliances/openSUSE-Leap-Micro.x86_64-Default.iso

# Extract boot files
sudo mount -o loop openSUSE-Leap-Micro.x86_64-Default.iso /mnt
sudo cp /mnt/boot/x86_64/loader/linux ./linux
sudo cp /mnt/boot/x86_64/loader/initrd ./initrd
sudo umount /mnt
```

### Image Update Strategy

```bash
# Automated update script
#!/bin/bash
# /usr/local/bin/update-pxe-images.sh

IMAGE_DIR="/var/www/html/images"
LOG_FILE="/var/log/pxe-image-update.log"

update_flatcar() {
    echo "Updating Flatcar images..." | tee -a $LOG_FILE
    cd $IMAGE_DIR/flatcar
    wget -N https://stable.release.flatcar-linux.net/amd64-usr/current/flatcar_production_pxe.vmlinuz
    wget -N https://stable.release.flatcar-linux.net/amd64-usr/current/flatcar_production_pxe_image.cpio.gz
}

update_kairos() {
    echo "Updating Kairos images..." | tee -a $LOG_FILE
    # Add Kairos update logic
}

# Run updates
update_flatcar
update_kairos

echo "Image update completed at $(date)" | tee -a $LOG_FILE
```

### Set Up Cron Job

```cron
# /etc/cron.weekly/update-pxe-images
0 2 * * 0 /usr/local/bin/update-pxe-images.sh
```

---

## DNS Requirements

### Minimum Requirements

- **Internal DNS server** or hosts file entries
- **Forward lookups** for infrastructure servers
- **Reverse lookups** (optional but recommended)

### DNS Entries

```dns
; Zone file for cluster.local

; Infrastructure servers
pxe-server.cluster.local.       IN  A   10.0.0.2
ipxe-server.cluster.local.      IN  A   10.0.0.3
dhcp-server.cluster.local.      IN  A   10.0.0.1

; BMC addresses
bmc01.cluster.local.            IN  A   192.168.1.10
bmc02.cluster.local.            IN  A   192.168.1.11
bmc03.cluster.local.            IN  A   192.168.1.12

; Beskar7 controller
beskar7.cluster.local.          IN  A   10.0.0.5

; Kubernetes API endpoint
api.cluster.local.              IN  A   10.0.1.10
```

### Alternative: /etc/hosts

If not using DNS:

```
# /etc/hosts

# Infrastructure
10.0.0.1    dhcp-server dhcp-server.cluster.local
10.0.0.2    pxe-server pxe-server.cluster.local
10.0.0.3    ipxe-server ipxe-server.cluster.local

# BMCs
192.168.1.10    bmc01 bmc01.cluster.local
192.168.1.11    bmc02 bmc02.cluster.local
192.168.1.12    bmc03 bmc03.cluster.local
```

---

## Firewall Configuration

### Required Firewall Rules

#### On Provisioning Network

```bash
# Allow DHCP
iptables -A INPUT -p udp --dport 67:68 -j ACCEPT
iptables -A OUTPUT -p udp --dport 67:68 -j ACCEPT

# Allow TFTP
iptables -A INPUT -p udp --dport 69 -j ACCEPT

# Allow HTTP/HTTPS
iptables -A INPUT -p tcp --dport 80 -j ACCEPT
iptables -A INPUT -p tcp --dport 443 -j ACCEPT

# Save rules
iptables-save > /etc/iptables/rules.v4
```

#### UFW (Ubuntu)

```bash
sudo ufw allow 67/udp    # DHCP server
sudo ufw allow 68/udp    # DHCP client
sudo ufw allow 69/udp    # TFTP
sudo ufw allow 80/tcp    # HTTP
sudo ufw allow 443/tcp   # HTTPS
```

#### firewalld (RHEL/CentOS)

```bash
sudo firewall-cmd --permanent --add-service=dhcp
sudo firewall-cmd --permanent --add-service=tftp
sudo firewall-cmd --permanent --add-service=http
sudo firewall-cmd --permanent --add-service=https
sudo firewall-cmd --reload
```

### Port Summary

| Service | Port | Protocol | Direction | Required For |
|---------|------|----------|-----------|--------------|
| DHCP Server | 67 | UDP | Inbound | All |
| DHCP Client | 68 | UDP | Outbound | All |
| TFTP | 69 | UDP | Inbound | PXE |
| HTTP | 80 | TCP | Inbound | iPXE, images |
| HTTPS | 443 | TCP | Inbound | iPXE (optional) |
| Redfish | 443 | TCP | Outbound | Beskar7 |

---

## Hardware Requirements

### Bare Metal Servers

**BMC Requirements:**
- ✅ Redfish API support (v1.0.0+)
- ✅ Network boot capability (PXE/UEFI)
- ✅ Virtual media support (optional, for PreBakedISO mode)
- ✅ IPMI or Redfish power control
- ✅ Network connectivity to management network

**Server Requirements:**
- ✅ At least 2 network interfaces (BMC + data)
- ✅ UEFI firmware (recommended) or BIOS
- ✅ Network boot enabled in BIOS/UEFI
- ✅ Sufficient RAM for chosen OS (minimum 4 GB)
- ✅ Boot order configured (Network first during provisioning)

### Infrastructure Servers

**Minimum for Development:**
- 1 server for all services (DHCP, TFTP, HTTP)
- 4 GB RAM
- 2 CPU cores
- 50 GB storage
- 1 Gbps network

**Recommended for Production:**
- Separate servers for each service
- DHCP server: 2 GB RAM, 1 core
- TFTP server: 2 GB RAM, 1 core, 20 GB storage
- HTTP server: 8 GB RAM, 4 cores, 100 GB storage
- Load balancer for HTTP (optional)
- High availability pairs (recommended)

---

## Optional Components

### 1. Load Balancer (Recommended for Production)

**Purpose**: Distribute load across multiple HTTP servers

**Options**:
- HAProxy
- Nginx (in proxy mode)
- Hardware load balancer

**Configuration Example** (HAProxy):

```haproxy
# /etc/haproxy/haproxy.cfg

frontend http_front
    bind *:80
    default_backend http_back

backend http_back
    balance roundrobin
    server http1 10.0.0.3:80 check
    server http2 10.0.0.4:80 check
```

### 2. Caching Proxy (Optional)

**Purpose**: Reduce bandwidth and speed up downloads

**Options**:
- Squid
- Varnish
- nginx proxy_cache

### 3. Configuration Management (Recommended)

**Purpose**: Manage OS configurations

**Options**:
- Ansible for template generation
- Terraform for infrastructure
- GitOps workflow for configs

### 4. Monitoring (Recommended)

**Components to Monitor**:
- DHCP lease utilization
- TFTP request rate
- HTTP server bandwidth
- Disk space on image server
- Boot success/failure rates

**Tools**:
- Prometheus + Grafana
- ELK Stack
- Custom scripts

### 5. Logging (Recommended)

**Centralized Logging** for:
- DHCP requests and leases
- TFTP transfers
- HTTP downloads
- Boot successes/failures

**Tools**:
- rsyslog
- Elasticsearch + Kibana
- Loki

---

## Architecture Reference

### Minimal Development Setup

```
┌─────────────────────────────────────────────────┐
│ Single Server (All-in-One)                     │
│                                                  │
│  ┌──────────────────────────────────────────┐  │
│  │ Docker Containers:                       │  │
│  │  - netbootxyz (DHCP + TFTP + HTTP)      │  │
│  │  - Beskar7 Controller (K8s)             │  │
│  └──────────────────────────────────────────┘  │
│                                                  │
│  Network Interfaces:                            │
│   eth0: 10.0.0.1 (Data/PXE)                    │
│   eth1: 192.168.1.1 (Management/BMC)           │
└─────────────────────────────────────────────────┘
                    │
            ┌───────┴────────┐
            │                │
    ┌───────▼──────┐  ┌──────▼──────┐
    │ Server 1 BMC │  │ Server 2 BMC│
    │ 192.168.1.10 │  │ 192.168.1.11│
    │ Data: DHCP   │  │ Data: DHCP  │
    └──────────────┘  └─────────────┘
```

### Production Setup

```
┌──────────────────────────────────────────────────────┐
│ Management Network (192.168.1.0/24)                  │
│                                                       │
│  ┌─────────────┐    ┌─────────────┐                │
│  │ Beskar7 Ctrl│    │ BMC01...BMCn│                │
│  │ 192.168.1.5 │    │ .10, .11... │                │
│  └─────────────┘    └─────────────┘                │
└──────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────┐
│ Provisioning Network (10.0.0.0/16) - VLAN 20        │
│                                                       │
│  ┌─────────┐  ┌─────────┐  ┌──────────┐            │
│  │ DHCP HA │  │ TFTP    │  │ HTTP LB  │            │
│  │ Primary │  │ Primary │  │ HAProxy  │            │
│  │ .1      │  │ .2      │  │ .5       │            │
│  └─────────┘  └─────────┘  └────┬─────┘            │
│                                  │                   │
│  ┌─────────┐  ┌─────────┐  ┌────▼────┐  ┌────────┐│
│  │ DHCP HA │  │ TFTP    │  │ HTTP1   │  │ HTTP2  ││
│  │ Backup  │  │ Backup  │  │ .3      │  │ .4     ││
│  │ .11     │  │ .12     │  └─────────┘  └────────┘│
│  └─────────┘  └─────────┘                          │
└──────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────┐
│ Production Network (10.1.0.0/16) - VLAN 30          │
│                                                       │
│  ┌───────────────────────────────────────┐          │
│  │ Kubernetes Cluster                     │          │
│  │  - Control Planes: 10.1.1.10-12       │          │
│  │  - Workers: 10.1.2.x                  │          │
│  └───────────────────────────────────────┘          │
└──────────────────────────────────────────────────────┘
```

---

## Validation Checklist

Use this checklist before deploying Beskar7:

### Network Infrastructure
- [ ] Management network configured and routable
- [ ] Data/provisioning network configured
- [ ] VLANs configured (if used)
- [ ] Routing between networks working
- [ ] Firewall rules allow required ports
- [ ] DNS resolution working (or hosts files configured)

### DHCP Server
- [ ] DHCP server installed and running
- [ ] DHCP pool configured with sufficient addresses
- [ ] PXE boot options configured (next-server, filename)
- [ ] iPXE chainloading configured (if using iPXE)
- [ ] DHCP server accessible from provisioning network
- [ ] Test DHCP requests succeeding

### TFTP Server (for PXE)
- [ ] TFTP server installed and running
- [ ] Boot files present in `/var/lib/tftpboot`
- [ ] Permissions set correctly (755 directories, 644 files)
- [ ] Test TFTP file retrieval working
- [ ] Firewall allows UDP port 69

### HTTP Server (for iPXE)
- [ ] HTTP server installed and running
- [ ] iPXE scripts present and accessible
- [ ] OS images downloaded and accessible
- [ ] Configuration files created
- [ ] Test HTTP downloads working
- [ ] Firewall allows TCP port 80 (and 443)

### OS Images
- [ ] Flatcar images downloaded (if using)
- [ ] Kairos images downloaded (if using)
- [ ] openSUSE Leap Micro images downloaded (if using)
- [ ] Images extracted correctly (kernel, initrd)
- [ ] Checksums verified
- [ ] Images accessible via HTTP/TFTP
- [ ] Sufficient disk space available

### BMC/Hardware
- [ ] BMCs accessible via Redfish API
- [ ] BMC credentials configured
- [ ] Network boot enabled in BIOS/UEFI
- [ ] Boot order configured correctly
- [ ] BMCs connected to management network
- [ ] Server data NICs connected to provisioning network

### Beskar7 Controller
- [ ] Beskar7 controller deployed
- [ ] Controller can reach BMCs
- [ ] Controller has network access to provisioning network
- [ ] PhysicalHost resources created
- [ ] Credentials secrets created

### Testing
- [ ] Manual PXE boot test successful
- [ ] DHCP lease obtained
- [ ] Boot files downloaded via TFTP/HTTP
- [ ] OS boots successfully
- [ ] Network configuration correct post-boot

### Optional Components
- [ ] Load balancer configured (if used)
- [ ] Monitoring configured (if used)
- [ ] Logging configured (if used)
- [ ] Backup strategy defined
- [ ] Documentation updated for your environment

---

## Quick Validation Script

```bash
#!/bin/bash
# validate-pxe-infrastructure.sh

echo "=== PXE/iPXE Infrastructure Validation ==="
echo

# Network connectivity
echo "1. Testing network connectivity..."
ping -c 1 10.0.0.2 > /dev/null && echo "  ✓ PXE server reachable" || echo "  ✗ PXE server unreachable"
ping -c 1 10.0.0.3 > /dev/null && echo "  ✓ HTTP server reachable" || echo "  ✗ HTTP server unreachable"

# DHCP
echo "2. Testing DHCP service..."
systemctl is-active --quiet isc-dhcp-server && echo "  ✓ DHCP server running" || echo "  ✗ DHCP server not running"

# TFTP
echo "3. Testing TFTP service..."
systemctl is-active --quiet tftpd-hpa && echo "  ✓ TFTP server running" || echo "  ✗ TFTP server not running"
timeout 2 tftp -v 10.0.0.2 -c get pxelinux.0 /tmp/test 2>&1 | grep -q "Received" && echo "  ✓ TFTP file transfer works" || echo "  ✗ TFTP file transfer failed"

# HTTP
echo "4. Testing HTTP service..."
systemctl is-active --quiet nginx && echo "  ✓ HTTP server running" || echo "  ✗ HTTP server not running"
curl -s -o /dev/null -w "%{http_code}" http://10.0.0.3/ipxe/boot.ipxe | grep -q "200" && echo "  ✓ HTTP access works" || echo "  ✗ HTTP access failed"

# Files
echo "5. Checking required files..."
[ -f /var/lib/tftpboot/pxelinux.0 ] && echo "  ✓ pxelinux.0 exists" || echo "  ✗ pxelinux.0 missing"
[ -f /var/lib/tftpboot/bootx64.efi ] && echo "  ✓ bootx64.efi exists" || echo "  ✗ bootx64.efi missing"
[ -f /var/www/html/ipxe/boot.ipxe ] && echo "  ✓ boot.ipxe exists" || echo "  ✗ boot.ipxe missing"

# Disk space
echo "6. Checking disk space..."
df -h /var/www/html | grep -v Filesystem

echo
echo "=== Validation Complete ==="
```

---

## Troubleshooting Common Issues

### DHCP Issues

**Problem**: Clients not getting IP addresses

**Solutions**:
1. Check DHCP server is running: `systemctl status isc-dhcp-server`
2. Verify network interface: `ip addr show`
3. Check DHCP logs: `journalctl -u isc-dhcp-server -f`
4. Test broadcast domain: `tcpdump -i eth0 port 67 or port 68`
5. Verify DHCP pool not exhausted: `cat /var/lib/dhcp/dhcpd.leases`

### TFTP Issues

**Problem**: TFTP timeouts or file not found

**Solutions**:
1. Check TFTP server: `systemctl status tftpd-hpa`
2. Verify file exists: `ls -la /var/lib/tftpboot/pxelinux.0`
3. Check permissions: `ls -ld /var/lib/tftpboot`
4. Test locally: `tftp localhost` then `get pxelinux.0`
5. Check firewall: `sudo ufw status | grep 69`

### HTTP Issues

**Problem**: iPXE script not loading

**Solutions**:
1. Check nginx: `systemctl status nginx`
2. Test URL: `curl http://10.0.0.3/ipxe/boot.ipxe`
3. Check logs: `tail -f /var/log/nginx/error.log`
4. Verify file exists: `ls -la /var/www/html/ipxe/boot.ipxe`
5. Check syntax: Review iPXE script for errors

---

## Next Steps

After setting up infrastructure:

1. **Test manually** - Boot a server via PXE without Beskar7
2. **Deploy Beskar7** - Follow [installation guide](../docs/quick-start.md)
3. **Create PhysicalHosts** - Register your servers
4. **Test with Beskar7** - Use examples in this directory
5. **Monitor and optimize** - Track boot times and success rates
6. **Automate** - Create CI/CD pipelines for image updates

---

## Additional Resources

- **Beskar7 Documentation**: [../docs/README.md](../docs/README.md)
- **PXE Testing Guide**: [PXE_TESTING_GUIDE.md](PXE_TESTING_GUIDE.md)
- **Quick Start**: [PXE_QUICK_START.md](PXE_QUICK_START.md)
- **PXE Specification**: https://www.intel.com/content/www/us/en/download/19098/
- **iPXE Documentation**: https://ipxe.org/docs
- **Netboot.xyz**: https://netboot.xyz/
- **Redfish Specification**: https://www.dmtf.org/standards/redfish

---

**Document Version**: 1.0  
**Last Updated**: 2025-10-22  
**Maintained by**: Beskar7 Team

