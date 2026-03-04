package storage

import (
	"testing"
	"time"

	"github.com/nstalgic/nekzus/internal/types"
)

// TestSaveDeployment tests saving a toolbox deployment
func TestSaveDeployment(t *testing.T) {
	store := setupToolboxTestDB(t)
	defer cleanupToolboxTestDB(t, store)

	deployment := &types.ToolboxDeployment{
		ID:                "deploy_test123",
		ServiceTemplateID: "grafana",
		ServiceName:       "my-grafana",
		Status:            types.DeploymentStatusPending,
		ContainerID:       "",
		ContainerName:     "my-grafana",
		NetworkNames:      []string{"bridge"},
		VolumeNames:       []string{"grafana-data"},
		EnvVars: map[string]string{
			"GF_ADMIN_PASSWORD": "secret",
		},
		DeployedBy: "device123",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	err := store.SaveDeployment(deployment)
	if err != nil {
		t.Fatalf("Failed to save deployment: %v", err)
	}

	// Retrieve and verify
	retrieved, err := store.GetDeployment(deployment.ID)
	if err != nil {
		t.Fatalf("Failed to get deployment: %v", err)
	}

	if retrieved.ID != deployment.ID {
		t.Errorf("Expected ID %s, got %s", deployment.ID, retrieved.ID)
	}
	if retrieved.ServiceTemplateID != deployment.ServiceTemplateID {
		t.Errorf("Expected ServiceTemplateID %s, got %s", deployment.ServiceTemplateID, retrieved.ServiceTemplateID)
	}
	if retrieved.ServiceName != deployment.ServiceName {
		t.Errorf("Expected ServiceName %s, got %s", deployment.ServiceName, retrieved.ServiceName)
	}
	if retrieved.Status != deployment.Status {
		t.Errorf("Expected Status %s, got %s", deployment.Status, retrieved.Status)
	}
	if retrieved.DeployedBy != deployment.DeployedBy {
		t.Errorf("Expected DeployedBy %s, got %s", deployment.DeployedBy, retrieved.DeployedBy)
	}
}

// TestGetDeployment_NotFound tests getting a non-existent deployment
func TestGetDeployment_NotFound(t *testing.T) {
	store := setupToolboxTestDB(t)
	defer cleanupToolboxTestDB(t, store)

	_, err := store.GetDeployment("nonexistent")
	if err == nil {
		t.Error("Expected error when getting non-existent deployment")
	}
}

// TestListDeployments tests listing all deployments
func TestListDeployments(t *testing.T) {
	store := setupToolboxTestDB(t)
	defer cleanupToolboxTestDB(t, store)

	// Save multiple deployments
	deployments := []*types.ToolboxDeployment{
		{
			ID:                "deploy_1",
			ServiceTemplateID: "grafana",
			ServiceName:       "grafana-1",
			Status:            types.DeploymentStatusDeployed,
			ContainerID:       "container_1",
			ContainerName:     "grafana-1",
			DeployedBy:        "device123",
			CreatedAt:         time.Now(),
			UpdatedAt:         time.Now(),
		},
		{
			ID:                "deploy_2",
			ServiceTemplateID: "pihole",
			ServiceName:       "pihole-1",
			Status:            types.DeploymentStatusPending,
			ContainerName:     "pihole-1",
			DeployedBy:        "device123",
			CreatedAt:         time.Now(),
			UpdatedAt:         time.Now(),
		},
		{
			ID:                "deploy_3",
			ServiceTemplateID: "uptime-kuma",
			ServiceName:       "uptime-kuma-1",
			Status:            types.DeploymentStatusDeployed,
			ContainerID:       "container_3",
			ContainerName:     "uptime-kuma-1",
			DeployedBy:        "device456",
			CreatedAt:         time.Now(),
			UpdatedAt:         time.Now(),
		},
	}

	for _, deployment := range deployments {
		if err := store.SaveDeployment(deployment); err != nil {
			t.Fatalf("Failed to save deployment: %v", err)
		}
	}

	// List all deployments
	retrieved, err := store.ListDeployments()
	if err != nil {
		t.Fatalf("Failed to list deployments: %v", err)
	}

	if len(retrieved) != 3 {
		t.Errorf("Expected 3 deployments, got %d", len(retrieved))
	}
}

// TestListDeploymentsByStatus tests filtering deployments by status
func TestListDeploymentsByStatus(t *testing.T) {
	store := setupToolboxTestDB(t)
	defer cleanupToolboxTestDB(t, store)

	// Save deployments with different statuses
	deployments := []*types.ToolboxDeployment{
		{
			ID:                "deploy_1",
			ServiceTemplateID: "grafana",
			ServiceName:       "grafana-1",
			Status:            types.DeploymentStatusDeployed,
			ContainerName:     "grafana-1",
			DeployedBy:        "device123",
			CreatedAt:         time.Now(),
			UpdatedAt:         time.Now(),
		},
		{
			ID:                "deploy_2",
			ServiceTemplateID: "pihole",
			ServiceName:       "pihole-1",
			Status:            types.DeploymentStatusPending,
			ContainerName:     "pihole-1",
			DeployedBy:        "device123",
			CreatedAt:         time.Now(),
			UpdatedAt:         time.Now(),
		},
		{
			ID:                "deploy_3",
			ServiceTemplateID: "uptime-kuma",
			ServiceName:       "uptime-kuma-1",
			Status:            types.DeploymentStatusDeployed,
			ContainerName:     "uptime-kuma-1",
			DeployedBy:        "device123",
			CreatedAt:         time.Now(),
			UpdatedAt:         time.Now(),
		},
	}

	for _, deployment := range deployments {
		if err := store.SaveDeployment(deployment); err != nil {
			t.Fatalf("Failed to save deployment: %v", err)
		}
	}

	// List deployed services only
	deployed, err := store.ListDeploymentsByStatus(types.DeploymentStatusDeployed)
	if err != nil {
		t.Fatalf("Failed to list deployments by status: %v", err)
	}

	if len(deployed) != 2 {
		t.Errorf("Expected 2 deployed services, got %d", len(deployed))
	}

	// List pending services only
	pending, err := store.ListDeploymentsByStatus(types.DeploymentStatusPending)
	if err != nil {
		t.Fatalf("Failed to list deployments by status: %v", err)
	}

	if len(pending) != 1 {
		t.Errorf("Expected 1 pending service, got %d", len(pending))
	}
}

// TestUpdateDeploymentStatus tests updating deployment status
func TestUpdateDeploymentStatus(t *testing.T) {
	store := setupToolboxTestDB(t)
	defer cleanupToolboxTestDB(t, store)

	deployment := &types.ToolboxDeployment{
		ID:                "deploy_test123",
		ServiceTemplateID: "grafana",
		ServiceName:       "my-grafana",
		Status:            types.DeploymentStatusPending,
		ContainerName:     "my-grafana",
		DeployedBy:        "device123",
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	if err := store.SaveDeployment(deployment); err != nil {
		t.Fatalf("Failed to save deployment: %v", err)
	}

	// Update status to deploying
	err := store.UpdateDeploymentStatus(deployment.ID, types.DeploymentStatusDeploying, "")
	if err != nil {
		t.Fatalf("Failed to update deployment status: %v", err)
	}

	// Verify status changed
	retrieved, err := store.GetDeployment(deployment.ID)
	if err != nil {
		t.Fatalf("Failed to get deployment: %v", err)
	}

	if retrieved.Status != types.DeploymentStatusDeploying {
		t.Errorf("Expected status %s, got %s", types.DeploymentStatusDeploying, retrieved.Status)
	}

	// Update status to failed with error message
	err = store.UpdateDeploymentStatus(deployment.ID, types.DeploymentStatusFailed, "Deployment failed: port conflict")
	if err != nil {
		t.Fatalf("Failed to update deployment status: %v", err)
	}

	// Verify status and error message
	retrieved, err = store.GetDeployment(deployment.ID)
	if err != nil {
		t.Fatalf("Failed to get deployment: %v", err)
	}

	if retrieved.Status != types.DeploymentStatusFailed {
		t.Errorf("Expected status %s, got %s", types.DeploymentStatusFailed, retrieved.Status)
	}
	if retrieved.ErrorMessage != "Deployment failed: port conflict" {
		t.Errorf("Expected error message 'Deployment failed: port conflict', got %s", retrieved.ErrorMessage)
	}
}

// TestUpdateDeploymentContainer tests updating deployment container info
func TestUpdateDeploymentContainer(t *testing.T) {
	store := setupToolboxTestDB(t)
	defer cleanupToolboxTestDB(t, store)

	deployment := &types.ToolboxDeployment{
		ID:                "deploy_test123",
		ServiceTemplateID: "grafana",
		ServiceName:       "my-grafana",
		Status:            types.DeploymentStatusDeploying,
		ContainerName:     "my-grafana",
		DeployedBy:        "device123",
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	if err := store.SaveDeployment(deployment); err != nil {
		t.Fatalf("Failed to save deployment: %v", err)
	}

	// Update container ID
	err := store.UpdateDeploymentContainer(deployment.ID, "container_abc123", "route_xyz789")
	if err != nil {
		t.Fatalf("Failed to update deployment container: %v", err)
	}

	// Verify container ID and route ID updated
	retrieved, err := store.GetDeployment(deployment.ID)
	if err != nil {
		t.Fatalf("Failed to get deployment: %v", err)
	}

	if retrieved.ContainerID != "container_abc123" {
		t.Errorf("Expected container ID 'container_abc123', got %s", retrieved.ContainerID)
	}
	if retrieved.RouteID != "route_xyz789" {
		t.Errorf("Expected route ID 'route_xyz789', got %s", retrieved.RouteID)
	}
}

// TestDeleteDeployment tests deleting a deployment
func TestDeleteDeployment(t *testing.T) {
	store := setupToolboxTestDB(t)
	defer cleanupToolboxTestDB(t, store)

	deployment := &types.ToolboxDeployment{
		ID:                "deploy_test123",
		ServiceTemplateID: "grafana",
		ServiceName:       "my-grafana",
		Status:            types.DeploymentStatusDeployed,
		ContainerID:       "container_abc123",
		ContainerName:     "my-grafana",
		DeployedBy:        "device123",
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	if err := store.SaveDeployment(deployment); err != nil {
		t.Fatalf("Failed to save deployment: %v", err)
	}

	// Delete deployment
	err := store.DeleteDeployment(deployment.ID)
	if err != nil {
		t.Fatalf("Failed to delete deployment: %v", err)
	}

	// Verify deletion
	_, err = store.GetDeployment(deployment.ID)
	if err == nil {
		t.Error("Expected error when getting deleted deployment")
	}
}

// TestDeleteDeployment_NotFound tests deleting a non-existent deployment
func TestDeleteDeployment_NotFound(t *testing.T) {
	store := setupToolboxTestDB(t)
	defer cleanupToolboxTestDB(t, store)

	// Deleting non-existent deployment should not error (idempotent)
	err := store.DeleteDeployment("nonexistent")
	if err != nil {
		t.Errorf("Deleting non-existent deployment should be idempotent, got error: %v", err)
	}
}

// Test helpers specific to toolbox tests
func setupToolboxTestDB(t *testing.T) *Store {
	t.Helper()
	dbPath := t.TempDir() + "/toolbox_test.db"
	store, err := NewStore(Config{DatabasePath: dbPath})
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	return store
}

func cleanupToolboxTestDB(t *testing.T, store *Store) {
	t.Helper()
	if store != nil {
		store.Close()
	}
}
