// Package export provides functionality to export Docker container configurations
// to Docker Compose YAML format.
package export

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
	"gopkg.in/yaml.v3"
)

// sensitiveKeyPatterns contains patterns to detect sensitive environment variables
var sensitiveKeyPatterns = []string{
	"PASSWORD", "PASSWD", "PWD",
	"SECRET", "PRIVATE_KEY", "API_KEY", "ACCESS_KEY",
	"TOKEN", "AUTH", "CREDENTIAL",
	"CERTIFICATE", "CERT",
}

// ExportOptions configures the export behavior
type ExportOptions struct {
	SanitizeSecrets bool // Redact sensitive environment variables
	IncludeVolumes  bool // Include volume configurations
	IncludeNetworks bool // Include network configurations
}

// Validate validates the export options
func (o *ExportOptions) Validate() error {
	return nil
}

// ExportResult contains the exported compose file and metadata
type ExportResult struct {
	Format      string   `json:"format"`
	Content     string   `json:"content"`
	Filename    string   `json:"filename"`
	Warnings    []string `json:"warnings,omitempty"`
	EnvContent  string   `json:"env_content,omitempty"`
	EnvFilename string   `json:"env_filename,omitempty"`
}

// VolumeConfig represents a volume in the compose file
type VolumeConfig struct {
	Type     string `yaml:"type,omitempty"`
	Source   string `yaml:"source,omitempty"`
	Target   string `yaml:"target"`
	ReadOnly bool   `yaml:"read_only,omitempty"`
}

// NetworkConfig represents a network configuration for a service
type NetworkConfig struct {
	Aliases []string `yaml:"aliases,omitempty"`
}

// HealthCheckConfig represents a health check in the compose file
type HealthCheckConfig struct {
	Test        []string `yaml:"test,omitempty"`
	Interval    string   `yaml:"interval,omitempty"`
	Timeout     string   `yaml:"timeout,omitempty"`
	Retries     int      `yaml:"retries,omitempty"`
	StartPeriod string   `yaml:"start_period,omitempty"`
}

// ServiceConfig represents a service in the compose file
type ServiceConfig struct {
	Image         string                    `yaml:"image"`
	ContainerName string                    `yaml:"container_name,omitempty"`
	Environment   map[string]string         `yaml:"environment,omitempty"`
	Ports         []string                  `yaml:"ports,omitempty"`
	Volumes       []VolumeConfig            `yaml:"volumes,omitempty"`
	Networks      map[string]*NetworkConfig `yaml:"networks,omitempty"`
	Restart       string                    `yaml:"restart,omitempty"`
	Labels        map[string]string         `yaml:"labels,omitempty"`
	HealthCheck   *HealthCheckConfig        `yaml:"healthcheck,omitempty"`
	Privileged    bool                      `yaml:"privileged,omitempty"`
	CapAdd        []string                  `yaml:"cap_add,omitempty"`
	CapDrop       []string                  `yaml:"cap_drop,omitempty"`
	User          string                    `yaml:"user,omitempty"`
	WorkingDir    string                    `yaml:"working_dir,omitempty"`
	Entrypoint    []string                  `yaml:"entrypoint,omitempty"`
	Command       []string                  `yaml:"command,omitempty"`
}

// ComposeFile represents the complete docker-compose.yml structure
type ComposeFile struct {
	Services map[string]ServiceConfig     `yaml:"services"`
	Volumes  map[string]map[string]string `yaml:"volumes,omitempty"`
	Networks map[string]map[string]string `yaml:"networks,omitempty"`
}

// IsSensitiveKey checks if an environment variable key contains sensitive data
func IsSensitiveKey(key string) bool {
	upperKey := strings.ToUpper(key)
	for _, pattern := range sensitiveKeyPatterns {
		if strings.Contains(upperKey, pattern) {
			return true
		}
	}
	return false
}

