package storage

import (
	"errors"
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/types"
)

// MockDeviceRepository is a mock implementation of DeviceRepository for testing
type MockDeviceRepository struct {
	devices           map[string]*DeviceInfo
	saveErr           error
	getErr            error
	listErr           error
	deleteErr         error
	updateLastSeenErr error
	requestsToday     int
}

func NewMockDeviceRepository() *MockDeviceRepository {
	return &MockDeviceRepository{
		devices: make(map[string]*DeviceInfo),
	}
}

func (m *MockDeviceRepository) SaveDevice(deviceID, deviceName, platform, platformVersion string, scopes []string) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.devices[deviceID] = &DeviceInfo{
		ID:              deviceID,
		Name:            deviceName,
		Platform:        platform,
		PlatformVersion: platformVersion,
		Scopes:          scopes,
		PairedAt:        time.Now(),
	}
	return nil
}

func (m *MockDeviceRepository) GetDevice(deviceID string) (*DeviceInfo, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	device, exists := m.devices[deviceID]
	if !exists {
		return nil, errors.New("device not found")
	}
	return device, nil
}

func (m *MockDeviceRepository) ListDevices() ([]DeviceInfo, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	devices := make([]DeviceInfo, 0, len(m.devices))
	for _, device := range m.devices {
		devices = append(devices, *device)
	}
	return devices, nil
}

func (m *MockDeviceRepository) DeleteDevice(deviceID string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.devices, deviceID)
	return nil
}

func (m *MockDeviceRepository) UpdateDeviceLastSeen(deviceID string) error {
	if m.updateLastSeenErr != nil {
		return m.updateLastSeenErr
	}
	if device, exists := m.devices[deviceID]; exists {
		device.LastSeen = time.Now()
		return nil
	}
	return errors.New("device not found")
}

func (m *MockDeviceRepository) GetDeviceRequestsToday(deviceID string) (int, error) {
	return m.requestsToday, nil
}

// TestDeviceRepositoryInterface tests that Store implements DeviceRepository
func TestDeviceRepositoryInterface(t *testing.T) {
	// This test verifies that Store satisfies the DeviceRepository interface
	var _ DeviceRepository = (*Store)(nil)

	// Also test with in-memory database
	store, err := NewStore(Config{DatabasePath: ":memory:"})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Test SaveDevice
	err = store.SaveDevice("test-device", "Test Device", "ios", "17.0", []string{"read", "write"})
	if err != nil {
		t.Errorf("SaveDevice failed: %v", err)
	}

	// Test GetDevice
	device, err := store.GetDevice("test-device")
	if err != nil {
		t.Errorf("GetDevice failed: %v", err)
	}
	if device == nil {
		t.Error("Expected device, got nil")
	}
	if device.ID != "test-device" {
		t.Errorf("Expected device ID 'test-device', got %s", device.ID)
	}

	// Test ListDevices
	devices, err := store.ListDevices()
	if err != nil {
		t.Errorf("ListDevices failed: %v", err)
	}
	if len(devices) != 1 {
		t.Errorf("Expected 1 device, got %d", len(devices))
	}

	// Test UpdateDeviceLastSeen
	err = store.UpdateDeviceLastSeen("test-device")
	if err != nil {
		t.Errorf("UpdateDeviceLastSeen failed: %v", err)
	}

	// Test DeleteDevice
	err = store.DeleteDevice("test-device")
	if err != nil {
		t.Errorf("DeleteDevice failed: %v", err)
	}

	// Verify deletion
	device, err = store.GetDevice("test-device")
	if device != nil {
		t.Error("Expected nil after deletion, got device")
	}
}

// TestMockDeviceRepository tests the mock implementation
func TestMockDeviceRepository(t *testing.T) {
	mock := NewMockDeviceRepository()

	// Test SaveDevice
	err := mock.SaveDevice("mock-device", "Mock Device", "android", "14", []string{"read"})
	if err != nil {
		t.Errorf("SaveDevice failed: %v", err)
	}

	// Test GetDevice
	device, err := mock.GetDevice("mock-device")
	if err != nil {
		t.Errorf("GetDevice failed: %v", err)
	}
	if device.ID != "mock-device" {
		t.Errorf("Expected device ID 'mock-device', got %s", device.ID)
	}

	// Test ListDevices
	devices, err := mock.ListDevices()
	if err != nil {
		t.Errorf("ListDevices failed: %v", err)
	}
	if len(devices) != 1 {
		t.Errorf("Expected 1 device, got %d", len(devices))
	}

	// Test DeleteDevice
	err = mock.DeleteDevice("mock-device")
	if err != nil {
		t.Errorf("DeleteDevice failed: %v", err)
	}

	// Verify deletion
	_, err = mock.GetDevice("mock-device")
	if err == nil {
		t.Error("Expected error for non-existent device, got nil")
	}
}

// TestMockDeviceRepositoryErrors tests error handling with mock
func TestMockDeviceRepositoryErrors(t *testing.T) {
	mock := NewMockDeviceRepository()

	// Test SaveDevice error
	mock.saveErr = errors.New("save failed")
	err := mock.SaveDevice("test", "Test", "ios", "17", []string{"read"})
	if err == nil {
		t.Error("Expected error, got nil")
	}
	mock.saveErr = nil

	// Test GetDevice error
	mock.getErr = errors.New("get failed")
	_, err = mock.GetDevice("test")
	if err == nil {
		t.Error("Expected error, got nil")
	}
	mock.getErr = nil

	// Test ListDevices error
	mock.listErr = errors.New("list failed")
	_, err = mock.ListDevices()
	if err == nil {
		t.Error("Expected error, got nil")
	}
	mock.listErr = nil

	// Test DeleteDevice error
	mock.deleteErr = errors.New("delete failed")
	err = mock.DeleteDevice("test")
	if err == nil {
		t.Error("Expected error, got nil")
	}
	mock.deleteErr = nil

	// Test UpdateDeviceLastSeen error
	mock.updateLastSeenErr = errors.New("update failed")
	err = mock.UpdateDeviceLastSeen("test")
	if err == nil {
		t.Error("Expected error, got nil")
	}
}

