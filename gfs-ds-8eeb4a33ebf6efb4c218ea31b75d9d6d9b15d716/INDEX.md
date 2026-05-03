# GFS Multi-Mode Deployment - Complete System Guide

## 📖 Complete Documentation Index

### 🎯 Starting Points

**Choose ONE based on your preference:**

1. **[GETTINGSTARTED.md](GETTINGSTARTED.md)** ← **NEW USERS START HERE** ⭐
   - Overview of both deployment modes
   - Quick choices based on your scenario
   - Learning path progression
   - 5-minute read

2. **[setup.sh](setup.sh)** ← **Prefer Interactive Setup?** 👈 **RUN THIS**
   - Interactive menu-driven setup
   - Automatically configures your deployment
   - Asks for server IP in external mode
   - No manual editing needed

3. **[QUICKSTART.md](QUICKSTART.md)** ← **Want to Start Immediately?**
   - Copy-paste ready commands
   - Side-by-side mode comparison
   - Common operations examples
   - 10-minute quick start

### 📚 Reference Documentation

4. **[DEPLOYMENT_MODES.md](DEPLOYMENT_MODES.md)** - Architecture & Design
   - How Docker mode works (internal DNS)
   - How External mode works (IP + port mapping)
   - Configuration details
   - Production recommendations

5. **[DEPLOYMENT_GUIDE.md](DEPLOYMENT_GUIDE.md)** - Detailed Guide
   - Multi-machine setups
   - Advanced configurations
   - Cross-machine networking
   - Troubleshooting in depth

6. **[VERIFICATION_CHECKLIST.md](VERIFICATION_CHECKLIST.md)** - Quality Assurance
   - Step-by-step verification for both modes
   - Health checks
   - Stress testing procedures
   - Success criteria

### 📋 Implementation Details

7. **[IMPLEMENTATION_SUMMARY.md](IMPLEMENTATION_SUMMARY.md)** - What Changed
   - All new files created
   - All files modified
   - System architecture changes
   - Before/after comparison

---

## 🚀 Quick Decision Tree

```
"Which mode should I use?"

├─ Running client inside Docker container?
│  └─ YES → Docker Mode ✅
│     (containers can reach each other by name)
│
├─ Running client on HOST MACHINE?
│  ├─ Same as server? → Docker Mode ✅ (use docker exec)
│  └─ Different? → External Mode ✅
│
└─ Running client on REMOTE DEVICE?
   └─ YES → External Mode ✅
      (needs IP + firewall access)
```

---

## 🎬 Three Ways to Get Started

### Option 1: Interactive Setup (Recommended) ⭐
```bash
chmod +x setup.sh
./setup.sh
# Follow the menu
```
**Time: 2-3 minutes | Skills needed: None**

### Option 2: Docker Mode Quick Start
```bash
cp .env.docker .env
docker compose down -v && docker compose up -d --build
docker exec -it gfs-client /usr/local/bin/gfs-client \
  --config /app/configs/docker/client-config.yml
```
**Time: 1-2 minutes | Skills needed: Basic Docker**

### Option 3: Manual External Mode
```bash
cp .env.external .env
sed -i 's/10.236.97.159/YOUR_SERVER_IP/g' .env
docker compose down -v && docker compose up -d --build
export PATH=$HOME/go/bin:$PATH && make build-client
./bin/gfs-client --config configs/external/client-config.yml
```
**Time: 3-5 minutes | Skills needed: Docker + Go**

---

## 📊 Deployment Mode Comparison

| Feature | Docker | External |
|---------|--------|----------|
| Client location | Inside container | Host or remote |
| Setup time | 1 min | 3 min |
| Configuration complexity | Simple (DNS) | Medium (IP) |
| Use case | Testing, dev | Production, remote |
| Firewall needed | No | Yes |
| Port mapping | Yes | Yes |
| Data sharing | Between containers | Across network |
| Performance | Excellent | Good |

---

## 🔧 System Components

### Configuration Files
- **`.env.docker`** - Internal Docker mode template
- **`.env.external`** - External mode template
- **`.env`** - Active configuration (user creates from template)
- **`configs/docker/client-config.yml`** - Docker client configuration
- **`configs/external/client-config.yml`** - External client configuration

### Services (docker-compose.yml)
- **gfs-master** - Metadata & coordination server (port 50051)
- **gfs-chunk1** - First chunk server (port 8080)
- **gfs-chunk2** - Second chunk server (port 8081)
- **gfs-client** - Client container (Docker mode only)

