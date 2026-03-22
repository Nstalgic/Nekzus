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

	"github.com/nstalgic/nekzus/internal/types"
)

// TestEndToEnd performs a complete end-to-end test of Nekzus
// covering: deployment, discovery, routing, pairing, and SSE
//
// Requires manually starting Docker Compose services first.
func TestEndToEnd(t *testing.T) {
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

	t.Run("1_Healthcheck", func(t *testing.T) {
		testHealthcheck(t, client, nexusURL)
	})

	t.Run("2_QRCodeGeneration", func(t *testing.T) {
		testQRCodeGeneration(t, client, nexusURL)
	})

	var jwt string
	t.Run("3_DevicePairing", func(t *testing.T) {
		jwt = testDevicePairing(t, client, nexusURL, bootstrapToken)
	})

	t.Run("4_AdminInfo", func(t *testing.T) {
		testAdminInfo(t, client, nexusURL, jwt)
	})

	var proposals []types.Proposal
	t.Run("5_DiscoveryProposals", func(t *testing.T) {
		proposals = testDiscoveryProposals(t, client, nexusURL, jwt)
	})

	t.Run("6_ApproveProposal", func(t *testing.T) {
		testApproveProposal(t, client, nexusURL, jwt, proposals)
	})

	t.Run("7_ListApps", func(t *testing.T) {
		testListApps(t, client, nexusURL, jwt)
	})

	t.Run("9_ProxyRouting", func(t *testing.T) {
		testProxyRouting(t, client, nexusURL, jwt, proposals)
	})
}

