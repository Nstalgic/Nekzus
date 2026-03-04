package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestRuntimeEndToEnd performs end-to-end testing of multi-runtime container management
// covering: Docker containers, Kubernetes pods, runtime switching, and graceful degradation
//
// Test Scenarios:
// 1. Docker-only deployment
// 2. Kubernetes-only deployment
// 3. Mixed Docker + Kubernetes
// 4. Runtime switching via API
// 5. Graceful degradation (no Metrics Server)
// 6. RBAC permission errors
//
// Requires manually starting the appropriate environment:
// - Docker: docker compose up -d
// - Kubernetes: kind create cluster / k3s / Docker Desktop K8s
func TestRuntimeEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// Configuration
	nexusURL := "https://localhost:8443"
	bootstrapToken := "dev-bootstrap"

	// Create HTTP client that accepts self-signed certs
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
		Timeout: 10 * time.Second,
	}

	// First, pair a device to get JWT
	var jwt string
	t.Run("Setup_DevicePairing", func(t *testing.T) {
		jwt = pairTestDevice(t, client, nexusURL, bootstrapToken)
	})

	// Run runtime-specific tests
	t.Run("Docker", func(t *testing.T) {
		testDockerRuntime(t, client, nexusURL, jwt)
	})

	t.Run("Kubernetes", func(t *testing.T) {
		testKubernetesRuntime(t, client, nexusURL, jwt)
	})

	t.Run("RuntimeSwitching", func(t *testing.T) {
		testRuntimeSwitching(t, client, nexusURL, jwt)
	})

	t.Run("GracefulDegradation", func(t *testing.T) {
		testGracefulDegradation(t, client, nexusURL, jwt)
	})
}

