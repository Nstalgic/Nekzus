package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
)

// TestExportHandler_HandleExportContainer tests container export to compose
func TestExportHandler_HandleExportContainer(t *testing.T) {
	tests := []struct {
		name            string
		method          string
		containerID     string
		requestBody     string
		mockInspectFunc func(ctx context.Context, containerID string) (types.ContainerJSON, error)
		expectedStatus  int
		checkResponse   func(t *testing.T, body []byte)
	}{
		{
			name:        "successful export",
			method:      http.MethodPost,
			containerID: "test-container-123",
			requestBody: `{"sanitize_secrets": true}`,
			mockInspectFunc: func(ctx context.Context, containerID string) (types.ContainerJSON, error) {
				return types.ContainerJSON{
					ContainerJSONBase: &container.ContainerJSONBase{
						Name: "/nginx-test",
						HostConfig: &container.HostConfig{
							PortBindings: nat.PortMap{
								"80/tcp": []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: "8080"}},
							},
							RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
						},
					},
					Config: &container.Config{
						Image: "nginx:latest",
						Env:   []string{"NGINX_HOST=localhost"},
					},
					Mounts: []container.MountPoint{
						{
							Type:        mount.TypeVolume,
							Name:        "nginx-data",
							Destination: "/usr/share/nginx/html",
							RW:          true,
						},
					},
					NetworkSettings: &container.NetworkSettings{
						Networks: map[string]*network.EndpointSettings{
							"webnet": {Aliases: []string{"nginx"}},
						},
					},
				}, nil
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body []byte) {
				var resp ExportResponse
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("failed to unmarshal response: %v", err)
				}
				if resp.Format != "compose" {
					t.Errorf("expected format 'compose', got %s", resp.Format)
				}
				if !strings.Contains(resp.Content, "nginx:latest") {
					t.Error("expected content to contain 'nginx:latest'")
				}
				if !strings.Contains(resp.Content, "8080:80") {
					t.Error("expected content to contain port mapping '8080:80'")
				}
				if resp.Filename != "nginx-test-compose.yml" {
					t.Errorf("expected filename 'nginx-test-compose.yml', got %s", resp.Filename)
				}
			},
		},
		{
			name:        "export with sensitive data sanitization",
			method:      http.MethodPost,
			containerID: "postgres-123",
			requestBody: `{"sanitize_secrets": true}`,
			mockInspectFunc: func(ctx context.Context, containerID string) (types.ContainerJSON, error) {
				return types.ContainerJSON{
					ContainerJSONBase: &container.ContainerJSONBase{
						Name: "/postgres",
						HostConfig: &container.HostConfig{
							RestartPolicy: container.RestartPolicy{Name: "always"},
						},
					},
					Config: &container.Config{
						Image: "postgres:15",
						Env:   []string{"POSTGRES_USER=admin", "POSTGRES_PASSWORD=supersecret"},
					},
					Mounts:          []container.MountPoint{},
					NetworkSettings: &container.NetworkSettings{Networks: map[string]*network.EndpointSettings{}},
				}, nil
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body []byte) {
				var resp ExportResponse
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("failed to unmarshal response: %v", err)
				}
				// Password should be sanitized from compose content
				if strings.Contains(resp.Content, "supersecret") {
					t.Error("expected sensitive password to be sanitized from compose content")
				}
				// Should contain variable reference in compose
				if !strings.Contains(resp.Content, "${POSTGRES_PASSWORD}") {
					t.Error("expected password variable reference in compose output")
				}
				// Should have env file content
				if resp.EnvContent == "" {
					t.Error("expected env file content to be generated")
				}
				// Env file should have CHANGE_ME placeholder
				if !strings.Contains(resp.EnvContent, "POSTGRES_PASSWORD=CHANGE_ME") {
					t.Error("expected CHANGE_ME placeholder in env file")
				}
				// Should have env filename
				if resp.EnvFilename != ".env.example" {
					t.Errorf("expected env filename '.env.example', got %s", resp.EnvFilename)
				}
			},
		},
		{
			name:        "export without sanitization",
			method:      http.MethodPost,
			containerID: "postgres-123",
			requestBody: `{"sanitize_secrets": false}`,
			mockInspectFunc: func(ctx context.Context, containerID string) (types.ContainerJSON, error) {
				return types.ContainerJSON{
					ContainerJSONBase: &container.ContainerJSONBase{
						Name: "/postgres",
						HostConfig: &container.HostConfig{
							RestartPolicy: container.RestartPolicy{Name: "always"},
						},
					},
					Config: &container.Config{
						Image: "postgres:15",
						Env:   []string{"POSTGRES_PASSWORD=supersecret"},
					},
					Mounts:          []container.MountPoint{},
					NetworkSettings: &container.NetworkSettings{Networks: map[string]*network.EndpointSettings{}},
				}, nil
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body []byte) {
				var resp ExportResponse
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("failed to unmarshal response: %v", err)
				}
				// Password should NOT be sanitized
				if !strings.Contains(resp.Content, "supersecret") {
					t.Error("expected sensitive password to be present when sanitization is disabled")
				}
			},
		},
		{
			name:        "container not found",
			method:      http.MethodPost,
			containerID: "nonexistent",
			requestBody: `{}`,
			mockInspectFunc: func(ctx context.Context, containerID string) (types.ContainerJSON, error) {
				return types.ContainerJSON{}, fmt.Errorf("No such container: %s", containerID)
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "method not allowed",
			method:         http.MethodGet,
			containerID:    "test-container",
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "empty container ID",
			method:         http.MethodPost,
			containerID:    "",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:        "default options when body is empty",
			method:      http.MethodPost,
			containerID: "test-container-123",
			requestBody: `{}`,
			mockInspectFunc: func(ctx context.Context, containerID string) (types.ContainerJSON, error) {
				return types.ContainerJSON{
					ContainerJSONBase: &container.ContainerJSONBase{
						Name: "/simple-app",
						HostConfig: &container.HostConfig{
							RestartPolicy: container.RestartPolicy{Name: "no"},
						},
					},
					Config: &container.Config{
						Image: "alpine:latest",
					},
					Mounts:          []container.MountPoint{},
					NetworkSettings: &container.NetworkSettings{Networks: map[string]*network.EndpointSettings{}},
				}, nil
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body []byte) {
				var resp ExportResponse
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("failed to unmarshal response: %v", err)
				}
				if resp.Format != "compose" {
					t.Errorf("expected format 'compose', got %s", resp.Format)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock Docker client
			mockClient := &MockDockerClient{
				ContainerInspectFunc: tt.mockInspectFunc,
			}

			// Create handler
			handler := NewExportHandler(mockClient)

			// Create request
			url := fmt.Sprintf("/api/v1/containers/%s/export", tt.containerID)
			var req *http.Request
			if tt.requestBody != "" {
				req = httptest.NewRequest(tt.method, url, strings.NewReader(tt.requestBody))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(tt.method, url, nil)
			}
			w := httptest.NewRecorder()

			// Execute handler
			handler.HandleExportContainer(w, req)

			// Check status code
			if w.Code != tt.expectedStatus {
				t.Errorf("HandleExportContainer() status = %d, want %d, body: %s", w.Code, tt.expectedStatus, w.Body.String())
			}

			// Check response body if provided
			if tt.checkResponse != nil && w.Code == http.StatusOK {
				tt.checkResponse(t, w.Body.Bytes())
			}
		})
	}
}

