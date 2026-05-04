package client

import (
	"context"
	"fmt"
	"hash/crc32"
	"strings"
	"time"

	chunk_ops "github.com/Mit-Vin/GFS-Distributed-Systems/api/proto/chunk_operations"
	common_pb "github.com/Mit-Vin/GFS-Distributed-Systems/api/proto/common"

	"github.com/google/uuid"
	"google.golang.org/grpc"
)

const opMaxRetries = 3

const ChunkSize = 1 * 1024 * 1024 // 1MB

func (c *Client) PushDataToPrimary(ctx context.Context, chunkHandle string, data []byte) (string, error) {
	// Get chunk information from cache
	c.chunkCacheMu.RLock()
	chunkInfo, exists := c.chunkHandleCache[chunkHandle]
	c.chunkCacheMu.RUnlock()

	if !exists {
		return "", fmt.Errorf("chunk information not found in cache for handle, request again: %s", chunkHandle)
	}

	if chunkInfo.PrimaryLocation == nil {
		return "", fmt.Errorf("primary location not found for chunk: %s", chunkHandle)
	}

	conn, err := grpc.Dial(chunkInfo.PrimaryLocation.ServerAddress, grpc.WithInsecure())
	if err != nil {
		return "", fmt.Errorf("failed to connect to primary server: %v", err)
	}
	defer conn.Close()

	client := chunk_ops.NewChunkOperationServiceClient(conn)

	checksum := crc32.ChecksumIEEE(data)
	operationId := uuid.New().String() // Generate unique operation ID

	req := &chunk_ops.PushDataToPrimaryRequest{
		ChunkHandle: &common_pb.ChunkHandle{
			Handle: chunkHandle,
		},
		Data:               data,
		Checksum:           checksum,
		OperationId:        operationId,
		SecondaryLocations: chunkInfo.SecondaryLocations,
	}

	resp, err := client.PushDataToPrimary(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to push data: %v", err)
	}

	if resp.Status.Code != common_pb.Status_OK {
		return "", fmt.Errorf("push data failed: %s", resp.Status.Message)
	}

	return operationId, nil
}

func (c *Client) Read(ctx context.Context, filename string, offset int64, length int64) ([]byte, error) {
	if length <= 0 {
		return nil, fmt.Errorf("invalid read length: %d", length)
	}

	startChunk := offset / ChunkSize
	endChunk := (offset + length - 1) / ChunkSize

	chunks, err := c.GetChunkInfo(ctx, filename, startChunk, endChunk)
	if err != nil {
		return nil, fmt.Errorf("failed to get chunk info: %v", err)
	}

	result := make([]byte, 0, length)
	remainingLength := length
	currentOffset := offset

	for i := startChunk; i <= endChunk; i++ {
		chunkInfo, ok := chunks[i]
		if !ok {
			return nil, fmt.Errorf("chunk info missing for index %d", i)
		}

		chunkOffset := currentOffset % ChunkSize
		bytesToRead := ChunkSize - chunkOffset
		if bytesToRead > remainingLength {
			bytesToRead = remainingLength
		}

		var serverAddr string
		if chunkInfo.PrimaryLocation != nil {
			serverAddr = chunkInfo.PrimaryLocation.ServerAddress
		} else if len(chunkInfo.SecondaryLocations) > 0 {
			serverAddr = chunkInfo.SecondaryLocations[0].ServerAddress
		} else {
			return nil, fmt.Errorf("no available servers for chunk %s", chunkInfo.ChunkHandle.Handle)
		}

		opCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		conn, connErr := grpc.Dial(serverAddr, grpc.WithInsecure())
		if connErr != nil {
			cancel()
			return nil, fmt.Errorf("failed to connect to chunk server: %v", connErr)
		}

		readResp, readErr := chunk_ops.NewChunkOperationServiceClient(conn).ReadChunk(opCtx,
			&chunk_ops.ReadChunkRequest{
				ChunkHandle: chunkInfo.ChunkHandle,
				Offset:      chunkOffset,
				Length:      bytesToRead,
			})
		conn.Close()
		cancel()

		if readErr != nil {
			return nil, fmt.Errorf("failed to read from chunk %s: %v",
				chunkInfo.ChunkHandle.Handle, readErr)
		}
		if readResp.Status.Code != common_pb.Status_OK {
			// Chunk written with fewer bytes than expected — treat as EOF.
			if strings.Contains(readResp.Status.Message, "offset is beyond chunk size") {
				break
			}
			return nil, fmt.Errorf("read chunk failed: %s", readResp.Status.Message)
		}

		result = append(result, readResp.Data...)
		bytesRead := int64(len(readResp.Data))
		remainingLength -= bytesRead
		currentOffset += bytesRead
		if remainingLength <= 0 {
			break
		}
	}

	return result, nil
}

