package router

import (
	"fmt"
	"path"
	"strings"
	"sync"

	"github.com/nstalgic/nekzus/internal/types"
)

// NormalizePath cleans and normalizes a URL path
// Handles path traversal (/../), double slashes, and ensures leading slash
func NormalizePath(p string) string {
	if p == "" {
		return "/"
	}
	// Remember if path had trailing slash
	hadTrailingSlash := strings.HasSuffix(p, "/")

	// path.Clean handles /../, /./, and // normalization
	cleaned := path.Clean(p)

	// Ensure leading slash
	if !strings.HasPrefix(cleaned, "/") {
		cleaned = "/" + cleaned
	}

	// Restore trailing slash if original path had it (and it's not just "/")
	if hadTrailingSlash && cleaned != "/" {
		cleaned = cleaned + "/"
	}

	return cleaned
}

// radixNode represents a node in the radix tree
type radixNode struct {
	// prefix is the path segment for this node
	prefix string
	// route is the route data (nil if this is an intermediate node)
	route *types.Route
	// children are the child nodes
	children []*radixNode
}

// RadixTree is a radix tree (compressed trie) for efficient route matching
// Thread-safe for concurrent read/write access
type RadixTree struct {
	mu   sync.RWMutex
	root *radixNode
	size int
}

// NewRadixTree creates a new empty radix tree
func NewRadixTree() *RadixTree {
	return &RadixTree{
		root: &radixNode{
			prefix:   "",
			children: make([]*radixNode, 0),
		},
		size: 0,
	}
}

// Insert adds or updates a route in the tree
func (t *RadixTree) Insert(route types.Route) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// If updating existing route, delete old path first
	if existingRoute, found := t.findByIDLocked(route.RouteID); found {
		if existingRoute.PathBase != route.PathBase {
			t.deleteByID(t.root, route.RouteID)
		}
	}

	t.insert(t.root, route.PathBase, &route)
}

// InsertWithValidation adds a route with path collision detection
func (t *RadixTree) InsertWithValidation(route types.Route) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Check for existing route with same path but different ID
	if existing := t.findByPathLocked(route.PathBase); existing != nil {
		if existing.RouteID != route.RouteID {
			return fmt.Errorf("path %q already used by route %q", route.PathBase, existing.RouteID)
		}
	}

	// If updating existing route, delete old path first
	if existingRoute, found := t.findByIDLocked(route.RouteID); found {
		if existingRoute.PathBase != route.PathBase {
			t.deleteByID(t.root, route.RouteID)
		}
	}

	t.insert(t.root, route.PathBase, &route)
	return nil
}

// findByPathLocked finds a route by exact path match (caller must hold lock)
func (t *RadixTree) findByPathLocked(path string) *types.Route {
	return t.findByPathHelper(t.root, path)
}

// findByPathHelper recursively searches for a route by exact path
func (t *RadixTree) findByPathHelper(node *radixNode, path string) *types.Route {
	if path == "" {
		return node.route
	}

	for _, child := range node.children {
		if strings.HasPrefix(path, child.prefix) {
			remainder := path[len(child.prefix):]
			if result := t.findByPathHelper(child, remainder); result != nil {
				return result
			}
		}
	}

	return nil
}

