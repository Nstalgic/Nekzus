package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"testing"
)

// TestHandleSystemResources tests the system resources endpoint
func TestHandleSystemResources(t *testing.T) {
	handler := NewSystemHandler("", "")

	tests := []struct {
		name           string
		method         string
		expectedStatus int
		checkResponse  func(t *testing.T, body map[string]interface{})
	}{
		{
			name:           "successful GET request",
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body map[string]interface{}) {
				// Check that required fields are present
				if _, ok := body["cpu"]; !ok {
					t.Error("Expected 'cpu' field in response")
				}
				if _, ok := body["ram"]; !ok {
					t.Error("Expected 'ram' field in response")
				}
				if _, ok := body["disk"]; !ok {
					t.Error("Expected 'disk' field in response")
				}
				if _, ok := body["storage_size"]; !ok {
					t.Error("Expected 'storage_size' field in response")
				}

				// Check that values are reasonable percentages
				if cpu, ok := body["cpu"].(float64); ok {
					if cpu < 0 || cpu > 100 {
						t.Errorf("CPU percentage out of range: %.2f", cpu)
					}
				}
				if ram, ok := body["ram"].(float64); ok {
					if ram < 0 || ram > 100 {
						t.Errorf("RAM percentage out of range: %.2f", ram)
					}
				}
				// Check that storage_size is a non-negative number
				if storageSize, ok := body["storage_size"].(float64); ok {
					if storageSize < 0 {
						t.Errorf("Storage size should be non-negative: %.0f", storageSize)
					}
				}

				// Check for absolute memory values
				if _, ok := body["ram_used"]; !ok {
					t.Error("Expected 'ram_used' field in response")
				}
				if _, ok := body["ram_total"]; !ok {
					t.Error("Expected 'ram_total' field in response")
				}
				if ramUsed, ok := body["ram_used"].(float64); ok {
					if ramUsed < 0 {
						t.Errorf("RAM used should be non-negative: %.0f", ramUsed)
					}
				}
				if ramTotal, ok := body["ram_total"].(float64); ok {
					if ramTotal < 0 {
						t.Errorf("RAM total should be non-negative: %.0f", ramTotal)
					}
				}

				// Check for absolute disk values
				if _, ok := body["disk_used"]; !ok {
					t.Error("Expected 'disk_used' field in response")
				}
				if _, ok := body["disk_total"]; !ok {
					t.Error("Expected 'disk_total' field in response")
				}
				if diskUsed, ok := body["disk_used"].(float64); ok {
					if diskUsed < 0 {
						t.Errorf("Disk used should be non-negative: %.0f", diskUsed)
					}
				}
				if diskTotal, ok := body["disk_total"].(float64); ok {
					if diskTotal < 0 {
						t.Errorf("Disk total should be non-negative: %.0f", diskTotal)
					}
				}
			},
		},
		{
			name:           "POST method not allowed",
			method:         http.MethodPost,
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "PUT method not allowed",
			method:         http.MethodPut,
			expectedStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/api/v1/system/resources", nil)
			w := httptest.NewRecorder()

			handler.HandleSystemResources(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedStatus == http.StatusOK && tt.checkResponse != nil {
				var response map[string]interface{}
				if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				tt.checkResponse(t, response)
			}
		})
	}
}

// TestGetCPUPercent tests CPU percentage calculation
func TestGetCPUPercent(t *testing.T) {
	handler := NewSystemHandler("", "")

	// First call to initialize
	_ = handler.getCPUPercent()

	// Second call to get actual measurement
	cpuPercent := handler.getCPUPercent()

	if cpuPercent < 0 || cpuPercent > 100 {
		t.Errorf("CPU percentage out of range: %.2f", cpuPercent)
	}
}

// TestGetRAMPercent tests RAM percentage calculation
func TestGetRAMPercent(t *testing.T) {
	ramPercent := getRAMPercent()

	if ramPercent < 0 || ramPercent > 100 {
		t.Errorf("RAM percentage out of range: %.2f", ramPercent)
	}

	// Verify it returns reasonable values
	// In tests, RAM usage should be less than 95%
	if ramPercent > 95 {
		t.Logf("Warning: RAM usage is very high: %.2f%%", ramPercent)
	}
}

// TestGetDiskPercent tests disk percentage calculation
func TestGetDiskPercent(t *testing.T) {
	// Test with root path
	diskPercent := getDiskPercent("/")

	if diskPercent < 0 || diskPercent > 100 {
		t.Errorf("Disk percentage out of range: %.2f", diskPercent)
	}

	// Test with invalid path
	invalidDisk := getDiskPercent("/nonexistent/path/that/does/not/exist")
	if invalidDisk != 0 {
		t.Errorf("Expected 0 for invalid path, got %.2f", invalidDisk)
	}
}

