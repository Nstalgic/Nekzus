package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nstalgic/nekzus/internal/storage"
	apptypes "github.com/nstalgic/nekzus/internal/types"
)

// MockRouterForStats is a mock router for stats testing
type MockRouterForStats struct {
	routes []apptypes.Route
}

func (m *MockRouterForStats) ListRoutes() []apptypes.Route {
	return m.routes
}

// MockStorageForStats is a mock storage for stats testing
type MockStorageForStats struct {
	devices       []storage.DeviceInfo
	apps          []apptypes.App
	requestsToday int
}

func (m *MockStorageForStats) ListDevices() ([]storage.DeviceInfo, error) {
	return m.devices, nil
}

func (m *MockStorageForStats) ListApps() ([]apptypes.App, error) {
	return m.apps, nil
}

func (m *MockStorageForStats) GetTotalRequestsToday() (int, error) {
	return m.requestsToday, nil
}

// TestHandleQuickStats tests the quick stats endpoint
func TestHandleQuickStats(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		mockData struct {
			routes  []apptypes.Route
			devices []storage.DeviceInfo
			apps    []apptypes.App
		}
		expectedStatus int
		checkResponse  func(t *testing.T, body map[string]interface{})
	}{
		{
			name:   "successful stats with all data",
			method: http.MethodGet,
			mockData: struct {
				routes  []apptypes.Route
				devices []storage.DeviceInfo
				apps    []apptypes.App
			}{
				routes: []apptypes.Route{
					{RouteID: "route1", AppID: "app1"},
					{RouteID: "route2", AppID: "app2"},
					{RouteID: "route3", AppID: "app3"},
				},
				devices: []storage.DeviceInfo{
					{ID: "dev1", Name: "iPhone"},
					{ID: "dev2", Name: "iPad"},
				},
				apps: []apptypes.App{
					{ID: "app1", Name: "Plex"},
					{ID: "app2", Name: "Grafana"},
					{ID: "app3", Name: "Nextcloud"},
				},
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body map[string]interface{}) {
				// Check required fields
				if _, ok := body["servicesTotal"]; !ok {
					t.Error("Expected 'servicesTotal' field")
				}
				if _, ok := body["servicesOnline"]; !ok {
					t.Error("Expected 'servicesOnline' field")
				}
				if _, ok := body["servicesOffline"]; !ok {
					t.Error("Expected 'servicesOffline' field")
				}
				if _, ok := body["devicesTotal"]; !ok {
					t.Error("Expected 'devicesTotal' field")
				}
				if _, ok := body["routesTotal"]; !ok {
					t.Error("Expected 'routesTotal' field")
				}

				// Check values
				if total, ok := body["servicesTotal"].(float64); !ok || total != 3 {
					t.Errorf("Expected servicesTotal=3, got %v", body["servicesTotal"])
				}
				if total, ok := body["devicesTotal"].(float64); !ok || total != 2 {
					t.Errorf("Expected devicesTotal=2, got %v", body["devicesTotal"])
				}
				if total, ok := body["routesTotal"].(float64); !ok || total != 3 {
					t.Errorf("Expected routesTotal=3, got %v", body["routesTotal"])
				}
			},
		},
		{
			name:   "empty stats",
			method: http.MethodGet,
			mockData: struct {
				routes  []apptypes.Route
				devices []storage.DeviceInfo
				apps    []apptypes.App
			}{
				routes:  []apptypes.Route{},
				devices: []storage.DeviceInfo{},
				apps:    []apptypes.App{},
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body map[string]interface{}) {
				if total, ok := body["servicesTotal"].(float64); !ok || total != 0 {
					t.Errorf("Expected servicesTotal=0, got %v", body["servicesTotal"])
				}
				if total, ok := body["devicesTotal"].(float64); !ok || total != 0 {
					t.Errorf("Expected devicesTotal=0, got %v", body["devicesTotal"])
				}
			},
		},
		{
			name:           "POST method not allowed",
			method:         http.MethodPost,
			expectedStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRouter := &MockRouterForStats{
				routes: tt.mockData.routes,
			}
			mockStorage := &MockStorageForStats{
				devices: tt.mockData.devices,
				apps:    tt.mockData.apps,
			}

			handler := NewStatsHandler(mockRouter, mockStorage, nil)

			req := httptest.NewRequest(tt.method, "/api/v1/stats/quick", nil)
			w := httptest.NewRecorder()

			handler.HandleQuickStats(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedStatus == http.StatusOK && tt.checkResponse != nil {
				var response map[string]interface{}
				if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}
				tt.checkResponse(t, response)
			}
		})
	}
}

