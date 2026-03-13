import Tabs from "@theme/Tabs";
import TabItem from "@theme/TabItem";

# Proxmox VE

This guide covers deploying Nekzus on Proxmox Virtual Environment (PVE). Proxmox is a powerful open-source virtualization platform that supports both VMs and LXC containers, making it ideal for homelab and production deployments.

---

## Overview

Proxmox offers three deployment options for Nekzus:

| Method | Use Case | Resource Usage | Complexity |
|--------|----------|----------------|------------|
| **LXC Container** | Recommended for most users | Low (256 MB RAM) | Easy |
| **VM with Docker** | Full isolation, complex networking | Medium (512 MB+ RAM) | Medium |
| **Docker on Host** | Quick testing, single-node setups | Minimal | Easy |

---

## Prerequisites

Before proceeding, ensure you have:

- Proxmox VE 7.0 or newer installed
- Root or administrative access to the Proxmox host
- At least 512 MB free RAM and 2 GB disk space
- Network access to Docker Hub (or local registry)
- Basic familiarity with Proxmox web interface

---

## Option 1: LXC Container (Recommended)

LXC containers provide near-native performance with minimal overhead. This is the recommended deployment method for most users.

### Step 1: Download Container Template

1. Open the Proxmox web interface
2. Navigate to your storage (e.g., `local`)
3. Click **CT Templates**
4. Click **Templates** and download one of these:

    - **Debian 12** (recommended)
    - **Ubuntu 22.04 LTS**
    - **Alpine Linux** (smallest footprint)

<Tabs>
<TabItem value="via-web-interface" label="Via Web Interface">


1. Select your storage under Datacenter
2. Click **CT Templates** in the content panel
3. Click **Templates** button
4. Search for "debian-12" or "ubuntu-22.04"
5. Click **Download**

</TabItem>
<TabItem value="via-command-line" label="Via Command Line">


```bash
# SSH into Proxmox host
ssh root@proxmox-host

# Update template list
pveam update

# List available templates
pveam available | grep -E "debian|ubuntu"

# Download Debian 12 template
pveam download local debian-12-standard_12.2-1_amd64.tar.zst
```

</TabItem>
</Tabs>

### Step 2: Create the LXC Container

<Tabs>
<TabItem value="via-web-interface" label="Via Web Interface">


1. Click **Create CT** in the top-right corner
2. Configure the container:

**General Tab:**

- **CT ID**: Choose a unique ID (e.g., `110`)
- **Hostname**: `nekzus`
- **Password**: Set a root password
- **SSH public key**: (Optional) Add your SSH key

**Template Tab:**

- **Storage**: Select where you downloaded the template
- **Template**: Choose your downloaded template

**Disks Tab:**

- **Storage**: Select your preferred storage
- **Disk size**: 4 GB minimum (8 GB recommended)

**CPU Tab:**

- **Cores**: 1 (2 recommended for production)

**Memory Tab:**

- **Memory**: 512 MB (256 MB minimum)
- **Swap**: 256 MB

**Network Tab:**

- **Bridge**: `vmbr0` (or your network bridge)
- **IPv4**: DHCP or static IP
- **IPv6**: Optional

3. Click **Finish** to create the container

</TabItem>
<TabItem value="via-command-line" label="Via Command Line">


```bash
# Create container with recommended settings
pct create 110 local:vztmpl/debian-12-standard_12.2-1_amd64.tar.zst \
  --hostname nekzus \
  --memory 512 \
  --swap 256 \
  --cores 2 \
  --rootfs local-lvm:8 \
  --net0 name=eth0,bridge=vmbr0,ip=dhcp \
  --unprivileged 1 \
  --features nesting=1 \
  --onboot 1 \
  --password

# Start the container
pct start 110
```

</TabItem>
</Tabs>

### Step 3: Configure Container Features

For Docker to run inside the LXC container, enable nesting:

```bash
# On Proxmox host
pct set 110 --features nesting=1
```

:::warning[Privileged vs Unprivileged Containers]


**Unprivileged (Recommended):**

- More secure, runs with reduced privileges
- Requires `nesting=1` feature for Docker
- Some Docker images may have limitations

**Privileged:**

- Full access to host resources
- Easier Docker compatibility
- Less secure, use only if necessary