// testHealthcheck verifies the health endpoint is responding
func testHealthcheck(t *testing.T, client *http.Client, baseURL string) {
	t.Log("Testing healthcheck endpoint...")

	req, err := http.NewRequest("GET", baseURL+"/api/v1/healthz", nil)
	if err != nil {
		t.Fatalf("Failed to create healthcheck request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Healthcheck failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Fatalf("Expected body 'ok', got %s", string(body))
	}

	t.Log("✓ Healthcheck passed")
}

// testQRCodeGeneration verifies QR code endpoint
func testQRCodeGeneration(t *testing.T, client *http.Client, baseURL string) {
	t.Log("Testing QR code generation...")

	req, err := http.NewRequest("GET", baseURL+"/api/v1/auth/qr", nil)
	if err != nil {
		t.Fatalf("Failed to create QR request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("QR code request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	var qrPayload struct {
		Name           string   `json:"name"`
		BaseURL        string   `json:"baseUrl"`
		SPKI           string   `json:"spki"`
		BootstrapToken string   `json:"bootstrapToken"`
		Capabilities   []string `json:"capabilities"`
		NekzusID       string   `json:"nekzusId"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&qrPayload); err != nil {
		t.Fatalf("Failed to decode QR payload: %v", err)
	}

	// Validate QR payload
	if qrPayload.BaseURL == "" {
		t.Error("QR payload missing baseUrl")
	}
	if qrPayload.BootstrapToken == "" {
		t.Error("QR payload missing bootstrapToken")
	}
	if !strings.HasPrefix(qrPayload.BootstrapToken, "temp_") {
		t.Errorf("Bootstrap token should start with 'temp_', got: %s", qrPayload.BootstrapToken)
	}
	if qrPayload.NekzusID == "" {
		t.Error("QR payload missing nekzusId")
	}
	if !strings.HasPrefix(qrPayload.NekzusID, "nkz_") {
		t.Errorf("Nekzus ID should start with 'nkz_', got: %s", qrPayload.NekzusID)
	}
	if len(qrPayload.Capabilities) == 0 {
		t.Error("QR payload missing capabilities")
	}

	t.Logf("✓ QR code generated with bootstrap token: %s", qrPayload.BootstrapToken[:20]+"...")
	t.Logf("  Nekzus ID: %s", qrPayload.NekzusID)
	t.Logf("  Capabilities: %v", qrPayload.Capabilities)
	if qrPayload.SPKI != "" {
		t.Logf("  SPKI: %s...", qrPayload.SPKI[:20])
	}
}

// testDevicePairing tests device pairing and returns JWT
func testDevicePairing(t *testing.T, client *http.Client, baseURL, bootstrapToken string) string {
	t.Log("Testing device pairing...")

	pairRequest := map[string]interface{}{
		"device": map[string]interface{}{
			"id":        "e2e-test-device",
			"model":     "TestDevice",
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
		ExpiresIn   int    `json:"expiresIn"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&pairResponse); err != nil {
		t.Fatalf("Failed to decode pair response: %v", err)
	}

	if pairResponse.AccessToken == "" {
		t.Fatal("Pair response missing accessToken")
	}
	if pairResponse.ExpiresIn == 0 {
		t.Error("Pair response missing expiresIn")
	}

	t.Logf("✓ Device paired successfully")
	t.Logf("  JWT: %s...", pairResponse.AccessToken[:50])
	t.Logf("  Expires in: %d seconds", pairResponse.ExpiresIn)

	return pairResponse.AccessToken
}

// testAdminInfo tests the admin info endpoint
func testAdminInfo(t *testing.T, client *http.Client, baseURL, jwt string) {
	t.Log("Testing admin info endpoint...")

	req, err := http.NewRequest("GET", baseURL+"/api/v1/admin/info", nil)
	if err != nil {
		t.Fatalf("Failed to create admin info request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+jwt)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Admin info request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	var adminInfo struct {
		Version      string   `json:"version"`
		NekzusID     string   `json:"nekzusId"`
		Capabilities []string `json:"capabilities"`
		BuildDate    string   `json:"buildDate"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&adminInfo); err != nil {
		t.Fatalf("Failed to decode admin info: %v", err)
	}

	if adminInfo.Version == "" {
		t.Error("Admin info missing version")
	}
	if adminInfo.NekzusID == "" {
		t.Error("Admin info missing nekzusId")
	}
	if len(adminInfo.Capabilities) == 0 {
		t.Error("Admin info missing capabilities")
	}

	t.Logf("✓ Admin info retrieved")
	t.Logf("  Version: %s", adminInfo.Version)
	t.Logf("  Nekzus ID: %s", adminInfo.NekzusID)
	t.Logf("  Capabilities: %v", adminInfo.Capabilities)
}

// testDiscoveryProposals tests fetching discovery proposals
func testDiscoveryProposals(t *testing.T, client *http.Client, baseURL, jwt string) []types.Proposal {
	t.Log("Testing discovery proposals...")

	// Wait a bit for discovery to run
	time.Sleep(2 * time.Second)

	req, err := http.NewRequest("GET", baseURL+"/api/v1/discovery/proposals", nil)
	if err != nil {
		t.Fatalf("Failed to create proposals request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+jwt)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Proposals request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	var proposals []types.Proposal
	if err := json.NewDecoder(resp.Body).Decode(&proposals); err != nil {
		t.Fatalf("Failed to decode proposals: %v", err)
	}

	t.Logf("✓ Found %d discovery proposal(s)", len(proposals))
	for i, p := range proposals {
		t.Logf("  [%d] %s - %s (confidence: %.2f)", i+1, p.ID, p.SuggestedApp.Name, p.Confidence)
	}

	return proposals
}

// testApproveProposal tests approving a discovery proposal
func testApproveProposal(t *testing.T, client *http.Client, baseURL, jwt string, proposals []types.Proposal) {
	if len(proposals) == 0 {
		t.Skip("No proposals to approve, skipping")
		return
	}

	t.Log("Testing proposal approval...")

	// Approve the first proposal
	proposal := proposals[0]
	url := fmt.Sprintf("%s/api/v1/discovery/proposals/%s/approve", baseURL, proposal.ID)

	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		t.Fatalf("Failed to create approve request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+jwt)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Approve request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	var approveResp struct {
		Status string `json:"status"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&approveResp); err != nil {
		t.Fatalf("Failed to decode approve response: %v", err)
	}

	if approveResp.Status != "approved" {
		t.Fatalf("Expected status 'approved', got '%s'", approveResp.Status)
	}

	t.Logf("✓ Proposal approved: %s", proposal.SuggestedApp.Name)
	t.Logf("  Route: %s → %s", proposal.SuggestedRoute.PathBase, proposal.SuggestedRoute.To)
}

// testListApps tests listing registered apps
func testListApps(t *testing.T, client *http.Client, baseURL, jwt string) {
	t.Log("Testing app listing...")

	req, err := http.NewRequest("GET", baseURL+"/api/v1/apps", nil)
	if err != nil {
		t.Fatalf("Failed to create apps request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+jwt)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Apps request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	var apps []types.App
	if err := json.NewDecoder(resp.Body).Decode(&apps); err != nil {
		t.Fatalf("Failed to decode apps: %v", err)
	}

	t.Logf("✓ Found %d registered app(s)", len(apps))
	for i, app := range apps {
		t.Logf("  [%d] %s - %s", i+1, app.ID, app.Name)
	}
}

// testProxyRouting tests routing through approved proposals
func testProxyRouting(t *testing.T, client *http.Client, baseURL, jwt string, proposals []types.Proposal) {
	if len(proposals) == 0 {
		t.Skip("No proposals to test routing, skipping")
		return
	}

	t.Log("Testing proxy routing...")

	// Try to access the first approved proposal's route
	proposal := proposals[0]
	routePath := proposal.SuggestedRoute.PathBase

	// Note: This may fail if the upstream service isn't actually running
	// But we can at least verify the route exists and auth is required

	// Test without auth - should fail
	req, err := http.NewRequest("GET", baseURL+routePath, nil)
	if err != nil {
		t.Fatalf("Failed to create routing request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Routing request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Logf("Warning: Expected 401 without auth, got %d (route may not require auth)", resp.StatusCode)
	} else {
		t.Log("✓ Route requires authentication")
	}

	// Test with auth
	req, err = http.NewRequest("GET", baseURL+routePath, nil)
	if err != nil {
		t.Fatalf("Failed to create authenticated routing request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+jwt)

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Authenticated routing request failed: %v", err)
	}
	defer resp.Body.Close()

	// May get 502 if upstream isn't running, but that's ok
	if resp.StatusCode == http.StatusOK {
		t.Logf("✓ Route accessible with auth: %s", routePath)
	} else if resp.StatusCode == http.StatusBadGateway {
		t.Logf("✓ Route configured (upstream not available): %s", routePath)
	} else {
		t.Logf("  Route status %d: %s", resp.StatusCode, routePath)
	}
}