// insert is the recursive helper for Insert
func (t *RadixTree) insert(node *radixNode, path string, route *types.Route) {
	// If path is empty, store route at this node
	if path == "" {
		if node.route == nil {
			t.size++
		}
		node.route = route
		return
	}

	// Find child with matching prefix
	for _, child := range node.children {
		// Calculate common prefix length
		commonLen := commonPrefixLen(path, child.prefix)

		if commonLen > 0 {
			// Case 1: Path exactly matches child prefix
			if commonLen == len(child.prefix) && commonLen == len(path) {
				if child.route == nil {
					t.size++
				}
				child.route = route
				return
			}

			// Case 2: Path matches child prefix, continue down the tree
			if commonLen == len(child.prefix) {
				t.insert(child, path[commonLen:], route)
				return
			}

			// Case 3: Partial match, need to split the node
			// Create new intermediate node with common prefix
			newChild := &radixNode{
				prefix:   child.prefix[commonLen:],
				route:    child.route,
				children: child.children,
			}

			// Update current child to have only common prefix
			child.prefix = child.prefix[:commonLen]
			child.route = nil
			child.children = []*radixNode{newChild}

			// If there's remaining path, create another child
			if commonLen < len(path) {
				remainingPath := path[commonLen:]
				newRoute := &radixNode{
					prefix:   remainingPath,
					route:    route,
					children: make([]*radixNode, 0),
				}
				child.children = append(child.children, newRoute)
				t.size++
			} else {
				// Path ends at the split point
				child.route = route
				t.size++
			}
			return
		}
	}

	// No matching child found, create new child
	newChild := &radixNode{
		prefix:   path,
		route:    route,
		children: make([]*radixNode, 0),
	}
	node.children = append(node.children, newChild)
	t.size++
}

// Search finds the longest matching route for the given path
func (t *RadixTree) Search(path string) (types.Route, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.search(t.root, path)
}

// search is the recursive helper for Search
func (t *RadixTree) search(node *radixNode, path string) (types.Route, bool) {
	var bestMatch *types.Route

	// Check if current node has a route
	// A route matches if:
	// 1. Path is empty (exact match)
	// 2. Path continues with '/' (proper prefix boundary)
	// 3. Current node's route pathBase ends with '/' (prefix route that accepts sub-paths)
	if node.route != nil {
		if path == "" || (len(path) > 0 && path[0] == '/') {
			bestMatch = node.route
		} else if strings.HasSuffix(node.route.PathBase, "/") {
			// Routes ending with '/' are prefix routes that match any sub-path
			bestMatch = node.route
		}
	}

	// If path is empty, return current best match
	if path == "" {
		if bestMatch != nil {
			return *bestMatch, true
		}
		return types.Route{}, false
	}

	// Search children for better matches
	for _, child := range node.children {
		if strings.HasPrefix(path, child.prefix) {
			remainder := path[len(child.prefix):]

			// Recursively search this child
			if match, found := t.search(child, remainder); found {
				// Update best match if this is longer
				if bestMatch == nil || len(match.PathBase) > len(bestMatch.PathBase) {
					bestMatch = &match
				}
			}
		}
	}

	if bestMatch != nil {
		return *bestMatch, true
	}
	return types.Route{}, false
}

// Delete removes a route from the tree by RouteID
func (t *RadixTree) Delete(routeID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.deleteByID(t.root, routeID)
}

// deleteByID recursively searches for and deletes a route by ID
func (t *RadixTree) deleteByID(node *radixNode, routeID string) bool {
	// Check current node
	if node.route != nil && node.route.RouteID == routeID {
		node.route = nil
		t.size--
		return true
	}

	// Search children
	for _, child := range node.children {
		if t.deleteByID(child, routeID) {
			return true
		}
	}

	return false
}

// findByID finds a route by RouteID (helper for Insert updates)
func (t *RadixTree) findByID(routeID string) (*types.Route, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.findByIDHelper(t.root, routeID)
}

// findByIDLocked finds a route by RouteID (caller must hold lock)
func (t *RadixTree) findByIDLocked(routeID string) (*types.Route, bool) {
	return t.findByIDHelper(t.root, routeID)
}

// findByIDHelper recursively searches for a route by ID
func (t *RadixTree) findByIDHelper(node *radixNode, routeID string) (*types.Route, bool) {
	if node.route != nil && node.route.RouteID == routeID {
		return node.route, true
	}

	for _, child := range node.children {
		if route, found := t.findByIDHelper(child, routeID); found {
			return route, true
		}
	}

	return nil, false
}

// Size returns the number of routes in the tree
func (t *RadixTree) Size() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.size
}

// commonPrefixLen returns the length of the common prefix between two strings
func commonPrefixLen(a, b string) int {
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}

	for i := 0; i < minLen; i++ {
		if a[i] != b[i] {
			return i
		}
	}

	return minLen
}
