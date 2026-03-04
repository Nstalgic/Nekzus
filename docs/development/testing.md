# Testing Guide

This guide covers the testing philosophy, practices, and tools used in Nekzus development. Test-Driven Development (TDD) is mandatory for all contributions.

---

## Testing Philosophy

### TDD is Mandatory

Nekzus follows strict Test-Driven Development practices. This is not optional.

**TDD Workflow:**

1. **Red** - Write failing tests FIRST that define expected behavior
2. **Green** - Implement minimum code to make tests pass
3. **Refactor** - Clean up code while keeping tests green
4. **Document** - Update documentation to reflect changes

**Core Principles:**

- Tests define expected behavior and API contracts
- All new features require test coverage FIRST
- Never write production code without tests first
- Maintain 80%+ coverage for critical packages

---

## Test Structure

### Directory Organization

```
nekzus/
├── cmd/nekzus/
│   ├── main_test.go              # Application startup tests
│   ├── e2e_test.go               # End-to-end integration tests
│   ├── proxy_test.go             # Proxy handler tests
│   ├── websocket_handler_test.go # WebSocket handler tests
│   └── *_test.go                 # Other handler tests
│
├── internal/
│   ├── auth/
│   │   ├── jwt_test.go           # JWT authentication tests
│   │   └── scopes_test.go        # Authorization scope tests
│   ├── proxy/
│   │   ├── proxy_test.go         # HTTP proxy tests
│   │   └── websocket_test.go     # WebSocket proxy tests
│   ├── middleware/
│   │   ├── ratelimit_test.go     # Rate limiting tests
│   │   └── *_test.go             # Other middleware tests
│   ├── discovery/
│   │   ├── docker_test.go        # Docker discovery tests
│   │   └── kubernetes_test.go    # Kubernetes discovery tests
│   ├── toolbox/
│   │   └── manager_test.go       # Toolbox catalog tests
│   └── storage/
│       └── *_test.go             # Database storage tests
│
└── tests/
    └── e2e/
        ├── docker-compose.e2e.yaml
        └── test-runner/
            ├── basic_test.go     # Basic E2E tests
            └── advanced_test.go  # Advanced E2E tests
```

### Test Types

| Type | Purpose | Command | Location |
|------|---------|---------|----------|
| Unit Tests | Test individual functions/methods | `go test -short ./...` | `*_test.go` alongside code |
| Integration Tests | Test component interactions | `go test ./...` | `cmd/nekzus/*_test.go` |
| E2E Tests | Test full system behavior | `make e2e-test` | `tests/e2e/` |

---

## Running Tests

### All Tests with Race Detector

The recommended way to run all tests:

```bash
go test -race ./...
```

!!! warning "Always Use Race Detector"
    The `-race` flag detects data races in concurrent code. Always include it when running tests locally or in CI.

### Unit Tests Only (Fast)

Skip long-running integration and E2E tests:

```bash
go test -race ./... -short
```

### Specific Package

Test a single package with verbose output:

```bash
go test -race ./internal/proxy/... -v
```

### Specific Test Function

Run a single test by name:

```bash
go test -race ./internal/auth/... -v -run TestSignAndParseJWT
```

### Test with Coverage

Generate coverage report:

```bash
# Generate coverage profile
go test -race -coverprofile=coverage.out ./...

# View coverage in browser
go tool cover -html=coverage.out

# View coverage summary
go tool cover -func=coverage.out
```

---

## Makefile Commands

The Makefile provides convenient test targets:

```bash
# Run all tests with race detector
make test

# Run unit tests only (skip E2E)
make test-short

# Start E2E test environment
make e2e

# Run E2E test battery
make e2e-test

# Stop E2E environment
make e2e-down

# Fast E2E with persistent infrastructure
make test-infra-up   # Start once
make test-fast       # Run repeatedly
make test-infra-down # Stop when done
```

---

## Writing Tests

### Table-Driven Tests

Use table-driven tests for comprehensive coverage:

```go
func TestIsValidAppID(t *testing.T) {
    tests := []struct {
        name  string
        appID string
        want  bool
    }{
        {
            name:  "simple lowercase",
            appID: "grafana",
            want:  true,
        },
        {
            name:  "with dashes",
            appID: "uptime-kuma",
            want:  true,
        },
        {
            name:  "empty string",
            appID: "",
            want:  false,
        },
        {
            name:  "with spaces",
            appID: "my app",
            want:  false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := isValidAppID(tt.appID)
            if got != tt.want {
                t.Errorf("isValidAppID(%q) = %v, want %v", tt.appID, got, tt.want)
            }
        })
    }
}
```