To create a privileged container, add `--unprivileged 0` to the creation command.

:::


### Step 4: Install Docker in LXC

Start the container and install Docker:

```bash
# Enter the container
pct enter 110

# Update packages
apt update && apt upgrade -y

# Install prerequisites
apt install -y ca-certificates curl gnupg

# Add Docker GPG key
install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/debian/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
chmod a+r /etc/apt/keyrings/docker.gpg

# Add Docker repository
echo \
  "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/debian \
  $(. /etc/os-release && echo "$VERSION_CODENAME") stable" | \
  tee /etc/apt/sources.list.d/docker.list > /dev/null

# Install Docker
apt update
apt install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin

# Verify installation
docker run hello-world
```

### Step 5: Deploy Nekzus

Create the deployment directory and configuration:

```bash
# Create project directory
mkdir -p /opt/nekzus
cd /opt/nekzus

# Create docker-compose.yml
cat > docker-compose.yml << 'EOF'
services:
  nekzus:
    image: nstalgic/nekzus:latest
    container_name: nekzus
    environment:
      NEKZUS_JWT_SECRET: "${NEKZUS_JWT_SECRET}"
      NEKZUS_BOOTSTRAP_TOKEN: "${NEKZUS_BOOTSTRAP_TOKEN}"
      NEKZUS_BASE_URL: "${NEKZUS_BASE_URL:-http://nekzus:8080}"
      NEKZUS_ADDR: ":8080"
    ports:
      - "8080:8080"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - nekzus-data:/app/data
    restart: unless-stopped
    healthcheck:
      test: ["/app/nekzus", "--health"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s

volumes:
  nekzus-data:
EOF

# Generate secure secrets
cat > .env << EOF
NEKZUS_JWT_SECRET=$(openssl rand -base64 32)
NEKZUS_BOOTSTRAP_TOKEN=$(openssl rand -base64 24)
NEKZUS_BASE_URL=http://$(hostname -I | awk '{print $1}'):8080
EOF

# Start Nekzus
docker compose up -d

# Verify it's running
docker compose logs -f
```

### Step 6: Configure Auto-Start

Enable the container to start on boot:

```bash
# On Proxmox host
pct set 110 --onboot 1
```

Docker will automatically start the Nekzus container when the LXC boots.

---

## Option 2: Virtual Machine with Docker

VMs provide full isolation and are ideal for complex networking scenarios or when running untrusted workloads.

### Step 1: Download ISO

Download a Linux ISO for your VM:

<Tabs>
<TabItem value="via-web-interface" label="Via Web Interface">


1. Navigate to your storage (e.g., `local`)
2. Click **ISO Images**
3. Click **Download from URL**
4. Use one of these URLs:
    - Debian 12: `https://cdimage.debian.org/debian-cd/current/amd64/iso-cd/debian-12.4.0-amd64-netinst.iso`
    - Ubuntu 22.04: `https://releases.ubuntu.com/22.04/ubuntu-22.04.3-live-server-amd64.iso`

</TabItem>
<TabItem value="via-command-line" label="Via Command Line">


```bash
# SSH into Proxmox host
cd /var/lib/vz/template/iso/

# Download Debian 12 netinst
wget https://cdimage.debian.org/debian-cd/current/amd64/iso-cd/debian-12.4.0-amd64-netinst.iso
```

</TabItem>
</Tabs>

### Step 2: Create the VM

<Tabs>
<TabItem value="via-web-interface" label="Via Web Interface">


1. Click **Create VM** in the top-right corner
2. Configure the VM:

**General Tab:**

- **VM ID**: Choose a unique ID (e.g., `200`)
- **Name**: `nekzus-vm`

**OS Tab:**

- **ISO image**: Select your downloaded ISO
- **Type**: Linux
- **Version**: 6.x - 2.6 Kernel (or latest)

**System Tab:**

- **BIOS**: Default (SeaBIOS)
- **Machine**: q35
- **Qemu Agent**: Enable (install `qemu-guest-agent` later)

**Disks Tab:**

- **Bus/Device**: VirtIO Block
- **Storage**: Select your storage
- **Disk size**: 16 GB minimum (32 GB recommended)
- **Discard**: Enable (for SSD/thin provisioning)

