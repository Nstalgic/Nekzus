package security

import (
	"fmt"
	"net"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
)

// RiskLevel represents the security risk level of a port binding
type RiskLevel string

const (
	// RiskNone indicates no security risk (localhost binding or no exposure)
	RiskNone RiskLevel = "none"
	// RiskWarning indicates potential risk (private network exposure)
	RiskWarning RiskLevel = "warning"
	// RiskCritical indicates critical security risk (public exposure)
	RiskCritical RiskLevel = "critical"
)

// PortBinding represents a single port binding with risk analysis
type PortBinding struct {
	Port     string    // e.g., "8080/tcp"
	HostIP   string    // e.g., "0.0.0.0", "127.0.0.1"
	HostPort string    // e.g., "8080"
	Risk     RiskLevel // Risk level for this binding
	Message  string    // Human-readable message
}

// PortExposureAnalysis contains the complete analysis of container port bindings
type PortExposureAnalysis struct {
	OverallRisk       RiskLevel     // Highest risk level found
	HasPublicExposure bool          // True if any port is exposed to public internet
	Bindings          []PortBinding // All port bindings analyzed
	Recommendations   []string      // Security recommendations
	Summary           string        // Human-readable summary
}

// AnalyzePortBindings analyzes Docker port bindings for security risks
func AnalyzePortBindings(ports nat.PortMap) PortExposureAnalysis {
	analysis := PortExposureAnalysis{
		OverallRisk:       RiskNone,
		HasPublicExposure: false,
		Bindings:          []PortBinding{},
		Recommendations:   []string{},
	}

	if len(ports) == 0 {
		analysis.Summary = "No ports exposed"
		return analysis
	}

	// Analyze each port binding
	for port, bindings := range ports {
		if len(bindings) == 0 {
			// Port defined but not bound (internal only)
			continue
		}

		for _, binding := range bindings {
			risk := classifyBindingRisk(binding.HostIP)

			portBinding := PortBinding{
				Port:     string(port),
				HostIP:   binding.HostIP,
				HostPort: binding.HostPort,
				Risk:     risk,
				Message:  generateBindingMessage(binding.HostIP, binding.HostPort, risk),
			}

			analysis.Bindings = append(analysis.Bindings, portBinding)

			// Update overall risk (take highest)
			if risk == RiskCritical {
				analysis.OverallRisk = RiskCritical
				analysis.HasPublicExposure = true
			} else if risk == RiskWarning && analysis.OverallRisk == RiskNone {
				analysis.OverallRisk = RiskWarning
			}
		}
	}

	// Generate recommendations
	analysis.Recommendations = getRecommendations(analysis.OverallRisk, analysis.HasPublicExposure)

	// Generate summary
	analysis.Summary = generateSummary(analysis)

	return analysis
}

// InspectContainerPorts inspects a Docker container's port configuration
func InspectContainerPorts(containerJSON *container.InspectResponse) (PortExposureAnalysis, error) {
	if containerJSON == nil {
		return PortExposureAnalysis{}, fmt.Errorf("container inspect response is nil")
	}

	if containerJSON.NetworkSettings == nil {
		return PortExposureAnalysis{
			OverallRisk: RiskNone,
			Summary:     "No network settings found",
		}, nil
	}

	return AnalyzePortBindings(containerJSON.NetworkSettings.Ports), nil
}

// classifyBindingRisk determines the risk level of a host IP binding
func classifyBindingRisk(hostIP string) RiskLevel {
	// Normalize empty to 0.0.0.0
	if hostIP == "" {
		hostIP = "0.0.0.0"
	}

	// Safe: localhost bindings
	if isLocalhost(hostIP) {
		return RiskNone
	}

	// Critical: all interfaces (0.0.0.0, ::)
	if isAllInterfaces(hostIP) {
		return RiskCritical
	}

	// Parse IP
	ip := net.ParseIP(hostIP)
	if ip == nil {
		// If we can't parse it, treat as localhost names
		if strings.ToLower(hostIP) == "localhost" {
			return RiskNone
		}
		// Unknown format, assume risky
		return RiskCritical
	}

	// Warning: private network ranges
	if isPrivateIP(ip) {
		return RiskWarning
	}

	// Critical: public IP
	return RiskCritical
}

// isLocalhost checks if the IP is a localhost address
func isLocalhost(hostIP string) bool {
	return hostIP == "127.0.0.1" ||
		hostIP == "::1" ||
		strings.ToLower(hostIP) == "localhost"
}

// isAllInterfaces checks if the IP represents all network interfaces
func isAllInterfaces(hostIP string) bool {
	return hostIP == "0.0.0.0" || hostIP == "::"
}

// isPrivateIP checks if an IP is in a private network range
func isPrivateIP(ip net.IP) bool {
	// RFC 1918 private ranges
	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
	}

	for _, cidr := range privateRanges {
		_, subnet, _ := net.ParseCIDR(cidr)
		if subnet.Contains(ip) {
			return true
		}
	}

	return false
}

// generateBindingMessage creates a human-readable message for a binding
func generateBindingMessage(hostIP, hostPort string, risk RiskLevel) string {
	if hostIP == "" {
		hostIP = "0.0.0.0"
	}

	switch risk {
	case RiskNone:
		return fmt.Sprintf("Port %s bound to %s (localhost only, safe)", hostPort, hostIP)
	case RiskWarning:
		return fmt.Sprintf("Port %s bound to %s (private network exposure)", hostPort, hostIP)
	case RiskCritical:
		if isAllInterfaces(hostIP) {
			return fmt.Sprintf("Port %s bound to %s (PUBLICLY ACCESSIBLE)", hostPort, hostIP)
		}
		return fmt.Sprintf("Port %s bound to %s (public IP, EXTERNAL ACCESS)", hostPort, hostIP)
	default:
		return fmt.Sprintf("Port %s bound to %s", hostPort, hostIP)
	}
}

// getRecommendations generates security recommendations based on risk analysis
func getRecommendations(risk RiskLevel, hasPublic bool) []string {
	recommendations := []string{}

	switch risk {
	case RiskCritical:
		if hasPublic {
			recommendations = append(recommendations,
				"CRITICAL: Ports are exposed to the public internet",
				"Ensure the service has proper authentication enabled",
				"Consider using firewall rules to restrict access",
				"Change Docker port binding to 127.0.0.1 if external access is not needed",
				"Use Nekzus proxy for secure authenticated access instead of direct exposure",
			)
		}
	case RiskWarning:
		recommendations = append(recommendations,
			"Ports are accessible on your local network",
			"Ensure the service has authentication if accessed by multiple devices",
			"Consider using Nekzus proxy for centralized access control",
		)
	case RiskNone:
		// No recommendations needed
	}

	return recommendations
}

// generateSummary creates a human-readable summary of the analysis
func generateSummary(analysis PortExposureAnalysis) string {
	if len(analysis.Bindings) == 0 {
		return "No ports exposed"
	}

	switch analysis.OverallRisk {
	case RiskCritical:
		publicCount := 0
		for _, binding := range analysis.Bindings {
			if binding.Risk == RiskCritical {
				publicCount++
			}
		}
		return fmt.Sprintf("SECURITY WARNING: %d port(s) publicly exposed to the internet", publicCount)
	case RiskWarning:
		return fmt.Sprintf("%d port(s) exposed on private network", len(analysis.Bindings))
	case RiskNone:
		return fmt.Sprintf("%d port(s) bound to localhost (safe)", len(analysis.Bindings))
	default:
		return fmt.Sprintf("%d port(s) analyzed", len(analysis.Bindings))
	}
}
