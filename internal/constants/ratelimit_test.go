package constants

import "testing"

func TestRateLimitConstants(t *testing.T) {
	tests := []struct {
		name      string
		rate      float64
		burst     int
		wantRate  float64
		wantBurst int
	}{
		{
			name:      "QR rate limit is 1 req/sec with burst of 5",
			rate:      QRLimitRate,
			burst:     QRLimitBurst,
			wantRate:  1.0,
			wantBurst: 5,
		},
		{
			name:      "Auth rate limit is 10 req/min (0.167) with burst of 10",
			rate:      AuthLimitRate,
			burst:     AuthLimitBurst,
			wantRate:  0.167,
			wantBurst: 10,
		},
		{
			name:      "Device rate limit is 30 req/min (0.5) with burst of 30",
			rate:      DeviceLimitRate,
			burst:     DeviceLimitBurst,
			wantRate:  0.5,
			wantBurst: 30,
		},
		{
			name:      "Container rate limit is 60 req/min (1.0) with burst of 60",
			rate:      ContainerLimitRate,
			burst:     ContainerLimitBurst,
			wantRate:  1.0,
			wantBurst: 60,
		},
		{
			name:      "Health rate limit is 30 req/min (0.5) with burst of 30",
			rate:      HealthLimitRate,
			burst:     HealthLimitBurst,
			wantRate:  0.5,
			wantBurst: 30,
		},
		{
			name:      "Metrics rate limit is 30 req/min (0.5) with burst of 30",
			rate:      MetricsLimitRate,
			burst:     MetricsLimitBurst,
			wantRate:  0.5,
			wantBurst: 30,
		},
		{
			name:      "WebSocket rate limit is 10 req/min (0.167) with burst of 10",
			rate:      WebSocketLimitRate,
			burst:     WebSocketLimitBurst,
			wantRate:  0.167,
			wantBurst: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.rate != tt.wantRate {
				t.Errorf("Rate = %v, want %v", tt.rate, tt.wantRate)
			}
			if tt.burst != tt.wantBurst {
				t.Errorf("Burst = %v, want %v", tt.burst, tt.wantBurst)
			}
		})
	}
}

func TestHealthCheckConstants(t *testing.T) {
	if HealthCheckMemoryThresholdMB != 512 {
		t.Errorf("HealthCheckMemoryThresholdMB = %v, want 512", HealthCheckMemoryThresholdMB)
	}

	if HealthCheckDiskThresholdPercent != 90 {
		t.Errorf("HealthCheckDiskThresholdPercent = %v, want 90", HealthCheckDiskThresholdPercent)
	}
}

func TestRequestBodySizeLimits(t *testing.T) {
	tests := []struct {
		name     string
		size     int64
		wantSize int64
	}{
		{
			name:     "Auth request body limit is 1 MB",
			size:     MaxAuthRequestBodySize,
			wantSize: 1 * 1024 * 1024, // 1 MB
		},
		{
			name:     "Toolbox deploy request body limit is 2 MB",
			size:     MaxToolboxDeployBodySize,
			wantSize: 2 * 1024 * 1024, // 2 MB
		},
		{
			name:     "Device management request body limit is 512 KB",
			size:     MaxDeviceRequestBodySize,
			wantSize: 512 * 1024, // 512 KB
		},
		{
			name:     "System request body limit is 1 MB",
			size:     MaxSystemRequestBodySize,
			wantSize: 1 * 1024 * 1024, // 1 MB
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.size != tt.wantSize {
				t.Errorf("Size = %v, want %v", tt.size, tt.wantSize)
			}
		})
	}
}

func TestTimeoutConstants(t *testing.T) {
	tests := []struct {
		name        string
		timeout     int
		wantSeconds int
	}{
		{
			name:        "Toolbox deployment timeout is 5 minutes",
			timeout:     DeploymentTimeoutSeconds,
			wantSeconds: 300, // 5 minutes
		},
		{
			name:        "Health check timeout is 10 seconds",
			timeout:     HealthCheckTimeoutSeconds,
			wantSeconds: 10,
		},
		{
			name:        "Graceful shutdown timeout is 30 seconds",
			timeout:     GracefulShutdownTimeoutSeconds,
			wantSeconds: 30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.timeout != tt.wantSeconds {
				t.Errorf("Timeout = %v, want %v", tt.timeout, tt.wantSeconds)
			}
		})
	}
}