// CollectSensitiveVars extracts sensitive environment variables from an env list
func CollectSensitiveVars(env []string) map[string]string {
	result := make(map[string]string)
	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key, value := parts[0], parts[1]
		if IsSensitiveKey(key) {
			result[key] = value
		}
	}
	return result
}

// GenerateEnvFile creates the content for a .env file with CHANGE_ME placeholders
func GenerateEnvFile(sensitiveVars map[string]string) string {
	if len(sensitiveVars) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("# Environment variables for Docker Compose\n")
	sb.WriteString("# IMPORTANT: Replace CHANGE_ME values with actual secrets before use\n")
	sb.WriteString("# Do not commit this file to version control with real values\n\n")

	// Sort keys for consistent output
	keys := make([]string, 0, len(sensitiveVars))
	for k := range sensitiveVars {
		keys = append(keys, k)
	}
	// Simple sort
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] > keys[j] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}

	for _, key := range keys {
		sb.WriteString(fmt.Sprintf("%s=CHANGE_ME\n", key))
	}

	return sb.String()
}

// SanitizeContainerName removes the leading slash from container names
func SanitizeContainerName(name string) string {
	return strings.TrimPrefix(name, "/")
}

// MapEnvironmentWithEnvFile maps Docker environment variables to compose format,
// using simple ${VAR} references for sensitive vars (for use with .env file)
func MapEnvironmentWithEnvFile(env []string, sanitizeSecrets bool) (map[string]string, map[string]string) {
	result := make(map[string]string)
	sensitiveVars := make(map[string]string)

	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key, value := parts[0], parts[1]

		if sanitizeSecrets && IsSensitiveKey(key) {
			// Use simple variable reference - value will be in .env file
			result[key] = fmt.Sprintf("${%s}", key)
			sensitiveVars[key] = value
		} else {
			result[key] = value
		}
	}

	return result, sensitiveVars
}

// MapEnvironment maps Docker environment variables to compose format
// Returns the mapped environment and any warnings about sanitized values
func MapEnvironment(env []string, sanitizeSecrets bool) (map[string]string, []string) {
	result := make(map[string]string)
	var warnings []string

	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) != 2 {
			continue // Skip malformed entries
		}

		key, value := parts[0], parts[1]

		if sanitizeSecrets && IsSensitiveKey(key) {
			result[key] = fmt.Sprintf("${%s:?Required}", key)
			warnings = append(warnings, fmt.Sprintf("Environment variable '%s' was redacted for security", key))
		} else {
			result[key] = value
		}
	}

	return result, warnings
}

// MapVolumes maps Docker mount points to compose volume format
func MapVolumes(mounts []container.MountPoint) []VolumeConfig {
	var volumes []VolumeConfig

	for _, m := range mounts {
		vol := VolumeConfig{
			Target:   m.Destination,
			ReadOnly: !m.RW,
		}

		switch m.Type {
		case mount.TypeVolume:
			vol.Type = "volume"
			vol.Source = m.Name
		case mount.TypeBind:
			vol.Type = "bind"
			vol.Source = m.Source
		case mount.TypeTmpfs:
			vol.Type = "tmpfs"
			vol.Source = ""
		default:
			vol.Type = string(m.Type)
			vol.Source = m.Source
		}

		volumes = append(volumes, vol)
	}

	return volumes
}

// MapPorts maps Docker port bindings to compose port format
func MapPorts(portBindings nat.PortMap) []string {
	var ports []string

	for containerPort, bindings := range portBindings {
		for _, binding := range bindings {
			port := containerPort.Port()
			proto := containerPort.Proto()

			var portStr string

			// Build port string
			if binding.HostIP != "" && binding.HostIP != "0.0.0.0" {
				portStr = fmt.Sprintf("%s:%s:%s", binding.HostIP, binding.HostPort, port)
			} else {
				portStr = fmt.Sprintf("%s:%s", binding.HostPort, port)
			}

			// Add protocol suffix if not tcp
			if proto != "tcp" {
				portStr = fmt.Sprintf("%s/%s", portStr, proto)
			}

			ports = append(ports, portStr)
		}
	}

	return ports
}

