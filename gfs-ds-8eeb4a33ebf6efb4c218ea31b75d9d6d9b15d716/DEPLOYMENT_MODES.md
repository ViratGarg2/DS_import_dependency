# GFS Multi-Mode Client Deployment System

A flexible deployment system for the Google File System (GFS) implementation that supports both internal Docker clients and external clients on different machines.

## Overview

This system allows you to run GFS with clients in two ways:

1. **Docker Mode** - Client runs inside Docker container, uses internal DNS
2. **External Mode** - Client runs on host or remote machine, uses IP addresses

---

## Quick Setup

### Use the Setup Script (Recommended)
```bash
chmod +x setup.sh
./setup.sh
```

This interactive script will:
- Guide you through mode selection
- Create appropriate `.env` file
- Show you how to start services and run client

---

## Manual Setup

### Docker Mode (Default)
```bash
cp .env.docker .env
docker compose down -v    # Clean slate
docker compose up -d --build

# Use client inside container:
docker exec -it gfs-client /usr/local/bin/gfs-client \
  --config /app/configs/docker/client-config.yml
```

### External Mode
```bash
cp .env.external .env
# Edit IP if different: sed -i 's/10.236.97.159/YOUR_IP/g' .env

docker compose down -v
docker compose up -d --build

# On client machine, build and run:
export PATH=$HOME/go/bin:$PATH
make build-client
./bin/gfs-client --config configs/external/client-config.yml
```

---

## File Structure

### Configuration Files
```
.env.docker              # Docker mode environment config (internal DNS)
.env.external            # External mode environment config (IP addresses)
.env                     # Active configuration (copy of one of the above)

configs/
├── docker/
│   └── client-config.yml     # Docker mode client config (uses "master" DNS)
├── external/
│   └── client-config.yml     # External mode client config (uses server IP)
├── chunkserver-config.yml
└── general-config.yml
```

### Documentation
```
README.md                # This file
QUICKSTART.md            # Quick reference guide
DEPLOYMENT_GUIDE.md      # Detailed architecture and setup
setup.sh                 # Interactive setup script
Makefile                 # Build targets (proto, build-client, etc)
```

### Key Differences in Configuration

**Docker Mode (.env.docker)**
```
GFS_CHUNK1_HOST=chunk1
GFS_CHUNK2_HOST=chunk2
GFS_MASTER_HOST=master
```
- Uses container names (resolved by Docker DNS inside network)
- No external IP needed
- Fastest for local testing

**External Mode (.env.external)**
```
GFS_CHUNK1_HOST=10.236.97.159
GFS_CHUNK2_HOST=10.236.97.159
GFS_MASTER_HOST=10.236.97.159
```
- Uses server's public IP address
- Allows clients on different machines
- Requires firewall to allow ports 50051, 8080, 8081

---

## How It Works

### Docker Mode Architecture
```
┌─────────────────────────────────┐
│   Docker Network (gfsnet)       │
│                                 │
│  ┌─────────┐  ┌─────────┐      │
│  │ master  │  │ chunk1  │      │
│  │ :50051  │  │ :8080   │      │
│  └────┬────┘  └────┬────┘      │
│       │            │            │
│    ┌──┴──────────┐ │            │
│    │ gfs-client  │◄┘            │
│    │ (DNS)       │              │
│    └─────────────┘              │
│                                 │
└─────────────────────────────────┘

Configuration:
- master: chunk1:8080, chunk2:8081 (advertised)
- client config host: "master" (DNS resolves in Docker network)
```

### External Mode Architecture
```
┌─────────────────────────────────────┐
│  Docker Host (10.236.97.159)       │
│                                     │
│  Port Mapping:                      │
│  :50051 ──→ master :50051 (gRPC)   │
│  :8080  ──→ chunk1 :8080  (gRPC)   │
│  :8081  ──→ chunk2 :8081  (gRPC)   │
│                                     │
│  Advertise: 10.236.97.159:8080/8081│
│                                     │
└─────────────────────────────────────┘
      │           │          │
      ▼           ▼          ▼
   Host Client  Remote Device 1  Remote Device N
   ~/.bin/gfs-client
   Uses: 10.236.97.159:50051

Configuration:
- master: 10.236.97.159:8080, 10.236.97.159:8081 (advertised via IP)
- client config host: "10.236.97.159"
```

---

## Operations

### Available Commands in GFS Client
```
create <filename>                    # Create new file
delete <filename>                    # Delete file
rename <old> <new>                   # Rename file
append <file> "<data>"               # Append to file
read <file> <offset> <length>        # Read from file
ls [path]                           # List files
chunks <filename>                    # Show chunk info
help                                # Show help
exit                                # Exit
```

### Example: Multi-Step Operation
```bash
# Inside gfs-client prompt:
gfs> create document.txt
File created successfully: document.txt

gfs> append document.txt "Hello World"
Successfully appended data at offset 0

gfs> append document.txt " - More data"
Successfully appended data at offset 11

gfs> read document.txt 0 100
Successfully read 23 bytes:
"Hello World - More data"

gfs> ls
/
`-- document.txt

