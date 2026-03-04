package discovery

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/nstalgic/nekzus/internal/types"
)

var mdnsLog = slog.With("package", "discovery", "component", "mdns")

// mDNS Discovery Worker

// MDNSDiscoveryWorker discovers services via mDNS/Bonjour/Zeroconf.
type MDNSDiscoveryWorker struct {
	dm            *DiscoveryManager
	services      []string      // Service types to discover (e.g., "_http._tcp")
	domain        string        // mDNS domain (usually ".local")
	scanInterval  time.Duration // How often to scan
	debouncer     *debouncer
	knownServices map[string]bool
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
}

// MDNSService represents a discovered mDNS service.
type MDNSService struct {
	Name   string
	Type   string
	Domain string
	Host   string
	Port   int
	TXT    map[string]string
	IPv4   []net.IP
	IPv6   []net.IP
}

// NewMDNSDiscoveryWorker creates an mDNS discovery worker.
func NewMDNSDiscoveryWorker(dm *DiscoveryManager, services []string, scanInterval time.Duration) *MDNSDiscoveryWorker {
	ctx, cancel := context.WithCancel(context.Background())

	if len(services) == 0 {
		// Default service types to discover
		services = []string{
			"_http._tcp",
			"_https._tcp",
			"_ssh._tcp",
			"_smb._tcp",
			"_printer._tcp",
			"_workstation._tcp",
		}
	}

	return &MDNSDiscoveryWorker{
		dm:            dm,
		services:      services,
		domain:        "local.",
		scanInterval:  scanInterval,
		debouncer:     newDebouncer(60 * time.Second),
		knownServices: make(map[string]bool),
		ctx:           ctx,
		cancel:        cancel,
	}
}

// Name returns the worker identifier.
func (m *MDNSDiscoveryWorker) Name() string {
	return "mdns"
}

// Start begins mDNS service discovery.
// Note: mDNS discovery is not fully implemented. This worker will start but
// will not discover any services until a proper mDNS library is integrated.
func (m *MDNSDiscoveryWorker) Start(ctx context.Context) error {
	// Log that mDNS is not implemented
	mdnsLog.Info("worker started - not fully implemented, no services will be discovered")
	mdnsLog.Info("to implement, add github.com/hashicorp/mdns or github.com/grandcat/zeroconf")

	// Block until context is cancelled - do not process mock data
	select {
	case <-ctx.Done():
		return nil
	case <-m.ctx.Done():
		return nil
	}
}

// Stop gracefully shuts down the worker.
func (m *MDNSDiscoveryWorker) Stop() error {
	m.cancel()
	return nil
}

// scan performs mDNS discovery for configured service types.
// Note: Placeholder - real implementation requires mDNS library
// (github.com/hashicorp/mdns or github.com/grandcat/zeroconf).
func (m *MDNSDiscoveryWorker) scan() error {

	// Do not process mock services - mDNS is not implemented
	return nil
}

// processService converts an mDNS service to a proposal.
func (m *MDNSDiscoveryWorker) processService(svc *MDNSService) {
	// Determine if this is a web service we can proxy
	if !m.isWebService(svc.Type) {
		return
	}

	// Get primary IP address
	host := m.getPrimaryIP(svc)
	if host == "" {
		host = svc.Host
	}

	// Determine scheme
	scheme := m.getScheme(svc)

	// Generate proposal ID
	proposalID := generateProposalID("mdns", scheme, svc.Host, svc.Port)

	// Debounce
	if !m.debouncer.ShouldProcess(proposalID) {
		return
	}

	// Extract metadata from TXT records
	appID := m.getTXTValue(svc, "app_id", sanitize(svc.Name))
	appName := m.getTXTValue(svc, "app_name", svc.Name)
	pathBase := m.getTXTValue(svc, "path", fmt.Sprintf("/apps/%s/", appID))

	// Build proposal
	proposal := &types.Proposal{
		ID:             proposalID,
		Source:         "mdns",
		DetectedScheme: scheme,
		DetectedHost:   host,
		DetectedPort:   svc.Port,
		Confidence:     m.calculateConfidence(svc),
		SuggestedApp: types.App{
			ID:   appID,
			Name: appName,
			Icon: m.getTXTValue(svc, "icon", ""),
			Tags: m.getTags(svc),
			Endpoints: map[string]string{
				"lan":  fmt.Sprintf("%s://%s:%d", scheme, host, svc.Port),
				"mdns": fmt.Sprintf("%s://%s:%d", scheme, svc.Host, svc.Port),
			},
		},
		SuggestedRoute: types.Route{
			RouteID:  fmt.Sprintf("route:%s", appID),
			AppID:    appID,
			PathBase: pathBase,
			To:       fmt.Sprintf("%s://%s:%d", scheme, host, svc.Port),
			Scopes:   m.getScopes(svc),
		},
		SecurityNotes: m.getSecurityNotes(scheme, svc),
	}

	m.dm.SubmitProposal(proposal)
}