// MapRestartPolicy maps Docker restart policy to compose format
func MapRestartPolicy(policy container.RestartPolicy) string {
	switch policy.Name {
	case "always":
		return "always"
	case "unless-stopped":
		return "unless-stopped"
	case "on-failure":
		if policy.MaximumRetryCount > 0 {
			return fmt.Sprintf("on-failure:%d", policy.MaximumRetryCount)
		}
		return "on-failure"
	case "no", "":
		return "no"
	default:
		return string(policy.Name)
	}
}

// MapNetworks maps Docker network settings to compose format
func MapNetworks(networks map[string]*network.EndpointSettings) map[string]*NetworkConfig {
	result := make(map[string]*NetworkConfig)

	for name, settings := range networks {
		// Skip the default bridge network
		if name == "bridge" {
			continue
		}

		netConfig := &NetworkConfig{}
		if settings != nil && len(settings.Aliases) > 0 {
			netConfig.Aliases = settings.Aliases
		}

		result[name] = netConfig
	}

	return result
}

// MapHealthCheck maps Docker health check config to compose format
func MapHealthCheck(healthCheck *container.HealthConfig) *HealthCheckConfig {
	if healthCheck == nil || len(healthCheck.Test) == 0 {
		return nil
	}

	result := &HealthCheckConfig{
		Test:    healthCheck.Test,
		Retries: healthCheck.Retries,
	}

	if healthCheck.Interval > 0 {
		result.Interval = healthCheck.Interval.String()
	}
	if healthCheck.Timeout > 0 {
		result.Timeout = healthCheck.Timeout.String()
	}
	if healthCheck.StartPeriod > 0 {
		result.StartPeriod = healthCheck.StartPeriod.String()
	}

	return result
}

// CollectTopLevelVolumes extracts named volumes for the top-level volumes section
func CollectTopLevelVolumes(volumes []VolumeConfig) map[string]map[string]string {
	result := make(map[string]map[string]string)

	for _, v := range volumes {
		if v.Type == "volume" && v.Source != "" {
			// Named volumes go in the top-level volumes section
			result[v.Source] = map[string]string{}
		}
	}

	return result
}

// CollectTopLevelNetworks extracts networks for the top-level networks section
func CollectTopLevelNetworks(networks map[string]*NetworkConfig) map[string]map[string]string {
	result := make(map[string]map[string]string)

	for name := range networks {
		result[name] = map[string]string{}
	}

	return result
}

