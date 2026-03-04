package health

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func TestStorageChecker(t *testing.T) {
	// Test with nil database (in-memory mode)
	checker := NewStorageChecker(nil)
	if checker == nil {
		t.Fatal("checker should not be nil")
	}

	if checker.Name() != "storage" {
		t.Errorf("expected name 'storage', got %s", checker.Name())
	}

	ctx := context.Background()
	health := checker.Check(ctx)

	if health.Status != StatusDegraded {
		t.Errorf("expected degraded status for nil db, got %s", health.Status)
	}

	// Test with real database
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	checker = NewStorageChecker(db)
	health = checker.Check(ctx)

	if health.Status != StatusHealthy {
		t.Errorf("expected healthy status, got %s: %s", health.Status, health.Message)
	}

	if health.Details == nil {
		t.Error("expected details to be present")
	}

	// Test with closed database
	db.Close()
	health = checker.Check(ctx)

	if health.Status != StatusUnhealthy {
		t.Errorf("expected unhealthy status for closed db, got %s", health.Status)
	}
}

func TestDiscoveryChecker(t *testing.T) {
	// Test disabled discovery
	checker := NewDiscoveryChecker(false, nil)
	if checker.Name() != "discovery" {
		t.Errorf("expected name 'discovery', got %s", checker.Name())
	}

	ctx := context.Background()
	health := checker.Check(ctx)

	if health.Status != StatusHealthy {
		t.Errorf("expected healthy status when disabled, got %s", health.Status)
	}

	// Test enabled with no workers
	workersActive := func() int { return 0 }
	checker = NewDiscoveryChecker(true, workersActive)
	health = checker.Check(ctx)

	if health.Status != StatusDegraded {
		t.Errorf("expected degraded status with no workers, got %s", health.Status)
	}

	// Test enabled with active workers
	workersActive = func() int { return 2 }
	checker = NewDiscoveryChecker(true, workersActive)
	health = checker.Check(ctx)

	if health.Status != StatusHealthy {
		t.Errorf("expected healthy status with active workers, got %s", health.Status)
	}

	if health.Details == nil {
		t.Error("expected details to be present")
	}

	activeWorkers, ok := health.Details["activeWorkers"]
	if !ok || activeWorkers != 2 {
		t.Errorf("expected activeWorkers=2, got %v", activeWorkers)
	}
}

func TestAuthChecker(t *testing.T) {
	// Test with nil validation function
	checker := NewAuthChecker(nil)
	if checker.Name() != "authentication" {
		t.Errorf("expected name 'authentication', got %s", checker.Name())
	}

	ctx := context.Background()
	health := checker.Check(ctx)

	if health.Status != StatusHealthy {
		t.Errorf("expected healthy status with nil validator, got %s", health.Status)
	}

	// Test with successful validation
	validateSuccess := func() error { return nil }
	checker = NewAuthChecker(validateSuccess)
	health = checker.Check(ctx)

	if health.Status != StatusHealthy {
		t.Errorf("expected healthy status, got %s", health.Status)
	}

	// Test with failed validation
	validateFail := func() error { return sql.ErrNoRows }
	checker = NewAuthChecker(validateFail)
	health = checker.Check(ctx)

	if health.Status != StatusUnhealthy {
		t.Errorf("expected unhealthy status, got %s", health.Status)
	}
}

func TestProxyCacheChecker(t *testing.T) {
	// Test with nil cache size function
	checker := NewProxyCacheChecker(nil)
	if checker.Name() != "proxy_cache" {
		t.Errorf("expected name 'proxy_cache', got %s", checker.Name())
	}

	ctx := context.Background()
	health := checker.Check(ctx)

	if health.Status != StatusHealthy {
		t.Errorf("expected healthy status, got %s", health.Status)
	}

	// Test with small cache
	cacheSize := func() int { return 10 }
	checker = NewProxyCacheChecker(cacheSize)
	health = checker.Check(ctx)

	if health.Status != StatusHealthy {
		t.Errorf("expected healthy status for small cache, got %s", health.Status)
	}

	// Test with large cache
	cacheSize = func() int { return 150 }
	checker = NewProxyCacheChecker(cacheSize)
	health = checker.Check(ctx)

	if health.Status != StatusDegraded {
		t.Errorf("expected degraded status for large cache, got %s", health.Status)
	}

	if health.Details == nil {
		t.Error("expected details to be present")
	}
}

func TestEventBusChecker(t *testing.T) {
	// Test with nil connections function
	checker := NewEventBusChecker(nil)
	if checker.Name() != "event_bus" {
		t.Errorf("expected name 'event_bus', got %s", checker.Name())
	}

	ctx := context.Background()
	health := checker.Check(ctx)

	if health.Status != StatusHealthy {
		t.Errorf("expected healthy status, got %s", health.Status)
	}

	// Test with normal connection count
	activeConnections := func() int { return 50 }
	checker = NewEventBusChecker(activeConnections)
	health = checker.Check(ctx)

	if health.Status != StatusHealthy {
		t.Errorf("expected healthy status, got %s", health.Status)
	}

	// Test with high connection count
	activeConnections = func() int { return 1500 }
	checker = NewEventBusChecker(activeConnections)
	health = checker.Check(ctx)

	if health.Status != StatusDegraded {
		t.Errorf("expected degraded status for high connections, got %s", health.Status)
	}

	if health.Details == nil {
		t.Error("expected details to be present")
	}
}