func (c *Client) Write(ctx context.Context, filename string, offset int64, data []byte) (int, error) {
	startChunk := offset / ChunkSize
	endChunk := (offset + int64(len(data))) / ChunkSize
	chunks, err := c.GetChunkInfo(ctx, filename, startChunk, endChunk)
	if err != nil {
		return 0, fmt.Errorf("failed to get chunk info: %v", err)
	}

	totalWritten := 0
	remainingData := data
	currentOffset := offset

	for i := startChunk; i <= endChunk; i++ {
		chunkInfo, ok := chunks[i]
		if !ok {
			return totalWritten, fmt.Errorf("chunk info missing for index %d", i)
		}

		chunkOffset := currentOffset % ChunkSize
		var chunkData []byte
		if bytesRem := ChunkSize - chunkOffset; int64(len(remainingData)) > bytesRem {
			chunkData = remainingData[:bytesRem]
			remainingData = remainingData[bytesRem:]
		} else {
			chunkData = remainingData
			remainingData = nil
		}

		// chunkErr is always set before a continue so it is non-nil when the
		// loop exhausts all retries via the "not primary" path.
		var chunkErr error
		for attempt := 0; attempt < opMaxRetries; attempt++ {
			opCtx, cancel := context.WithTimeout(ctx, 30*time.Second)

			operationId, pushErr := c.PushDataToPrimary(opCtx, chunkInfo.ChunkHandle.Handle, chunkData)
			if pushErr != nil {
				cancel()
				chunkErr = fmt.Errorf("push chunk %d: %v", i, pushErr)
				if strings.Contains(pushErr.Error(), "not primary") {
					// The master assigned a primary but the BECOME_PRIMARY command
					// travels async through the ReportChunk stream.  Sleep briefly so
					// the chunk server has time to process it before we retry.
					time.Sleep(300 * time.Millisecond)
					c.invalidateChunkEntry(filename, i)
					fetchCtx, fetchCancel := context.WithTimeout(ctx, 10*time.Second)
					if fresh, ferr := c.GetChunkInfo(fetchCtx, filename, i, i); ferr == nil {
						if fi, fiOk := fresh[i]; fiOk {
							chunkInfo, chunks[i] = fi, fi
						}
					}
					fetchCancel()
					continue // chunkErr already set; loop retries
				}
				break // non-retryable
			}

			conn, cerr := grpc.Dial(chunkInfo.PrimaryLocation.ServerAddress, grpc.WithInsecure())
			if cerr != nil {
				cancel()
				chunkErr = fmt.Errorf("dial primary chunk %d: %v", i, cerr)
				break
			}

			writeResp, werr := chunk_ops.NewChunkOperationServiceClient(conn).WriteChunk(opCtx,
				&chunk_ops.WriteChunkRequest{
					ChunkHandle: chunkInfo.ChunkHandle,
					Offset:      chunkOffset,
					Secondaries: chunkInfo.SecondaryLocations,
					OperationId: operationId,
				})
			conn.Close()
			cancel()

			if werr != nil {
				chunkErr = fmt.Errorf("write chunk %d: %v", i, werr)
				break
			}
			if writeResp.Status.Code != common_pb.Status_OK {
				chunkErr = fmt.Errorf("write chunk %d failed: %s", i, writeResp.Status.Message)
				if strings.Contains(writeResp.Status.Message, "not primary") {
					time.Sleep(300 * time.Millisecond)
					c.invalidateChunkEntry(filename, i)
					fetchCtx, fetchCancel := context.WithTimeout(ctx, 10*time.Second)
					if fresh, ferr := c.GetChunkInfo(fetchCtx, filename, i, i); ferr == nil {
						if fi, fiOk := fresh[i]; fiOk {
							chunkInfo, chunks[i] = fi, fi
						}
					}
					fetchCancel()
					continue // retry
				}
				break // non-retryable write failure
			}

			chunkErr = nil
			break // success
		}

		if chunkErr != nil {
			return totalWritten, chunkErr
		}
		totalWritten += len(chunkData)
		currentOffset += int64(len(chunkData))
		if len(remainingData) == 0 {
			break
		}
	}

	return totalWritten, nil
}