// ExportToCompose exports a container configuration to Docker Compose YAML format
func ExportToCompose(containerJSON *container.InspectResponse, options ExportOptions) (*ExportResult, error) {
	if containerJSON == nil {
		return nil, fmt.Errorf("container inspection data is required")
	}

	var warnings []string

	// Get sanitized container name
	containerName := SanitizeContainerName(containerJSON.Name)

	// Create service config
	service := ServiceConfig{
		ContainerName: containerName,
	}

	// Map image
	if containerJSON.Config != nil {
		service.Image = containerJSON.Config.Image

		// Map environment variables
		env, envWarnings := MapEnvironment(containerJSON.Config.Env, options.SanitizeSecrets)
		if len(env) > 0 {
			service.Environment = env
		}
		warnings = append(warnings, envWarnings...)

		// Map labels (exclude compose-internal labels)
		if len(containerJSON.Config.Labels) > 0 {
			labels := make(map[string]string)
			for k, v := range containerJSON.Config.Labels {
				// Skip Docker Compose internal labels
				if strings.HasPrefix(k, "com.docker.compose.") {
					continue
				}
				labels[k] = v
			}
			if len(labels) > 0 {
				service.Labels = labels
			}
		}

		// Map user
		if containerJSON.Config.User != "" {
			service.User = containerJSON.Config.User
		}

		// Map working directory
		if containerJSON.Config.WorkingDir != "" {
			service.WorkingDir = containerJSON.Config.WorkingDir
		}

		// Map entrypoint
		if len(containerJSON.Config.Entrypoint) > 0 {
			service.Entrypoint = containerJSON.Config.Entrypoint
		}

		// Map command
		if len(containerJSON.Config.Cmd) > 0 {
			service.Command = containerJSON.Config.Cmd
		}

		// Map health check
		if containerJSON.Config.Healthcheck != nil {
			service.HealthCheck = MapHealthCheck(containerJSON.Config.Healthcheck)
		}
	}

	// Map host config settings
	if containerJSON.HostConfig != nil {
		// Map ports
		ports := MapPorts(containerJSON.HostConfig.PortBindings)
		if len(ports) > 0 {
			service.Ports = ports
		}

		// Map restart policy
		service.Restart = MapRestartPolicy(containerJSON.HostConfig.RestartPolicy)

		// Map privileged mode
		if containerJSON.HostConfig.Privileged {
			service.Privileged = true
			warnings = append(warnings, "Container runs in privileged mode (security risk)")
		}

		// Map capabilities
		if len(containerJSON.HostConfig.CapAdd) > 0 {
			service.CapAdd = containerJSON.HostConfig.CapAdd
		}
		if len(containerJSON.HostConfig.CapDrop) > 0 {
			service.CapDrop = containerJSON.HostConfig.CapDrop
		}
	}

	// Map volumes
	var topLevelVolumes map[string]map[string]string
	if options.IncludeVolumes && len(containerJSON.Mounts) > 0 {
		volumes := MapVolumes(containerJSON.Mounts)
		if len(volumes) > 0 {
			service.Volumes = volumes
			topLevelVolumes = CollectTopLevelVolumes(volumes)
		}
	}

	// Map networks
	var topLevelNetworks map[string]map[string]string
	if options.IncludeNetworks && containerJSON.NetworkSettings != nil && len(containerJSON.NetworkSettings.Networks) > 0 {
		networks := MapNetworks(containerJSON.NetworkSettings.Networks)
		if len(networks) > 0 {
			service.Networks = networks
			topLevelNetworks = CollectTopLevelNetworks(networks)
		}
	}

	// Build compose file
	compose := ComposeFile{
		Services: map[string]ServiceConfig{
			containerName: service,
		},
	}

	if len(topLevelVolumes) > 0 {
		compose.Volumes = topLevelVolumes
	}

	if len(topLevelNetworks) > 0 {
		compose.Networks = topLevelNetworks
	}

	// Marshal to YAML
	yamlBytes, err := yaml.Marshal(compose)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal compose file: %w", err)
	}

	return &ExportResult{
		Format:   "compose",
		Content:  string(yamlBytes),
		Filename: fmt.Sprintf("%s-compose.yml", containerName),
		Warnings: warnings,
	}, nil
}

