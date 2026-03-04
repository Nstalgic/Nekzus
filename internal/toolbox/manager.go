package toolbox

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/compose-spec/compose-go/v2/loader"
	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/nstalgic/nekzus/internal/types"
	"gopkg.in/yaml.v3"
)

// Manager handles toolbox service catalog and deployments.
type Manager struct {
	catalogPath string // Can be file (YAML) or directory (Compose)
	templates   map[string]*types.ServiceTemplate
	mu          sync.RWMutex
}

// catalogFile represents the structure of the toolbox catalog YAML file (DEPRECATED).
type catalogFile struct {
	Services []*types.ServiceTemplate `yaml:"services"`
}

// NewManager creates a new toolbox manager.
func NewManager(catalogPath string) *Manager {
	return &Manager{
		catalogPath: catalogPath,
		templates:   make(map[string]*types.ServiceTemplate),
	}
}

// LoadCatalog loads service templates from either:
// 1. A directory containing Compose files (NEW: preferred)
// 2. A YAML catalog file (DEPRECATED: backward compatibility)
func (m *Manager) LoadCatalog() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if catalogPath is a directory or file
	info, err := os.Stat(m.catalogPath)
	if err != nil {
		return fmt.Errorf("failed to stat catalog path: %w", err)
	}

	if info.IsDir() {
		// NEW: Load from Docker Compose files
		return m.loadComposeDirectory()
	} else {
		// DEPRECATED: Load from YAML file
		return m.loadYAMLFile()
	}
}

