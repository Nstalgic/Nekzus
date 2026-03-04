package handlers

import (
	"bufio"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	apperrors "github.com/nstalgic/nekzus/internal/errors"
	"github.com/nstalgic/nekzus/internal/httputil"
)

var syslog = slog.With("package", "handlers")

// SystemHandler handles system resource endpoints
type SystemHandler struct {
	cpuMutex      sync.Mutex
	lastCPUTotal  uint64
	lastCPUSystem uint64
	lastCPUTime   time.Time
	databasePath  string
	hostRootPath  string // Path to mounted host root (empty = use container metrics)

	// For host CPU delta calculation
	hostCPUMutex     sync.Mutex
	lastHostCPUTotal uint64
	lastHostCPUIdle  uint64
	lastHostCPUTime  time.Time
}

// NewSystemHandler creates a new system handler
// databasePath: path to the database file for storage size calculation
// hostRootPath: path to mounted host root (e.g., "/mnt/host"), empty for container metrics
func NewSystemHandler(databasePath, hostRootPath string) *SystemHandler {
	return &SystemHandler{
		lastCPUTime:     time.Now(),
		databasePath:    databasePath,
		hostRootPath:    hostRootPath,
		lastHostCPUTime: time.Now(),
	}
}

// SystemResourcesResponse represents system resource metrics
type SystemResourcesResponse struct {
	CPU         float64         `json:"cpu"`          // CPU usage percentage
	RAM         float64         `json:"ram"`          // RAM usage percentage
	RAMUsed     uint64          `json:"ram_used"`     // RAM used in bytes
	RAMTotal    uint64          `json:"ram_total"`    // RAM total in bytes
	Disk        float64         `json:"disk"`         // Disk usage percentage
	DiskUsed    uint64          `json:"disk_used"`    // Disk used in bytes
	DiskTotal   uint64          `json:"disk_total"`   // Disk total in bytes
	StorageSize int64           `json:"storage_size"` // Database file size in bytes
	Network     *NetworkMetrics `json:"network"`      // Network I/O (optional)
}

// NetworkMetrics represents network I/O statistics
type NetworkMetrics struct {
	RxBytes uint64 `json:"rx_bytes"` // Bytes received
	TxBytes uint64 `json:"tx_bytes"` // Bytes transmitted
}

// HandleSystemResources returns system resource usage statistics
// GET /api/v1/system/resources
func (h *SystemHandler) HandleSystemResources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apperrors.WriteJSON(w, apperrors.New("METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed))
		return
	}

	var cpuPercent float64
	var ramStats RAMStats
	var diskStats DiskStats

	if h.hostRootPath != "" {
		// Use host metrics from mounted /proc
		cpuPercent = h.getHostCPUPercent()
		ramStats = h.getHostRAMStats()
		diskStats = h.getHostDiskStats()
	} else {
		// Use container metrics (default)
		cpuPercent = h.getCPUPercent()
		ramStats = getRAMStats()
		diskStats = getDiskStats("/")
	}

	storageSize := h.getStorageSize()

	response := SystemResourcesResponse{
		CPU:         cpuPercent,
		RAM:         ramStats.Percent,
		RAMUsed:     ramStats.Used,
		RAMTotal:    ramStats.Total,
		Disk:        diskStats.Percent,
		DiskUsed:    diskStats.Used,
		DiskTotal:   diskStats.Total,
		StorageSize: storageSize,
		Network:     nil,
	}

	if err := httputil.WriteJSON(w, http.StatusOK, response); err != nil {
		syslog.Error("Error encoding JSON response", "error", err)
	}
}

// getCPUPercent calculates CPU usage percentage
// This uses a delta calculation between two measurements
func (h *SystemHandler) getCPUPercent() float64 {
	h.cpuMutex.Lock()
	defer h.cpuMutex.Unlock()

	// Read CPU times
	var rusage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &rusage); err != nil {
		syslog.Warn("Failed to get CPU usage", "error", err)
		return 0
	}

	// Convert timeval to nanoseconds
	userTime := uint64(rusage.Utime.Sec)*1e9 + uint64(rusage.Utime.Usec)*1e3
	systemTime := uint64(rusage.Stime.Sec)*1e9 + uint64(rusage.Stime.Usec)*1e3
	totalCPUTime := userTime + systemTime

	now := time.Now()
	elapsed := now.Sub(h.lastCPUTime)

	// Need at least two samples for delta calculation
	if h.lastCPUTime.IsZero() || elapsed < 100*time.Millisecond {
		h.lastCPUTotal = totalCPUTime
		h.lastCPUTime = now
		return 0
	}

	// Calculate CPU percentage
	cpuDelta := float64(totalCPUTime - h.lastCPUTotal)
	timeDelta := float64(elapsed.Nanoseconds())

	cpuPercent := (cpuDelta / timeDelta) * 100.0 * float64(runtime.NumCPU())

	// Update last values
	h.lastCPUTotal = totalCPUTime
	h.lastCPUTime = now

	// Clamp to 0-100 range
	if cpuPercent < 0 {
		cpuPercent = 0
	}
	if cpuPercent > 100 {
		cpuPercent = 100
	}

	return cpuPercent
}