### Subtests

Use subtests to organize related test cases:

```go
func TestAPIKeyStorage(t *testing.T) {
    // Setup
    tmpDB := "test_apikey.db"
    defer os.Remove(tmpDB)

    store, err := NewStore(Config{DatabasePath: tmpDB})
    if err != nil {
        t.Fatalf("Failed to create store: %v", err)
    }
    defer store.Close()

    t.Run("CreateAPIKey", func(t *testing.T) {
        apiKey := &types.APIKey{
            ID:      "key-123",
            Name:    "Test API Key",
            KeyHash: "hash123",
        }

        err := store.CreateAPIKey(apiKey)
        if err != nil {
            t.Fatalf("Failed to create API key: %v", err)
        }
    })

    t.Run("GetAPIKey", func(t *testing.T) {
        retrieved, err := store.GetAPIKey("key-123")
        if err != nil {
            t.Fatalf("Failed to get API key: %v", err)
        }
        if retrieved == nil {
            t.Fatal("Expected API key to exist")
        }
    })
}
```

### Test Helpers

Use `t.Helper()` for cleaner stack traces:

```go
func setupTestComposeDir(t *testing.T) string {
    t.Helper()

    tempDir := t.TempDir()

    grafanaDir := filepath.Join(tempDir, "grafana")
    if err := os.MkdirAll(grafanaDir, 0755); err != nil {
        t.Fatalf("Failed to create grafana directory: %v", err)
    }

    // Write test files...

    return tempDir
}
```

### HTTP Handler Tests

Use `httptest` for testing HTTP handlers:

```go
func TestRateLimitMiddleware(t *testing.T) {
    limiter := ratelimit.NewLimiter(1.0, 2)
    defer limiter.Stop()

    handler := RateLimit(limiter)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte("OK"))
    }))

    // First request should succeed
    req1 := httptest.NewRequest("GET", "/test", nil)
    req1.RemoteAddr = "192.168.1.100:12345"
    w1 := httptest.NewRecorder()
    handler.ServeHTTP(w1, req1)

    if w1.Code != http.StatusOK {
        t.Errorf("First request: got status %d, want %d", w1.Code, http.StatusOK)
    }

    // Second request should succeed (within burst)
    req2 := httptest.NewRequest("GET", "/test", nil)
    req2.RemoteAddr = "192.168.1.100:12345"
    w2 := httptest.NewRecorder()
    handler.ServeHTTP(w2, req2)

    if w2.Code != http.StatusOK {
        t.Errorf("Second request: got status %d, want %d", w2.Code, http.StatusOK)
    }

    // Third request should be rate limited
    req3 := httptest.NewRequest("GET", "/test", nil)
    req3.RemoteAddr = "192.168.1.100:12345"
    w3 := httptest.NewRecorder()
    handler.ServeHTTP(w3, req3)

    if w3.Code != http.StatusTooManyRequests {
        t.Errorf("Third request: got status %d, want %d", w3.Code, http.StatusTooManyRequests)
    }
}
```

### Skipping Long Tests

Use short mode to skip integration tests:

```go
func TestEndToEnd(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping E2E test in short mode")
    }

    // Long-running test code...
}
```

---

## Mocking

### Mock Interfaces

Create mock implementations for testing:

```go
// mockProposalStore implements ProposalStore for testing
type mockProposalStore struct {
    proposals map[string]*types.Proposal
    mu        sync.Mutex
}

func (m *mockProposalStore) SaveProposal(p *types.Proposal) error {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.proposals[p.ID] = p
    return nil
}

func (m *mockProposalStore) GetProposal(id string) (*types.Proposal, error) {
    m.mu.Lock()
    defer m.mu.Unlock()
    return m.proposals[id], nil
}
```

### Mock Event Bus

```go
type mockEventBus struct {
    events []interface{}
    mu     sync.Mutex
}

func (m *mockEventBus) Publish(event interface{}) {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.events = append(m.events, event)
}
```

### Test Servers

Use `httptest.Server` for testing HTTP clients:

```go
func TestWebSocketProxy(t *testing.T) {
    // Create mock upstream server
    upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte("OK"))
    }))
    defer upstream.Close()

    // Create proxy pointing to mock server
    proxy := NewWebSocketProxy(upstream.URL)

    // Test proxy behavior...
}
```

