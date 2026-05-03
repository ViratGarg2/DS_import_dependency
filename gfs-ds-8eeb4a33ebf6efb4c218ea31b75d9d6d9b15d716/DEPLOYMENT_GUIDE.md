# GFS Deployment Guide: Docker & External Clients

This guide explains how to deploy and run GFS with two different client modes:
1. **Docker Mode** - Client runs inside Docker container
2. **External Mode** - Client runs on host machine or remote device

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                    GFS Server Machine                           │
│                  (10.236.97.159 in this example)                │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │          Docker Network (gfsnet - 172.20.0.0/16)        │   │
│  │                                                           │   │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐   │   │
│  │  │ gfs-master   │  │ gfs-chunk1   │  │ gfs-chunk2   │   │   │
│  │  │ :50051       │  │ :8080        │  │ :8081        │   │   │
│  │  │ 172.20.0.5   │  │ 172.20.0.4   │  │ 172.20.0.3   │   │   │
│  │  └──────────────┘  └──────────────┘  └──────────────┘   │   │
│  │                                                           │   │
│  │  ┌──────────────────────────────────────────────────┐   │   │
│  │  │         gfs-client (Docker Mode)                 │   │   │
│  │  │   Uses: master, chunk1, chunk2 (DNS names)      │   │   │
│  │  └──────────────────────────────────────────────────┘   │   │
│  └──────────────────────────────────────────────────────────┘   │
│         │               │               │                       │
│         └───────────────┼───────────────┘                       │
│                         │                                        │
│        Port Mappings:   │                                        │
│        50051 ────────────┤ Master gRPC                           │
│        8080  ────────────┤ Chunk1 gRPC                           │
│        8081  ────────────┤ Chunk2 gRPC                           │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
         │                    │                    │
         │                    │                    │
    ┌────▼──────┐        ┌────▼──────┐       ┌────▼──────┐
    │   Host    │        │  Remote   │       │  Docker   │
    │  Machine  │        │  Device   │       │  Client   │
    │           │        │           │       │           │
    │  External │        │ External  │       │  Docker   │
    │   Config  │        │  Config   │       │   Config  │
    │           │        │           │       │           │
    └───────────┘        └───────────┘       └───────────┘
```

---

## Mode 1: Docker Internal Client

**Use Case:** Client runs inside Docker container, accessing other containers by name.

### Setup

```bash
cd /path/to/gfs-project

# Copy Docker mode environment config
cp .env.docker .env

# Start services
docker compose up -d --build
```

### Running Client

**Method A: Interactive Shell**
```bash
docker exec -it gfs-client /usr/local/bin/gfs-client \
  --config /app/configs/docker/client-config.yml
```

**Method B: Script with Commands**
```bash
docker exec -i gfs-client /usr/local/bin/gfs-client \
  --config /app/configs/docker/client-config.yml << 'EOF'
create myfile.txt
append myfile.txt "Hello World"
read myfile.txt 0 100
ls
exit
EOF
```

### Configuration Details

**Environment Variables** (`.env.docker`)
```
GFS_MODE=docker
GFS_CHUNK1_HOST=chunk1        # DNS name resolves inside Docker network
GFS_CHUNK2_HOST=chunk2        # DNS name resolves inside Docker network
GFS_MASTER_HOST=master        # DNS name resolves inside Docker network
```

**Client Config** (`configs/docker/client-config.yml`)
```yaml
connection:
  master:
    host: "master"            # Uses Docker container name
    port: 50051
```

**Chunkserver Addresses Advertised to Master:**
- `chunk1:8080` → Resolves to `172.20.0.4:8080` inside Docker network
- `chunk2:8081` → Resolves to `172.20.0.3:8081` inside Docker network

---

## Mode 2: External Client (Host or Remote Device)

**Use Case:** Client runs outside Docker (host machine or remote device) and needs to reach Docker services via exposed ports.

### Setup

```bash
cd /path/to/gfs-project

# Copy External mode environment config (edit IP if needed)
cp .env.external .env

# If using on a remote device, update the IP in .env:
# Edit .env and change GFS_CHUNK1_HOST, GFS_CHUNK2_HOST, GFS_MASTER_HOST
# to your server's actual IP address

# Start services
docker compose up -d --build
```

### Building the Client Binary

**On Host/Remote Machine:**
```bash
cd /path/to/gfs-project

# Ensure Go environment is configured
configure_python_environment or source your Python venv if needed

# Build client binary
make build-client

# The binary will be at ./bin/gfs-client
```

### Running External Client

**Method A: Interactive Mode**
```bash
./bin/gfs-client --config configs/external/client-config.yml
```

**Method B: Script with Commands**
```bash
./bin/gfs-client --config configs/external/client-config.yml << 'EOF'
create testfile.txt
append testfile.txt "Data from external client"
read testfile.txt 0 100
ls
exit
EOF
```

### Configuration Details

**Environment Variables** (`.env.external`)
```
GFS_MODE=external
GFS_CHUNK1_HOST=10.236.97.159      # Server's actual IP
GFS_CHUNK2_HOST=10.236.97.159      # Server's actual IP
GFS_MASTER_HOST=10.236.97.159      # Server's actual IP
```

**Client Config** (`configs/external/client-config.yml`)
```yaml
connection:
  master:
    host: "10.236.97.159"           # Server's actual IP
    port: 50051
