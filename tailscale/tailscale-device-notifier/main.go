package main

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/charmbracelet/log"
	"tailscale.com/client/local"
	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tailcfg"
)

type DeviceMonitor struct {
	client        *local.Client
	lastStates    map[tailcfg.StableNodeID]bool
	lastSelfState bool // Track self online status
	pollInterval  time.Duration
	logger        *log.Logger
}

func NewDeviceMonitor() *DeviceMonitor {
	// Create a structured logger with nice formatting
	logger := log.New(os.Stdout)
	logger.SetLevel(log.InfoLevel)
	logger.SetReportTimestamp(true)
	logger.SetTimeFormat("15:04:05")

	return &DeviceMonitor{
		client:       &local.Client{},
		lastStates:   make(map[tailcfg.StableNodeID]bool),
		pollInterval: 5 * time.Second,
		logger:       logger,
	}
}

func (dm *DeviceMonitor) fetchStatus(ctx context.Context) (*ipnstate.Status, error) {
	status, err := dm.client.Status(ctx)
	if err != nil {
		return nil, err
	}
	return status, nil
}

func (dm *DeviceMonitor) isNodeOnline(peer *ipnstate.PeerStatus) bool {
	// A peer is considered online if:
	// 1. It has been seen recently (within last 2 minutes)
	// 2. It's currently active or has recent activity
	// 3. It's marked as Online by the control plane

	// If the peer is explicitly marked as online, use that
	if peer.Online {
		return true
	}

	// Consider online if last seen within 2 minutes
	return time.Since(peer.LastSeen) < 2*time.Minute
}

func (dm *DeviceMonitor) getOnlineReason(peer *ipnstate.PeerStatus) string {
	if peer.Online {
		return "marked as online by control plane"
	}
	if time.Since(peer.LastSeen) < 2*time.Minute {
		return "seen recently (within 2 minutes)"
	}
	return "unknown"
}

func (dm *DeviceMonitor) getOfflineReason(peer *ipnstate.PeerStatus) string {
	if !peer.Online && time.Since(peer.LastSeen) >= 2*time.Minute {
		return "not seen recently and marked offline"
	}
	if !peer.Online {
		return "marked as offline by control plane"
	}
	if time.Since(peer.LastSeen) >= 2*time.Minute {
		return "not seen recently"
	}
	return "unknown"
}

func (dm *DeviceMonitor) checkDeviceChanges(status *ipnstate.Status) {
	currentStates := make(map[tailcfg.StableNodeID]bool)

	// Check self status
	selfOnline := status.BackendState == ipn.Running.String()

	// Only log self status if it changed
	if dm.lastSelfState != selfOnline {
		if selfOnline {
			dm.logger.Info("Self came ONLINE",
				"dns_name", status.Self.DNSName)
		} else {
			dm.logger.Info("Self went OFFLINE",
				"dns_name", status.Self.DNSName)
		}
		dm.lastSelfState = selfOnline
	}

	// Check all peers
	for _, peer := range status.Peer {
		isOnline := dm.isNodeOnline(peer)
		currentStates[peer.ID] = isOnline

		// Check if this is a new device or if status changed
		lastStatus, existed := dm.lastStates[peer.ID]

		if !existed {
			// New device discovered
			statusStr := "offline"
			if isOnline {
				statusStr = "online"
			}
			dm.logger.Info("New device discovered",
				"dns_name", peer.DNSName,
				"hostname", peer.HostName,
				"os", peer.OS,
				"status", statusStr)

			// Print full device JSON for new devices
			if deviceJSON, err := json.MarshalIndent(peer, "", "  "); err == nil {
				dm.logger.Info("Full device JSON",
					"dns_name", peer.DNSName,
					"device_json", string(deviceJSON))
			}
		} else if lastStatus != isOnline {
			// Status changed
			if isOnline {
				reason := dm.getOnlineReason(peer)
				dm.logger.Info("Device came ONLINE",
					"dns_name", peer.DNSName,
					"hostname", peer.HostName,
					"os", peer.OS,
					"ip", peer.TailscaleIPs[0].String(),
					"reason", reason)
			} else {
				reason := dm.getOfflineReason(peer)
				lastSeenStr := peer.LastSeen.Format("15:04:05")
				dm.logger.Info("Device went OFFLINE",
					"dns_name", peer.DNSName,
					"hostname", peer.HostName,
					"os", peer.OS,
					"last_seen", lastSeenStr,
					"reason", reason)
			}

			// Print full device JSON for status changes
			if deviceJSON, err := json.MarshalIndent(peer, "", "  "); err == nil {
				dm.logger.Info("Full device JSON",
					"dns_name", peer.DNSName,
					"device_json", string(deviceJSON))
			}
		}
	}

	// Check for devices that are no longer in the peer list
	for nodeID, wasOnline := range dm.lastStates {
		if _, exists := currentStates[nodeID]; !exists && wasOnline {
			dm.logger.Info("Device removed from network",
				"node_id", nodeID)
		}
	}

	dm.lastStates = currentStates
}

func (dm *DeviceMonitor) printInitialStatus(status *ipnstate.Status) {
	dm.logger.Info("🚀 Tailscale Status",
		"backend_state", status.BackendState)
	dm.logger.Info("🏠 Tailnet Information",
		"magic_dns_suffix", status.CurrentTailnet.MagicDNSSuffix,
		"peer_count", len(status.Peer))

	// Print self info and initialize self state
	selfOnline := status.BackendState == ipn.Running.String()
	dm.lastSelfState = selfOnline
	dm.logger.Info("📱 Self Device Status",
		"dns_name", status.Self.DNSName,
		"hostname", status.Self.HostName,
		"os", status.Self.OS,
		"ip", status.Self.TailscaleIPs[0].String(),
		"status", map[bool]string{true: "online", false: "offline"}[selfOnline])

	// Initialize states without logging changes
	for _, peer := range status.Peer {
		isOnline := dm.isNodeOnline(peer)
		dm.lastStates[peer.ID] = isOnline

		onlineStatus := "offline"
		if isOnline {
			onlineStatus = "online"
		}

		lastSeenStr := peer.LastSeen.Format("15:04:05")

		dm.logger.Info("Initial Peer Status",
			"dns_name", peer.DNSName,
			"hostname", peer.HostName,
			"os", peer.OS,
			"status", onlineStatus,
			"last_seen", lastSeenStr)
	}
}

func (dm *DeviceMonitor) Start(ctx context.Context) error {
	dm.logger.Info("Starting Tailscale device monitor",
		"poll_interval", dm.pollInterval)

	// Initial fetch to populate baseline
	status, err := dm.fetchStatus(ctx)
	if err != nil {
		return err
	}

	dm.printInitialStatus(status)

	ticker := time.NewTicker(dm.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			dm.logger.Info("Stopping device monitor")
			return ctx.Err()
		case <-ticker.C:
			status, err := dm.fetchStatus(ctx)
			if err != nil {
				dm.logger.Error("Error fetching status",
					"error", err)
				continue
			}

			dm.checkDeviceChanges(status)
		}
	}
}

func main() {
	monitor := NewDeviceMonitor()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := monitor.Start(ctx); err != nil && err != context.Canceled {
		monitor.logger.Error("Monitor failed",
			"error", err)
		os.Exit(1)
	}
}
