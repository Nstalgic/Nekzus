package storage

import (
	"time"

	"github.com/nstalgic/nekzus/internal/types"
)

// DeviceRepository defines operations for managing device data.
// This interface enables dependency injection and easier testing with mock implementations.
type DeviceRepository interface {
	// SaveDevice creates or updates a device record
	SaveDevice(deviceID, deviceName, platform, platformVersion string, scopes []string) error

	// GetDevice retrieves a device by ID
	GetDevice(deviceID string) (*DeviceInfo, error)

	// ListDevices returns all registered devices
	ListDevices() ([]DeviceInfo, error)

	// DeleteDevice removes a device record
	DeleteDevice(deviceID string) error

	// UpdateDeviceLastSeen updates the last seen timestamp for a device
	UpdateDeviceLastSeen(deviceID string) error

	// GetDeviceRequestsToday returns the number of requests made by a device today
	GetDeviceRequestsToday(deviceID string) (int, error)
}

// AppRepository defines operations for managing application data.
type AppRepository interface {
	// SaveApp creates or updates an app record
	SaveApp(app types.App) error

	// GetApp retrieves an app by ID
	GetApp(id string) (*types.App, error)

	// ListApps returns all registered apps
	ListApps() ([]types.App, error)

	// DeleteApp removes an app record
	DeleteApp(id string) error
}

// RouteRepository defines operations for managing route data.
type RouteRepository interface {
	// SaveRoute creates or updates a route record
	SaveRoute(route types.Route) error

	// GetRoute retrieves a route by ID
	GetRoute(routeID string) (*types.Route, error)

	// ListRoutes returns all registered routes
	ListRoutes() ([]types.Route, error)

	// DeleteRoute removes a route record
	DeleteRoute(routeID string) error
}

// ProposalRepository defines operations for managing discovery proposals.
type ProposalRepository interface {
	// SaveProposal creates or updates a discovery proposal
	SaveProposal(proposal types.Proposal) error

	// GetProposal retrieves a proposal by ID
	GetProposal(id string) (*types.Proposal, error)

	// ListProposals returns all pending proposals
	ListProposals() ([]types.Proposal, error)

	// DeleteProposal removes a proposal record
	DeleteProposal(id string) error

	// CleanupStaleProposals removes proposals older than the specified duration
	CleanupStaleProposals(olderThan time.Duration) error

	// ClearProposals removes all proposals (useful for testing)
	ClearProposals() error
}

// Ensure Store implements all repository interfaces at compile time
var _ DeviceRepository = (*Store)(nil)
var _ AppRepository = (*Store)(nil)
var _ RouteRepository = (*Store)(nil)
var _ ProposalRepository = (*Store)(nil)
