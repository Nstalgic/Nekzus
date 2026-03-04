package backup

import (
	"fmt"
	"log/slog"
	"sync"
	"time"
)

var log = slog.With("package", "backup")

// Scheduler handles automatic periodic backups
type Scheduler struct {
	manager    *Manager
	interval   time.Duration
	retention  int // Number of backups to keep
	mu         sync.Mutex
	running    bool
	stop       chan struct{}
	done       chan struct{}
	lastBackup *time.Time
	nextBackup *time.Time
}

// SchedulerStatus represents the current status of the backup scheduler
type SchedulerStatus struct {
	Running        bool       `json:"running"`
	Interval       string     `json:"interval"`
	Retention      int        `json:"retention"`
	LastBackupTime *time.Time `json:"last_backup_time,omitempty"`
	NextBackupTime *time.Time `json:"next_backup_time,omitempty"`
	BackupCount    int        `json:"backup_count"`
}

// NewScheduler creates a new backup scheduler
func NewScheduler(manager *Manager, interval time.Duration, retention int) *Scheduler {
	return &Scheduler{
		manager:   manager,
		interval:  interval,
		retention: retention,
		stop:      make(chan struct{}),
		done:      make(chan struct{}),
	}
}

// Start begins the periodic backup scheduler
func (s *Scheduler) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("backup scheduler is already running")
	}

	s.running = true
	s.stop = make(chan struct{})
	s.done = make(chan struct{})

	go s.run()

	log.Info("Backup scheduler started", "interval", s.interval, "retention", s.retention)
	return nil
}

// Stop stops the periodic backup scheduler
func (s *Scheduler) Stop() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return fmt.Errorf("backup scheduler is not running")
	}
	s.mu.Unlock()

	// Signal stop
	close(s.stop)

	// Wait for scheduler to finish
	<-s.done

	s.mu.Lock()
	s.running = false
	s.mu.Unlock()

	log.Info("Backup scheduler stopped")
	return nil
}

// run is the main scheduler loop
func (s *Scheduler) run() {
	defer close(s.done)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	// Run first backup immediately
	if err := s.performBackup(); err != nil {
		log.Error("Backup scheduler: error during initial backup", "error", err)
	}

	// Then run periodically
	for {
		select {
		case <-ticker.C:
			if err := s.performBackup(); err != nil {
				log.Error("Backup scheduler: error during periodic backup", "error", err)
			}
		case <-s.stop:
			return
		}
	}
}

// performBackup creates a backup and applies retention policy
func (s *Scheduler) performBackup() error {
	now := time.Now()

	// Create backup
	description := fmt.Sprintf("Automatic backup at %s", now.Format("2006-01-02 15:04:05"))
	snapshot, err := s.manager.CreateBackup(description)
	if err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	// Save to disk
	if err := s.manager.SaveBackup(snapshot); err != nil {
		return fmt.Errorf("failed to save backup: %w", err)
	}

	log.Info("Backup created", "id", snapshot.ID)

	// Update last backup time
	s.mu.Lock()
	s.lastBackup = &now
	next := now.Add(s.interval)
	s.nextBackup = &next
	s.mu.Unlock()

	// Apply retention policy
	if s.retention > 0 {
		deleted, err := s.manager.CleanupOldBackups(s.retention)
		if err != nil {
			log.Error("Backup scheduler: failed to cleanup old backups", "error", err)
		} else if deleted > 0 {
			log.Info("Backup scheduler: cleaned up old backups", "count", deleted)
		}
	}

	return nil
}

// TriggerBackup manually triggers a backup (outside of the schedule)
func (s *Scheduler) TriggerBackup(description string) (*Snapshot, error) {
	now := time.Now()

	// Create backup
	snapshot, err := s.manager.CreateBackup(description)
	if err != nil {
		return nil, fmt.Errorf("failed to create backup: %w", err)
	}

	// Save to disk
	if err := s.manager.SaveBackup(snapshot); err != nil {
		return nil, fmt.Errorf("failed to save backup: %w", err)
	}

	log.Info("Manual backup created", "id", snapshot.ID)

	// Update last backup time
	s.mu.Lock()
	s.lastBackup = &now
	s.mu.Unlock()

	return snapshot, nil
}

// Status returns the current status of the scheduler
func (s *Scheduler) Status() SchedulerStatus {
	s.mu.Lock()
	defer s.mu.Unlock()

	backups, _ := s.manager.ListBackups()

	return SchedulerStatus{
		Running:        s.running,
		Interval:       s.interval.String(),
		Retention:      s.retention,
		LastBackupTime: s.lastBackup,
		NextBackupTime: s.nextBackup,
		BackupCount:    len(backups),
	}
}
