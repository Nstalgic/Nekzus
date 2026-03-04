package validation

import (
	"testing"

	"github.com/nstalgic/nekzus/internal/types"
)

func TestValidateApp(t *testing.T) {
	tests := []struct {
		name    string
		app     types.App
		wantErr bool
	}{
		{
			name: "valid app",
			app: types.App{
				ID:   "my-app",
				Name: "My Application",
			},
			wantErr: false,
		},
		{
			name: "valid app with tags",
			app: types.App{
				ID:   "my-app",
				Name: "My Application",
				Tags: []string{"backend", "api"},
			},
			wantErr: false,
		},
		{
			name: "invalid app ID",
			app: types.App{
				ID:   "my app",
				Name: "My Application",
			},
			wantErr: true,
		},
		{
			name: "invalid app name",
			app: types.App{
				ID:   "my-app",
				Name: "<script>alert(1)</script>",
			},
			wantErr: true,
		},
		{
			name: "invalid tag",
			app: types.App{
				ID:   "my-app",
				Name: "My Application",
				Tags: []string{"<script>"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateApp(tt.app)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateApp() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateRoute(t *testing.T) {
	tests := []struct {
		name    string
		route   types.Route
		wantErr bool
	}{
		{
			name: "valid route",
			route: types.Route{
				RouteID:  "route-1",
				AppID:    "app-1",
				PathBase: "/api",
				To:       "http://localhost:8080",
			},
			wantErr: false,
		},
		{
			name: "valid route with scopes",
			route: types.Route{
				RouteID:  "route-1",
				AppID:    "app-1",
				PathBase: "/api",
				To:       "http://localhost:8080",
				Scopes:   []string{"read:data", "write:data"},
			},
			wantErr: false,
		},
		{
			name: "invalid route ID",
			route: types.Route{
				RouteID:  "route@1",
				AppID:    "app-1",
				PathBase: "/api",
				To:       "http://localhost:8080",
			},
			wantErr: true,
		},
		{
			name: "invalid app ID",
			route: types.Route{
				RouteID:  "route-1",
				AppID:    "",
				PathBase: "/api",
				To:       "http://localhost:8080",
			},
			wantErr: true,
		},
		{
			name: "invalid path",
			route: types.Route{
				RouteID:  "route-1",
				AppID:    "app-1",
				PathBase: "../etc/passwd",
				To:       "http://localhost:8080",
			},
			wantErr: true,
		},
		{
			name: "invalid URL",
			route: types.Route{
				RouteID:  "route-1",
				AppID:    "app-1",
				PathBase: "/api",
				To:       "javascript:alert(1)",
			},
			wantErr: true,
		},
		{
			name: "invalid scope",
			route: types.Route{
				RouteID:  "route-1",
				AppID:    "app-1",
				PathBase: "/api",
				To:       "http://localhost:8080",
				Scopes:   []string{"invalid"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRoute(tt.route)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRoute() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateDeviceRegistration(t *testing.T) {
	tests := []struct {
		name     string
		deviceID string
		platform string
		wantErr  bool
	}{
		{
			name:     "valid iOS device",
			deviceID: "device-123",
			platform: "ios",
			wantErr:  false,
		},
		{
			name:     "valid Android device",
			deviceID: "device-456",
			platform: "android",
			wantErr:  false,
		},
		{
			name:     "invalid device ID",
			deviceID: "device 123",
			platform: "ios",
			wantErr:  true,
		},
		{
			name:     "invalid platform",
			deviceID: "device-123",
			platform: "windows",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDeviceRegistration(tt.deviceID, tt.platform)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDeviceRegistration() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateAPIKey(t *testing.T) {
	tests := []struct {
		name    string
		key     types.APIKey
		wantErr bool
	}{
		{
			name: "valid API key",
			key: types.APIKey{
				ID:     "key-1",
				Name:   "My API Key",
				Scopes: []string{"read:devices", "write:apps"},
			},
			wantErr: false,
		},
		{
			name: "invalid ID",
			key: types.APIKey{
				ID:     "",
				Name:   "My API Key",
				Scopes: []string{"read:devices"},
			},
			wantErr: true,
		},
		{
			name: "invalid name",
			key: types.APIKey{
				ID:     "key-1",
				Name:   "<script>",
				Scopes: []string{"read:devices"},
			},
			wantErr: true,
		},
		{
			name: "invalid scope",
			key: types.APIKey{
				ID:     "key-1",
				Name:   "My API Key",
				Scopes: []string{"invalid-scope"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAPIKey(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAPIKey() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
