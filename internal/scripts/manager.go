package scripts

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// Manager handles script catalog operations.
type Manager struct {
	scriptsDir string
	mu         sync.RWMutex
}

// NewManager creates a new script manager.
func NewManager(scriptsDir string) *Manager {
	return &Manager{
		scriptsDir: scriptsDir,
	}
}

// ScanDirectory scans the scripts directory for available script files.
// Returns scripts that can be registered.
func (m *Manager) ScanDirectory() ([]AvailableScript, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.scriptsDir == "" {
		return nil, fmt.Errorf("scripts directory not configured")
	}

	// Check if directory exists
	info, err := os.Stat(m.scriptsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to access scripts directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("scripts path is not a directory: %s", m.scriptsDir)
	}

	var scripts []AvailableScript

	// Walk the directory tree
	err = filepath.Walk(m.scriptsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Skip hidden files
		if strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		// Get relative path from scripts directory
		relPath, err := filepath.Rel(m.scriptsDir, path)
		if err != nil {
			return err
		}

		// Detect script type
		scriptType := m.DetectScriptType(info.Name())

		scripts = append(scripts, AvailableScript{
			Path:       relPath,
			Name:       info.Name(),
			ScriptType: scriptType,
			Size:       info.Size(),
			ModTime:    info.ModTime(),
		})

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to scan scripts directory: %w", err)
	}

	return scripts, nil
}

// DetectScriptType determines the script type from the filename.
func (m *Manager) DetectScriptType(filename string) ScriptType {
	ext := strings.ToLower(filepath.Ext(filename))

	switch ext {
	case ".sh", ".bash":
		return ScriptTypeShell
	case ".py":
		return ScriptTypePython
	case ".go":
		// .go files are source, not binaries - but we might support them later
		return ScriptTypeGoBinary
	default:
		// No extension or unknown extension - assume it's a binary
		return ScriptTypeGoBinary
	}
}

// ValidateParameters validates the provided parameters against the script definition.
func (m *Manager) ValidateParameters(script *Script, params map[string]string) error {
	for _, param := range script.Parameters {
		value, provided := params[param.Name]

		// Check required parameters
		if param.Required && !provided {
			if param.Default == "" {
				return fmt.Errorf("missing required parameter: %s", param.Name)
			}
		}

		// Validate pattern if provided
		if provided && param.Validation != "" {
			matched, err := regexp.MatchString(param.Validation, value)
			if err != nil {
				return fmt.Errorf("invalid validation pattern for %s: %w", param.Name, err)
			}
			if !matched {
				return fmt.Errorf("parameter %s failed validation: value '%s' does not match pattern '%s'",
					param.Name, value, param.Validation)
			}
		}
	}

	return nil
}

// ApplyDefaults applies default values to parameters that weren't provided.
func (m *Manager) ApplyDefaults(script *Script, params map[string]string) map[string]string {
	result := make(map[string]string)

	// Copy provided params
	for k, v := range params {
		result[k] = v
	}

	// Apply defaults for missing params
	for _, param := range script.Parameters {
		if _, exists := result[param.Name]; !exists && param.Default != "" {
			result[param.Name] = param.Default
		}
	}

	return result
}

// BuildEnvironment builds the environment variables for script execution.
func (m *Manager) BuildEnvironment(script *Script, params map[string]string, dryRun bool) []string {
	// Start with current environment (optional - can be disabled for cleaner env)
	// For now, we'll build a clean environment with only what's needed

	var env []string

	// Add PATH from current environment for basic command access
	if path := os.Getenv("PATH"); path != "" {
		env = append(env, "PATH="+path)
	}

	// Add HOME for scripts that need it
	if home := os.Getenv("HOME"); home != "" {
		env = append(env, "HOME="+home)
	}

	// Add script's static environment variables
	for key, value := range script.Environment {
		env = append(env, key+"="+value)
	}

	// Add user-provided parameters as environment variables
	for key, value := range params {
		env = append(env, key+"="+value)
	}

	// Add dry run flag if enabled
	if dryRun {
		env = append(env, "DRY_RUN=true")
	}

	return env
}

// GetScriptPath returns the full filesystem path for a script.
func (m *Manager) GetScriptPath(script *Script) string {
	return filepath.Join(m.scriptsDir, script.ScriptPath)
}

// ValidateScriptExists checks if the script file exists on the filesystem.
func (m *Manager) ValidateScriptExists(script *Script) error {
	path := m.GetScriptPath(script)

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("script file not found: %s", script.ScriptPath)
		}
		return fmt.Errorf("failed to access script file: %w", err)
	}

	if info.IsDir() {
		return fmt.Errorf("script path is a directory, not a file: %s", script.ScriptPath)
	}

	return nil
}

// IsExecutable checks if the script file is executable.
func (m *Manager) IsExecutable(script *Script) (bool, error) {
	path := m.GetScriptPath(script)

	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}

	// Check if any execute bit is set
	mode := info.Mode()
	return mode&0111 != 0, nil
}

// GetScriptsDir returns the configured scripts directory.
func (m *Manager) GetScriptsDir() string {
	return m.scriptsDir
}
