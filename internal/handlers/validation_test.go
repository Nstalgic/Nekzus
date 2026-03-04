package handlers

import (
	"strings"
	"testing"
)

// TestPairRequest_Validate tests input validation for device pairing
func TestPairRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     PairRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid iOS device",
			req: PairRequest{
				Device: DeviceInfo{
					ID:       "test-device-123",
					Model:    "iPhone 14 Pro",
					Platform: "ios",
				},
			},
			wantErr: false,
		},
		{
			name: "valid Android device",
			req: PairRequest{
				Device: DeviceInfo{
					ID:       "android-456",
					Model:    "Samsung Galaxy S23",
					Platform: "android",
				},
			},
			wantErr: false,
		},
		{
			name: "valid web device",
			req: PairRequest{
				Device: DeviceInfo{
					Platform: "web",
				},
			},
			wantErr: false,
		},
		{
			name: "empty platform - required",
			req: PairRequest{
				Device: DeviceInfo{
					ID:    "test",
					Model: "Test Device",
				},
			},
			wantErr: true,
			errMsg:  "platform is required",
		},
		{
			name: "device ID too long",
			req: PairRequest{
				Device: DeviceInfo{
					ID:       strings.Repeat("a", 256), // 256 chars - too long
					Platform: "ios",
				},
			},
			wantErr: true,
			errMsg:  "device ID too long",
		},
		{
			name: "device model too long",
			req: PairRequest{
				Device: DeviceInfo{
					ID:       "test",
					Model:    strings.Repeat("a", 256), // 256 chars - too long
					Platform: "ios",
				},
			},
			wantErr: true,
			errMsg:  "device model too long",
		},
		{
			name: "invalid platform",
			req: PairRequest{
				Device: DeviceInfo{
					ID:       "test",
					Model:    "Test Device",
					Platform: "invalid-platform",
				},
			},
			wantErr: true,
			errMsg:  "platform",
		},
		{
			name: "max length device ID - valid",
			req: PairRequest{
				Device: DeviceInfo{
					ID:       strings.Repeat("a", 255), // 255 chars - max allowed
					Platform: "ios",
				},
			},
			wantErr: false,
		},
		{
			name: "max length model - valid",
			req: PairRequest{
				Device: DeviceInfo{
					ID:       "test",
					Model:    strings.Repeat("a", 255), // 255 chars - max allowed
					Platform: "ios",
				},
			},
			wantErr: false,
		},
		{
			name: "push token present",
			req: PairRequest{
				Device: DeviceInfo{
					ID:        "test",
					Platform:  "ios",
					PushToken: stringPtr("valid-apns-token-here"),
				},
			},
			wantErr: false,
		},
		{
			name: "push token too long",
			req: PairRequest{
				Device: DeviceInfo{
					ID:        "test",
					Platform:  "ios",
					PushToken: stringPtr(strings.Repeat("a", 513)), // 513 chars - too long
				},
			},
			wantErr: true,
			errMsg:  "push token too long",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("Validate() expected error containing %q, got nil", tt.errMsg)
					return
				}
				// Check error message contains expected text
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %v, want error containing %q", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error: %v", err)
				}
			}
		})
	}
}

// TestIsValidPlatform tests platform validation
func TestIsValidPlatform(t *testing.T) {
	tests := []struct {
		platform string
		want     bool
	}{
		{"ios", true},
		{"android", true},
		{"web", true},
		{"desktop", true},
		{"linux", true},
		{"macos", true},
		{"windows", true},
		{"", false},
		{"invalid", false},
		{"iOS", false},     // Case sensitive
		{"Android", false}, // Case sensitive
		{"mobile", false},  // Not in list
		{"unknown", false}, // Not in list
	}

	for _, tt := range tests {
		t.Run(tt.platform, func(t *testing.T) {
			got := isValidPlatform(tt.platform)
			if got != tt.want {
				t.Errorf("isValidPlatform(%q) = %v, want %v", tt.platform, got, tt.want)
			}
		})
	}
}

// Helper function to create string pointer
func stringPtr(s string) *string {
	return &s
}

// TestValidateDeviceMetadata tests device metadata validation
func TestValidateDeviceMetadata(t *testing.T) {
	tests := []struct {
		name     string
		deviceID string
		model    string
		platform string
		wantErr  bool
	}{
		{
			name:     "all valid",
			deviceID: "test-123",
			model:    "iPhone 14",
			platform: "ios",
			wantErr:  false,
		},
		{
			name:     "empty model - allowed",
			deviceID: "test-123",
			model:    "",
			platform: "ios",
			wantErr:  false,
		},
		{
			name:     "special characters in model",
			deviceID: "test-123",
			model:    "Samsung Galaxy S23 (5G)",
			platform: "android",
			wantErr:  false,
		},
		{
			name:     "unicode in model",
			deviceID: "test-123",
			model:    "设备名称",
			platform: "android",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := PairRequest{
				Device: DeviceInfo{
					ID:       tt.deviceID,
					Model:    tt.model,
					Platform: tt.platform,
				},
			}
			err := req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestUpdateDeviceMetadataRequest_Validate tests device metadata update validation
func TestUpdateDeviceMetadataRequest_Validate(t *testing.T) {
	tests := []struct {
		name       string
		deviceName string
		wantErr    bool
		errMsg     string
	}{
		{
			name:       "valid name",
			deviceName: "My iPhone",
			wantErr:    false,
		},
		{
			name:       "empty name - invalid",
			deviceName: "",
			wantErr:    true,
			errMsg:     "deviceName cannot be empty",
		},
		{
			name:       "name too long",
			deviceName: strings.Repeat("a", 256), // 256 chars - too long
			wantErr:    true,
			errMsg:     "deviceName too long",
		},
		{
			name:       "max length name",
			deviceName: strings.Repeat("a", 255), // 255 chars - max allowed
			wantErr:    false,
		},
		{
			name:       "unicode characters",
			deviceName: "我的设备",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := UpdateDeviceMetadataRequest{
				DeviceName: tt.deviceName,
			}
			err := req.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("Validate() expected error containing %q, got nil", tt.errMsg)
					return
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %v, want error containing %q", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error: %v", err)
				}
			}
		})
	}
}
