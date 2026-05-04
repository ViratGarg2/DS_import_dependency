// GFS Benchmark Suite — mirrors the measurements in the GFS paper (§6).
//
// Benchmarks:
//   1. Aggregate Read Throughput   — N concurrent clients reading distinct files
//   2. Aggregate Write Throughput  — N concurrent clients writing distinct files
//   3. Record Append Concurrency   — N clients appending to one shared file
//   4. File Size Impact on Latency — latency from 4 KB to 8 MB
//   5. Chunk Boundary Overhead     — cost of crossing a 1 MB boundary
//   6. Master Operation Latency    — ops/sec for metadata-only operations
//
// Usage (from project root after building):
//
//	go run ./benchmarking/ [-config <path>] [-results <dir>] [-bench all|read,write,...]
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	gfs "github.com/Mit-Vin/GFS-Distributed-Systems/internal/client"
)

// ── constants ─────────────────────────────────────────────────────────────────

const (
	chunkSz         = 1 * 1024 * 1024 // 1 MB — must match ChunkSize in client
	appendPayload   = 64 * 1024        // 64 KB — well under the 256 KB append limit
	benchFileSizeMB = 32              // file size per client in throughput tests (32 chunks)
)

// ── CLI flags ─────────────────────────────────────────────────────────────────

var (
	configPath    = flag.String("config", "", "path to client-config YAML (auto-detected if empty)")
	resultsDir    = flag.String("results", "", "directory for JSON result files (auto-detected if empty)")
	benchList     = flag.String("bench", "all", "comma-separated benchmarks to run: read,write,append,filesize,boundary,master,comparison,sustained,mixed,all")
	manageCluster = flag.Bool("manage-cluster", true, "automatically start/stop master and chunk servers")
)

// activeResultsDir is the resolved output directory, set once in main().
var activeResultsDir string

// ── result types (serialised to JSON) ─────────────────────────────────────────

type ReadThroughputResult struct {
	Concurrency    int     `json:"concurrency"`
	ThroughputMBps float64 `json:"throughput_mbps"`
	TotalMB        float64 `json:"total_mb"`
	DurationSec    float64 `json:"duration_sec"`
}

type WriteThroughputResult struct {
	Concurrency    int     `json:"concurrency"`
	ThroughputMBps float64 `json:"throughput_mbps"`
	TotalMB        float64 `json:"total_mb"`
	DurationSec    float64 `json:"duration_sec"`
}

type AppendResult struct {
	Concurrency   int     `json:"concurrency"`
	TotalAppends  int     `json:"total_appends"`
	OpsPerSec     float64 `json:"ops_per_sec"`
	AvgLatencyMs  float64 `json:"avg_latency_ms"`
	P50LatencyMs  float64 `json:"p50_latency_ms"`
	P99LatencyMs  float64 `json:"p99_latency_ms"`
	DurationSec   float64 `json:"duration_sec"`
}

type FileSizeResult struct {
	FileSizeMB     float64 `json:"file_size_mb"`
	FileSizeBytes  int64   `json:"file_size_bytes"`
	WriteLatencyMs float64 `json:"write_latency_ms"`
	ReadLatencyMs  float64 `json:"read_latency_ms"`
	WriteMBps      float64 `json:"write_mbps"`
	ReadMBps       float64 `json:"read_mbps"`
}

type BoundaryResult struct {
	AccessType     string  `json:"access_type"` // "within_chunk" | "cross_boundary"
	SizeKB         int     `json:"size_kb"`
	WriteLatencyMs float64 `json:"write_latency_ms"`
	ReadLatencyMs  float64 `json:"read_latency_ms"`
}

