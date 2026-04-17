package main

import (
	"net"
	"os"
	"testing"
	"time"
)

// requireE2EEnvironment skips the calling test unless the environment is
// explicitly prepared for E2E runs. A test is skipped when any of these
// are true:
//
//   - go test -short is in effect (matches CI, which uses -short)
//   - NEKZUS_E2E is unset (opt-in gate for local runs)
//   - A quick TCP probe against addr fails (no server to talk to)
//
// With this helper, `go test ./...` passes on any dev machine or pipeline
// that doesn't bring up the demo stack. To actually exercise the E2E paths:
//
//	make demo                                    # start stack on :8443
//	NEKZUS_E2E=1 go test -run TestEndToEnd ./cmd/nekzus/
//	# or
//	make test-e2e
func requireE2EEnvironment(t *testing.T, addr string) {
	t.Helper()

	if testing.Short() {
		t.Skip("e2e: -short mode")
	}

	if os.Getenv("NEKZUS_E2E") == "" {
		t.Skip("e2e: set NEKZUS_E2E=1 (and 'make demo') to run this test")
	}

	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		t.Skipf("e2e: cannot reach %s (%v) — is nekzus running? Try 'make demo'", addr, err)
	}
	_ = conn.Close()
}