// isWebService determines if a service type is web-accessible.
func (m *MDNSDiscoveryWorker) isWebService(serviceType string) bool {
	webTypes := []string{
		"_http._tcp",
		"_https._tcp",
		"_homeassistant._tcp",
		"_hap._tcp", // HomeKit
	}

	for _, t := range webTypes {
		if strings.Contains(serviceType, t) {
			return true
		}
	}
	return false
}

// getPrimaryIP extracts the primary IPv4 address.
func (m *MDNSDiscoveryWorker) getPrimaryIP(svc *MDNSService) string {
	if len(svc.IPv4) > 0 {
		return svc.IPv4[0].String()
	}
	if len(svc.IPv6) > 0 {
		return fmt.Sprintf("[%s]", svc.IPv6[0].String())
	}
	return ""
}

// getScheme determines HTTP vs HTTPS.
func (m *MDNSDiscoveryWorker) getScheme(svc *MDNSService) string {
	if strings.Contains(svc.Type, "_https") {
		return "https"
	}
	if svc.Port == 443 || svc.Port == 8443 {
		return "https"
	}
	// Check TXT record
	if scheme := m.getTXTValue(svc, "scheme", ""); scheme != "" {
		return scheme
	}
	return "http"
}

// getTXTValue retrieves a value from TXT records or returns default.
func (m *MDNSDiscoveryWorker) getTXTValue(svc *MDNSService, key, defaultValue string) string {
	if val, ok := svc.TXT[key]; ok && val != "" {
		return val
	}
	return defaultValue
}

// getTags extracts tags from TXT records.
func (m *MDNSDiscoveryWorker) getTags(svc *MDNSService) []string {
	if tags := svc.TXT["tags"]; tags != "" {
		return strings.Split(tags, ",")
	}

	// Auto-tag based on service type
	tags := []string{"mdns"}
	if strings.Contains(svc.Type, "_homeassistant") {
		tags = append(tags, "homeautomation")
	} else if strings.Contains(svc.Type, "_printer") {
		tags = append(tags, "printer")
	}

	return tags
}

// getScopes extracts required scopes from TXT records.
func (m *MDNSDiscoveryWorker) getScopes(svc *MDNSService) []string {
	if scopes := svc.TXT["scopes"]; scopes != "" {
		return strings.Split(scopes, ",")
	}

	// Default scope
	appID := m.getTXTValue(svc, "app_id", sanitize(svc.Name))
	return []string{fmt.Sprintf("access:%s", appID)}
}

// calculateConfidence determines confidence score.
func (m *MDNSDiscoveryWorker) calculateConfidence(svc *MDNSService) float64 {
	confidence := 0.7 // Base confidence for mDNS

	// Higher if explicitly configured with Nekzus metadata
	if svc.TXT["nekzus_enable"] == "true" {
		confidence = 0.95
	} else if svc.TXT["app_id"] != "" {
		confidence = 0.85
	}

	// Known service types get higher confidence
	knownServices := map[string]bool{
		"_homeassistant._tcp": true,
		"_hap._tcp":           true,
	}
	if knownServices[svc.Type] {
		confidence += 0.1
	}

	if confidence > 1.0 {
		confidence = 1.0
	}

	return confidence
}

// getSecurityNotes generates security warnings.
func (m *MDNSDiscoveryWorker) getSecurityNotes(scheme string, svc *MDNSService) []string {
	notes := []string{"Discovered via mDNS", "Local network only", "JWT required"}

	if scheme == "http" {
		notes = append(notes, "Upstream uses HTTP (unencrypted)")
	}

	return notes
}

// Real mDNS implementation example (requires github.com/hashicorp/mdns):
/*
func (m *MDNSDiscoveryWorker) scanWithMDNS() error {
	import "github.com/hashicorp/mdns"

	for _, serviceType := range m.services {
		entriesCh := make(chan *mdns.ServiceEntry, 100)

		params := &mdns.QueryParam{
			Service: serviceType,
			Domain:  m.domain,
			Timeout: 5 * time.Second,
			Entries: entriesCh,
		}

		go func() {
			if err := mdns.Query(params); err != nil {
				mdnsLog.Error("mdns query failed",
					"service_type", serviceType,
					"error", err)
			}
			close(entriesCh)
		}()

		for entry := range entriesCh {
			txtMap := make(map[string]string)
			for _, txt := range entry.InfoFields {
				parts := strings.SplitN(txt, "=", 2)
				if len(parts) == 2 {
					txtMap[parts[0]] = parts[1]
				}
			}

			svc := &MDNSService{
				Name:   entry.Name,
				Type:   serviceType,
				Domain: m.domain,
				Host:   entry.Host,
				Port:   entry.Port,
				TXT:    txtMap,
				IPv4:   []net.IP{entry.AddrV4},
				IPv6:   []net.IP{entry.AddrV6},
			}

			m.processService(svc)
		}
	}

	return nil
}
*/