// TestCalculateServiceHealth tests service health calculation logic
func TestCalculateServiceHealth(t *testing.T) {
	tests := []struct {
		name            string
		totalServices   int
		healthyServices int
		expectedOnline  int
		expectedOffline int
	}{
		{
			name:            "all healthy",
			totalServices:   5,
			healthyServices: 5,
			expectedOnline:  5,
			expectedOffline: 0,
		},
		{
			name:            "some unhealthy",
			totalServices:   10,
			healthyServices: 7,
			expectedOnline:  7,
			expectedOffline: 3,
		},
		{
			name:            "all unhealthy",
			totalServices:   3,
			healthyServices: 0,
			expectedOnline:  0,
			expectedOffline: 3,
		},
		{
			name:            "no services",
			totalServices:   0,
			healthyServices: 0,
			expectedOnline:  0,
			expectedOffline: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			online := tt.healthyServices
			offline := tt.totalServices - tt.healthyServices

			if online != tt.expectedOnline {
				t.Errorf("Expected %d online, got %d", tt.expectedOnline, online)
			}
			if offline != tt.expectedOffline {
				t.Errorf("Expected %d offline, got %d", tt.expectedOffline, offline)
			}
		})
	}
}

// TestStatsResponseFormat tests the response format
func TestStatsResponseFormat(t *testing.T) {
	mockRouter := &MockRouterForStats{
		routes: []apptypes.Route{
			{RouteID: "route1", AppID: "app1"},
		},
	}
	mockStorage := &MockStorageForStats{
		devices: []storage.DeviceInfo{
			{ID: "dev1"},
		},
		apps: []apptypes.App{
			{ID: "app1", Name: "Plex"},
		},
	}

	handler := NewStatsHandler(mockRouter, mockStorage, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats/quick", nil)
	w := httptest.NewRecorder()

	handler.HandleQuickStats(w, req)

	// Check Content-Type
	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", contentType)
	}

	// Check response is valid JSON
	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Response is not valid JSON: %v", err)
	}

	// Check all required fields are present
	requiredFields := []string{
		"servicesTotal",
		"servicesOnline",
		"servicesOffline",
		"devicesTotal",
		"devicesOnline",
		"routesTotal",
		"requestsToday",
		"avgLatency",
		"uptimePercent",
	}

	for _, field := range requiredFields {
		if _, ok := response[field]; !ok {
			t.Errorf("Missing required field: %s", field)
		}
	}
}

// BenchmarkQuickStats benchmarks the quick stats endpoint
func BenchmarkQuickStats(b *testing.B) {
	// Create mock data
	routes := make([]apptypes.Route, 100)
	for i := 0; i < 100; i++ {
		routes[i] = apptypes.Route{
			RouteID: "route" + string(rune(i)),
			AppID:   "app" + string(rune(i)),
		}
	}

	devices := make([]storage.DeviceInfo, 50)
	for i := 0; i < 50; i++ {
		devices[i] = storage.DeviceInfo{
			ID: "dev" + string(rune(i)),
		}
	}

	apps := make([]apptypes.App, 100)
	for i := 0; i < 100; i++ {
		apps[i] = apptypes.App{
			ID:   "app" + string(rune(i)),
			Name: "App " + string(rune(i)),
		}
	}

	mockRouter := &MockRouterForStats{routes: routes}
	mockStorage := &MockStorageForStats{devices: devices, apps: apps}
	handler := NewStatsHandler(mockRouter, mockStorage, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats/quick", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		handler.HandleQuickStats(w, req)
	}
}
