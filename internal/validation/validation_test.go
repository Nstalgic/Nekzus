package validation

import (
	"strings"
	"testing"
)

func TestValidateID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{
			name:    "valid alphanumeric ID",
			id:      "app123",
			wantErr: false,
		},
		{
			name:    "valid with hyphens",
			id:      "my-app-id",
			wantErr: false,
		},
		{
			name:    "valid with underscores",
			id:      "my_app_id",
			wantErr: false,
		},
		{
			name:    "valid mixed",
			id:      "my-app_123",
			wantErr: false,
		},
		{
			name:    "empty ID",
			id:      "",
			wantErr: true,
		},
		{
			name:    "too long (>100 chars)",
			id:      strings.Repeat("a", 101),
			wantErr: true,
		},
		{
			name:    "contains spaces",
			id:      "my app",
			wantErr: true,
		},
		{
			name:    "contains special chars",
			id:      "app@123",
			wantErr: true,
		},
		{
			name:    "contains slash",
			id:      "app/123",
			wantErr: true,
		},
		{
			name:    "starts with number",
			id:      "123app",
			wantErr: false,
		},
		{
			name:    "contains only numbers",
			id:      "12345",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateID(tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateID() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "valid HTTP URL",
			url:     "http://example.com",
			wantErr: false,
		},
		{
			name:    "valid HTTPS URL",
			url:     "https://example.com",
			wantErr: false,
		},
		{
			name:    "valid with port",
			url:     "http://localhost:8080",
			wantErr: false,
		},
		{
			name:    "valid with path",
			url:     "http://example.com/api/v1",
			wantErr: false,
		},
		{
			name:    "empty URL",
			url:     "",
			wantErr: true,
		},
		{
			name:    "missing scheme",
			url:     "example.com",
			wantErr: true,
		},
		{
			name:    "invalid scheme",
			url:     "ftp://example.com",
			wantErr: true,
		},
		{
			name:    "javascript URL (XSS)",
			url:     "javascript:alert(1)",
			wantErr: true,
		},
		{
			name:    "data URL",
			url:     "data:text/html,<script>alert(1)</script>",
			wantErr: true,
		},
		{
			name:    "file URL",
			url:     "file:///etc/passwd",
			wantErr: true,
		},
		{
			name:    "localhost IP",
			url:     "http://127.0.0.1:8080",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateURL() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidatePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "valid path",
			path:    "/api/v1",
			wantErr: false,
		},
		{
			name:    "valid nested path",
			path:    "/api/v1/users/123",
			wantErr: false,
		},
		{
			name:    "root path",
			path:    "/",
			wantErr: false,
		},
		{
			name:    "empty path",
			path:    "",
			wantErr: true,
		},
		{
			name:    "relative path",
			path:    "api/v1",
			wantErr: true,
		},
		{
			name:    "path traversal (..)",
			path:    "/api/../etc/passwd",
			wantErr: true,
		},
		{
			name:    "path traversal at start",
			path:    "/../etc/passwd",
			wantErr: true,
		},
		{
			name:    "encoded path traversal",
			path:    "/api/%2e%2e/etc/passwd",
			wantErr: true,
		},
		{
			name:    "too long (>500 chars)",
			path:    "/" + strings.Repeat("a", 501),
			wantErr: true,
		},
		{
			name:    "contains null byte",
			path:    "/api\x00/v1",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePath() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		maxLen  int
		wantErr bool
	}{
		{
			name:    "valid name",
			value:   "My Application",
			maxLen:  100,
			wantErr: false,
		},
		{
			name:    "valid with numbers",
			value:   "App 123",
			maxLen:  100,
			wantErr: false,
		},
		{
			name:    "empty name",
			value:   "",
			maxLen:  100,
			wantErr: true,
		},
		{
			name:    "exceeds max length",
			value:   strings.Repeat("a", 101),
			maxLen:  100,
			wantErr: true,
		},
		{
			name:    "contains HTML tags",
			value:   "<script>alert(1)</script>",
			maxLen:  100,
			wantErr: true,
		},
		{
			name:    "contains angle brackets",
			value:   "Name <tag>",
			maxLen:  100,
			wantErr: true,
		},
		{
			name:    "valid with punctuation",
			value:   "My App! (v2.0)",
			maxLen:  100,
			wantErr: false,
		},
		{
			name:    "only whitespace",
			value:   "   ",
			maxLen:  100,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateName(tt.value, tt.maxLen)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateName() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidatePlatform(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		wantErr  bool
	}{
		{
			name:     "valid iOS",
			platform: "ios",
			wantErr:  false,
		},
		{
			name:     "valid Android",
			platform: "android",
			wantErr:  false,
		},
		{
			name:     "valid web",
			platform: "web",
			wantErr:  false,
		},
		{
			name:     "valid desktop",
			platform: "desktop",
			wantErr:  false,
		},
		{
			name:     "empty platform",
			platform: "",
			wantErr:  true,
		},
		{
			name:     "invalid platform",
			platform: "windows",
			wantErr:  true,
		},
		{
			name:     "case sensitive",
			platform: "iOS",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePlatform(tt.platform)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePlatform() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateScope(t *testing.T) {
	tests := []struct {
		name    string
		scope   string
		wantErr bool
	}{
		{
			name:    "valid scope",
			scope:   "read:devices",
			wantErr: false,
		},
		{
			name:    "valid with wildcard",
			scope:   "admin:*",
			wantErr: false,
		},
		{
			name:    "valid multiple parts",
			scope:   "write:apps:settings",
			wantErr: false,
		},
		{
			name:    "empty scope",
			scope:   "",
			wantErr: true,
		},
		{
			name:    "invalid chars",
			scope:   "read@devices",
			wantErr: true,
		},
		{
			name:    "no colon",
			scope:   "readdevices",
			wantErr: true,
		},
		{
			name:    "starts with colon",
			scope:   ":devices",
			wantErr: true,
		},
		{
			name:    "ends with colon",
			scope:   "read:",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateScope(tt.scope)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateScope() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateDuration(t *testing.T) {
	tests := []struct {
		name    string
		dur     string
		wantErr bool
	}{
		{
			name:    "valid seconds",
			dur:     "30s",
			wantErr: false,
		},
		{
			name:    "valid minutes",
			dur:     "5m",
			wantErr: false,
		},
		{
			name:    "valid hours",
			dur:     "2h",
			wantErr: false,
		},
		{
			name:    "valid mixed",
			dur:     "1h30m",
			wantErr: false,
		},
		{
			name:    "empty duration",
			dur:     "",
			wantErr: true,
		},
		{
			name:    "invalid format",
			dur:     "30",
			wantErr: true,
		},
		{
			name:    "invalid unit",
			dur:     "30x",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDuration(tt.dur)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDuration() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSanitizeString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "clean string",
			input: "Hello World",
			want:  "Hello World",
		},
		{
			name:  "remove HTML tags",
			input: "<script>alert(1)</script>",
			want:  "alert(1)",
		},
		{
			name:  "trim whitespace",
			input: "  Hello  ",
			want:  "Hello",
		},
		{
			name:  "remove null bytes",
			input: "Hello\x00World",
			want:  "HelloWorld",
		},
		{
			name:  "mixed",
			input: "  <b>Hello</b>\x00World  ",
			want:  "HelloWorld",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeString(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeString() = %q, want %q", got, tt.want)
			}
		})
	}
}
