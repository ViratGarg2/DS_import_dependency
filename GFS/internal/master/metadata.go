package master

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
)

func (m *Master) LoadMetadata(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		log.Printf("Metadata file doesn't exist, starting with empty state.")
		// Initialize empty maps if metadata file doesn't exist
		m.filesMu.Lock()
		m.chunksMu.Lock()
		if m.files == nil {
			m.files = make(map[string]*FileInfo)
		}
		if m.chunks == nil {
			m.chunks = make(map[string]*ChunkInfo)
		}
		m.filesMu.Unlock()
		m.chunksMu.Unlock()
	} else if len(data) > 0 {
		var metadata struct {
			Files  map[string]*FileInfo  `json:"files"`
			Chunks map[string]*ChunkInfo `json:"chunks"`
		}

		if err := json.Unmarshal(data, &metadata); err != nil {
			return err
		}

		m.filesMu.Lock()
		m.chunksMu.Lock()

		// Initialize maps if they're nil
		if m.files == nil {
			m.files = make(map[string]*FileInfo)
		}
		if m.chunks == nil {
			m.chunks = make(map[string]*ChunkInfo)
		}

		// Copy files data directly
		m.files = metadata.Files

		// Convert stored chunk info to full chunk info
		for chunkID, storedInfo := range metadata.Chunks {
			m.chunks[chunkID] = &ChunkInfo{
				Size:            storedInfo.Size,
				Version:         storedInfo.Version,
				Locations:       make(map[string]bool),
				ServerAddresses: make(map[string]string),
				StaleReplicas:   make(map[string]bool),
			}
		}

		m.filesMu.Unlock()
		m.chunksMu.Unlock()
	}

	return m.replayOperationLog()
}

func (m *Master) checkpointMetadata() error {
	// Snapshot phase: hold filesMu + chunksMu read locks while serialising.
	// Any handler that is mid-flight (holding the write lock) will finish before
	// we read.  Once we release these read locks, handlers can run again.
	m.filesMu.RLock()
	m.chunksMu.RLock()

	storedChunks := make(map[string]*ChunkInfo)
	for chunkID, chunk := range m.chunks {
		storedChunks[chunkID] = &ChunkInfo{
			Size:    chunk.Size,
			Version: chunk.Version,
		}
	}

	metadata := struct {
		Files  map[string]*FileInfo  `json:"files"`
		Chunks map[string]*ChunkInfo `json:"chunks"`
	}{
		Files:  m.files,
		Chunks: storedChunks,
	}

	data, err := json.MarshalIndent(metadata, "", "  ")
	m.filesMu.RUnlock()
	m.chunksMu.RUnlock()

	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %v", err)
	}

	// Write checkpoint to disk atomically via temp-file rename.
	tempFile := m.Config.Metadata.Database.Path + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temporary metadata file: %v", err)
	}

	if err := os.Rename(tempFile, m.Config.Metadata.Database.Path); err != nil {
		return fmt.Errorf("failed to rename metadata file: %v", err)
	}

	// Truncate the log now that the checkpoint is on disk.
	// We hold opLog.mu only for this brief critical section.
	// NOTE: there is a small race window between releasing filesMu above and
	// acquiring opLog.mu here.  An operation that logs in that window will have
	// its entry truncated even though it is not in the just-written checkpoint.
	// The replay code is idempotent (see replayOperationLog), so re-applying
	// an entry that the checkpoint already captured is safe, but an entry that
	// falls into this window will be silently lost on restart.  Eliminating
	// this race requires restructuring handlers so they do not hold filesMu
	// while logging (enabling opLog.mu to be acquired first in both paths).
	m.opLog.mu.Lock()
	defer m.opLog.mu.Unlock()

	if err := m.opLog.logFile.Truncate(0); err != nil {
		return fmt.Errorf("failed to truncate operation log: %v", err)
	}

	if _, err := m.opLog.logFile.Seek(0, 0); err != nil {
		return fmt.Errorf("failed to reset operation log position: %v", err)
	}

	m.opLog.writer = bufio.NewWriter(m.opLog.logFile)

	return nil
}