type MasterOpResult struct {
	Operation    string  `json:"operation"`
	TotalOps     int     `json:"total_ops"`
	OpsPerSec    float64 `json:"ops_per_sec"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	P99LatencyMs float64 `json:"p99_latency_ms"`
}

type ComparisonResult struct {
	Operation       string  `json:"operation"`
	BaselineMBps    float64 `json:"baseline_mbps"`
	GFSMBps         float64 `json:"gfs_mbps"`
	BaselineLatMs   float64 `json:"baseline_lat_ms"`
	GFSLatMs        float64 `json:"gfs_lat_ms"`
	ThroughputPct   float64 `json:"throughput_pct"`   // GFS / Baseline * 100
	LatencyOverhead float64 `json:"latency_overhead"` // GFS_ms / Baseline_ms
}

type SustainedResult struct {
	ElapsedSec         float64 `json:"elapsed_sec"`
	ThroughputMBps     float64 `json:"throughput_mbps"`
	TotalMBTransferred float64 `json:"total_mb_transferred"`
	Operation          string  `json:"operation"`
}

type MixedWorkloadResult struct {
	Readers       int     `json:"readers"`
	Writers       int     `json:"writers"`
	ReadMBps      float64 `json:"read_mbps"`
	WriteMBps     float64 `json:"write_mbps"`
	AggregateMBps float64 `json:"aggregate_mbps"`
	ReadBytes     int64   `json:"read_bytes"`
	WriteBytes    int64   `json:"write_bytes"`
	DurationSec   float64 `json:"duration_sec"`
}

// ── helpers ───────────────────────────────────────────────────────────────────

// resolveResultsDir returns the directory where JSON results should be written.
// When run from the project root, it puts results inside benchmarking/results/
// so that plot.py can find them with its default relative path.
func resolveResultsDir() string {
	if *resultsDir != "" {
		return *resultsDir
	}
	// Running from project root (go run ./benchmarking/)
	if _, err := os.Stat("benchmarking"); err == nil {
		return "benchmarking/results"
	}
	// Running from inside benchmarking/
	return "results"
}

// resolveConfig finds the client config when no explicit -config flag is given.
// Tries paths relative to both the project root (go run ./benchmarking/) and
// the benchmarking directory (go run .).
func resolveConfig() string {
	if clusterClientCfgPath != "" {
		return clusterClientCfgPath
	}
	if *configPath != "" {
		return *configPath
	}
	candidates := []string{
		"configs/client-config.yml",    // running from project root
		"../configs/client-config.yml", // running from benchmarking/
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "configs/client-config.yml" // fallback with clear error downstream
}

func newClient() (*gfs.Client, error) {
	return gfs.NewClient(resolveConfig())
}

func randData(n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	return b
}

func saveJSON(filename string, v interface{}) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		log.Printf("[save] marshal error: %v", err)
		return
	}
	path := fmt.Sprintf("%s/%s", activeResultsDir, filename)
	if err := os.WriteFile(path, data, 0644); err != nil {
		log.Printf("[save] write error %s: %v", path, err)
		return
	}
	log.Printf("  → saved %s", path)
}

func pct(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	return sorted[int(float64(len(sorted)-1)*p/100.0)]
}

func meanF(s []float64) float64 {
	if len(s) == 0 {
		return 0
	}
	var sum float64
	for _, v := range s {
		sum += v
	}
	return sum / float64(len(s))
}

func sortedF(s []float64) []float64 {
	c := make([]float64, len(s))
	copy(c, s)
	sort.Float64s(c)
	return c
}

func cleanup(filenames []string) {
	c, err := newClient()
	if err != nil {
		return
	}
	defer c.Close()
	ctx := context.Background()
	for _, fn := range filenames {
		if err := c.Delete(ctx, fn); err != nil {
			// ignore — file may not exist (already GC'd or never created)
		}
	}
}

// freshCreate deletes fn if it already exists, then creates it fresh.
func freshCreate(c *gfs.Client, ctx context.Context, fn string) error {
	c.Delete(ctx, fn) // ignore error — file may not exist
	return c.Create(ctx, fn)
}

// seedFile creates fn (replacing any existing copy) and writes sz bytes of random data.
func seedFile(c *gfs.Client, ctx context.Context, fn string, sz int) bool {
	if err := freshCreate(c, ctx, fn); err != nil {
		log.Printf("  [seed] create %s: %v", fn, err)
		return false
	}
	if _, err := c.Write(ctx, fn, 0, randData(sz)); err != nil {
		log.Printf("  [seed] write %s: %v", fn, err)
		return false
	}
	return true
}

// ── Benchmark 1: Aggregate Read Throughput ────────────────────────────────────
// Each client reads a distinct pre-seeded file entirely.  Mimics GFS §6.2 where
// N clients each stream a 64 MB file; here files are benchFileSizeMB MB (scaled
// down for a local cluster).

func benchmarkReadThroughput() {
	log.Println("\n=== Benchmark 1: Aggregate Read Throughput ===")

	fileSizeBytes := int64(benchFileSizeMB * chunkSz)
	concurrencies := []int{1, 2, 3, 4}
	results := make([]ReadThroughputResult, 0, len(concurrencies))

	for _, n := range concurrencies {
		log.Printf("  concurrency = %d", n)
		filenames := make([]string, n)
		for i := range filenames {
			filenames[i] = fmt.Sprintf("bench/read/c%d/f%d", n, i)
		}

		// Sequential seed phase (not timed)
		func() {
			c, err := newClient()
			if err != nil {
				log.Printf("  seed client: %v", err)
				return
			}
			defer c.Close()
			ctx := context.Background()
			for _, fn := range filenames {
				seedFile(c, ctx, fn, int(fileSizeBytes))
			}
		}()

		// Parallel read phase (timed)
		var totalBytes int64
		var wg sync.WaitGroup
		start := time.Now()
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(fn string) {
				defer wg.Done()
				c, err := newClient()
				if err != nil {
					return
				}
				defer c.Close()
				ctx := context.Background()
				data, err := c.Read(ctx, fn, 0, fileSizeBytes)
				if err != nil {
					log.Printf("  read %s: %v", fn, err)
					return
				}
				atomic.AddInt64(&totalBytes, int64(len(data)))
			}(filenames[i])
		}
		wg.Wait()
		dur := time.Since(start).Seconds()

		mb := float64(totalBytes) / (1024 * 1024)
		tput := mb / dur
		log.Printf("    %.2f MB/s  (%.1f MB in %.2fs)", tput, mb, dur)
		results = append(results, ReadThroughputResult{
			Concurrency:    n,
			ThroughputMBps: tput,
			TotalMB:        mb,
			DurationSec:    dur,
		})

		cleanup(filenames)
	}

	saveJSON("read_throughput.json", results)
}

// ── Benchmark 2: Aggregate Write Throughput ───────────────────────────────────
// N clients each write benchFileSizeMB MB to a distinct file concurrently.
// Mirrors GFS §6.2 write-bandwidth test; multi-hop pipeline overhead is
// implicit (data goes client → primary → 2 secondaries before ACK).

func benchmarkWriteThroughput() {
	log.Println("\n=== Benchmark 2: Aggregate Write Throughput ===")

	fileSizeBytes := int64(benchFileSizeMB * chunkSz)
	concurrencies := []int{1, 2, 3, 4}
	results := make([]WriteThroughputResult, 0, len(concurrencies))

	for _, n := range concurrencies {
		log.Printf("  concurrency = %d", n)
		filenames := make([]string, n)
		for i := range filenames {
			filenames[i] = fmt.Sprintf("bench/write/c%d/f%d", n, i)
		}

		// Pre-create (not timed — we only time the actual write)
		func() {
			c, err := newClient()
			if err != nil {
				return
			}
			defer c.Close()
			ctx := context.Background()
			for _, fn := range filenames {
				freshCreate(c, ctx, fn)
			}
		}()

		payload := randData(int(fileSizeBytes))
		var totalBytes int64
		var wg sync.WaitGroup
		start := time.Now()
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(fn string) {
				defer wg.Done()
				c, err := newClient()
				if err != nil {
					return
				}
				defer c.Close()
				ctx := context.Background()
				written, err := c.Write(ctx, fn, 0, payload)
				if err != nil {
					log.Printf("  write %s: %v", fn, err)
					return
				}
				atomic.AddInt64(&totalBytes, int64(written))
			}(filenames[i])
		}
		wg.Wait()
		dur := time.Since(start).Seconds()

		mb := float64(totalBytes) / (1024 * 1024)
		tput := mb / dur
		log.Printf("    %.2f MB/s  (%.1f MB in %.2fs)", tput, mb, dur)
		results = append(results, WriteThroughputResult{
			Concurrency:    n,
			ThroughputMBps: tput,
			TotalMB:        mb,
			DurationSec:    dur,
		})

		cleanup(filenames)
	}

	saveJSON("write_throughput.json", results)
}

// ── Benchmark 3: Record Append Concurrency ────────────────────────────────────
// N clients all append to a single shared file, stressing the primary's
// serialisation of concurrent appends.  Mirrors GFS §6.3.

func benchmarkAppendConcurrency() {
	log.Println("\n=== Benchmark 3: Record Append Concurrency ===")

	const appendsPerClient = 100
	payload := randData(appendPayload)
	concurrencies := []int{1, 2, 3}
	results := make([]AppendResult, 0, len(concurrencies))

	for _, n := range concurrencies {
		log.Printf("  concurrency = %d", n)
		sharedFile := fmt.Sprintf("bench/append/c%d/shared", n)

		func() {
			c, err := newClient()
			if err != nil {
				return
			}
			defer c.Close()
			freshCreate(c, context.Background(), sharedFile)
		}()

		var (
			wg           sync.WaitGroup
			mu           sync.Mutex
			latencies    []float64
			totalAppends int64
		)

		start := time.Now()
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				c, err := newClient()
				if err != nil {
					return
				}
				defer c.Close()
				ctx := context.Background()
				for j := 0; j < appendsPerClient; j++ {
					t0 := time.Now()
					_, _, err := c.Append(ctx, sharedFile, payload)
					latMs := float64(time.Since(t0).Microseconds()) / 1000.0
					if err != nil {
						log.Printf("  append: %v", err)
						continue
					}
					atomic.AddInt64(&totalAppends, 1)
					mu.Lock()
					latencies = append(latencies, latMs)
					mu.Unlock()
				}
			}()
		}
		wg.Wait()
		dur := time.Since(start).Seconds()

		sorted := sortedF(latencies)
		opsPerSec := float64(totalAppends) / dur
		avg := meanF(latencies)
		log.Printf("    %.1f ops/s  avg %.2fms  p99 %.2fms",
			opsPerSec, avg, pct(sorted, 99))

		results = append(results, AppendResult{
			Concurrency:  n,
			TotalAppends: int(totalAppends),
			OpsPerSec:    opsPerSec,
			AvgLatencyMs: avg,
			P50LatencyMs: pct(sorted, 50),
			P99LatencyMs: pct(sorted, 99),
			DurationSec:  dur,
		})

		cleanup([]string{sharedFile})
	}

	saveJSON("append_concurrency.json", results)
}

// ── Benchmark 4: File Size Impact on Latency ─────────────────────────────────
// Single-client write + read latency across a range of file sizes (4 KB → 8 MB).
// Shows the fixed master-lookup overhead at small sizes vs. the linear transfer
// cost that dominates at large sizes — analogous to GFS §6.1 latency analysis.

func benchmarkFileSizeLatency() {
	log.Println("\n=== Benchmark 4: File Size Impact on Latency ===")

	fileSizes := []int64{
		4 * 1024,         // 4 KB   — sub-chunk, master overhead dominates
		64 * 1024,        // 64 KB
		256 * 1024,       // 256 KB
		1 * 1024 * 1024,
		4*1024*1024,
		20*1024*1024,  // 1 MB   — exactly one chunk

	}
	results := make([]FileSizeResult, 0, len(fileSizes))

	for _, sz := range fileSizes {
		fn := fmt.Sprintf("bench/filesize/sz%d", sz)
		payload := randData(int(sz))

		c, err := newClient()
		if err != nil {
			log.Printf("  client error: %v", err)
			continue
		}
		ctx := context.Background()
		freshCreate(c, ctx, fn)

		t0 := time.Now()
		_, writeErr := c.Write(ctx, fn, 0, payload)
		writeMs := float64(time.Since(t0).Microseconds()) / 1000.0

		t0 = time.Now()
		_, readErr := c.Read(ctx, fn, 0, sz)
		readMs := float64(time.Since(t0).Microseconds()) / 1000.0

		c.Close()

		if writeErr != nil {
			log.Printf("  write error (sz=%d): %v", sz, writeErr)
		}
		if readErr != nil {
			log.Printf("  read error (sz=%d): %v", sz, readErr)
		}

		szMB := float64(sz) / (1024 * 1024)
		writeMBps, readMBps := 0.0, 0.0
		// Only compute throughput for successful ops — failed ops return instantly,
		// giving nonsensical values like 300,000 MB/s.
		if writeErr == nil && writeMs > 0 {
			writeMBps = szMB / (writeMs / 1000)
		}
		if readErr == nil && readMs > 0 {
			readMBps = szMB / (readMs / 1000)
		}

		log.Printf("  %8.3f MB  write %.1fms (%.1f MB/s)  read %.1fms (%.1f MB/s)",
			szMB, writeMs, writeMBps, readMs, readMBps)

		results = append(results, FileSizeResult{
			FileSizeMB:     szMB,
			FileSizeBytes:  sz,
			WriteLatencyMs: writeMs,
			ReadLatencyMs:  readMs,
			WriteMBps:      writeMBps,
			ReadMBps:       readMBps,
		})
		cleanup([]string{fn})
	}

	saveJSON("filesize_latency.json", results)
}

// ── Benchmark 5: Chunk Boundary Overhead ─────────────────────────────────────
// Compares latency when an operation is entirely within one 1 MB chunk vs.
// when it spans the boundary between two chunks (requiring two master lookups
// and two chunk-server round-trips).

func benchmarkChunkBoundary() {
	log.Println("\n=== Benchmark 5: Chunk Boundary Overhead ===")

	type tc struct {
		label  string
		offset int64
		size   int
	}
	cases := []tc{
		// Entirely inside chunk 0
		{"within_chunk_512k", 0, 512 * 1024},
		// Spans chunk 0 → 1  (starts at 768 KB, ends at 1280 KB)
		{"cross_boundary_512k", 768 * 1024, 512 * 1024},
		// Entire chunk 0
		{"within_chunk_1m", 0, chunkSz},
		// Spans chunk 0 → 1  (starts at 512 KB, ends at 1536 KB)
		{"cross_boundary_1m", 512 * 1024, chunkSz},
	}

	const reps = 5
	results := make([]BoundaryResult, 0, len(cases))

	for _, tc := range cases {
		fn := fmt.Sprintf("bench/boundary/%s", tc.label)
		fileSize := int(tc.offset) + tc.size
		payload := randData(tc.size)

		c, err := newClient()
		if err != nil {
			log.Printf("  client error: %v", err)
			continue
		}
		ctx := context.Background()
		freshCreate(c, ctx, fn)
		// Seed: allocate all needed chunks. On error, log but still benchmark
		// (we'll measure the failure latency, which is still meaningful data).
		if _, err := c.Write(ctx, fn, 0, randData(fileSize)); err != nil {
			log.Printf("  seed error %s: %v (will still time the ops)", tc.label, err)
		} else {
			// Warm-up only when seed succeeded
			c.Write(ctx, fn, tc.offset, payload)
			c.Read(ctx, fn, tc.offset, int64(tc.size))
		}

		// Time writes
		var wSum float64
		for i := 0; i < reps; i++ {
			t0 := time.Now()
			c.Write(ctx, fn, tc.offset, payload)
			wSum += float64(time.Since(t0).Microseconds()) / 1000.0
		}

		// Time reads
		var rSum float64
		for i := 0; i < reps; i++ {
			t0 := time.Now()
			c.Read(ctx, fn, tc.offset, int64(tc.size))
			rSum += float64(time.Since(t0).Microseconds()) / 1000.0
		}
		c.Close()

		wAvg := wSum / float64(reps)
		rAvg := rSum / float64(reps)
		log.Printf("  %-28s  write %.1fms  read %.1fms", tc.label, wAvg, rAvg)

		results = append(results, BoundaryResult{
			AccessType:     tc.label,
			SizeKB:         tc.size / 1024,
			WriteLatencyMs: wAvg,
			ReadLatencyMs:  rAvg,
		})

		cleanup([]string{fn})
	}

	saveJSON("chunk_boundary.json", results)
}

// ── Benchmark 6: Master Operation Latency ────────────────────────────────────
// Measures throughput and latency of pure metadata operations (no data I/O),
// revealing the master's single-threaded bottleneck discussed in GFS §6.4.

func benchmarkMasterLatency() {
	log.Println("\n=== Benchmark 6: Master Operation Latency ===")

	const totalOps = 200
	results := make([]MasterOpResult, 0)

	c, err := newClient()
	if err != nil {
		log.Printf("  client error: %v", err)
		return
	}
	defer c.Close()
	ctx := context.Background()

	// Pre-create files for rename + delete tests
	for i := 0; i < totalOps; i++ {
		freshCreate(c, ctx, fmt.Sprintf("bench/master/rename/src%d", i))
		freshCreate(c, ctx, fmt.Sprintf("bench/master/delete/f%d", i))
	}

	runOp := func(name string, fn func(i int) error) {
		latencies := make([]float64, 0, totalOps)
		var success int
		start := time.Now()
		for i := 0; i < totalOps; i++ {
			t0 := time.Now()
			err := fn(i)
			lat := float64(time.Since(t0).Microseconds()) / 1000.0
			latencies = append(latencies, lat)
			if err == nil {
				success++
			}
		}
		dur := time.Since(start).Seconds()
		sorted := sortedF(latencies)
		avg := meanF(latencies)
		ops := float64(success) / dur
		log.Printf("  %-16s  %.0f ops/s  avg %.2fms  p99 %.2fms",
			name, ops, avg, pct(sorted, 99))
		results = append(results, MasterOpResult{
			Operation:    name,
			TotalOps:     success,
			OpsPerSec:    ops,
			AvgLatencyMs: avg,
			P99LatencyMs: pct(sorted, 99),
		})
	}

	runOp("CreateFile", func(i int) error {
		return c.Create(ctx, fmt.Sprintf("bench/master/create/f%d", i))
	})

	runOp("ListNamespace", func(i int) error {
		_, err := c.ListNamespace(ctx, false)
		return err
	})

	runOp("RenameFile", func(i int) error {
		return c.Rename(ctx,
			fmt.Sprintf("bench/master/rename/src%d", i),
			fmt.Sprintf("bench/master/rename/dst%d", i))
	})

	runOp("DeleteFile", func(i int) error {
		return c.Delete(ctx, fmt.Sprintf("bench/master/delete/f%d", i))
	})

	// Cleanup
	for i := 0; i < totalOps; i++ {
		c.Delete(ctx, fmt.Sprintf("bench/master/create/f%d", i))
		c.Delete(ctx, fmt.Sprintf("bench/master/rename/dst%d", i))
	}

	saveJSON("master_latency.json", results)
}

// ── Benchmark 7: GFS vs Local Filesystem Baseline ────────────────────────────
// Compares GFS throughput and latency against direct local disk I/O for three
// access patterns: sequential write, sequential read, and random small reads.
// Reveals the overhead introduced by distributed coordination and replication.

func benchmarkComparison() {
	log.Println("\n=== Benchmark 7: GFS vs Local Filesystem Baseline ===")

	const sizeMB = 32
	const sizeBytes = sizeMB * chunkSz
	const randomOps = 200
	const randomBlock = 4 * 1024 // 4 KB random read

	payload := randData(sizeBytes)
	results := make([]ComparisonResult, 0, 3)

	// ── Local filesystem baseline ─────────────────────────────────────────
	tmp, err := os.CreateTemp("", "gfs-bench-*")
	if err != nil {
		log.Printf("  cannot create temp file: %v", err)
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	// Sequential write
	t0 := time.Now()
	tmp.Write(payload)
	tmp.Sync()
	baseWriteMs := float64(time.Since(t0).Microseconds()) / 1000.0
	tmp.Close()

	// Sequential read
	t0 = time.Now()
	rf, _ := os.Open(tmpPath)
	io.ReadAll(rf)
	baseReadMs := float64(time.Since(t0).Microseconds()) / 1000.0
	rf.Close()

	// Random small reads
	rf, _ = os.Open(tmpPath)
	buf := make([]byte, randomBlock)
	t0 = time.Now()
	for i := 0; i < randomOps; i++ {
		off := rand.Int63n(int64(sizeBytes) - randomBlock)
		rf.ReadAt(buf, off)
	}
	baseRandomMs := float64(time.Since(t0).Microseconds()) / 1000.0
	rf.Close()

	// ── GFS ───────────────────────────────────────────────────────────────
	c, err := newClient()
	if err != nil {
		log.Printf("  GFS client error: %v", err)
		return
	}
	defer c.Close()
	ctx := context.Background()

	fn := "bench/comparison/seq"
	freshCreate(c, ctx, fn)

	// Sequential write
	t0 = time.Now()
	_, gfsWriteErr := c.Write(ctx, fn, 0, payload)
	gfsWriteMs := float64(time.Since(t0).Microseconds()) / 1000.0

	// Sequential read
	t0 = time.Now()
	_, gfsReadErr := c.Read(ctx, fn, 0, int64(sizeBytes))
	gfsReadMs := float64(time.Since(t0).Microseconds()) / 1000.0

	// Random small reads
	t0 = time.Now()
	for i := 0; i < randomOps; i++ {
		off := rand.Int63n(int64(sizeBytes) - randomBlock)
		c.Read(ctx, fn, off, randomBlock)
	}
	gfsRandomMs := float64(time.Since(t0).Microseconds()) / 1000.0

	if gfsWriteErr != nil {
		log.Printf("  GFS write error: %v", gfsWriteErr)
	}
	if gfsReadErr != nil {
		log.Printf("  GFS read error: %v", gfsReadErr)
	}
	cleanup([]string{fn})

	// ── Build results ─────────────────────────────────────────────────────
	szMB := float64(sizeMB)

	add := func(op string, baseMs, gfsMs float64, gfsErr error, sequential bool) {
		var baseMBps, gfsMBps float64
		if sequential {
			baseMBps = szMB / (baseMs / 1000)
			if gfsErr == nil {
				gfsMBps = szMB / (gfsMs / 1000)
			}
		} else {
			// Random I/O: report as MB/s of 4 KB blocks
			baseMBps = float64(randomOps) * float64(randomBlock) / (1024 * 1024) / (baseMs / 1000)
			if gfsErr == nil {
				gfsMBps = float64(randomOps) * float64(randomBlock) / (1024 * 1024) / (gfsMs / 1000)
			}
		}
		tputPct := 0.0
		if baseMBps > 0 {
			tputPct = gfsMBps / baseMBps * 100
		}
		latOvhd := 0.0
		if baseMs > 0 {
			latOvhd = gfsMs / baseMs
		}
		log.Printf("  %-20s  base %.1fms (%.1f MB/s)  gfs %.1fms (%.1f MB/s)  overhead %.1fx",
			op, baseMs, baseMBps, gfsMs, gfsMBps, latOvhd)
		results = append(results, ComparisonResult{
			Operation:       op,
			BaselineMBps:    baseMBps,
			GFSMBps:         gfsMBps,
			BaselineLatMs:   baseMs,
			GFSLatMs:        gfsMs,
			ThroughputPct:   tputPct,
			LatencyOverhead: latOvhd,
		})
	}

	add("Sequential Write", baseWriteMs, gfsWriteMs, gfsWriteErr, true)
	add("Sequential Read", baseReadMs, gfsReadMs, gfsReadErr, true)
	add("Random I/O", baseRandomMs, gfsRandomMs, nil, false)

	saveJSON("comparison.json", results)
}

// ── Benchmark 8: Sustained Throughput ────────────────────────────────────────
// A single client streams reads/writes continuously for 30 s; throughput is
// sampled every 5 s to reveal ramp-up behaviour and steady-state performance.

func sampleThroughput(op string, dur, interval time.Duration, counter *int64) []SustainedResult {
	results := make([]SustainedResult, 0)
	start := time.Now()
	prev := int64(0)
	prevT := start
	for time.Since(start) < dur {
		time.Sleep(interval)
		now := time.Now()
		cur := atomic.LoadInt64(counter)
		intervalMB := float64(cur-prev) / (1024 * 1024)
		secs := now.Sub(prevT).Seconds()
		mbps := 0.0
		if secs > 0 {
			mbps = intervalMB / secs
		}
		totalMB := float64(cur) / (1024 * 1024)
		elapsed := now.Sub(start).Seconds()
		log.Printf("  [%s] t=%.0fs  %.1f MB/s (%.0f MB total)", op, elapsed, mbps, totalMB)
		results = append(results, SustainedResult{
			ElapsedSec:         elapsed,
			ThroughputMBps:     mbps,
			TotalMBTransferred: totalMB,
			Operation:          op,
		})
		prev = cur
		prevT = now
	}
	return results
}

func benchmarkSustainedThroughput() {
	log.Println("\n=== Benchmark 8: Sustained Throughput ===")

	const testDuration = 30 * time.Second
	const sampleEvery = 5 * time.Second

	results := make([]SustainedResult, 0)

	// ── Sustained Write ───────────────────────────────────────────────────
	log.Println("  streaming writes for 30 s …")
	{
		c, err := newClient()
		if err != nil {
			log.Printf("  [sustained-write] client: %v", err)
		} else {
			ctx := context.Background()
			fn := "bench/sustained/write"
			freshCreate(c, ctx, fn)

			var bytesWritten int64
			stop := make(chan struct{})
			data := randData(chunkSz)

			go func() {
				var off int64
				for {
					select {
					case <-stop:
						return
					default:
						n, err := c.Write(ctx, fn, off, data)
						if err == nil {
							atomic.AddInt64(&bytesWritten, int64(n))
							off += int64(n)
						}
					}
				}
			}()

			results = append(results, sampleThroughput("write", testDuration, sampleEvery, &bytesWritten)...)
			close(stop)
			c.Close()
			cleanup([]string{fn})
		}
	}

	// ── Sustained Read ────────────────────────────────────────────────────
	log.Println("  streaming reads for 30 s …")
	{
		c, err := newClient()
		if err != nil {
			log.Printf("  [sustained-read] client: %v", err)
		} else {
			ctx := context.Background()
			fn := "bench/sustained/read"
			fileSize := int64(benchFileSizeMB * chunkSz)
			freshCreate(c, ctx, fn)
			if _, seedErr := c.Write(ctx, fn, 0, randData(int(fileSize))); seedErr != nil {
				log.Printf("  [sustained-read] seed: %v", seedErr)
			} else {
				var bytesRead int64
				stop := make(chan struct{})

				go func() {
					for {
						select {
						case <-stop:
							return
						default:
							data, err := c.Read(ctx, fn, 0, fileSize)
							if err == nil {
								atomic.AddInt64(&bytesRead, int64(len(data)))
							}
						}
					}
				}()

				results = append(results, sampleThroughput("read", testDuration, sampleEvery, &bytesRead)...)
				close(stop)
			}
			c.Close()
			cleanup([]string{fn})
		}
	}

	saveJSON("sustained_throughput.json", results)
}

// ── Benchmark 9: Mixed Read/Write Workload ────────────────────────────────────
// Simultaneous readers and writers on distinct files.  Tests how GFS handles
// concurrent mixed traffic — a realistic production pattern.

func benchmarkMixedWorkload() {
	log.Println("\n=== Benchmark 9: Mixed Read/Write Workload ===")

	const testDuration = 10 * time.Second
	fileSizeBytes := int64(benchFileSizeMB * chunkSz)

	configs := []struct{ readers, writers int }{
		{2, 2},
		{4, 4},
		// {8, 2},
		// {2, 8},
		// {8, 8},
	}
	results := make([]MixedWorkloadResult, 0, len(configs))

	for _, cfg := range configs {
		log.Printf("  readers=%d writers=%d", cfg.readers, cfg.writers)

		readerFiles := make([]string, cfg.readers)
		writerFiles := make([]string, cfg.writers)
		for i := range readerFiles {
			readerFiles[i] = fmt.Sprintf("bench/mixed/r%dw%d/read%d", cfg.readers, cfg.writers, i)
		}
		for i := range writerFiles {
			writerFiles[i] = fmt.Sprintf("bench/mixed/r%dw%d/write%d", cfg.readers, cfg.writers, i)
		}

		func() {
			c, err := newClient()
			if err != nil {
				return
			}
			defer c.Close()
			ctx := context.Background()
			for _, fn := range readerFiles {
				seedFile(c, ctx, fn, int(fileSizeBytes))
			}
			for _, fn := range writerFiles {
				freshCreate(c, ctx, fn)
			}
		}()

		var bytesRead, bytesWritten int64
		stop := make(chan struct{})
		var wg sync.WaitGroup

		for _, fn := range readerFiles {
			wg.Add(1)
			go func(fn string) {
				defer wg.Done()
				c, err := newClient()
				if err != nil {
					return
				}
				defer c.Close()
				ctx := context.Background()
				for {
					select {
					case <-stop:
						return
					default:
						data, err := c.Read(ctx, fn, 0, fileSizeBytes)
						if err == nil {
							atomic.AddInt64(&bytesRead, int64(len(data)))
						}
					}
				}
			}(fn)
		}

		for _, fn := range writerFiles {
			wg.Add(1)
			go func(fn string) {
				defer wg.Done()
				c, err := newClient()
				if err != nil {
					return
				}
				defer c.Close()
				ctx := context.Background()
				data := randData(chunkSz)
				var off int64
				for {
					select {
					case <-stop:
						return
					default:
						n, err := c.Write(ctx, fn, off, data)
						if err == nil {
							atomic.AddInt64(&bytesWritten, int64(n))
							off += int64(n)
						}
					}
				}
			}(fn)
		}

		time.Sleep(testDuration)
		close(stop)
		wg.Wait()

		readMBps := float64(bytesRead) / (1024 * 1024) / testDuration.Seconds()
		writeMBps := float64(bytesWritten) / (1024 * 1024) / testDuration.Seconds()
		log.Printf("    read %.1f MB/s  write %.1f MB/s  total %.1f MB/s",
			readMBps, writeMBps, readMBps+writeMBps)

		results = append(results, MixedWorkloadResult{
			Readers:       cfg.readers,
			Writers:       cfg.writers,
			ReadMBps:      readMBps,
			WriteMBps:     writeMBps,
			AggregateMBps: readMBps + writeMBps,
			ReadBytes:     bytesRead,
			WriteBytes:    bytesWritten,
			DurationSec:   testDuration.Seconds(),
		})

		allFiles := append(readerFiles, writerFiles...)
		cleanup(allFiles)
	}

	saveJSON("mixed_workload.json", results)
}

// ── main ──────────────────────────────────────────────────────────────────────

func main() {
	flag.Parse()
	activeResultsDir = resolveResultsDir()
	if err := os.MkdirAll(activeResultsDir, 0755); err != nil {
		log.Fatalf("cannot create results dir: %v", err)
	}

	want := map[string]bool{}
	for _, b := range strings.Split(*benchList, ",") {
		want[strings.TrimSpace(b)] = true
	}
	runAll := want["all"]

	if *manageCluster {
		root, err := findProjectRoot()
		if err != nil {
			log.Fatalf("[main] cannot find project root: %v", err)
		}
		cm, err := startCluster(root)
		if err != nil {
			log.Fatalf("[main] failed to start cluster: %v", err)
		}
		clusterClientCfgPath = cm.ClientCfg
		setupCleanupOnSignal(cm)
		defer cm.Stop()
	}

	cfg := resolveConfig()
	log.Printf("=== GFS Benchmark Suite ===")
	log.Printf("config:  %s", cfg)
	log.Printf("results: %s/", activeResultsDir)

	if _, err := os.Stat(cfg); err != nil {
		log.Fatalf("config file not found: %s", cfg)
	}
	probe, err := newClient()
	if err != nil {
		log.Fatalf("cannot connect to master: %v", err)
	}
	if _, err := probe.ListNamespace(context.Background(), false); err != nil {
		log.Fatalf("master reachability check failed: %v", err)
	}
	probe.Close()
	log.Println("master reachable — starting benchmarks")

	if runAll || want["read"] {
		benchmarkReadThroughput()
	}
	if runAll || want["write"] {
		benchmarkWriteThroughput()
	}
	if runAll || want["append"] {
		benchmarkAppendConcurrency()
	}
	if runAll || want["filesize"] {
		benchmarkFileSizeLatency()
	}
	if runAll || want["boundary"] {
		benchmarkChunkBoundary()
	}
	if runAll || want["master"] {
		benchmarkMasterLatency()
	}
	if runAll || want["comparison"] {
		benchmarkComparison()
	}
	// if runAll || want["sustained"] {
	// 	benchmarkSustainedThroughput()
	// }
	// if runAll || want["mixed"] {
	// 	benchmarkMixedWorkload()
	// }

	log.Println("\n=== Done. Run plot.py to generate charts. ===")
}
