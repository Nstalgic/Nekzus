package federation

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/nstalgic/nekzus/internal/storage"
	"github.com/nstalgic/nekzus/internal/types"
)

var syncerLog = slog.With("package", "federation")

// CatalogSyncer manages service catalog synchronization across federated peers
type CatalogSyncer struct {
	mu            sync.RWMutex
	localPeerID   string
	storage       *storage.Store
	vectorClock   VectorClock
	broadcastFunc func(msg []byte)                      // Callback to broadcast gossip messages
	eventCallback func(eventType string, data interface{}) // Callback to publish events
}

// NewCatalogSyncer creates a new catalog syncer
func NewCatalogSyncer(localPeerID string, store *storage.Store) *CatalogSyncer {
	return &CatalogSyncer{
		localPeerID: localPeerID,
		storage:     store,
		vectorClock: NewVectorClock(),
	}
}

// SetBroadcastFunc sets the callback function for broadcasting gossip messages
func (cs *CatalogSyncer) SetBroadcastFunc(fn func(msg []byte)) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.broadcastFunc = fn
}

// SetEventCallback sets the callback function for publishing events
func (cs *CatalogSyncer) SetEventCallback(fn func(eventType string, data interface{})) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.eventCallback = fn
}

// OnLocalServiceAdded is called when a new service is discovered locally
func (cs *CatalogSyncer) OnLocalServiceAdded(app *types.App) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	// Increment vector clock for local peer
	cs.vectorClock.Increment(cs.localPeerID)

	// Create federated service record
	federatedService := &FederatedService{
		ServiceID:    app.ID,
		OriginPeerID: cs.localPeerID,
		App:          app,
		VectorClock:  cs.vectorClock.Copy(),
		Tombstone:    false,
		LastSeen:     time.Now(),
	}

	// Save to storage
	if cs.storage != nil {
		if err := saveFederatedServiceToStorage(cs.storage, federatedService); err != nil {
			syncerLog.Error("failed to save local service to federated catalog",
				"error", err)
		}
	}

	// Broadcast update to peers
	return cs.broadcastServiceUpdate(federatedService)
}

// OnLocalServiceUpdated is called when a local service is updated
func (cs *CatalogSyncer) OnLocalServiceUpdated(app *types.App) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	// Increment vector clock
	cs.vectorClock.Increment(cs.localPeerID)

	// Update federated service record
	federatedService := &FederatedService{
		ServiceID:    app.ID,
		OriginPeerID: cs.localPeerID,
		App:          app,
		VectorClock:  cs.vectorClock.Copy(),
		Tombstone:    false,
		LastSeen:     time.Now(),
	}

	// Save to storage
	if cs.storage != nil {
		if err := saveFederatedServiceToStorage(cs.storage, federatedService); err != nil {
			syncerLog.Error("failed to update federated service",
				"error", err)
		}
	}

	// Broadcast update
	return cs.broadcastServiceUpdate(federatedService)
}

// OnLocalServiceDeleted is called when a local service is removed
func (cs *CatalogSyncer) OnLocalServiceDeleted(serviceID string) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	// Increment vector clock
	cs.vectorClock.Increment(cs.localPeerID)

	// Get existing service to preserve metadata
	var existingService *FederatedService
	if cs.storage != nil {
		serviceData, _ := cs.storage.GetFederatedService(serviceID)
		if serviceData != nil {
			var err error
			existingService, err = federatedServiceFromStorage(serviceData)
			if err != nil {
				syncerLog.Error("failed to convert stored service",
					"error", err)
			}
		}
	}

	// Create tombstone
	tombstone := &FederatedService{
		ServiceID:    serviceID,
		OriginPeerID: cs.localPeerID,
		VectorClock:  cs.vectorClock.Copy(),
		Tombstone:    true,
		LastSeen:     time.Now(),
	}

	// Preserve app metadata if available
	if existingService != nil {
		tombstone.App = existingService.App
	}

	// Save tombstone to storage
	if cs.storage != nil {
		if err := saveFederatedServiceToStorage(cs.storage, tombstone); err != nil {
			syncerLog.Error("failed to save tombstone",
				"error", err)
		}
	}

	// Broadcast deletion
	return cs.broadcastServiceDelete(tombstone)
}

