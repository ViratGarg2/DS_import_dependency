# GFS Deployment Verification Checklist

Use this checklist to verify your GFS deployment is working correctly.

## Pre-Deployment Checklist

- [ ] Docker installed: `docker --version`
- [ ] Docker Compose installed: `docker compose --version`
- [ ] Go installed (for building locally): `go version`
- [ ] Git repository cloned: `ls -la .git`
- [ ] All files present: `ls -la | grep -E "docker-compose|Dockerfile|Makefile"`

## Docker Mode Setup Verification

### Step 1: Environment Configuration
```bash
[ ] cp .env.docker .env
[ ] grep "GFS_MODE=docker" .env
[ ] grep "GFS_CHUNK1_HOST=chunk1" .env
```

### Step 2: Services Start
```bash
[ ] docker compose down -v
[ ] docker compose up -d --build
[ ] docker compose ps           # All 4 services should show "Up"
```

### Step 3: Service Health Checks
```bash
# Master
[ ] docker logs gfs-master | grep -i "running"
[ ] docker logs gfs-master | grep -i "listening"

# Chunk1
[ ] docker logs gfs-chunk1 | grep -i "running"
[ ] docker logs gfs-chunk1 | grep "advertise=chunk1:8080"

# Chunk2
[ ] docker logs gfs-chunk2 | grep -i "running"
[ ] docker logs gfs-chunk2 | grep "advertise=chunk2:8081"
```

### Step 4: Heartbeat Verification
```bash
[ ] docker logs gfs-master | grep "Received HeartBeat" | wc -l
    # Should show heartbeats from both chunks (look for at least 2)
```

### Step 5: Basic Operations
```bash
# Create
[ ] docker exec -i gfs-client /usr/local/bin/gfs-client \
      --config /app/configs/docker/client-config.yml << 'EOF'
    create verify.txt
    exit
    EOF
    # Should show: "File created successfully"

# Append
[ ] docker exec -i gfs-client /usr/local/bin/gfs-client \
      --config /app/configs/docker/client-config.yml << 'EOF'
    append verify.txt "test data"
    exit
    EOF
    # Should show: "Successfully appended"

# Read
[ ] docker exec -i gfs-client /usr/local/bin/gfs-client \
      --config /app/configs/docker/client-config.yml << 'EOF'
    read verify.txt 0 10
    exit
    EOF
    # Should show: "Successfully read X bytes"

# List
[ ] docker exec -i gfs-client /usr/local/bin/gfs-client \
      --config /app/configs/docker/client-config.yml << 'EOF'
    ls
    exit
    EOF
    # Should list: verify.txt
```

## External Mode Setup Verification

### Step 1: Environment Configuration
```bash
[ ] cp .env.external .env
[ ] grep "GFS_MODE=external" .env
[ ] grep "GFS_CHUNK1_HOST=10.236.97.159" .env
    # (or your actual server IP)
```

### Step 2: Verify Server IP
```bash
[ ] hostname -I | awk '{print $1}'
    # Note this IP for next steps

[ ] sed -i 's/10.236.97.159/YOUR_ACTUAL_IP/g' .env
    # Update if different
```

### Step 3: Services Start
```bash
[ ] docker compose down -v
[ ] docker compose up -d --build
[ ] docker ps | grep -E "gfs-master|gfs-chunk"
    # Should show running containers
```

### Step 4: Service Configuration Verification
```bash
# Verify chunk servers are advertising external IP
[ ] docker logs gfs-chunk1 | grep -i "advertise=10.236.97.159:8080"
[ ] docker logs gfs-chunk2 | grep -i "advertise=10.236.97.159:8081"
```

### Step 5: Network Connectivity (from client machine)
```bash
# Test master port
[ ] timeout 3 bash -c 'cat < /dev/null > /dev/tcp/10.236.97.159/50051'
    # Should succeed with no output

# Test chunk1 port
[ ] timeout 3 bash -c 'cat < /dev/null > /dev/tcp/10.236.97.159/8080'
    # Should succeed with no output

# Test chunk2 port
[ ] timeout 3 bash -c 'cat < /dev/null > /dev/tcp/10.236.97.159/8081'
    # Should succeed with no output
```

### Step 6: Build Client Binary
```bash
# On client machine
[ ] export PATH=$HOME/go/bin:$PATH
[ ] which protoc-gen-go
    # Should show path to protoc-gen-go

[ ] which protoc-gen-go-grpc
    # Should show path to protoc-gen-go-grpc

[ ] make build-client
    # Should show "Building GFS client..."

[ ] ls -lh bin/gfs-client
    # Should show binary file (~15MB)
```

