# Nekzus

A secure API gateway and discovery service for local network applications.

## Features

- **Modern Web Dashboard** - Built-in React UI with multiple themes
- **Auto-Discovery** - Find services via Docker, Kubernetes, or mDNS
- **Reverse Proxy** - Route traffic with WebSocket support
- **QR Code Pairing** - Fast mobile app onboarding
- **Secure by Default** - TLS, JWT authentication

## Quick Start

```bash
docker run -d \
  --name nekzus \
  -p 8080:8080 \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -v nekzus-data:/app/data \
  nstalgic/nekzus:latest
```

Access the web dashboard at [http://localhost:8080](http://localhost:8080)

## Documentation

<div class="grid cards" markdown>

-   :material-rocket-launch:{ .lg .middle } **Getting Started**

    ---

    Install Nekzus and get up and running in minutes

    [:octicons-arrow-right-24: Installation](getting-started/installation.md)

-   :material-cog:{ .lg .middle } **Configuration**

    ---

    Configure routes, discovery, and authentication

    [:octicons-arrow-right-24: Configuration](getting-started/configuration.md)

-   :material-server:{ .lg .middle } **Platforms**

    ---

    Platform-specific guides for Synology, Unraid, Proxmox, and more

    [:octicons-arrow-right-24: Platforms](platforms/index.md)

-   :material-api:{ .lg .middle } **API Reference**

    ---

    Complete API documentation for integrations

    [:octicons-arrow-right-24: API Reference](reference/api.md)

</div>

## Community

- [GitHub Repository](https://github.com/nstalgic/nekzus)
- [Docker Hub](https://hub.docker.com/r/nstalgic/nekzus)
- [Issue Tracker](https://github.com/nstalgic/nekzus/issues)
