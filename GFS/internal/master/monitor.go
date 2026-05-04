package master

import (
	"log"
	"strings"
	"time"
)

func (m *Master) monitorServerHealth() {
	ticker := time.NewTicker(time.Duration(m.Config.Health.CheckInterval) * time.Second)
	for range ticker.C {
		m.serversMu.Lock()
		for serverId, server := range m.servers {
			server.mu.Lock()
			timeSinceLastHeartbeat := time.Since(server.LastHeartbeat)
			if timeSinceLastHeartbeat > time.Duration(m.Config.Health.Timeout)*time.Second {
				if server.Status == "ACTIVE" {
					server.Status = "INACTIVE"
					server.FailureCount++
				} else if server.Status == "INACTIVE" {
					server.FailureCount++
					if server.FailureCount >= m.Config.Health.MaxFailures {
						server.Status = "DEAD"
						go m.handleServerFailure(serverId)
					}
				}
			}
			server.mu.Unlock()
		}
		m.serversMu.Unlock()
	}
}

func (m *Master) monitorChunkReplication() {
	ticker := time.NewTicker(120 * time.Second)
	for range ticker.C {
		m.chunksMu.RLock()
		for chunkHandle, chunkInfo := range m.chunks {
			chunkInfo.mu.RLock()
			if len(chunkInfo.Locations) < m.Config.Replication.Factor {
				go m.initiateReplication(chunkHandle)
			}
			chunkInfo.mu.RUnlock()
		}
		m.chunksMu.RUnlock()
	}
}

// monitorOrphanedFiles periodically detects files whose chunks have no live
// replicas on any active chunkserver and removes them from master metadata.
//
// Design:
//   - Initial sleep of 2×health.check_interval gives chunkservers time to
//     reconnect and report their chunks after a master restart.
//   - A file must appear location-less in TWO consecutive scans before it is
//     removed (two-scan confirmation), guarding against transient zero-location
//     states during normal operation.
//   - The scan is skipped entirely when no chunkserver is active, so a master
//     that starts before any chunkserver doesn't wipe its metadata.
func (m *Master) monitorOrphanedFiles() {
	stabilization := time.Duration(m.Config.Health.CheckInterval*2) * time.Second
	time.Sleep(stabilization)
	log.Printf("[OrphanMonitor] Active after %v stabilization period", stabilization)

	// prevCandidates: filenames flagged as location-less in the previous scan.
	prevCandidates := make(map[string]bool)

	ticker := time.NewTicker(time.Duration(m.Config.Health.CheckInterval) * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		// Only run when at least one chunkserver is actively heartbeating.
		m.serversMu.RLock()
		activeServers := 0
		for _, srv := range m.servers {
			srv.mu.RLock()
			if srv.Status == "ACTIVE" {
				activeServers++
			}
			srv.mu.RUnlock()
		}
		m.serversMu.RUnlock()

		if activeServers == 0 {
			prevCandidates = make(map[string]bool)
			continue
		}

		// Scan: files whose every chunk has zero locations.
		currentCandidates := m.findLocationlessFiles()

		// Confirm: only act on files flagged in both this scan and the previous one.
		var confirmed []string
		for name := range currentCandidates {
			if prevCandidates[name] {
				confirmed = append(confirmed, name)
			}
		}
		prevCandidates = currentCandidates

		if len(confirmed) == 0 {
			continue
		}

		log.Printf("[OrphanMonitor] %d confirmed orphaned file(s): %v", len(confirmed), confirmed)
		m.cleanupDanglingFileMetadata()
	}
}

// findLocationlessFiles returns the set of non-trash filenames whose every
// chunk currently has zero locations in the master's in-memory map.
func (m *Master) findLocationlessFiles() map[string]bool {
	result := make(map[string]bool)

	m.filesMu.RLock()
	defer m.filesMu.RUnlock()
	m.chunksMu.RLock()
	defer m.chunksMu.RUnlock()

	for filename, fileInfo := range m.files {
		if strings.HasPrefix(filename, m.Config.Deletion.TrashDirPrefix) {
			continue
		}
		if fileInfo == nil {
			result[filename] = true
			continue
		}
		fileInfo.mu.RLock()
		if len(fileInfo.Chunks) == 0 {
			fileInfo.mu.RUnlock()
			continue // empty file — valid, keep it
		}
		hasLive := false
		for _, h := range fileInfo.Chunks {
			chunk := m.chunks[h]
			if chunk == nil {
				continue
			}
			chunk.mu.RLock()
			hasLive = len(chunk.Locations) > 0
			chunk.mu.RUnlock()
			if hasLive {
				break
			}
		}
		fileInfo.mu.RUnlock()
		if !hasLive {
			result[filename] = true
		}
	}
	return result
}

func (m *Master) cleanupExpiredLeases() {
	ticker := time.NewTicker(time.Duration(m.Config.Health.CheckInterval) * time.Second)
	for range ticker.C {
		m.chunksMu.Lock()
		now := time.Now()
		for _, chunkInfo := range m.chunks {
			chunkInfo.mu.Lock()
			if !chunkInfo.LeaseExpiration.IsZero() && now.After(chunkInfo.LeaseExpiration) {
				chunkInfo.Primary = ""
				chunkInfo.LeaseExpiration = time.Time{}
			}
			chunkInfo.mu.Unlock()
		}
		m.chunksMu.Unlock()
	}
}