// loadComposeDirectory scans a directory for Compose files and loads service templates.
func (m *Manager) loadComposeDirectory() error {
	// Clear existing templates
	m.templates = make(map[string]*types.ServiceTemplate)

	// Scan directory for subdirectories
	entries, err := os.ReadDir(m.catalogPath)
	if err != nil {
		return fmt.Errorf("failed to read catalog directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		serviceID := entry.Name()
		composePath := filepath.Join(m.catalogPath, serviceID, "docker-compose.yml")

		// Check if docker-compose.yml exists
		if _, err := os.Stat(composePath); os.IsNotExist(err) {
			// Skip directories without docker-compose.yml
			continue
		}

		// Load Compose project
		template, err := m.loadComposeFile(composePath, serviceID)
		if err != nil {
			// Log error but continue with other services
			fmt.Printf("Warning: failed to load service '%s': %v\n", serviceID, err)
			continue
		}

		// Validate required fields
		if template.Name == "" || template.Category == "" {
			fmt.Printf("Warning: skipping service '%s': missing required labels (name, category)\n", serviceID)
			continue
		}

		// Add to catalog
		m.templates[serviceID] = template
	}

	return nil
}

// loadComposeFile loads a single Compose file and extracts metadata.
func (m *Manager) loadComposeFile(composePath string, serviceID string) (*types.ServiceTemplate, error) {
	// Read Compose file
	composeData, err := os.ReadFile(composePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read compose file: %w", err)
	}

	// Parse Compose file using compose-go loader
	configDetails := composetypes.ConfigDetails{
		WorkingDir: filepath.Dir(composePath),
		ConfigFiles: []composetypes.ConfigFile{
			{
				Filename: composePath,
				Content:  composeData,
			},
		},
	}

	// Set project name via option function
	withProjectName := func(opts *loader.Options) {
		opts.SetProjectName(serviceID, false)
	}

	project, err := loader.LoadWithContext(context.Background(), configDetails, withProjectName)
	if err != nil {
		return nil, fmt.Errorf("failed to parse compose file: %w", err)
	}

	// Find the primary service (typically has toolbox labels)
	var primaryService *composetypes.ServiceConfig
	for serviceName, svc := range project.Services {
		// Look for service with toolbox labels
		if _, hasName := svc.Labels[types.ToolboxLabelName]; hasName {
			// Need to create a copy to get a pointer
			svcCopy := svc
			primaryService = &svcCopy
			break
		}
		// Keep track of first service as fallback
		if primaryService == nil {
			svcCopy := svc
			primaryService = &svcCopy
			_ = serviceName // Use serviceName to avoid linter warning
		}
	}

	if primaryService == nil {
		return nil, fmt.Errorf("no services found in compose file")
	}

	// Extract metadata from labels
	template := extractMetadataFromLabels(primaryService.Labels, serviceID)
	template.ComposeProject = project
	template.ComposeFilePath = composePath

	// Extract environment variables from raw YAML (compose-go expands variables, so we need raw content)
	template.EnvVars = extractEnvironmentVariablesFromRaw(string(composeData))

	return template, nil
}

// extractMetadataFromLabels extracts toolbox metadata from service labels.
func extractMetadataFromLabels(labels composetypes.Labels, serviceID string) *types.ServiceTemplate {
	template := &types.ServiceTemplate{
		ID:            serviceID,
		Name:          labels[types.ToolboxLabelName],
		Icon:          labels[types.ToolboxLabelIcon],
		Category:      labels[types.ToolboxLabelCategory],
		Description:   labels[types.ToolboxLabelDescription],
		Documentation: labels[types.ToolboxLabelDocs],
		ImageURL:      labels[types.ToolboxLabelImageURL],
		RepositoryURL: labels[types.ToolboxLabelRepoURL],
	}

	// Parse tags (comma-separated)
	if tagsStr := labels[types.ToolboxLabelTags]; tagsStr != "" {
		tagList := strings.Split(tagsStr, ",")
		template.Tags = make([]string, 0, len(tagList))
		for _, tag := range tagList {
			trimmed := strings.TrimSpace(tag)
			if trimmed != "" {
				template.Tags = append(template.Tags, trimmed)
			}
		}
	}

	return template
}

// extractEnvironmentVariables extracts env var definitions from Compose service.
func extractEnvironmentVariables(svc *composetypes.ServiceConfig) []types.EnvironmentVariable {
	envVars := []types.EnvironmentVariable{}
	varPattern := regexp.MustCompile(`\$\{([A-Z_][A-Z0-9_]*)(:-([^}]*))?\}`)

	// Track unique variables
	seen := make(map[string]bool)

	// Extract from environment section
	for key, value := range svc.Environment {
		if value == nil {
			continue
		}

		// Check if value contains variable substitution
		matches := varPattern.FindAllStringSubmatch(*value, -1)
		for _, match := range matches {
			varName := match[1]
			defaultValue := ""
			if len(match) > 3 {
				defaultValue = match[3]
			}

			if !seen[varName] {
				seen[varName] = true
				envVars = append(envVars, types.EnvironmentVariable{
					Name:        varName,
					Label:       formatVarLabel(varName),
					Description: fmt.Sprintf("Environment variable for %s", key),
					Required:    defaultValue == "", // Required if no default
					Default:     defaultValue,
					Type:        inferVarType(varName),
				})
			}
		}
	}

	// Extract from container_name
	if svc.ContainerName != "" {
		matches := varPattern.FindAllStringSubmatch(svc.ContainerName, -1)
		for _, match := range matches {
			varName := match[1]
			defaultValue := ""
			if len(match) > 3 {
				defaultValue = match[3]
			}

			if !seen[varName] {
				seen[varName] = true
				envVars = append(envVars, types.EnvironmentVariable{
					Name:        varName,
					Label:       formatVarLabel(varName),
					Description: "Container name",
					Required:    false,
					Default:     defaultValue,
					Type:        "text",
				})
			}
		}
	}

	// Extract from ports
	for _, port := range svc.Ports {
		if port.Published != "" {
			matches := varPattern.FindAllStringSubmatch(port.Published, -1)
			for _, match := range matches {
				varName := match[1]
				defaultValue := ""
				if len(match) > 3 {
					defaultValue = match[3]
				}

				if !seen[varName] {
					seen[varName] = true
					envVars = append(envVars, types.EnvironmentVariable{
						Name:        varName,
						Label:       formatVarLabel(varName),
						Description: fmt.Sprintf("Port mapping (default: %s)", defaultValue),
						Required:    false,
						Default:     defaultValue,
						Type:        "number",
					})
				}
			}
		}
	}

	// Extract from image (for tag customization)
	if svc.Image != "" {
		matches := varPattern.FindAllStringSubmatch(svc.Image, -1)
		for _, match := range matches {
			varName := match[1]
			defaultValue := ""
			if len(match) > 3 {
				defaultValue = match[3]
			}

			if !seen[varName] {
				seen[varName] = true
				envVars = append(envVars, types.EnvironmentVariable{
					Name:        varName,
					Label:       formatVarLabel(varName),
					Description: fmt.Sprintf("Image tag (default: %s)", defaultValue),
					Required:    false,
					Default:     defaultValue,
					Type:        "text",
				})
			}
		}
	}

	return envVars
}

// extractEnvironmentVariablesFromRaw extracts env var definitions from raw YAML content.
// This is needed because compose-go expands variables during loading, losing the patterns.
func extractEnvironmentVariablesFromRaw(rawYAML string) []types.EnvironmentVariable {
	envVars := []types.EnvironmentVariable{}
	varPattern := regexp.MustCompile(`\$\{([A-Z_][A-Z0-9_]*)(:-([^}]*))?\}`)

	// Track unique variables
	seen := make(map[string]bool)

	// Find all variable patterns in the raw YAML
	matches := varPattern.FindAllStringSubmatch(rawYAML, -1)
	for _, match := range matches {
		varName := match[1]
		defaultValue := ""
		hasDefault := false
		if len(match) > 2 && match[2] != "" {
			// match[2] is the ":-default" part - if it exists, there's a default
			hasDefault = true
			if len(match) > 3 {
				defaultValue = match[3]
			}
		}

		// Skip SERVICE_NAME as it's auto-generated
		if varName == "SERVICE_NAME" {
			continue
		}

		if !seen[varName] {
			seen[varName] = true
			envVars = append(envVars, types.EnvironmentVariable{
				Name:        varName,
				Label:       formatVarLabel(varName),
				Description: fmt.Sprintf("Configuration for %s", formatVarLabel(varName)),
				Required:    !hasDefault, // Required only if no default syntax present
				Default:     defaultValue,
				Type:        inferVarType(varName),
			})
		}
	}

	return envVars
}

// formatVarLabel converts an environment variable name to a human-readable label.
func formatVarLabel(varName string) string {
	// Remove common prefixes
	label := strings.TrimPrefix(varName, "GF_")
	label = strings.TrimPrefix(label, "PIHOLE_")
	label = strings.TrimPrefix(label, "UPTIME_")
	label = strings.TrimPrefix(label, "MEMOS_")

	// Replace underscores with spaces
	label = strings.ReplaceAll(label, "_", " ")

	// Title case
	words := strings.Fields(label)
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(string(word[0])) + strings.ToLower(word[1:])
		}
	}

	return strings.Join(words, " ")
}

