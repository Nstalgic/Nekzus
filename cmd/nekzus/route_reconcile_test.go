package main

import (
	"testing"

	"github.com/nstalgic/nekzus/internal/types"
)

// TestReconcileRouteTarget_UpdatesHostWhenContainerIPChanged verifies the core
// fix: after a Docker/NAS restart the container IP may change. Discovery sees
// the same container (matched by ContainerID) with a new IP. The existing
// route's `To` field must be updated to the new host so the proxy and health
// checker stop talking to the stale IP.
func TestReconcileRouteTarget_UpdatesHostWhenContainerIPChanged(t *testing.T) {
	app := newTestApplication(t)

	const appID = "sonarr"
	existing := types.Route{
		RouteID:     "route:sonarr",
		AppID:       appID,
		PathBase:    "/apps/sonarr/",
		To:          "http://172.18.0.5:8989",
		ContainerID: "abc123def456",
	}
	if err := app.managers.Router.UpsertRoute(existing); err != nil {
		t.Fatalf("failed to seed route: %v", err)
	}

	proposal := &types.Proposal{
		ID:             "proposal_docker_http_sonarr_8989",
		Source:         "docker",
		DetectedScheme: "http",
		DetectedHost:   "172.18.0.7",
		DetectedPort:   8989,
		SuggestedApp:   types.App{ID: appID, Name: "Sonarr"},
		SuggestedRoute: types.Route{
			RouteID:     "route:sonarr",
			AppID:       appID,
			To:          "http://172.18.0.7:8989",
			ContainerID: "abc123def456",
		},
	}

	app.ReconcileRouteTarget(proposal)

	got, ok := app.managers.Router.GetRouteByAppID(appID)
	if !ok {
		t.Fatal("route disappeared after reconcile")
	}
	if got.To != "http://172.18.0.7:8989" {
		t.Errorf("route.To not updated: got %q, want %q", got.To, "http://172.18.0.7:8989")
	}
}

// TestReconcileRouteTarget_PreservesUserPort verifies that if the user has
// configured a non-default port on an existing route, reconciliation keeps the
// user's port and only swaps the host. The proposal's port is ignored.
func TestReconcileRouteTarget_PreservesUserPort(t *testing.T) {
	app := newTestApplication(t)

	const appID = "sonarr"
	existing := types.Route{
		RouteID:     "route:sonarr",
		AppID:       appID,
		PathBase:    "/apps/sonarr/",
		To:          "http://172.18.0.5:9090", // user picked non-default port
		ContainerID: "abc123def456",
	}
	if err := app.managers.Router.UpsertRoute(existing); err != nil {
		t.Fatalf("failed to seed route: %v", err)
	}

	proposal := &types.Proposal{
		SuggestedApp: types.App{ID: appID},
		SuggestedRoute: types.Route{
			AppID:       appID,
			To:          "http://172.18.0.7:8989", // default port
			ContainerID: "abc123def456",
		},
	}

	app.ReconcileRouteTarget(proposal)

	got, _ := app.managers.Router.GetRouteByAppID(appID)
	if got.To != "http://172.18.0.7:9090" {
		t.Errorf("port not preserved: got %q, want %q", got.To, "http://172.18.0.7:9090")
	}
}

// TestReconcileRouteTarget_DoesNotUpdateWhenContainerIDDiffers guards against
// accidentally clobbering a route when the proposal belongs to a different
// container that happens to share the same app ID.
func TestReconcileRouteTarget_DoesNotUpdateWhenContainerIDDiffers(t *testing.T) {
	app := newTestApplication(t)

	const appID = "sonarr"
	existing := types.Route{
		RouteID:     "route:sonarr",
		AppID:       appID,
		PathBase:    "/apps/sonarr/",
		To:          "http://172.18.0.5:8989",
		ContainerID: "aaaaaaaaaaaa",
	}
	if err := app.managers.Router.UpsertRoute(existing); err != nil {
		t.Fatalf("failed to seed route: %v", err)
	}

	proposal := &types.Proposal{
		SuggestedApp: types.App{ID: appID},
		SuggestedRoute: types.Route{
			AppID:       appID,
			To:          "http://172.18.0.7:8989",
			ContainerID: "bbbbbbbbbbbb",
		},
	}

	app.ReconcileRouteTarget(proposal)

	got, _ := app.managers.Router.GetRouteByAppID(appID)
	if got.To != "http://172.18.0.5:8989" {
		t.Errorf("route should not change when ContainerIDs differ: got %q", got.To)
	}
}

// TestReconcileRouteTarget_SkipsWhenRouteHasNoContainerID leaves
// manually-configured routes (no ContainerID) untouched. Only discovery-owned
// routes are candidates for auto-reconciliation.
func TestReconcileRouteTarget_SkipsWhenRouteHasNoContainerID(t *testing.T) {
	app := newTestApplication(t)

	const appID = "sonarr"
	existing := types.Route{
		RouteID:  "route:sonarr",
		AppID:    appID,
		PathBase: "/apps/sonarr/",
		To:       "http://manual-host:8989",
	}
	if err := app.managers.Router.UpsertRoute(existing); err != nil {
		t.Fatalf("failed to seed route: %v", err)
	}

	proposal := &types.Proposal{
		SuggestedApp: types.App{ID: appID},
		SuggestedRoute: types.Route{
			AppID:       appID,
			To:          "http://172.18.0.7:8989",
			ContainerID: "abc123def456",
		},
	}

	app.ReconcileRouteTarget(proposal)

	got, _ := app.managers.Router.GetRouteByAppID(appID)
	if got.To != "http://manual-host:8989" {
		t.Errorf("manual route should not be changed: got %q", got.To)
	}
}

// TestReconcileRouteTarget_NoChangeWhenHostMatches ensures the common path
// (container IP unchanged, discovery just re-confirming) does not churn the
// route or trigger storage writes.
func TestReconcileRouteTarget_NoChangeWhenHostMatches(t *testing.T) {
	app := newTestApplication(t)

	const appID = "sonarr"
	existing := types.Route{
		RouteID:     "route:sonarr",
		AppID:       appID,
		PathBase:    "/apps/sonarr/",
		To:          "http://172.18.0.5:8989",
		ContainerID: "abc123def456",
	}
	if err := app.managers.Router.UpsertRoute(existing); err != nil {
		t.Fatalf("failed to seed route: %v", err)
	}

	proposal := &types.Proposal{
		SuggestedApp: types.App{ID: appID},
		SuggestedRoute: types.Route{
			AppID:       appID,
			To:          "http://172.18.0.5:8989",
			ContainerID: "abc123def456",
		},
	}

	app.ReconcileRouteTarget(proposal)

	got, _ := app.managers.Router.GetRouteByAppID(appID)
	if got.To != "http://172.18.0.5:8989" {
		t.Errorf("unchanged route mutated: got %q", got.To)
	}
}