**CPU Tab:**

- **Sockets**: 1
- **Cores**: 2 (4 recommended for production)
- **Type**: host (best performance)

**Memory Tab:**

- **Memory**: 1024 MB (2048 MB recommended)

**Network Tab:**

- **Bridge**: `vmbr0`
- **Model**: VirtIO (paravirtualized)

3. Click **Finish** and start the VM

</TabItem>
<TabItem value="via-command-line" label="Via Command Line">


```bash
# Create VM
qm create 200 \
  --name nekzus-vm \
  --memory 2048 \
  --cores 2 \
  --sockets 1 \
  --cpu host \
  --net0 virtio,bridge=vmbr0 \
  --scsihw virtio-scsi-pci \
  --scsi0 local-lvm:32,discard=on \
  --ide2 local:iso/debian-12.4.0-amd64-netinst.iso,media=cdrom \
  --boot order=scsi0;ide2 \
  --agent enabled=1 \
  --onboot 1

# Start VM
qm start 200
```

</TabItem>
</Tabs>

### Step 3: Install Operating System

1. Open the VM console via Proxmox web interface
2. Complete the OS installation
3. Install required packages after first boot:

```bash
# Update system
apt update && apt upgrade -y

# Install QEMU guest agent for better Proxmox integration
apt install -y qemu-guest-agent
systemctl enable qemu-guest-agent
systemctl start qemu-guest-agent

# Install Docker (same as LXC step)
apt install -y ca-certificates curl gnupg

install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/debian/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
chmod a+r /etc/apt/keyrings/docker.gpg

echo \
  "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/debian \
  $(. /etc/os-release && echo "$VERSION_CODENAME") stable" | \
  tee /etc/apt/sources.list.d/docker.list > /dev/null

apt update
apt install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin
```

### Step 4: Deploy Nekzus

Follow the same deployment steps as the LXC container (Step 5 above).

---

## Option 3: Docker on Proxmox Host

For quick testing or single-node setups, you can run Docker directly on the Proxmox host.

:::warning[Production Considerations]


Running Docker on the Proxmox host is not recommended for production because:

- Container issues can affect the hypervisor
- Resource contention with VMs/LXCs
- Security risk if containers are compromised
- Makes Proxmox upgrades more complex

:::


### Step 1: Install Docker on Proxmox Host

```bash
# SSH into Proxmox host
ssh root@proxmox-host

# Install Docker
apt update
apt install -y docker.io docker-compose-plugin

# Enable Docker service
systemctl enable docker
systemctl start docker
```

### Step 2: Deploy Nekzus

```bash
# Create project directory
mkdir -p /opt/nekzus
cd /opt/nekzus

# Create docker-compose.yml and .env (same as LXC Step 5)
# Then start:
docker compose up -d
```

---

## Network Configuration

### Bridge Networking

Proxmox uses Linux bridges for networking. The default bridge `vmbr0` connects VMs/LXCs to your physical network.

**View current bridges:**

```bash
# On Proxmox host
cat /etc/network/interfaces
```

**Example bridge configuration:**

```text
auto vmbr0
iface vmbr0 inet static
    address 192.168.1.100/24
    gateway 192.168.1.1
    bridge-ports enp0s3
    bridge-stp off
    bridge-fd 0
```

### VLAN Configuration

To isolate Nekzus on a separate VLAN:

**1. Create a VLAN-aware bridge:**

Add to `/etc/network/interfaces`:

```text
auto vmbr0
iface vmbr0 inet manual
    bridge-ports enp0s3
    bridge-stp off
    bridge-fd 0
    bridge-vlan-aware yes
    bridge-vids 2-4094
```

**2. Assign VLAN to container/VM:**

<Tabs>
<TabItem value="lxc-container" label="LXC Container">


```bash
# Assign VLAN 100 to container
pct set 110 --net0 name=eth0,bridge=vmbr0,tag=100,ip=dhcp
```

</TabItem>
<TabItem value="virtual-machine" label="Virtual Machine">


```bash
# Assign VLAN 100 to VM
qm set 200 --net0 virtio,bridge=vmbr0,tag=100
```

</TabItem>
</Tabs>

### Static IP Configuration

For production deployments, use static IPs:

<Tabs>
<TabItem value="lxc-container" label="LXC Container">