// OnRemoteServiceUpdate is called when receiving a service update from a remote peer
func (cs *CatalogSyncer) OnRemoteServiceUpdate(remoteService *FederatedService) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	// Get existing service from storage
	var existingService *FederatedService
	if cs.storage != nil {
		serviceData, _ := cs.storage.GetFederatedService(remoteService.ServiceID)
		if serviceData != nil {
			var err error
			existingService, err = federatedServiceFromStorage(serviceData)
			if err != nil {
				syncerLog.Error("failed to convert stored service",
					"error", err)
			}
		}
	}

	// Resolve conflict
	resolved := ResolveConflict(existingService, remoteService, cs.localPeerID)

	// If remote won or this is new, save it
	if resolved == remoteService {
		if cs.storage != nil {
			if err := saveFederatedServiceToStorage(cs.storage, remoteService); err != nil {
				return fmt.Errorf("failed to save remote service: %w", err)
			}
		}

		// Merge remote vector clock into local
		cs.vectorClock.Merge(remoteService.VectorClock)

		syncerLog.Info("merged remote service",
			"service_id", remoteService.ServiceID,
			"origin_peer_id", remoteService.OriginPeerID)

		// Publish event
		if cs.eventCallback != nil {
			eventType := "federation_service_added"
			if remoteService.Tombstone {
				eventType = "federation_service_removed"
			}
			cs.eventCallback(eventType, remoteService)
		}
	}

	return nil
}

// GetLocalVectorClock returns a copy of the local vector clock
func (cs *CatalogSyncer) GetLocalVectorClock() VectorClock {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.vectorClock.Copy()
}

// GetAllServices returns all federated services including tombstones
// Used for anti-entropy state serialization
func (cs *CatalogSyncer) GetAllServices() ([]*FederatedService, error) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	if cs.storage == nil {
		return []*FederatedService{}, nil
	}

	serviceDataList, err := cs.storage.ListFederatedServices()
	if err != nil {
		return nil, err
	}

	// Convert storage data to FederatedService
	services := make([]*FederatedService, 0, len(serviceDataList))
	for _, data := range serviceDataList {
		service, err := federatedServiceFromStorage(data)
		if err != nil {
			syncerLog.Error("failed to convert stored service",
				"error", err)
			continue
		}
		services = append(services, service)
	}

	return services, nil
}

// MergeRemoteServices processes multiple remote services atomically
// Used during anti-entropy sync to merge entire catalog state
func (cs *CatalogSyncer) MergeRemoteServices(services []*FederatedService) error {
	if len(services) == 0 {
		return nil
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()

	for _, remoteService := range services {
		// Skip services that originated from us
		if remoteService.OriginPeerID == cs.localPeerID {
			continue
		}

		// Get existing service from storage
		var existingService *FederatedService
		if cs.storage != nil {
			serviceData, _ := cs.storage.GetFederatedService(remoteService.ServiceID)
			if serviceData != nil {
				var err error
				existingService, err = federatedServiceFromStorage(serviceData)
				if err != nil {
					syncerLog.Error("failed to convert stored service",
						"error", err)
				}
			}
		}

		// Resolve conflict
		resolved := ResolveConflict(existingService, remoteService, cs.localPeerID)

		// If remote won or this is new, save it
		if resolved == remoteService {
			if cs.storage != nil {
				if err := saveFederatedServiceToStorage(cs.storage, remoteService); err != nil {
					syncerLog.Error("failed to save merged service",
						"service_id", remoteService.ServiceID,
						"error", err)
					continue
				}
			}

			// Merge remote vector clock into local
			cs.vectorClock.Merge(remoteService.VectorClock)

			// Publish event
			if cs.eventCallback != nil {
				eventType := "federation_service_added"
				if remoteService.Tombstone {
					eventType = "federation_service_removed"
				}
				cs.eventCallback(eventType, remoteService)
			}
		}
	}

	return nil
}

// GetFederatedCatalog returns all active federated services
func (cs *CatalogSyncer) GetFederatedCatalog() ([]*FederatedService, error) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	if cs.storage == nil {
		return []*FederatedService{}, nil
	}

	serviceDataList, err := cs.storage.ListActiveFederatedServices()
	if err != nil {
		return nil, err
	}

	// Convert storage data to FederatedService
	services := make([]*FederatedService, 0, len(serviceDataList))
	for _, data := range serviceDataList {
		service, err := federatedServiceFromStorage(data)
		if err != nil {
			syncerLog.Error("failed to convert stored service",
				"error", err)
			continue
		}
		services = append(services, service)
	}

	return services, nil
}