// BatchExportToCompose exports multiple container configurations to a single Docker Compose YAML file
// with deduplicated networks and volumes
func BatchExportToCompose(containers []*container.InspectResponse, options ExportOptions, stackName string) (*ExportResult, error) {
	if len(containers) == 0 {
		return nil, fmt.Errorf("at least one container is required for batch export")
	}

	var warnings []string
	services := make(map[string]ServiceConfig)
	allVolumes := make(map[string]map[string]string)
	allNetworks := make(map[string]map[string]string)
	usedNames := make(map[string]bool)

	for _, containerJSON := range containers {
		if containerJSON == nil {
			continue
		}

		// Get sanitized container name
		containerName := SanitizeContainerName(containerJSON.Name)

		// Handle name collisions by appending a suffix
		originalName := containerName
		suffix := 1
		for usedNames[containerName] {
			containerName = fmt.Sprintf("%s-%d", originalName, suffix)
			suffix++
		}
		usedNames[containerName] = true

		// Create service config
		service := ServiceConfig{
			ContainerName: originalName, // Keep original for container_name
		}

		// Map image and config
		if containerJSON.Config != nil {
			service.Image = containerJSON.Config.Image

			// Map environment variables
			env, envWarnings := MapEnvironment(containerJSON.Config.Env, options.SanitizeSecrets)
			if len(env) > 0 {
				service.Environment = env
			}
			warnings = append(warnings, envWarnings...)

			// Map labels (exclude compose-internal labels)
			if len(containerJSON.Config.Labels) > 0 {
				labels := make(map[string]string)
				for k, v := range containerJSON.Config.Labels {
					if strings.HasPrefix(k, "com.docker.compose.") {
						continue
					}
					labels[k] = v
				}
				if len(labels) > 0 {
					service.Labels = labels
				}
			}

			// Map user
			if containerJSON.Config.User != "" {
				service.User = containerJSON.Config.User
			}

			// Map working directory
			if containerJSON.Config.WorkingDir != "" {
				service.WorkingDir = containerJSON.Config.WorkingDir
			}

			// Map entrypoint
			if len(containerJSON.Config.Entrypoint) > 0 {
				service.Entrypoint = containerJSON.Config.Entrypoint
			}

			// Map command
			if len(containerJSON.Config.Cmd) > 0 {
				service.Command = containerJSON.Config.Cmd
			}

			// Map health check
			if containerJSON.Config.Healthcheck != nil {
				service.HealthCheck = MapHealthCheck(containerJSON.Config.Healthcheck)
			}
		}

		// Map host config settings
		if containerJSON.HostConfig != nil {
			// Map ports
			ports := MapPorts(containerJSON.HostConfig.PortBindings)
			if len(ports) > 0 {
				service.Ports = ports
			}

			// Map restart policy
			service.Restart = MapRestartPolicy(containerJSON.HostConfig.RestartPolicy)

			// Map privileged mode
			if containerJSON.HostConfig.Privileged {
				service.Privileged = true
				warnings = append(warnings, fmt.Sprintf("Container '%s' runs in privileged mode (security risk)", originalName))
			}

			// Map capabilities
			if len(containerJSON.HostConfig.CapAdd) > 0 {
				service.CapAdd = containerJSON.HostConfig.CapAdd
			}
			if len(containerJSON.HostConfig.CapDrop) > 0 {
				service.CapDrop = containerJSON.HostConfig.CapDrop
			}
		}

		// Map volumes and collect for deduplication
		if options.IncludeVolumes && len(containerJSON.Mounts) > 0 {
			volumes := MapVolumes(containerJSON.Mounts)
			if len(volumes) > 0 {
				service.Volumes = volumes
				// Collect top-level volumes (deduplicated via map)
				for volName, volConfig := range CollectTopLevelVolumes(volumes) {
					allVolumes[volName] = volConfig
				}
			}
		}

		// Map networks and collect for deduplication
		if options.IncludeNetworks && containerJSON.NetworkSettings != nil && len(containerJSON.NetworkSettings.Networks) > 0 {
			networks := MapNetworks(containerJSON.NetworkSettings.Networks)
			if len(networks) > 0 {
				service.Networks = networks
				// Collect top-level networks (deduplicated via map)
				for netName, netConfig := range CollectTopLevelNetworks(networks) {
					allNetworks[netName] = netConfig
				}
			}
		}

		services[containerName] = service
	}

	// Build compose file
	compose := ComposeFile{
		Services: services,
	}

	if len(allVolumes) > 0 {
		compose.Volumes = allVolumes
	}

	if len(allNetworks) > 0 {
		compose.Networks = allNetworks
	}

	// Marshal to YAML
	yamlBytes, err := yaml.Marshal(compose)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal compose file: %w", err)
	}

	// Generate filename from stack name or first container
	filename := stackName
	if filename == "" {
		// Use first container name as fallback
		for name := range services {
			filename = name
			break
		}
	}

	return &ExportResult{
		Format:   "compose",
		Content:  string(yamlBytes),
		Filename: fmt.Sprintf("%s-compose.yml", filename),
		Warnings: warnings,
	}, nil
}