func TestMemoryChecker(t *testing.T) {
	checker := NewMemoryChecker(0)
	if checker.Name() != "memory" {
		t.Errorf("expected name 'memory', got %s", checker.Name())
	}

	ctx := context.Background()
	health := checker.Check(ctx)

	// With no limit, should always be healthy
	if health.Status != StatusHealthy {
		t.Errorf("expected healthy status with no limit, got %s", health.Status)
	}

	if health.Details == nil {
		t.Error("expected details to be present")
	}

	// Verify details contain memory info
	if _, ok := health.Details["allocMB"]; !ok {
		t.Error("expected allocMB in details")
	}

	if _, ok := health.Details["sysMB"]; !ok {
		t.Error("expected sysMB in details")
	}

	if _, ok := health.Details["numGC"]; !ok {
		t.Error("expected numGC in details")
	}

	if _, ok := health.Details["goroutines"]; !ok {
		t.Error("expected goroutines in details")
	}

	// Test with very low limit (should trigger degraded if exceeded)
	checker = NewMemoryChecker(1) // 1MB limit
	health = checker.Check(ctx)

	// Status depends on actual memory usage - just verify it's valid
	if health.Status != StatusHealthy && health.Status != StatusDegraded {
		t.Errorf("expected healthy or degraded status, got %s", health.Status)
	}

	// If memory is actually over limit, should be degraded
	allocMB, ok := health.Details["allocMB"]
	if ok {
		if allocMB.(uint64) > 1 && health.Status != StatusDegraded {
			t.Errorf("expected degraded status when allocMB=%d > 1, got %s", allocMB, health.Status)
		}
	}
}

func TestCheckerNames(t *testing.T) {
	checkers := []struct {
		checker Checker
		name    string
	}{
		{NewStorageChecker(nil), "storage"},
		{NewDiscoveryChecker(true, nil), "discovery"},
		{NewAuthChecker(nil), "authentication"},
		{NewProxyCacheChecker(nil), "proxy_cache"},
		{NewEventBusChecker(nil), "event_bus"},
		{NewMemoryChecker(0), "memory"},
	}

	for _, tc := range checkers {
		if tc.checker.Name() != tc.name {
			t.Errorf("expected name %s, got %s", tc.name, tc.checker.Name())
		}
	}
}

func TestCheckerContext(t *testing.T) {
	// Test that checkers respect context cancellation
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Create a database checker with timeout
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	checker := NewStorageChecker(db)
	health := checker.Check(ctx)

	// Should complete before timeout
	if health.Status == StatusUnknown {
		t.Error("check should complete before context timeout")
	}
}

func TestStorageCheckerConnectionPool(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	// Set connection pool limits
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)

	checker := NewStorageChecker(db)
	ctx := context.Background()
	health := checker.Check(ctx)

	if health.Status != StatusHealthy {
		t.Errorf("expected healthy status, got %s", health.Status)
	}

	// Verify details contain pool stats
	if health.Details == nil {
		t.Fatal("expected details to be present")
	}

	if _, ok := health.Details["maxOpenConnections"]; !ok {
		t.Error("expected maxOpenConnections in details")
	}
}

