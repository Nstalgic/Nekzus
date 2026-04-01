package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/nstalgic/nekzus/internal/notifications"
	"github.com/nstalgic/nekzus/internal/types"
)

var storageLog = slog.With("package", "storage")

// Store provides persistent storage for Nexus data.
type Store struct {
	db *sql.DB
}

// Config holds storage configuration.
type Config struct {
	DatabasePath string
}

// NewStore creates and initializes a new storage instance.
func NewStore(cfg Config) (*Store, error) {
	db, err := sql.Open("sqlite3", cfg.DatabasePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool for SQLite
	// With WAL mode, SQLite supports multiple readers + 1 writer concurrently
	// Using 10 connections allows better throughput under load (9 concurrent reads + 1 write)
	db.SetMaxOpenConns(10)                  // Allow up to 10 concurrent connections
	db.SetMaxIdleConns(5)                   // Keep 5 idle connections for reuse
	db.SetConnMaxLifetime(30 * time.Minute) // Rotate connections periodically
	db.SetConnMaxIdleTime(10 * time.Minute) // Close idle connections after 10 minutes

	// Enable WAL mode for better concurrency
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	// Set busy timeout to 5 seconds
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		return nil, fmt.Errorf("failed to set busy timeout: %w", err)
	}

	store := &Store{db: db}

	// Run migrations
	if err := store.migrate(); err != nil {
		db.Close() // Clean up on migration failure
		return nil, fmt.Errorf("migration failed: %w", err)
	}

	return store, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// DB returns the underlying database connection for health checks
func (s *Store) DB() *sql.DB {
	return s.db
}

// migrate runs database migrations to create/update schema.
func (s *Store) migrate() error {
	migrations := []string{
		// Apps table
		`CREATE TABLE IF NOT EXISTS apps (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			icon TEXT,
			tags TEXT, -- JSON array
			endpoints TEXT, -- JSON object
			discovery_meta TEXT, -- JSON object for discovery metadata
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		// Routes table
		`CREATE TABLE IF NOT EXISTS routes (
			route_id TEXT PRIMARY KEY,
			app_id TEXT NOT NULL,
			path_base TEXT NOT NULL,
			target_url TEXT NOT NULL,
			container_id TEXT DEFAULT '',
			scopes TEXT, -- JSON array
			websocket INTEGER DEFAULT 0,
			strip_prefix INTEGER DEFAULT 1,
			rewrite_html INTEGER DEFAULT 1,
			health_check_path TEXT DEFAULT '',
			health_check_timeout TEXT DEFAULT '',
			health_check_interval TEXT DEFAULT '',
			expected_status_codes TEXT DEFAULT '[]', -- JSON array
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (app_id) REFERENCES apps(id) ON DELETE CASCADE
		)`,
		// Proposals table
		`CREATE TABLE IF NOT EXISTS proposals (
			id TEXT PRIMARY KEY,
			source TEXT NOT NULL,
			detected_scheme TEXT NOT NULL,
			detected_host TEXT NOT NULL,
			detected_port INTEGER NOT NULL,
			available_ports TEXT, -- JSON array of port options
			confidence REAL NOT NULL,
			suggested_app TEXT NOT NULL, -- JSON object
			suggested_route TEXT NOT NULL, -- JSON object
			tags TEXT, -- JSON array
			security_notes TEXT, -- JSON array
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_seen DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		// Device sessions table
		`CREATE TABLE IF NOT EXISTS devices (
			device_id TEXT PRIMARY KEY,
			device_name TEXT NOT NULL,
			platform TEXT,
			platform_version TEXT,
			scopes TEXT NOT NULL, -- JSON array
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_seen DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		// Service health table
		`CREATE TABLE IF NOT EXISTS service_health (
			app_id TEXT PRIMARY KEY,
			status TEXT NOT NULL DEFAULT 'unknown', -- healthy, unhealthy, unknown
			last_check_time DATETIME,
			last_success_time DATETIME,
			consecutive_failures INTEGER DEFAULT 0,
			error_message TEXT,
			FOREIGN KEY (app_id) REFERENCES apps(id) ON DELETE CASCADE
		)`,
		// Activity events table
		`CREATE TABLE IF NOT EXISTS activity_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			event_id TEXT NOT NULL,
			event_type TEXT NOT NULL,
			icon TEXT NOT NULL,
			icon_class TEXT DEFAULT '',
			message TEXT NOT NULL,
			details TEXT DEFAULT '',
			timestamp INTEGER NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		// Create indexes
		`CREATE INDEX IF NOT EXISTS idx_routes_app_id ON routes(app_id)`,
		`CREATE INDEX IF NOT EXISTS idx_routes_path_base ON routes(path_base)`,
		`CREATE INDEX IF NOT EXISTS idx_proposals_source ON proposals(source)`,
		`CREATE INDEX IF NOT EXISTS idx_proposals_last_seen ON proposals(last_seen)`,
		`CREATE INDEX IF NOT EXISTS idx_devices_last_seen ON devices(last_seen)`,
		`CREATE INDEX IF NOT EXISTS idx_service_health_status ON service_health(status)`,
		`CREATE INDEX IF NOT EXISTS idx_service_health_last_check ON service_health(last_check_time)`,
		`CREATE INDEX IF NOT EXISTS idx_activity_events_timestamp ON activity_events(timestamp DESC)`,
		// Certificates table
		`CREATE TABLE IF NOT EXISTS certificates (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			domain TEXT NOT NULL UNIQUE,
			certificate_pem BLOB NOT NULL,
			private_key_pem BLOB NOT NULL,
			issuer TEXT NOT NULL,
			not_before DATETIME NOT NULL,
			not_after DATETIME NOT NULL,
			subject_alternative_names TEXT,
			fingerprint_sha256 TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			renewal_attempt_count INTEGER DEFAULT 0,
			last_renewal_attempt DATETIME,
			last_renewal_error TEXT
		)`,
		// Certificate history table
		`CREATE TABLE IF NOT EXISTS certificate_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			domain TEXT NOT NULL,
			action TEXT NOT NULL,
			issuer TEXT NOT NULL,
			fingerprint_sha256 TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			created_by TEXT,
			metadata TEXT
		)`,
		// Certificate indexes
		`CREATE INDEX IF NOT EXISTS idx_cert_domain ON certificates(domain)`,
		`CREATE INDEX IF NOT EXISTS idx_cert_expiry ON certificates(not_after)`,
		`CREATE INDEX IF NOT EXISTS idx_cert_history_domain ON certificate_history(domain)`,
		`CREATE INDEX IF NOT EXISTS idx_cert_history_action ON certificate_history(action)`,
		// API keys table
		`CREATE TABLE IF NOT EXISTS api_keys (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			key_hash TEXT NOT NULL UNIQUE,
			prefix TEXT NOT NULL,
			scopes TEXT NOT NULL,
			expires_at DATETIME,
			last_used_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			created_by TEXT,
			revoked_at DATETIME
		)`,
		// API keys indexes
		`CREATE INDEX IF NOT EXISTS idx_apikeys_hash ON api_keys(key_hash)`,
		`CREATE INDEX IF NOT EXISTS idx_apikeys_created ON api_keys(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_apikeys_revoked ON api_keys(revoked_at)`,
		// Notifications table
		`CREATE TABLE IF NOT EXISTS notifications (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			device_id TEXT NOT NULL,
			type TEXT NOT NULL,
			payload TEXT NOT NULL,
			status TEXT DEFAULT 'pending',
			retry_count INTEGER DEFAULT 0,
			max_retries INTEGER DEFAULT 3,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			expires_at DATETIME,
			delivered_at DATETIME,
			last_attempt_at DATETIME,
			error_message TEXT,
			FOREIGN KEY (device_id) REFERENCES devices(device_id) ON DELETE CASCADE
		)`,
		// Notifications indexes
		`CREATE INDEX IF NOT EXISTS idx_notifications_device_status ON notifications(device_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_notifications_expires ON notifications(expires_at)`,
		`CREATE INDEX IF NOT EXISTS idx_notifications_status ON notifications(status)`,
		// Request logs table
		`CREATE TABLE IF NOT EXISTS request_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			device_id TEXT NOT NULL,
			date DATE NOT NULL,
			request_count INTEGER DEFAULT 0,
			bytes_transferred INTEGER DEFAULT 0,
			avg_latency_ms FLOAT DEFAULT 0,
			error_count INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(device_id, date),
			FOREIGN KEY (device_id) REFERENCES devices(device_id) ON DELETE CASCADE
		)`,
		// Request logs indexes
		`CREATE INDEX IF NOT EXISTS idx_request_logs_device_date ON request_logs(device_id, date)`,
		`CREATE INDEX IF NOT EXISTS idx_request_logs_date ON request_logs(date)`,
		// Audit logs table
		`CREATE TABLE IF NOT EXISTS audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp DATETIME NOT NULL,
			action TEXT NOT NULL,
			actor_id TEXT NOT NULL,
			actor_ip TEXT NOT NULL,
			target_id TEXT NOT NULL,
			details TEXT,
			success INTEGER NOT NULL,
			error TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		// Audit logs indexes
		`CREATE INDEX IF NOT EXISTS idx_audit_logs_timestamp ON audit_logs(timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_logs_actor ON audit_logs(actor_id)`,
		// Toolbox deployments table
		`CREATE TABLE IF NOT EXISTS toolbox_deployments (
			id TEXT PRIMARY KEY,
			service_template_id TEXT NOT NULL,
			service_name TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			container_id TEXT,
			container_name TEXT,
			project_name TEXT,
			network_names TEXT,
			volume_names TEXT,
			env_vars TEXT,
			custom_image TEXT,
			custom_port INTEGER DEFAULT 0,
			route_id TEXT,
			error_message TEXT,
			deployed_at DATETIME,
			deployed_by TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		// Toolbox deployments indexes
		`CREATE INDEX IF NOT EXISTS idx_toolbox_deployments_status ON toolbox_deployments(status)`,
		`CREATE INDEX IF NOT EXISTS idx_toolbox_deployments_template ON toolbox_deployments(service_template_id)`,
		`CREATE INDEX IF NOT EXISTS idx_toolbox_deployments_deployed_by ON toolbox_deployments(deployed_by)`,
		// Federation peers table
		`CREATE TABLE IF NOT EXISTS federation_peers (
			peer_id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			api_address TEXT NOT NULL,
			gossip_address TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'online',
			last_seen DATETIME,
			metadata TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		// Federation peers indexes
		`CREATE INDEX IF NOT EXISTS idx_federation_peers_status ON federation_peers(status)`,
		`CREATE INDEX IF NOT EXISTS idx_federation_peers_last_seen ON federation_peers(last_seen)`,
		// Federation services table
		`CREATE TABLE IF NOT EXISTS federation_services (
			service_id TEXT PRIMARY KEY,
			origin_peer_id TEXT NOT NULL,
			app_data TEXT NOT NULL,
			confidence REAL,
			last_seen DATETIME,
			tombstone INTEGER DEFAULT 0,
			vector_clock TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (origin_peer_id) REFERENCES federation_peers(peer_id) ON DELETE CASCADE
		)`,
		// Federation services indexes
		`CREATE INDEX IF NOT EXISTS idx_federation_services_origin ON federation_services(origin_peer_id)`,
		`CREATE INDEX IF NOT EXISTS idx_federation_services_tombstone ON federation_services(tombstone)`,
		`CREATE INDEX IF NOT EXISTS idx_federation_services_last_seen ON federation_services(last_seen)`,
		// System secrets table (for auto-generated secrets like JWT key)
		`CREATE TABLE IF NOT EXISTS system_secrets (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		// Proxy session cookies table (for mobile webview session persistence)
		`CREATE TABLE IF NOT EXISTS proxy_session_cookies (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			device_id TEXT NOT NULL,
			app_id TEXT NOT NULL,
			cookie_name TEXT NOT NULL,
			cookie_value_encrypted BLOB NOT NULL,
			cookie_path TEXT DEFAULT '/',
			cookie_domain TEXT,
			expires_at DATETIME,
			secure INTEGER DEFAULT 0,
			http_only INTEGER DEFAULT 0,
			same_site TEXT DEFAULT 'Lax',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(device_id, app_id, cookie_name),
			FOREIGN KEY (device_id) REFERENCES devices(device_id) ON DELETE CASCADE
		)`,
		// Proxy session cookies indexes
		`CREATE INDEX IF NOT EXISTS idx_proxy_cookies_device_app ON proxy_session_cookies(device_id, app_id)`,
		`CREATE INDEX IF NOT EXISTS idx_proxy_cookies_expires ON proxy_session_cookies(expires_at)`,
		// Scripts table (metadata only - content is on filesystem)
		`CREATE TABLE IF NOT EXISTS scripts (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			category TEXT DEFAULT 'general',
			script_path TEXT NOT NULL,
			script_type TEXT NOT NULL,
			timeout_seconds INTEGER DEFAULT 300,
			parameters TEXT DEFAULT '[]',
			environment TEXT DEFAULT '{}',
			allowed_scopes TEXT DEFAULT '[]',
			dry_run_command TEXT,
			created_by TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		// Script executions table
		`CREATE TABLE IF NOT EXISTS script_executions (
			id TEXT PRIMARY KEY,
			script_id TEXT NOT NULL,
			workflow_id TEXT,
			workflow_ex_id TEXT,
			status TEXT NOT NULL DEFAULT 'pending',
			is_dry_run INTEGER DEFAULT 0,
			triggered_by TEXT NOT NULL,
			triggered_ip TEXT,
			parameters TEXT DEFAULT '{}',
			output TEXT,
			exit_code INTEGER,
			error_message TEXT,
			started_at DATETIME,
			completed_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (script_id) REFERENCES scripts(id) ON DELETE CASCADE
		)`,
		// Scripts indexes
		`CREATE INDEX IF NOT EXISTS idx_scripts_category ON scripts(category)`,
		`CREATE INDEX IF NOT EXISTS idx_script_executions_script_id ON script_executions(script_id)`,
		`CREATE INDEX IF NOT EXISTS idx_script_executions_status ON script_executions(status)`,
		`CREATE INDEX IF NOT EXISTS idx_script_executions_created ON script_executions(created_at DESC)`,
		// Workflows table
		`CREATE TABLE IF NOT EXISTS workflows (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			steps TEXT NOT NULL,
			allowed_scopes TEXT DEFAULT '[]',
			created_by TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		// Workflow executions table
		`CREATE TABLE IF NOT EXISTS workflow_executions (
			id TEXT PRIMARY KEY,
			workflow_id TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			current_step INTEGER DEFAULT 0,
			triggered_by TEXT NOT NULL,
			started_at DATETIME,
			completed_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (workflow_id) REFERENCES workflows(id) ON DELETE CASCADE
		)`,
		// Workflow indexes
		`CREATE INDEX IF NOT EXISTS idx_workflow_executions_workflow_id ON workflow_executions(workflow_id)`,
		`CREATE INDEX IF NOT EXISTS idx_workflow_executions_status ON workflow_executions(status)`,
		// Script schedules table
		`CREATE TABLE IF NOT EXISTS script_schedules (
			id TEXT PRIMARY KEY,
			script_id TEXT,
			workflow_id TEXT,
			cron_expression TEXT NOT NULL,
			parameters TEXT DEFAULT '{}',
			enabled INTEGER DEFAULT 1,
			last_run_at DATETIME,
			next_run_at DATETIME,
			created_by TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		// Script schedules indexes
		`CREATE INDEX IF NOT EXISTS idx_script_schedules_enabled ON script_schedules(enabled)`,
		`CREATE INDEX IF NOT EXISTS idx_script_schedules_next_run ON script_schedules(next_run_at) WHERE enabled = 1`,

		// WebSocket sessions table (MQTT-style session persistence)
		`CREATE TABLE IF NOT EXISTS ws_sessions (
			device_id TEXT PRIMARY KEY,
			subscriptions TEXT NOT NULL,
			last_will TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_seen DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (device_id) REFERENCES devices(device_id) ON DELETE CASCADE
		)`,
		// WebSocket retained messages table
		`CREATE TABLE IF NOT EXISTS ws_retained_messages (
			topic TEXT PRIMARY KEY,
			message TEXT NOT NULL,
			expires_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		// WebSocket pending messages table (QoS 1/2)
		`CREATE TABLE IF NOT EXISTS ws_pending_messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			message_id TEXT UNIQUE NOT NULL,
			device_id TEXT NOT NULL,
			topic TEXT NOT NULL,
			message TEXT NOT NULL,
			qos INTEGER NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			retry_count INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			expires_at DATETIME,
			FOREIGN KEY (device_id) REFERENCES devices(device_id) ON DELETE CASCADE
		)`,
		// WebSocket message deduplication table (QoS 2)
		`CREATE TABLE IF NOT EXISTS ws_message_dedup (
			message_id TEXT NOT NULL,
			device_id TEXT NOT NULL,
			received_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			expires_at DATETIME NOT NULL,
			PRIMARY KEY (message_id, device_id)
		)`,
		// WebSocket indexes
		`CREATE INDEX IF NOT EXISTS idx_ws_sessions_last_seen ON ws_sessions(last_seen)`,
		`CREATE INDEX IF NOT EXISTS idx_ws_retained_expires ON ws_retained_messages(expires_at)`,
		`CREATE INDEX IF NOT EXISTS idx_ws_pending_device_status ON ws_pending_messages(device_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_ws_pending_expires ON ws_pending_messages(expires_at)`,
		`CREATE INDEX IF NOT EXISTS idx_ws_dedup_expires ON ws_message_dedup(expires_at)`,
	}

	for _, migration := range migrations {
		if _, err := s.db.Exec(migration); err != nil {
			return fmt.Errorf("migration error: %w", err)
		}
	}

	return nil
}

// Apps Management

// SaveApp persists an app to the database.
func (s *Store) SaveApp(app types.App) error {
	tagsJSON, err := json.Marshal(app.Tags)
	if err != nil {
		return fmt.Errorf("failed to marshal tags: %w", err)
	}

	endpointsJSON, err := json.Marshal(app.Endpoints)
	if err != nil {
		return fmt.Errorf("failed to marshal endpoints: %w", err)
	}

	// Marshal discovery metadata (nil becomes "null" in JSON)
	var discoveryMetaJSON []byte
	if app.DiscoveryMeta != nil {
		discoveryMetaJSON, err = json.Marshal(app.DiscoveryMeta)
		if err != nil {
			return fmt.Errorf("failed to marshal discovery_meta: %w", err)
		}
	}

	query := `
		INSERT INTO apps (id, name, icon, tags, endpoints, discovery_meta, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			icon = excluded.icon,
			tags = excluded.tags,
			endpoints = excluded.endpoints,
			discovery_meta = excluded.discovery_meta,
			updated_at = CURRENT_TIMESTAMP
	`

	var discoveryMetaStr *string
	if discoveryMetaJSON != nil {
		s := string(discoveryMetaJSON)
		discoveryMetaStr = &s
	}

	_, err = s.db.Exec(query, app.ID, app.Name, app.Icon, string(tagsJSON), string(endpointsJSON), discoveryMetaStr)
	if err != nil {
		return fmt.Errorf("failed to save app: %w", err)
	}

	return nil
}

// GetApp retrieves an app by ID.
func (s *Store) GetApp(id string) (*types.App, error) {
	query := `SELECT id, name, icon, tags, endpoints, discovery_meta FROM apps WHERE id = ?`

	var app types.App
	var tagsJSON, endpointsJSON string
	var discoveryMetaJSON sql.NullString

	err := s.db.QueryRow(query, id).Scan(
		&app.ID,
		&app.Name,
		&app.Icon,
		&tagsJSON,
		&endpointsJSON,
		&discoveryMetaJSON,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get app: %w", err)
	}

	if err := json.Unmarshal([]byte(tagsJSON), &app.Tags); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
	}

	if err := json.Unmarshal([]byte(endpointsJSON), &app.Endpoints); err != nil {
		return nil, fmt.Errorf("failed to unmarshal endpoints: %w", err)
	}

	if discoveryMetaJSON.Valid && discoveryMetaJSON.String != "" {
		var meta types.DiscoveryMetadata
		if err := json.Unmarshal([]byte(discoveryMetaJSON.String), &meta); err != nil {
			return nil, fmt.Errorf("failed to unmarshal discovery_meta: %w", err)
		}
		app.DiscoveryMeta = &meta
	}

	return &app, nil
}

// ListApps retrieves all apps.
func (s *Store) ListApps() ([]types.App, error) {
	query := `SELECT id, name, icon, tags, endpoints, discovery_meta FROM apps ORDER BY name`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list apps: %w", err)
	}
	defer rows.Close()

	var apps []types.App
	for rows.Next() {
		var app types.App
		var tagsJSON, endpointsJSON string
		var discoveryMetaJSON sql.NullString

		if err := rows.Scan(&app.ID, &app.Name, &app.Icon, &tagsJSON, &endpointsJSON, &discoveryMetaJSON); err != nil {
			return nil, fmt.Errorf("failed to scan app: %w", err)
		}

		if err := json.Unmarshal([]byte(tagsJSON), &app.Tags); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
		}

		if err := json.Unmarshal([]byte(endpointsJSON), &app.Endpoints); err != nil {
			return nil, fmt.Errorf("failed to unmarshal endpoints: %w", err)
		}

		if discoveryMetaJSON.Valid && discoveryMetaJSON.String != "" {
			var meta types.DiscoveryMetadata
			if err := json.Unmarshal([]byte(discoveryMetaJSON.String), &meta); err != nil {
				return nil, fmt.Errorf("failed to unmarshal discovery_meta: %w", err)
			}
			app.DiscoveryMeta = &meta
		}

		apps = append(apps, app)
	}

	return apps, rows.Err()
}

// DeleteApp removes an app and its routes.
func (s *Store) DeleteApp(id string) error {
	query := `DELETE FROM apps WHERE id = ?`
	_, err := s.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete app: %w", err)
	}
	return nil
}

// Routes Management

// SaveRoute persists a route to the database.
func (s *Store) SaveRoute(route types.Route) error {
	scopesJSON, err := json.Marshal(route.Scopes)
	if err != nil {
		return fmt.Errorf("failed to marshal scopes: %w", err)
	}

	expectedStatusCodesJSON, err := json.Marshal(route.ExpectedStatusCodes)
	if err != nil {
		return fmt.Errorf("failed to marshal expected_status_codes: %w", err)
	}

	query := `
		INSERT INTO routes (route_id, app_id, path_base, target_url, container_id, scopes, websocket, strip_prefix, rewrite_html,
			health_check_path, health_check_timeout, health_check_interval, expected_status_codes, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(route_id) DO UPDATE SET
			app_id = excluded.app_id,
			path_base = excluded.path_base,
			target_url = excluded.target_url,
			container_id = excluded.container_id,
			scopes = excluded.scopes,
			websocket = excluded.websocket,
			strip_prefix = excluded.strip_prefix,
			rewrite_html = excluded.rewrite_html,
			health_check_path = excluded.health_check_path,
			health_check_timeout = excluded.health_check_timeout,
			health_check_interval = excluded.health_check_interval,
			expected_status_codes = excluded.expected_status_codes,
			updated_at = CURRENT_TIMESTAMP
	`

	websocket := 0
	if route.Websocket {
		websocket = 1
	}
	stripPrefix := 0
	if route.StripPrefix {
		stripPrefix = 1
	}
	rewriteHTML := 0
	if route.RewriteHTML {
		rewriteHTML = 1
	}

	_, err = s.db.Exec(query, route.RouteID, route.AppID, route.PathBase, route.To, route.ContainerID, string(scopesJSON),
		websocket, stripPrefix, rewriteHTML,
		route.HealthCheckPath, route.HealthCheckTimeout, route.HealthCheckInterval, string(expectedStatusCodesJSON))
	if err != nil {
		return fmt.Errorf("failed to save route: %w", err)
	}

	return nil
}

// GetRoute retrieves a route by ID.
func (s *Store) GetRoute(routeID string) (*types.Route, error) {
	query := `SELECT route_id, app_id, path_base, target_url, COALESCE(container_id, ''), scopes, websocket, strip_prefix, rewrite_html,
		COALESCE(health_check_path, ''), COALESCE(health_check_timeout, ''), COALESCE(health_check_interval, ''), COALESCE(expected_status_codes, '[]')
		FROM routes WHERE route_id = ?`

	var route types.Route
	var scopesJSON string
	var expectedStatusCodesJSON string
	var websocket, stripPrefix, rewriteHTML int

	err := s.db.QueryRow(query, routeID).Scan(
		&route.RouteID,
		&route.AppID,
		&route.PathBase,
		&route.To,
		&route.ContainerID,
		&scopesJSON,
		&websocket,
		&stripPrefix,
		&rewriteHTML,
		&route.HealthCheckPath,
		&route.HealthCheckTimeout,
		&route.HealthCheckInterval,
		&expectedStatusCodesJSON,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get route: %w", err)
	}

	if err := json.Unmarshal([]byte(scopesJSON), &route.Scopes); err != nil {
		return nil, fmt.Errorf("failed to unmarshal scopes: %w", err)
	}

	if expectedStatusCodesJSON != "" && expectedStatusCodesJSON != "[]" {
		if err := json.Unmarshal([]byte(expectedStatusCodesJSON), &route.ExpectedStatusCodes); err != nil {
			return nil, fmt.Errorf("failed to unmarshal expected_status_codes: %w", err)
		}
	}

	route.Websocket = websocket == 1
	route.StripPrefix = stripPrefix == 1
	route.RewriteHTML = rewriteHTML == 1

	return &route, nil
}

// ListRoutes retrieves all routes.
func (s *Store) ListRoutes() ([]types.Route, error) {
	query := `SELECT route_id, app_id, path_base, target_url, COALESCE(container_id, ''), scopes, websocket, strip_prefix, rewrite_html,
		COALESCE(health_check_path, ''), COALESCE(health_check_timeout, ''), COALESCE(health_check_interval, ''), COALESCE(expected_status_codes, '[]')
		FROM routes ORDER BY path_base`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list routes: %w", err)
	}
	defer rows.Close()

	var routes []types.Route
	for rows.Next() {
		var route types.Route
		var scopesJSON string
		var expectedStatusCodesJSON string
		var websocket, stripPrefix, rewriteHTML int

		if err := rows.Scan(&route.RouteID, &route.AppID, &route.PathBase, &route.To, &route.ContainerID, &scopesJSON, &websocket, &stripPrefix, &rewriteHTML,
			&route.HealthCheckPath, &route.HealthCheckTimeout, &route.HealthCheckInterval, &expectedStatusCodesJSON); err != nil {
			return nil, fmt.Errorf("failed to scan route: %w", err)
		}

		if err := json.Unmarshal([]byte(scopesJSON), &route.Scopes); err != nil {
			return nil, fmt.Errorf("failed to unmarshal scopes: %w", err)
		}

		if expectedStatusCodesJSON != "" && expectedStatusCodesJSON != "[]" {
			if err := json.Unmarshal([]byte(expectedStatusCodesJSON), &route.ExpectedStatusCodes); err != nil {
				return nil, fmt.Errorf("failed to unmarshal expected_status_codes: %w", err)
			}
		}

		route.Websocket = websocket == 1
		route.StripPrefix = stripPrefix == 1
		route.RewriteHTML = rewriteHTML == 1
		routes = append(routes, route)
	}

	return routes, rows.Err()
}

// DeleteRoute removes a route.
func (s *Store) DeleteRoute(routeID string) error {
	query := `DELETE FROM routes WHERE route_id = ?`
	_, err := s.db.Exec(query, routeID)
	if err != nil {
		return fmt.Errorf("failed to delete route: %w", err)
	}
	return nil
}

// Proposals Management

// SaveProposal persists a proposal to the database.
func (s *Store) SaveProposal(proposal types.Proposal) error {
	appJSON, err := json.Marshal(proposal.SuggestedApp)
	if err != nil {
		return fmt.Errorf("failed to marshal suggested app: %w", err)
	}

	routeJSON, err := json.Marshal(proposal.SuggestedRoute)
	if err != nil {
		return fmt.Errorf("failed to marshal suggested route: %w", err)
	}

	tagsJSON, err := json.Marshal(proposal.Tags)
	if err != nil {
		return fmt.Errorf("failed to marshal tags: %w", err)
	}

	notesJSON, err := json.Marshal(proposal.SecurityNotes)
	if err != nil {
		return fmt.Errorf("failed to marshal security notes: %w", err)
	}

	portsJSON, err := json.Marshal(proposal.AvailablePorts)
	if err != nil {
		return fmt.Errorf("failed to marshal available ports: %w", err)
	}

	query := `
		INSERT INTO proposals (
			id, source, detected_scheme, detected_host, detected_port, available_ports,
			confidence, suggested_app, suggested_route, tags, security_notes, last_seen
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			confidence = excluded.confidence,
			available_ports = excluded.available_ports,
			suggested_app = excluded.suggested_app,
			suggested_route = excluded.suggested_route,
			tags = excluded.tags,
			security_notes = excluded.security_notes,
			last_seen = CURRENT_TIMESTAMP
	`

	_, err = s.db.Exec(query,
		proposal.ID,
		proposal.Source,
		proposal.DetectedScheme,
		proposal.DetectedHost,
		proposal.DetectedPort,
		string(portsJSON),
		proposal.Confidence,
		string(appJSON),
		string(routeJSON),
		string(tagsJSON),
		string(notesJSON),
	)
	if err != nil {
		return fmt.Errorf("failed to save proposal: %w", err)
	}

	return nil
}

// GetProposal retrieves a proposal by ID.
func (s *Store) GetProposal(id string) (*types.Proposal, error) {
	query := `
		SELECT id, source, detected_scheme, detected_host, detected_port, available_ports,
			confidence, suggested_app, suggested_route, tags, security_notes, last_seen
		FROM proposals WHERE id = ?
	`

	var proposal types.Proposal
	var appJSON, routeJSON, tagsJSON, notesJSON, lastSeen string
	var portsJSON sql.NullString

	err := s.db.QueryRow(query, id).Scan(
		&proposal.ID,
		&proposal.Source,
		&proposal.DetectedScheme,
		&proposal.DetectedHost,
		&proposal.DetectedPort,
		&portsJSON,
		&proposal.Confidence,
		&appJSON,
		&routeJSON,
		&tagsJSON,
		&notesJSON,
		&lastSeen,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get proposal: %w", err)
	}

	if err := json.Unmarshal([]byte(appJSON), &proposal.SuggestedApp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal app: %w", err)
	}

	if err := json.Unmarshal([]byte(routeJSON), &proposal.SuggestedRoute); err != nil {
		return nil, fmt.Errorf("failed to unmarshal route: %w", err)
	}

	if err := json.Unmarshal([]byte(tagsJSON), &proposal.Tags); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
	}

	if err := json.Unmarshal([]byte(notesJSON), &proposal.SecurityNotes); err != nil {
		return nil, fmt.Errorf("failed to unmarshal security notes: %w", err)
	}

	if portsJSON.Valid && portsJSON.String != "" {
		if err := json.Unmarshal([]byte(portsJSON.String), &proposal.AvailablePorts); err != nil {
			return nil, fmt.Errorf("failed to unmarshal available ports: %w", err)
		}
	}

	proposal.LastSeen = lastSeen

	return &proposal, nil
}

// ListProposals retrieves all proposals.
func (s *Store) ListProposals() ([]types.Proposal, error) {
	query := `
		SELECT id, source, detected_scheme, detected_host, detected_port, available_ports,
			confidence, suggested_app, suggested_route, tags, security_notes, last_seen
		FROM proposals ORDER BY last_seen DESC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list proposals: %w", err)
	}
	defer rows.Close()

	var proposals []types.Proposal
	for rows.Next() {
		var proposal types.Proposal
		var appJSON, routeJSON, tagsJSON, notesJSON, lastSeen string
		var portsJSON sql.NullString

		if err := rows.Scan(
			&proposal.ID,
			&proposal.Source,
			&proposal.DetectedScheme,
			&proposal.DetectedHost,
			&proposal.DetectedPort,
			&portsJSON,
			&proposal.Confidence,
			&appJSON,
			&routeJSON,
			&tagsJSON,
			&notesJSON,
			&lastSeen,
		); err != nil {
			return nil, fmt.Errorf("failed to scan proposal: %w", err)
		}

		if err := json.Unmarshal([]byte(appJSON), &proposal.SuggestedApp); err != nil {
			return nil, fmt.Errorf("failed to unmarshal app: %w", err)
		}

		if err := json.Unmarshal([]byte(routeJSON), &proposal.SuggestedRoute); err != nil {
			return nil, fmt.Errorf("failed to unmarshal route: %w", err)
		}

		if err := json.Unmarshal([]byte(tagsJSON), &proposal.Tags); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
		}

		if err := json.Unmarshal([]byte(notesJSON), &proposal.SecurityNotes); err != nil {
			return nil, fmt.Errorf("failed to unmarshal security notes: %w", err)
		}

		if portsJSON.Valid && portsJSON.String != "" {
			if err := json.Unmarshal([]byte(portsJSON.String), &proposal.AvailablePorts); err != nil {
				return nil, fmt.Errorf("failed to unmarshal available ports: %w", err)
			}
		}

		proposal.LastSeen = lastSeen
		proposals = append(proposals, proposal)
	}

	return proposals, rows.Err()
}

// DeleteProposal removes a proposal.
func (s *Store) DeleteProposal(id string) error {
	query := `DELETE FROM proposals WHERE id = ?`
	_, err := s.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete proposal: %w", err)
	}
	return nil
}

// CleanupStaleProposals removes proposals not seen in the specified duration.
func (s *Store) CleanupStaleProposals(olderThan time.Duration) error {
	query := `DELETE FROM proposals WHERE datetime(last_seen) < datetime('now', ?)`
	duration := fmt.Sprintf("-%d seconds", int(olderThan.Seconds()))

	result, err := s.db.Exec(query, duration)
	if err != nil {
		return fmt.Errorf("failed to cleanup proposals: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows > 0 {
		// Log cleanup action
		_ = rows // Could log this
	}

	return nil
}

// ClearProposals removes all proposals from storage.
func (s *Store) ClearProposals() error {
	query := `DELETE FROM proposals`
	_, err := s.db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to clear proposals: %w", err)
	}
	return nil
}

// Device Sessions Management

// SaveDevice persists a device session.
func (s *Store) SaveDevice(deviceID, deviceName, platform, platformVersion string, scopes []string) error {
	scopesJSON, err := json.Marshal(scopes)
	if err != nil {
		return fmt.Errorf("failed to marshal scopes: %w", err)
	}

	query := `
		INSERT INTO devices (device_id, device_name, platform, platform_version, scopes, last_seen)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(device_id) DO UPDATE SET
			device_name = excluded.device_name,
			platform = excluded.platform,
			platform_version = excluded.platform_version,
			scopes = excluded.scopes,
			last_seen = CURRENT_TIMESTAMP
	`

	_, err = s.db.Exec(query, deviceID, deviceName, platform, platformVersion, string(scopesJSON))
	if err != nil {
		storageLog.Error("Failed to save device", "device_id", deviceID, "platform", platform, "error", err)
		return fmt.Errorf("failed to save device: %w", err)
	}

	storageLog.Info("Device saved", "device_id", deviceID, "device_name", deviceName, "platform", platform)
	return nil
}

// GetDevice retrieves a device by ID.
func (s *Store) GetDevice(deviceID string) (*DeviceInfo, error) {
	query := `SELECT device_id, device_name, platform, platform_version, scopes, created_at, last_seen FROM devices WHERE device_id = ?`

	var device DeviceInfo
	var scopesJSON string
	var platform, platformVersion sql.NullString

	err := s.db.QueryRow(query, deviceID).Scan(
		&device.ID,
		&device.Name,
		&platform,
		&platformVersion,
		&scopesJSON,
		&device.PairedAt,
		&device.LastSeen,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get device: %w", err)
	}

	// Handle nullable platform fields
	if platform.Valid {
		device.Platform = platform.String
	}
	if platformVersion.Valid {
		device.PlatformVersion = platformVersion.String
	}

	if err := json.Unmarshal([]byte(scopesJSON), &device.Scopes); err != nil {
		return nil, fmt.Errorf("failed to unmarshal scopes: %w", err)
	}

	// Compute device status based on last seen time
	device.ComputeStatus()

	// Query actual requests today
	count, err := s.GetDeviceRequestsToday(device.ID)
	if err != nil {
		// Log warning but don't fail the entire request
		device.RequestsToday = 0
	} else {
		device.RequestsToday = count
	}

	return &device, nil
}

// ListDevices retrieves all devices.
func (s *Store) ListDevices() ([]DeviceInfo, error) {
	today := time.Now().Format("2006-01-02")

	// LEFT JOIN with request_logs to get today's request count for all devices in one query
	query := `
		SELECT
			d.device_id, d.device_name, d.platform, d.platform_version,
			d.scopes, d.created_at, d.last_seen,
			COALESCE(r.request_count, 0) as requests_today
		FROM devices d
		LEFT JOIN request_logs r ON d.device_id = r.device_id AND r.date = ?
		ORDER BY d.last_seen DESC
	`

	rows, err := s.db.Query(query, today)
	if err != nil {
		return nil, fmt.Errorf("failed to list devices: %w", err)
	}
	defer rows.Close()

	// Initialize as empty slice to ensure JSON serialization returns [] instead of null
	devices := make([]DeviceInfo, 0)
	for rows.Next() {
		var device DeviceInfo
		var scopesJSON string
		var platform, platformVersion sql.NullString

		if err := rows.Scan(
			&device.ID, &device.Name, &platform, &platformVersion,
			&scopesJSON, &device.PairedAt, &device.LastSeen,
			&device.RequestsToday,
		); err != nil {
			return nil, fmt.Errorf("failed to scan device: %w", err)
		}

		// Handle nullable platform fields
		if platform.Valid {
			device.Platform = platform.String
		}
		if platformVersion.Valid {
			device.PlatformVersion = platformVersion.String
		}

		if err := json.Unmarshal([]byte(scopesJSON), &device.Scopes); err != nil {
			return nil, fmt.Errorf("failed to unmarshal scopes: %w", err)
		}

		// Compute device status based on last seen time
		device.ComputeStatus()

		devices = append(devices, device)
	}

	return devices, rows.Err()
}

// DeleteDevice removes a device session.
func (s *Store) DeleteDevice(deviceID string) error {
	query := `DELETE FROM devices WHERE device_id = ?`
	_, err := s.db.Exec(query, deviceID)
	if err != nil {
		storageLog.Error("Failed to delete device", "device_id", deviceID, "error", err)
		return fmt.Errorf("failed to delete device: %w", err)
	}
	storageLog.Info("Device deleted", "device_id", deviceID)
	return nil
}

// UpdateDeviceLastSeen updates the last_seen timestamp for a device.
func (s *Store) UpdateDeviceLastSeen(deviceID string) error {
	query := `UPDATE devices SET last_seen = CURRENT_TIMESTAMP WHERE device_id = ?`
	_, err := s.db.Exec(query, deviceID)
	if err != nil {
		storageLog.Warn("Failed to update device last_seen", "device_id", deviceID, "error", err)
		return fmt.Errorf("failed to update device last seen: %w", err)
	}
	return nil
}

// Request Tracking Management

// IncrementRequestCount atomically increments request metrics for a device on the current date.
// Uses UPSERT logic to handle first request and subsequent updates.
func (s *Store) IncrementRequestCount(deviceID string, latency time.Duration, bytes int64, isError bool) error {
	today := time.Now().Format("2006-01-02")
	latencyMs := float64(latency.Milliseconds())

	errorIncrement := 0
	if isError {
		errorIncrement = 1
	}

	// UPSERT query that atomically handles both INSERT and UPDATE
	// For new records: Initialize with first request values
	// For existing records: Increment counts and compute running average for latency
	query := `
		INSERT INTO request_logs (device_id, date, request_count, bytes_transferred, avg_latency_ms, error_count)
		VALUES (?, ?, 1, ?, ?, ?)
		ON CONFLICT(device_id, date) DO UPDATE SET
			request_count = request_count + 1,
			bytes_transferred = bytes_transferred + excluded.bytes_transferred,
			avg_latency_ms = (avg_latency_ms * request_count + excluded.avg_latency_ms) / (request_count + 1),
			error_count = error_count + excluded.error_count,
			updated_at = CURRENT_TIMESTAMP
	`

	_, err := s.db.Exec(query, deviceID, today, bytes, latencyMs, errorIncrement)
	if err != nil {
		return fmt.Errorf("failed to increment request count: %w", err)
	}

	return nil
}

// GetDeviceRequestsToday retrieves the request count for a device for the current date.
func (s *Store) GetDeviceRequestsToday(deviceID string) (int, error) {
	today := time.Now().Format("2006-01-02")

	query := `
		SELECT request_count
		FROM request_logs
		WHERE device_id = ? AND date = ?
	`

	var count int
	err := s.db.QueryRow(query, deviceID, today).Scan(&count)
	if err == sql.ErrNoRows {
		return 0, nil // No requests today
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get device requests today: %w", err)
	}

	return count, nil
}

// GetTotalRequestsToday retrieves the total request count across all devices for the current date.
func (s *Store) GetTotalRequestsToday() (int, error) {
	today := time.Now().Format("2006-01-02")

	query := `
		SELECT COALESCE(SUM(request_count), 0)
		FROM request_logs
		WHERE date = ?
	`

	var total int
	err := s.db.QueryRow(query, today).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("failed to get total requests today: %w", err)
	}

	return total, nil
}

// Audit Log Management

// LogAuditEvent persists an audit log event to the database
func (s *Store) LogAuditEvent(event AuditEvent) error {
	// Marshal details to JSON
	var detailsJSON string
	if event.Details != nil {
		data, err := json.Marshal(event.Details)
		if err != nil {
			return fmt.Errorf("failed to marshal details: %w", err)
		}
		detailsJSON = string(data)
	}

	successInt := 0
	if event.Success {
		successInt = 1
	}

	query := `
		INSERT INTO audit_logs (timestamp, action, actor_id, actor_ip, target_id, details, success, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := s.db.Exec(query,
		event.Timestamp,
		event.Action,
		event.ActorID,
		event.ActorIP,
		event.TargetID,
		detailsJSON,
		successInt,
		event.Error,
	)

	if err != nil {
		return fmt.Errorf("failed to log audit event: %w", err)
	}

	return nil
}

// ListAuditLogs retrieves audit logs with pagination
func (s *Store) ListAuditLogs(limit, offset int) ([]AuditEvent, error) {
	query := `
		SELECT id, timestamp, action, actor_id, actor_ip, target_id, details, success, error
		FROM audit_logs
		ORDER BY timestamp DESC
		LIMIT ? OFFSET ?
	`

	rows, err := s.db.Query(query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list audit logs: %w", err)
	}
	defer rows.Close()

	var logs []AuditEvent
	for rows.Next() {
		var log AuditEvent
		var detailsJSON sql.NullString
		var errorMsg sql.NullString
		var successInt int

		if err := rows.Scan(
			&log.ID,
			&log.Timestamp,
			&log.Action,
			&log.ActorID,
			&log.ActorIP,
			&log.TargetID,
			&detailsJSON,
			&successInt,
			&errorMsg,
		); err != nil {
			return nil, fmt.Errorf("failed to scan audit log: %w", err)
		}

		log.Success = successInt == 1

		if errorMsg.Valid {
			log.Error = errorMsg.String
		}

		if detailsJSON.Valid && detailsJSON.String != "" {
			if err := json.Unmarshal([]byte(detailsJSON.String), &log.Details); err != nil {
				return nil, fmt.Errorf("failed to unmarshal details: %w", err)
			}
		}

		logs = append(logs, log)
	}

	if logs == nil {
		logs = []AuditEvent{}
	}

	return logs, rows.Err()
}

// ListAuditLogsByAction retrieves audit logs filtered by action type
func (s *Store) ListAuditLogsByAction(action Action, limit, offset int) ([]AuditEvent, error) {
	query := `
		SELECT id, timestamp, action, actor_id, actor_ip, target_id, details, success, error
		FROM audit_logs
		WHERE action = ?
		ORDER BY timestamp DESC
		LIMIT ? OFFSET ?
	`

	rows, err := s.db.Query(query, action, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list audit logs by action: %w", err)
	}
	defer rows.Close()

	var logs []AuditEvent
	for rows.Next() {
		var log AuditEvent
		var detailsJSON sql.NullString
		var errorMsg sql.NullString
		var successInt int

		if err := rows.Scan(
			&log.ID,
			&log.Timestamp,
			&log.Action,
			&log.ActorID,
			&log.ActorIP,
			&log.TargetID,
			&detailsJSON,
			&successInt,
			&errorMsg,
		); err != nil {
			return nil, fmt.Errorf("failed to scan audit log: %w", err)
		}

		log.Success = successInt == 1

		if errorMsg.Valid {
			log.Error = errorMsg.String
		}

		if detailsJSON.Valid && detailsJSON.String != "" {
			if err := json.Unmarshal([]byte(detailsJSON.String), &log.Details); err != nil {
				return nil, fmt.Errorf("failed to unmarshal details: %w", err)
			}
		}

		logs = append(logs, log)
	}

	if logs == nil {
		logs = []AuditEvent{}
	}

	return logs, rows.Err()
}

// ListAuditLogsByActor retrieves audit logs filtered by actor ID
func (s *Store) ListAuditLogsByActor(actorID string, limit, offset int) ([]AuditEvent, error) {
	query := `
		SELECT id, timestamp, action, actor_id, actor_ip, target_id, details, success, error
		FROM audit_logs
		WHERE actor_id = ?
		ORDER BY timestamp DESC
		LIMIT ? OFFSET ?
	`

	rows, err := s.db.Query(query, actorID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list audit logs by actor: %w", err)
	}
	defer rows.Close()

	var logs []AuditEvent
	for rows.Next() {
		var log AuditEvent
		var detailsJSON sql.NullString
		var errorMsg sql.NullString
		var successInt int

		if err := rows.Scan(
			&log.ID,
			&log.Timestamp,
			&log.Action,
			&log.ActorID,
			&log.ActorIP,
			&log.TargetID,
			&detailsJSON,
			&successInt,
			&errorMsg,
		); err != nil {
			return nil, fmt.Errorf("failed to scan audit log: %w", err)
		}

		log.Success = successInt == 1

		if errorMsg.Valid {
			log.Error = errorMsg.String
		}

		if detailsJSON.Valid && detailsJSON.String != "" {
			if err := json.Unmarshal([]byte(detailsJSON.String), &log.Details); err != nil {
				return nil, fmt.Errorf("failed to unmarshal details: %w", err)
			}
		}

		logs = append(logs, log)
	}

	if logs == nil {
		logs = []AuditEvent{}
	}

	return logs, rows.Err()
}

// Audit Log Types

// Action represents different types of auditable actions
type Action string

const (
	ActionDevicePaired      Action = "device.paired"
	ActionDeviceRevoked     Action = "device.revoked"
	ActionDeviceUpdated     Action = "device.updated"
	ActionAppRegistered     Action = "app.registered"
	ActionAppDeleted        Action = "app.deleted"
	ActionRouteCreated      Action = "route.created"
	ActionRouteDeleted      Action = "route.deleted"
	ActionConfigReloaded    Action = "config.reloaded"
	ActionProposalApproved  Action = "proposal.approved"
	ActionProposalDismissed Action = "proposal.dismissed"
)

// AuditEvent represents an audit log event
type AuditEvent struct {
	ID        int64                  `json:"id"`
	Timestamp time.Time              `json:"timestamp"`
	Action    Action                 `json:"action"`
	ActorID   string                 `json:"actorId"`         // device ID or "system"
	ActorIP   string                 `json:"actorIp"`         // IP address of actor
	TargetID  string                 `json:"targetId"`        // ID of affected resource
	Details   map[string]interface{} `json:"details"`         // Additional context
	Success   bool                   `json:"success"`         // Whether operation succeeded
	Error     string                 `json:"error,omitempty"` // Error message if failed
}

// DeviceInfo holds device session information.
type DeviceInfo struct {
	ID              string    `json:"id"`              // Frontend expects "id"
	Name            string    `json:"name"`            // Frontend expects "name"
	Platform        string    `json:"platform"`        // e.g., "ios", "android"
	PlatformVersion string    `json:"platformVersion"` // e.g., "17.0"
	Scopes          []string  `json:"scopes"`
	PairedAt        time.Time `json:"pairedAt"` // Frontend expects "pairedAt" (renamed from CreatedAt)
	LastSeen        time.Time `json:"lastSeen"`
	Status          string    `json:"status"`        // online, offline (computed from LastSeen)
	RequestsToday   int       `json:"requestsToday"` // Request count for today (default 0 for now)
}

// ComputeStatus computes device status based on lastSeen timestamp.
// A device is "online" if last seen within 5 minutes, otherwise "offline".
func (d *DeviceInfo) ComputeStatus() {
	if time.Since(d.LastSeen) < 5*time.Minute {
		d.Status = "online"
	} else {
		d.Status = "offline"
	}
}

// Service Health Management

// ServiceHealth holds service health status information.
type ServiceHealth struct {
	AppID               string     `json:"appId"`
	Status              string     `json:"status"` // healthy, unhealthy, unknown
	LastCheckTime       *time.Time `json:"lastCheckTime,omitempty"`
	LastSuccessTime     *time.Time `json:"lastSuccessTime,omitempty"`
	ConsecutiveFailures int        `json:"consecutiveFailures"`
	ErrorMessage        string     `json:"errorMessage,omitempty"`
}

// SaveServiceHealth persists service health status.
func (s *Store) SaveServiceHealth(health ServiceHealth) error {
	query := `
		INSERT INTO service_health (app_id, status, last_check_time, last_success_time, consecutive_failures, error_message)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(app_id) DO UPDATE SET
			status = excluded.status,
			last_check_time = excluded.last_check_time,
			last_success_time = excluded.last_success_time,
			consecutive_failures = excluded.consecutive_failures,
			error_message = excluded.error_message
	`

	_, err := s.db.Exec(query,
		health.AppID,
		health.Status,
		health.LastCheckTime,
		health.LastSuccessTime,
		health.ConsecutiveFailures,
		health.ErrorMessage,
	)
	if err != nil {
		return fmt.Errorf("failed to save service health: %w", err)
	}

	return nil
}

// GetServiceHealth retrieves health status for a service.
func (s *Store) GetServiceHealth(appID string) (*ServiceHealth, error) {
	query := `
		SELECT app_id, status, last_check_time, last_success_time, consecutive_failures, error_message
		FROM service_health WHERE app_id = ?
	`

	var health ServiceHealth
	var lastCheckTime, lastSuccessTime sql.NullTime
	var errorMessage sql.NullString

	err := s.db.QueryRow(query, appID).Scan(
		&health.AppID,
		&health.Status,
		&lastCheckTime,
		&lastSuccessTime,
		&health.ConsecutiveFailures,
		&errorMessage,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get service health: %w", err)
	}

	if lastCheckTime.Valid {
		health.LastCheckTime = &lastCheckTime.Time
	}
	if lastSuccessTime.Valid {
		health.LastSuccessTime = &lastSuccessTime.Time
	}
	if errorMessage.Valid {
		health.ErrorMessage = errorMessage.String
	}

	return &health, nil
}

// ListServiceHealth retrieves health status for all services.
func (s *Store) ListServiceHealth() ([]ServiceHealth, error) {
	query := `
		SELECT app_id, status, last_check_time, last_success_time, consecutive_failures, error_message
		FROM service_health ORDER BY app_id
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list service health: %w", err)
	}
	defer rows.Close()

	var healthStatuses []ServiceHealth
	for rows.Next() {
		var health ServiceHealth
		var lastCheckTime, lastSuccessTime sql.NullTime
		var errorMessage sql.NullString

		if err := rows.Scan(
			&health.AppID,
			&health.Status,
			&lastCheckTime,
			&lastSuccessTime,
			&health.ConsecutiveFailures,
			&errorMessage,
		); err != nil {
			return nil, fmt.Errorf("failed to scan service health: %w", err)
		}

		if lastCheckTime.Valid {
			health.LastCheckTime = &lastCheckTime.Time
		}
		if lastSuccessTime.Valid {
			health.LastSuccessTime = &lastSuccessTime.Time
		}
		if errorMessage.Valid {
			health.ErrorMessage = errorMessage.String
		}

		healthStatuses = append(healthStatuses, health)
	}

	return healthStatuses, rows.Err()
}

// DeleteServiceHealth removes health status for a service.
func (s *Store) DeleteServiceHealth(appID string) error {
	query := `DELETE FROM service_health WHERE app_id = ?`
	_, err := s.db.Exec(query, appID)
	if err != nil {
		return fmt.Errorf("failed to delete service health: %w", err)
	}
	return nil
}

// Activity Events Management

// AddActivity adds a new activity event to the database and prunes old events.
func (s *Store) AddActivity(event types.ActivityEvent) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert new event
	query := `
		INSERT INTO activity_events (event_id, event_type, icon, icon_class, message, details, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	_, err = tx.Exec(query, event.ID, event.Type, event.Icon, event.IconClass, event.Message, event.Details, event.Timestamp)
	if err != nil {
		return fmt.Errorf("failed to insert activity event: %w", err)
	}

	// Prune old events - keep only last 10
	pruneQuery := `
		DELETE FROM activity_events
		WHERE id NOT IN (
			SELECT id FROM activity_events
			ORDER BY timestamp DESC
			LIMIT 10
		)
	`
	_, err = tx.Exec(pruneQuery)
	if err != nil {
		return fmt.Errorf("failed to prune old activity events: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetRecentActivity returns the last 10 activity events, newest first.
func (s *Store) GetRecentActivity() ([]types.ActivityEvent, error) {
	query := `
		SELECT event_id, event_type, icon, icon_class, message, details, timestamp
		FROM activity_events
		ORDER BY timestamp DESC
		LIMIT 10
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query activity events: %w", err)
	}
	defer rows.Close()

	var events []types.ActivityEvent
	for rows.Next() {
		var event types.ActivityEvent
		if err := rows.Scan(
			&event.ID,
			&event.Type,
			&event.Icon,
			&event.IconClass,
			&event.Message,
			&event.Details,
			&event.Timestamp,
		); err != nil {
			return nil, fmt.Errorf("failed to scan activity event: %w", err)
		}
		events = append(events, event)
	}

	if events == nil {
		events = []types.ActivityEvent{} // Return empty slice, not nil
	}

	return events, rows.Err()
}

// PruneOldActivity removes all but the last 10 activity events.
func (s *Store) PruneOldActivity() error {
	query := `
		DELETE FROM activity_events
		WHERE id NOT IN (
			SELECT id FROM activity_events
			ORDER BY timestamp DESC
			LIMIT 10
		)
	`
	_, err := s.db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to prune old activity events: %w", err)
	}
	return nil
}

// Certificate Management

// StoredCertificate represents a certificate stored in the database
type StoredCertificate struct {
	ID                      int64
	Domain                  string
	CertificatePEM          []byte
	PrivateKeyPEM           []byte
	Issuer                  string
	NotBefore               time.Time
	NotAfter                time.Time
	SubjectAlternativeNames string
	FingerprintSHA256       string
	CreatedAt               time.Time
	UpdatedAt               time.Time
	RenewalAttemptCount     int
	LastRenewalAttempt      *time.Time
	LastRenewalError        *string
}

// CertificateHistoryEntry represents a certificate audit log entry
type CertificateHistoryEntry struct {
	ID                int64
	Domain            string
	Action            string
	Issuer            string
	FingerprintSHA256 string
	CreatedAt         time.Time
	CreatedBy         string
	Metadata          string
}

// StoreCertificate saves or updates a certificate in the database
func (s *Store) StoreCertificate(cert *StoredCertificate) error {
	query := `
		INSERT INTO certificates (
			domain, certificate_pem, private_key_pem, issuer,
			not_before, not_after, subject_alternative_names,
			fingerprint_sha256, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(domain) DO UPDATE SET
			certificate_pem = excluded.certificate_pem,
			private_key_pem = excluded.private_key_pem,
			issuer = excluded.issuer,
			not_before = excluded.not_before,
			not_after = excluded.not_after,
			subject_alternative_names = excluded.subject_alternative_names,
			fingerprint_sha256 = excluded.fingerprint_sha256,
			updated_at = excluded.updated_at
	`

	now := time.Now()
	_, err := s.db.Exec(
		query,
		cert.Domain,
		cert.CertificatePEM,
		cert.PrivateKeyPEM,
		cert.Issuer,
		cert.NotBefore,
		cert.NotAfter,
		cert.SubjectAlternativeNames,
		cert.FingerprintSHA256,
		now,
		now,
	)

	if err != nil {
		return fmt.Errorf("failed to store certificate: %w", err)
	}

	return nil
}

// GetCertificate retrieves a certificate by domain
func (s *Store) GetCertificate(domain string) (*StoredCertificate, error) {
	query := `
		SELECT id, domain, certificate_pem, private_key_pem, issuer,
			not_before, not_after, subject_alternative_names,
			fingerprint_sha256, created_at, updated_at,
			renewal_attempt_count, last_renewal_attempt, last_renewal_error
		FROM certificates
		WHERE domain = ?
	`

	cert := &StoredCertificate{}
	err := s.db.QueryRow(query, domain).Scan(
		&cert.ID,
		&cert.Domain,
		&cert.CertificatePEM,
		&cert.PrivateKeyPEM,
		&cert.Issuer,
		&cert.NotBefore,
		&cert.NotAfter,
		&cert.SubjectAlternativeNames,
		&cert.FingerprintSHA256,
		&cert.CreatedAt,
		&cert.UpdatedAt,
		&cert.RenewalAttemptCount,
		&cert.LastRenewalAttempt,
		&cert.LastRenewalError,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get certificate: %w", err)
	}

	return cert, nil
}

// ListCertificates returns all certificates from the database
func (s *Store) ListCertificates() ([]*StoredCertificate, error) {
	query := `
		SELECT id, domain, certificate_pem, private_key_pem, issuer,
			not_before, not_after, subject_alternative_names,
			fingerprint_sha256, created_at, updated_at,
			renewal_attempt_count, last_renewal_attempt, last_renewal_error
		FROM certificates
		ORDER BY domain ASC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list certificates: %w", err)
	}
	defer rows.Close()

	var certificates []*StoredCertificate
	for rows.Next() {
		cert := &StoredCertificate{}
		err := rows.Scan(
			&cert.ID,
			&cert.Domain,
			&cert.CertificatePEM,
			&cert.PrivateKeyPEM,
			&cert.Issuer,
			&cert.NotBefore,
			&cert.NotAfter,
			&cert.SubjectAlternativeNames,
			&cert.FingerprintSHA256,
			&cert.CreatedAt,
			&cert.UpdatedAt,
			&cert.RenewalAttemptCount,
			&cert.LastRenewalAttempt,
			&cert.LastRenewalError,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan certificate: %w", err)
		}
		certificates = append(certificates, cert)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating certificates: %w", err)
	}

	return certificates, nil
}

// DeleteCertificate removes a certificate from the database
func (s *Store) DeleteCertificate(domain string) error {
	query := `DELETE FROM certificates WHERE domain = ?`
	result, err := s.db.Exec(query, domain)
	if err != nil {
		return fmt.Errorf("failed to delete certificate: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("certificate not found: %s", domain)
	}

	return nil
}

// AddCertificateHistory adds an audit log entry for certificate operations
func (s *Store) AddCertificateHistory(entry CertificateHistoryEntry) error {
	query := `
		INSERT INTO certificate_history (
			domain, action, issuer, fingerprint_sha256,
			created_at, created_by, metadata
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	_, err := s.db.Exec(
		query,
		entry.Domain,
		entry.Action,
		entry.Issuer,
		entry.FingerprintSHA256,
		entry.CreatedAt,
		entry.CreatedBy,
		entry.Metadata,
	)

	if err != nil {
		return fmt.Errorf("failed to add certificate history: %w", err)
	}

	return nil
}

// GetCertificateHistory retrieves the history for a specific domain
func (s *Store) GetCertificateHistory(domain string) ([]CertificateHistoryEntry, error) {
	query := `
		SELECT id, domain, action, issuer, fingerprint_sha256,
			created_at, created_by, metadata
		FROM certificate_history
		WHERE domain = ?
		ORDER BY created_at DESC
	`

	rows, err := s.db.Query(query, domain)
	if err != nil {
		return nil, fmt.Errorf("failed to get certificate history: %w", err)
	}
	defer rows.Close()

	var history []CertificateHistoryEntry
	for rows.Next() {
		var entry CertificateHistoryEntry
		err := rows.Scan(
			&entry.ID,
			&entry.Domain,
			&entry.Action,
			&entry.Issuer,
			&entry.FingerprintSHA256,
			&entry.CreatedAt,
			&entry.CreatedBy,
			&entry.Metadata,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan history entry: %w", err)
		}
		history = append(history, entry)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating history: %w", err)
	}

	return history, nil
}

// API Keys Management

// CreateAPIKey creates a new API key in the database
func (s *Store) CreateAPIKey(apiKey *types.APIKey) error {
	scopesJSON, err := json.Marshal(apiKey.Scopes)
	if err != nil {
		return fmt.Errorf("failed to marshal scopes: %w", err)
	}

	query := `
		INSERT INTO api_keys (
			id, name, key_hash, prefix, scopes,
			expires_at, last_used_at, created_at, created_by, revoked_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = s.db.Exec(
		query,
		apiKey.ID,
		apiKey.Name,
		apiKey.KeyHash,
		apiKey.Prefix,
		string(scopesJSON),
		apiKey.ExpiresAt,
		apiKey.LastUsedAt,
		apiKey.CreatedAt,
		apiKey.CreatedBy,
		apiKey.RevokedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create API key: %w", err)
	}

	return nil
}

// GetAPIKey retrieves an API key by ID
func (s *Store) GetAPIKey(id string) (*types.APIKey, error) {
	query := `
		SELECT id, name, key_hash, prefix, scopes,
			expires_at, last_used_at, created_at, created_by, revoked_at
		FROM api_keys
		WHERE id = ?
	`

	var apiKey types.APIKey
	var scopesJSON string
	var expiresAt, lastUsedAt, revokedAt sql.NullTime
	var createdBy sql.NullString

	err := s.db.QueryRow(query, id).Scan(
		&apiKey.ID,
		&apiKey.Name,
		&apiKey.KeyHash,
		&apiKey.Prefix,
		&scopesJSON,
		&expiresAt,
		&lastUsedAt,
		&apiKey.CreatedAt,
		&createdBy,
		&revokedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get API key: %w", err)
	}

	if err := json.Unmarshal([]byte(scopesJSON), &apiKey.Scopes); err != nil {
		return nil, fmt.Errorf("failed to unmarshal scopes: %w", err)
	}

	if expiresAt.Valid {
		apiKey.ExpiresAt = &expiresAt.Time
	}
	if lastUsedAt.Valid {
		apiKey.LastUsedAt = &lastUsedAt.Time
	}
	if createdBy.Valid {
		apiKey.CreatedBy = createdBy.String
	}
	if revokedAt.Valid {
		apiKey.RevokedAt = &revokedAt.Time
	}

	return &apiKey, nil
}

// GetAPIKeyByHash retrieves an API key by its hash (for authentication)
func (s *Store) GetAPIKeyByHash(hash string) (*types.APIKey, error) {
	query := `
		SELECT id, name, key_hash, prefix, scopes,
			expires_at, last_used_at, created_at, created_by, revoked_at
		FROM api_keys
		WHERE key_hash = ? AND revoked_at IS NULL
	`

	var apiKey types.APIKey
	var scopesJSON string
	var expiresAt, lastUsedAt, revokedAt sql.NullTime
	var createdBy sql.NullString

	err := s.db.QueryRow(query, hash).Scan(
		&apiKey.ID,
		&apiKey.Name,
		&apiKey.KeyHash,
		&apiKey.Prefix,
		&scopesJSON,
		&expiresAt,
		&lastUsedAt,
		&apiKey.CreatedAt,
		&createdBy,
		&revokedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get API key by hash: %w", err)
	}

	if err := json.Unmarshal([]byte(scopesJSON), &apiKey.Scopes); err != nil {
		return nil, fmt.Errorf("failed to unmarshal scopes: %w", err)
	}

	if expiresAt.Valid {
		apiKey.ExpiresAt = &expiresAt.Time
	}
	if lastUsedAt.Valid {
		apiKey.LastUsedAt = &lastUsedAt.Time
	}
	if createdBy.Valid {
		apiKey.CreatedBy = createdBy.String
	}
	if revokedAt.Valid {
		apiKey.RevokedAt = &revokedAt.Time
	}

	return &apiKey, nil
}

// ListAPIKeys retrieves all API keys
func (s *Store) ListAPIKeys() ([]*types.APIKey, error) {
	query := `
		SELECT id, name, key_hash, prefix, scopes,
			expires_at, last_used_at, created_at, created_by, revoked_at
		FROM api_keys
		ORDER BY created_at DESC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list API keys: %w", err)
	}
	defer rows.Close()

	var apiKeys []*types.APIKey
	for rows.Next() {
		var apiKey types.APIKey
		var scopesJSON string
		var expiresAt, lastUsedAt, revokedAt sql.NullTime
		var createdBy sql.NullString

		err := rows.Scan(
			&apiKey.ID,
			&apiKey.Name,
			&apiKey.KeyHash,
			&apiKey.Prefix,
			&scopesJSON,
			&expiresAt,
			&lastUsedAt,
			&apiKey.CreatedAt,
			&createdBy,
			&revokedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan API key: %w", err)
		}

		if err := json.Unmarshal([]byte(scopesJSON), &apiKey.Scopes); err != nil {
			return nil, fmt.Errorf("failed to unmarshal scopes: %w", err)
		}

		if expiresAt.Valid {
			apiKey.ExpiresAt = &expiresAt.Time
		}
		if lastUsedAt.Valid {
			apiKey.LastUsedAt = &lastUsedAt.Time
		}
		if createdBy.Valid {
			apiKey.CreatedBy = createdBy.String
		}
		if revokedAt.Valid {
			apiKey.RevokedAt = &revokedAt.Time
		}

		apiKeys = append(apiKeys, &apiKey)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating API keys: %w", err)
	}

	return apiKeys, nil
}

// RevokeAPIKey marks an API key as revoked
func (s *Store) RevokeAPIKey(id string) error {
	query := `UPDATE api_keys SET revoked_at = CURRENT_TIMESTAMP WHERE id = ?`
	result, err := s.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to revoke API key: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("API key not found: %s", id)
	}

	return nil
}

// UpdateAPIKeyLastUsed updates the last_used_at timestamp for an API key
func (s *Store) UpdateAPIKeyLastUsed(id string) error {
	query := `UPDATE api_keys SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?`
	_, err := s.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to update API key last used: %w", err)
	}
	return nil
}

// DeleteAPIKey permanently deletes an API key from the database
func (s *Store) DeleteAPIKey(id string) error {
	query := `DELETE FROM api_keys WHERE id = ?`
	result, err := s.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete API key: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("API key not found: %s", id)
	}

	return nil
}

// Notifications Management

// EnqueueNotification adds a new notification to the queue
func (s *Store) EnqueueNotification(deviceID, msgType string, payload json.RawMessage, ttl time.Duration, maxRetries int) (int64, error) {
	query := `
		INSERT INTO notifications (device_id, type, payload, status, max_retries, created_at, expires_at)
		VALUES (?, ?, ?, 'pending', ?, CURRENT_TIMESTAMP, ?)
	`

	expiresAt := time.Now().Add(ttl)
	result, err := s.db.Exec(query, deviceID, msgType, string(payload), maxRetries, expiresAt)
	if err != nil {
		return 0, fmt.Errorf("failed to enqueue notification: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get notification ID: %w", err)
	}

	return id, nil
}

// GetPendingNotifications retrieves all pending notifications for a device
func (s *Store) GetPendingNotifications(deviceID string) ([]*notifications.StoredNotification, error) {
	query := `
		SELECT id, device_id, type, payload, status, retry_count, max_retries,
			   created_at, expires_at, delivered_at, last_attempt_at, error_message
		FROM notifications
		WHERE device_id = ? AND status IN ('pending', 'failed') AND datetime(expires_at) > datetime('now')
		ORDER BY created_at ASC
	`

	rows, err := s.db.Query(query, deviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending notifications: %w", err)
	}
	defer rows.Close()

	var result []*notifications.StoredNotification
	for rows.Next() {
		var notif notifications.StoredNotification
		var payloadStr string
		var deliveredAt, lastAttemptAt sql.NullTime
		var errorMessage sql.NullString

		err := rows.Scan(
			&notif.ID,
			&notif.DeviceID,
			&notif.Type,
			&payloadStr,
			&notif.Status,
			&notif.RetryCount,
			&notif.MaxRetries,
			&notif.CreatedAt,
			&notif.ExpiresAt,
			&deliveredAt,
			&lastAttemptAt,
			&errorMessage,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan notification: %w", err)
		}

		notif.Payload = json.RawMessage(payloadStr)
		if deliveredAt.Valid {
			notif.DeliveredAt = deliveredAt.Time
		}
		if lastAttemptAt.Valid {
			notif.LastAttemptAt = lastAttemptAt.Time
		}
		if errorMessage.Valid {
			notif.ErrorMessage = errorMessage.String
		}

		result = append(result, &notif)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating notifications: %w", err)
	}

	if result == nil {
		result = []*notifications.StoredNotification{} // Return empty slice, not nil
	}

	return result, nil
}

// MarkNotificationDelivered marks a notification as delivered
func (s *Store) MarkNotificationDelivered(id int64) error {
	query := `
		UPDATE notifications
		SET status = 'delivered', delivered_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`

	_, err := s.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to mark notification delivered: %w", err)
	}

	return nil
}

// MarkNotificationExpired marks a notification as expired
func (s *Store) MarkNotificationExpired(id int64) error {
	query := `
		UPDATE notifications
		SET status = 'expired', error_message = 'notification expired'
		WHERE id = ?
	`

	_, err := s.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to mark notification expired: %w", err)
	}

	return nil
}

// UpdateNotificationRetry updates retry count and checks if max retries reached
func (s *Store) UpdateNotificationRetry(id int64, errorMsg string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Increment retry count
	updateQuery := `
		UPDATE notifications
		SET retry_count = retry_count + 1,
			last_attempt_at = CURRENT_TIMESTAMP,
			error_message = ?
		WHERE id = ?
	`
	_, err = tx.Exec(updateQuery, errorMsg, id)
	if err != nil {
		return fmt.Errorf("failed to update retry count: %w", err)
	}

	// Check if max retries reached
	checkQuery := `
		UPDATE notifications
		SET status = 'failed'
		WHERE id = ? AND retry_count >= max_retries
	`
	_, err = tx.Exec(checkQuery, id)
	if err != nil {
		return fmt.Errorf("failed to check max retries: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetNotificationByID retrieves a single notification by ID
func (s *Store) GetNotificationByID(id int64) (*notifications.StoredNotification, error) {
	query := `
		SELECT id, device_id, type, payload, status, retry_count, max_retries,
			   created_at, expires_at, delivered_at, last_attempt_at, error_message
		FROM notifications
		WHERE id = ?
	`

	var notif notifications.StoredNotification
	var payloadStr string
	var deliveredAt, lastAttemptAt sql.NullTime
	var errorMessage sql.NullString

	err := s.db.QueryRow(query, id).Scan(
		&notif.ID,
		&notif.DeviceID,
		&notif.Type,
		&payloadStr,
		&notif.Status,
		&notif.RetryCount,
		&notif.MaxRetries,
		&notif.CreatedAt,
		&notif.ExpiresAt,
		&deliveredAt,
		&lastAttemptAt,
		&errorMessage,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get notification by ID: %w", err)
	}

	notif.Payload = json.RawMessage(payloadStr)
	if deliveredAt.Valid {
		notif.DeliveredAt = deliveredAt.Time
	}
	if lastAttemptAt.Valid {
		notif.LastAttemptAt = lastAttemptAt.Time
	}
	if errorMessage.Valid {
		notif.ErrorMessage = errorMessage.String
	}

	return &notif, nil
}

// GetAllRetryableNotifications retrieves all pending notifications across all devices
// Used by the background retry processor
func (s *Store) GetAllRetryableNotifications() ([]*notifications.StoredNotification, error) {
	query := `
		SELECT id, device_id, type, payload, status, retry_count, max_retries,
			   created_at, expires_at, delivered_at, last_attempt_at, error_message
		FROM notifications
		WHERE status = 'pending' AND datetime(expires_at) > datetime('now')
		ORDER BY created_at ASC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get retryable notifications: %w", err)
	}
	defer rows.Close()

	var result []*notifications.StoredNotification
	for rows.Next() {
		var notif notifications.StoredNotification
		var payloadStr string
		var deliveredAt, lastAttemptAt sql.NullTime
		var errorMessage sql.NullString

		err := rows.Scan(
			&notif.ID,
			&notif.DeviceID,
			&notif.Type,
			&payloadStr,
			&notif.Status,
			&notif.RetryCount,
			&notif.MaxRetries,
			&notif.CreatedAt,
			&notif.ExpiresAt,
			&deliveredAt,
			&lastAttemptAt,
			&errorMessage,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan notification: %w", err)
		}

		notif.Payload = json.RawMessage(payloadStr)
		if deliveredAt.Valid {
			notif.DeliveredAt = deliveredAt.Time
		}
		if lastAttemptAt.Valid {
			notif.LastAttemptAt = lastAttemptAt.Time
		}
		if errorMessage.Valid {
			notif.ErrorMessage = errorMessage.String
		}

		result = append(result, &notif)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating notifications: %w", err)
	}

	if result == nil {
		result = []*notifications.StoredNotification{}
	}

	return result, nil
}

// ResetNotificationForRetry resets a notification status to pending for retry
func (s *Store) ResetNotificationForRetry(id int64) error {
	query := `UPDATE notifications SET status = 'pending', retry_count = 0, error_message = NULL WHERE id = ?`
	result, err := s.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to reset notification for retry: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("notification not found")
	}

	return nil
}

// DeleteNotificationsForDevice removes all pending notifications for a device
// Called when a device is revoked to clean up orphaned notifications
func (s *Store) DeleteNotificationsForDevice(deviceID string) (int64, error) {
	query := `DELETE FROM notifications WHERE device_id = ? AND status = 'pending'`

	result, err := s.db.Exec(query, deviceID)
	if err != nil {
		return 0, fmt.Errorf("failed to delete notifications for device: %w", err)
	}

	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get deleted count: %w", err)
	}

	return count, nil
}

// StaleNotificationSummary represents aggregated stale notification info per device
type StaleNotificationSummary struct {
	DeviceID     string    `json:"deviceId"`
	DeviceName   string    `json:"deviceName"`
	Count        int       `json:"count"`
	OldestAt     time.Time `json:"oldestAt"`
	NewestAt     time.Time `json:"newestAt"`
	Types        []string  `json:"types"`
	TotalRetries int       `json:"totalRetries"`
}

// GetStaleNotifications returns notifications that have been pending longer than the threshold
func (s *Store) GetStaleNotifications(staleThreshold time.Duration) ([]*StaleNotificationSummary, error) {
	thresholdTime := time.Now().Add(-staleThreshold)

	query := `
		SELECT
			n.device_id,
			COALESCE(d.device_name, n.device_id) as device_name,
			COUNT(*) as count,
			MIN(n.created_at) as oldest_at,
			MAX(n.created_at) as newest_at,
			GROUP_CONCAT(DISTINCT n.type) as types,
			SUM(n.retry_count) as total_retries
		FROM notifications n
		LEFT JOIN devices d ON n.device_id = d.device_id
		WHERE n.status = 'pending' AND datetime(n.created_at) < datetime(?)
		GROUP BY n.device_id
		ORDER BY oldest_at ASC
	`

	rows, err := s.db.Query(query, thresholdTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get stale notifications: %w", err)
	}
	defer rows.Close()

	var result []*StaleNotificationSummary
	for rows.Next() {
		var summary StaleNotificationSummary
		var typesStr string
		var oldestAtStr, newestAtStr string

		err := rows.Scan(
			&summary.DeviceID,
			&summary.DeviceName,
			&summary.Count,
			&oldestAtStr,
			&newestAtStr,
			&typesStr,
			&summary.TotalRetries,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan stale notification: %w", err)
		}

		// Parse time strings (SQLite returns timestamps as strings)
		if oldestAtStr != "" {
			if t, err := time.Parse(time.RFC3339Nano, oldestAtStr); err == nil {
				summary.OldestAt = t
			} else if t, err := time.Parse("2006-01-02 15:04:05", oldestAtStr); err == nil {
				summary.OldestAt = t
			}
		}
		if newestAtStr != "" {
			if t, err := time.Parse(time.RFC3339Nano, newestAtStr); err == nil {
				summary.NewestAt = t
			} else if t, err := time.Parse("2006-01-02 15:04:05", newestAtStr); err == nil {
				summary.NewestAt = t
			}
		}

		// Parse comma-separated types
		if typesStr != "" {
			summary.Types = strings.Split(typesStr, ",")
		}

		result = append(result, &summary)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating stale notifications: %w", err)
	}

	if result == nil {
		result = []*StaleNotificationSummary{}
	}

	return result, nil
}

// NotificationQueueStats represents overall notification queue statistics
type NotificationQueueStats struct {
	TotalPending   int `json:"totalPending"`
	TotalDelivered int `json:"totalDelivered"`
	TotalFailed    int `json:"totalFailed"`
	StaleCount     int `json:"staleCount"` // Pending > stale threshold
}

// GetNotificationQueueStats returns statistics about the notification queue
func (s *Store) GetNotificationQueueStats(staleThreshold time.Duration) (*NotificationQueueStats, error) {
	thresholdTime := time.Now().Add(-staleThreshold)

	query := `
		SELECT
			COALESCE(SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END), 0) as total_pending,
			COALESCE(SUM(CASE WHEN status = 'delivered' THEN 1 ELSE 0 END), 0) as total_delivered,
			COALESCE(SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END), 0) as total_failed,
			COALESCE(SUM(CASE WHEN status = 'pending' AND datetime(created_at) < datetime(?) THEN 1 ELSE 0 END), 0) as stale_count
		FROM notifications
	`

	var stats NotificationQueueStats
	err := s.db.QueryRow(query, thresholdTime).Scan(
		&stats.TotalPending,
		&stats.TotalDelivered,
		&stats.TotalFailed,
		&stats.StaleCount,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get notification stats: %w", err)
	}

	return &stats, nil
}

// NotificationListItem represents a notification in the list view
type NotificationListItem struct {
	ID            int64     `json:"id"`
	DeviceID      string    `json:"deviceId"`
	DeviceName    string    `json:"deviceName"`
	Type          string    `json:"type"`
	Status        string    `json:"status"`
	RetryCount    int       `json:"retryCount"`
	MaxRetries    int       `json:"maxRetries"`
	CreatedAt     time.Time `json:"createdAt"`
	ExpiresAt     time.Time `json:"expiresAt"`
	DeliveredAt   time.Time `json:"deliveredAt,omitempty"`
	LastAttemptAt time.Time `json:"lastAttemptAt,omitempty"`
	ErrorMessage  string    `json:"errorMessage,omitempty"`
	IsStale       bool      `json:"isStale"`
}

// NotificationListFilter defines filters for listing notifications
type NotificationListFilter struct {
	Status   string // pending, delivered, failed, or empty for all
	DeviceID string // filter by device
	Type     string // filter by notification type
	Limit    int    // max results (default 50)
	Offset   int    // pagination offset
}

// NotificationListResult contains paginated notification results
type NotificationListResult struct {
	Notifications []*NotificationListItem `json:"notifications"`
	Total         int                     `json:"total"`
	Limit         int                     `json:"limit"`
	Offset        int                     `json:"offset"`
}

// ListNotifications returns notifications with filtering and pagination
func (s *Store) ListNotifications(filter NotificationListFilter, staleThreshold time.Duration) (*NotificationListResult, error) {
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	if filter.Limit > 200 {
		filter.Limit = 200
	}

	thresholdTime := time.Now().Add(-staleThreshold)

	// Build WHERE clause
	where := "1=1"
	args := []interface{}{}

	if filter.Status != "" {
		where += " AND n.status = ?"
		args = append(args, filter.Status)
	}
	if filter.DeviceID != "" {
		where += " AND n.device_id = ?"
		args = append(args, filter.DeviceID)
	}
	if filter.Type != "" {
		where += " AND n.type = ?"
		args = append(args, filter.Type)
	}

	// Get total count
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM notifications n WHERE %s`, where)
	var total int
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("failed to count notifications: %w", err)
	}

	// Get notifications
	query := fmt.Sprintf(`
		SELECT
			n.id, n.device_id, COALESCE(d.device_name, n.device_id) as device_name,
			n.type, n.status, n.retry_count, n.max_retries,
			n.created_at, n.expires_at, n.delivered_at, n.last_attempt_at, n.error_message,
			CASE WHEN n.status = 'pending' AND datetime(n.created_at) < datetime(?) THEN 1 ELSE 0 END as is_stale
		FROM notifications n
		LEFT JOIN devices d ON n.device_id = d.device_id
		WHERE %s
		ORDER BY n.created_at DESC
		LIMIT ? OFFSET ?
	`, where)

	// Add stale threshold, limit and offset to args
	queryArgs := append([]interface{}{thresholdTime}, args...)
	queryArgs = append(queryArgs, filter.Limit, filter.Offset)

	rows, err := s.db.Query(query, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to list notifications: %w", err)
	}
	defer rows.Close()

	var notifications []*NotificationListItem
	for rows.Next() {
		var item NotificationListItem
		var deliveredAt, lastAttemptAt sql.NullTime
		var errorMessage sql.NullString
		var isStale int

		err := rows.Scan(
			&item.ID, &item.DeviceID, &item.DeviceName,
			&item.Type, &item.Status, &item.RetryCount, &item.MaxRetries,
			&item.CreatedAt, &item.ExpiresAt, &deliveredAt, &lastAttemptAt, &errorMessage,
			&isStale,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan notification: %w", err)
		}

		if deliveredAt.Valid {
			item.DeliveredAt = deliveredAt.Time
		}
		if lastAttemptAt.Valid {
			item.LastAttemptAt = lastAttemptAt.Time
		}
		if errorMessage.Valid {
			item.ErrorMessage = errorMessage.String
		}
		item.IsStale = isStale == 1

		notifications = append(notifications, &item)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating notifications: %w", err)
	}

	if notifications == nil {
		notifications = []*NotificationListItem{}
	}

	return &NotificationListResult{
		Notifications: notifications,
		Total:         total,
		Limit:         filter.Limit,
		Offset:        filter.Offset,
	}, nil
}

// DismissNotification marks a notification as dismissed (removes from queue)
func (s *Store) DismissNotification(id int64) error {
	query := `UPDATE notifications SET status = 'dismissed' WHERE id = ? AND status = 'pending'`

	result, err := s.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to dismiss notification: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check affected rows: %w", err)
	}

	if affected == 0 {
		return fmt.Errorf("notification not found or already processed")
	}

	return nil
}

// DismissNotificationsForDevice dismisses all pending notifications for a device
func (s *Store) DismissNotificationsForDevice(deviceID string) (int64, error) {
	query := `UPDATE notifications SET status = 'dismissed' WHERE device_id = ? AND status = 'pending'`

	result, err := s.db.Exec(query, deviceID)
	if err != nil {
		return 0, fmt.Errorf("failed to dismiss notifications: %w", err)
	}

	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get affected count: %w", err)
	}

	return count, nil
}

// ClearDeliveredNotifications deletes all delivered notifications from the queue
func (s *Store) ClearDeliveredNotifications() (int64, error) {
	query := `DELETE FROM notifications WHERE status = 'delivered'`

	result, err := s.db.Exec(query)
	if err != nil {
		return 0, fmt.Errorf("failed to clear delivered notifications: %w", err)
	}

	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get affected count: %w", err)
	}

	return count, nil
}

// Toolbox Deployments

// SaveDeployment saves or updates a toolbox deployment.
func (s *Store) SaveDeployment(deployment *types.ToolboxDeployment) error {
	if deployment == nil {
		return fmt.Errorf("deployment cannot be nil")
	}

	// Serialize array fields to JSON
	networkNamesJSON, err := json.Marshal(deployment.NetworkNames)
	if err != nil {
		return fmt.Errorf("failed to marshal network names: %w", err)
	}

	volumeNamesJSON, err := json.Marshal(deployment.VolumeNames)
	if err != nil {
		return fmt.Errorf("failed to marshal volume names: %w", err)
	}

	envVarsJSON, err := json.Marshal(deployment.EnvVars)
	if err != nil {
		return fmt.Errorf("failed to marshal env vars: %w", err)
	}

	query := `
		INSERT INTO toolbox_deployments (
			id, service_template_id, service_name, status,
			container_id, container_name, project_name, network_names, volume_names,
			env_vars, custom_image, custom_port, route_id, error_message, deployed_at, deployed_by,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			service_template_id = excluded.service_template_id,
			service_name = excluded.service_name,
			status = excluded.status,
			container_id = excluded.container_id,
			container_name = excluded.container_name,
			project_name = excluded.project_name,
			network_names = excluded.network_names,
			volume_names = excluded.volume_names,
			env_vars = excluded.env_vars,
			custom_image = excluded.custom_image,
			custom_port = excluded.custom_port,
			route_id = excluded.route_id,
			error_message = excluded.error_message,
			deployed_at = excluded.deployed_at,
			deployed_by = excluded.deployed_by,
			updated_at = excluded.updated_at
	`

	_, err = s.db.Exec(query,
		deployment.ID,
		deployment.ServiceTemplateID,
		deployment.ServiceName,
		deployment.Status,
		deployment.ContainerID,
		deployment.ContainerName,
		deployment.ProjectName,
		string(networkNamesJSON),
		string(volumeNamesJSON),
		string(envVarsJSON),
		deployment.CustomImage,
		deployment.CustomPort,
		deployment.RouteID,
		deployment.ErrorMessage,
		deployment.DeployedAt,
		deployment.DeployedBy,
		deployment.CreatedAt,
		deployment.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to save deployment: %w", err)
	}

	return nil
}

// GetDeployment retrieves a toolbox deployment by ID.
func (s *Store) GetDeployment(id string) (*types.ToolboxDeployment, error) {
	query := `
		SELECT id, service_template_id, service_name, status,
			   container_id, container_name, project_name, network_names, volume_names,
			   env_vars, custom_image, custom_port, route_id, error_message, deployed_at, deployed_by,
			   created_at, updated_at
		FROM toolbox_deployments
		WHERE id = ?
	`

	var deployment types.ToolboxDeployment
	var networkNamesJSON, volumeNamesJSON, envVarsJSON string
	var deployedAt sql.NullTime
	var customImage, projectName sql.NullString

	err := s.db.QueryRow(query, id).Scan(
		&deployment.ID,
		&deployment.ServiceTemplateID,
		&deployment.ServiceName,
		&deployment.Status,
		&deployment.ContainerID,
		&deployment.ContainerName,
		&projectName,
		&networkNamesJSON,
		&volumeNamesJSON,
		&envVarsJSON,
		&customImage,
		&deployment.CustomPort,
		&deployment.RouteID,
		&deployment.ErrorMessage,
		&deployedAt,
		&deployment.DeployedBy,
		&deployment.CreatedAt,
		&deployment.UpdatedAt,
	)

	if customImage.Valid {
		deployment.CustomImage = customImage.String
	}
	if projectName.Valid {
		deployment.ProjectName = projectName.String
	}

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("deployment not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}

	// Handle nullable deployed_at
	if deployedAt.Valid {
		deployment.DeployedAt = &deployedAt.Time
	}

	// Deserialize JSON fields
	if err := json.Unmarshal([]byte(networkNamesJSON), &deployment.NetworkNames); err != nil {
		return nil, fmt.Errorf("failed to unmarshal network names: %w", err)
	}

	if err := json.Unmarshal([]byte(volumeNamesJSON), &deployment.VolumeNames); err != nil {
		return nil, fmt.Errorf("failed to unmarshal volume names: %w", err)
	}

	if err := json.Unmarshal([]byte(envVarsJSON), &deployment.EnvVars); err != nil {
		return nil, fmt.Errorf("failed to unmarshal env vars: %w", err)
	}

	return &deployment, nil
}

// ListDeployments retrieves all toolbox deployments.
func (s *Store) ListDeployments() ([]*types.ToolboxDeployment, error) {
	query := `
		SELECT id, service_template_id, service_name, status,
			   container_id, container_name, project_name, network_names, volume_names,
			   env_vars, custom_image, custom_port, route_id, error_message, deployed_at, deployed_by,
			   created_at, updated_at
		FROM toolbox_deployments
		ORDER BY created_at DESC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments: %w", err)
	}
	defer rows.Close()

	var deployments []*types.ToolboxDeployment
	for rows.Next() {
		var deployment types.ToolboxDeployment
		var networkNamesJSON, volumeNamesJSON, envVarsJSON string
		var deployedAt sql.NullTime
		var customImage, projectName sql.NullString

		err := rows.Scan(
			&deployment.ID,
			&deployment.ServiceTemplateID,
			&deployment.ServiceName,
			&deployment.Status,
			&deployment.ContainerID,
			&deployment.ContainerName,
			&projectName,
			&networkNamesJSON,
			&volumeNamesJSON,
			&envVarsJSON,
			&customImage,
			&deployment.CustomPort,
			&deployment.RouteID,
			&deployment.ErrorMessage,
			&deployedAt,
			&deployment.DeployedBy,
			&deployment.CreatedAt,
			&deployment.UpdatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan deployment: %w", err)
		}

		// Handle nullable fields
		if deployedAt.Valid {
			deployment.DeployedAt = &deployedAt.Time
		}
		if customImage.Valid {
			deployment.CustomImage = customImage.String
		}
		if projectName.Valid {
			deployment.ProjectName = projectName.String
		}

		// Deserialize JSON fields
		if err := json.Unmarshal([]byte(networkNamesJSON), &deployment.NetworkNames); err != nil {
			return nil, fmt.Errorf("failed to unmarshal network names: %w", err)
		}

		if err := json.Unmarshal([]byte(volumeNamesJSON), &deployment.VolumeNames); err != nil {
			return nil, fmt.Errorf("failed to unmarshal volume names: %w", err)
		}

		if err := json.Unmarshal([]byte(envVarsJSON), &deployment.EnvVars); err != nil {
			return nil, fmt.Errorf("failed to unmarshal env vars: %w", err)
		}

		deployments = append(deployments, &deployment)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating deployments: %w", err)
	}

	return deployments, nil
}

// ListDeploymentsByStatus retrieves deployments filtered by status.
func (s *Store) ListDeploymentsByStatus(status string) ([]*types.ToolboxDeployment, error) {
	query := `
		SELECT id, service_template_id, service_name, status,
			   container_id, container_name, project_name, network_names, volume_names,
			   env_vars, custom_image, custom_port, route_id, error_message, deployed_at, deployed_by,
			   created_at, updated_at
		FROM toolbox_deployments
		WHERE status = ?
		ORDER BY created_at DESC
	`

	rows, err := s.db.Query(query, status)
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments by status: %w", err)
	}
	defer rows.Close()

	var deployments []*types.ToolboxDeployment
	for rows.Next() {
		var deployment types.ToolboxDeployment
		var networkNamesJSON, volumeNamesJSON, envVarsJSON string
		var deployedAt sql.NullTime
		var customImage, projectName sql.NullString

		err := rows.Scan(
			&deployment.ID,
			&deployment.ServiceTemplateID,
			&deployment.ServiceName,
			&deployment.Status,
			&deployment.ContainerID,
			&deployment.ContainerName,
			&projectName,
			&networkNamesJSON,
			&volumeNamesJSON,
			&envVarsJSON,
			&customImage,
			&deployment.CustomPort,
			&deployment.RouteID,
			&deployment.ErrorMessage,
			&deployedAt,
			&deployment.DeployedBy,
			&deployment.CreatedAt,
			&deployment.UpdatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan deployment: %w", err)
		}

		// Handle nullable fields
		if deployedAt.Valid {
			deployment.DeployedAt = &deployedAt.Time
		}
		if customImage.Valid {
			deployment.CustomImage = customImage.String
		}
		if projectName.Valid {
			deployment.ProjectName = projectName.String
		}

		// Deserialize JSON fields
		if err := json.Unmarshal([]byte(networkNamesJSON), &deployment.NetworkNames); err != nil {
			return nil, fmt.Errorf("failed to unmarshal network names: %w", err)
		}

		if err := json.Unmarshal([]byte(volumeNamesJSON), &deployment.VolumeNames); err != nil {
			return nil, fmt.Errorf("failed to unmarshal volume names: %w", err)
		}

		if err := json.Unmarshal([]byte(envVarsJSON), &deployment.EnvVars); err != nil {
			return nil, fmt.Errorf("failed to unmarshal env vars: %w", err)
		}

		deployments = append(deployments, &deployment)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating deployments: %w", err)
	}

	return deployments, nil
}

// UpdateDeploymentStatus updates the status and error message of a deployment.
func (s *Store) UpdateDeploymentStatus(id, status, errorMessage string) error {
	query := `
		UPDATE toolbox_deployments
		SET status = ?, error_message = ?, updated_at = ?
		WHERE id = ?
	`

	result, err := s.db.Exec(query, status, errorMessage, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update deployment status: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("deployment not found: %s", id)
	}

	return nil
}

// UpdateDeploymentContainer updates the container ID and route ID of a deployment.
func (s *Store) UpdateDeploymentContainer(id, containerID, routeID string) error {
	query := `
		UPDATE toolbox_deployments
		SET container_id = ?, route_id = ?, updated_at = ?
		WHERE id = ?
	`

	result, err := s.db.Exec(query, containerID, routeID, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update deployment container: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("deployment not found: %s", id)
	}

	return nil
}

// DeleteDeployment deletes a toolbox deployment by ID.
func (s *Store) DeleteDeployment(id string) error {
	query := `DELETE FROM toolbox_deployments WHERE id = ?`

	_, err := s.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete deployment: %w", err)
	}

	// Deletion is idempotent - no error if deployment doesn't exist
	return nil
}

// Federation Peer Storage Methods

// PeerInfo represents a federation peer (avoiding circular import with federation package)
type PeerInfo struct {
	ID            string
	Name          string
	APIAddress    string
	GossipAddress string
	Status        string
	LastSeen      time.Time
	Metadata      map[string]string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// SavePeer persists a peer to the database.
func (s *Store) SavePeer(peer *PeerInfo) error {
	metadataJSON, err := json.Marshal(peer.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `
		INSERT INTO federation_peers (peer_id, name, api_address, gossip_address, status, last_seen, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(peer_id) DO UPDATE SET
			name = excluded.name,
			api_address = excluded.api_address,
			gossip_address = excluded.gossip_address,
			status = excluded.status,
			last_seen = excluded.last_seen,
			metadata = excluded.metadata,
			updated_at = CURRENT_TIMESTAMP
	`

	_, err = s.db.Exec(query, peer.ID, peer.Name, peer.APIAddress, peer.GossipAddress, peer.Status, peer.LastSeen, string(metadataJSON))
	if err != nil {
		return fmt.Errorf("failed to save peer: %w", err)
	}

	return nil
}

// ListPeers retrieves all peers from the database.
func (s *Store) ListPeers() ([]*PeerInfo, error) {
	query := `
		SELECT peer_id, name, api_address, gossip_address, status, last_seen, metadata, created_at, updated_at
		FROM federation_peers
		ORDER BY created_at ASC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list peers: %w", err)
	}
	defer rows.Close()

	var peers []*PeerInfo
	for rows.Next() {
		var peer PeerInfo
		var metadataJSON string

		err := rows.Scan(
			&peer.ID,
			&peer.Name,
			&peer.APIAddress,
			&peer.GossipAddress,
			&peer.Status,
			&peer.LastSeen,
			&metadataJSON,
			&peer.CreatedAt,
			&peer.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan peer: %w", err)
		}

		if err := json.Unmarshal([]byte(metadataJSON), &peer.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal peer metadata: %w", err)
		}

		peers = append(peers, &peer)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating peers: %w", err)
	}

	return peers, nil
}

// GetPeer retrieves a specific peer by ID.
func (s *Store) GetPeer(peerID string) (*PeerInfo, error) {
	query := `
		SELECT peer_id, name, api_address, gossip_address, status, last_seen, metadata, created_at, updated_at
		FROM federation_peers
		WHERE peer_id = ?
	`

	var peer PeerInfo
	var metadataJSON string

	err := s.db.QueryRow(query, peerID).Scan(
		&peer.ID,
		&peer.Name,
		&peer.APIAddress,
		&peer.GossipAddress,
		&peer.Status,
		&peer.LastSeen,
		&metadataJSON,
		&peer.CreatedAt,
		&peer.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("peer not found: %s", peerID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get peer: %w", err)
	}

	if err := json.Unmarshal([]byte(metadataJSON), &peer.Metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal peer metadata: %w", err)
	}

	return &peer, nil
}

// DeletePeer deletes a peer from the database.
func (s *Store) DeletePeer(peerID string) error {
	query := `DELETE FROM federation_peers WHERE peer_id = ?`

	_, err := s.db.Exec(query, peerID)
	if err != nil {
		return fmt.Errorf("failed to delete peer: %w", err)
	}

	// Deletion is idempotent - no error if peer doesn't exist
	return nil
}

// Federation Services Storage Methods

// FederatedServiceData represents a federated service stored in the database
// This is a storage-specific struct to avoid circular dependencies
type FederatedServiceData struct {
	ServiceID    string
	OriginPeerID string
	AppData      string // JSON-serialized types.App
	Confidence   float64
	LastSeen     time.Time
	Tombstone    bool
	VectorClock  string // JSON-serialized VectorClock
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// SaveFederatedService persists a federated service to the database
func (s *Store) SaveFederatedService(serviceID, originPeerID string, appData string, confidence float64, lastSeen time.Time, tombstone bool, vectorClock string) error {
	query := `
		INSERT INTO federation_services (
			service_id, origin_peer_id, app_data, confidence, last_seen, tombstone, vector_clock, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(service_id) DO UPDATE SET
			origin_peer_id = excluded.origin_peer_id,
			app_data = excluded.app_data,
			confidence = excluded.confidence,
			last_seen = excluded.last_seen,
			tombstone = excluded.tombstone,
			vector_clock = excluded.vector_clock,
			updated_at = CURRENT_TIMESTAMP
	`

	tombstoneInt := 0
	if tombstone {
		tombstoneInt = 1
	}

	_, err := s.db.Exec(query, serviceID, originPeerID, appData, confidence, lastSeen, tombstoneInt, vectorClock)
	if err != nil {
		return fmt.Errorf("failed to save federated service: %w", err)
	}

	return nil
}

// GetFederatedService retrieves a federated service by ID
func (s *Store) GetFederatedService(serviceID string) (*FederatedServiceData, error) {
	query := `
		SELECT service_id, origin_peer_id, app_data, confidence, last_seen, tombstone, vector_clock, created_at, updated_at
		FROM federation_services
		WHERE service_id = ?
	`

	var service FederatedServiceData
	var tombstoneInt int

	err := s.db.QueryRow(query, serviceID).Scan(
		&service.ServiceID,
		&service.OriginPeerID,
		&service.AppData,
		&service.Confidence,
		&service.LastSeen,
		&tombstoneInt,
		&service.VectorClock,
		&service.CreatedAt,
		&service.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get federated service: %w", err)
	}

	service.Tombstone = tombstoneInt == 1

	return &service, nil
}

// ListFederatedServices retrieves all federated services (including tombstones)
func (s *Store) ListFederatedServices() ([]*FederatedServiceData, error) {
	query := `
		SELECT service_id, origin_peer_id, app_data, confidence, last_seen, tombstone, vector_clock, created_at, updated_at
		FROM federation_services
		ORDER BY service_id ASC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list federated services: %w", err)
	}
	defer rows.Close()

	var services []*FederatedServiceData
	for rows.Next() {
		var service FederatedServiceData
		var tombstoneInt int

		err := rows.Scan(
			&service.ServiceID,
			&service.OriginPeerID,
			&service.AppData,
			&service.Confidence,
			&service.LastSeen,
			&tombstoneInt,
			&service.VectorClock,
			&service.CreatedAt,
			&service.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan federated service: %w", err)
		}

		service.Tombstone = tombstoneInt == 1
		services = append(services, &service)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating federated services: %w", err)
	}

	return services, nil
}

// ListActiveFederatedServices retrieves all non-tombstone federated services
func (s *Store) ListActiveFederatedServices() ([]*FederatedServiceData, error) {
	query := `
		SELECT service_id, origin_peer_id, app_data, confidence, last_seen, tombstone, vector_clock, created_at, updated_at
		FROM federation_services
		WHERE tombstone = 0
		ORDER BY service_id ASC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list active federated services: %w", err)
	}
	defer rows.Close()

	var services []*FederatedServiceData
	for rows.Next() {
		var service FederatedServiceData
		var tombstoneInt int

		err := rows.Scan(
			&service.ServiceID,
			&service.OriginPeerID,
			&service.AppData,
			&service.Confidence,
			&service.LastSeen,
			&tombstoneInt,
			&service.VectorClock,
			&service.CreatedAt,
			&service.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan federated service: %w", err)
		}

		service.Tombstone = tombstoneInt == 1
		services = append(services, &service)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating active federated services: %w", err)
	}

	return services, nil
}

// DeleteFederatedService removes a federated service from the database
func (s *Store) DeleteFederatedService(serviceID string) error {
	query := `DELETE FROM federation_services WHERE service_id = ?`

	_, err := s.db.Exec(query, serviceID)
	if err != nil {
		return fmt.Errorf("failed to delete federated service: %w", err)
	}

	// Deletion is idempotent - no error if service doesn't exist
	return nil
}

// System Secrets Management

// GetSystemSecret retrieves a system secret by key.
// Returns empty string and nil error if the secret doesn't exist.
func (s *Store) GetSystemSecret(key string) (string, error) {
	query := `SELECT value FROM system_secrets WHERE key = ?`

	var value string
	err := s.db.QueryRow(query, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil // Secret doesn't exist, not an error
	}
	if err != nil {
		return "", fmt.Errorf("failed to get system secret: %w", err)
	}

	return value, nil
}

// SetSystemSecret stores or updates a system secret.
func (s *Store) SetSystemSecret(key, value string) error {
	query := `
		INSERT INTO system_secrets (key, value, created_at, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = CURRENT_TIMESTAMP
	`

	_, err := s.db.Exec(query, key, value)
	if err != nil {
		return fmt.Errorf("failed to set system secret: %w", err)
	}

	return nil
}

// DeleteSystemSecret removes a system secret by key.
func (s *Store) DeleteSystemSecret(key string) error {
	query := `DELETE FROM system_secrets WHERE key = ?`

	_, err := s.db.Exec(query, key)
	if err != nil {
		return fmt.Errorf("failed to delete system secret: %w", err)
	}

	return nil
}