// ExportToComposeWithEnv exports a container configuration to Docker Compose YAML format
// with a separate .env file for sensitive variables
func ExportToComposeWithEnv(containerJSON *container.InspectResponse, options ExportOptions) (*ExportResult, error) {
	if containerJSON == nil {
		return nil, fmt.Errorf("container inspection data is required")
	}

	var warnings []string
	allSensitiveVars := make(map[string]string)

	// Get sanitized container name
	containerName := SanitizeContainerName(containerJSON.Name)

	// Create service config
	service := ServiceConfig{
		ContainerName: containerName,
	}

	// Map image
	if containerJSON.Config != nil {
		service.Image = containerJSON.Config.Image

		// Map environment variables with env file support
		if options.SanitizeSecrets {
			env, sensitiveVars := MapEnvironmentWithEnvFile(containerJSON.Config.Env, true)
			if len(env) > 0 {
				service.Environment = env
			}
			// Collect sensitive vars for env file
			for k, v := range sensitiveVars {
				allSensitiveVars[k] = v
			}
		} else {
			env, envWarnings := MapEnvironment(containerJSON.Config.Env, false)
			if len(env) > 0 {
				service.Environment = env
			}
			warnings = append(warnings, envWarnings...)
		}

		// Map labels (exclude compose-internal labels)
		if len(containerJSON.Config.Labels) > 0 {
			labels := make(map[string]string)
			for k, v := range containerJSON.Config.Labels {
				if strings.HasPrefix(k, "com.docker.compose.") {
					continue
				}
				labels[k] = v
			}
			if len(labels) > 0 {
				service.Labels = labels
			}
		}

		// Map user
		if containerJSON.Config.User != "" {
			service.User = containerJSON.Config.User
		}

		// Map working directory
		if containerJSON.Config.WorkingDir != "" {
			service.WorkingDir = containerJSON.Config.WorkingDir
		}

		// Map entrypoint
		if len(containerJSON.Config.Entrypoint) > 0 {
			service.Entrypoint = containerJSON.Config.Entrypoint
		}

		// Map command
		if len(containerJSON.Config.Cmd) > 0 {
			service.Command = containerJSON.Config.Cmd
		}

		// Map health check
		if containerJSON.Config.Healthcheck != nil {
			service.HealthCheck = MapHealthCheck(containerJSON.Config.Healthcheck)
		}
	}

	// Map host config settings
	if containerJSON.HostConfig != nil {
		// Map ports
		ports := MapPorts(containerJSON.HostConfig.PortBindings)
		if len(ports) > 0 {
			service.Ports = ports
		}

		// Map restart policy
		service.Restart = MapRestartPolicy(containerJSON.HostConfig.RestartPolicy)

		// Map privileged mode
		if containerJSON.HostConfig.Privileged {
			service.Privileged = true
			warnings = append(warnings, "Container runs in privileged mode (security risk)")
		}

		// Map capabilities
		if len(containerJSON.HostConfig.CapAdd) > 0 {
			service.CapAdd = containerJSON.HostConfig.CapAdd
		}
		if len(containerJSON.HostConfig.CapDrop) > 0 {
			service.CapDrop = containerJSON.HostConfig.CapDrop
		}
	}

	// Map volumes
	var topLevelVolumes map[string]map[string]string
	if options.IncludeVolumes && len(containerJSON.Mounts) > 0 {
		volumes := MapVolumes(containerJSON.Mounts)
		if len(volumes) > 0 {
			service.Volumes = volumes
			topLevelVolumes = CollectTopLevelVolumes(volumes)
		}
	}

	// Map networks
	var topLevelNetworks map[string]map[string]string
	if options.IncludeNetworks && containerJSON.NetworkSettings != nil && len(containerJSON.NetworkSettings.Networks) > 0 {
		networks := MapNetworks(containerJSON.NetworkSettings.Networks)
		if len(networks) > 0 {
			service.Networks = networks
			topLevelNetworks = CollectTopLevelNetworks(networks)
		}
	}

	// Build compose file
	compose := ComposeFile{
		Services: map[string]ServiceConfig{
			containerName: service,
		},
	}

	if len(topLevelVolumes) > 0 {
		compose.Volumes = topLevelVolumes
	}

	if len(topLevelNetworks) > 0 {
		compose.Networks = topLevelNetworks
	}

	// Marshal to YAML
	yamlBytes, err := yaml.Marshal(compose)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal compose file: %w", err)
	}

	result := &ExportResult{
		Format:   "compose",
		Content:  string(yamlBytes),
		Filename: fmt.Sprintf("%s-compose.yml", containerName),
		Warnings: warnings,
	}

	// Generate env file if there are sensitive vars
	if len(allSensitiveVars) > 0 {
		result.EnvContent = GenerateEnvFile(allSensitiveVars)
		result.EnvFilename = ".env.example"
	}

	return result, nil
}

