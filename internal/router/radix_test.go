package router

import (
	"fmt"
	"testing"

	"github.com/nstalgic/nekzus/internal/types"
)

func TestRadixTree_Insert(t *testing.T) {
	tests := []struct {
		name   string
		routes []types.Route
		want   int
	}{
		{
			name: "insert single route",
			routes: []types.Route{
				{RouteID: "r1", PathBase: "/api", To: "http://localhost:8080"},
			},
			want: 1,
		},
		{
			name: "insert multiple routes",
			routes: []types.Route{
				{RouteID: "r1", PathBase: "/api", To: "http://localhost:8080"},
				{RouteID: "r2", PathBase: "/api/users", To: "http://localhost:8081"},
				{RouteID: "r3", PathBase: "/web", To: "http://localhost:8082"},
			},
			want: 3,
		},
		{
			name: "insert with common prefix",
			routes: []types.Route{
				{RouteID: "r1", PathBase: "/api/users", To: "http://localhost:8080"},
				{RouteID: "r2", PathBase: "/api/posts", To: "http://localhost:8081"},
			},
			want: 2,
		},
		{
			name: "insert nested paths",
			routes: []types.Route{
				{RouteID: "r1", PathBase: "/a", To: "http://localhost:8080"},
				{RouteID: "r2", PathBase: "/a/b", To: "http://localhost:8081"},
				{RouteID: "r3", PathBase: "/a/b/c", To: "http://localhost:8082"},
			},
			want: 3,
		},
		{
			name:   "empty tree",
			routes: []types.Route{},
			want:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tree := NewRadixTree()

			for _, route := range tt.routes {
				tree.Insert(route)
			}

			count := tree.Size()
			if count != tt.want {
				t.Errorf("Size() = %d, want %d", count, tt.want)
			}
		})
	}
}

func TestRadixTree_Search(t *testing.T) {
	tests := []struct {
		name       string
		routes     []types.Route
		searchPath string
		wantFound  bool
		wantPrefix string
	}{
		{
			name: "exact match",
			routes: []types.Route{
				{RouteID: "r1", PathBase: "/api", To: "http://localhost:8080"},
			},
			searchPath: "/api",
			wantFound:  true,
			wantPrefix: "/api",
		},
		{
			name: "prefix match",
			routes: []types.Route{
				{RouteID: "r1", PathBase: "/api", To: "http://localhost:8080"},
			},
			searchPath: "/api/users",
			wantFound:  true,
			wantPrefix: "/api",
		},
		{
			name: "longest prefix match",
			routes: []types.Route{
				{RouteID: "r1", PathBase: "/api", To: "http://localhost:8080"},
				{RouteID: "r2", PathBase: "/api/users", To: "http://localhost:8081"},
				{RouteID: "r3", PathBase: "/api/users/admin", To: "http://localhost:8082"},
			},
			searchPath: "/api/users/admin/settings",
			wantFound:  true,
			wantPrefix: "/api/users/admin",
		},
		{
			name: "no match",
			routes: []types.Route{
				{RouteID: "r1", PathBase: "/api", To: "http://localhost:8080"},
			},
			searchPath: "/web",
			wantFound:  false,
		},
		{
			name: "partial path no match",
			routes: []types.Route{
				{RouteID: "r1", PathBase: "/api/users", To: "http://localhost:8080"},
			},
			searchPath: "/api",
			wantFound:  false,
		},
		{
			name:       "empty tree",
			routes:     []types.Route{},
			searchPath: "/any",
			wantFound:  false,
		},
		{
			name: "multiple routes different prefixes",
			routes: []types.Route{
				{RouteID: "r1", PathBase: "/api", To: "http://localhost:8080"},
				{RouteID: "r2", PathBase: "/web", To: "http://localhost:8081"},
				{RouteID: "r3", PathBase: "/mobile", To: "http://localhost:8082"},
			},
			searchPath: "/web/home",
			wantFound:  true,
			wantPrefix: "/web",
		},
		{
			name: "similar paths - should match correct one",
			routes: []types.Route{
				{RouteID: "r1", PathBase: "/api", To: "http://localhost:8080"},
				{RouteID: "r2", PathBase: "/app", To: "http://localhost:8081"},
			},
			searchPath: "/app/settings",
			wantFound:  true,
			wantPrefix: "/app",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tree := NewRadixTree()

			for _, route := range tt.routes {
				tree.Insert(route)
			}

			got, found := tree.Search(tt.searchPath)

			if found != tt.wantFound {
				t.Errorf("Search(%q) found = %v, want %v", tt.searchPath, found, tt.wantFound)
			}

			if found && got.PathBase != tt.wantPrefix {
				t.Errorf("Search(%q) prefix = %q, want %q", tt.searchPath, got.PathBase, tt.wantPrefix)
			}
		})
	}
}

