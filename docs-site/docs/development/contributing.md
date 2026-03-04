import Tabs from "@theme/Tabs";
import TabItem from "@theme/TabItem";

# Contributing to Nekzus

Thank you for your interest in contributing to Nekzus! This guide covers everything you need to know to contribute effectively to the project.

## Table of Contents

- [Getting Started](#getting-started)
- [Development Workflow](#development-workflow)
- [Code Standards](#code-standards)
- [Test-Driven Development](#test-driven-development)
- [Testing](#testing)
- [Pull Request Process](#pull-request-process)
- [Documentation](#documentation)
- [Issue Guidelines](#issue-guidelines)

---

## Getting Started

### Prerequisites

Before you begin, ensure you have the following installed:

| Tool | Version | Purpose |
|------|---------|---------|
| Go | 1.22+ | Backend development |
| Node.js | 20+ | Frontend development |
| Docker | Latest | Container testing |
| Make | Any | Build automation |
| Git | Latest | Version control |

### Fork and Clone

1. **Fork the repository** on GitHub

2. **Clone your fork**:

    ```bash
    git clone https://github.com/YOUR_USERNAME/nekzus.git
    cd nekzus
    ```

3. **Add the upstream remote**:

    ```bash
    git remote add upstream https://github.com/nstalgic/nekzus.git
    ```

4. **Verify remotes**:

    ```bash
    git remote -v
    # origin    https://github.com/YOUR_USERNAME/nekzus.git (fetch)
    # origin    https://github.com/YOUR_USERNAME/nekzus.git (push)
    # upstream  https://github.com/nstalgic/nekzus.git (fetch)
    # upstream  https://github.com/nstalgic/nekzus.git (push)
    ```

### Setting Up the Development Environment

<Tabs>
<TabItem value="macos" label="macOS">


```bash
# Install dependencies with Homebrew
brew install go node docker docker-compose

# For Docker on Apple Silicon
brew install colima
colima start --cpu 6 --memory 8 --disk 60 --vm-type=vz
docker context use colima

# Or use the make target
make first-setup
```

</TabItem>
<TabItem value="linux" label="Linux">


```bash
# Install Go (Ubuntu/Debian)
sudo apt update
sudo apt install golang-go nodejs npm docker.io docker-compose

# Add user to docker group
sudo usermod -aG docker $USER
newgrp docker
```

</TabItem>
<TabItem value="windows" label="Windows">


```powershell
# Install with winget
winget install GoLang.Go
winget install OpenJS.NodeJS.LTS
winget install Docker.DockerDesktop

# Or use WSL2 with Linux instructions
```

</TabItem>
</Tabs>

### Install Dependencies

```bash
# Install Go dependencies
go mod download

# Install frontend dependencies
cd web && npm install && cd ..

# Verify installation
go version
node --version
docker --version
```

### Build and Run

```bash
# Build everything (web UI + Go binary)
make build-all

# Run in development mode (no TLS)
make run-insecure

# Or start the demo environment
make demo
```

Access the web dashboard at [http://localhost:8080](http://localhost:8080)

---

## Development Workflow

### Branch Naming Convention

Use descriptive branch names with the following prefixes:

| Prefix | Purpose | Example |
|--------|---------|---------|
| `feature/` | New features | `feature/websocket-compression` |
| `fix/` | Bug fixes | `fix/proxy-timeout-handling` |
| `refactor/` | Code refactoring | `refactor/discovery-manager` |
| `docs/` | Documentation updates | `docs/api-reference` |
| `test/` | Test additions/fixes | `test/federation-e2e` |
| `chore/` | Maintenance tasks | `chore/update-dependencies` |

### Creating a Branch

```bash
# Sync with upstream
git fetch upstream
git checkout main
git merge upstream/main

# Create your feature branch
git checkout -b feature/your-feature-name
```

### Commit Message Format

Follow the Conventional Commits specification:

```
<type>(<scope>): <description>

[optional body]

[optional footer]
```

**Types:**

- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `style`: Code style changes (formatting, semicolons)
- `refactor`: Code refactoring
- `test`: Adding or updating tests
- `chore`: Maintenance tasks

**Examples:**

```bash
# Feature
git commit -m "feat(proxy): add WebSocket compression support"

# Bug fix
git commit -m "fix(discovery): handle Docker socket timeout"

# Documentation
git commit -m "docs(api): update authentication endpoints"

# With body
git commit -m "fix(auth): validate JWT audience claim

The audience claim was not being validated, allowing tokens
issued for other services to be accepted.

Closes #123"
```

### Keep Your Branch Updated

```bash
# Fetch latest changes
git fetch upstream

# Rebase your branch
git rebase upstream/main

# Force push if needed (only for your own branches)
git push origin feature/your-feature --force-with-lease
```

---

## Code Standards

### Go Code Style

#### Formatting

All Go code must be formatted with `gofmt`:

```bash
# Format all Go files
go fmt ./...

# Or use make target
make fmt
```

#### Linting

Run the linter before committing:

```bash
# Run golangci-lint
make lint

# Or directly
golangci-lint run
```

#### Error Handling

Use the structured error package from `internal/errors`:

```go
import apperrors "github.com/nstalgic/nekzus/internal/errors"

// Create a new error
func doSomething() error {
    if somethingFailed {
        return apperrors.New(
            "OPERATION_FAILED",
            "Operation failed due to invalid input",
            http.StatusBadRequest,
        )
    }
    return nil
}

// Wrap an existing error
func processData(data []byte) error {
    result, err := parseData(data)
    if err != nil {
        return apperrors.Wrap(
            err,
            "PARSE_ERROR",
            "Failed to parse input data",
            http.StatusBadRequest,
        )
    }
    return nil
}

// Write error response
func handleRequest(w http.ResponseWriter, r *http.Request) {
    err := doSomething()
    if err != nil {
        apperrors.WriteJSON(w, err)
        return
    }
}
```

#### Storage Operations

Always check for nil storage and handle gracefully:

```go
// Check if storage is available
if app.storage != nil {
    device, err := app.storage.GetDevice(deviceID)
    if err != nil {
        // Handle error
    }
}

// Async updates for non-critical operations
go func() {
    if app.storage != nil {
        if err := app.storage.UpdateDeviceLastSeen(deviceID); err != nil {
            log.Printf("Warning: failed to update last seen: %v", err)
        }
    }
}()
```

#### Metrics Recording

Record metrics for observability:

```go
// HTTP requests
app.metrics.RecordHTTPRequest(method, path, status, duration, reqSize, respSize)

// Authentication
app.metrics.RecordAuthPairing("success", platform, duration)

// Proxy requests
app.metrics.RecordProxyRequest(appID, status, duration)
```

### React/JavaScript Code Style

#### File Organization

```
web/src/
├── components/      # Reusable UI components
│   ├── Button.jsx
│   └── Card.jsx
├── contexts/        # React contexts
│   ├── AuthContext.jsx
│   └── SettingsContext.jsx
├── pages/           # Page components
│   ├── Dashboard.jsx
│   └── Settings.jsx
├── hooks/           # Custom hooks
│   └── useApi.js
└── styles/          # CSS files
    ├── base.css
    ├── themes.css
    └── app.css
```

#### Component Structure

```jsx
import { useState, useEffect } from 'react';
import PropTypes from 'prop-types';

export function MyComponent({ title, onAction }) {
  const [isLoading, setIsLoading] = useState(false);

  useEffect(() => {
    // Effect logic
  }, []);

  const handleClick = () => {
    setIsLoading(true);
    onAction();
  };

  return (
    <div className="my-component">
      <h2>{title}</h2>
      <button onClick={handleClick} disabled={isLoading}>
        {isLoading ? 'Loading...' : 'Click Me'}
      </button>
    </div>
  );
}

MyComponent.propTypes = {
  title: PropTypes.string.isRequired,
  onAction: PropTypes.func.isRequired,
};
```

### CSS Conventions

Nekzus uses a design token system with CSS custom properties. **Never use hardcoded values.**

#### Design Token Architecture

```
base.css    - Design tokens and base styles
themes.css  - Theme-specific overrides (8 themes)
app.css     - Application component styles
```

#### Correct Usage

```css
/* CORRECT - Use CSS variables */
.card {
  padding: var(--space-4);
  background: var(--bg-secondary);
  color: var(--text-primary);
  border: var(--border-width) solid var(--border-color);
  border-radius: var(--radius-md);
}

.button-primary {
  background: var(--accent-primary);
  color: var(--text-white);
  font-weight: var(--font-weight-bold);
}

/* INCORRECT - Hardcoded values */
.card {
  padding: 16px;           /* Use var(--space-4) */
  background: #1a1f2e;     /* Use var(--bg-secondary) */
  color: #f8fafc;          /* Use var(--text-primary) */
  border-radius: 8px;      /* Use var(--radius-md) */
}
```

#### Available Design Tokens

| Category | Examples |
|----------|----------|
| **Spacing** | `--space-1` through `--space-12` (8px scale) |
| **Colors** | `--bg-primary`, `--text-primary`, `--accent-primary` |
| **Typography** | `--font-mono`, `--font-weight-bold` |
| **Borders** | `--border-width`, `--border-color`, `--radius-md` |

---

## Test-Driven Development

:::warning[TDD is Mandatory]

Nekzus practices Test-Driven Development. This is **NOT optional**. All contributions must follow the TDD workflow.

:::


### The TDD Cycle

```d2
direction: right

red: Red\nWrite Failing Test {
  style.fill: "#fee2e2"
}
green: Green\nImplement Code {
  style.fill: "#d1fae5"
}
refactor: Refactor\nClean Up {
  style.fill: "#dbeafe"
}
document: Document\nUpdate Docs {
  style.fill: "#f3e8ff"
}

red -> green -> refactor -> document -> red
```

1. **Red** - Write a failing test that defines the expected behavior
2. **Green** - Write the minimum code necessary to pass the test
3. **Refactor** - Clean up the code while keeping tests green
4. **Document** - Update documentation to reflect changes

### TDD Rules

| Rule | Description |
|------|-------------|
| Tests First | Write tests BEFORE implementation code |
| Behavior-Driven | Tests define expected behavior and API contracts |
| Complete Coverage | All new features REQUIRE test coverage FIRST |
| Coverage Targets | Maintain 80%+ coverage for critical packages |

### Example: TDD Workflow

**Step 1: Write the failing test first**

```go
// internal/proxy/cache_test.go
func TestCache_GetOrCreate(t *testing.T) {
    cache := NewCache()
    target, _ := url.Parse("http://localhost:8080")

    // First call should create proxy
    proxy1 := cache.GetOrCreate(target)
    if proxy1 == nil {
        t.Fatal("expected non-nil proxy")
    }

    // Second call should return same proxy (cached)
    proxy2 := cache.GetOrCreate(target)
    if proxy1 != proxy2 {
        t.Error("expected same proxy instance from cache")
    }
}
```

**Step 2: Run the test (it fails)**

```bash
go test -v ./internal/proxy/...
# --- FAIL: TestCache_GetOrCreate
```

**Step 3: Implement the minimum code to pass**

```go
// internal/proxy/cache.go
func (c *Cache) GetOrCreate(target *url.URL) *httputil.ReverseProxy {
    c.mu.Lock()
    defer c.mu.Unlock()

    key := target.String()
    if proxy, exists := c.proxies[key]; exists {
        return proxy
    }

    proxy := httputil.NewSingleHostReverseProxy(target)
    c.proxies[key] = proxy
    return proxy
}
```

**Step 4: Run the test (it passes)**

```bash
go test -v ./internal/proxy/...
# --- PASS: TestCache_GetOrCreate
```

**Step 5: Refactor if needed, keeping tests green**

---

## Testing

### Running Tests

```bash
# All tests with race detector (recommended)
go test -race ./...

# Unit tests only (fast, skip E2E)
go test -race -short ./...

# Specific package with verbose output
go test -race -v ./internal/proxy/...

# With coverage report
go test -race -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Make Targets for Testing

| Command | Description |
|---------|-------------|
| `make test` | Run all tests with race detector |
| `make test-short` | Run unit tests only (skip E2E) |
| `make test-fast` | Fast E2E with persistent infrastructure |
| `make e2e` | Start E2E test environment |
| `make e2e-test` | Run E2E test battery |

### Test Organization

```
project/
├── cmd/nekzus/
│   ├── main_test.go           # Integration tests
│   ├── e2e_test.go            # E2E tests
│   └── *_test.go              # Handler tests
├── internal/
│   ├── proxy/
│   │   ├── proxy.go
│   │   └── proxy_test.go      # Unit tests
│   └── auth/
│       ├── jwt.go
│       └── jwt_test.go        # Unit tests
└── tests/
    └── e2e/                   # E2E test infrastructure
```

### Writing Good Tests

#### Table-Driven Tests

```go
func TestValidateInput(t *testing.T) {
    tests := []struct {
        name        string
        input       string
        wantErr     bool
        errContains string
    }{
        {
            name:    "valid input",
            input:   "hello",
            wantErr: false,
        },
        {
            name:        "empty input",
            input:       "",
            wantErr:     true,
            errContains: "input required",
        },
        {
            name:        "too long",
            input:       strings.Repeat("a", 1000),
            wantErr:     true,
            errContains: "exceeds maximum",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateInput(tt.input)

            if tt.wantErr {
                if err == nil {
                    t.Error("expected error, got nil")
                } else if !strings.Contains(err.Error(), tt.errContains) {
                    t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
                }
                return
            }

            if err != nil {
                t.Fatalf("unexpected error: %v", err)
            }
        })
    }
}
```

#### Test Helpers

```go
func setupTestServer(t *testing.T) (*httptest.Server, func()) {
    t.Helper()

    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte(`{"status": "ok"}`))
    })

    server := httptest.NewServer(handler)

    cleanup := func() {
        server.Close()
    }

    return server, cleanup
}
```

### Coverage Requirements

| Package Type | Minimum Coverage |
|--------------|------------------|
| Critical (auth, proxy, storage) | 80%+ |
| Standard (handlers, middleware) | 70%+ |
| Utilities | 60%+ |

Check coverage:

```bash
# Generate coverage report
go test -race -coverprofile=coverage.out ./...

# View coverage by package
go tool cover -func=coverage.out

# View detailed HTML report
go tool cover -html=coverage.out -o coverage.html
```

---

## Pull Request Process

### Before Submitting

- [ ] Tests written FIRST (TDD)
- [ ] All tests pass: `go test -race ./...`
- [ ] Code formatted: `go fmt ./...`
- [ ] Linter passes: `make lint`
- [ ] Documentation updated
- [ ] Commit messages follow convention

### Creating a Pull Request

1. **Push your branch**:

    ```bash
    git push origin feature/your-feature
    ```

2. **Create PR on GitHub** with a descriptive title

3. **Fill out the PR template**:

    ```markdown
    ## Description
    Brief description of changes

    ## Type of Change
    - [ ] Bug fix
    - [ ] New feature
    - [ ] Breaking change
    - [ ] Documentation update

    ## Testing
    - [ ] Unit tests added/updated
    - [ ] Integration tests added/updated
    - [ ] All tests pass locally

    ## Checklist
    - [ ] TDD workflow followed
    - [ ] Code formatted with go fmt
    - [ ] Linter passes
    - [ ] Documentation updated
    - [ ] No hardcoded CSS values
    ```

### CI Checks

The following checks run automatically on PRs:

| Check | Description |
|-------|-------------|
| **Test** | Runs all tests with race detector |
| **Lint** | Checks go fmt and go vet |
| **Build** | Verifies successful compilation |

### Review Process

1. **Automated Checks** - CI must pass
2. **Code Review** - At least one maintainer review
3. **Testing** - Reviewer may request additional tests
4. **Approval** - PR approved and merged

### After Merge

```bash
# Sync your local main
git checkout main
git fetch upstream
git merge upstream/main

# Delete your feature branch
git branch -d feature/your-feature
git push origin --delete feature/your-feature
```

---

## Documentation

### Documentation Structure

```
docs/
├── getting-started/    # Installation and setup
├── guides/             # How-to guides
├── features/           # Feature documentation
├── reference/          # API and CLI reference
├── development/        # Contributing guides
├── platforms/          # Platform-specific docs
└── kubernetes/         # Kubernetes deployment
```

### Writing Documentation

Documentation uses MkDocs Material with these conventions:

#### Admonitions

```markdown
:::note[Optional Title]

This is a note.

:::


:::warning

This is a warning.

:::


:::tip

This is a tip.

:::

```

#### Code Blocks

````markdown
```bash title="Terminal"
make build
```

```go title="example.go" linenums="1" hl_lines="3 4"
func main() {
    // Setup
    config := LoadConfig()
    server := NewServer(config)
    server.Start()
}
```
````

#### Tabs

````markdown
<Tabs>
<TabItem value="macos" label="macOS">

```bash
brew install nekzus
```

</TabItem>
<TabItem value="linux" label="Linux">

```bash
apt install nekzus
```

</TabItem>
</Tabs>
````

### Building Documentation

```bash
# Install dependencies
cd docs-site
npm install

# Serve locally with hot reload
npm start

# Build static site
npm run build
```

### When to Update Docs

- New features require documentation
- API changes need reference updates
- Configuration changes need examples
- Bug fixes may need troubleshooting guides

---

## Issue Guidelines

### Reporting Bugs

Use the bug report template:

```markdown
**Describe the bug**
A clear description of what the bug is.

**To Reproduce**
Steps to reproduce:
1. Go to '...'
2. Click on '...'
3. See error

**Expected behavior**
What you expected to happen.

**Environment**
- OS: [e.g., macOS 14.0]
- Nekzus version: [e.g., v1.2.0]
- Docker version: [e.g., 24.0.0]
- Browser: [e.g., Chrome 120]

**Logs**
```
Relevant log output
```

**Screenshots**
If applicable, add screenshots.
```

### Feature Requests

Use the feature request template:

```markdown
**Is your feature request related to a problem?**
A clear description of the problem.

**Describe the solution you'd like**
A clear description of what you want to happen.

**Describe alternatives you've considered**
Any alternative solutions or features you've considered.

**Additional context**
Add any other context, mockups, or examples.
```

### Good First Issues

Look for issues labeled `good first issue` if you are new to the project. These are specifically chosen to be approachable for newcomers.

### Issue Labels

| Label | Description |
|-------|-------------|
| `bug` | Something is not working |
| `enhancement` | New feature or request |
| `documentation` | Documentation improvements |
| `good first issue` | Good for newcomers |
| `help wanted` | Extra attention needed |
| `question` | Further information requested |

---

## Getting Help

- **GitHub Issues** - For bugs and feature requests
- **GitHub Discussions** - For questions and community help
- **Documentation** - [https://nstalgic.github.io/nekzus/](https://nstalgic.github.io/nekzus/)

---

## Code of Conduct

Be respectful, inclusive, and constructive. We are all here to build something great together.

---

Thank you for contributing to Nekzus!
