import Tabs from "@theme/Tabs";
import TabItem from "@theme/TabItem";

# Raspberry Pi

This guide covers deploying Nekzus on Raspberry Pi devices. The Raspberry Pi makes an excellent low-power home server for running Nekzus as your local network gateway.

---

## Requirements

### Hardware

| Component | Minimum | Recommended |
|-----------|---------|-------------|
| Model | Raspberry Pi 3B+ (arm64) | Raspberry Pi 4 (4GB+) or Pi 5 |
| RAM | 1 GB | 4 GB+ |
| Storage | 8 GB SD card | 32 GB+ SSD via USB 3.0 |
| Network | Ethernet or Wi-Fi | Gigabit Ethernet |

:::info[Architecture Note]

Nekzus requires **64-bit ARM (arm64/aarch64)**. Raspberry Pi 3B+ and later support arm64 with the appropriate OS. Older 32-bit (armv7) Raspberry Pi models are not supported.

:::


### Operating System

Supported 64-bit operating systems:

| OS | Version | Recommended |
|----|---------|-------------|
| Raspberry Pi OS | 64-bit (Bookworm) | Yes |
| Ubuntu Server | 22.04 LTS or 24.04 LTS | Yes |
| Debian | 12 (Bookworm) | Yes |
| DietPi | 64-bit | Yes |

### Software Dependencies

<Tabs>
<TabItem value="docker-recommended-" label="Docker (Recommended)">


- Docker 24.0+ (arm64 build)
- Docker Compose v2.20+
- Network access to Docker Hub

</TabItem>
<TabItem value="building-from-source" label="Building from Source">


- Go 1.25+ (arm64)
- Node.js 20+ and npm
- GCC and libc6-dev
- Make
- Git

</TabItem>
</Tabs>

---

## Pre-Installation Setup

### Flash the OS

