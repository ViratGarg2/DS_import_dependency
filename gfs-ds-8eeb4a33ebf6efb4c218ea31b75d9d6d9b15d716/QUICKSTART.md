# Quick Start Guide: GFS Multi-Mode Deployment

This guide covers both Docker internal and external client modes.

## Prerequisites

- Docker & Docker Compose installed
- Go 1.23+ (for building client binaries locally)
- Protoc installed (with Go plugins, for building locally)

---

## Mode Selection Matrix

| Scenario | Recommended | Config File | Client Location |
|----------|-------------|-------------|-----------------|
| Testing locally, client in Docker | Docker Mode | `.env.docker` | Inside container |
| Client on host machine | Docker Mode + Docker CLI | `.env.docker` | Host via `docker exec` |
| Client on different machine on network | External Mode | `.env.external` | Remote device |
| Multi-region deployment | External Mode | `.env.external` | Multiple remote locations |

---

## Quick Start: Docker Mode (Recommended for Testing)

### Step 1: Prepare Docker Configuration
```bash
cd /path/to/gfs-project

# Use Docker internal mode (default, fastest for testing)
cp .env.docker .env

# Verify .env is correct
cat .env | head -3
# Should show: GFS_MODE=docker, GFS_CHUNK1_HOST=chunk1, etc.
```

### Step 2: Start Services
```bash
docker compose down -v    # Clean slate (optional, removes old data)
docker compose up -d --build
```

### Step 3: Use Client Inside Docker Container
```bash
# Interactive mode
docker exec -it gfs-client /usr/local/bin/gfs-client \
  --config /app/configs/docker/client-config.yml

# Running commands
docker exec -i gfs-client /usr/local/bin/gfs-client \
  --config /app/configs/docker/client-config.yml << 'EOF'
create myfile.txt
append myfile.txt "test data"
read myfile.txt 0 100
ls
exit
EOF
```

---

## Quick Start: External Mode (For Remote Clients)

### Step 1: Identify Server IP
```bash
# Find your server's IP
hostname -I
# Example output: 10.236.97.159

# Or use:  
ip addr show | grep "inet " | grep -v 127.0.0.1
```

### Step 2: Configure External Mode
```bash
cd /path/to/gfs-project

# Copy external config
cp .env.external .env

# Edit with your server IP (replace 10.236.97.159 if different)
sed -i 's/10.236.97.159/YOUR_SERVER_IP/g' .env

# Verify
cat .env | grep "GFS_.*_HOST"
```

### Step 3: Start Services with External Mode
```bash
docker compose down -v    # Clean slate
docker compose up -d --build
```

### Step 4: Build Client (On host/remote machine)
```bash
# On your client machine, clone the repo and build:
cd /path/to/gfs-project

# Ensure proto tools are available
export PATH=$HOME/go/bin:$PATH

# Build client
make build-client

# Binary at: ./bin/gfs-client
```

### Step 5: Run Client from Host/Remote
```bash
# On host/remote machine:
./bin/gfs-client --config configs/external/client-config.yml

# Then in the interactive prompt:
create file.txt
append file.txt "data from external machine"
read file.txt 0 100
ls
exit
```

---

## File Operations Reference

### Available Commands
```
create <filename>                    # Create new file
append <filename> "<data>"           # Append data to file
read <filename> <offset> <length>    # Read from file
delete <filename>                    # Delete file
rename <oldname> <newname>           # Rename file
ls [path]                           # List files
chunks <filename>                    # List chunks for file
help                                # Show help
exit                                # Exit client
```

### Example Session
```
gfs> create document.txt
File created successfully: document.txt

gfs> append document.txt "Hello World"
Successfully appended data at offset 0

gfs> read document.txt 0 50
Successfully read 11 bytes:
"Hello World"

gfs> append document.txt " - Extended content"
Successfully appended data at offset 11

gfs> read document.txt 0 100
Successfully read 30 bytes:
"Hello World - Extended content"

gfs> ls
/
`-- document.txt

gfs> chunks document.txt
Chunks for document.txt:
- Chunk 1: [Server addresses...]

gfs> exit
Goodbye!
```

---

## Switching Between Modes

### Switch to Docker Mode
```bash
cd /path/to/gfs-project
cp .env.docker .env
docker compose down -v
docker compose up -d --build
docker exec -it gfs-client /usr/local/bin/gfs-client \
  --config /app/configs/docker/client-config.yml
```

### Switch to External Mode
```bash
cd /path/to/gfs-project
cp .env.external .env
# Edit IP if needed: sed -i 's/10.236.97.159/YOUR_IP/g' .env
docker compose down -v
docker compose up -d --build
# On client machine: ./bin/gfs-client --config configs/external/client-config.yml
```

---

## Troubleshooting

### Issue: Can't reach master from external client
**Check:**
```bash
# From client machine, test connectivity to master
timeout 3 bash -c 'cat < /dev/null > /dev/tcp/10.236.97.159/50051'
# Should succeed without error

