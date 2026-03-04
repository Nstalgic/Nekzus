package logger

import (
	"log/slog"
	"os"
)

// Component constants for consistent logging across the codebase
const (
	CompMain          = "main"
	CompFederation    = "federation"
	CompToolbox       = "toolbox"
	CompDiscovery     = "discovery"
	CompProxy         = "proxy"
	CompAuth          = "auth"
	CompNotifications = "notifications"
	CompHandlers      = "handlers"
	CompMiddleware    = "middleware"
	CompStorage       = "storage"
	CompHealth        = "health"
	CompRouter        = "router"
	CompWebSocket     = "websocket"
	CompDocker        = "docker"
	CompKubernetes    = "kubernetes"
	CompMDNS          = "mdns"
	CompConfig        = "config"
	CompMetrics       = "metrics"
	CompJobs          = "jobs"
	CompActivity      = "activity"
	CompCertManager   = "certmanager"
)

// Setup initializes the global slog logger with the given log level
func Setup(level slog.Level) *slog.Logger {
	opts := &slog.HandlerOptions{
		Level: level,
		// Add source location for debugging
		AddSource: false,
	}

	handler := slog.NewJSONHandler(os.Stdout, opts)
	logger := slog.New(handler)

	// Set as default logger
	slog.SetDefault(logger)

	return logger
}

// SetupText initializes the global logger with text output (for development)
func SetupText(level slog.Level) *slog.Logger {
	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: false,
	}

	handler := slog.NewTextHandler(os.Stdout, opts)
	logger := slog.New(handler)

	// Set as default logger
	slog.SetDefault(logger)

	return logger
}

// WithComponent creates a logger with package and component context
func WithComponent(pkg, comp string) *slog.Logger {
	return slog.With("package", pkg, "component", comp)
}

// WithPackage creates a logger with package context only
func WithPackage(pkg string) *slog.Logger {
	return slog.With("package", pkg)
}