```bash
pct set 110 --net0 name=eth0,bridge=vmbr0,ip=192.168.1.110/24,gw=192.168.1.1
```

</TabItem>
<TabItem value="inside-vm-container" label="Inside VM/Container">


Edit `/etc/network/interfaces`:

```text
auto eth0
iface eth0 inet static
    address 192.168.1.110/24
    gateway 192.168.1.1
    dns-nameservers 192.168.1.1 8.8.8.8
```

</TabItem>
</Tabs>

### Port Forwarding (NAT)

If your Proxmox host is behind NAT, forward ports to the container/VM:

```bash
# On Proxmox host - Add to /etc/network/interfaces under vmbr0
post-up iptables -t nat -A PREROUTING -i vmbr0 -p tcp --dport 8080 -j DNAT --to 192.168.1.110:8080
post-down iptables -t nat -D PREROUTING -i vmbr0 -p tcp --dport 8080 -j DNAT --to 192.168.1.110:8080
```

---

## Storage Configuration

### Bind Mounts (LXC)

Mount host directories into LXC containers for persistent storage:

```bash
# Create host directory
mkdir -p /mnt/nekzus-data

# Add bind mount to container
pct set 110 --mp0 /mnt/nekzus-data,mp=/app/data

# Restart container
pct restart 110
```

Update your `docker-compose.yml` to use the mounted path:

```yaml
volumes:
  - /app/data:/app/data
```

### Volume Configuration

**LXC with additional disk:**

```bash
# Add 10GB disk to container
pct set 110 --mp1 local-lvm:10,mp=/data
```

**VM with additional disk:**

```bash
# Add 50GB disk to VM
qm set 200 --scsi1 local-lvm:50
```

### Storage Recommendations

| Storage Type | Use Case | Performance |
|--------------|----------|-------------|
| **local-lvm** | VMs, high-performance LXCs | Best |
| **local-zfs** | Snapshots, data integrity | Good |
| **NFS/CIFS** | Shared storage, backups | Moderate |
| **local** | ISO images, templates | N/A |

---

## Docker Socket Access for Discovery

Nekzus uses the Docker socket for automatic service discovery. Configure access based on your deployment method.

### LXC Container

The Docker socket inside the LXC is at `/var/run/docker.sock`. The `docker-compose.yml` already mounts it:

```yaml
volumes:
  - /var/run/docker.sock:/var/run/docker.sock:ro
```

### VM

Same as LXC - mount the local Docker socket inside the VM.

### Host Docker with Proxmox LXC/VM Discovery

To discover containers running on the Proxmox host from within an LXC:

**Method 1: Pass-through Docker socket (Privileged LXC only)**

```bash
# On Proxmox host
# Add to /etc/pve/lxc/110.conf
lxc.mount.entry: /var/run/docker.sock var/run/docker.sock none bind,create=file 0 0

# Requires privileged container
pct set 110 --unprivileged 0
```

**Method 2: Docker TCP socket (Recommended for unprivileged)**

On Proxmox host, enable Docker TCP:

```bash
# Create Docker daemon override
mkdir -p /etc/systemd/system/docker.service.d/
cat > /etc/systemd/system/docker.service.d/override.conf << 'EOF'
[Service]
ExecStart=
ExecStart=/usr/bin/dockerd -H fd:// -H tcp://0.0.0.0:2375
EOF

# Reload and restart Docker
systemctl daemon-reload
systemctl restart docker
```

In your LXC container, configure Nekzus to use TCP:

```yaml
# config.yaml
discovery:
  docker:
    enabled: true
    socket_path: "tcp://192.168.1.100:2375"  # Proxmox host IP
    poll_interval: "30s"
```

:::danger[Security Warning]


Exposing Docker TCP without TLS is a security risk. For production:

- Use Docker TLS certificates
- Restrict access with firewall rules
- Consider using a Unix socket proxy

:::


---

## TLS Configuration

### Using Caddy (Recommended)

Deploy with Caddy for automatic TLS:

```yaml title="docker-compose.yml"
services:
  nekzus:
    image: nstalgic/nekzus:latest
    container_name: nekzus
    networks:
      - nekzus
    environment:
      NEKZUS_JWT_SECRET: "${NEKZUS_JWT_SECRET}"
      NEKZUS_BOOTSTRAP_TOKEN: "${NEKZUS_BOOTSTRAP_TOKEN}"
      NEKZUS_BASE_URL: "${NEKZUS_BASE_URL}"
      NEKZUS_ADDR: ":8080"
    command: ["--config", "/app/configs/config.yaml", "--insecure-http"]
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - nekzus-data:/app/data
    restart: unless-stopped

  caddy:
    image: caddy:2.8-alpine
    container_name: nekzus-caddy
    depends_on:
      - nekzus
    networks:
      - nekzus
    ports:
      - "8443:8443"
      - "80:80"
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy-data:/data
      - caddy-config:/config
    restart: unless-stopped

networks:
  nekzus:
    driver: bridge

volumes:
  nekzus-data:
  caddy-data:
  caddy-config:
```

```text title="Caddyfile"
:8443 {
    reverse_proxy nekzus:8080
    tls internal
}

:80 {
    redir https://{host}:8443{uri} permanent
}
```

### Using Proxmox Reverse Proxy

If you already have a reverse proxy on Proxmox, configure it to forward to Nekzus:

**Nginx example:**

```nginx
server {
    listen 443 ssl;
    server_name nekzus.yourdomain.com;

    ssl_certificate /etc/ssl/certs/nekzus.crt;
    ssl_certificate_key /etc/ssl/private/nekzus.key;

    location / {
        proxy_pass http://192.168.1.110:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

---

## Backup and Restore

### Proxmox Backup

**Backup LXC container:**

```bash
# Manual backup
vzdump 110 --storage local --compress zstd --mode snapshot

# Restore
pct restore 111 /var/lib/vz/dump/vzdump-lxc-110-*.tar.zst
```

**Backup VM:**

```bash
# Manual backup
vzdump 200 --storage local --compress zstd --mode snapshot

# Restore
qmrestore /var/lib/vz/dump/vzdump-qemu-200-*.vma.zst 201
```

### Automated Backups

Configure automated backups in Proxmox:

1. Navigate to **Datacenter** > **Backup**
2. Click **Add**
3. Configure schedule, storage, and compression
4. Select your Nekzus container/VM

### Nekzus Data Backup

In addition to Proxmox backups, Nekzus has built-in backup functionality:

```yaml title="config.yaml"
backup:
  enabled: true
  directory: "/app/data/backups"
  schedule: "24h"
  retention: 7
