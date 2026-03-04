package websocket

import (
	"encoding/json"
	"time"
)

// SessionStore provides session persistence for WebSocket clients.
// This allows clients to resume their subscriptions after reconnecting.
type SessionStore interface {
	SaveSession(session *Session) error
	GetSession(deviceID string) (*Session, error)
	DeleteSession(deviceID string) error
	UpdateLastSeen(deviceID string) error
	CleanExpiredSessions(maxAge time.Duration) (int, error)
}

// InMemorySessionStore provides an in-memory session store for testing.
type InMemorySessionStore struct {
	sessions map[string]*Session
}

// NewInMemorySessionStore creates a new in-memory session store.
func NewInMemorySessionStore() *InMemorySessionStore {
	return &InMemorySessionStore{
		sessions: make(map[string]*Session),
	}
}

// SaveSession saves a session.
func (s *InMemorySessionStore) SaveSession(session *Session) error {
	s.sessions[session.DeviceID] = session
	return nil
}

// GetSession retrieves a session by device ID.
func (s *InMemorySessionStore) GetSession(deviceID string) (*Session, error) {
	session, exists := s.sessions[deviceID]
	if !exists {
		return nil, nil
	}
	return session, nil
}

// DeleteSession deletes a session.
func (s *InMemorySessionStore) DeleteSession(deviceID string) error {
	delete(s.sessions, deviceID)
	return nil
}

// UpdateLastSeen updates the last seen timestamp for a session.
func (s *InMemorySessionStore) UpdateLastSeen(deviceID string) error {
	session, exists := s.sessions[deviceID]
	if exists {
		session.LastSeen = time.Now()
	}
	return nil
}

// CleanExpiredSessions removes sessions older than maxAge.
func (s *InMemorySessionStore) CleanExpiredSessions(maxAge time.Duration) (int, error) {
	cutoff := time.Now().Add(-maxAge)
	count := 0
	for id, session := range s.sessions {
		if session.LastSeen.Before(cutoff) {
			delete(s.sessions, id)
			count++
		}
	}
	return count, nil
}

// MarshalSession serializes a session to JSON.
func MarshalSession(session *Session) ([]byte, error) {
	return json.Marshal(session)
}

// UnmarshalSession deserializes a session from JSON.
func UnmarshalSession(data []byte) (*Session, error) {
	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, err
	}
	return &session, nil
}

// SessionFromClient creates a session from a client's current state.
func SessionFromClient(client *Client) *Session {
	now := time.Now()
	return &Session{
		DeviceID:      client.deviceID,
		Subscriptions: client.GetSubscriptions(),
		LastWill:      client.GetLastWill(),
		CreatedAt:     now,
		LastSeen:      now,
	}
}

// RestoreClientFromSession restores a client's subscriptions from a session.
func RestoreClientFromSession(client *Client, session *Session) {
	if session == nil {
		return
	}

	// Restore subscriptions
	for pattern, opts := range session.Subscriptions {
		_ = client.SubscribeToTopics([]string{pattern}, opts)
	}

	// Restore last will
	if session.LastWill != nil {
		client.SetLastWill(session.LastWill)
	}
}

// SessionManager manages session persistence for the WebSocket manager.
type SessionManager struct {
	store         SessionStore
	sessionExpiry time.Duration
}

// NewSessionManager creates a new session manager.
func NewSessionManager(store SessionStore, sessionExpiry time.Duration) *SessionManager {
	if sessionExpiry == 0 {
		sessionExpiry = 24 * time.Hour
	}
	return &SessionManager{
		store:         store,
		sessionExpiry: sessionExpiry,
	}
}

// SaveClientSession saves a client's session.
func (m *SessionManager) SaveClientSession(client *Client) error {
	if m.store == nil {
		return nil
	}
	session := SessionFromClient(client)
	return m.store.SaveSession(session)
}

// RestoreClientSession restores a client's session from storage.
func (m *SessionManager) RestoreClientSession(client *Client) error {
	if m.store == nil {
		return nil
	}

	session, err := m.store.GetSession(client.deviceID)
	if err != nil {
		return err
	}

	RestoreClientFromSession(client, session)
	return nil
}

// DeleteClientSession deletes a client's session.
func (m *SessionManager) DeleteClientSession(deviceID string) error {
	if m.store == nil {
		return nil
	}
	return m.store.DeleteSession(deviceID)
}

// UpdateClientLastSeen updates the last seen timestamp.
func (m *SessionManager) UpdateClientLastSeen(deviceID string) error {
	if m.store == nil {
		return nil
	}
	return m.store.UpdateLastSeen(deviceID)
}

// CleanExpiredSessions removes expired sessions.
func (m *SessionManager) CleanExpiredSessions() (int, error) {
	if m.store == nil {
		return 0, nil
	}
	return m.store.CleanExpiredSessions(m.sessionExpiry)
}