func TestRadixTree_Delete(t *testing.T) {
	tests := []struct {
		name       string
		routes     []types.Route
		deleteID   string
		searchPath string
		wantFound  bool
		wantSize   int
	}{
		{
			name: "delete existing route",
			routes: []types.Route{
				{RouteID: "r1", PathBase: "/api", To: "http://localhost:8080"},
				{RouteID: "r2", PathBase: "/web", To: "http://localhost:8081"},
			},
			deleteID:   "r1",
			searchPath: "/api",
			wantFound:  false,
			wantSize:   1,
		},
		{
			name: "delete non-existent route",
			routes: []types.Route{
				{RouteID: "r1", PathBase: "/api", To: "http://localhost:8080"},
			},
			deleteID:   "nonexistent",
			searchPath: "/api",
			wantFound:  true,
			wantSize:   1,
		},
		{
			name: "delete from nested paths",
			routes: []types.Route{
				{RouteID: "r1", PathBase: "/a", To: "http://localhost:8080"},
				{RouteID: "r2", PathBase: "/a/b", To: "http://localhost:8081"},
				{RouteID: "r3", PathBase: "/a/b/c", To: "http://localhost:8082"},
			},
			deleteID:   "r2",
			searchPath: "/a/b",
			wantFound:  true, // Should match /a since it's still there
			wantSize:   2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tree := NewRadixTree()

			for _, route := range tt.routes {
				tree.Insert(route)
			}

			tree.Delete(tt.deleteID)

			_, found := tree.Search(tt.searchPath)
			if found != tt.wantFound {
				t.Errorf("After delete, Search(%q) found = %v, want %v", tt.searchPath, found, tt.wantFound)
			}

			size := tree.Size()
			if size != tt.wantSize {
				t.Errorf("After delete, Size() = %d, want %d", size, tt.wantSize)
			}
		})
	}
}

func TestRadixTree_Update(t *testing.T) {
	t.Run("update existing route", func(t *testing.T) {
		tree := NewRadixTree()

		// Insert initial route
		route1 := types.Route{
			RouteID:  "r1",
			PathBase: "/api",
			To:       "http://localhost:8080",
		}
		tree.Insert(route1)

		// Update the same route with different target
		route2 := types.Route{
			RouteID:  "r1",
			PathBase: "/api",
			To:       "http://localhost:9090",
		}
		tree.Insert(route2)

		// Should still have only 1 route
		if size := tree.Size(); size != 1 {
			t.Errorf("Size() = %d, want 1", size)
		}

		// Should return updated route
		got, found := tree.Search("/api")
		if !found {
			t.Fatal("Route not found")
		}
		if got.To != "http://localhost:9090" {
			t.Errorf("To = %q, want %q", got.To, "http://localhost:9090")
		}
	})

	t.Run("update route with different path", func(t *testing.T) {
		tree := NewRadixTree()

		// Insert initial route
		route1 := types.Route{
			RouteID:  "r1",
			PathBase: "/api",
			To:       "http://localhost:8080",
		}
		tree.Insert(route1)

		// Update same RouteID with different path
		route2 := types.Route{
			RouteID:  "r1",
			PathBase: "/web",
			To:       "http://localhost:8080",
		}
		tree.Insert(route2)

		// Old path should not match
		_, found := tree.Search("/api")
		if found {
			t.Error("Old path should not be found")
		}

		// New path should match
		got, found := tree.Search("/web")
		if !found {
			t.Fatal("New path not found")
		}
		if got.RouteID != "r1" {
			t.Errorf("RouteID = %q, want r1", got.RouteID)
		}
	})
}