### Step 7: Test External Client Operations
```bash
# On external/remote client machine
[ ] export PATH=$HOME/go/bin:$PATH

[ ] ./bin/gfs-client --config configs/external/client-config.yml << 'EOF'
    create external-test.txt
    append external-test.txt "External client data"
    read external-test.txt 0 30
    ls
    exit
    EOF
    # All commands should complete successfully
```

## Cross-Mode Verification

### Verify Data Consistency
After completing both modes, verify data is shared:

```bash
# Create file in Docker mode
[ ] docker exec -i gfs-client /usr/local/bin/gfs-client \
      --config /app/configs/docker/client-config.yml << 'EOF'
    create shared-file.txt
    append shared-file.txt "Created in Docker"
    exit
    EOF

# Read from external client
[ ] ./bin/gfs-client --config configs/external/client-config.yml << 'EOF'
    read shared-file.txt 0 100
    exit
    EOF
    # Should see: "Created in Docker"

# Append from external client
[ ] ./bin/gfs-client --config configs/external/client-config.yml << 'EOF'
    append shared-file.txt " - Modified from external"
    exit
    EOF

# Verify in Docker mode
[ ] docker exec -i gfs-client /usr/local/bin/gfs-client \
      --config /app/configs/docker/client-config.yml << 'EOF'
    read shared-file.txt 0 100
    exit
    EOF
    # Should see both messages
```

## Performance Baseline

Record these for future comparison:

```
Docker Mode Client Operations:
- Create file: ___ ms
- Append 1KB: ___ ms
- Read 1KB: ___ ms
- List 10 files: ___ ms

External Mode Client Operations:
- Create file: ___ ms
- Append 1KB: ___ ms
- Read 1KB: ___ ms
- List 10 files: ___ ms
```

## Stress Testing (Optional)

### Create Multiple Files
```bash
# In Docker mode
for i in {1..100}; do
  docker exec -i gfs-client /usr/local/bin/gfs-client \
    --config /app/configs/docker/client-config.yml << EOF
create file_$i.txt
append file_$i.txt "Content $i"
exit
EOF
done

docker exec -i gfs-client /usr/local/bin/gfs-client \
  --config /app/configs/docker/client-config.yml << EOF
ls
exit
EOF
# Should list all 100 files
```

### Large Data Operations
```bash
# Create large file
docker exec -i gfs-client /usr/local/bin/gfs-client \
  --config /app/configs/docker/client-config.yml << EOF
create large.txt
EOF

# Append large data (multiple times)
for i in {1..10}; do
  docker exec -i gfs-client /usr/local/bin/gfs-client \
    --config /app/configs/docker/client-config.yml << EOF
append large.txt "$(head -c 1000 /dev/urandom | base64)"
exit
EOF
done

# Verify
docker exec -i gfs-client /usr/local/bin/gfs-client \
  --config /app/configs/docker/client-config.yml << EOF
read large.txt 0 100
exit
EOF
```

## Cleanup and Verification

### Clean State Test
```bash
# Stop everything
[ ] docker compose down -v

# Restart fresh
[ ] docker compose up -d --build

# Verify services start from clean state
[ ] docker ps
[ ] docker logs gfs-master | grep -i "running"
```

### Final Checklist
- [ ] All services running and healthy
- [ ] Both modes tested and working
- [ ] Data persists across operations
- [ ] No errors in logs
- [ ] Documentation accessible
- [ ] Setup script working
- [ ] One mode selected and configured in `.env`

## Success Criteria

Your deployment is **READY** when:

✅ All 4 services (master, chunk1, chunk2, client) show "Up" status
✅ Heartbeats being received by master (check logs)
✅ Docker mode client: create → append → read all work
✅ External mode client: build succeeds and operations work
✅ No error messages in service logs
✅ `.env` file configured for your chosen mode
✅ `setup.sh` runs without errors

## Troubleshooting by Symptom

| Symptom | Check | Fix |
|---------|-------|-----|
| No containers running | `docker compose ps` | `docker compose up -d --build` |
| Connection refused | `docker logs gfs-master` | Check for startup errors, verify ports |
| Proto error | `which protoc-gen-go` | Install: `go install google.golang.org/protobuf/cmd/protoc-gen-go@latest` |
| Can't reach from remote | Test connectivity to IPs/ports | Check firewall allows 50051, 8080, 8081 |
| File operations hang | Check master logs | Look for error messages or stuck operations |
| Other errors | Check all logs | `docker logs gfs-master gfs-chunk1 gfs-chunk2` |

---

Once all checkboxes are marked, your GFS deployment is verified and ready for use!