// BatchExportToComposeWithEnv exports multiple container configurations to a single Docker Compose YAML file
// with a separate .env file for sensitive variables and deduplicated networks/volumes
func BatchExportToComposeWithEnv(containers []*container.InspectResponse, options ExportOptions, stackName string) (*ExportResult, error) {
	if len(containers) == 0 {
		return nil, fmt.Errorf("at least one container is required for batch export")
	}

	var warnings []string
	services := make(map[string]ServiceConfig)
	allVolumes := make(map[string]map[string]string)
	allNetworks := make(map[string]map[string]string)
	allSensitiveVars := make(map[string]string)
	usedNames := make(map[string]bool)

	for _, containerJSON := range containers {
		if containerJSON == nil {
			continue
		}

		// Get sanitized container name
		containerName := SanitizeContainerName(containerJSON.Name)

		// Handle name collisions by appending a suffix
		originalName := containerName
		suffix := 1
		for usedNames[containerName] {
			containerName = fmt.Sprintf("%s-%d", originalName, suffix)
			suffix++
		}
		usedNames[containerName] = true

		// Create service config
		service := ServiceConfig{
			ContainerName: originalName,
		}

		// Map image and config
		if containerJSON.Config != nil {
			service.Image = containerJSON.Config.Image

			// Map environment variables with env file support
			if options.SanitizeSecrets {
				env, sensitiveVars := MapEnvironmentWithEnvFile(containerJSON.Config.Env, true)
				if len(env) > 0 {
					service.Environment = env
				}
				// Collect sensitive vars for env file (deduplicated across containers)
				for k, v := range sensitiveVars {
					allSensitiveVars[k] = v
				}
			} else {
				env, envWarnings := MapEnvironment(containerJSON.Config.Env, false)
				if len(env) > 0 {
					service.Environment = env
				}
				warnings = append(warnings, envWarnings...)
			}

			// Map labels (exclude compose-internal labels)
			if len(containerJSON.Config.Labels) > 0 {
				labels := make(map[string]string)
				for k, v := range containerJSON.Config.Labels {
					if strings.HasPrefix(k, "com.docker.compose.") {
						continue
					}
					labels[k] = v
				}
				if len(labels) > 0 {
					service.Labels = labels
				}
			}

			// Map user
			if containerJSON.Config.User != "" {
				service.User = containerJSON.Config.User
			}

			// Map working directory
			if containerJSON.Config.WorkingDir != "" {
				service.WorkingDir = containerJSON.Config.WorkingDir
			}

			// Map entrypoint
			if len(containerJSON.Config.Entrypoint) > 0 {
				service.Entrypoint = containerJSON.Config.Entrypoint
			}

			// Map command
			if len(containerJSON.Config.Cmd) > 0 {
				service.Command = containerJSON.Config.Cmd
			}

			// Map health check
			if containerJSON.Config.Healthcheck != nil {
				service.HealthCheck = MapHealthCheck(containerJSON.Config.Healthcheck)
			}
		}

		// Map host config settings
		if containerJSON.HostConfig != nil {
			// Map ports
			ports := MapPorts(containerJSON.HostConfig.PortBindings)
			if len(ports) > 0 {
				service.Ports = ports
			}

			// Map restart policy
			service.Restart = MapRestartPolicy(containerJSON.HostConfig.RestartPolicy)

			// Map privileged mode
			if containerJSON.HostConfig.Privileged {
				service.Privileged = true
				warnings = append(warnings, fmt.Sprintf("Container '%s' runs in privileged mode (security risk)", originalName))
			}

			// Map capabilities
			if len(containerJSON.HostConfig.CapAdd) > 0 {
				service.CapAdd = containerJSON.HostConfig.CapAdd
			}
			if len(containerJSON.HostConfig.CapDrop) > 0 {
				service.CapDrop = containerJSON.HostConfig.CapDrop
			}
		}

		// Map volumes and collect for deduplication
		if options.IncludeVolumes && len(containerJSON.Mounts) > 0 {
			volumes := MapVolumes(containerJSON.Mounts)
			if len(volumes) > 0 {
				service.Volumes = volumes
				for volName, volConfig := range CollectTopLevelVolumes(volumes) {
					allVolumes[volName] = volConfig
				}
			}
		}

		// Map networks and collect for deduplication
		if options.IncludeNetworks && containerJSON.NetworkSettings != nil && len(containerJSON.NetworkSettings.Networks) > 0 {
			networks := MapNetworks(containerJSON.NetworkSettings.Networks)
			if len(networks) > 0 {
				service.Networks = networks
				for netName, netConfig := range CollectTopLevelNetworks(networks) {
					allNetworks[netName] = netConfig
				}
			}
		}

		services[containerName] = service
	}

	// Build compose file
	compose := ComposeFile{
		Services: services,
	}

	if len(allVolumes) > 0 {
		compose.Volumes = allVolumes
	}

	if len(allNetworks) > 0 {
		compose.Networks = allNetworks
	}

	// Marshal to YAML
	yamlBytes, err := yaml.Marshal(compose)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal compose file: %w", err)
	}

	// Generate filename from stack name or first container
	filename := stackName
	if filename == "" {
		for name := range services {
			filename = name
			break
		}
	}

	result := &ExportResult{
		Format:   "compose",
		Content:  string(yamlBytes),
		Filename: fmt.Sprintf("%s-compose.yml", filename),
		Warnings: warnings,
	}

	// Generate env file if there are sensitive vars
	if len(allSensitiveVars) > 0 {
		result.EnvContent = GenerateEnvFile(allSensitiveVars)
		result.EnvFilename = ".env.example"
	}

	return result, nil
}