// BenchmarkSystemResources benchmarks the system resources endpoint
func BenchmarkSystemResources(b *testing.B) {
	handler := NewSystemHandler("", "")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/resources", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		handler.HandleSystemResources(w, req)
	}
}

// TestSystemResourcesConcurrency tests concurrent requests
func TestSystemResourcesConcurrency(t *testing.T) {
	handler := NewSystemHandler("", "")

	// Run 100 concurrent requests
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func() {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/system/resources", nil)
			w := httptest.NewRecorder()
			handler.HandleSystemResources(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Expected status 200, got %d", w.Code)
			}
			done <- true
		}()
	}

	// Wait for all requests to complete
	for i := 0; i < 100; i++ {
		<-done
	}
}

// TestGetStorageSize tests storage size calculation
func TestGetStorageSize(t *testing.T) {
	// Test with empty path
	handler := NewSystemHandler("", "")
	size := handler.getStorageSize()
	if size != 0 {
		t.Errorf("Expected 0 for empty path, got %d", size)
	}

	// Test with in-memory database
	handler = NewSystemHandler(":memory:", "")
	size = handler.getStorageSize()
	if size != 0 {
		t.Errorf("Expected 0 for :memory: path, got %d", size)
	}

	// Test with non-existent file
	handler = NewSystemHandler("/nonexistent/path/test.db", "")
	size = handler.getStorageSize()
	if size != 0 {
		t.Errorf("Expected 0 for non-existent file, got %d", size)
	}

	// Test with a real file
	tmpFile, err := os.CreateTemp("", "test-*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write some data to the file
	testData := []byte("test data for storage size calculation")
	if _, err := tmpFile.Write(testData); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	handler = NewSystemHandler(tmpFile.Name(), "")
	size = handler.getStorageSize()
	if size != int64(len(testData)) {
		t.Errorf("Expected size %d, got %d", len(testData), size)
	}
}

// TestMemoryStats tests memory stats directly
func TestMemoryStats(t *testing.T) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Verify we can read memory stats
	if m.Sys == 0 {
		t.Error("Expected non-zero system memory")
	}

	if m.Alloc > m.Sys {
		t.Error("Allocated memory should not exceed system memory")
	}
}

// TestParseProcStat tests parsing of /proc/stat content
func TestParseProcStat(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantErr  bool
		validate func(t *testing.T, total, idle uint64)
	}{
		{
			name: "valid /proc/stat content",
			content: `cpu  10132153 290696 3084719 46828483 16683 0 25195 0 0 0
cpu0 1393280 32966 572056 13343292 6130 0 17875 0 0 0
`,
			wantErr: false,
			validate: func(t *testing.T, total, idle uint64) {
				// user + nice + system + idle + iowait + irq + softirq + steal
				expectedTotal := uint64(10132153 + 290696 + 3084719 + 46828483 + 16683 + 0 + 25195 + 0)
				expectedIdle := uint64(46828483)
				if total != expectedTotal {
					t.Errorf("Expected total %d, got %d", expectedTotal, total)
				}
				if idle != expectedIdle {
					t.Errorf("Expected idle %d, got %d", expectedIdle, idle)
				}
			},
		},
		{
			name:    "empty content",
			content: "",
			wantErr: true,
		},
		{
			name:    "no cpu line",
			content: "some random content\n",
			wantErr: true,
		},
		{
			name: "malformed cpu line - not enough fields",
			content: `cpu  100 200
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			total, idle, err := parseProcStat(tt.content)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseProcStat() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.validate != nil {
				tt.validate(t, total, idle)
			}
		})
	}
}

// TestParseProcMeminfo tests parsing of /proc/meminfo content
func TestParseProcMeminfo(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantErr     bool
		wantPercent float64
	}{
		{
			name: "valid /proc/meminfo content",
			content: `MemTotal:       16384000 kB
MemFree:         1234567 kB
MemAvailable:    8192000 kB
Buffers:          123456 kB
Cached:          2345678 kB
`,
			wantErr:     false,
			wantPercent: 50.0, // (16384000 - 8192000) / 16384000 * 100
		},
		{
			name: "high memory usage",
			content: `MemTotal:       16000000 kB
MemAvailable:    1600000 kB
`,
			wantErr:     false,
			wantPercent: 90.0, // (16000000 - 1600000) / 16000000 * 100
		},
		{
			name:    "empty content",
			content: "",
			wantErr: true,
		},
		{
			name: "missing MemTotal",
			content: `MemAvailable:    8192000 kB
`,
			wantErr: true,
		},
		{
			name: "missing MemAvailable",
			content: `MemTotal:       16384000 kB
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			percent, err := parseProcMeminfo(tt.content)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseProcMeminfo() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				// Allow small floating point difference
				if percent < tt.wantPercent-0.1 || percent > tt.wantPercent+0.1 {
					t.Errorf("parseProcMeminfo() = %.2f, want %.2f", percent, tt.wantPercent)
				}
			}
		})
	}
}