# Test chunk ports too:
timeout 3 bash -c 'cat < /dev/null > /dev/tcp/10.236.97.159/8080'
timeout 3 bash -c 'cat < /dev/null > /dev/tcp/10.236.97.159/8081'
```

**Solution:** Ensure firewall allows ports 50051, 8080, 8081. Update client config IP if needed.

### Issue: "No available chunk servers" error
**Check:**
```bash
# Verify chunks are running
docker compose ps | grep chunk

# Check chunk logs
docker logs gfs-chunk1 | grep -i "error\|advertise"
docker logs gfs-chunk2 | grep -i "error\|advertise"
```

**Solution:** Restart services with clean state: `docker compose down -v && docker compose up -d --build`

### Issue: Address already in use
**Solution:**
```bash
# Stop conflicting containers
docker compose down

# Or if specific port conflicts:
docker ps -a | grep -E "8080|8081|50051"
docker stop <container_id>
```

### Issue: Proto code not generated
**Solution (for local development):**
```bash
# Ensure protoc tools are in PATH
export PATH=$HOME/go/bin:$PATH

# Regenerate protos
make proto

# Then build
make build-client
```

---

## Configuration Details

### Docker Mode (.env.docker)
```
GFS_MODE=docker
GFS_CHUNK1_HOST=chunk1      # DNS inside Docker network
GFS_CHUNK2_HOST=chunk2      # DNS inside Docker network
GFS_MASTER_HOST=master      # DNS inside Docker network
```

**Client Config:** `configs/docker/client-config.yml`
```yaml
connection:
  master:
    host: "master"   # Resolves inside Docker network
    port: 50051
```

**How it works:**
- Services run in Docker network (gfsnet)
- Container names resolve automatically via Docker DNS
- Clients inside Docker use container names

### External Mode (.env.external)
```
GFS_MODE=external
GFS_CHUNK1_HOST=10.236.97.159      # Server's public IP
GFS_CHUNK2_HOST=10.236.97.159      # Server's public IP
GFS_MASTER_HOST=10.236.97.159      # Server's public IP
```

**Client Config:** `configs/external/client-config.yml`
```yaml
connection:
  master:
    host: "10.236.97.159"   # Server's actual IP
    port: 50051
```

**How it works:**
- Services still in Docker, but advertise external IP
- Docker port forwarding exposes ports to host
- External clients reach servers via public IP + host port mappings
- Host port 50051 → Docker 50051 (Master gRPC)
- Host port 8080 → Docker 8080 (Chunk1 gRPC)
- Host port 8081 → Docker 8081 (Chunk2 gRPC)

---

## Architecture Diagrams

### Docker Mode (Internal Clients)
```
┌─────────────────────────────────────┐
│    Docker Host (10.236.97.159)     │
│                                     │
│  ┌──────────────────────────────┐  │
│  │   Docker Network (gfsnet)    │  │
│  │                              │  │
│  │  master ←──→ chunk1          │  │
│  │       ↑         ↑            │  │
│  │       │         │            │  │
│  │   client (DNS resolution)    │  │
│  │                              │  │
│  └──────────────────────────────┘  │
│                                     │
└─────────────────────────────────────┘

Client uses: dns://master:50051
            dns://chunk1:8080
            dns://chunk2:8081
```

### External Mode (Remote Clients)
```
┌─────────────────────────────────────┐
│    Docker Host (10.236.97.159)     │
│                                     │
│     :50051 ──→ gfs-master :50051   │
│     :8080  ──→ gfs-chunk1 :8080    │
│     :8081  ──→ gfs-chunk2 :8081    │
│                                     │
└─────────────────────────────────────┘
              ↑      ↑       ↑
              │      │       │
       ┌──────┴──────┼───────┘
       │             │
    Host Client  Remote Client
    ./bin/gfs-   ./bin/gfs-
    client        client

Uses IP: 10.236.97.159:50051, :8080, :8081
```

---

## Production Recommendations

1. **Use External Mode for:**
   - Multi-machine deployments
   - Separating client and server infrastructure
   - Cloud deployments (clients in different regions)

2. **Use Docker Mode for:**
   - Local development and testing
   - Single-machine deployments
   - CI/CD environments

3. **Security:**
   - Client mode: Restrict network access to Docker bridge
   - External mode: Use firewall to limit port access
   - Use VPN for inter-machine communication in production

4. **Monitoring:**
   - Check logs: `docker logs gfs-master`, `docker logs gfs-chunk1/2`
   - Test connectivity regularly
   - Monitor port usage: `netstat -tlnp | grep -E "50051|8080|8081"`

---

## See Also

- [DEPLOYMENT_GUIDE.md](DEPLOYMENT_GUIDE.md) - Detailed architecture and advanced setup
- [Makefile](Makefile) - Build targets including `make build-client`
- `configs/docker/client-config.yml` - Docker mode client configuration
- `configs/external/client-config.yml` - External mode client configuration