---

## Test Fixtures

### Temporary Directories

Use `t.TempDir()` for test directories (auto-cleaned):

```go
func TestLoadCatalog(t *testing.T) {
    catalogDir := t.TempDir()

    // Create test files in tempDir
    composeContent := `services:
  grafana:
    image: grafana/grafana:latest
    labels:
      nekzus.toolbox.name: "Grafana"
      nekzus.toolbox.category: "monitoring"
`

    grafanaDir := filepath.Join(catalogDir, "grafana")
    os.MkdirAll(grafanaDir, 0755)
    os.WriteFile(
        filepath.Join(grafanaDir, "docker-compose.yml"),
        []byte(composeContent),
        0644,
    )

    // Test catalog loading...
}
```

### Temporary Databases

Use temporary files for database tests:

```go
func TestStorage(t *testing.T) {
    tmpDB := filepath.Join(t.TempDir(), "test.db")

    store, err := NewStore(Config{DatabasePath: tmpDB})
    if err != nil {
        t.Fatalf("Failed to create store: %v", err)
    }
    defer store.Close()

    // Run tests...
}
```

### Test Data

Create helper functions for common test data:

```go
func createTestAPIKey(t *testing.T, store *Store) *types.APIKey {
    t.Helper()

    apiKey := &types.APIKey{
        ID:        "key-" + uuid.New().String()[:8],
        Name:      "Test API Key",
        KeyHash:   "hash-" + uuid.New().String()[:8],
        Prefix:    "nekzus_test",
        Scopes:    []string{"read:catalog"},
        CreatedAt: time.Now(),
    }

    if err := store.CreateAPIKey(apiKey); err != nil {
        t.Fatalf("Failed to create test API key: %v", err)
    }

    return apiKey
}
```

---

## Race Condition Testing

### Race Detector

Always run tests with `-race`:

```bash
go test -race ./...
```

### Testing Concurrent Access

```go
func TestManager_UpdateBootstrapTokens_RaceCondition(t *testing.T) {
    mgr, err := NewManager(
        []byte(strings.Repeat("a", 32)),
        "test-issuer",
        "test-audience",
        []string{"initial-token"},
    )
    if err != nil {
        t.Fatalf("failed to create manager: %v", err)
    }
    defer mgr.Stop()

    var wg sync.WaitGroup
    done := make(chan struct{})

    // Run multiple goroutines updating tokens
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func(n int) {
            defer wg.Done()
            for {
                select {
                case <-done:
                    return
                default:
                    tokens := []string{"token-" + string(rune('A'+n))}
                    mgr.UpdateBootstrapTokens(tokens)
                }
            }
        }(i)
    }

    // Run multiple goroutines validating tokens
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for {
                select {
                case <-done:
                    return
                default:
                    mgr.ValidateBootstrap("any-token")
                }
            }
        }()
    }

    // Let it run briefly
    time.Sleep(100 * time.Millisecond)
    close(done)
    wg.Wait()
}
```

---

## Coverage Requirements

### Minimum Coverage

- **Critical packages** (auth, proxy, storage): 80%+
- **Business logic**: 70%+
- **Utilities**: 60%+

### Checking Coverage

```bash
# Generate coverage report
go test -coverprofile=coverage.out ./...

# View summary
go tool cover -func=coverage.out | grep total

# View detailed report
go tool cover -html=coverage.out
```

### Coverage by Package

```bash
# Coverage for specific package
go test -cover ./internal/auth/...

# Coverage with function breakdown
go test -coverprofile=auth.out ./internal/auth/...
go tool cover -func=auth.out
```

---

## E2E Testing

### Environment Setup

Start the E2E test environment:

```bash
# Start E2E environment
make e2e

# View logs
make e2e-logs

# Check status
make e2e-status
```

### Running E2E Tests

```bash
# Run test battery
make e2e-test

# Run with TAP output (CI format)
make e2e-test-tap

# Run with JSON output
make e2e-test-json
```

### E2E Test Structure