// TestNewSystemHandlerWithHostRoot tests constructor with host root path
func TestNewSystemHandlerWithHostRoot(t *testing.T) {
	// Without host root (container metrics)
	handler := NewSystemHandler("", "")
	if handler.hostRootPath != "" {
		t.Errorf("Expected empty hostRootPath, got %s", handler.hostRootPath)
	}

	// With host root (host metrics)
	handler = NewSystemHandler("", "/mnt/host")
	if handler.hostRootPath != "/mnt/host" {
		t.Errorf("Expected hostRootPath '/mnt/host', got %s", handler.hostRootPath)
	}
}

// TestHostMetricsFallback tests that container metrics are used when host path is empty
func TestHostMetricsFallback(t *testing.T) {
	// Handler without host root path should use container metrics
	handler := NewSystemHandler("", "")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/resources", nil)
	w := httptest.NewRecorder()

	handler.HandleSystemResources(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Verify we get valid metrics
	if cpu, ok := response["cpu"].(float64); !ok || cpu < 0 {
		t.Errorf("Expected valid CPU metric, got %v", response["cpu"])
	}
	if ram, ok := response["ram"].(float64); !ok || ram < 0 {
		t.Errorf("Expected valid RAM metric, got %v", response["ram"])
	}
}

// TestHostMetricsWithMountedProc tests reading from a simulated host /proc
func TestHostMetricsWithMountedProc(t *testing.T) {
	// Create a temporary directory structure simulating /mnt/host
	tmpDir, err := os.MkdirTemp("", "host-metrics-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create proc directory
	procDir := tmpDir + "/proc"
	if err := os.MkdirAll(procDir, 0755); err != nil {
		t.Fatalf("Failed to create proc dir: %v", err)
	}

	// Create /proc/stat
	statContent := `cpu  10132153 290696 3084719 46828483 16683 0 25195 0 0 0
cpu0 1393280 32966 572056 13343292 6130 0 17875 0 0 0
`
	if err := os.WriteFile(procDir+"/stat", []byte(statContent), 0644); err != nil {
		t.Fatalf("Failed to write stat file: %v", err)
	}

	// Create /proc/meminfo
	meminfoContent := `MemTotal:       16384000 kB
MemFree:         1234567 kB
MemAvailable:    8192000 kB
Buffers:          123456 kB
Cached:          2345678 kB
`
	if err := os.WriteFile(procDir+"/meminfo", []byte(meminfoContent), 0644); err != nil {
		t.Fatalf("Failed to write meminfo file: %v", err)
	}

	// Create handler with host root path
	handler := NewSystemHandler("", tmpDir)

	// Test RAM reading (doesn't require delta)
	ramPercent := handler.getHostRAMPercent()
	expectedRAM := 50.0 // (16384000 - 8192000) / 16384000 * 100
	if ramPercent < expectedRAM-0.1 || ramPercent > expectedRAM+0.1 {
		t.Errorf("Expected RAM %.2f%%, got %.2f%%", expectedRAM, ramPercent)
	}

	// Test CPU reading (first call initializes, second gets delta)
	_ = handler.getHostCPUPercent() // Initialize

	// Update stat file with new values (simulating CPU usage)
	statContent2 := `cpu  10232153 290696 3184719 46928483 16683 0 25195 0 0 0
cpu0 1393280 32966 572056 13343292 6130 0 17875 0 0 0
`
	if err := os.WriteFile(procDir+"/stat", []byte(statContent2), 0644); err != nil {
		t.Fatalf("Failed to write updated stat file: %v", err)
	}

	cpuPercent := handler.getHostCPUPercent()
	// CPU should be between 0 and 100
	if cpuPercent < 0 || cpuPercent > 100 {
		t.Errorf("CPU percentage out of range: %.2f", cpuPercent)
	}

	// Test disk reading
	diskPercent := handler.getHostDiskPercent()
	if diskPercent < 0 || diskPercent > 100 {
		t.Errorf("Disk percentage out of range: %.2f", diskPercent)
	}
}
