package proxy

import (
	"errors"
	"net/http"
	"time"

	"github.com/nstalgic/nekzus/internal/crypto"
	"github.com/nstalgic/nekzus/internal/storage"
)

// CookieStore defines the interface for proxy cookie storage operations.
type CookieStore interface {
	SaveProxyCookie(cookie storage.ProxyCookie, key []byte) error
	GetProxyCookies(deviceID, appID string, key []byte) ([]storage.ProxyCookie, error)
	DeleteProxyCookiesForApp(deviceID, appID string) error
	DeleteProxyCookiesForDevice(deviceID string) error
	SaveDevice(deviceID, name, platform, version string, scopes []string) error
}

// SessionCookieManager handles capturing and replaying backend service cookies
// for mobile webview session persistence.
type SessionCookieManager struct {
	store         CookieStore
	encryptor     *crypto.CookieEncryptor
	encryptionKey []byte
}

// NewSessionCookieManager creates a new session cookie manager.
func NewSessionCookieManager(store CookieStore, encryptor *crypto.CookieEncryptor, key []byte) *SessionCookieManager {
	return &SessionCookieManager{
		store:         store,
		encryptor:     encryptor,
		encryptionKey: key,
	}
}

// CaptureResponseCookies extracts Set-Cookie headers from a backend response
// and stores them for later replay.
func (m *SessionCookieManager) CaptureResponseCookies(deviceID, appID string, cookies []*http.Cookie) error {
	if deviceID == "" {
		return errors.New("device ID is required for cookie capture")
	}
	if appID == "" {
		return errors.New("app ID is required for cookie capture")
	}
	if len(cookies) == 0 {
		return nil
	}

	now := time.Now()

	for _, cookie := range cookies {
		// Skip cookies that are already expired
		if !cookie.Expires.IsZero() && cookie.Expires.Before(now) {
			continue
		}

		// Skip cookies being deleted (empty value with past expiry)
		if cookie.MaxAge < 0 {
			// Cookie is being deleted - remove from storage
			m.store.DeleteProxyCookiesForApp(deviceID, appID)
			continue
		}

		proxyCookie := storage.ProxyCookie{
			DeviceID:     deviceID,
			AppID:        appID,
			CookieName:   cookie.Name,
			CookieValue:  cookie.Value,
			CookiePath:   cookie.Path,
			CookieDomain: cookie.Domain,
			Secure:       cookie.Secure,
			HttpOnly:     cookie.HttpOnly,
			SameSite:     sameSiteToString(cookie.SameSite),
		}

		// Handle expiry
		if !cookie.Expires.IsZero() {
			proxyCookie.ExpiresAt = &cookie.Expires
		} else if cookie.MaxAge > 0 {
			expiry := now.Add(time.Duration(cookie.MaxAge) * time.Second)
			proxyCookie.ExpiresAt = &expiry
		}
		// If both Expires and MaxAge are unset, it's a session cookie (no expiry)

		if err := m.store.SaveProxyCookie(proxyCookie, m.encryptionKey); err != nil {
			return err
		}
	}

	return nil
}

// InjectRequestCookies adds stored cookies to an outgoing proxy request.
func (m *SessionCookieManager) InjectRequestCookies(deviceID, appID string, r *http.Request) error {
	if deviceID == "" {
		return errors.New("device ID is required for cookie injection")
	}
	if appID == "" {
		return errors.New("app ID is required for cookie injection")
	}

	cookies, err := m.store.GetProxyCookies(deviceID, appID, m.encryptionKey)
	if err != nil {
		return err
	}

	for _, c := range cookies {
		r.AddCookie(&http.Cookie{
			Name:  c.CookieName,
			Value: c.CookieValue,
		})
	}

	return nil
}

// ClearCookies removes all stored cookies for a device+app combination.
func (m *SessionCookieManager) ClearCookies(deviceID, appID string) error {
	return m.store.DeleteProxyCookiesForApp(deviceID, appID)
}

// ClearAllCookies removes all stored cookies for a device.
func (m *SessionCookieManager) ClearAllCookies(deviceID string) error {
	return m.store.DeleteProxyCookiesForDevice(deviceID)
}

// sameSiteToString converts http.SameSite to string for storage.
func sameSiteToString(s http.SameSite) string {
	switch s {
	case http.SameSiteStrictMode:
		return "Strict"
	case http.SameSiteLaxMode:
		return "Lax"
	case http.SameSiteNoneMode:
		return "None"
	default:
		return "Lax"
	}
}
