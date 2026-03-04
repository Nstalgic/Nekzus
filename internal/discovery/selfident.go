package discovery

import (
	"os"
	"strings"
)

// SelfIdentity holds information about the current running instance
type SelfIdentity struct {
	ContainerID   string
	ContainerName string
	Hostname      string
}

// DetectSelfIdentity attempts to determine if we're running in a container
// and extracts identifying information
func DetectSelfIdentity() *SelfIdentity {
	ident := &SelfIdentity{}

	// Get hostname (works both in containers and bare metal)
	if hostname, err := os.Hostname(); err == nil {
		ident.Hostname = hostname
	}

	// Try to detect container ID from cgroup (Docker/Kubernetes)
	if containerID := detectContainerIDFromCgroup(); containerID != "" {
		ident.ContainerID = containerID
	}

	// Try to detect from Docker environment variables
	if containerID := os.Getenv("HOSTNAME"); containerID != "" {
		// In Docker, HOSTNAME is often set to the container ID or name
		// If it looks like a container ID (12+ hex chars), use it
		if len(containerID) >= 12 && isHexString(containerID[:12]) {
			if ident.ContainerID == "" {
				ident.ContainerID = containerID
			}
		}
		// Also use it as container name if it's not a hex ID
		if !isHexString(containerID) {
			ident.ContainerName = containerID
		}
	}

	return ident
}

// detectContainerIDFromCgroup reads /proc/self/cgroup to extract container ID
func detectContainerIDFromCgroup() string {
	// Read /proc/self/cgroup
	data, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return ""
	}

	// Parse cgroup format:
	// Docker: 12:perf_event:/docker/abc123def456...
	// Kubernetes: 11:devices:/kubepods/besteffort/pod.../abc123def456
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.Contains(line, "/docker/") {
			// Extract container ID from Docker cgroup
			parts := strings.Split(line, "/docker/")
			if len(parts) >= 2 {
				containerID := strings.TrimSpace(parts[1])
				// Container ID might have additional path components, take first segment
				if idx := strings.Index(containerID, "/"); idx > 0 {
					containerID = containerID[:idx]
				}
				// Docker IDs are 64 hex chars, but we only need first 12 for matching
				if len(containerID) >= 12 {
					return containerID[:12]
				}
			}
		} else if strings.Contains(line, "/kubepods/") {
			// Extract from Kubernetes cgroup
			// Format: .../kubepods/besteffort/pod.../containerID
			parts := strings.Split(line, "/")
			if len(parts) > 0 {
				lastPart := parts[len(parts)-1]
				// Remove .scope suffix if present
				lastPart = strings.TrimSuffix(lastPart, ".scope")
				// Check if it looks like a container ID
				if len(lastPart) >= 12 && isHexString(lastPart[:12]) {
					return lastPart[:12]
				}
			}
		}
	}

	return ""
}

// isHexString checks if a string contains only hexadecimal characters
func isHexString(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
