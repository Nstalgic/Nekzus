# Contributing to Nekzus

Thank you for your interest in contributing to Nekzus! This guide will help you get started.

## Getting Started

1. **Fork** the repository on GitHub
2. **Clone** your fork locally:
   ```bash
   git clone https://github.com/nstalgic/Nekzus.git
   cd Nekzus
   ```
3. **Create a branch** for your change:
   ```bash
   git checkout -b feature/my-change
   ```

## Development Setup

### Prerequisites

- Go 1.25+
- Node.js 20+ (for the web dashboard)
- Docker (optional, for container-related features)

### Build & Run

```bash
# Install dependencies and build everything
make build-all

# Run locally without TLS (development)
make run-insecure

# Run the web UI with hot reload
make dev-web

# Run tests
make test
```

See the full list of available commands with `make help`.

## Making Changes

### Code Style

- **Go**: Run `make fmt` before committing. Run `make lint` to check for issues.
- **TypeScript/React** (web UI): Follow the existing patterns in `web/src/`.
- Keep changes focused — one feature or fix per PR.

### Commit Messages

Use clear, descriptive commit messages:

```
Add health check retry logic for flaky services

Previously, services that returned a single failed health check were
immediately marked as down. This adds a configurable retry threshold
before changing service status.
```

### Testing

- Run `make test` to execute all tests with the race detector
- Run `make test-short` to skip integration/E2E tests
- Add tests for new functionality
- Ensure existing tests pass before submitting a PR

## Submitting a Pull Request

1. Push your branch to your fork
2. Open a Pull Request against the `main` branch
3. Fill in a clear title and description of your changes
4. Link any related issues (e.g., `Closes #42`)
5. Wait for review — maintainers may request changes

### PR Guidelines

- Keep PRs small and focused when possible
- Include tests for new features or bug fixes
- Update documentation if your change affects user-facing behavior
- Make sure CI passes before requesting review

## Reporting Issues

- Use [GitHub Issues](https://github.com/Nstalgic/Nekzus/issues) to report bugs or request features
- Include steps to reproduce for bug reports
- Check existing issues before opening a new one

## Questions?

Open a [GitHub Discussion](https://github.com/Nstalgic/Nekzus/discussions) or file an issue if you need help.

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).