// RAMStats holds RAM usage statistics
type RAMStats struct {
	Percent float64
	Used    uint64
	Total   uint64
}

// getRAMStats calculates RAM usage percentage and absolute values
func getRAMStats() RAMStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// For container metrics, use Go's memory stats
	// Sys = total memory obtained from the OS
	// Alloc = bytes allocated and still in use
	if m.Sys == 0 {
		return RAMStats{}
	}

	return RAMStats{
		Percent: float64(m.Alloc) / float64(m.Sys) * 100.0,
		Used:    m.Alloc,
		Total:   m.Sys,
	}
}

// getRAMPercent calculates RAM usage percentage (kept for compatibility)
func getRAMPercent() float64 {
	return getRAMStats().Percent
}

// DiskStats holds disk usage statistics
type DiskStats struct {
	Percent float64
	Used    uint64
	Total   uint64
}

// getDiskStats calculates disk usage percentage and absolute values for a given path
func getDiskStats(path string) DiskStats {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		syslog.Warn("Failed to get disk stats", "path", path, "error", err)
		return DiskStats{}
	}

	// Calculate total and used space
	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bfree * uint64(stat.Bsize)

	if total == 0 {
		return DiskStats{}
	}

	used := total - free
	return DiskStats{
		Percent: float64(used) / float64(total) * 100.0,
		Used:    used,
		Total:   total,
	}
}

// getDiskPercent calculates disk usage percentage for a given path (kept for compatibility)
func getDiskPercent(path string) float64 {
	return getDiskStats(path).Percent
}

// getStorageSize returns the database file size in bytes
func (h *SystemHandler) getStorageSize() int64 {
	if h.databasePath == "" || h.databasePath == ":memory:" {
		return 0
	}

	info, err := os.Stat(h.databasePath)
	if err != nil {
		syslog.Warn("Failed to get storage size", "path", h.databasePath, "error", err)
		return 0
	}

	return info.Size()
}

// getNetworkStats returns network I/O statistics
// Reads from /proc/net/dev on Linux, returns nil on other platforms
func getNetworkStats() *NetworkMetrics {
	if runtime.GOOS != "linux" {
		// Network stats only supported on Linux
		return nil
	}

	file, err := os.Open("/proc/net/dev")
	if err != nil {
		return nil
	}
	defer file.Close()

	var totalRx, totalTx uint64
	scanner := bufio.NewScanner(file)

	// Skip first two header lines
	scanner.Scan()
	scanner.Scan()

	// Parse each interface line
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)

		if len(fields) < 10 {
			continue
		}

		// Skip loopback interface
		if strings.HasPrefix(fields[0], "lo:") {
			continue
		}

		// fields[1] = rx_bytes, fields[9] = tx_bytes
		rxBytes, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}

		txBytes, err := strconv.ParseUint(fields[9], 10, 64)
		if err != nil {
			continue
		}

		totalRx += rxBytes
		totalTx += txBytes
	}

	if err := scanner.Err(); err != nil {
		return nil
	}

	return &NetworkMetrics{
		RxBytes: totalRx,
		TxBytes: totalTx,
	}
}

// --- Host Metrics Functions ---

// getHostCPUPercent reads CPU usage from host's /proc/stat
func (h *SystemHandler) getHostCPUPercent() float64 {
	h.hostCPUMutex.Lock()
	defer h.hostCPUMutex.Unlock()

	statPath := h.hostRootPath + "/proc/stat"
	data, err := os.ReadFile(statPath)
	if err != nil {
		syslog.Warn("Failed to read host /proc/stat", "path", statPath, "error", err)
		return 0
	}

	total, idle, err := parseProcStat(string(data))
	if err != nil {
		syslog.Warn("Failed to parse host /proc/stat", "error", err)
		return 0
	}

	now := time.Now()
	elapsed := now.Sub(h.lastHostCPUTime)

	// Need at least two samples for delta calculation
	if h.lastHostCPUTime.IsZero() || elapsed < 100*time.Millisecond {
		h.lastHostCPUTotal = total
		h.lastHostCPUIdle = idle
		h.lastHostCPUTime = now
		return 0
	}

	// Calculate CPU percentage from delta
	totalDelta := float64(total - h.lastHostCPUTotal)
	idleDelta := float64(idle - h.lastHostCPUIdle)

	var cpuPercent float64
	if totalDelta > 0 {
		cpuPercent = (1.0 - idleDelta/totalDelta) * 100.0
	}

	// Update last values
	h.lastHostCPUTotal = total
	h.lastHostCPUIdle = idle
	h.lastHostCPUTime = now

	// Clamp to 0-100 range
	if cpuPercent < 0 {
		cpuPercent = 0
	}
	if cpuPercent > 100 {
		cpuPercent = 100
	}

	return cpuPercent
}

