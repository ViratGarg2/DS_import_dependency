package main

import (
	"bytes"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"text/template"
	"time"
)

// Ports used exclusively by the benchmark cluster — chosen to avoid collisions
// with any user-owned cluster running on the default ports (50051, 8080-8086).
const benchMasterPort = 50052

var benchChunkPorts = []int{9001, 9002, 9003}

// ClusterManager owns the master and chunk-server subprocesses for the
// duration of a benchmark run.
type ClusterManager struct {
	masterProc *exec.Cmd
	chunkProcs []*exec.Cmd
	tmpDir     string
	ClientCfg  string // path to the generated client-config YAML
}

// clusterClientCfgPath is set when the benchmark starts its own cluster so
// resolveConfig() returns the generated config rather than the project default.
var clusterClientCfgPath string

// ── project-root detection ────────────────────────────────────────────────────

func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found — are you inside the project tree?")
		}
		dir = parent
	}
}

// ── binary build ──────────────────────────────────────────────────────────────

func buildBinaries(root string) error {
	binDir := filepath.Join(root, "bin")
	os.MkdirAll(binDir, 0755)

	targets := []struct{ out, pkg string }{
		{filepath.Join(binDir, "gfs-master"), "./cmd/master/"},
		{filepath.Join(binDir, "gfs-chunkserver"), "./cmd/chunkserver/"},
	}
	for _, t := range targets {
		log.Printf("[cluster] building %s …", filepath.Base(t.out))
		cmd := exec.Command("go", "build", "-o", t.out, t.pkg)
		cmd.Dir = root
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("build %s: %v", t.pkg, err)
		}
	}
	return nil
}

// ── readiness polling ─────────────────────────────────────────────────────────

func waitForPort(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for %s", addr)
}

// ── config templates ──────────────────────────────────────────────────────────

var masterCfgTmpl = template.Must(template.New("master").Parse(`
chunk:
  size: 1048576
  naming_pattern: "chunk_{id}"
  checksum_algorithm: "sha256"
  verify_on_read: true
chunk_server:
  max_chunks: 2000
  storage_path: "{{.ChunksDir}}"
deletion:
  g_c_interval: 3600
  retention_period: 7200
  g_c_delete_batch_size: 100
  trash_dir_prefix: "/.trash/"
health:
  check_interval: 5
  timeout: 10
  max_failures: 3
metadata:
  database:
    type: "json"
    path: "{{.MasterDir}}/metadata.json"
    backup_interval: 3600
  max_filename_length: 255
  max_directory_depth: 64
lease:
  lease_timeout: 60
operation_log:
  path: "{{.MasterDir}}/operation-log.json"
replication:
  factor: 3
  timeout: 180
server:
  host: "localhost"
  port: {{.Port}}
  max_connections: 200
  connection_timeout: 30
  max_request_size: 104857600
  thread_pool_size: 50
`))

var chunkCfgTmpl = template.Must(template.New("chunk").Parse(`
server:
  master_address: "localhost:{{.MasterPort}}"
  data_dir: "{{.DataDir}}"
  heartbeat_interval: 5
  lease_timeout: 60
  lease_request_interval: 50
storage:
  max_chunk_size: 1048576
  buffer_size: 65536
  flush_interval: 5
operation:
  read_timeout: 30
  write_timeout: 60
  retry_attempts: 3
  retry_delay: 5
`))

var clientCfgTmpl = template.Must(template.New("client").Parse(`
connection:
  master:
    host: "localhost"
    port: {{.MasterPort}}
    timeout: 30
  max_retries: 3
  retry_interval: 1
  request_timeout: 60
cache:
  chunk_location:
    enabled: true
    size: 1000
    ttl: 60
  metadata:
    enabled: true
    size: 10000
    ttl: 600
operation:
  chunk:
    read_size: 1048576
    write_size: 1048576
    verify_writes: true
  retries:
    max_attempts: 3
    backoff_base: 2
  timeouts:
    read: 30
    write: 60
    delete: 30
logging:
  level: "error"
  format: "json"
  directory: "{{.LogDir}}"
  max_size: 10
  max_files: 1
monitoring:
  enabled: false
  update_interval: 60
  metrics_port: 9090
`))

func renderCfg(tmpl *template.Template, data interface{}) ([]byte, error) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ── cluster start / stop ──────────────────────────────────────────────────────