### Build Targets (Makefile)
```bash
make proto              # Generate .pb.go files from .proto
make build-client       # Build GFS client binary
make build-master       # Build GFS master binary
make build-chunkserver  # Build GFS chunkserver binary
make build-all          # Build all binaries
make help               # Show all targets
```

### Helper Scripts
- **`setup.sh`** - Interactive setup (menu-driven)
- **`test_external_mode.sh`** - External mode test script

---

## 📖 Reading Paths by User Type

### 👨‍💻 Developer (Local Testing)
1. Read: [GETTINGSTARTED.md](GETTINGSTARTED.md#-user-profiles)
2. Run: `./setup.sh` → Select Docker Mode
3. Read: [QUICKSTART.md](QUICKSTART.md) for operations
4. Reference: [DEPLOYMENT_MODES.md](DEPLOYMENT_MODES.md) for architecture

### 🚀 Production Ops (Deploying to Production)
1. Read: [DEPLOYMENT_MODES.md](DEPLOYMENT_MODES.md)
2. Setup: External mode via  `./setup.sh` or manual
3. Verify: Use [VERIFICATION_CHECKLIST.md](VERIFICATION_CHECKLIST.md)
4. Reference: [DEPLOYMENT_GUIDE.md](DEPLOYMENT_GUIDE.md) for advanced

### 🏗️ Architect (Designing Deployment)
1. Read: [DEPLOYMENT_MODES.md](DEPLOYMENT_MODES.md) - Architecture section
2. Review: [DEPLOYMENT_GUIDE.md](DEPLOYMENT_GUIDE.md) - All scenarios
3. Check: [VERIFICATION_CHECKLIST.md](VERIFICATION_CHECKLIST.md) - Success criteria
4. Reference: [IMPLEMENTATION_SUMMARY.md](IMPLEMENTATION_SUMMARY.md) - What changed

### 📚 Learning New System
1. **5 min:** [GETTINGSTARTED.md](GETTINGSTARTED.md) - Overview
2. **10 min:** Choose & run setup option
3. **15 min:** Try basic operations
4. **20 min:** Read [DEPLOYMENT_MODES.md](DEPLOYMENT_MODES.md)
5. **30 min:** Read security & monitoring sections

---

## 🎯 Common Tasks

### Switch Deployment Modes
```bash
# From Docker to External
cp .env.external .env
sed -i 's/10.236.97.159/YOUR_IP/g' .env
docker compose down -v && docker compose up -d --build

# From External to Docker
cp .env.docker .env
docker compose down -v && docker compose up -d --build
```

### Build Client for Remote Machine
```bash
export PATH=$HOME/go/bin:$PATH
make build-client
# Binary at: ./bin/gfs-client
```

### View Service Logs
```bash
docker logs gfs-master           # Master logs
docker logs gfs-chunk1           # Chunk 1 logs
docker logs gfs-chunk2           # Chunk 2 logs
docker logs gfs-client           # Client logs (Docker mode)
```

### Test Connectivity
```bash
# From client machine to server
timeout 3 bash -c 'cat < /dev/null > /dev/tcp/SERVER_IP/50051'
# Should succeed with no output
```

### Create Test Files
```bash
# Docker mode
docker exec -i gfs-client /usr/local/bin/gfs-client \
  --config /app/configs/docker/client-config.yml << 'EOF'
create test.txt
append test.txt "test data"
read test.txt 0 100
exit
EOF

# External mode
./bin/gfs-client --config configs/external/client-config.yml << 'EOF'
create test.txt
append test.txt "test data"
read test.txt 0 100
exit
EOF
```

---

## ✅ Verification

### Minimal Verification (2 min)
```bash
docker compose ps  # All 4 services should be Up

# Test one operation
docker exec -i gfs-client /usr/local/bin/gfs-client \
  --config /app/configs/docker/client-config.yml << 'EOF'
create test.txt
append test.txt "works"
read test.txt 0 100
exit
EOF
# Should see: "Successfully read..."
```

### Full Verification (10 min)
Use [VERIFICATION_CHECKLIST.md](VERIFICATION_CHECKLIST.md) - Follows systematic verification for:
- Pre-deployment
- Docker mode
- External mode
- Cross-mode consistency
- Performance baseline

---

## 🆘 Quick Troubleshooting

| Issue | Check | Fix |
|-------|-------|-----|
| Services not running | `docker compose ps` | `docker compose up -d --build` |
| Connection refused | `docker logs gfs-master` | Check for errors in startup |
| "lookup chunk1: no such host" | Mode mismatch | `cp .env.docker .env` |
| Proto compilation error | `which protoc-gen-go` | Install: `go install google.golang.org/protobuf/cmd/protoc-gen-go@latest` |
| Port already in use | `docker compose ps` | `docker compose down` |
| Can't reach from remote | Test: `timeout 3 bash -c 'cat < /dev/null > /dev/tcp/IP/PORT'` | Check firewall, verify IP |

Full troubleshooting: See respective guide (QUICKSTART, DEPLOYMENT_MODES, or DEPLOYMENT_GUIDE)

---

## 📦 What's New

### Files Created (9)
✅ `.env.docker` - Docker mode config template  
✅ `.env.external` - External mode config template  
✅ `configs/external/client-config.yml` - External client config  
✅ `setup.sh` - Interactive setup script  
✅ `GETTINGSTARTED.md` - This is where users start  
✅ `QUICKSTART.md` - Quick reference guide  
✅ `DEPLOYMENT_MODES.md` - Architecture guide  
✅ `VERIFICATION_CHECKLIST.md` - QA checklist  
✅ `IMPLEMENTATION_SUMMARY.md` - Change documentation  

### Files Modified (2)
✅ `docker-compose.yml` - Parameterized for both modes  
✅ `Makefile` - Added build targets  

### Key Features
✅ **No code changes needed** - Config-based via .env  
✅ **Easy switching** - Just copy different template  
✅ **Two deployment modes** - Docker internal + External  
✅ **Production ready** - For local to enterprise use  
✅ **Comprehensive docs** - 5 guides for different users  
✅ **Interactive setup** - Automated via setup.sh  

---

## 🎓 Learning Resources

### For Understanding GFS
- See `README.md` - Project overview
- See `docs/` directory - Architecture documentation
- See protocol buffer definitions - See `api/proto/` directory

### For Understanding Deployment
- [DEPLOYMENT_MODES.md](DEPLOYMENT_MODES.md) - How modes work
- [DEPLOYMENT_GUIDE.md](DEPLOYMENT_GUIDE.md) - Advanced scenarios

### For Hands-On Learning
1. Start with Docker mode (easier)
2. Follow [QUICKSTART.md](QUICKSTART.md)
3. Try all operations: create, append, read, delete, etc.
4. Progress to External mode when ready

---

## 🚀 Now What?

### If you want to...

**...build and run it NOW** 
→ Go to [GETTINGSTARTED.md](GETTINGSTARTED.md)

**...prefer interactive setup**
→ Run `./setup.sh`

**...want quick reference**
→ Read [QUICKSTART.md](QUICKSTART.md)

**...need to understand system design**
→ Read [DEPLOYMENT_MODES.md](DEPLOYMENT_MODES.md)

**...verify everything works**
→ Use [VERIFICATION_CHECKLIST.md](VERIFICATION_CHECKLIST.md)

**...understand what changed**
→ Read [IMPLEMENTATION_SUMMARY.md](IMPLEMENTATION_SUMMARY.md)

---

## 📞 Key Takeaways

1. **Two modes available**: Docker (internal) and External (cross-machine)
2. **Easy to switch**: Just copy a different `.env` file
3. **No code changes**: Configuration-based via environment variables
4. **Production ready**: Works for local dev to enterprise deployments
5. **Well documented**: 5+ guides for different scenarios
6. **Automated setup**: Interactive script handles most decisions

---

## 🎯 Quick Reference Card

```
┌─────────────────────────────────────────────────────────┐
│ GFS MULTI-MODE DEPLOYMENT QUICK REFERENCE              │
├─────────────────────────────────────────────────────────┤
│                                                         │
│ Start here:     → GETTINGSTARTED.md                     │
│ Interactive:    → ./setup.sh                            │
│ Quick commands: → QUICKSTART.md                         │
│ Architecture:   → DEPLOYMENT_MODES.md                   │
│ Full guide:     → DEPLOYMENT_GUIDE.md                   │
│ Verify setup:   → VERIFICATION_CHECKLIST.md             │
│                                                         │
│ Docker Mode:    cp .env.docker .env                     │
│ External Mode:  cp .env.external .env                   │
│                                                         │
│ Start services: docker compose up -d --build            │
│ Docker client:  docker exec -it gfs-client ...          │
│ Build external: make build-client                       │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

---

## ✨ Ready?

1. Choose your path above
2. Follow the instructions
3. You'll have a fully functional GFS deployment in 2-5 minutes

**Let's get started! 🚀**