func BenchmarkRadixTree_Search(b *testing.B) {
	tree := NewRadixTree()

	// Insert 100 routes
	routes := make([]types.Route, 100)
	for i := 0; i < 100; i++ {
		routes[i] = types.Route{
			RouteID:  "route-" + string(rune('0'+i%10)),
			PathBase: "/api/v" + string(rune('0'+i/10)) + "/resource" + string(rune('0'+i%10)),
			To:       "http://localhost:8080",
		}
		tree.Insert(routes[i])
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		tree.Search("/api/v5/resource3/action/detail")
	}
}

func BenchmarkLinearSearch_Search(b *testing.B) {
	// Simulate linear search (current implementation)
	routes := make([]types.Route, 100)
	for i := 0; i < 100; i++ {
		routes[i] = types.Route{
			RouteID:  "route-" + string(rune('0'+i%10)),
			PathBase: "/api/v" + string(rune('0'+i/10)) + "/resource" + string(rune('0'+i%10)),
			To:       "http://localhost:8080",
		}
	}

	searchPath := "/api/v5/resource3/action/detail"

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Linear search implementation
		var best types.Route
		bestLen := -1

		for _, rt := range routes {
			if len(searchPath) >= len(rt.PathBase) && searchPath[:len(rt.PathBase)] == rt.PathBase {
				if l := len(rt.PathBase); l > bestLen {
					best, bestLen = rt, l
				}
			}
		}
		_ = best
	}
}

// TestRadixTree_ConcurrentAccess tests thread safety with race detector
func TestRadixTree_ConcurrentAccess(t *testing.T) {
	tree := NewRadixTree()

	// Insert initial routes
	for i := 0; i < 10; i++ {
		tree.Insert(types.Route{
			RouteID:  fmt.Sprintf("init-%d", i),
			PathBase: fmt.Sprintf("/api/v1/resource%d", i),
			To:       "http://localhost:8080",
		})
	}

	done := make(chan bool)

	// 5 concurrent readers
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				tree.Search("/api/v1/resource5")
			}
			done <- true
		}()
	}

	// 5 concurrent writers
	for i := 0; i < 5; i++ {
		go func(id int) {
			for j := 0; j < 50; j++ {
				tree.Insert(types.Route{
					RouteID:  fmt.Sprintf("r%d-%d", id, j),
					PathBase: fmt.Sprintf("/concurrent/%d/%d", id, j),
					To:       "http://localhost:8080",
				})
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Tree should still be functional
	if tree.Size() < 10 {
		t.Errorf("Tree should have at least 10 routes, got %d", tree.Size())
	}
}

// TestRadixTree_ConcurrentDelete tests concurrent delete operations
func TestRadixTree_ConcurrentDelete(t *testing.T) {
	tree := NewRadixTree()

	// Insert routes
	for i := 0; i < 100; i++ {
		tree.Insert(types.Route{
			RouteID:  fmt.Sprintf("route-%d", i),
			PathBase: fmt.Sprintf("/api/route%d", i),
			To:       "http://localhost:8080",
		})
	}

	done := make(chan bool)

	// Concurrent deletes
	for i := 0; i < 5; i++ {
		go func(start int) {
			for j := 0; j < 20; j++ {
				tree.Delete(fmt.Sprintf("route-%d", start*20+j))
			}
			done <- true
		}(i)
	}

	// Concurrent searches during deletes
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				tree.Search("/api/route50")
			}
			done <- true
		}()
	}

	// Wait for all
	for i := 0; i < 10; i++ {
		<-done
	}
}