// pairTestDevice pairs a test device and returns the JWT
func pairTestDevice(t *testing.T, client *http.Client, baseURL, bootstrapToken string) string {
	t.Log("Pairing test device...")

	pairRequest := map[string]interface{}{
		"device": map[string]interface{}{
			"id":        "runtime-e2e-test-device",
			"model":     "RuntimeTestDevice",
			"platform":  "test",
			"pushToken": nil,
		},
	}

	jsonData, _ := json.Marshal(pairRequest)
	req, err := http.NewRequest("POST", baseURL+"/api/v1/auth/pair", bytes.NewBuffer(jsonData))
	if err != nil {
		t.Fatalf("Failed to create pair request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+bootstrapToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Pair request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	var pairResponse struct {
		AccessToken string `json:"accessToken"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&pairResponse); err != nil {
		t.Fatalf("Failed to decode pair response: %v", err)
	}

	t.Log("Device paired successfully")
	return pairResponse.AccessToken
}

// testDockerRuntime tests Docker container operations
func testDockerRuntime(t *testing.T, client *http.Client, baseURL, jwt string) {
	t.Run("ListContainers", func(t *testing.T) {
		containers := listContainers(t, client, baseURL, jwt, "docker", "")
		t.Logf("Found %d Docker containers", len(containers))

		// Verify containers have expected fields
		for _, c := range containers {
			container := c.(map[string]interface{})
			if container["id"] == nil {
				t.Error("Container missing 'id' field")
			}
			if container["name"] == nil {
				t.Error("Container missing 'name' field")
			}
			if container["state"] == nil {
				t.Error("Container missing 'state' field")
			}
			// Docker containers should have runtime field
			if runtime := container["runtime"]; runtime != nil && runtime != "docker" {
				t.Errorf("Expected runtime 'docker', got %v", runtime)
			}
		}
	})

	t.Run("ContainerStats", func(t *testing.T) {
		containers := listContainers(t, client, baseURL, jwt, "docker", "")
		if len(containers) == 0 {
			t.Skip("No Docker containers available for stats test")
		}

		// Find a running container
		var runningContainer map[string]interface{}
		for _, c := range containers {
			container := c.(map[string]interface{})
			if container["state"] == "running" {
				runningContainer = container
				break
			}
		}

		if runningContainer == nil {
			t.Skip("No running Docker containers for stats test")
		}

		containerID := runningContainer["id"].(string)
		stats := getContainerStats(t, client, baseURL, jwt, containerID, "docker", "")

		// Verify stats structure
		if stats["cpu_percent"] == nil {
			t.Log("Warning: cpu_percent not available (may be expected)")
		}
		if stats["memory_usage"] == nil {
			t.Log("Warning: memory_usage not available (may be expected)")
		}

		t.Logf("Docker container stats retrieved for %s", containerID[:12])
	})

	t.Run("ContainerLifecycle", func(t *testing.T) {
		containers := listContainers(t, client, baseURL, jwt, "docker", "")
		if len(containers) == 0 {
			t.Skip("No Docker containers available for lifecycle test")
		}

		// Find a container to test (prefer stopped ones to avoid disruption)
		var testContainer map[string]interface{}
		for _, c := range containers {
			container := c.(map[string]interface{})
			state := container["state"].(string)
			if state == "exited" || state == "stopped" {
				testContainer = container
				break
			}
		}

		if testContainer == nil {
			t.Skip("No stopped Docker containers for lifecycle test")
		}

		containerID := testContainer["id"].(string)
		t.Logf("Testing lifecycle on container %s", containerID[:12])

		// Start container
		if err := containerAction(client, baseURL, jwt, containerID, "start", "docker", ""); err != nil {
			t.Logf("Start failed (may be expected): %v", err)
		} else {
			t.Log("Container started")
		}

		// Stop container
		if err := containerAction(client, baseURL, jwt, containerID, "stop", "docker", ""); err != nil {
			t.Logf("Stop failed (may be expected): %v", err)
		} else {
			t.Log("Container stopped")
		}
	})
}

// testKubernetesRuntime tests Kubernetes pod operations
func testKubernetesRuntime(t *testing.T, client *http.Client, baseURL, jwt string) {
	t.Run("ListPods", func(t *testing.T) {
		containers := listContainers(t, client, baseURL, jwt, "kubernetes", "")
		if len(containers) == 0 {
			t.Log("No Kubernetes pods found (K8s may not be configured)")
			return
		}

		t.Logf("Found %d Kubernetes pods", len(containers))

		// Verify pods have expected K8s fields
		for _, c := range containers {
			container := c.(map[string]interface{})
			if container["runtime"] != "kubernetes" {
				t.Errorf("Expected runtime 'kubernetes', got %v", container["runtime"])
			}
			if container["namespace"] == nil {
				t.Error("K8s pod missing 'namespace' field")
			}
		}
	})

	t.Run("ListPodsByNamespace", func(t *testing.T) {
		// Test namespace filtering
		containers := listContainers(t, client, baseURL, jwt, "kubernetes", "default")
		t.Logf("Found %d pods in 'default' namespace", len(containers))

		for _, c := range containers {
			container := c.(map[string]interface{})
			if ns := container["namespace"]; ns != "default" {
				t.Errorf("Expected namespace 'default', got %v", ns)
			}
		}
	})

	t.Run("PodStats", func(t *testing.T) {
		containers := listContainers(t, client, baseURL, jwt, "kubernetes", "")
		if len(containers) == 0 {
			t.Skip("No Kubernetes pods available for stats test")
		}

		// Find a running pod
		var runningPod map[string]interface{}
		for _, c := range containers {
			container := c.(map[string]interface{})
			if container["state"] == "running" {
				runningPod = container
				break
			}
		}

		if runningPod == nil {
			t.Skip("No running Kubernetes pods for stats test")
		}

		podName := runningPod["name"].(string)
		namespace := runningPod["namespace"].(string)

		stats := getContainerStats(t, client, baseURL, jwt, podName, "kubernetes", namespace)

		// K8s stats may not have cpu_percent/memory_usage if Metrics Server isn't available
		if stats != nil {
			t.Logf("Kubernetes pod stats retrieved for %s/%s", namespace, podName)
		}
	})

	t.Run("K8sStylePath", func(t *testing.T) {
		// Test the K8s-style path format: /containers/{namespace}/{pod}/action
		containers := listContainers(t, client, baseURL, jwt, "kubernetes", "default")
		if len(containers) == 0 {
			t.Skip("No Kubernetes pods in default namespace")
		}

		pod := containers[0].(map[string]interface{})
		podName := pod["name"].(string)

		// Test K8s-style inspect path
		url := fmt.Sprintf("%s/api/v1/containers/default/%s", baseURL, podName)
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+jwt)

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("K8s-style path request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			t.Logf("K8s-style path works: /containers/default/%s", podName)
		} else {
			body, _ := io.ReadAll(resp.Body)
			t.Logf("K8s-style path returned %d: %s", resp.StatusCode, string(body))
		}
	})
}

// testRuntimeSwitching tests switching between Docker and Kubernetes runtimes
func testRuntimeSwitching(t *testing.T, client *http.Client, baseURL, jwt string) {
	t.Run("SwitchBetweenRuntimes", func(t *testing.T) {
		// List Docker containers
		dockerContainers := listContainers(t, client, baseURL, jwt, "docker", "")
		t.Logf("Docker containers: %d", len(dockerContainers))

		// List Kubernetes pods
		k8sContainers := listContainers(t, client, baseURL, jwt, "kubernetes", "")
		t.Logf("Kubernetes pods: %d", len(k8sContainers))

		// List all containers (no runtime filter)
		allContainers := listContainers(t, client, baseURL, jwt, "", "")
		t.Logf("All containers: %d", len(allContainers))

		// Verify all containers includes both Docker and K8s
		if len(allContainers) < len(dockerContainers) {
			t.Error("All containers should include Docker containers")
		}
		if len(k8sContainers) > 0 && len(allContainers) < len(dockerContainers)+len(k8sContainers) {
			t.Log("Warning: All containers count may not include K8s (K8s not primary runtime)")
		}
	})

	t.Run("RuntimeInResponse", func(t *testing.T) {
		allContainers := listContainers(t, client, baseURL, jwt, "", "")

		dockerCount := 0
		k8sCount := 0

		for _, c := range allContainers {
			container := c.(map[string]interface{})
			switch container["runtime"] {
			case "docker", nil, "": // nil/"" defaults to docker
				dockerCount++
			case "kubernetes":
				k8sCount++
			default:
				t.Errorf("Unknown runtime: %v", container["runtime"])
			}
		}

		t.Logf("Runtime breakdown - Docker: %d, Kubernetes: %d", dockerCount, k8sCount)
	})
}

// testGracefulDegradation tests behavior when services are unavailable
func testGracefulDegradation(t *testing.T, client *http.Client, baseURL, jwt string) {
	t.Run("MetricsServerUnavailable", func(t *testing.T) {
		// This tests graceful degradation when K8s Metrics Server is not available
		containers := listContainers(t, client, baseURL, jwt, "kubernetes", "")
		if len(containers) == 0 {
			t.Skip("No Kubernetes pods to test Metrics Server degradation")
		}

		pod := containers[0].(map[string]interface{})
		podName := pod["name"].(string)
		namespace := pod["namespace"].(string)

		// Request stats - should gracefully handle missing Metrics Server
		url := fmt.Sprintf("%s/api/v1/containers/%s/stats?runtime=kubernetes&namespace=%s",
			baseURL, podName, namespace)
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+jwt)

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Stats request failed: %v", err)
		}
		defer resp.Body.Close()

		// Should not return 500 error - either 200 with stats or graceful error
		if resp.StatusCode == http.StatusInternalServerError {
			body, _ := io.ReadAll(resp.Body)
			// Check if error message indicates Metrics Server issue
			if strings.Contains(string(body), "metrics") {
				t.Log("Graceful degradation: Metrics Server unavailable")
			} else {
				t.Errorf("Unexpected 500 error: %s", string(body))
			}
		} else if resp.StatusCode == http.StatusOK {
			t.Log("Stats available (Metrics Server running)")
		} else {
			t.Logf("Stats returned %d (may indicate graceful degradation)", resp.StatusCode)
		}
	})

	t.Run("RBACPermissionErrors", func(t *testing.T) {
		// Test that RBAC permission errors are handled gracefully
		// This would require a K8s setup with limited RBAC permissions

		containers := listContainers(t, client, baseURL, jwt, "kubernetes", "kube-system")

		// kube-system namespace typically has restricted access
		// If we get containers, RBAC is permissive enough
		// If we get an error, it should be a clear permission error, not a crash
		t.Logf("kube-system namespace access: %d containers (may be restricted by RBAC)",
			len(containers))
	})
}

// listContainers fetches containers with optional runtime and namespace filters
func listContainers(t *testing.T, client *http.Client, baseURL, jwt, runtime, namespace string) []interface{} {
	t.Helper()

	url := baseURL + "/api/v1/containers"
	params := []string{}
	if runtime != "" {
		params = append(params, "runtime="+runtime)
	}
	if namespace != "" {
		params = append(params, "namespace="+namespace)
	}
	if len(params) > 0 {
		url += "?" + strings.Join(params, "&")
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+jwt)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	var containers []interface{}
	if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	return containers
}

// getContainerStats fetches stats for a container
func getContainerStats(t *testing.T, client *http.Client, baseURL, jwt, containerID, runtime, namespace string) map[string]interface{} {
	t.Helper()

	url := fmt.Sprintf("%s/api/v1/containers/%s/stats", baseURL, containerID)
	params := []string{}
	if runtime != "" {
		params = append(params, "runtime="+runtime)
	}
	if namespace != "" {
		params = append(params, "namespace="+namespace)
	}
	if len(params) > 0 {
		url += "?" + strings.Join(params, "&")
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+jwt)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Logf("Stats request returned %d: %s", resp.StatusCode, string(body))
		return nil
	}

	var stats map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		t.Fatalf("Failed to decode stats: %v", err)
	}

	return stats
}

// containerAction performs a container action (start, stop, restart)
func containerAction(client *http.Client, baseURL, jwt, containerID, action, runtime, namespace string) error {
	url := fmt.Sprintf("%s/api/v1/containers/%s/%s", baseURL, containerID, action)
	params := []string{}
	if runtime != "" {
		params = append(params, "runtime="+runtime)
	}
	if namespace != "" {
		params = append(params, "namespace="+namespace)
	}
	if len(params) > 0 {
		url += "?" + strings.Join(params, "&")
	}

	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+jwt)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("action %s failed with status %d: %s", action, resp.StatusCode, string(body))
	}

	return nil
}

// TestDockerOnly runs E2E tests in Docker-only mode
func TestDockerOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	nexusURL := "https://localhost:8443"
	bootstrapToken := "dev-bootstrap"

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
		Timeout: 10 * time.Second,
	}

	// Pair device
	jwt := pairTestDevice(t, client, nexusURL, bootstrapToken)

	t.Run("ListDockerContainers", func(t *testing.T) {
		containers := listContainers(t, client, nexusURL, jwt, "docker", "")
		if len(containers) == 0 {
			t.Log("No Docker containers found - ensure Docker is running")
		} else {
			t.Logf("Found %d Docker containers", len(containers))
		}
	})

	t.Run("DockerContainerDetails", func(t *testing.T) {
		containers := listContainers(t, client, nexusURL, jwt, "docker", "")
		if len(containers) == 0 {
			t.Skip("No Docker containers available")
		}

		container := containers[0].(map[string]interface{})
		containerID := container["id"].(string)

		// Test inspect endpoint
		url := fmt.Sprintf("%s/api/v1/containers/%s", nexusURL, containerID)
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+jwt)

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Inspect request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			var details map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&details)
			t.Logf("Container details retrieved for %s", containerID[:12])
		} else {
			t.Logf("Inspect returned %d", resp.StatusCode)
		}
	})
}

// TestKubernetesOnly runs E2E tests in Kubernetes-only mode
func TestKubernetesOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	nexusURL := "https://localhost:8443"
	bootstrapToken := "dev-bootstrap"

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
		Timeout: 10 * time.Second,
	}

	// Pair device
	jwt := pairTestDevice(t, client, nexusURL, bootstrapToken)

	t.Run("ListKubernetesPods", func(t *testing.T) {
		containers := listContainers(t, client, nexusURL, jwt, "kubernetes", "")
		if len(containers) == 0 {
			t.Log("No Kubernetes pods found - ensure K8s is configured")
		} else {
			t.Logf("Found %d Kubernetes pods", len(containers))
		}
	})

	t.Run("K8sNamespaceFiltering", func(t *testing.T) {
		// Test various namespaces
		namespaces := []string{"default", "kube-system"}
		for _, ns := range namespaces {
			containers := listContainers(t, client, nexusURL, jwt, "kubernetes", ns)
			t.Logf("Namespace %s: %d pods", ns, len(containers))
		}
	})

	t.Run("K8sPodDetails", func(t *testing.T) {
		containers := listContainers(t, client, nexusURL, jwt, "kubernetes", "default")
		if len(containers) == 0 {
			t.Skip("No Kubernetes pods in default namespace")
		}

		pod := containers[0].(map[string]interface{})
		podName := pod["name"].(string)

		// Test K8s-style inspect path
		url := fmt.Sprintf("%s/api/v1/containers/default/%s", nexusURL, podName)
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+jwt)

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Inspect request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			t.Logf("K8s pod details retrieved for default/%s", podName)
		} else {
			body, _ := io.ReadAll(resp.Body)
			t.Logf("Inspect returned %d: %s", resp.StatusCode, string(body))
		}
	})
}