```go
func TestEndToEnd(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping E2E test in short mode")
    }

    client := &http.Client{
        Transport: &http.Transport{
            TLSClientConfig: &tls.Config{
                InsecureSkipVerify: true,
            },
        },
        Timeout: 10 * time.Second,
    }

    t.Run("1_Healthcheck", func(t *testing.T) {
        resp, err := client.Get(nexusURL + "/api/v1/healthz")
        if err != nil {
            t.Fatalf("Healthcheck failed: %v", err)
        }
        defer resp.Body.Close()

        if resp.StatusCode != http.StatusOK {
            t.Fatalf("Expected status 200, got %d", resp.StatusCode)
        }
    })

    t.Run("2_DevicePairing", func(t *testing.T) {
        // Test device pairing...
    })

    t.Run("3_ProxyRouting", func(t *testing.T) {
        // Test proxy routing...
    })
}
```

### Persistent Test Infrastructure

For faster iterative testing:

```bash
# Start infrastructure once
make test-infra-up

# Run fast tests repeatedly
make test-fast

# Stop when done
make test-infra-down
```

---

## Frontend Testing

### React Component Testing

The frontend uses Vitest for testing React components:

```bash
cd web

# Run tests
npm test

# Run with coverage
npm run test:coverage

# Watch mode
npm run test:watch
```

### Component Test Example

```jsx
import { render, screen } from '@testing-library/react';
import { describe, it, expect } from 'vitest';
import { ServiceCard } from './ServiceCard';

describe('ServiceCard', () => {
  it('renders service name', () => {
    const service = {
      id: 'grafana',
      name: 'Grafana',
      status: 'online',
    };

    render(<ServiceCard service={service} />);

    expect(screen.getByText('Grafana')).toBeInTheDocument();
  });

  it('shows online status indicator', () => {
    const service = {
      id: 'grafana',
      name: 'Grafana',
      status: 'online',
    };

    render(<ServiceCard service={service} />);

    expect(screen.getByTestId('status-indicator')).toHaveClass('online');
  });
});
```

---

## CI/CD Integration

### GitHub Actions

Tests run automatically on every push and pull request:

```yaml
# .github/workflows/test.yml
name: Tests

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.25'

      - name: Run tests
        run: go test -race -v ./...

      - name: Run tests with coverage
        run: go test -race -coverprofile=coverage.out ./...

      - name: Upload coverage
        uses: codecov/codecov-action@v4
        with:
          files: ./coverage.out
```

### Pre-commit Checks

Run tests before committing:

```bash
# Run short tests
go test -race ./... -short

# Format code
go fmt ./...

# Lint (if golangci-lint installed)
golangci-lint run
```

---

## Troubleshooting

### Common Issues

**Tests timing out:**

```bash
# Increase timeout
go test -timeout 5m ./...
```

**Race conditions detected:**

```bash
# Run with race detector to find the issue
go test -race -v ./path/to/package -run TestName
```

**Database tests failing:**

```bash
# Ensure SQLite is installed
go build -tags sqlite ./...

# Run storage tests specifically
go test -v ./internal/storage/...
```

**E2E tests failing:**

```bash
# Check if services are running
make e2e-status

# View logs
make e2e-logs

# Restart environment
make e2e-down && make e2e
```

### Debugging Tests

```bash
# Verbose output
go test -v ./...

# Print to stdout during tests
t.Logf("Debug: value = %v", value)

# Run single test with verbose output
go test -v -run TestSpecificFunction ./path/to/package
```

---

## Best Practices

### Test Naming

- Use descriptive names: `TestValidateLabels_InvalidAppID`
- Group related tests with subtests
- Use underscores to separate concepts

### Test Independence

- Each test should be independent
- Use `t.Cleanup()` or `defer` for cleanup
- Avoid shared state between tests

### Assertions

- Use clear error messages
- Include actual and expected values
- Fail fast with `t.Fatalf()` for setup errors

### Cleanup

```go
func TestWithCleanup(t *testing.T) {
    // Create temporary resources
    tmpDir := t.TempDir()  // Auto-cleaned

    // Manual cleanup
    t.Cleanup(func() {
        // Cleanup code here
    })

    // Or use defer
    defer func() {
        // Cleanup code here
    }()
}
```

---

## Summary

| Command | Purpose |
|---------|---------|
| `go test -race ./...` | Run all tests with race detector |
| `go test -race ./... -short` | Run unit tests only |
| `go test -race ./internal/proxy/... -v` | Test specific package |
| `make test` | Makefile: all tests |
| `make test-short` | Makefile: unit tests |
| `make e2e` | Start E2E environment |
| `make e2e-test` | Run E2E tests |

**Remember:**

1. TDD is mandatory - write tests first
2. Always use the race detector
3. Maintain 80%+ coverage for critical packages
4. Run tests before committing
