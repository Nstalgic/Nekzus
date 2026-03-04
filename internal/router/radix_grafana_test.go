package router

import (
	"github.com/nstalgic/nekzus/internal/types"
	"testing"
)

func TestRadixTree_GrafanaScenario(t *testing.T) {
	tree := NewRadixTree()

	route := types.Route{
		RouteID:  "route:grafana",
		AppID:    "grafana",
		PathBase: "/apps/grafana/",
		To:       "http://grafana:3000",
	}

	tree.Insert(route)

	tests := []struct {
		path        string
		shouldMatch bool
	}{
		{"/apps/grafana/", true},
		{"/apps/grafana/login", true},
		{"/apps/grafana/api/health", true},
		{"/apps/other/", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result, found := tree.Search(tt.path)
			if found != tt.shouldMatch {
				t.Errorf("Search(%q): got found=%v, want %v", tt.path, found, tt.shouldMatch)
			}
			if found && result.RouteID != route.RouteID {
				t.Errorf("Search(%q): got routeID=%s, want %s", tt.path, result.RouteID, route.RouteID)
			}
		})
	}
}
