# GFS Deployment System - Getting Started

Welcome! This directory contains a flexible deployment system for GFS (Google File System) that supports running clients both inside Docker containers and on external/host machines.

## 🚀 Quick Start (Choice 1: Use Interactive Setup)

```bash
chmod +x setup.sh
./setup.sh
```

The interactive script will guide you through:
- Selecting Docker or External mode
- Configuring your deployment
- Starting services
- Running the client

**Time to working system:** ~2 minutes

---

## 🎯 Quick Start (Choice 2: Command Line)

### For Local Testing (Docker Mode)
```bash
# Copy Docker configuration
cp .env.docker .env

# Start services
docker compose up -d --build

# Run client inside Docker container
docker exec -it gfs-client /usr/local/bin/gfs-client \
  --config /app/configs/docker/client-config.yml

# Try these commands:
# gfs> create test.txt
# gfs> append test.txt "Hello GFS"
# gfs> read test.txt 0 100
# gfs> exit
```

### For Multi-Machine Setup (External Mode)
```bash
# Copy External configuration
cp .env.external .env

# Update your server IP (if different from 10.236.97.159):
sed -i 's/10.236.97.159/YOUR_SERVER_IP/g' .env

# Start services
docker compose up -d --build

# On client machine (host or remote):
export PATH=$HOME/go/bin:$PATH
make build-client
./bin/gfs-client --config configs/external/client-config.yml
```

---

## 📚 Documentation

Choose your starting point:

### 1. **[QUICKSTART.md](QUICKSTART.md)** ← START HERE
Best for: Getting up and running quickly
- Quick examples for both modes
- Common commands reference
- Basic troubleshooting

### 2. **[DEPLOYMENT_MODES.md](DEPLOYMENT_MODES.md)** ← Read next
Best for: Understanding the architecture
- How each mode works
- Configuration details
- Production recommendations

### 3. **[DEPLOYMENT_GUIDE.md](DEPLOYMENT_GUIDE.md)**
Best for: Deep dive and complex scenarios
- Detailed architecture diagrams
- Advanced configuration
- Multi-machine deployments

### 4. **[README.md](README.md)** 
Complete project documentation

---

## 🐳 What Changed

This deployment system adds:

### New Configuration Files
- `.env.docker` - Docker mode configuration (internal DNS)
- `.env.external` - External mode configuration (IP-based)
- `.env` - Active configuration (you'll choose)

### New Client Configurations
- `configs/docker/client-config.yml` - For Docker clients
- `configs/external/client-config.yml` - For external clients

### New Build Targets (Makefile)
```bash
make build-client        # Build GFS client binary locally
make build-master        # Build master locally
make build-chunkserver   # Build chunkserver locally
make build-all           # Build all
```

### New Documentation
- `QUICKSTART.md` - Quick reference
- `DEPLOYMENT_MODES.md` - Comprehensive guide
- `setup.sh` - Interactive setup helper
- This file!

### Updated Docker Compose
- Parameterized services using `.env` variables
- Supports both internal and external modes
- Automatically switches based on configuration

---

## 🤔 Which Mode Should I Use?

| Scenario | Mode | When |
|----------|------|------|
| Testing locally | **Docker** | Learning, development, quick testing |
| Client on same machine | **Docker** | Using `docker exec` from host |
| Client on different machine | **External** | Multi-machine setup, remote client |
| Production | **External** | Separate client and server machines |
| CI/CD pipeline | **Docker** | Container-based testing |

---

## 💡 Key Concepts

### Docker Mode
- **Client location:** Inside Docker container
- **Client config:** Uses container names (`master`, `chunk1`, `chunk2`)
- **How it works:** Docker network DNS resolves names
- **Use case:** Local testing, development
- **Advantage:** Simple, no IP configuration needed

### External Mode
- **Client location:** Host machine or remote device
- **Client config:** Uses IP addresses (e.g., `10.236.97.159`)
- **How it works:** Docker port forwarding exposes services to host
- **Use case:** Multi-machine deployments
- **Advantage:** Separates client and server infrastructure

---

## 🔧 Common Tasks

### Test If Everything Works
```bash
# Docker mode
docker exec -i gfs-client /usr/local/bin/gfs-client \
  --config /app/configs/docker/client-config.yml << 'EOF'
create test.txt
append test.txt "test"
read test.txt 0 10
ls
exit
EOF

# External mode
./bin/gfs-client --config configs/external/client-config.yml << 'EOF'
# Same commands as above
EOF
```

### View Service Logs
```bash
docker logs gfs-master          # Master logs
docker logs gfs-chunk1          # Chunk 1 logs
docker logs gfs-chunk2          # Chunk 2 logs
docker logs gfs-client          # Client logs (Docker mode)
```

### Clean Restart
```bash
docker compose down -v          # Remove everything
docker compose up -d --build    # Rebuild and start fresh
```

### Switch Between Modes
```bash
# To Docker mode:
cp .env.docker .env
docker compose down -v
docker compose up -d --build

# To External mode:
cp .env.external .env
# Edit IP if needed: sed -i 's/10.236.97.159/YOUR_IP/g' .env
docker compose down -v
docker compose up -d --build
```

---

## ⚠️ Troubleshooting Quick Reference

| Problem | Solution |
|---------|----------|
| "lookup chunk1: no such host" | You're in external mode but trying Docker config. `cp .env.external .env` |
| Connection refused | Services not running. `docker compose ps` to check |
| Port already in use | Stop other services: `docker compose down` |
| Proto compilation error | Install: `go install google.golang.org/protobuf/cmd/protoc-gen-go@latest` |
| Can't reach from remote | Firewall blocking ports. Allow 50051, 8080, 8081 |

For more help, see [QUICKSTART.md](QUICKSTART.md) or [DEPLOYMENT_MODES.md](DEPLOYMENT_MODES.md).

---

## 📋 File Tree

```
.
├── README.md                           # Main project README
├── QUICKSTART.md                       # Quick reference (START HERE)
├── DEPLOYMENT_MODES.md                 # Comprehensive deployment guide
├── DEPLOYMENT_GUIDE.md                 # Detailed architecture
├── setup.sh                            # Interactive setup helper
│
├── .env.docker                         # Docker mode config template
├── .env.external                       # External mode config template
├── .env                                # Active config (created from template)
│
├── docker-compose.yml                  # Services with env var params
├── Dockerfile                          # Build image
├── Makefile                            # Build targets
│
├── configs/
│   ├── docker/
│   │   └── client-config.yml           # Docker mode: uses "master" DNS
│   ├── external/
│   │   └── client-config.yml           # External mode: uses server IP
│   ├── chunkserver-config.yml
│   └── general-config.yml
│
├── cmd/
│   ├── client/main.go                  # Client source
│   ├── master/main.go                  # Master source
│   └── chunkserver/main.go             # Chunkserver source
│
├── internal/
│   ├── client/                         # Client implementation
│   ├── master/                         # Master implementation
│   └── chunkserver/                    # Chunkserver implementation
│
├── api/proto/                          # Protocol buffer definitions
│
├── bin/
│   ├── gfs-client                      # Built client binary (local)
│   ├── gfs-master                      # Built master binary (local)
│   └── gfs-chunkserver                 # Built chunkserver binary (local)
│
└── storage/                            # Runtime data (created by Docker)
    ├── chunks/
    ├── master/
    └── ...
```

---

## 🎓 Learning Path

1. **5 min:** Read this file → understand Docker vs External mode
2. **10 min:** Run `./setup.sh` → get system running
3. **5 min:** Try basic operations (create, append, read, ls)
4. **15 min:** Read [QUICKSTART.md](QUICKSTART.md) → learn commands
5. **20 min:** Read [DEPLOYMENT_MODES.md](DEPLOYMENT_MODES.md) → understand architecture
6. **Optional:** Read [DEPLOYMENT_GUIDE.md](DEPLOYMENT_GUIDE.md) → advanced topics

---

## 🚢 Ready to Deploy?

### Development/Testing
1. `./setup.sh` → Select "1" (Docker Mode)
2. Follow prompts
3. Use Docker client for testing

### Production (Multi-Machine)
1. Server machine: `./setup.sh` → Select "2" (External Mode)
2. Enter server IP address
3. Start services: `docker compose up -d --build`
4. On client machines:
   - Build: `make build-client`
   - Run: `./bin/gfs-client --config configs/external/client-config.yml`

---

## ❓ FAQ

**Q: Can I run both Docker and External clients at the same time?**
A: No, you need to switch modes. Stop services, change `.env`, restart.

**Q: What if I want to use different ports?**
A: Edit `.env` to change `GFS_*_PORT` variables (advanced).

**Q: How do I persist data between restarts?**
A: Data is automatically persisted in Docker volumes. Use `docker volume ls` to see them.

**Q: Can I run 3+ chunk servers?**
A: Yes, edit docker-compose.yml to add more `chunk3`, `chunk4` services.

**Q: How do I monitor what's happening?**
A: Use `docker logs` for each service. See logs section in QUICKSTART.md.

---

## 📞 Support

- Check [QUICKSTART.md](QUICKSTART.md) Troubleshooting section
- Review service logs: `docker logs gfs-master`
- Verify configuration: `cat .env`
- See [DEPLOYMENT_MODES.md](DEPLOYMENT_MODES.md) for detailed help

---

## Summary

You now have a flexible GFS deployment system that works in two ways:

- **Docker Mode**: Perfect for testing locally with `docker exec`
- **External Mode**: Perfect for production with remote clients

Choose your starting point:
1. **Quick:** Run `./setup.sh`
2. **Immediate:** Use Docker Mode commands above
3. **Learn:** Read QUICKSTART.md

Happy GFS-ing! 🚀
