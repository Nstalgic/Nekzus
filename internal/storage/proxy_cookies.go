package storage

import (
	"fmt"
	"time"

	"github.com/nstalgic/nekzus/internal/crypto"
)

// ProxyCookie represents a stored backend service cookie for session persistence.
type ProxyCookie struct {
	ID           int64      `json:"id"`
	DeviceID     string     `json:"deviceId"`
	AppID        string     `json:"appId"`
	CookieName   string     `json:"cookieName"`
	CookieValue  string     `json:"cookieValue,omitempty"` // Decrypted; omitted in API responses
	CookiePath   string     `json:"cookiePath"`
	CookieDomain string     `json:"cookieDomain,omitempty"`
	ExpiresAt    *time.Time `json:"expiresAt,omitempty"`
	Secure       bool       `json:"secure"`
	HttpOnly     bool       `json:"httpOnly"`
	SameSite     string     `json:"sameSite"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

// SaveProxyCookie stores or updates a proxy session cookie with encryption.
// Uses UPSERT to handle both insert and update cases.
func (s *Store) SaveProxyCookie(cookie ProxyCookie, encryptionKey []byte) error {
	encryptor, err := crypto.NewCookieEncryptor(encryptionKey)
	if err != nil {
		return fmt.Errorf("failed to create encryptor: %w", err)
	}

	encryptedValue, err := encryptor.Encrypt(cookie.CookieValue)
	if err != nil {
		return fmt.Errorf("failed to encrypt cookie value: %w", err)
	}

	// Format expiry time for SQLite datetime comparison
	var expiresAtStr *string
	if cookie.ExpiresAt != nil {
		formatted := cookie.ExpiresAt.UTC().Format("2006-01-02 15:04:05")
		expiresAtStr = &formatted
	}

	query := `
		INSERT INTO proxy_session_cookies (
			device_id, app_id, cookie_name, cookie_value_encrypted,
			cookie_path, cookie_domain, expires_at, secure, http_only, same_site,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(device_id, app_id, cookie_name) DO UPDATE SET
			cookie_value_encrypted = excluded.cookie_value_encrypted,
			cookie_path = excluded.cookie_path,
			cookie_domain = excluded.cookie_domain,
			expires_at = excluded.expires_at,
			secure = excluded.secure,
			http_only = excluded.http_only,
			same_site = excluded.same_site,
			updated_at = CURRENT_TIMESTAMP
	`

	_, err = s.db.Exec(query,
		cookie.DeviceID,
		cookie.AppID,
		cookie.CookieName,
		encryptedValue,
		cookie.CookiePath,
		cookie.CookieDomain,
		expiresAtStr,
		cookie.Secure,
		cookie.HttpOnly,
		cookie.SameSite,
	)
	if err != nil {
		return fmt.Errorf("failed to save proxy cookie: %w", err)
	}

	return nil
}

// GetProxyCookies retrieves all valid (non-expired) cookies for a device+app combination.
// Session cookies (no expiry) and cookies with future expiry are returned.
func (s *Store) GetProxyCookies(deviceID, appID string, encryptionKey []byte) ([]ProxyCookie, error) {
	encryptor, err := crypto.NewCookieEncryptor(encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create encryptor: %w", err)
	}

	query := `
		SELECT id, device_id, app_id, cookie_name, cookie_value_encrypted,
			cookie_path, cookie_domain, expires_at, secure, http_only, same_site,
			created_at, updated_at
		FROM proxy_session_cookies
		WHERE device_id = ? AND app_id = ?
		AND (expires_at IS NULL OR datetime(expires_at) > datetime('now'))
	`

	rows, err := s.db.Query(query, deviceID, appID)
	if err != nil {
		return nil, fmt.Errorf("failed to query proxy cookies: %w", err)
	}
	defer rows.Close()

	var cookies []ProxyCookie
	for rows.Next() {
		var c ProxyCookie
		var encryptedValue []byte

		err := rows.Scan(
			&c.ID,
			&c.DeviceID,
			&c.AppID,
			&c.CookieName,
			&encryptedValue,
			&c.CookiePath,
			&c.CookieDomain,
			&c.ExpiresAt,
			&c.Secure,
			&c.HttpOnly,
			&c.SameSite,
			&c.CreatedAt,
			&c.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan proxy cookie: %w", err)
		}

		// Decrypt cookie value
		decryptedValue, err := encryptor.Decrypt(encryptedValue)
		if err != nil {
			// Skip cookies that fail to decrypt (key rotation case)
			continue
		}
		c.CookieValue = decryptedValue

		cookies = append(cookies, c)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating proxy cookies: %w", err)
	}

	return cookies, nil
}

// DeleteProxyCookie removes a specific cookie by device, app, and name.
func (s *Store) DeleteProxyCookie(deviceID, appID, cookieName string) error {
	query := `DELETE FROM proxy_session_cookies WHERE device_id = ? AND app_id = ? AND cookie_name = ?`
	_, err := s.db.Exec(query, deviceID, appID, cookieName)
	if err != nil {
		return fmt.Errorf("failed to delete proxy cookie: %w", err)
	}
	return nil
}

// DeleteProxyCookiesForApp removes all cookies for a device+app combination.
func (s *Store) DeleteProxyCookiesForApp(deviceID, appID string) error {
	query := `DELETE FROM proxy_session_cookies WHERE device_id = ? AND app_id = ?`
	_, err := s.db.Exec(query, deviceID, appID)
	if err != nil {
		return fmt.Errorf("failed to delete proxy cookies for app: %w", err)
	}
	return nil
}

// DeleteProxyCookiesForDevice removes all cookies for a device.
func (s *Store) DeleteProxyCookiesForDevice(deviceID string) error {
	query := `DELETE FROM proxy_session_cookies WHERE device_id = ?`
	_, err := s.db.Exec(query, deviceID)
	if err != nil {
		return fmt.Errorf("failed to delete proxy cookies for device: %w", err)
	}
	return nil
}

// CleanupExpiredProxyCookies removes all expired cookies from storage.
// Returns the number of cookies deleted.
func (s *Store) CleanupExpiredProxyCookies() (int64, error) {
	query := `DELETE FROM proxy_session_cookies WHERE expires_at IS NOT NULL AND datetime(expires_at) <= datetime('now')`
	result, err := s.db.Exec(query)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup expired proxy cookies: %w", err)
	}

	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get deleted count: %w", err)
	}

	return deleted, nil
}

// ListProxyCookieSummaries returns cookie summaries (without values) for a device.
// Used for the session management API.
func (s *Store) ListProxyCookieSummaries(deviceID string) ([]ProxyCookie, error) {
	query := `
		SELECT id, device_id, app_id, cookie_name,
			cookie_path, cookie_domain, expires_at, secure, http_only, same_site,
			created_at, updated_at
		FROM proxy_session_cookies
		WHERE device_id = ?
		AND (expires_at IS NULL OR datetime(expires_at) > datetime('now'))
		ORDER BY app_id, cookie_name
	`

	rows, err := s.db.Query(query, deviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to query proxy cookie summaries: %w", err)
	}
	defer rows.Close()

	var cookies []ProxyCookie
	for rows.Next() {
		var c ProxyCookie
		err := rows.Scan(
			&c.ID,
			&c.DeviceID,
			&c.AppID,
			&c.CookieName,
			&c.CookiePath,
			&c.CookieDomain,
			&c.ExpiresAt,
			&c.Secure,
			&c.HttpOnly,
			&c.SameSite,
			&c.CreatedAt,
			&c.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan proxy cookie summary: %w", err)
		}
		cookies = append(cookies, c)
	}

	return cookies, nil
}
