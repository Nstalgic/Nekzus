package audit

import (
	"encoding/json"
	"log/slog"
	"time"

	"github.com/nstalgic/nekzus/internal/storage"
)

var log = slog.With("package", "audit")

// Action represents different types of auditable actions
type Action string

const (
	ActionDevicePaired      Action = "device.paired"
	ActionDeviceRevoked     Action = "device.revoked"
	ActionDeviceUpdated     Action = "device.updated"
	ActionAppRegistered     Action = "app.registered"
	ActionAppDeleted        Action = "app.deleted"
	ActionRouteCreated      Action = "route.created"
	ActionRouteDeleted      Action = "route.deleted"
	ActionConfigReloaded    Action = "config.reloaded"
	ActionProposalApproved  Action = "proposal.approved"
	ActionProposalDismissed Action = "proposal.dismissed"
)

// Event represents an audit log event
type Event struct {
	Timestamp time.Time              `json:"timestamp"`
	Action    Action                 `json:"action"`
	ActorID   string                 `json:"actorId"`         // device ID or "system"
	ActorIP   string                 `json:"actorIp"`         // IP address of actor
	TargetID  string                 `json:"targetId"`        // ID of affected resource
	Details   map[string]interface{} `json:"details"`         // Additional context
	Success   bool                   `json:"success"`         // Whether operation succeeded
	Error     string                 `json:"error,omitempty"` // Error message if failed
}

// Logger handles audit logging
type Logger struct {
	storage *storage.Store
}

// NewLogger creates a new audit logger
func NewLogger(store *storage.Store) *Logger {
	return &Logger{
		storage: store,
	}
}

// Log records an audit event
func (l *Logger) Log(event Event) {
	// Set timestamp if not provided
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Log to application logs
	eventJSON, _ := json.Marshal(event)
	if event.Success {
		log.Info("AUDIT", "event", string(eventJSON))
	} else {
		log.Warn("AUDIT [FAILED]", "event", string(eventJSON))
	}

	// Persist to storage if available
	if l.storage != nil {
		// Convert audit.Event to storage.AuditEvent
		storageEvent := storage.AuditEvent{
			Timestamp: event.Timestamp,
			Action:    storage.Action(event.Action),
			ActorID:   event.ActorID,
			ActorIP:   event.ActorIP,
			TargetID:  event.TargetID,
			Details:   event.Details,
			Success:   event.Success,
			Error:     event.Error,
		}

		// Persist asynchronously to avoid blocking
		go func() {
			if err := l.storage.LogAuditEvent(storageEvent); err != nil {
				log.Warn("Failed to persist audit event to storage", "error", err)
			}
		}()
	}
}

// LogDevicePaired logs successful device pairing
func (l *Logger) LogDevicePaired(deviceID, actorIP, platform string) {
	l.Log(Event{
		Action:   ActionDevicePaired,
		ActorID:  deviceID,
		ActorIP:  actorIP,
		TargetID: deviceID,
		Details: map[string]interface{}{
			"platform": platform,
		},
		Success: true,
	})
}

// LogDeviceRevoked logs device revocation
func (l *Logger) LogDeviceRevoked(deviceID, revokedBy, actorIP string, success bool, err error) {
	event := Event{
		Action:   ActionDeviceRevoked,
		ActorID:  revokedBy,
		ActorIP:  actorIP,
		TargetID: deviceID,
		Success:  success,
	}
	if err != nil {
		event.Error = err.Error()
	}
	l.Log(event)
}

// LogDeviceUpdated logs device metadata updates
func (l *Logger) LogDeviceUpdated(deviceID, updatedBy, actorIP, newName string, success bool, err error) {
	event := Event{
		Action:   ActionDeviceUpdated,
		ActorID:  updatedBy,
		ActorIP:  actorIP,
		TargetID: deviceID,
		Details: map[string]interface{}{
			"newName": newName,
		},
		Success: success,
	}
	if err != nil {
		event.Error = err.Error()
	}
	l.Log(event)
}

// LogAppRegistered logs app registration (from discovery proposals)
func (l *Logger) LogAppRegistered(appID, actorID, actorIP string, success bool, err error) {
	event := Event{
		Action:   ActionAppRegistered,
		ActorID:  actorID,
		ActorIP:  actorIP,
		TargetID: appID,
		Success:  success,
	}
	if err != nil {
		event.Error = err.Error()
	}
	l.Log(event)
}

// LogConfigReloaded logs configuration reload events
func (l *Logger) LogConfigReloaded(actorID, actorIP string, changes map[string]int, success bool, err error) {
	event := Event{
		Action:   ActionConfigReloaded,
		ActorID:  actorID,
		ActorIP:  actorIP,
		TargetID: "config",
		Details:  map[string]interface{}{"changes": changes},
		Success:  success,
	}
	if err != nil {
		event.Error = err.Error()
	}
	l.Log(event)
}

// LogProposalAction logs proposal approval/dismissal
func (l *Logger) LogProposalAction(action Action, proposalID, actorID, actorIP string, success bool, err error) {
	event := Event{
		Action:   action,
		ActorID:  actorID,
		ActorIP:  actorIP,
		TargetID: proposalID,
		Success:  success,
	}
	if err != nil {
		event.Error = err.Error()
	}
	l.Log(event)
}