// TestExportHandler_HandleExportContainer_ComplexContainer tests export of a complex container
func TestExportHandler_HandleExportContainer_ComplexContainer(t *testing.T) {
	mockClient := &MockDockerClient{
		ContainerInspectFunc: func(ctx context.Context, containerID string) (types.ContainerJSON, error) {
			return types.ContainerJSON{
				ContainerJSONBase: &container.ContainerJSONBase{
					Name: "/media-server",
					HostConfig: &container.HostConfig{
						PortBindings: nat.PortMap{
							"8080/tcp": []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: "8080"}},
							"8443/tcp": []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: "8443"}},
						},
						RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
						Privileged:    true,
						CapAdd:        []string{"SYS_ADMIN"},
					},
				},
				Config: &container.Config{
					Image:      "plexinc/pms-docker:latest",
					Env:        []string{"PLEX_UID=1000", "PLEX_GID=1000", "TZ=America/New_York"},
					User:       "1000:1000",
					WorkingDir: "/app",
					Healthcheck: &container.HealthConfig{
						Test:     []string{"CMD", "curl", "-f", "http://localhost:32400/web"},
						Interval: 30000000000, // 30s in nanoseconds
						Timeout:  10000000000, // 10s in nanoseconds
						Retries:  3,
					},
					Labels: map[string]string{
						"maintainer":      "test",
						"nekzus.app.id":   "plex",
						"nekzus.app.name": "Plex",
					},
				},
				Mounts: []container.MountPoint{
					{
						Type:        mount.TypeVolume,
						Name:        "plex-config",
						Destination: "/config",
						RW:          true,
					},
					{
						Type:        mount.TypeBind,
						Source:      "/mnt/media",
						Destination: "/data",
						RW:          true,
					},
				},
				NetworkSettings: &container.NetworkSettings{
					Networks: map[string]*network.EndpointSettings{
						"media-network": {Aliases: []string{"plex", "media-server"}},
					},
				},
			}, nil
		},
	}

	handler := NewExportHandler(mockClient)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/containers/complex-123/export",
		strings.NewReader(`{"sanitize_secrets": true, "include_volumes": true, "include_networks": true}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleExportContainer(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("HandleExportContainer() status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp ExportResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	// Verify complex configuration is captured
	content := resp.Content

	// Check image
	if !strings.Contains(content, "plexinc/pms-docker:latest") {
		t.Error("expected content to contain image")
	}

	// Check ports
	if !strings.Contains(content, "8080:8080") {
		t.Error("expected content to contain port 8080")
	}
	if !strings.Contains(content, "8443:8443") {
		t.Error("expected content to contain port 8443")
	}

	// Check volumes (both named and bind)
	if !strings.Contains(content, "plex-config") {
		t.Error("expected content to contain named volume")
	}
	if !strings.Contains(content, "/mnt/media") {
		t.Error("expected content to contain bind mount")
	}

	// Check network
	if !strings.Contains(content, "media-network") {
		t.Error("expected content to contain network")
	}

	// Check restart policy
	if !strings.Contains(content, "unless-stopped") {
		t.Error("expected content to contain restart policy")
	}

	// Check privileged warning
	foundPrivilegedWarning := false
	for _, w := range resp.Warnings {
		if strings.Contains(w, "privileged") {
			foundPrivilegedWarning = true
			break
		}
	}
	if !foundPrivilegedWarning {
		t.Error("expected warning about privileged mode")
	}

	// Check health check
	if !strings.Contains(content, "healthcheck") {
		t.Error("expected content to contain healthcheck")
	}

	// Check user
	if !strings.Contains(content, "1000:1000") {
		t.Error("expected content to contain user")
	}

	// Check capabilities
	if !strings.Contains(content, "SYS_ADMIN") {
		t.Error("expected content to contain capability")
	}

	// Check labels (should exclude compose internal labels)
	if !strings.Contains(content, "maintainer") {
		t.Error("expected content to contain custom labels")
	}
}

// --- Batch Export Handler Tests ---

// TestExportHandler_HandleBatchExport tests batch export of multiple containers
func TestExportHandler_HandleBatchExport(t *testing.T) {
	tests := []struct {
		name            string
		method          string
		requestBody     string
		mockInspectFunc func(ctx context.Context, containerID string) (types.ContainerJSON, error)
		expectedStatus  int
		checkResponse   func(t *testing.T, body []byte)
	}{
		{
			name:   "successful batch export",
			method: http.MethodPost,
			requestBody: `{
				"container_ids": ["plex-123", "sonarr-456"],
				"stack_name": "media-stack",
				"sanitize_secrets": true,
				"include_volumes": true,
				"include_networks": true
			}`,
			mockInspectFunc: func(ctx context.Context, containerID string) (types.ContainerJSON, error) {
				switch containerID {
				case "plex-123":
					return types.ContainerJSON{
						ContainerJSONBase: &container.ContainerJSONBase{
							Name:       "/plex",
							HostConfig: &container.HostConfig{RestartPolicy: container.RestartPolicy{Name: "unless-stopped"}},
						},
						Config: &container.Config{
							Image: "plexinc/pms-docker:latest",
							Env:   []string{"PLEX_UID=1000"},
						},
						Mounts: []container.MountPoint{
							{Type: mount.TypeVolume, Name: "plex-config", Destination: "/config", RW: true},
						},
						NetworkSettings: &container.NetworkSettings{
							Networks: map[string]*network.EndpointSettings{
								"media-network": {Aliases: []string{"plex"}},
							},
						},
					}, nil
				case "sonarr-456":
					return types.ContainerJSON{
						ContainerJSONBase: &container.ContainerJSONBase{
							Name:       "/sonarr",
							HostConfig: &container.HostConfig{RestartPolicy: container.RestartPolicy{Name: "unless-stopped"}},
						},
						Config: &container.Config{
							Image: "linuxserver/sonarr:latest",
							Env:   []string{"PUID=1000"},
						},
						Mounts: []container.MountPoint{
							{Type: mount.TypeVolume, Name: "sonarr-config", Destination: "/config", RW: true},
						},
						NetworkSettings: &container.NetworkSettings{
							Networks: map[string]*network.EndpointSettings{
								"media-network": {Aliases: []string{"sonarr"}},
							},
						},
					}, nil
				}
				return types.ContainerJSON{}, fmt.Errorf("container not found: %s", containerID)
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body []byte) {
				var resp ExportResponse
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("failed to unmarshal response: %v", err)
				}
				if resp.Format != "compose" {
					t.Errorf("expected format 'compose', got %s", resp.Format)
				}
				// Check both services are in content
				if !strings.Contains(resp.Content, "plexinc/pms-docker:latest") {
					t.Error("expected content to contain plex image")
				}
				if !strings.Contains(resp.Content, "linuxserver/sonarr:latest") {
					t.Error("expected content to contain sonarr image")
				}
				// Check network is deduplicated (only once in top-level)
				if !strings.Contains(resp.Content, "media-network") {
					t.Error("expected content to contain media-network")
				}
				if resp.Filename != "media-stack-compose.yml" {
					t.Errorf("expected filename 'media-stack-compose.yml', got %s", resp.Filename)
				}
			},
		},
		{
			name:           "empty container IDs",
			method:         http.MethodPost,
			requestBody:    `{"container_ids": [], "stack_name": "test"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "missing container IDs",
			method:         http.MethodPost,
			requestBody:    `{"stack_name": "test"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "method not allowed",
			method:         http.MethodGet,
			requestBody:    `{}`,
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "invalid JSON",
			method:         http.MethodPost,
			requestBody:    `{invalid json`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:   "partial failure - one container not found",
			method: http.MethodPost,
			requestBody: `{
				"container_ids": ["existing-123", "nonexistent-456"],
				"stack_name": "test-stack"
			}`,
			mockInspectFunc: func(ctx context.Context, containerID string) (types.ContainerJSON, error) {
				if containerID == "existing-123" {
					return types.ContainerJSON{
						ContainerJSONBase: &container.ContainerJSONBase{
							Name:       "/existing",
							HostConfig: &container.HostConfig{RestartPolicy: container.RestartPolicy{Name: "no"}},
						},
						Config:          &container.Config{Image: "nginx:latest"},
						Mounts:          []container.MountPoint{},
						NetworkSettings: &container.NetworkSettings{Networks: map[string]*network.EndpointSettings{}},
					}, nil
				}
				return types.ContainerJSON{}, fmt.Errorf("No such container: %s", containerID)
			},
			expectedStatus: http.StatusPartialContent, // 206 - some containers exported
			checkResponse: func(t *testing.T, body []byte) {
				var resp ExportResponse
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("failed to unmarshal response: %v", err)
				}
				// Should still have the successful container
				if !strings.Contains(resp.Content, "nginx:latest") {
					t.Error("expected content to contain nginx image")
				}
				// Should have warning about failed container
				foundWarning := false
				for _, w := range resp.Warnings {
					if strings.Contains(w, "nonexistent-456") {
						foundWarning = true
						break
					}
				}
				if !foundWarning {
					t.Error("expected warning about failed container")
				}
			},
		},
		{
			name:   "all containers not found",
			method: http.MethodPost,
			requestBody: `{
				"container_ids": ["nonexistent-1", "nonexistent-2"],
				"stack_name": "test-stack"
			}`,
			mockInspectFunc: func(ctx context.Context, containerID string) (types.ContainerJSON, error) {
				return types.ContainerJSON{}, fmt.Errorf("No such container: %s", containerID)
			},
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockDockerClient{
				ContainerInspectFunc: tt.mockInspectFunc,
			}

			handler := NewExportHandler(mockClient)

			req := httptest.NewRequest(tt.method, "/api/v1/containers/batch/export", strings.NewReader(tt.requestBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.HandleBatchExport(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("HandleBatchExport() status = %d, want %d, body: %s", w.Code, tt.expectedStatus, w.Body.String())
			}

			if tt.checkResponse != nil && (w.Code == http.StatusOK || w.Code == http.StatusPartialContent) {
				tt.checkResponse(t, w.Body.Bytes())
			}
		})
	}
}

// TestExportHandler_HandlePreviewExport tests container export preview (no download)
func TestExportHandler_HandlePreviewExport(t *testing.T) {
	mockClient := &MockDockerClient{
		ContainerInspectFunc: func(ctx context.Context, containerID string) (types.ContainerJSON, error) {
			return types.ContainerJSON{
				ContainerJSONBase: &container.ContainerJSONBase{
					Name:       "/nginx-test",
					HostConfig: &container.HostConfig{RestartPolicy: container.RestartPolicy{Name: "unless-stopped"}},
				},
				Config: &container.Config{
					Image: "nginx:latest",
					Env:   []string{"NGINX_HOST=localhost", "ADMIN_PASSWORD=secret"},
				},
				Mounts:          []container.MountPoint{},
				NetworkSettings: &container.NetworkSettings{Networks: map[string]*network.EndpointSettings{}},
			}, nil
		},
	}

	handler := NewExportHandler(mockClient)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/containers/test-123/export/preview",
		strings.NewReader(`{"sanitize_secrets": true}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandlePreviewExport(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("HandlePreviewExport() status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp ExportResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	// Should contain YAML content
	if !strings.Contains(resp.Content, "nginx:latest") {
		t.Error("expected content to contain nginx image")
	}

	// Should have sanitized sensitive vars
	if strings.Contains(resp.Content, "secret") {
		t.Error("expected sensitive password to be sanitized")
	}

	// Should have env file content with CHANGE_ME placeholder
	if !strings.Contains(resp.EnvContent, "ADMIN_PASSWORD=CHANGE_ME") {
		t.Error("expected env content to contain CHANGE_ME placeholder")
	}
}

// TestExportHandler_HandleBatchPreviewExport tests batch export preview
func TestExportHandler_HandleBatchPreviewExport(t *testing.T) {
	mockClient := &MockDockerClient{
		ContainerInspectFunc: func(ctx context.Context, containerID string) (types.ContainerJSON, error) {
			switch containerID {
			case "web-123":
				return types.ContainerJSON{
					ContainerJSONBase: &container.ContainerJSONBase{
						Name:       "/web",
						HostConfig: &container.HostConfig{RestartPolicy: container.RestartPolicy{Name: "unless-stopped"}},
					},
					Config: &container.Config{
						Image: "nginx:latest",
					},
					Mounts:          []container.MountPoint{},
					NetworkSettings: &container.NetworkSettings{Networks: map[string]*network.EndpointSettings{}},
				}, nil
			case "db-456":
				return types.ContainerJSON{
					ContainerJSONBase: &container.ContainerJSONBase{
						Name:       "/db",
						HostConfig: &container.HostConfig{RestartPolicy: container.RestartPolicy{Name: "unless-stopped"}},
					},
					Config: &container.Config{
						Image: "postgres:15",
					},
					Mounts:          []container.MountPoint{},
					NetworkSettings: &container.NetworkSettings{Networks: map[string]*network.EndpointSettings{}},
				}, nil
			}
			return types.ContainerJSON{}, fmt.Errorf("container not found: %s", containerID)
		},
	}

	handler := NewExportHandler(mockClient)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/containers/batch/export/preview",
		strings.NewReader(`{
			"container_ids": ["web-123", "db-456"],
			"stack_name": "mystack"
		}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleBatchPreviewExport(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("HandleBatchPreviewExport() status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp ExportResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	// Should contain both services in YAML
	if !strings.Contains(resp.Content, "nginx:latest") {
		t.Error("expected content to contain nginx image")
	}
	if !strings.Contains(resp.Content, "postgres:15") {
		t.Error("expected content to contain postgres image")
	}

	// Should have correct filename
	if resp.Filename != "mystack-compose.yml" {
		t.Errorf("expected filename 'mystack-compose.yml', got %s", resp.Filename)
	}
}

// TestExportHandler_HandleBatchExportZip tests batch export with ZIP format
func TestExportHandler_HandleBatchExportZip(t *testing.T) {
	mockClient := &MockDockerClient{
		ContainerInspectFunc: func(ctx context.Context, containerID string) (types.ContainerJSON, error) {
			switch containerID {
			case "web-123":
				return types.ContainerJSON{
					ContainerJSONBase: &container.ContainerJSONBase{
						Name:       "/web",
						HostConfig: &container.HostConfig{RestartPolicy: container.RestartPolicy{Name: "unless-stopped"}},
					},
					Config: &container.Config{
						Image: "nginx:latest",
						Env:   []string{"API_KEY=secret123"},
					},
					Mounts:          []container.MountPoint{},
					NetworkSettings: &container.NetworkSettings{Networks: map[string]*network.EndpointSettings{}},
				}, nil
			case "db-456":
				return types.ContainerJSON{
					ContainerJSONBase: &container.ContainerJSONBase{
						Name:       "/db",
						HostConfig: &container.HostConfig{RestartPolicy: container.RestartPolicy{Name: "unless-stopped"}},
					},
					Config: &container.Config{
						Image: "postgres:15",
						Env:   []string{"POSTGRES_PASSWORD=dbsecret"},
					},
					Mounts:          []container.MountPoint{},
					NetworkSettings: &container.NetworkSettings{Networks: map[string]*network.EndpointSettings{}},
				}, nil
			}
			return types.ContainerJSON{}, fmt.Errorf("container not found: %s", containerID)
		},
	}

	handler := NewExportHandler(mockClient)

	// Request with format=zip query parameter
	req := httptest.NewRequest(http.MethodPost, "/api/v1/containers/batch/export?format=zip",
		strings.NewReader(`{
			"container_ids": ["web-123", "db-456"],
			"stack_name": "myapp",
			"sanitize_secrets": true
		}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleBatchExport(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("HandleBatchExport() status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Check content type is application/zip
	contentType := w.Header().Get("Content-Type")
	if contentType != "application/zip" {
		t.Errorf("expected Content-Type 'application/zip', got %s", contentType)
	}

	// Check content disposition header
	contentDisposition := w.Header().Get("Content-Disposition")
	if !strings.Contains(contentDisposition, "myapp.zip") {
		t.Errorf("expected Content-Disposition to contain 'myapp.zip', got %s", contentDisposition)
	}

	// Verify it's a valid ZIP by checking magic bytes
	body := w.Body.Bytes()
	if len(body) < 4 || body[0] != 0x50 || body[1] != 0x4B {
		t.Error("expected response to be a valid ZIP file (magic bytes PK)")
	}
}