// CreateZipBundle creates a ZIP archive containing the compose file and optional env file
// Returns the ZIP data as bytes, the filename, and any error
func CreateZipBundle(result *ExportResult, stackName string) ([]byte, string, error) {
	if result == nil {
		return nil, "", fmt.Errorf("export result is required")
	}

	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	// Add compose file
	composeWriter, err := zipWriter.Create(result.Filename)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create compose file in ZIP: %w", err)
	}
	if _, err := composeWriter.Write([]byte(result.Content)); err != nil {
		return nil, "", fmt.Errorf("failed to write compose content: %w", err)
	}

	// Add env file if present
	if result.EnvContent != "" && result.EnvFilename != "" {
		envWriter, err := zipWriter.Create(result.EnvFilename)
		if err != nil {
			return nil, "", fmt.Errorf("failed to create env file in ZIP: %w", err)
		}
		if _, err := envWriter.Write([]byte(result.EnvContent)); err != nil {
			return nil, "", fmt.Errorf("failed to write env content: %w", err)
		}
	}

	// Close the ZIP writer
	if err := zipWriter.Close(); err != nil {
		return nil, "", fmt.Errorf("failed to close ZIP: %w", err)
	}

	// Generate filename
	zipFilename := fmt.Sprintf("%s.zip", stackName)

	return buf.Bytes(), zipFilename, nil
}
