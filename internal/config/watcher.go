package config

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/nstalgic/nekzus/internal/types"
)

var watcherLog = slog.With("package", "config.watcher")

// ReloadHandler is called when configuration is successfully reloaded
type ReloadHandler func(oldConfig, newConfig types.ServerConfig) error

// Watcher watches configuration files for changes and triggers reloads
type Watcher struct {
	configPath     string
	currentConfig  types.ServerConfig
	fsWatcher      *fsnotify.Watcher
	handlers       []ReloadHandler
	metrics        WatcherMetrics
	mu             sync.RWMutex // Protects currentConfig, handlers, debounceTimer
	reloadMu       sync.Mutex   // Ensures only one reload runs at a time
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	debounceTimer  *time.Timer
	debouncePeriod time.Duration
}

// WatcherMetrics defines the interface for recording config watcher metrics
type WatcherMetrics interface {
	RecordConfigReload(status string, duration time.Duration)
	IncrementConfigReloadTotal(status string)
}

// NewWatcher creates a new configuration file watcher
func NewWatcher(configPath string, initialConfig types.ServerConfig, metrics WatcherMetrics) (*Watcher, error) {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	w := &Watcher{
		configPath:     configPath,
		currentConfig:  initialConfig,
		fsWatcher:      fsWatcher,
		handlers:       []ReloadHandler{},
		metrics:        metrics,
		ctx:            ctx,
		cancel:         cancel,
		debouncePeriod: 500 * time.Millisecond, // Debounce rapid file changes
	}

	return w, nil
}

// RegisterReloadHandler adds a handler to be called on successful config reload
func (w *Watcher) RegisterReloadHandler(handler ReloadHandler) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.handlers = append(w.handlers, handler)
}

// Start begins watching the configuration file for changes
func (w *Watcher) Start() error {
	// Watch the config file directory (not the file itself, as editors may replace it)
	configDir := filepath.Dir(w.configPath)
	if err := w.fsWatcher.Add(configDir); err != nil {
		return fmt.Errorf("failed to watch config directory: %w", err)
	}

	watcherLog.Info("Config watcher started", "path", w.configPath)

	w.wg.Add(1)
	go w.watchLoop()

	return nil
}

// Stop stops the configuration watcher
func (w *Watcher) Stop() error {
	watcherLog.Info("Stopping config watcher")
	w.cancel()

	// Stop debounce timer if active
	w.mu.Lock()
	if w.debounceTimer != nil {
		w.debounceTimer.Stop()
	}
	w.mu.Unlock()

	if err := w.fsWatcher.Close(); err != nil {
		watcherLog.Error("Error closing file watcher", "error", err)
	}

	w.wg.Wait()
	watcherLog.Info("Config watcher stopped")
	return nil
}

// watchLoop monitors file system events
func (w *Watcher) watchLoop() {
	defer w.wg.Done()

	for {
		select {
		case <-w.ctx.Done():
			return

		case event, ok := <-w.fsWatcher.Events:
			if !ok {
				return
			}

			// Only process events for our config file
			if filepath.Clean(event.Name) != filepath.Clean(w.configPath) {
				continue
			}

			// Debounce rapid changes (editors often write multiple times)
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				w.debounceReload()
			}

		case err, ok := <-w.fsWatcher.Errors:
			if !ok {
				return
			}
			watcherLog.Error("Config watcher error", "error", err)
		}
	}
}

// debounceReload prevents multiple rapid reloads
func (w *Watcher) debounceReload() {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Stop existing timer if present
	if w.debounceTimer != nil {
		w.debounceTimer.Stop()
	}

	// Start new timer
	w.debounceTimer = time.AfterFunc(w.debouncePeriod, func() {
		if err := w.reload(); err != nil {
			watcherLog.Error("Config reload failed", "error", err)
			if w.metrics != nil {
				w.metrics.IncrementConfigReloadTotal("error")
			}
		}
	})
}

// reload loads and applies new configuration
func (w *Watcher) reload() error {
	// Prevent concurrent reloads - only one reload at a time
	w.reloadMu.Lock()
	defer w.reloadMu.Unlock()

	start := time.Now()

	watcherLog.Info("Config change detected, reloading")

	// Load new configuration
	newConfig, err := Load(w.configPath)
	if err != nil {
		watcherLog.Error("Failed to load new config", "error", err)
		if w.metrics != nil {
			w.metrics.RecordConfigReload("error_load", time.Since(start))
		}
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Validate that critical settings haven't changed
	w.mu.RLock()
	oldConfig := w.currentConfig
	w.mu.RUnlock()

	if err := w.validateReload(oldConfig, newConfig); err != nil {
		watcherLog.Error("Config validation failed", "error", err)
		if w.metrics != nil {
			w.metrics.RecordConfigReload("error_validation", time.Since(start))
		}
		return fmt.Errorf("config validation failed: %w", err)
	}

	// Call all registered handlers
	w.mu.RLock()
	handlers := make([]ReloadHandler, len(w.handlers))
	copy(handlers, w.handlers)
	w.mu.RUnlock()

	for _, handler := range handlers {
		if err := handler(oldConfig, newConfig); err != nil {
			watcherLog.Error("Reload handler failed", "error", err)
			if w.metrics != nil {
				w.metrics.RecordConfigReload("error_handler", time.Since(start))
			}
			return fmt.Errorf("handler failed: %w", err)
		}
	}

	// Update current config
	w.mu.Lock()
	w.currentConfig = newConfig
	w.mu.Unlock()

	duration := time.Since(start)
	watcherLog.Info("Config reloaded successfully", "duration", duration)

	if w.metrics != nil {
		w.metrics.RecordConfigReload("success", duration)
		w.metrics.IncrementConfigReloadTotal("success")
	}

	return nil
}

// validateReload ensures that non-reloadable settings haven't changed
func (w *Watcher) validateReload(old, new types.ServerConfig) error {
	// Check critical settings that require restart
	if old.Server.Addr != new.Server.Addr {
		return fmt.Errorf("server.addr cannot be changed without restart (old=%s, new=%s)",
			old.Server.Addr, new.Server.Addr)
	}

	if old.Server.TLSCert != new.Server.TLSCert {
		return fmt.Errorf("server.tls_cert cannot be changed without restart")
	}

	if old.Server.TLSKey != new.Server.TLSKey {
		return fmt.Errorf("server.tls_key cannot be changed without restart")
	}

	if old.Auth.HS256Secret != new.Auth.HS256Secret {
		return fmt.Errorf("auth.hs256_secret cannot be changed without restart (would invalidate existing tokens)")
	}

	if old.Auth.Issuer != new.Auth.Issuer {
		return fmt.Errorf("auth.issuer cannot be changed without restart")
	}

	if old.Auth.Audience != new.Auth.Audience {
		return fmt.Errorf("auth.audience cannot be changed without restart")
	}

	if old.Storage.DatabasePath != new.Storage.DatabasePath {
		return fmt.Errorf("storage.database_path cannot be changed without restart")
	}

	// Metrics path cannot be changed (endpoint is registered at startup)
	// But enabled flag can be changed (it's just a runtime toggle)
	if old.Metrics.Path != new.Metrics.Path {
		return fmt.Errorf("metrics.path cannot be changed without restart")
	}

	return nil
}

// GetCurrentConfig returns the current configuration (thread-safe)
func (w *Watcher) GetCurrentConfig() types.ServerConfig {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.currentConfig
}