// broadcastServiceUpdate broadcasts a service update via gossip
func (cs *CatalogSyncer) broadcastServiceUpdate(service *FederatedService) error {
	if cs.broadcastFunc == nil {
		return nil // No broadcast function set
	}

	// Create gossip message
	msg := AppUpdateMsg{
		ServiceID:    service.ServiceID,
		OriginPeerID: service.OriginPeerID,
		App:          service.App,
		VectorClock:  service.VectorClock,
		Tombstone:    service.Tombstone,
	}

	// Serialize to JSON
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal gossip message: %w", err)
	}

	// Broadcast
	cs.broadcastFunc(data)

	return nil
}

// broadcastServiceDelete broadcasts a service deletion via gossip
func (cs *CatalogSyncer) broadcastServiceDelete(tombstone *FederatedService) error {
	if cs.broadcastFunc == nil {
		return nil
	}

	// Create delete message
	msg := AppDeleteMsg{
		ServiceID:    tombstone.ServiceID,
		OriginPeerID: tombstone.OriginPeerID,
		VectorClock:  tombstone.VectorClock,
	}

	// Serialize to JSON
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal delete message: %w", err)
	}

	// Broadcast
	cs.broadcastFunc(data)

	return nil
}

// Storage Conversion Helpers

// saveFederatedServiceToStorage converts a FederatedService to storage format and saves it
func saveFederatedServiceToStorage(store *storage.Store, fs *FederatedService) error {
	// Serialize app data
	appJSON, err := json.Marshal(fs.App)
	if err != nil {
		return fmt.Errorf("failed to marshal app: %w", err)
	}

	// Serialize vector clock
	clockJSON, err := json.Marshal(fs.VectorClock)
	if err != nil {
		return fmt.Errorf("failed to marshal vector clock: %w", err)
	}

	// Use default confidence if not set
	confidence := fs.Confidence
	if confidence == 0 {
		confidence = 1.0
	}

	return store.SaveFederatedService(
		fs.ServiceID,
		fs.OriginPeerID,
		string(appJSON),
		confidence,
		fs.LastSeen,
		fs.Tombstone,
		string(clockJSON),
	)
}

// federatedServiceFromStorage converts storage data to FederatedService
func federatedServiceFromStorage(data *storage.FederatedServiceData) (*FederatedService, error) {
	// Deserialize app
	app := &types.App{}
	if err := json.Unmarshal([]byte(data.AppData), app); err != nil {
		return nil, fmt.Errorf("failed to unmarshal app: %w", err)
	}

	// Deserialize vector clock
	var vectorClock VectorClock
	if err := json.Unmarshal([]byte(data.VectorClock), &vectorClock); err != nil {
		return nil, fmt.Errorf("failed to unmarshal vector clock: %w", err)
	}

	return &FederatedService{
		ServiceID:    data.ServiceID,
		OriginPeerID: data.OriginPeerID,
		App:          app,
		Confidence:   data.Confidence,
		VectorClock:  vectorClock,
		Tombstone:    data.Tombstone,
		LastSeen:     data.LastSeen,
	}, nil
}
