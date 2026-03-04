package security

import (
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
)

// TestAnalyzePortBindings tests the port exposure detection logic
func TestAnalyzePortBindings(t *testing.T) {
	tests := []struct {
		name          string
		ports         nat.PortMap
		wantRiskLevel RiskLevel
		wantCount     int
		wantHasPublic bool
	}{
		{
			name:          "No ports exposed",
			ports:         nat.PortMap{},
			wantRiskLevel: RiskNone,
			wantCount:     0,
			wantHasPublic: false,
		},
		{
			name: "Port bound to 127.0.0.1 (safe)",
			ports: nat.PortMap{
				"8080/tcp": []nat.PortBinding{
					{HostIP: "127.0.0.1", HostPort: "8080"},
				},
			},
			wantRiskLevel: RiskNone,
			wantCount:     1,
			wantHasPublic: false,
		},
		{
			name: "Port bound to localhost (safe)",
			ports: nat.PortMap{
				"8080/tcp": []nat.PortBinding{
					{HostIP: "localhost", HostPort: "8080"},
				},
			},
			wantRiskLevel: RiskNone,
			wantCount:     1,
			wantHasPublic: false,
		},
		{
			name: "Port bound to 0.0.0.0 (CRITICAL RISK)",
			ports: nat.PortMap{
				"8080/tcp": []nat.PortBinding{
					{HostIP: "0.0.0.0", HostPort: "8080"},
				},
			},
			wantRiskLevel: RiskCritical,
			wantCount:     1,
			wantHasPublic: true,
		},
		{
			name: "Port bound to empty IP (defaults to 0.0.0.0, CRITICAL)",
			ports: nat.PortMap{
				"8080/tcp": []nat.PortBinding{
					{HostIP: "", HostPort: "8080"},
				},
			},
			wantRiskLevel: RiskCritical,
			wantCount:     1,
			wantHasPublic: true,
		},
		{
			name: "Port bound to private IP (WARNING)",
			ports: nat.PortMap{
				"8080/tcp": []nat.PortBinding{
					{HostIP: "192.168.1.100", HostPort: "8080"},
				},
			},
			wantRiskLevel: RiskWarning,
			wantCount:     1,
			wantHasPublic: false,
		},
		{
			name: "Port bound to public IP (CRITICAL RISK)",
			ports: nat.PortMap{
				"8080/tcp": []nat.PortBinding{
					{HostIP: "8.8.8.8", HostPort: "8080"},
				},
			},
			wantRiskLevel: RiskCritical,
			wantCount:     1,
			wantHasPublic: true,
		},
		{
			name: "Multiple ports with mixed risk",
			ports: nat.PortMap{
				"8080/tcp": []nat.PortBinding{
					{HostIP: "0.0.0.0", HostPort: "8080"},
				},
				"9090/tcp": []nat.PortBinding{
					{HostIP: "127.0.0.1", HostPort: "9090"},
				},
				"3000/tcp": []nat.PortBinding{
					{HostIP: "192.168.1.100", HostPort: "3000"},
				},
			},
			wantRiskLevel: RiskCritical, // Highest risk wins
			wantCount:     3,
			wantHasPublic: true,
		},
		{
			name: "No bindings but ports defined (internal only)",
			ports: nat.PortMap{
				"8080/tcp": []nat.PortBinding{},
			},
			wantRiskLevel: RiskNone,
			wantCount:     0,
			wantHasPublic: false,
		},
		{
			name: "Multiple bindings for same port",
			ports: nat.PortMap{
				"8080/tcp": []nat.PortBinding{
					{HostIP: "127.0.0.1", HostPort: "8080"},
					{HostIP: "0.0.0.0", HostPort: "8081"},
				},
			},
			wantRiskLevel: RiskCritical, // One public binding = critical
			wantCount:     2,
			wantHasPublic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AnalyzePortBindings(tt.ports)

			if result.OverallRisk != tt.wantRiskLevel {
				t.Errorf("AnalyzePortBindings() OverallRisk = %v, want %v", result.OverallRisk, tt.wantRiskLevel)
			}

			if len(result.Bindings) != tt.wantCount {
				t.Errorf("AnalyzePortBindings() binding count = %d, want %d", len(result.Bindings), tt.wantCount)
			}

			if result.HasPublicExposure != tt.wantHasPublic {
				t.Errorf("AnalyzePortBindings() HasPublicExposure = %v, want %v", result.HasPublicExposure, tt.wantHasPublic)
			}
		})
	}
}

