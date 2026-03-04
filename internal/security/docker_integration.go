package security

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/docker/docker/client"
)

var log = slog.With("package", "security")

// CheckContainerPortExposure inspects a Docker container by name/ID and analyzes port exposure
func CheckContainerPortExposure(ctx context.Context, dockerClient *client.Client, containerNameOrID string) (*PortExposureAnalysis, error) {
	if dockerClient == nil {
		return nil, fmt.Errorf("docker client is nil")
	}

	if containerNameOrID == "" {
		return nil, fmt.Errorf("container name or ID is required")
	}

	// Inspect the container
	containerJSON, err := dockerClient.ContainerInspect(ctx, containerNameOrID)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container %s: %w", containerNameOrID, err)
	}

	// Analyze port bindings
	analysis, err := InspectContainerPorts(&containerJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze port bindings: %w", err)
	}

	return &analysis, nil
}

// ExtractContainerID extracts container ID or name from a detected host string
// Host can be container name (for Docker DNS) or IP address
func ExtractContainerID(detectedHost string, source string) string {
	// If source is Docker and host looks like a container name (not an IP), use it
	if source == "docker" && !isIPAddress(detectedHost) {
		return detectedHost
	}
	return ""
}

// isIPAddress checks if a string looks like an IP address
func isIPAddress(host string) bool {
	// Simple check: IPs contain dots or colons, container names typically don't
	return strings.Contains(host, ".") || strings.Contains(host, ":")
}

// ShouldCheckPortExposure determines if we should check port exposure for a proposal
func ShouldCheckPortExposure(source string, dockerClient *client.Client) bool {
	// Only check for Docker discoveries when Docker client is available
	return source == "docker" && dockerClient != nil
}

// LogPortExposureCheck logs the port exposure check results
func LogPortExposureCheck(appID string, analysis *PortExposureAnalysis) {
	if analysis == nil {
		return
	}

	switch analysis.OverallRisk {
	case RiskCritical:
		log.Warn("app has CRITICAL port exposure risk",
			"app_id", appID,
			"summary", analysis.Summary,
			"component", "security")
	case RiskWarning:
		log.Info("app has port exposure warning",
			"app_id", appID,
			"summary", analysis.Summary,
			"component", "security")
	case RiskNone:
		log.Debug("app port exposure is safe",
			"app_id", appID,
			"summary", analysis.Summary,
			"component", "security")
	}
}
