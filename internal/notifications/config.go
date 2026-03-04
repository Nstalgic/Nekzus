package notifications

import (
	"time"

	"github.com/nstalgic/nekzus/internal/types"
)

// ServiceConfig holds configuration for the notification service
type ServiceConfig struct {
	// DefaultTTL is the default time-to-live for notifications
	DefaultTTL time.Duration

	// DefaultMaxRetries is the default number of retry attempts
	DefaultMaxRetries int

	// TypeConfigs holds per-message-type configuration
	TypeConfigs map[string]TypeConfig
}

// TypeConfig holds configuration for a specific notification type
type TypeConfig struct {
	// TTL is the time-to-live for this notification type
	TTL time.Duration

	// MaxRetries is the maximum number of delivery attempts
	MaxRetries int
}

// Stale notification threshold - notifications pending longer than this
// will trigger a warning in the Web UI
const StaleNotificationThreshold = 24 * time.Hour

// DefaultServiceConfig returns the default notification service configuration
// TTLs are set to 30 days by default since mobile devices may be offline for extended periods.
// Stale notification warnings appear in the Web UI after 24 hours.
func DefaultServiceConfig() ServiceConfig {
	return ServiceConfig{
		DefaultTTL:        30 * 24 * time.Hour, // 30 days - mobile may be offline for extended periods
		DefaultMaxRetries: 10,
		TypeConfigs: map[string]TypeConfig{
			// Critical: Device must re-pair after TLS upgrade
			types.WSMsgTypeRepairRequired: {
				TTL:        30 * 24 * time.Hour, // 30 days
				MaxRetries: 20,
			},

			// Critical: Device needs to know it's been revoked
			types.WSMsgTypeDeviceRevoked: {
				TTL:        30 * 24 * time.Hour, // 30 days
				MaxRetries: 10,
			},

			// Informational: Other devices learn about new pairing
			types.WSMsgTypeDevicePaired: {
				TTL:        30 * 24 * time.Hour, // 30 days
				MaxRetries: 10,
			},

			// Health status change
			types.WSMsgTypeHealthChange: {
				TTL:        7 * 24 * time.Hour, // 7 days - less critical
				MaxRetries: 5,
			},

			// Security: Port exposure warning
			types.WSMsgTypePortExposure: {
				TTL:        30 * 24 * time.Hour, // 30 days
				MaxRetries: 10,
			},
		},
	}
}

// GetTypeConfig returns the configuration for a specific message type,
// falling back to defaults if the type is not explicitly configured.
func (c ServiceConfig) GetTypeConfig(msgType string) TypeConfig {
	if cfg, ok := c.TypeConfigs[msgType]; ok {
		return cfg
	}
	return TypeConfig{
		TTL:        c.DefaultTTL,
		MaxRetries: c.DefaultMaxRetries,
	}
}