// TestRadixTree_PathBoundary tests path boundary validation
func TestRadixTree_PathBoundary(t *testing.T) {
	tree := NewRadixTree()

	// Insert route
	tree.Insert(types.Route{
		RouteID:  "r1",
		PathBase: "/api/user",
		To:       "http://localhost:8080",
	})

	tests := []struct {
		name       string
		searchPath string
		wantFound  bool
	}{
		{
			name:       "exact match",
			searchPath: "/api/user",
			wantFound:  true,
		},
		{
			name:       "with trailing path",
			searchPath: "/api/user/123",
			wantFound:  true,
		},
		{
			name:       "similar but different path - should NOT match",
			searchPath: "/api/users",
			wantFound:  false, // /api/user should not match /api/users
		},
		{
			name:       "similar with more path - should NOT match",
			searchPath: "/api/users/123",
			wantFound:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, found := tree.Search(tt.searchPath)
			if found != tt.wantFound {
				t.Errorf("Search(%q) found = %v, want %v", tt.searchPath, found, tt.wantFound)
			}
		})
	}
}

// TestRadixTree_OrphanNodeCleanup tests that deleted routes clean up orphan nodes
func TestRadixTree_OrphanNodeCleanup(t *testing.T) {
	tree := NewRadixTree()

	// Insert routes with nested paths
	tree.Insert(types.Route{
		RouteID:  "r1",
		PathBase: "/api/v1/users",
		To:       "http://localhost:8080",
	})
	tree.Insert(types.Route{
		RouteID:  "r2",
		PathBase: "/api/v1/posts",
		To:       "http://localhost:8081",
	})

	// Delete one route
	tree.Delete("r1")

	// Size should be 1
	if tree.Size() != 1 {
		t.Errorf("Size() = %d, want 1", tree.Size())
	}

	// Delete the other route
	tree.Delete("r2")

	// Size should be 0
	if tree.Size() != 0 {
		t.Errorf("Size() = %d, want 0", tree.Size())
	}

	// Tree should be able to insert new routes
	tree.Insert(types.Route{
		RouteID:  "r3",
		PathBase: "/new/path",
		To:       "http://localhost:8082",
	})

	if tree.Size() != 1 {
		t.Errorf("After reinsertion, Size() = %d, want 1", tree.Size())
	}
}

// TestRadixTree_PathCollision tests that duplicate paths return error
func TestRadixTree_PathCollision(t *testing.T) {
	tree := NewRadixTree()

	// Insert first route
	err := tree.InsertWithValidation(types.Route{
		RouteID:  "r1",
		PathBase: "/api/users",
		To:       "http://localhost:8080",
	})
	if err != nil {
		t.Fatalf("First insert failed: %v", err)
	}

	// Try to insert duplicate path with different RouteID
	err = tree.InsertWithValidation(types.Route{
		RouteID:  "r2",
		PathBase: "/api/users",
		To:       "http://localhost:8081",
	})
	if err == nil {
		t.Error("Expected error for duplicate path, got nil")
	}

	// Updating same route should succeed
	err = tree.InsertWithValidation(types.Route{
		RouteID:  "r1",
		PathBase: "/api/users",
		To:       "http://localhost:9090",
	})
	if err != nil {
		t.Errorf("Update same route failed: %v", err)
	}
}

// TestNormalizePath tests path normalization
func TestNormalizePath(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "normal path",
			input: "/api/users",
			want:  "/api/users",
		},
		{
			name:  "path with dot segments",
			input: "/api/../admin",
			want:  "/admin",
		},
		{
			name:  "path with double dot",
			input: "/api/v1/../v2/users",
			want:  "/api/v2/users",
		},
		{
			name:  "path with single dot",
			input: "/api/./users",
			want:  "/api/users",
		},
		{
			name:  "path with double slashes",
			input: "/api//users",
			want:  "/api/users",
		},
		{
			name:  "root path",
			input: "/",
			want:  "/",
		},
		{
			name:  "empty path",
			input: "",
			want:  "/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizePath(tt.input)
			if got != tt.want {
				t.Errorf("NormalizePath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