// getHostRAMStats reads RAM usage from host's /proc/meminfo
func (h *SystemHandler) getHostRAMStats() RAMStats {
	meminfoPath := h.hostRootPath + "/proc/meminfo"
	data, err := os.ReadFile(meminfoPath)
	if err != nil {
		syslog.Warn("Failed to read host /proc/meminfo", "path", meminfoPath, "error", err)
		return RAMStats{}
	}

	stats, err := parseProcMeminfoStats(string(data))
	if err != nil {
		syslog.Warn("Failed to parse host /proc/meminfo", "error", err)
		return RAMStats{}
	}

	return stats
}

// getHostRAMPercent reads RAM usage from host's /proc/meminfo (kept for compatibility)
func (h *SystemHandler) getHostRAMPercent() float64 {
	return h.getHostRAMStats().Percent
}

// getHostDiskStats reads disk usage from host root mount
func (h *SystemHandler) getHostDiskStats() DiskStats {
	return getDiskStats(h.hostRootPath)
}

// getHostDiskPercent reads disk usage from host root mount (kept for compatibility)
func (h *SystemHandler) getHostDiskPercent() float64 {
	return h.getHostDiskStats().Percent
}

// parseProcStat parses /proc/stat content and returns total and idle CPU ticks
func parseProcStat(content string) (total, idle uint64, err error) {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "cpu ") {
			fields := strings.Fields(line)
			// cpu user nice system idle iowait irq softirq steal guest guest_nice
			if len(fields) < 5 {
				return 0, 0, fmt.Errorf("malformed cpu line: not enough fields")
			}

			// Parse values: user, nice, system, idle, iowait, irq, softirq, steal
			var values []uint64
			for i := 1; i < len(fields) && i <= 8; i++ {
				v, err := strconv.ParseUint(fields[i], 10, 64)
				if err != nil {
					return 0, 0, fmt.Errorf("failed to parse cpu value: %w", err)
				}
				values = append(values, v)
			}

			if len(values) < 4 {
				return 0, 0, fmt.Errorf("malformed cpu line: not enough values")
			}

			// total = user + nice + system + idle + iowait + irq + softirq + steal
			for _, v := range values {
				total += v
			}
			// idle is the 4th value (index 3)
			idle = values[3]

			return total, idle, nil
		}
	}

	return 0, 0, fmt.Errorf("no cpu line found in /proc/stat")
}

// parseProcMeminfo parses /proc/meminfo content and returns RAM stats
// Values in /proc/meminfo are in kB, so we convert to bytes
func parseProcMeminfo(content string) (float64, error) {
	stats, err := parseProcMeminfoStats(content)
	if err != nil {
		return 0, err
	}
	return stats.Percent, nil
}

// parseProcMeminfoStats parses /proc/meminfo content and returns full RAM stats
func parseProcMeminfoStats(content string) (RAMStats, error) {
	var memTotal, memAvailable uint64
	var foundTotal, foundAvailable bool

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				v, err := strconv.ParseUint(fields[1], 10, 64)
				if err == nil {
					memTotal = v
					foundTotal = true
				}
			}
		} else if strings.HasPrefix(line, "MemAvailable:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				v, err := strconv.ParseUint(fields[1], 10, 64)
				if err == nil {
					memAvailable = v
					foundAvailable = true
				}
			}
		}

		if foundTotal && foundAvailable {
			break
		}
	}

	if !foundTotal {
		return RAMStats{}, fmt.Errorf("MemTotal not found in /proc/meminfo")
	}
	if !foundAvailable {
		return RAMStats{}, fmt.Errorf("MemAvailable not found in /proc/meminfo")
	}
	if memTotal == 0 {
		return RAMStats{}, nil
	}

	// Values in /proc/meminfo are in kB, convert to bytes
	totalBytes := memTotal * 1024
	availableBytes := memAvailable * 1024
	usedBytes := totalBytes - availableBytes

	return RAMStats{
		Percent: float64(usedBytes) / float64(totalBytes) * 100.0,
		Used:    usedBytes,
		Total:   totalBytes,
	}, nil
}
