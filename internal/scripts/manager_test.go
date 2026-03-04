package scripts

import (
	"os"
	"path/filepath"
	"testing"
)

// Test helpers

func setupTestScriptsDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Create some test scripts
	scriptsDir := filepath.Join(dir, "scripts")
	if err := os.MkdirAll(filepath.Join(scriptsDir, "backup"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(scriptsDir, "deploy"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create shell script
	shellScript := `#!/bin/bash
echo "Hello from backup script"
`
	if err := os.WriteFile(filepath.Join(scriptsDir, "backup", "db-backup.sh"), []byte(shellScript), 0755); err != nil {
		t.Fatal(err)
	}

	// Create another shell script
	deployScript := `#!/bin/bash
echo "Restarting $CONTAINER"
docker restart "$CONTAINER"
`
	if err := os.WriteFile(filepath.Join(scriptsDir, "deploy", "restart.sh"), []byte(deployScript), 0755); err != nil {
		t.Fatal(err)
	}

	// Create a Python script
	pythonScript := `#!/usr/bin/env python3
print("Hello from Python")
`
	if err := os.WriteFile(filepath.Join(scriptsDir, "backup", "report.py"), []byte(pythonScript), 0755); err != nil {
		t.Fatal(err)
	}

	return scriptsDir
}

// Manager Tests

func TestNewManager(t *testing.T) {
	scriptsDir := setupTestScriptsDir(t)

	manager := NewManager(scriptsDir)
	if manager == nil {
		t.Fatal("Expected manager to be created")
	}

	if manager.scriptsDir != scriptsDir {
		t.Errorf("Expected scriptsDir %s, got %s", scriptsDir, manager.scriptsDir)
	}
}

func TestManager_ScanDirectory(t *testing.T) {
	scriptsDir := setupTestScriptsDir(t)
	manager := NewManager(scriptsDir)

	available, err := manager.ScanDirectory()
	if err != nil {
		t.Fatalf("Failed to scan directory: %v", err)
	}

	if len(available) != 3 {
		t.Errorf("Expected 3 available scripts, got %d", len(available))
	}

	// Verify script types were detected
	scriptTypes := make(map[ScriptType]int)
	for _, s := range available {
		scriptTypes[s.ScriptType]++
	}

	if scriptTypes[ScriptTypeShell] != 2 {
		t.Errorf("Expected 2 shell scripts, got %d", scriptTypes[ScriptTypeShell])
	}
	if scriptTypes[ScriptTypePython] != 1 {
		t.Errorf("Expected 1 python script, got %d", scriptTypes[ScriptTypePython])
	}
}

func TestManager_DetectScriptType(t *testing.T) {
	tests := []struct {
		filename string
		expected ScriptType
	}{
		{"script.sh", ScriptTypeShell},
		{"script.bash", ScriptTypeShell},
		{"script.py", ScriptTypePython},
		{"mybinary", ScriptTypeGoBinary}, // No extension = binary
		{"script.exe", ScriptTypeGoBinary},
	}

	manager := NewManager("")

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			result := manager.DetectScriptType(tt.filename)
			if result != tt.expected {
				t.Errorf("Expected %s for %s, got %s", tt.expected, tt.filename, result)
			}
		})
	}
}

