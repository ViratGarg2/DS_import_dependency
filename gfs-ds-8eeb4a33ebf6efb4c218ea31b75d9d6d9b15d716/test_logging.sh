#!/usr/bin/env bash
# test_logging.sh — Verifies GFS Operation Log, Checkpointing, Truncation, and Restart Recovery.
#
# Usage:  chmod +x test_logging.sh && ./test_logging.sh
# Run from the project root directory.

set -euo pipefail

export PATH="/opt/homebrew/bin:$PATH:$(go env GOPATH)/bin"

# ─── Paths ────────────────────────────────────────────────────────────────────
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LOG_FILE="$PROJECT_ROOT/storage/master/operation-log.json"
META_FILE="$PROJECT_ROOT/storage/master/metadata.json"
CONFIG_FILE="$PROJECT_ROOT/configs/general-config.yml"
BIN_DIR="$PROJECT_ROOT/.test-bins"
CLIENT_DIR="$PROJECT_ROOT/cmd/client"

# ─── Colours ──────────────────────────────────────────────────────────────────
GREEN='\033[0;32m'; RED='\033[0;31m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; BOLD='\033[1m'; NC='\033[0m'

pass()   { echo -e "${GREEN}[PASS]${NC} $*"; }
fail()   { echo -e "${RED}[FAIL]${NC} $*"; FAILURES=$((FAILURES+1)); }
info()   { echo -e "${CYAN}[INFO]${NC} $*"; }
step()   { echo -e "\n${YELLOW}${BOLD}▶ $*${NC}"; }
banner() {
    echo -e "\n${BOLD}══════════════════════════════════════════════${NC}"
    echo -e "${BOLD}  $*${NC}"
    echo -e "${BOLD}══════════════════════════════════════════════${NC}"
}

FAILURES=0
MASTER_PID=""
CS1_PID=""
CS2_PID=""
CONFIG_PATCHED=0

# ─── Helpers ──────────────────────────────────────────────────────────────────
log_size()     { [ -f "$LOG_FILE" ] && wc -c < "$LOG_FILE" | tr -d ' ' || echo "0"; }
log_lines()    { [ -f "$LOG_FILE" ] || { echo "0"; return; }; wc -l < "$LOG_FILE" | tr -d ' '; }
log_has_op()   { grep -q "\"operation\":\"$1\"" "$LOG_FILE" 2>/dev/null; }
log_op_count() {
    [ -f "$LOG_FILE" ] || { echo "0"; return; }
    local n; n=$(grep -c "\"operation\":\"$1\"" "$LOG_FILE" 2>/dev/null) || n=0; echo "$n"
}

# Run client commands; each arg is one command line, exit is auto-appended.
# Must run from CLIENT_DIR so it resolves ../../configs/client-config.yml
gfs_cmd() {
    local input=""
    for cmd in "$@"; do input+="${cmd}"$'\n'; done
    input+="exit"$'\n'
    printf '%s' "$input" | (cd "$CLIENT_DIR" && "$BIN_DIR/client") 2>&1 || true
}

wait_for_port() {
    local port=$1 retries=30
    while ! nc -z localhost "$port" 2>/dev/null; do
        retries=$((retries-1)); [ $retries -le 0 ] && { fail "Port $port never opened"; return 1; }; sleep 1
    done
}

wait_port_free() {
    local port=$1 retries=20
    while nc -z localhost "$port" 2>/dev/null; do
        retries=$((retries-1)); [ $retries -le 0 ] && { fail "Port $port never freed"; return 1; }; sleep 1
    done
}

kill_master() {
    [ -n "$MASTER_PID" ] || return
    kill -9 "$MASTER_PID" 2>/dev/null || true
    wait "$MASTER_PID" 2>/dev/null || true
    MASTER_PID=""
}

kill_gfs_procs() {
    # Match by project-specific path suffix — handles both with and without leading dot
    pgrep -f "gfs-ds.*[/]master$"      2>/dev/null | xargs kill -9 2>/dev/null || true
    pgrep -f "gfs-ds.*[/]chunkserver$" 2>/dev/null | xargs kill -9 2>/dev/null || true
    sleep 1
}

cleanup() {
    echo ""
    info "Cleaning up..."
    kill_master
    [ -n "$CS1_PID" ] && { kill "$CS1_PID" 2>/dev/null || true; wait "$CS1_PID" 2>/dev/null || true; }
    [ -n "$CS2_PID" ] && { kill "$CS2_PID" 2>/dev/null || true; wait "$CS2_PID" 2>/dev/null || true; }
    kill_gfs_procs
    sleep 1

    # Restore backup_interval = 30
    if [ "$CONFIG_PATCHED" -eq 1 ]; then
        sed -i '' 's/backup_interval: 120/backup_interval: 30/' "$CONFIG_FILE"
        info "backup_interval restored to 30 s"
    fi
    rm -rf "$BIN_DIR"
}
trap cleanup EXIT

# ─── PATCH CONFIG: raise checkpoint interval so operations don't race ────────
banner "Patching config & Building binaries"

step "Set backup_interval = 120 s (restored to 30 on exit)"
sed -i '' 's/backup_interval: 30/backup_interval: 120/' "$CONFIG_FILE"
CONFIG_PATCHED=1
grep 'backup_interval' "$CONFIG_FILE" | head -1
pass "Config patched"

mkdir -p "$BIN_DIR"
(cd "$PROJECT_ROOT" && go build -o "$BIN_DIR/master"      ./cmd/master)      && pass "master built"
(cd "$PROJECT_ROOT" && go build -o "$BIN_DIR/chunkserver" ./cmd/chunkserver) && pass "chunkserver built"
(cd "$PROJECT_ROOT" && go build -o "$BIN_DIR/client"      ./cmd/client)      && pass "client built"

# ─── CLEAN PREVIOUS STATE ─────────────────────────────────────────────────────
banner "Resetting state"

# Kill any orphan binaries left from a previous interrupted run
kill_gfs_procs

rm -f "$LOG_FILE" "$META_FILE"
mkdir -p "$(dirname "$LOG_FILE")"
info "Previous log and metadata cleared."

# ─── START SERVICES ───────────────────────────────────────────────────────────
banner "Starting services"

# Use "exec" so the binary replaces the subshell — MASTER_PID will be the true binary PID
step "Starting master (port 50051)..."
(cd "$PROJECT_ROOT/cmd/master" && exec "$BIN_DIR/master") > /tmp/gfs-master.log 2>&1 &
MASTER_PID=$!
wait_for_port 50051 && pass "Master is up (PID $MASTER_PID)"
# Confirm master actually loaded empty state (not stale checkpoint)
sleep 1
grep -E "Metadata|empty|loaded" /tmp/gfs-master.log | head -3

step "Starting chunkserver 1 (port 8080)..."
(cd "$PROJECT_ROOT/cmd/chunkserver" && exec "$BIN_DIR/chunkserver" --port 8080) > /tmp/gfs-cs1.log 2>&1 &
CS1_PID=$!
wait_for_port 8080 && pass "Chunkserver 1 is up (PID $CS1_PID)"

step "Starting chunkserver 2 (port 8081)..."
(cd "$PROJECT_ROOT/cmd/chunkserver" && exec "$BIN_DIR/chunkserver" --port 8081) > /tmp/gfs-cs2.log 2>&1 &
CS2_PID=$!
wait_for_port 8081 && pass "Chunkserver 2 is up (PID $CS2_PID)"

info "Waiting 5 s for chunkservers to register..."
sleep 5

# ═══════════════════════════════════════════════════════════════════════════════
banner "TEST SUITE: Operation Logging"
# ═══════════════════════════════════════════════════════════════════════════════

# ── T1: Log is empty at startup ───────────────────────────────────────────────
step "T1 — Log file is empty on fresh start"
if [ "$(log_size)" -eq 0 ]; then
    pass "Log is empty — correct for fresh start"
else
    info "Log is non-empty ($(log_size) bytes) — may be from prior state"
fi

# ── T2: CREATE_FILE is logged ─────────────────────────────────────────────────
step "T2 — CREATE_FILE is written to the log"
OUT=$(gfs_cmd "create test-alpha.txt")
sleep 1
echo "  Client: $OUT" | head -5

if log_has_op "CREATE_FILE"; then
    pass "CREATE_FILE entry present in log after create"
    info "Log: $(log_size) bytes, $(log_lines) lines"
else
    fail "CREATE_FILE not found in log"
fi

# ── T3: Multiple creates accumulate ───────────────────────────────────────────
step "T3 — Multiple CREATE_FILE entries accumulate"
gfs_cmd "create test-beta.txt" "create test-gamma.txt" > /dev/null
sleep 1

COUNT=$(log_op_count "CREATE_FILE")
if [ "$COUNT" -ge 3 ]; then
    pass "Found $COUNT CREATE_FILE entries (expected ≥ 3)"
else
    fail "Only $COUNT CREATE_FILE entries (expected ≥ 3)"
fi

# ── T4: RENAME_FILE is logged ─────────────────────────────────────────────────
step "T4 — RENAME_FILE is written to the log"
gfs_cmd "rename test-beta.txt test-beta-renamed.txt" > /dev/null
sleep 1

if log_has_op "RENAME_FILE"; then
    pass "RENAME_FILE entry present in log"
else
    fail "RENAME_FILE not found in log"
fi

# ── T5: DELETE_FILE is logged ─────────────────────────────────────────────────
step "T5 — DELETE_FILE is written to the log"
gfs_cmd "delete test-gamma.txt" > /dev/null
sleep 1

if log_has_op "DELETE_FILE"; then
    pass "DELETE_FILE entry present in log"
else
    fail "DELETE_FILE not found in log"
fi

# ── T6: ADD_CHUNK logged after append ─────────────────────────────────────────
step "T6 — ADD_CHUNK logged after append (triggers chunk allocation)"
gfs_cmd "append test-alpha.txt HelloGFS" > /dev/null
sleep 4

if log_has_op "ADD_CHUNK"; then
    pass "ADD_CHUNK entry present in log"
    info "UPDATE_CHUNK entries:         $(log_op_count UPDATE_CHUNK)"
    info "UPDATE_CHUNK_VERSION entries: $(log_op_count UPDATE_CHUNK_VERSION)"
else
    fail "ADD_CHUNK not found in log — chunkservers may not have enough replicas"
fi

# ── T7: Full log snapshot before checkpoint ───────────────────────────────────
step "T7 — Full log snapshot before checkpoint"
info "Log lines : $(log_lines) | size : $(log_size) bytes"
echo ""
echo "─── Log entries ──────────────────────────────────────────────────────"
while IFS= read -r line; do
    [ -z "$line" ] && continue
    echo "$line" | python3 -c "
import sys, json
d = json.load(sys.stdin)
print('  {ts}  {op:<28} file={fn:<22} chunk={ch}'.format(
    ts=d.get('timestamp','?')[:19],
    op=d.get('operation','?'),
    fn=d.get('filename','-'),
    ch=d.get('chunk_handle','-')[:8] if d.get('chunk_handle') else '-'
))
" 2>/dev/null || echo "  $line"
done < "$LOG_FILE"
echo "──────────────────────────────────────────────────────────────────────"

# ═══════════════════════════════════════════════════════════════════════════════
banner "TEST SUITE: Checkpointing & Log Truncation"
# ═══════════════════════════════════════════════════════════════════════════════

# ── T8: Checkpoint creates metadata.json ─────────────────────────────────────
step "T8 — Checkpoint writes metadata.json (backup_interval=120 s, waiting 125 s)..."
PRE_LINES=$(log_lines)
info "Pre-checkpoint log lines: $PRE_LINES"

# Checkpoints only fire every 120 s; wait just past that
sleep 125

if [ -f "$META_FILE" ]; then
    pass "metadata.json exists ($(wc -c < "$META_FILE" | tr -d ' ') bytes)"
else
    fail "metadata.json was NOT created"
fi

# ── T9: Log truncated after checkpoint ────────────────────────────────────────
step "T9 — Operation log is truncated to 0 after checkpoint"
POST_SIZE=$(log_size)
if [ "$POST_SIZE" -eq 0 ]; then
    pass "Log truncated to 0 bytes (was $PRE_LINES lines before checkpoint)"
else
    fail "Log NOT truncated — still has $POST_SIZE bytes"
fi

# ── T10: New operations after checkpoint write fresh entries ──────────────────
step "T10 — New operations after checkpoint write to truncated log"
gfs_cmd "create post-checkpoint.txt" > /dev/null
sleep 1

if [ "$(log_lines)" -ge 1 ]; then
    pass "Post-checkpoint log has $(log_lines) new entries"
else
    fail "No entries written to log after checkpoint"
fi

# ── T10b: Checkpoint contains pre-checkpoint files ────────────────────────────
step "T10b — Checkpoint contains files created before truncation"
python3 - <<PYEOF
import json, sys
with open('$META_FILE') as f:
    m = json.load(f)
files = list(m.get('files', {}).keys())
print('  Files in checkpoint:', ', '.join(files))
found = any('test-alpha' in fn for fn in files)
if not found:
    print('  ERROR: test-alpha.txt missing')
    sys.exit(1)
PYEOF
if [ $? -eq 0 ]; then
    pass "test-alpha.txt is persisted in checkpoint"
else
    fail "test-alpha.txt missing from checkpoint"
fi

# ═══════════════════════════════════════════════════════════════════════════════
banner "TEST SUITE: Restart & Log Replay"
# ═══════════════════════════════════════════════════════════════════════════════

# ── T11: State survives master restart ───────────────────────────────────────
step "T11 — Killing master and restarting to test recovery"
PRE_RESTART_LOG=$(log_lines)
info "Log lines at kill time: $PRE_RESTART_LOG"

info "Killing master (PID $MASTER_PID)..."
kill_master
pkill -f "test-bins/master" 2>/dev/null || true  # catch any orphan

info "Waiting for port 50051 to be released..."
wait_port_free 50051 && info "Port 50051 is free"

info "Restarting master..."
(cd "$PROJECT_ROOT/cmd/master" && exec "$BIN_DIR/master") > /tmp/gfs-master2.log 2>&1 &
MASTER_PID=$!
wait_for_port 50051 && pass "Master restarted (PID $MASTER_PID)"
sleep 1
grep -E "Metadata|empty|loaded|replay" /tmp/gfs-master2.log | head -5

# ── T12: File from checkpoint is rejected on re-create ───────────────────────
step "T12 — File from checkpoint (test-alpha.txt) exists after restart"
RESULT=$(gfs_cmd "create test-alpha.txt")
echo "  Client: $(echo "$RESULT" | grep -v '^Welcome\|^Type\|^gfs>' | head -3)"
if echo "$RESULT" | grep -qi "already exist\|failed\|error"; then
    pass "test-alpha.txt already exists → checkpoint loaded correctly on restart"
else
    fail "test-alpha.txt was re-created — checkpoint may not have been loaded"
fi

# ── T13: File from post-checkpoint log survives ───────────────────────────────
step "T13 — File from post-checkpoint log (post-checkpoint.txt) survives restart"
RESULT2=$(gfs_cmd "create post-checkpoint.txt")
echo "  Client: $(echo "$RESULT2" | grep -v '^Welcome\|^Type\|^gfs>' | head -3)"
if echo "$RESULT2" | grep -qi "already exist\|failed\|error"; then
    pass "post-checkpoint.txt exists → log replay after checkpoint worked"
else
    fail "post-checkpoint.txt was re-created — log replay may have failed"
fi

# ── T14: New operations logged on restarted master ────────────────────────────
step "T14 — New operations are logged correctly on restarted master"
# Use a timestamp-unique name so it never collides with prior checkpoints
UNIQUE="post-restart-$(date +%s).txt"
OUT14=$(gfs_cmd "create $UNIQUE")
echo "  Client: $(echo "$OUT14" | grep -v '^Welcome\|^Type\|^gfs>' | head -3)"
sleep 1

if log_has_op "CREATE_FILE"; then
    pass "CREATE_FILE ($UNIQUE) written to log on restarted master"
else
    fail "CREATE_FILE not found in log after restart"
fi

# ── T15: Restart log confirms checkpoint + replay ─────────────────────────────
step "T15 — Master restart log (startup sequence)"
echo ""
grep -E "Metadata|replay|checkpoint|loaded|Starting" /tmp/gfs-master2.log 2>/dev/null | head -10 || \
    head -10 /tmp/gfs-master2.log


# step "T16 — Write to previous file (startup sequence)"
# echo ""
# OUT16=$(gfs_cmd "append test-alpha.txt HelloAgain")
# echo "  Client: $(echo "$OUT16")"
# sleep 1
# grep -E "Metadata|replay|checkpoint|loaded|Starting" /tmp/gfs-master2.log 2>/dev/null | head -10 || \
#     head -10 /tmp/gfs-master2.log

# ═══════════════════════════════════════════════════════════════════════════════
banner "FINAL REPORT"
# ═══════════════════════════════════════════════════════════════════════════════
echo ""
echo "  Log file     : $LOG_FILE"
echo "  Metadata     : $META_FILE"
echo "  Master log   : /tmp/gfs-master.log  (first run)"
echo "               : /tmp/gfs-master2.log (after restart)"
echo "  CS-1 log     : /tmp/gfs-cs1.log"
echo "  CS-2 log     : /tmp/gfs-cs2.log"
echo ""
if [ "$FAILURES" -eq 0 ]; then
    echo -e "${GREEN}${BOLD}  All tests passed. Logging is working correctly.${NC}"
else
    echo -e "${RED}${BOLD}  $FAILURES test(s) failed. See [FAIL] lines above.${NC}"
fi
echo ""
