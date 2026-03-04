package websocket

import (
	"time"

	"github.com/nstalgic/nekzus/internal/types"
)

// SubscriptionOptions configures topic subscription behavior.
type SubscriptionOptions struct {
	QoS int // 0=fire-forget, 1=at-least-once, 2=exactly-once
}

// LastWill is a message published when client disconnects unexpectedly.
type LastWill struct {
	Topic   string
	Message types.WebSocketMessage
	QoS     int
}

// Session stores client state for persistence across reconnections.
type Session struct {
	DeviceID      string
	Subscriptions map[string]SubscriptionOptions // topic pattern -> options
	LastWill      *LastWill
	CreatedAt     time.Time
	LastSeen      time.Time
}

// RetainedMessage stores the last message for a topic.
type RetainedMessage struct {
	Topic     string
	Message   types.WebSocketMessage
	ExpiresAt time.Time // Zero value means never expires
	CreatedAt time.Time
	UpdatedAt time.Time
}

// IsExpired returns true if the retained message has expired.
func (rm *RetainedMessage) IsExpired() bool {
	if rm.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(rm.ExpiresAt)
}

// PendingAck tracks a message awaiting acknowledgment (QoS 1/2).
type PendingAck struct {
	MessageID  string
	DeviceID   string
	Topic      string
	Message    types.WebSocketMessage
	QoS        int
	Status     string // "pending", "sent", "pubrec" (QoS 2 intermediate)
	RetryCount int
	SentAt     time.Time
	ExpiresAt  time.Time
}

// QoS level constants
const (
	QoSAtMostOnce  = 0 // Fire and forget
	QoSAtLeastOnce = 1 // Acknowledged delivery, may duplicate
	QoSExactlyOnce = 2 // Guaranteed exactly-once delivery
)

// Pending message status constants
const (
	StatusPending = "pending"
	StatusSent    = "sent"
	StatusPubRec  = "pubrec" // QoS 2 intermediate state
	StatusAcked   = "acked"
)

// SubscribeRequest represents a client request to subscribe to topics.
type SubscribeRequest struct {
	Topics []string `json:"topics"`
	QoS    int      `json:"qos"`
}

// UnsubscribeRequest represents a client request to unsubscribe from topics.
type UnsubscribeRequest struct {
	Topics []string `json:"topics"`
}

// SetLastWillRequest represents a client request to set their last will message.
type SetLastWillRequest struct {
	Topic   string      `json:"topic"`
	Message interface{} `json:"message"`
	QoS     int         `json:"qos"`
}

// AckRequest represents a client acknowledgment of a message.
type AckRequest struct {
	MessageID string `json:"messageId"`
}

// SubAckResponse represents the server's response to a subscribe request.
type SubAckResponse struct {
	Topics  []string `json:"topics"`
	Success bool     `json:"success"`
	Error   string   `json:"error,omitempty"`
}

// UnsubAckResponse represents the server's response to an unsubscribe request.
type UnsubAckResponse struct {
	Topics  []string `json:"topics"`
	Success bool     `json:"success"`
}

// LWTAckResponse represents the server's response to a set_last_will request.
type LWTAckResponse struct {
	Success bool   `json:"success"`
	Cleared bool   `json:"cleared,omitempty"` // true if LWT was cleared (empty topic)
	Error   string `json:"error,omitempty"`
}

// MaxSubscriptionsPerClient is the maximum number of topic subscriptions per client.
const MaxSubscriptionsPerClient = 100