// TestAppRepositoryInterface tests that Store implements AppRepository
func TestAppRepositoryInterface(t *testing.T) {
	var _ AppRepository = (*Store)(nil)

	store, err := NewStore(Config{DatabasePath: ":memory:"})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Test SaveApp
	app := types.App{
		ID:   "test-app",
		Name: "Test App",
		Icon: "📱",
	}
	err = store.SaveApp(app)
	if err != nil {
		t.Errorf("SaveApp failed: %v", err)
	}

	// Test GetApp
	retrievedApp, err := store.GetApp("test-app")
	if err != nil {
		t.Errorf("GetApp failed: %v", err)
	}
	if retrievedApp == nil {
		t.Fatal("Expected app, got nil")
	}
	if retrievedApp.ID != "test-app" {
		t.Errorf("Expected app ID 'test-app', got %s", retrievedApp.ID)
	}

	// Test ListApps
	apps, err := store.ListApps()
	if err != nil {
		t.Errorf("ListApps failed: %v", err)
	}
	if len(apps) != 1 {
		t.Errorf("Expected 1 app, got %d", len(apps))
	}

	// Test DeleteApp
	err = store.DeleteApp("test-app")
	if err != nil {
		t.Errorf("DeleteApp failed: %v", err)
	}
}

// TestRouteRepositoryInterface tests that Store implements RouteRepository
func TestRouteRepositoryInterface(t *testing.T) {
	var _ RouteRepository = (*Store)(nil)

	store, err := NewStore(Config{DatabasePath: ":memory:"})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Create app first (foreign key requirement)
	app := types.App{
		ID:   "test-app",
		Name: "Test App",
		Icon: "📦",
	}
	err = store.SaveApp(app)
	if err != nil {
		t.Fatalf("Failed to save app: %v", err)
	}

	// Test SaveRoute
	route := types.Route{
		RouteID:  "test-route",
		AppID:    "test-app",
		PathBase: "/test",
		To:       "http://localhost:8080",
		Scopes:   []string{"read"},
	}
	err = store.SaveRoute(route)
	if err != nil {
		t.Errorf("SaveRoute failed: %v", err)
	}

	// Test GetRoute
	retrievedRoute, err := store.GetRoute("test-route")
	if err != nil {
		t.Errorf("GetRoute failed: %v", err)
	}
	if retrievedRoute == nil {
		t.Fatal("Expected route, got nil")
	}
	if retrievedRoute.RouteID != "test-route" {
		t.Errorf("Expected route ID 'test-route', got %s", retrievedRoute.RouteID)
	}

	// Test ListRoutes
	routes, err := store.ListRoutes()
	if err != nil {
		t.Errorf("ListRoutes failed: %v", err)
	}
	if len(routes) != 1 {
		t.Errorf("Expected 1 route, got %d", len(routes))
	}

	// Test DeleteRoute
	err = store.DeleteRoute("test-route")
	if err != nil {
		t.Errorf("DeleteRoute failed: %v", err)
	}
}

// TestProposalRepositoryInterface tests that Store implements ProposalRepository
func TestProposalRepositoryInterface(t *testing.T) {
	var _ ProposalRepository = (*Store)(nil)

	store, err := NewStore(Config{DatabasePath: ":memory:"})
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Test SaveProposal
	proposal := types.Proposal{
		ID:             "test-proposal",
		Source:         "docker",
		DetectedScheme: "http",
		DetectedHost:   "localhost",
		DetectedPort:   9000,
		Confidence:     0.9,
		SuggestedApp: types.App{
			ID:   "test-service",
			Name: "Test Service",
			Icon: "🚀",
		},
		SuggestedRoute: types.Route{
			RouteID:  "test-route",
			AppID:    "test-service",
			PathBase: "/test",
			To:       "http://localhost:9000",
		},
	}
	err = store.SaveProposal(proposal)
	if err != nil {
		t.Errorf("SaveProposal failed: %v", err)
	}

	// Test GetProposal
	retrievedProposal, err := store.GetProposal("test-proposal")
	if err != nil {
		t.Errorf("GetProposal failed: %v", err)
	}
	if retrievedProposal == nil {
		t.Fatal("Expected proposal, got nil")
	}
	if retrievedProposal.ID != "test-proposal" {
		t.Errorf("Expected proposal ID 'test-proposal', got %s", retrievedProposal.ID)
	}

	// Test ListProposals
	proposals, err := store.ListProposals()
	if err != nil {
		t.Errorf("ListProposals failed: %v", err)
	}
	if len(proposals) != 1 {
		t.Errorf("Expected 1 proposal, got %d", len(proposals))
	}

	// Test ClearProposals
	err = store.ClearProposals()
	if err != nil {
		t.Errorf("ClearProposals failed: %v", err)
	}

	// Verify cleared
	proposals, err = store.ListProposals()
	if err != nil {
		t.Errorf("ListProposals failed: %v", err)
	}
	if len(proposals) != 0 {
		t.Errorf("Expected 0 proposals after clear, got %d", len(proposals))
	}
}
