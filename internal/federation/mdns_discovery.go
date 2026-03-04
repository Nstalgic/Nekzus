package federation

import (
	"fmt"
	"strconv"
	"strings"
)

// MDNSPeerInfo represents peer information extracted from mDNS TXT records
type MDNSPeerInfo struct {
	PeerID     string
	PeerName   string
	GossipPort int
	APIAddress string
}

// buildPeerTXTRecord creates TXT record fields for mDNS peer advertisement
func buildPeerTXTRecord(config Config) []string {
	return []string{
		fmt.Sprintf("peer_id=%s", config.LocalPeerID),
		fmt.Sprintf("peer_name=%s", config.LocalPeerName),
		fmt.Sprintf("gossip_port=%d", config.GossipAdvertisePort),
		fmt.Sprintf("api_address=%s", config.APIAddress),
	}
}

// parsePeerTXTRecord extracts peer info from mDNS TXT record fields
func parsePeerTXTRecord(txtFields []string) MDNSPeerInfo {
	info := MDNSPeerInfo{}

	for _, field := range txtFields {
		parts := strings.SplitN(field, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key, value := parts[0], parts[1]
		if value == "" {
			continue
		}

		switch key {
		case "peer_id":
			info.PeerID = value
		case "peer_name":
			info.PeerName = value
		case "gossip_port":
			if port, err := strconv.Atoi(value); err == nil {
				info.GossipPort = port
			}
		case "api_address":
			info.APIAddress = value
		}
	}

	return info
}

// buildGossipAddress creates a gossip address from host and port
func buildGossipAddress(host string, port int) string {
	// Check if IPv6
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		return fmt.Sprintf("[%s]:%d", host, port)
	}
	return fmt.Sprintf("%s:%d", host, port)
}

// shouldConnectToPeer determines if we should connect to a discovered peer
func shouldConnectToPeer(localPeerID, remotePeerID string) bool {
	// Don't connect to self
	if remotePeerID == "" || remotePeerID == localPeerID {
		return false
	}
	return true
}
