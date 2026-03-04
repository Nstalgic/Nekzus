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

<div className="card-grid">
  <div className="card">
    <h3>🚀 Getting Started</h3>
    <p>Install Nekzus and get up and running in minutes</p>
    <a href="getting-started/installation">→ Installation</a>
  </div>
  <div className="card">
    <h3>⚙️ Configuration</h3>
    <p>Configure routes, discovery, and authentication</p>
    <a href="getting-started/configuration">→ Configuration</a>
  </div>
  <div className="card">
    <h3>🖥️ Platforms</h3>
    <p>Platform-specific guides for Synology, Unraid, Proxmox, and more</p>
    <a href="platforms/">→ Platforms</a>
  </div>
  <div className="card">
    <h3>📡 API Reference</h3>
    <p>Complete API documentation for integrations</p>
    <a href="reference/api">→ API Reference</a>
  </div>
</div>

## Community

- [GitHub Repository](https://github.com/nstalgic/nekzus)
- [Docker Hub](https://hub.docker.com/r/nstalgic/nekzus)
- [Issue Tracker](https://github.com/nstalgic/nekzus/issues)