1. **Download Raspberry Pi Imager** from [raspberrypi.com/software](https://www.raspberrypi.com/software/)

2. **Select the OS:**
    - Choose "Raspberry Pi OS (64-bit)" or "Ubuntu Server 24.04 LTS (64-bit)"

3. **Configure settings** (click the gear icon):
    - Set hostname (e.g., `nekzus`)
    - Enable SSH
    - Set username and password
    - Configure Wi-Fi (if not using Ethernet)
    - Set locale and timezone

4. **Flash to SD card or SSD**

### Initial Boot

After booting, connect via SSH:

```bash
ssh pi@nekzus.local
# Or use the IP address
ssh pi@192.168.1.100
```

Update the system:

```bash
sudo apt update && sudo apt upgrade -y
sudo reboot
```

---

## Installation Methods

### Docker (Recommended)

Docker is the recommended installation method for Raspberry Pi deployments.

#### Install Docker

```bash
# Install Docker using the convenience script
curl -fsSL https://get.docker.com | sh

# Add your user to the docker group
sudo usermod -aG docker $USER

# Apply group changes (or log out and back in)
newgrp docker

# Verify installation
docker --version
docker compose version
```

#### Deploy Nekzus

Create a project directory:

```bash
mkdir -p ~/nekzus && cd ~/nekzus
```

Create `docker-compose.yml`:

```yaml title="docker-compose.yml"
services:
  nekzus:
    image: nstalgic/nekzus:latest
    container_name: nekzus
    environment:
      NEKZUS_JWT_SECRET: "${NEKZUS_JWT_SECRET}"
      NEKZUS_BOOTSTRAP_TOKEN: "${NEKZUS_BOOTSTRAP_TOKEN}"
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
      timeout: 10s
      retries: 3
      start_period: 30s
    deploy:
      resources:
        limits:
          memory: 512M

volumes:
  nekzus-data:
```

Create the `.env` file with secure secrets:

```bash
echo "NEKZUS_JWT_SECRET=$(openssl rand -base64 32)" > .env
echo "NEKZUS_BOOTSTRAP_TOKEN=$(openssl rand -base64 24)" >> .env
chmod 600 .env
```

Start the service:

```bash
docker compose up -d
```

Verify the deployment:

```bash
# Check container status
docker compose ps

# View logs
docker compose logs -f nekzus

# Test health endpoint
curl http://localhost:8080/healthz
```

---

### Building from Source

Building from source is useful for development or when you need custom builds.

#### Install Dependencies

```bash
# Update package list
sudo apt update

# Install build tools
sudo apt install -y gcc libc6-dev make git wget

# Install Go 1.25
wget https://go.dev/dl/go1.25.linux-arm64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.25.linux-arm64.tar.gz
rm go1.25.linux-arm64.tar.gz

# Add Go to PATH
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
echo 'export PATH=$PATH:$HOME/go/bin' >> ~/.bashrc
source ~/.bashrc

# Verify Go installation
go version

# Install Node.js 20 (for web UI build)
curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
sudo apt install -y nodejs

# Verify Node.js
node --version
npm --version
```

#### Build Nekzus

```bash
# Clone the repository
git clone https://github.com/nstalgic/nekzus.git
cd nekzus

# Build web UI and Go binary
make build-all

# Verify the build
./bin/nekzus --version
```

:::note[Build Time]

Building on a Raspberry Pi 4 takes approximately 3-5 minutes. On a Raspberry Pi 3B+, expect 8-12 minutes.

:::


#### Run Nekzus

```bash
# Generate secrets
export NEKZUS_JWT_SECRET=$(openssl rand -base64 32)
export NEKZUS_BOOTSTRAP_TOKEN=$(openssl rand -base64 24)

# Run without TLS (development)
./bin/nekzus --insecure-http

# Run with custom config
./bin/nekzus --config configs/config.yaml --insecure-http
```

---

## Performance Optimization

### Memory Management

Raspberry Pi devices have limited RAM. Apply these optimizations:

#### Docker Memory Limits

Set memory limits in your `docker-compose.yml`:

```yaml
deploy:
  resources:
    limits:
      memory: 512M
    reservations:
      memory: 256M
```

| Model | Recommended Limit | Notes |
|-------|-------------------|-------|
| Pi 3B+ (1GB) | 384M | Leaves room for OS |
| Pi 4 (2GB) | 512M | Comfortable margin |
| Pi 4 (4GB+) | 1G | Room for toolbox services |
| Pi 5 (8GB) | 2G | Full feature support |

#### Reduce Container Overhead

Disable features you do not need:

```yaml
environment:
  # Disable Kubernetes discovery if not using K8s
  NEKZUS_DISCOVERY_K8S_ENABLED: "false"
  # Reduce log verbosity
  NEKZUS_LOG_LEVEL: "warn"
```

### Storage Configuration

SD cards are slow and wear out quickly. SSD storage dramatically improves performance and reliability.

#### USB SSD Boot (Recommended)

1. **Prepare the SSD:**

    ```bash
    # List connected drives
    lsblk

    # Format the SSD (replace sdX with your device)
    sudo mkfs.ext4 /dev/sdX1
    ```

2. **Enable USB boot** (Pi 4/5 only):

    ```bash
    # Update bootloader for USB boot support
    sudo rpi-eeprom-update -d -a
    sudo reboot
    ```

3. **Flash OS to SSD** using Raspberry Pi Imager

4. **Boot from SSD** by removing the SD card

#### Docker Data on SSD

If using SD card for OS but SSD for data:

```bash
# Mount SSD
sudo mkdir -p /mnt/ssd
sudo mount /dev/sda1 /mnt/ssd

# Add to fstab for persistence
echo '/dev/sda1 /mnt/ssd ext4 defaults,noatime 0 2' | sudo tee -a /etc/fstab

# Configure Docker to use SSD
sudo mkdir -p /etc/docker
cat <<EOF | sudo tee /etc/docker/daemon.json
{
  "data-root": "/mnt/ssd/docker"
}
EOF

# Restart Docker
sudo systemctl restart docker
```

### Swap Configuration

Configure swap for memory-constrained devices:

#### Increase Swap Size

```bash
# Disable current swap
sudo dphys-swapfile swapoff

# Edit swap configuration
sudo nano /etc/dphys-swapfile
```

Set these values:

```ini title="/etc/dphys-swapfile"
CONF_SWAPSIZE=2048
CONF_SWAPFACTOR=2
CONF_MAXSWAP=4096
```

Apply changes:

```bash
sudo dphys-swapfile setup
sudo dphys-swapfile swapon

# Verify
free -h
```

#### Swap on SSD (Recommended)

Place swap on SSD for better performance:

```bash
# Create swap file on SSD
sudo fallocate -l 2G /mnt/ssd/swapfile
sudo chmod 600 /mnt/ssd/swapfile
sudo mkswap /mnt/ssd/swapfile
sudo swapon /mnt/ssd/swapfile

# Make permanent
echo '/mnt/ssd/swapfile none swap sw 0 0' | sudo tee -a /etc/fstab
```

### CPU Governor

Set the CPU governor for consistent performance:

```bash
# Check current governor
cat /sys/devices/system/cpu/cpu0/cpufreq/scaling_governor

# Set to performance mode
echo 'performance' | sudo tee /sys/devices/system/cpu/cpu*/cpufreq/scaling_governor

# Make permanent
echo 'GOVERNOR="performance"' | sudo tee /etc/default/cpufrequtils
sudo systemctl restart cpufrequtils
```

---

## Network Setup

### Static IP Configuration

A static IP ensures consistent access to Nekzus.

<Tabs>
<TabItem value="dhcpcd-raspberry-pi-os-" label="dhcpcd (Raspberry Pi OS)">


Edit `/etc/dhcpcd.conf`:

```bash
sudo nano /etc/dhcpcd.conf
```

Add at the end:

```ini title="/etc/dhcpcd.conf"
interface eth0
static ip_address=192.168.1.100/24
static routers=192.168.1.1
static domain_name_servers=192.168.1.1 8.8.8.8
```

Apply changes:

```bash
sudo systemctl restart dhcpcd
```

</TabItem>
<TabItem value="netplan-ubuntu-server-" label="Netplan (Ubuntu Server)">


Create `/etc/netplan/01-static.yaml`:

```yaml title="/etc/netplan/01-static.yaml"
network:
  version: 2
  ethernets:
    eth0:
      dhcp4: false
      addresses:
        - 192.168.1.100/24
      routes:
        - to: default
          via: 192.168.1.1
      nameservers:
        addresses:
          - 192.168.1.1
          - 8.8.8.8
```

Apply changes:

```bash
sudo netplan apply
```

</TabItem>
</Tabs>

### mDNS/Avahi for Discovery

Enable mDNS for local network discovery:

```bash
# Install Avahi
sudo apt install -y avahi-daemon avahi-utils

# Enable and start the service
sudo systemctl enable avahi-daemon
sudo systemctl start avahi-daemon

# Verify mDNS is working
avahi-browse -alr
```

Your Raspberry Pi will be accessible at `nekzus.local` (using your configured hostname).

#### Advertise Nekzus via mDNS

Create a service file for Nekzus discovery:

```bash
sudo nano /etc/avahi/services/nekzus.service
```

```xml title="/etc/avahi/services/nekzus.service"
<?xml version="1.0" standalone='no'?>
<!DOCTYPE service-group SYSTEM "avahi-service.dtd">
<service-group>
  <name replace-wildcards="yes">Nekzus on %h</name>
  <service>
    <type>_http._tcp</type>
    <port>8080</port>
    <txt-record>path=/</txt-record>
    <txt-record>type=nekzus</txt-record>
  </service>
</service-group>
```

Reload Avahi:

```bash
sudo systemctl reload avahi-daemon
```

### Firewall Configuration

Configure UFW (Uncomplicated Firewall) for security:

```bash
# Install UFW
sudo apt install -y ufw

# Set default policies
sudo ufw default deny incoming
sudo ufw default allow outgoing

# Allow SSH (important - do this first!)
sudo ufw allow ssh

# Allow Nekzus web interface
sudo ufw allow 8080/tcp comment 'Nekzus HTTP'

# Allow HTTPS if using TLS
sudo ufw allow 8443/tcp comment 'Nekzus HTTPS'

# Allow mDNS for discovery
sudo ufw allow 5353/udp comment 'mDNS'

# Enable firewall
sudo ufw enable

# Check status
sudo ufw status verbose
```

---

## Running as a Service

### systemd Service Setup

Create a systemd service for automatic startup and management.

#### For Docker Deployments

Docker Compose with `restart: unless-stopped` handles this automatically. For additional control:

```bash
sudo nano /etc/systemd/system/nekzus.service
```

```ini title="/etc/systemd/system/nekzus.service"
[Unit]
Description=Nekzus API Gateway
Requires=docker.service
After=docker.service network-online.target
Wants=network-online.target

[Service]
Type=oneshot
RemainAfterExit=yes
WorkingDirectory=/home/pi/nekzus
ExecStart=/usr/bin/docker compose up -d
ExecStop=/usr/bin/docker compose down
ExecReload=/usr/bin/docker compose restart
TimeoutStartSec=120
TimeoutStopSec=60

[Install]
WantedBy=multi-user.target
```

#### For Binary Deployments

```bash
sudo nano /etc/systemd/system/nekzus.service
```

```ini title="/etc/systemd/system/nekzus.service"
[Unit]
Description=Nekzus API Gateway
Documentation=https://github.com/nstalgic/nekzus
After=network-online.target docker.socket
Wants=network-online.target

[Service]
Type=simple
User=pi
Group=pi
WorkingDirectory=/home/pi/nekzus

# Environment variables
Environment="NEKZUS_ADDR=:8080"
EnvironmentFile=/home/pi/nekzus/.env

# Run command
ExecStart=/home/pi/nekzus/bin/nekzus --insecure-http
ExecReload=/bin/kill -HUP $MAINPID

# Restart policy
Restart=always
RestartSec=10
StartLimitInterval=60
StartLimitBurst=3

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=read-only
ReadWritePaths=/home/pi/nekzus/data
PrivateTmp=true

# Resource limits
MemoryMax=512M
TasksMax=100

[Install]
WantedBy=multi-user.target
```

#### Enable and Start the Service

```bash
# Reload systemd
sudo systemctl daemon-reload

# Enable service to start on boot
sudo systemctl enable nekzus

# Start the service
sudo systemctl start nekzus

# Check status
sudo systemctl status nekzus
```

#### Service Management Commands

```bash
# Start service
sudo systemctl start nekzus

# Stop service
sudo systemctl stop nekzus

# Restart service
sudo systemctl restart nekzus

# View logs
sudo journalctl -u nekzus -f

# View logs since boot
sudo journalctl -u nekzus -b
```

---

## Monitoring

### Resource Usage

Monitor system resources on your Raspberry Pi:

#### htop (Interactive)

```bash
sudo apt install -y htop
htop
```

#### Command-Line Monitoring

```bash
# CPU and memory usage
vmstat 1 5

# Docker container stats
docker stats --no-stream

# Disk usage
df -h

# Memory details
free -h

# Temperature (important for Pi!)
vcgencmd measure_temp
```

#### Prometheus Metrics

Nekzus exposes Prometheus metrics at `/metrics`. Set up monitoring:

```yaml title="docker-compose.yml (with monitoring)"
services:
  nekzus:
    # ... existing configuration ...

  prometheus:
    image: prom/prometheus:latest
    container_name: prometheus
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml:ro
      - prometheus-data:/prometheus
    ports:
      - "9090:9090"
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'
      - '--storage.tsdb.retention.time=7d'
    restart: unless-stopped
    deploy:
      resources:
        limits:
          memory: 256M

volumes:
  nekzus-data:
  prometheus-data:
```

```yaml title="prometheus.yml"
global:
  scrape_interval: 30s
  evaluation_interval: 30s

scrape_configs:
  - job_name: 'nekzus'
    static_configs:
      - targets: ['nekzus:8080']
    metrics_path: /metrics
```

### Log Management

#### View Logs

```bash
# Docker logs
docker compose logs -f nekzus

# With timestamps
docker compose logs -f --timestamps nekzus

# Last 100 lines
docker compose logs --tail 100 nekzus

# systemd logs
sudo journalctl -u nekzus -f
```

#### Log Rotation

Docker handles log rotation automatically. Configure limits:

```yaml title="docker-compose.yml"
services:
  nekzus:
    # ... existing configuration ...
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"
```

### Temperature Monitoring

Raspberry Pi can throttle under high temperatures. Monitor and alert:

```bash
# Check temperature
vcgencmd measure_temp

# Check for throttling
vcgencmd get_throttled
```

Throttling codes:

| Bit | Meaning |
|-----|---------|
| 0 | Under-voltage detected |
| 1 | Arm frequency capped |
| 2 | Currently throttled |
| 3 | Soft temperature limit active |
| 16 | Under-voltage has occurred |
| 17 | Arm frequency capped has occurred |
| 18 | Throttling has occurred |
| 19 | Soft temperature limit has occurred |

:::warning[Cooling Recommendation]

Use a heatsink and fan for sustained workloads. Active cooling prevents thermal throttling and extends SD card lifespan.

:::


---

## Troubleshooting

### ARM-Specific Issues

<details>
<summary>Container fails to start with exec format error</summary>


This error indicates an architecture mismatch. Ensure you are using arm64 images:

```bash
# Check system architecture
uname -m
# Should output: aarch64

# Verify Docker platform
docker info | grep Architecture
# Should output: aarch64

# Pull arm64 image explicitly
docker pull --platform linux/arm64 nstalgic/nekzus:latest
```

If running a 32-bit OS, upgrade to 64-bit Raspberry Pi OS or Ubuntu Server.

</details>


<details>
<summary>Build fails with CGO errors</summary>


SQLite requires CGO. Install the necessary dependencies:

```bash
sudo apt install -y gcc libc6-dev libsqlite3-dev

# Verify GCC is working
gcc --version

# Build with CGO enabled
CGO_ENABLED=1 go build -o bin/nekzus ./cmd/nekzus
```

</details>


<details>
<summary>Out of memory during build</summary>


Increase swap space temporarily:

```bash
# Create temporary swap
sudo fallocate -l 2G /swapfile
sudo chmod 600 /swapfile
sudo mkswap /swapfile
sudo swapon /swapfile

# Build
make build-all

# Remove temporary swap
sudo swapoff /swapfile
sudo rm /swapfile
```

Alternatively, cross-compile on a more powerful machine.

</details>


### Performance Issues

<details>
<summary>Slow response times</summary>


1. **Check if throttling:**
    ```bash
    vcgencmd get_throttled
    ```

2. **Check CPU/memory usage:**
    ```bash
    docker stats --no-stream
    ```

3. **Switch to SSD storage** - SD cards have high latency

4. **Reduce memory limits** if swapping excessively:
    ```bash
    free -h
    ```

</details>


<details>
<summary>High CPU usage at idle</summary>


1. **Check discovery poll intervals** - increase them:
    ```yaml
    discovery:
      docker:
        poll_interval: "60s"  # Increase from default
      mdns:
        scan_interval: "120s"
    ```

2. **Disable unused discovery providers:**
    ```yaml
    discovery:
      kubernetes:
        enabled: false
    ```

</details>


<details>
<summary>Container keeps restarting</summary>


Check logs for the cause:

```bash
docker compose logs nekzus --tail 50

# Check container exit code
docker inspect nekzus --format='{{.State.ExitCode}}'
```

Common causes:

- Out of memory (exit code 137)
- Configuration errors (exit code 1)
- Missing environment variables

</details>


### Network Issues

<details>
<summary>Cannot access from other devices</summary>


1. **Check firewall:**
    ```bash
    sudo ufw status
    sudo ufw allow 8080/tcp
    ```

2. **Verify the service is listening:**
    ```bash
    ss -tlnp | grep 8080
    ```

3. **Test locally first:**
    ```bash
    curl http://localhost:8080/healthz
    ```

4. **Check Pi IP address:**
    ```bash
    hostname -I
    ```

</details>


<details>
<summary>mDNS discovery not working</summary>


1. **Verify Avahi is running:**
    ```bash
    sudo systemctl status avahi-daemon
    ```

2. **Check mDNS port is open:**
    ```bash
    sudo ufw allow 5353/udp
    ```

3. **Test mDNS resolution:**
    ```bash
    avahi-resolve -n nekzus.local
    ```

</details>


<details>
<summary>Docker socket permission denied</summary>


```bash
# Add user to docker group
sudo usermod -aG docker $USER

# Log out and back in, or run:
newgrp docker

# Verify
docker ps
```

</details>


### Storage Issues

<details>
<summary>SD card is full</summary>


1. **Check disk usage:**
    ```bash
    df -h
    ```

2. **Clean Docker resources:**
    ```bash
    docker system prune -a --volumes
    ```

3. **Remove old logs:**
    ```bash
    sudo journalctl --vacuum-time=7d
    ```

</details>


<details>
<summary>Slow SD card performance</summary>


Check SD card speed class. Use at least A1/Class 10 cards, or preferably switch to USB SSD.

```bash
# Test write speed
dd if=/dev/zero of=/tmp/test bs=1M count=256 conv=fsync

# Test read speed
dd if=/tmp/test of=/dev/null bs=1M

# Clean up
rm /tmp/test
```

Expected speeds:

- Class 10 SD: 10-20 MB/s
- A1 SD: 30-50 MB/s
- USB 3.0 SSD: 200-400 MB/s

</details>


---

## Maintenance

### Updates

#### Docker Updates

```bash
cd ~/nekzus

# Pull latest image
docker compose pull

# Restart with new image
docker compose up -d

# Remove old images
docker image prune -f
```

#### Binary Updates

```bash
cd ~/nekzus

# Stop service
sudo systemctl stop nekzus

# Pull latest code
git pull origin main

# Rebuild
make build-all

# Start service
sudo systemctl start nekzus
```

### Backup

#### Backup Data Volume

```bash
# Stop the service
docker compose stop nekzus

# Backup volume
docker run --rm -v nekzus-data:/data -v $(pwd):/backup alpine \
  tar czf /backup/nekzus-backup-$(date +%Y%m%d).tar.gz -C /data .

# Restart
docker compose start nekzus
```

#### Restore from Backup

```bash
# Stop the service
docker compose stop nekzus

# Restore volume
docker run --rm -v nekzus-data:/data -v $(pwd):/backup alpine \
  tar xzf /backup/nekzus-backup-20240101.tar.gz -C /data

# Restart
docker compose start nekzus
```

---

## Next Steps

- [Quick Start Guide](../getting-started/quick-start) - Configure Nekzus
- [Docker Compose Guide](../guides/docker-compose) - Advanced Docker setups
- [Discovery Configuration](../features/discovery) - Set up service discovery
- [Troubleshooting Guide](../guides/troubleshooting) - General troubleshooting
