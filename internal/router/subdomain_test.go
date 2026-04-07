package router

import (
	"testing"
)

func TestExtractSubdomain(t *testing.T) {
	tests := []struct {
		name       string
		host       string
		baseDomain string
		want       string
		wantOK     bool
	}{
		{"simple", "sonarr.nekzus.local", "nekzus.local", "sonarr", true},
		{"with port", "sonarr.nekzus.local:8443", "nekzus.local", "sonarr", true},
		{"multi-level subdomain", "a.b.nekzus.local", "nekzus.local", "a.b", true},
		{"exact match (no subdomain)", "nekzus.local", "nekzus.local", "", false},
		{"different domain", "sonarr.other.com", "nekzus.local", "", false},
		{"empty host", "", "nekzus.local", "", false},
		{"empty base domain", "sonarr.nekzus.local", "", "", false},
		{"IP address", "192.168.1.100:8443", "nekzus.local", "", false},
		{"base domain with port", "sonarr.nekzus.local:8443", "nekzus.local:8443", "sonarr", true},
		{"case insensitive", "Sonarr.Nekzus.Local", "nekzus.local", "sonarr", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ExtractSubdomain(tt.host, tt.baseDomain)
			if ok != tt.wantOK {
				t.Errorf("ExtractSubdomain(%q, %q) ok = %v, want %v", tt.host, tt.baseDomain, ok, tt.wantOK)
			}
			if got != tt.want {
				t.Errorf("ExtractSubdomain(%q, %q) = %q, want %q", tt.host, tt.baseDomain, got, tt.want)
			}
		})
	}
}

func TestValidateSubdomain(t *testing.T) {
	tests := []struct {
		name    string
		sub     string
		wantErr bool
	}{
		{"valid simple", "sonarr", false},
		{"valid with hyphen", "my-app", false},
		{"valid with numbers", "app123", false},
		{"empty", "", true},
		{"starts with hyphen", "-sonarr", true},
		{"ends with hyphen", "sonarr-", true},
		{"contains uppercase", "Sonarr", true},
		{"contains underscore", "my_app", true},
		{"contains dot", "my.app", true},
		{"too long", string(make([]byte, 64)), true},
		{"max length (63)", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSubdomain(tt.sub)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSubdomain(%q) error = %v, wantErr %v", tt.sub, err, tt.wantErr)
			}
		})
	}
}