func (c *Client) Append(ctx context.Context, filename string, data []byte) (int64, string, error) {
	if int64(len(data)) >= ChunkSize/4 {
		return -1, "", fmt.Errorf("data (size: %v) should be less than 1/4th of chunkSize (%v)", int64(len(data)), ChunkSize)
	}

	// maxRollovers bounds how many full-chunk rollovers we allow in one Append call.
	// Each rollover means the current chunk was full and we advanced to the next one.
	const maxRollovers = 64
	primaryMisses := 0

	for rollover := 0; rollover <= maxRollovers; rollover++ {
		fetchCtx, fetchCancel := context.WithTimeout(ctx, 10*time.Second)
		chunkInfo, chunkIdx, err := c.GetLastChunkInfo(fetchCtx, filename)
		fetchCancel()
		if err != nil {
			return -1, "", fmt.Errorf("failed to get chunk info: %v", err)
		}

		opCtx, cancel := context.WithTimeout(ctx, 60*time.Second)

		operationId, pushErr := c.PushDataToPrimary(opCtx, chunkInfo.ChunkHandle.Handle, data)
		if pushErr != nil {
			cancel()
			if strings.Contains(pushErr.Error(), "not primary") {
				primaryMisses++
				if primaryMisses >= opMaxRetries {
					return -1, "", fmt.Errorf("push append (not primary after %d retries): %v", opMaxRetries, pushErr)
				}
				time.Sleep(300 * time.Millisecond)
				c.invalidateLastChunkEntry(filename)
				continue
			}
			return -1, "", fmt.Errorf("push append data: %v", pushErr)
		}
		primaryMisses = 0 // reset on successful push

		conn, connErr := grpc.Dial(chunkInfo.PrimaryLocation.ServerAddress, grpc.WithInsecure())
		if connErr != nil {
			cancel()
			return -1, "", fmt.Errorf("dial primary: %v", connErr)
		}

		idempID := uuid.New().String()
		appendResp, appendErr := chunk_ops.NewChunkOperationServiceClient(conn).RecordAppendChunk(opCtx,
			&chunk_ops.RecordAppendChunkRequest{
				ChunkHandle:      chunkInfo.ChunkHandle,
				Secondaries:      chunkInfo.SecondaryLocations,
				OperationId:      operationId,
				IdempotentencyId: idempID,
			})
		conn.Close()
		cancel()

		if appendErr != nil {
			return -1, "", fmt.Errorf("append RPC: %v; idempotency ID: %v", appendErr, idempID)
		}

		if appendResp.Status.Code != common_pb.Status_OK {
			if strings.Contains(appendResp.Status.Message, "not primary") {
				primaryMisses++
				if primaryMisses >= opMaxRetries {
					return -1, "", fmt.Errorf("append (not primary after %d retries): %s", opMaxRetries, appendResp.Status.Message)
				}
				time.Sleep(300 * time.Millisecond)
				c.invalidateLastChunkEntry(filename)
				continue
			}
			if strings.Contains(appendResp.Status.Message, "exceeds maximum chunk size") {
				// Current chunk is full. Force-allocate the next chunk by writing a
				// single sentinel byte at its start, then retry the append there.
				extCtx, extCancel := context.WithTimeout(ctx, 10*time.Second)
				c.Write(extCtx, filename, (chunkIdx+1)*ChunkSize, []byte{0}) //nolint
				extCancel()
				c.invalidateLastChunkEntry(filename)
				continue // rollover — does NOT count against primaryMisses
			}
			return -1, "", fmt.Errorf("append failed: %s", appendResp.Status.Message)
		}

		return appendResp.OffsetInChunk + ChunkSize*chunkIdx, idempID, nil
	}
	return -1, "", fmt.Errorf("append failed after %d chunk rollovers", maxRollovers)
}

// Supporting types for write operations
type WriteOperation struct {
	Primary     string   // Primary chunk server handle
	Secondaries []string // Secondary chunk server handles
	Offset      int64    // Write offset within chunk
	Data        []byte   // Data to be written
}

// // Helper method to check if current position is at end of file
// func (fh *FileHandle) isEOF() bool {
//     fh.mu.RLock()
//     defer fh.mu.RUnlock()
//     return fh.position >= fh.size
// }

// // Seek sets the offset for the next Read or Write on file to offset, interpreted
// // according to whence: 0 means relative to the origin of the file, 1 means
// // relative to the current offset, and 2 means relative to the end.
// func (fh *FileHandle) Seek(offset int64, whence int) (int64, error) {
//     fh.mu.Lock()
//     defer fh.mu.Unlock()

//     var abs int64
//     switch whence {
//     case io.SeekStart:
//         abs = offset
//     case io.SeekCurrent:
//         abs = fh.position + offset
//     case io.SeekEnd:
//         abs = fh.size + offset
//     default:
//         return 0, fmt.Errorf("invalid whence: %d", whence)
//     }

//     if abs < 0 {
//         return 0, fmt.Errorf("negative position: %d", abs)
//     }

//     fh.position = abs
//     return abs, nil
// }