gfs> exit
Goodbye!
```

---

## Environment Variables Reference

All variables with defaults shown (can be overridden in `.env`):

| Variable | Docker | External | Purpose |
|----------|--------|----------|---------|
| `GFS_MODE` | `docker` | `external` | Deployment mode |
| `GFS_CHUNK1_HOST` | `chunk1` | `10.236.97.159` | Chunk1 advertised hostname |
| `GFS_CHUNK2_HOST` | `chunk2` | `10.236.97.159` | Chunk2 advertised hostname |
| `GFS_MASTER_HOST` | `master` | `10.236.97.159` | Master advertised hostname |
| `GFS_MASTER_PORT` | `50051` | `50051` | Master gRPC port |
| `GFS_CHUNK1_PORT` | `8080` | `8080` | Chunk1 gRPC port |
| `GFS_CHUNK2_PORT` | `8081` | `8081` | Chunk2 gRPC port |

---

## Build Targets

The `Makefile` provides several targets:

```bash
make proto              # Generate protobuf code from .proto files
make build-client       # Build only client binary
make build-master       # Build only master binary
make build-chunkserver  # Build only chunkserver binary
make build-all          # Build all binaries (default)
make build              # Alias for build-all
make clean              # Remove generated code and binaries
make help               # Show all targets
```

### Building for External Mode

On host/remote machine:
```bash
# Set up Go bin directory in PATH
export PATH=$HOME/go/bin:$PATH

# Build client
make build-client

# Binary now at: ./bin/gfs-client
```

---

## Docker Compose Integration

All services are parameterized via environment variables:

### Services
- **gfs-master** - Master server (port 50051)
- **gfs-chunk1** - First chunk server (port 8080)
- **gfs-chunk2** - Second chunk server (port 8081)
- **gfs-client** - Client container (for Docker mode)

### Networks
- **gfsnet** - Docker bridge network for internal communication

### Volumes
- **master-data** - Master metadata persistence
- **chunk1-data** - Chunk1 storage
- **chunk2-data** - Chunk2 storage

### Port Mappings
These are dynamically set from `.env` variables:
- External port → Container port
- All ports configurable via environment variables

---

## Troubleshooting

### "No such container" error
**Cause:** Client container not running in Docker mode
```bash
docker compose up -d client
```

### Connection refused
**Cause:** Services not running
```bash
docker compose ps  # Check status
docker compose up -d --build  # Start if stopped
```

### "lookup chunk1: no such host" in external mode
**Cause:** Trying to use Docker mode config in external mode situation
```bash
cp .env.external .env
docker compose down -v
docker compose up -d --build
```

### Proto compilation errors
**Cause:** protoc tools not installed
```bash
# Install tools
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.35.1
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.3.0

# Ensure they're in PATH
export PATH=$HOME/go/bin:$PATH
make proto
```

### Port already in use
**Cause:** Another process or Docker service using the port
```bash
# Stop all services
docker compose down

# Or find and stop specific containers
docker ps -a | grep -E "gfs|8080|8081|50051"
docker stop <container_id>
```

---

## Production Checklist

- [ ] Choose appropriate mode (Docker for single machine, External for multi-machine)
- [ ] Update `.env` with correct IP addresses for your deployment
- [ ] Test connectivity: `nc -zv <host> <port>` for all 3 ports
- [ ] Verify firewall rules allow necessary ports
- [ ] Monitor service logs regularly
- [ ] Set up automated backups of chunk volumes
- [ ] Plan for failure scenarios and recovery
- [ ] Document your deployment configuration

---

## Advanced: Custom Configuration

### Multi-Port Setup
If you want to run multiple GFS instances, modify `.env` to use different ports:
```env
GFS_MODE=external
GFS_CHUNK1_HOST=10.236.97.159
GFS_CHUNK2_HOST=10.236.97.159
GFS_MASTER_PORT=50051
GFS_CHUNK1_PORT=9080
GFS_CHUNK2_PORT=9081
```

### Multi-Chunk Deployment
To add more chunk servers, edit `docker-compose.yml` and add new services like `chunk3`, `chunk4` with appropriate ports and environment variables.

### Custom Client Config
For special requirements, create a custom client config:
```bash
cp configs/external/client-config.yml configs/my-custom-config.yml
# Edit as needed
./bin/gfs-client --config configs/my-custom-config.yml
```

---

## See Also

- [QUICKSTART.md](QUICKSTART.md) - Quick reference for common tasks
- [DEPLOYMENT_GUIDE.md](DEPLOYMENT_GUIDE.md) - Detailed deployment architecture
- [Makefile](Makefile) - Available build targets
- `cmd/client/main.go` - Client source code
- `cmd/chunkserver/main.go` - Chunkserver source code
- `cmd/master/main.go` - Master source code

---

## Support

For issues or questions:
1. Check the logs: `docker compose logs gfs-master`
2. Verify connectivity: `timeout 3 bash -c 'cat < /dev/null > /dev/tcp/HOST/PORT'`
3. Review configuration: `cat .env`
4. See TROUBLESHOOTING section above

---

## License

Same as parent GFS project.