// inferVarType infers the variable type based on its name.
func inferVarType(varName string) string {
	lowerName := strings.ToLower(varName)

	// Password fields
	if strings.Contains(lowerName, "password") || strings.Contains(lowerName, "secret") || strings.Contains(lowerName, "token") {
		return "password"
	}

	// Port fields
	if strings.Contains(lowerName, "port") {
		return "number"
	}

	// URL fields
	if strings.Contains(lowerName, "url") || strings.Contains(lowerName, "host") {
		return "text"
	}

	// Default to text
	return "text"
}

// loadYAMLFile loads service templates from a YAML catalog file (DEPRECATED).
func (m *Manager) loadYAMLFile() error {
	// Read catalog file
	data, err := os.ReadFile(m.catalogPath)
	if err != nil {
		return fmt.Errorf("failed to read catalog file: %w", err)
	}

	// Parse YAML
	var catalog catalogFile
	if err := yaml.Unmarshal(data, &catalog); err != nil {
		return fmt.Errorf("failed to parse catalog YAML: %w", err)
	}

	// Clear existing templates
	m.templates = make(map[string]*types.ServiceTemplate)

	// Load templates into map
	for _, template := range catalog.Services {
		if template.ID == "" {
			return fmt.Errorf("service template missing required field: id")
		}
		m.templates[template.ID] = template
	}

	return nil
}

// GetService retrieves a service template by ID.
func (m *Manager) GetService(id string) (*types.ServiceTemplate, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	template, exists := m.templates[id]
	if !exists {
		return nil, fmt.Errorf("service not found: %s", id)
	}

	return template, nil
}

// ListServices returns all available service templates.
func (m *Manager) ListServices() []*types.ServiceTemplate {
	m.mu.RLock()
	defer m.mu.RUnlock()

	services := make([]*types.ServiceTemplate, 0, len(m.templates))
	for _, template := range m.templates {
		services = append(services, template)
	}

	return services
}

// FilterByCategory returns service templates filtered by category.
func (m *Manager) FilterByCategory(category string) []*types.ServiceTemplate {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var filtered []*types.ServiceTemplate
	for _, template := range m.templates {
		if template.Category == category {
			filtered = append(filtered, template)
		}
	}

	return filtered
}

// ValidateDeploymentRequest validates a deployment request.
func (m *Manager) ValidateDeploymentRequest(req *types.DeploymentRequest) error {
	if req == nil {
		return fmt.Errorf("deployment request cannot be nil")
	}

	// Validate service ID
	if req.ServiceID == "" {
		return fmt.Errorf("service ID is required")
	}

	// Validate service name
	if req.ServiceName == "" {
		return fmt.Errorf("service name is required")
	}

	// Get service template
	template, err := m.GetService(req.ServiceID)
	if err != nil {
		return fmt.Errorf("invalid service ID: %w", err)
	}

	// Validate required environment variables
	for _, envVar := range template.EnvVars {
		if envVar.Required {
			value, exists := req.EnvVars[envVar.Name]
			if !exists || value == "" {
				return fmt.Errorf("required environment variable missing: %s", envVar.Name)
			}
		}
	}

	return nil
}