// TestClassifyBindingRisk tests individual binding risk classification
func TestClassifyBindingRisk(t *testing.T) {
	tests := []struct {
		name     string
		hostIP   string
		wantRisk RiskLevel
	}{
		{"Localhost IPv4", "127.0.0.1", RiskNone},
		{"Localhost", "localhost", RiskNone},
		{"Localhost IPv6", "::1", RiskNone},
		{"All interfaces IPv4", "0.0.0.0", RiskCritical},
		{"All interfaces IPv6", "::", RiskCritical},
		{"Empty (defaults to all)", "", RiskCritical},
		{"Private 192.168", "192.168.1.1", RiskWarning},
		{"Private 10", "10.0.0.1", RiskWarning},
		{"Private 172.16", "172.16.0.1", RiskWarning},
		{"Private 172.31", "172.31.255.255", RiskWarning},
		{"Public IP", "8.8.8.8", RiskCritical},
		{"Public IP", "1.1.1.1", RiskCritical},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			risk := classifyBindingRisk(tt.hostIP)
			if risk != tt.wantRisk {
				t.Errorf("classifyBindingRisk(%s) = %v, want %v", tt.hostIP, risk, tt.wantRisk)
			}
		})
	}
}

// TestGetRecommendations tests recommendation generation
func TestGetRecommendations(t *testing.T) {
	tests := []struct {
		name                   string
		riskLevel              RiskLevel
		hasPublic              bool
		wantMinRecommendations int
	}{
		{
			name:                   "No risk",
			riskLevel:              RiskNone,
			hasPublic:              false,
			wantMinRecommendations: 0,
		},
		{
			name:                   "Critical risk with public exposure",
			riskLevel:              RiskCritical,
			hasPublic:              true,
			wantMinRecommendations: 2, // At least 2 recommendations
		},
		{
			name:                   "Warning risk without public",
			riskLevel:              RiskWarning,
			hasPublic:              false,
			wantMinRecommendations: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recommendations := getRecommendations(tt.riskLevel, tt.hasPublic)
			if len(recommendations) < tt.wantMinRecommendations {
				t.Errorf("getRecommendations() returned %d recommendations, want at least %d",
					len(recommendations), tt.wantMinRecommendations)
			}
		})
	}
}

// TestInspectContainerPorts tests Docker container inspection
func TestInspectContainerPorts(t *testing.T) {
	tests := []struct {
		name          string
		containerJSON *container.InspectResponse
		wantRiskLevel RiskLevel
		wantErr       bool
	}{
		{
			name: "Container with public port",
			containerJSON: &container.InspectResponse{
				NetworkSettings: &container.NetworkSettings{
					NetworkSettingsBase: container.NetworkSettingsBase{
						Ports: nat.PortMap{
							"8080/tcp": []nat.PortBinding{
								{HostIP: "0.0.0.0", HostPort: "8080"},
							},
						},
					},
				},
			},
			wantRiskLevel: RiskCritical,
			wantErr:       false,
		},
		{
			name: "Container with safe port",
			containerJSON: &container.InspectResponse{
				NetworkSettings: &container.NetworkSettings{
					NetworkSettingsBase: container.NetworkSettingsBase{
						Ports: nat.PortMap{
							"8080/tcp": []nat.PortBinding{
								{HostIP: "127.0.0.1", HostPort: "8080"},
							},
						},
					},
				},
			},
			wantRiskLevel: RiskNone,
			wantErr:       false,
		},
		{
			name:          "Nil container",
			containerJSON: nil,
			wantRiskLevel: RiskNone,
			wantErr:       true,
		},
		{
			name: "Container with no network settings",
			containerJSON: &container.InspectResponse{
				NetworkSettings: nil,
			},
			wantRiskLevel: RiskNone,
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := InspectContainerPorts(tt.containerJSON)
			if (err != nil) != tt.wantErr {
				t.Errorf("InspectContainerPorts() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil && result.OverallRisk != tt.wantRiskLevel {
				t.Errorf("InspectContainerPorts() OverallRisk = %v, want %v", result.OverallRisk, tt.wantRiskLevel)
			}
		})
	}
}