func startCluster(root string) (*ClusterManager, error) {
	log.Println("[cluster] building binaries …")
	if err := buildBinaries(root); err != nil {
		return nil, err
	}

	tmpDir, err := os.MkdirTemp("", "gfs-bench-*")
	if err != nil {
		return nil, err
	}
	log.Printf("[cluster] working dir: %s", tmpDir)

	masterDir := filepath.Join(tmpDir, "master")
	chunksDir := filepath.Join(tmpDir, "chunks")
	logDir := filepath.Join(tmpDir, "logs")
	for _, d := range []string{masterDir, chunksDir, logDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			os.RemoveAll(tmpDir)
			return nil, err
		}
	}

	cm := &ClusterManager{tmpDir: tmpDir}

	// ── write master config ───────────────────────────────────────────────
	mcfg, err := renderCfg(masterCfgTmpl, struct {
		MasterDir, ChunksDir string
		Port                 int
	}{masterDir, chunksDir, benchMasterPort})
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, err
	}
	masterCfgPath := filepath.Join(tmpDir, "master.yml")
	if err := os.WriteFile(masterCfgPath, mcfg, 0644); err != nil {
		os.RemoveAll(tmpDir)
		return nil, err
	}

	// ── write chunk-server config (shared; each server picks its own subdir) ──
	ccfg, err := renderCfg(chunkCfgTmpl, struct {
		MasterPort int
		DataDir    string
	}{benchMasterPort, chunksDir})
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, err
	}
	chunkCfgPath := filepath.Join(tmpDir, "chunk.yml")
	if err := os.WriteFile(chunkCfgPath, ccfg, 0644); err != nil {
		os.RemoveAll(tmpDir)
		return nil, err
	}

	// ── write client config ───────────────────────────────────────────────
	clcfg, err := renderCfg(clientCfgTmpl, struct {
		MasterPort int
		LogDir     string
	}{benchMasterPort, logDir})
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, err
	}
	clientCfgPath := filepath.Join(tmpDir, "client.yml")
	if err := os.WriteFile(clientCfgPath, clcfg, 0644); err != nil {
		os.RemoveAll(tmpDir)
		return nil, err
	}
	cm.ClientCfg = clientCfgPath

	masterBin := filepath.Join(root, "bin", "gfs-master")
	chunkBin := filepath.Join(root, "bin", "gfs-chunkserver")

	// ── start master ──────────────────────────────────────────────────────
	mlog, _ := os.Create(filepath.Join(logDir, "master.log"))
	cm.masterProc = exec.Command(masterBin, "-config", masterCfgPath)
	cm.masterProc.Stdout = mlog
	cm.masterProc.Stderr = mlog
	if err := cm.masterProc.Start(); err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("start master: %v", err)
	}
	log.Printf("[cluster] master PID %d on port %d", cm.masterProc.Process.Pid, benchMasterPort)

	if err := waitForPort(fmt.Sprintf("localhost:%d", benchMasterPort), 20*time.Second); err != nil {
		cm.Stop()
		return nil, fmt.Errorf("master not ready: %v", err)
	}
	log.Printf("[cluster] master ready")

	// ── start chunk servers ───────────────────────────────────────────────
	for i, port := range benchChunkPorts {
		clog, _ := os.Create(filepath.Join(logDir, fmt.Sprintf("chunk-%d.log", port)))
		cmd := exec.Command(chunkBin,
			"--port", strconv.Itoa(port),
			"--host", "localhost",
			"--config", chunkCfgPath,
		)
		cmd.Stdout = clog
		cmd.Stderr = clog
		if err := cmd.Start(); err != nil {
			cm.Stop()
			return nil, fmt.Errorf("start chunkserver %d on port %d: %v", i+1, port, err)
		}
		cm.chunkProcs = append(cm.chunkProcs, cmd)

		if err := waitForPort(fmt.Sprintf("localhost:%d", port), 20*time.Second); err != nil {
			cm.Stop()
			return nil, fmt.Errorf("chunkserver port %d not ready: %v", port, err)
		}
		log.Printf("[cluster] chunk server %d/%d ready on port %d (PID %d)",
			i+1, len(benchChunkPorts), port, cmd.Process.Pid)
	}

	// Wait for all chunk servers to complete their first heartbeat and get
	// listed as active servers in the master before the benchmarks begin.
	log.Printf("[cluster] waiting for chunk servers to register (heartbeat interval = 5s) …")
	time.Sleep(18 * time.Second)
	log.Printf("[cluster] cluster ready — starting benchmarks")
	return cm, nil
}

func (cm *ClusterManager) Stop() {
	log.Println("[cluster] sending SIGTERM to all servers …")
	for _, c := range cm.chunkProcs {
		if c != nil && c.Process != nil {
			c.Process.Signal(syscall.SIGTERM)
		}
	}
	if cm.masterProc != nil && cm.masterProc.Process != nil {
		cm.masterProc.Process.Signal(syscall.SIGTERM)
	}
	time.Sleep(3 * time.Second)
	// Force-kill anything still alive
	for _, c := range cm.chunkProcs {
		if c != nil && c.Process != nil {
			c.Process.Kill()
			c.Wait()
		}
	}
	if cm.masterProc != nil && cm.masterProc.Process != nil {
		cm.masterProc.Process.Kill()
		cm.masterProc.Wait()
	}
	if cm.tmpDir != "" {
		os.RemoveAll(cm.tmpDir)
	}
	log.Println("[cluster] all servers stopped, temp storage removed")
}

// setupCleanupOnSignal ensures Stop() is called even on Ctrl-C / SIGTERM.
func setupCleanupOnSignal(cm *ClusterManager) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-ch
		log.Printf("\n[cluster] caught %v — shutting down", sig)
		cm.Stop()
		os.Exit(1)
	}()
}
