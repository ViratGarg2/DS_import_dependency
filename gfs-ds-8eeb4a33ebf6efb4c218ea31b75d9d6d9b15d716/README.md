# GFS — Google File System Replica

**Team 7 · Import Dependency**

A production-faithful implementation of the [Google File System (2003)](https://research.google/pubs/pub51/) built in Go with gRPC.  
Implements all MVP deliverables from the project proposal: Read, Write, Record Append, Stale Replica Handling, Garbage Collection, Operation Logging, Checkpointing, and full benchmarking.

---

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Option A — Run Locally (Go)](#option-a--run-locally-go)
3. [Option B — Run with Docker](#option-b--run-with-docker)
   - [Internal mode (everything in Docker)](#internal-mode-recommended-for-testing)
   - [External mode (remote / multi-machine)](#external-mode-remote--multi-machine)
4. [Client Commands](#client-commands)
5. [Folder Structure](#folder-structure)
6. [Testing](#testing)
7. [Benchmarking](#benchmarking)
8. [Deliverables vs. Proposal](#deliverables-vs-proposal)

---

## Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| Go | 1.23+ | Build & run all components |
| protoc | any recent | Regenerate gRPC stubs (only needed after `.proto` changes) |
| Docker + Docker Compose | any recent | Containerised deployment |
| Python 3.9+ | optional | Run integration tests & plot benchmarks |

---

## Option A — Run Locally (Go)

### 1. Install Go toolchain

```bash
# macOS (Homebrew)
brew install go

# Linux
wget https://go.dev/dl/go1.23.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.23.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
```

### 2. Install protoc plugins (only needed after `.proto` changes)

```bash
export PATH="/opt/homebrew/bin:$PATH:$(go env GOPATH)/bin"

go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Regenerate stubs
make clean && make proto
```

### 3. Build all binaries

```bash
# From the repo root
go build ./...

# Or build named binaries into bin/
make build-all
# Produces: bin/gfs-master  bin/gfs-chunkserver  bin/gfs-client
```

### 4. Start the cluster (four separate terminals)

**Terminal 1 — Master**
```bash
cd cmd/master
go run main.go
# Listens on :50051 by default (edit configs/general-config.yml to change)
```

**Terminal 2 — Chunk Server 1**
```bash
cd cmd/chunkserver
go run main.go --port 8080
```

**Terminal 3 — Chunk Server 2** *(recommended for replication)*
```bash
cd cmd/chunkserver
go run main.go --port 8081
```

**Terminal 4 — Client**
```bash
cd cmd/client
go run main.go
# Opens the interactive gfs> prompt
```

### 5. Try it

```
gfs> create demo.txt
gfs> write demo.txt 0 hello
gfs> read demo.txt 0 5
gfs> append demo.txt _world
gfs> read demo.txt 0 11
gfs> ls
gfs> exit
```

---

## Option B — Run with Docker

### Internal mode *(recommended for testing)*

All components — master, chunk servers, and client — run inside the same Docker network. Container DNS handles addressing automatically.

```bash
# 1. Set configuration
cp .env.docker .env

# 2. Start everything (builds images automatically)
docker compose up -d --build

# 3. Open interactive client
docker exec -it gfs-client /usr/local/bin/gfs-client \
  --config /app/configs/docker/client-config.yml

# 4. Or pipe commands non-interactively
docker exec -i gfs-client /usr/local/bin/gfs-client \
  --config /app/configs/docker/client-config.yml << 'EOF'
create myfile.txt
append myfile.txt "hello from docker"
read myfile.txt 0 50
ls
exit
EOF

# 5. Stop cluster
docker compose down
```

**How it works:**
```
┌─────────────────── Docker network (gfsnet) ──────────────────┐
│  gfs-master :50051  ←──heartbeat──→  gfs-chunk1 :8080        │
│                     ←──heartbeat──→  gfs-chunk2 :8081        │
│  gfs-client  ──── DNS: master / chunk1 / chunk2 ────────────  │
└───────────────────────────────────────────────────────────────┘
```

---

### External mode *(remote / multi-machine)*

Use this when the client runs on a different machine than the Docker host.

```bash
# 1. Find the server's IP
ip -4 route get 1.1.1.1 | awk '{for(i=1;i<=NF;i++) if($i=="src"){print $(i+1); exit}}'
# e.g. 10.236.97.159

# 2. Set configuration (replace the IP if different)
cp .env.external .env
sed -i 's/10.236.97.159/YOUR_SERVER_IP/g' .env

# 3. Start cluster
docker compose down -v
docker compose up -d --build

# 4. On the CLIENT machine: build the client binary
export PATH=$HOME/go/bin:$PATH
make build-client                    # produces bin/gfs-client

# 5. Connect from client machine
./bin/gfs-client --config configs/external/client-config.yml
```

Port forwarding (host → container):

| Host port | Container | Service |
|-----------|-----------|---------|
| 50051 | gfs-master:50051 | Master gRPC |
| 8080 | gfs-chunk1:8080 | Chunk Server 1 |
| 8081 | gfs-chunk2:8081 | Chunk Server 2 |

---

## Client Commands

| Command | Description |
|---------|-------------|
| `create <filename>` | Create a new file |
| `write <filename> <offset> <data>` | Write data at byte offset |
| `writefile <gfs_file> <offset> <local_path>` | Write a local file into GFS |
| `read <filename> <offset> <length>` | Read bytes from a file |
| `append <filename> <data>` | Atomically append data (GFS record append) |
| `delete <filename>` | Soft-delete a file (moved to trash) |
| `rename <old> <new>` | Rename a file |
| `ls [path]` | List files in namespace |
| `chunks <filename> <start> <end>` | Show chunk handles and replica locations |
| `help` | Show all commands |
| `exit` | Quit |

---

## Folder Structure

```
.
├── api/proto/                  # gRPC service & message definitions
│   ├── chunk/                  #   Chunk ↔ Chunk Server protocol
│   ├── chunk_master/           #   Chunk Server ↔ Master protocol (heartbeat, lease)
│   ├── chunk_operations/       #   Client ↔ Chunk Server operations
│   ├── client_master/          #   Client ↔ Master metadata protocol
│   └── common/                 #   Shared types (ChunkHandle, Status, …)
│
├── cmd/
│   ├── master/main.go          # Master server entry point
│   ├── chunkserver/main.go     # Chunk Server entry point
│   └── client/main.go          # Interactive CLI client
│
├── internal/
│   ├── master/
│   │   ├── master.go           # gRPC handlers: HeartBeat, ReportChunk, GetFileChunksInfo, …
│   │   ├── utils.go            # assignNewPrimary, chunk server management
│   │   ├── metadata.go         # JSON persistence of namespace + chunk metadata
│   │   ├── operation-log.go    # Operation log (write-ahead log before ACK)
│   │   ├── monitor.go          # Background: health checks, re-replication, GC
│   │   └── types.go            # In-memory data structures (FileInfo, ChunkInfo, …)
│   ├── chunkserver/
│   │   ├── chunkserver.go      # Heartbeat loop, command dispatch (BECOME_PRIMARY, INIT_EMPTY, …)
│   │   ├── chunk_operations.go # PushDataToPrimary, WriteChunk, ReadChunk, RecordAppendChunk
│   │   ├── operation_queue.go  # Serialised mutation queue on the Primary
│   │   └── types.go            # ChunkMetadata, lease tracking
│   └── client/
│       ├── client.go           # Create/Delete/Rename/ListNamespace, chunk cache management
│       ├── file_operations.go  # Read / Write / Append with retry + lease-miss handling
│       └── types.go            # Client struct, cache types
│
├── configs/
│   ├── general-config.yml      # Master config (ports, lease timeout, GC intervals)
│   ├── chunkserver-config.yml  # Chunk server config (storage path, heartbeat interval)
│   ├── client-config.yml       # Client config (master address, cache TTL)
│   ├── docker/                 # Configs for Docker internal mode (DNS names)
│   └── external/               # Configs for external / multi-machine mode (IP addresses)
│
├── storage/
│   ├── master/
│   │   ├── metadata.json       # Persisted namespace + chunk metadata (checkpoint)
│   │   └── operation-log.json  # Write-ahead operation log
│   └── chunks/
│       └── server-<host>-<port>/  # One directory per chunk server; *.chunk files
│
├── benchmarking/
│   ├── benchmark.go            # Benchmark runner (auto-starts cluster, 9 benchmarks)
│   ├── cluster.go              # In-process cluster lifecycle manager
│   ├── plot.py                 # Generates PNG plots from JSON results
│   ├── results/                # JSON output files per benchmark
│   └── plots/                  # Generated PNG charts
│
├── tests/
│   └── pytest/                 # Integration test suite (auto-manages cluster)
│       ├── conftest.py         # GFSCluster fixture, binary builder
│       ├── utils/              # Parsing helpers
│       ├── read/               # test_read.py
│       ├── write/              # test_write.py
│       ├── record_append/      # test_record_append.py
│       ├── replication/        # test_replication.py
│       ├── re_replication/     # test_re_replication.py
│       ├── stale_replica_handling/ # test_stale_replica_handling.py
│       └── concurrent/         # test_concurrent.py (8-goroutine concurrent R/W/Append)
│
├── Dockerfile
├── docker-compose.yml
├── Makefile
├── go.mod
└── QUICKSTART.md
```

---

## Testing

Tests are written in pytest and spin up a real GFS cluster automatically — no manual setup required.

```bash
# Install Python dependencies (one time)
pip install pytest

# Run all tests
cd tests/pytest
python3 -m pytest -v

# Run a specific test suite
python3 -m pytest read/          # Read tests
python3 -m pytest write/         # Write + large-file tests
python3 -m pytest record_append/ # Atomic append tests
python3 -m pytest replication/   # Replication factor tests
python3 -m pytest re_replication/         # Re-replication after server failure
python3 -m pytest stale_replica_handling/ # Version-number stale detection
python3 -m pytest concurrent/            # 8-way concurrent reads, writes, appends

# Skip slow tests (>64 MB file tests)
python3 -m pytest -v -m "not slow"
```

Each test:
1. Compiles fresh binaries from source
2. Starts a master + 3 chunk servers on free ports
3. Runs the test scenario
4. Tears the cluster down and removes all temp files

---

## Benchmarking

The benchmark suite auto-manages its own cluster (master on port 50052, chunk servers on 9001–9003) and produces JSON results + PNG charts.

```bash
cd benchmarking

# Run all benchmarks (auto-starts cluster)
go run . -manage-cluster

# Run specific benchmarks
go run . -bench read,write,append

# Run against an already-running cluster
go run . -manage-cluster=false -config ../configs/client-config.yml

# Generate plots from saved results (requires matplotlib)
pip install matplotlib numpy
python3 plot.py -results results/ -out plots/
```

### Benchmark descriptions

| # | Name | What it measures |
|---|------|-----------------|
| 1 | Scalability | Latency & throughput as concurrency scales 1 → 8 |
| 2 | Aggregate Write Throughput | MB/s for N concurrent writers to distinct files |
| 3 | Record Append Concurrency | Throughput + per-op latency for N concurrent appenders on one file |
| 4 | File Size Impact on Latency | End-to-end latency from KB → multi-MB files |
| 5 | Chunk Boundary Overhead | Latency penalty when a write crosses the 1 MB chunk boundary |
| 6 | Master Operation Latency | Metadata ops/sec and lease grant latency |
| 7 | GFS vs. Baseline | GFS pipeline throughput vs. direct single-server write |
| 8 | Sustained Throughput | Single-client MB/s sampled every 5 s over 30 s (reveals ramp-up) |
| 9 | Mixed Workload | Simultaneous readers + writers (2R/2W → 8R/8W configurations) |

Results are saved as JSON in `benchmarking/results/` and charts in `benchmarking/plots/`.

---

## Troubleshooting



**"server is not primary for chunk"**  
This is transient — the master assigns a primary asynchronously. The client retries automatically with a 300 ms back-off. If it persists beyond 3 retries, restart the cluster.

**Address already in use**
```bash
docker compose down
lsof -i :50051 | awk 'NR>1 {print $2}' | xargs kill -9
```

**Proto code not generated**
```bash
export PATH=$HOME/go/bin:$PATH
make proto
```