```

**Chunkserver Addresses Advertised to Master:**
- `10.236.97.159:8080` → Reaches chunk1 via Docker port mapping
- `10.236.97.159:8081` → Reaches chunk2 via Docker port mapping

**Docker Port Mappings:**
```
Host:50051 → Container:50051 (Master gRPC)
Host:8080  → Container:8080  (Chunk1 gRPC)
Host:8081  → Container:8081  (Chunk2 gRPC)
```

---

## Switching Between Modes

### Switch from Docker to External

```bash
# Stop current services
docker compose down

# Copy external config
cp .env.external .env

# Edit IP if different from 10.236.97.159
# nano .env

# Restart
docker compose up -d --build

# Run client on host
./bin/gfs-client --config configs/external/client-config.yml
```

### Switch from External to Docker

```bash
# Stop current services
docker compose down

# Copy docker config
cp .env.docker .env

# Restart
docker compose up -d --build

# Run client in Docker container
docker exec -it gfs-client /usr/local/bin/gfs-client \
  --config /app/configs/docker/client-config.yml
```

---

## Troubleshooting

### Issue: "lookup chunk1: no such host" (Docker Mode)

**Cause:** Docker client trying to use external IP

**Solution:**
```bash
# Ensure .env is set to docker mode
cat .env
# Should show: GFS_MODE=docker

# Restart services
docker compose down
cp .env.docker .env
docker compose up -d --build

# Use the Docker client command
docker exec -it gfs-client /usr/local/bin/gfs-client ...
```

### Issue: "connection refused" (External Mode)

**Cause:** Firewall blocking ports or incorrect IP

**Solution:**
```bash
# Check firewall allows ports
nc -zv 10.236.97.159 50051
nc -zv 10.236.97.159 8080
nc -zv 10.236.97.159 8081

# Verify correct IP in .env
cat .env | grep GFS_.*_HOST

# Check Docker containers are running
docker compose ps

# Check port mappings
docker compose ps | grep -E "chunk|master"
```

### Issue: "No available chunk servers with valid primaries" (External Mode)

**Cause:** Client config pointing to wrong host

**Solution:**
```bash
# Verify client config
cat configs/external/client-config.yml | grep "host:"

# Should show the server's actual IP
# Update if necessary

# Restart client
./bin/gfs-client --config configs/external/client-config.yml
```

### Issue: Chunks not responding after switching modes

**Cause:** Stale state in volumes

**Solution:**
```bash
# Full cleanup
docker compose down -v

# Copy correct .env
cp .env.docker .env  # or .env.external

# Restart
docker compose up -d --build
```

---

## Advanced: Multi-Machine Setup

For a setup where GFS server runs on one machine and clients on multiple different devices:

### Server Machine (10.236.97.159)

```bash
cp .env.external .env
docker compose up -d --build
```

### Client Machine 1 (192.168.1.100)

```bash
# Create external config pointing to server
cat > client1-config.yml << 'EOF'
connection:
  master:
    host: "10.236.97.159"
    port: 50051
connection:
  max_retries: 3
  retry_interval: 1
EOF

./bin/gfs-client --config client1-config.yml
```

### Client Machine 2 (192.168.1.101)

```bash
# Same setup, just different client location
# Both can reach server at 10.236.97.159
./bin/gfs-client --config client2-config.yml
```

### Client Inside Docker (on server machine)

```bash
# Use docker mode client
docker exec -it gfs-client /usr/local/bin/gfs-client \
  --config /app/configs/docker/client-config.yml
```

---

## Environment Variable Summary

| Variable | Docker | External |
|----------|--------|----------|
| `GFS_MODE` | `docker` | `external` |
| `GFS_CHUNK1_HOST` | `chunk1` | `10.236.97.159` |
| `GFS_CHUNK2_HOST` | `chunk2` | `10.236.97.159` |
| `GFS_MASTER_HOST` | `master` | `10.236.97.159` |
| `GFS_CHUNK1_PORT` | `8080` | `8080` |
| `GFS_CHUNK2_PORT` | `8081` | `8081` |
| `GFS_MASTER_PORT` | `50051` | `50051` |

---

## Quick Start Scripts

**Start Docker Client Fast:**
```bash
#!/bin/bash
cd /path/to/gfs-project
cp .env.docker .env
docker compose down -v
docker compose up -d --build
docker exec -it gfs-client /usr/local/bin/gfs-client \
  --config /app/configs/docker/client-config.yml
```

**Start External Client Fast:**
```bash
#!/bin/bash
cd /path/to/gfs-project
cp .env.external .env
docker compose down -v
docker compose up -d --build
# On host machine:
./bin/gfs-client --config configs/external/client-config.yml
```

---

## Summary

- **Docker Mode**: Use for local testing with client in container. Simple DNS-based addressing.
- **External Mode**: Use for cross-machine setups. Requires firewall access to ports 50051, 8080, 8081.
- **Switching**: Just copy different `.env` file and restart compose.
- **Key Difference**: Internal mode uses container names (chunk1, chunk2), external mode uses IP addresses.