func TestCertificateChecker(t *testing.T) {
	ctx := context.Background()

	// Test with no certificates
	checker := NewCertificateChecker(func() []CertificateInfo {
		return []CertificateInfo{}
	})
	if checker.Name() != "certificates" {
		t.Errorf("expected name 'certificates', got %s", checker.Name())
	}

	health := checker.Check(ctx)
	if health.Status != StatusHealthy {
		t.Errorf("expected healthy status with no certs, got %s", health.Status)
	}

	// Test with healthy certificates (30+ days until expiry)
	now := time.Now()
	checker = NewCertificateChecker(func() []CertificateInfo {
		return []CertificateInfo{
			{Domain: "app1.local", NotAfter: now.Add(60 * 24 * time.Hour), Issuer: "self-signed"},
			{Domain: "app2.local", NotAfter: now.Add(90 * 24 * time.Hour), Issuer: "self-signed"},
		}
	})

	health = checker.Check(ctx)
	if health.Status != StatusHealthy {
		t.Errorf("expected healthy status with valid certs, got %s: %s", health.Status, health.Message)
	}

	if health.Details == nil {
		t.Fatal("expected details to be present")
	}

	totalCerts, ok := health.Details["totalCertificates"]
	if !ok || totalCerts != 2 {
		t.Errorf("expected totalCertificates=2, got %v", totalCerts)
	}

	// Test with certificate expiring soon (within 7-30 days) - degraded
	checker = NewCertificateChecker(func() []CertificateInfo {
		return []CertificateInfo{
			{Domain: "app1.local", NotAfter: now.Add(60 * 24 * time.Hour), Issuer: "self-signed"},
			{Domain: "expiring.local", NotAfter: now.Add(10 * 24 * time.Hour), Issuer: "self-signed"},
		}
	})

	health = checker.Check(ctx)
	if health.Status != StatusDegraded {
		t.Errorf("expected degraded status with cert expiring in 10 days, got %s", health.Status)
	}

	if _, ok := health.Details["expiringSoon"]; !ok {
		t.Error("expected expiringSoon in details")
	}

	// Test with certificate expiring very soon (within 7 days) - degraded with higher urgency
	checker = NewCertificateChecker(func() []CertificateInfo {
		return []CertificateInfo{
			{Domain: "urgent.local", NotAfter: now.Add(3 * 24 * time.Hour), Issuer: "self-signed"},
		}
	})

	health = checker.Check(ctx)
	if health.Status != StatusDegraded {
		t.Errorf("expected degraded status with cert expiring in 3 days, got %s", health.Status)
	}

	// Test with expired certificate - unhealthy
	checker = NewCertificateChecker(func() []CertificateInfo {
		return []CertificateInfo{
			{Domain: "expired.local", NotAfter: now.Add(-24 * time.Hour), Issuer: "self-signed"},
			{Domain: "valid.local", NotAfter: now.Add(60 * 24 * time.Hour), Issuer: "self-signed"},
		}
	})

	health = checker.Check(ctx)
	if health.Status != StatusUnhealthy {
		t.Errorf("expected unhealthy status with expired cert, got %s", health.Status)
	}

	expired, ok := health.Details["expired"]
	if !ok {
		t.Error("expected expired in details")
	}
	expiredList, ok := expired.([]string)
	if !ok || len(expiredList) != 1 {
		t.Errorf("expected 1 expired cert, got %v", expired)
	}

	// Test with multiple expired certificates
	checker = NewCertificateChecker(func() []CertificateInfo {
		return []CertificateInfo{
			{Domain: "expired1.local", NotAfter: now.Add(-24 * time.Hour), Issuer: "self-signed"},
			{Domain: "expired2.local", NotAfter: now.Add(-48 * time.Hour), Issuer: "self-signed"},
		}
	})

	health = checker.Check(ctx)
	if health.Status != StatusUnhealthy {
		t.Errorf("expected unhealthy status with multiple expired certs, got %s", health.Status)
	}

	expired, ok = health.Details["expired"]
	if !ok {
		t.Error("expected expired in details")
	}
	expiredList, ok = expired.([]string)
	if !ok || len(expiredList) != 2 {
		t.Errorf("expected 2 expired certs, got %v", expired)
	}

	// Test with nil certificate provider function
	checker = NewCertificateChecker(nil)
	health = checker.Check(ctx)
	if health.Status != StatusHealthy {
		t.Errorf("expected healthy status with nil provider, got %s", health.Status)
	}
}

func TestCertificateCheckerThresholds(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	tests := []struct {
		name            string
		daysUntilExpiry int
		expectedStatus  Status
	}{
		{"90 days - healthy", 90, StatusHealthy},
		{"31 days - healthy", 31, StatusHealthy},
		{"30 days - healthy", 30, StatusHealthy},
		{"29 days - degraded", 29, StatusDegraded},
		{"15 days - degraded", 15, StatusDegraded},
		{"7 days - degraded", 7, StatusDegraded},
		{"3 days - degraded", 3, StatusDegraded},
		{"1 day - degraded", 1, StatusDegraded},
		{"0 days (today) - unhealthy", 0, StatusUnhealthy},
		{"-1 day (expired) - unhealthy", -1, StatusUnhealthy},
		{"-10 days (expired) - unhealthy", -10, StatusUnhealthy},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := NewCertificateChecker(func() []CertificateInfo {
				return []CertificateInfo{
					{
						Domain:   "test.local",
						NotAfter: now.Add(time.Duration(tt.daysUntilExpiry) * 24 * time.Hour),
						Issuer:   "self-signed",
					},
				}
			})

			health := checker.Check(ctx)
			if health.Status != tt.expectedStatus {
				t.Errorf("expected status %s, got %s for %d days until expiry",
					tt.expectedStatus, health.Status, tt.daysUntilExpiry)
			}
		})
	}
}

func TestAllCheckersReturnValidStatus(t *testing.T) {
	ctx := context.Background()

	checkers := []Checker{
		NewStorageChecker(nil),
		NewDiscoveryChecker(true, func() int { return 1 }),
		NewAuthChecker(nil),
		NewProxyCacheChecker(func() int { return 5 }),
		NewEventBusChecker(func() int { return 10 }),
		NewMemoryChecker(1024),
		NewCertificateChecker(nil),
	}

	validStatuses := map[Status]bool{
		StatusHealthy:   true,
		StatusDegraded:  true,
		StatusUnhealthy: true,
		StatusUnknown:   true,
	}

	for _, checker := range checkers {
		health := checker.Check(ctx)

		if !validStatuses[health.Status] {
			t.Errorf("checker %s returned invalid status: %s", checker.Name(), health.Status)
		}

		if health.Message == "" {
			t.Errorf("checker %s returned empty message", checker.Name())
		}
	}
}