```

---

## Troubleshooting

### LXC Container Issues

<details>
<summary>Docker fails to start in LXC</summary>


**Symptom:** `Cannot connect to the Docker daemon`

**Solutions:**

1. Enable nesting feature:
    ```bash
    pct set 110 --features nesting=1
    pct restart 110
    ```

2. Check container is not running in unprivileged mode with incompatible settings:
    ```bash
    pct config 110 | grep -E "unprivileged|features"
    ```

3. For persistent issues, try privileged mode:
    ```bash
    pct set 110 --unprivileged 0
    pct restart 110
    ```

</details>


<details>
<summary>Permission denied errors</summary>


**Symptom:** `permission denied while trying to connect to Docker daemon socket`

**Solutions:**

1. Check socket permissions:
    ```bash
    ls -la /var/run/docker.sock
    ```

2. Add user to docker group:
    ```bash
    usermod -aG docker $USER
    ```

3. Restart Docker service:
    ```bash
    systemctl restart docker
    ```

</details>


<details>
<summary>Network connectivity issues in LXC</summary>


**Symptom:** Container cannot reach internet or other hosts

**Solutions:**

1. Check network configuration:
    ```bash
    pct config 110 | grep net
    ip addr show eth0
    ```

2. Verify DNS resolution:
    ```bash
    cat /etc/resolv.conf
    ping -c 3 8.8.8.8
    ```

3. Check bridge configuration on Proxmox host:
    ```bash
    brctl show
    ```

</details>


### VM Issues

<details>
<summary>VM network not working</summary>


**Symptom:** No network connectivity after OS installation

**Solutions:**

1. Ensure VirtIO drivers are installed:
    ```bash
    lsmod | grep virtio
    ```

2. Check network interface:
    ```bash
    ip link show
    dhclient -v eth0
    ```

3. Verify bridge configuration in Proxmox

</details>


<details>
<summary>Slow VM performance</summary>


**Symptom:** High latency, slow disk I/O

**Solutions:**

1. Use VirtIO drivers for disk and network
2. Enable CPU type `host`:
    ```bash
    qm set 200 --cpu host
    ```
3. Enable discard for SSD:
    ```bash
    qm set 200 --scsi0 local-lvm:32,discard=on
    ```
4. Install QEMU guest agent

</details>


### Docker Discovery Issues

<details>
<summary>Services not being discovered</summary>


**Symptom:** Docker containers not appearing in Nekzus

**Solutions:**

1. Verify Docker socket is mounted:
    ```bash
    docker compose exec nekzus ls -la /var/run/docker.sock
    ```

2. Check discovery is enabled in config:
    ```yaml
    discovery:
      docker:
        enabled: true
        socket_path: "unix:///var/run/docker.sock"
    ```

3. Ensure containers have Nekzus labels:
    ```yaml
    labels:
      nekzus.enable: "true"
      nekzus.app.name: "My Service"
    ```

4. Check Nekzus logs:
    ```bash
    docker compose logs nekzus | grep -i discovery
    ```

</details>


<details>
<summary>Docker socket access denied in unprivileged LXC</summary>


**Symptom:** Cannot access Docker socket from host

**Solutions:**

1. Use Docker TCP socket instead (see Docker Socket Access section)
2. Convert to privileged container if security permits
3. Use a socket proxy like `docker-socket-proxy`

</details>


### Resource Issues

<details>
<summary>Container/VM running out of memory</summary>


**Symptom:** OOM killer terminating processes

**Solutions:**

1. Increase memory allocation:
    ```bash
    # LXC
    pct set 110 --memory 1024

    # VM
    qm set 200 --memory 2048
    ```

2. Add swap:
    ```bash
    pct set 110 --swap 512
    ```

3. Check Docker resource limits in compose file

</details>


<details>
<summary>High CPU usage</summary>


**Symptom:** Container/VM using excessive CPU

**Solutions:**

1. Check running processes:
    ```bash
    docker stats
    top -c
    ```

2. Limit CPU in Proxmox:
    ```bash
    # LXC
    pct set 110 --cpulimit 2

    # VM
    qm set 200 --cpulimit 2
    ```

3. Check Nekzus health checks aren't too frequent

</details>


---

## Performance Optimization

### LXC Optimization

```bash
# Increase file descriptor limit
pct set 110 --mp0 /mnt/nekzus-data,mp=/app/data
pct set 110 --features nesting=1

# Add to /etc/pve/lxc/110.conf for better I/O
lxc.cgroup2.memory.max: 1G
lxc.cgroup2.cpu.max: 200000 100000
```

### VM Optimization

```bash
# Use VirtIO for best performance
qm set 200 --cpu host
qm set 200 --scsihw virtio-scsi-single
qm set 200 --net0 virtio,bridge=vmbr0

# Enable ballooning for memory efficiency
qm set 200 --balloon 512
```

### Docker Optimization

```yaml title="docker-compose.yml"
services:
  nekzus:
    # ... other config ...
    deploy:
      resources:
        limits:
          cpus: '2.0'
          memory: 1G
        reservations:
          cpus: '0.5'
          memory: 256M
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"
```

---

## Security Best Practices

1. **Use unprivileged LXC containers** when possible
2. **Enable Proxmox firewall** and restrict access to necessary ports
3. **Use strong secrets** for JWT and bootstrap tokens
4. **Keep systems updated**: Proxmox, container OS, and Docker
5. **Enable TLS** for all external communication
6. **Limit Docker socket access** - use read-only mounts
7. **Configure resource limits** to prevent DoS
8. **Use VLANs** to isolate Nekzus from sensitive networks
9. **Enable Proxmox two-factor authentication**
10. **Regular backups** with tested restore procedures

---

## Next Steps

- [Quick Start Guide](../getting-started/quick-start) - Configure Nekzus
- [Configuration Reference](../reference/configuration) - Detailed settings
- [Docker Compose Guide](../guides/docker-compose) - Advanced deployments
- [Troubleshooting Guide](../guides/troubleshooting) - Common issues