func TestManager_ValidateParameters(t *testing.T) {
	manager := NewManager("")

	script := &Script{
		ID:   "test",
		Name: "Test",
		Parameters: []ScriptParameter{
			{Name: "REQUIRED_PARAM", Label: "Required", Type: "text", Required: true},
			{Name: "OPTIONAL_PARAM", Label: "Optional", Type: "text", Required: false, Default: "default"},
			{Name: "VALIDATED_PARAM", Label: "Validated", Type: "text", Validation: "^[a-z]+$"},
		},
	}

	tests := []struct {
		name        string
		params      map[string]string
		expectError bool
	}{
		{
			name:        "all valid",
			params:      map[string]string{"REQUIRED_PARAM": "value", "VALIDATED_PARAM": "abc"},
			expectError: false,
		},
		{
			name:        "missing required",
			params:      map[string]string{},
			expectError: true,
		},
		{
			name:        "validation failed",
			params:      map[string]string{"REQUIRED_PARAM": "value", "VALIDATED_PARAM": "ABC123"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.ValidateParameters(script, tt.params)
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func TestManager_ApplyDefaults(t *testing.T) {
	manager := NewManager("")

	script := &Script{
		ID:   "test",
		Name: "Test",
		Parameters: []ScriptParameter{
			{Name: "PARAM1", Default: "default1"},
			{Name: "PARAM2", Default: "default2"},
			{Name: "PARAM3"}, // No default
		},
	}

	params := map[string]string{
		"PARAM1": "custom", // Override default
		// PARAM2 and PARAM3 not provided
	}

	result := manager.ApplyDefaults(script, params)

	if result["PARAM1"] != "custom" {
		t.Errorf("Expected PARAM1 to be 'custom', got '%s'", result["PARAM1"])
	}
	if result["PARAM2"] != "default2" {
		t.Errorf("Expected PARAM2 to be 'default2', got '%s'", result["PARAM2"])
	}
	if _, exists := result["PARAM3"]; exists {
		t.Error("Expected PARAM3 to not exist (no default)")
	}
}

func TestManager_BuildEnvironment(t *testing.T) {
	manager := NewManager("")

	script := &Script{
		ID:   "test",
		Name: "Test",
		Environment: map[string]string{
			"STATIC_VAR": "static_value",
		},
	}

	params := map[string]string{
		"DYNAMIC_VAR": "dynamic_value",
	}

	env := manager.BuildEnvironment(script, params, false)

	// Check static var
	found := false
	for _, e := range env {
		if e == "STATIC_VAR=static_value" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected STATIC_VAR in environment")
	}

	// Check dynamic param
	found = false
	for _, e := range env {
		if e == "DYNAMIC_VAR=dynamic_value" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected DYNAMIC_VAR in environment")
	}

	// Check DRY_RUN is not set when false
	for _, e := range env {
		if e == "DRY_RUN=true" {
			t.Error("DRY_RUN should not be set when dryRun is false")
		}
	}
}

func TestManager_BuildEnvironment_DryRun(t *testing.T) {
	manager := NewManager("")

	script := &Script{ID: "test", Name: "Test"}
	env := manager.BuildEnvironment(script, nil, true)

	// Check DRY_RUN is set
	found := false
	for _, e := range env {
		if e == "DRY_RUN=true" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected DRY_RUN=true in environment")
	}
}

func TestManager_GetScriptPath(t *testing.T) {
	scriptsDir := setupTestScriptsDir(t)
	manager := NewManager(scriptsDir)

	script := &Script{
		ScriptPath: "backup/db-backup.sh",
	}

	path := manager.GetScriptPath(script)
	expected := filepath.Join(scriptsDir, "backup", "db-backup.sh")

	if path != expected {
		t.Errorf("Expected path %s, got %s", expected, path)
	}

	// Verify file exists
	if _, err := os.Stat(path); err != nil {
		t.Errorf("Script file should exist at %s: %v", path, err)
	}
}

func TestManager_ValidateScriptExists(t *testing.T) {
	scriptsDir := setupTestScriptsDir(t)
	manager := NewManager(scriptsDir)

	// Test existing script
	script := &Script{ScriptPath: "backup/db-backup.sh"}
	if err := manager.ValidateScriptExists(script); err != nil {
		t.Errorf("Expected script to exist: %v", err)
	}

	// Test non-existing script
	missingScript := &Script{ScriptPath: "nonexistent/script.sh"}
	if err := manager.ValidateScriptExists(missingScript); err == nil {
		t.Error("Expected error for non-existing script")
	}
}

func TestManager_ScanDirectory_Empty(t *testing.T) {
	emptyDir := t.TempDir()
	manager := NewManager(emptyDir)

	available, err := manager.ScanDirectory()
	if err != nil {
		t.Fatalf("Failed to scan empty directory: %v", err)
	}

	if len(available) != 0 {
		t.Errorf("Expected 0 scripts in empty directory, got %d", len(available))
	}
}

func TestManager_ScanDirectory_NonExistent(t *testing.T) {
	manager := NewManager("/nonexistent/path")

	_, err := manager.ScanDirectory()
	if err == nil {
		t.Error("Expected error for non-existent directory")
	}
}
