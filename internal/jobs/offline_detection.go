package jobs

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/nstalgic/nekzus/internal/storage"
)

var log = slog.With("package", "jobs")

// OfflineMetrics defines the interface for updating offline detection metrics
type OfflineMetrics interface {
	SetDevicesOnline(count float64)
	SetDevicesOffline(count float64)
}

// OfflineDetectionJob periodically checks device online/offline status
type OfflineDetectionJob struct {
	storage          *storage.Store
	metrics          OfflineMetrics
	offlineThreshold time.Duration
	interval         time.Duration

	mu      sync.Mutex
	running bool
	stop    chan struct{}
	done    chan struct{}
}

// NewOfflineDetectionJob creates a new offline detection job
func NewOfflineDetectionJob(storage *storage.Store, metrics OfflineMetrics, offlineThreshold, interval time.Duration) *OfflineDetectionJob {
	return &OfflineDetectionJob{
		storage:          storage,
		metrics:          metrics,
		offlineThreshold: offlineThreshold,
		interval:         interval,
		stop:             make(chan struct{}),
		done:             make(chan struct{}),
	}
}

// Run executes one iteration of offline detection
func (j *OfflineDetectionJob) Run() error {
	// Get all devices from storage
	devices, err := j.storage.ListDevices()
	if err != nil {
		return fmt.Errorf("failed to list devices: %w", err)
	}

	// Count online and offline devices
	now := time.Now()
	onlineCount := 0
	offlineCount := 0

	for _, device := range devices {
		age := now.Sub(device.LastSeen)
		if age <= j.offlineThreshold {
			onlineCount++
		} else {
			offlineCount++
		}
	}

	// Update metrics
	j.metrics.SetDevicesOnline(float64(onlineCount))
	j.metrics.SetDevicesOffline(float64(offlineCount))

	return nil
}

// Start begins the periodic offline detection job
func (j *OfflineDetectionJob) Start() error {
	j.mu.Lock()
	defer j.mu.Unlock()

	if j.running {
		return fmt.Errorf("offline detection job is already running")
	}

	j.running = true
	j.stop = make(chan struct{})
	j.done = make(chan struct{})

	go j.run()

	return nil
}

// Stop stops the periodic offline detection job
func (j *OfflineDetectionJob) Stop() error {
	j.mu.Lock()
	if !j.running {
		j.mu.Unlock()
		return fmt.Errorf("offline detection job is not running")
	}
	j.mu.Unlock()

	// Signal stop
	close(j.stop)

	// Wait for job to finish
	<-j.done

	j.mu.Lock()
	j.running = false
	j.mu.Unlock()

	return nil
}

// run is the main loop for periodic offline detection
func (j *OfflineDetectionJob) run() {
	defer close(j.done)

	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()

	// Run immediately on start
	if err := j.Run(); err != nil {
		log.Error("Offline detection: error during initial run", "error", err)
	}

	// Then run periodically
	for {
		select {
		case <-ticker.C:
			if err := j.Run(); err != nil {
				log.Error("Offline detection: error during periodic run", "error", err)
			}
		case <-j.stop:
			return
		}
	}
}
