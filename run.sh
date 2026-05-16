#!/usr/bin/env bash
# ============================================================================
# run.sh — one-stop bring-up / tear-down for the AI CFO stack.
#
# Usage:
#   ./run.sh start       # start backend + frontend + (optional) llama-server
#   ./run.sh stop        # stop everything we started
#   ./run.sh status      # show which services are listening
#   ./run.sh logs <svc>  # tail logs (svc = backend | frontend | llama)
#   ./run.sh check       # verify prerequisites without starting anything
#   ./run.sh test        # run the python e2e suite against the running stack
#
# Environment overrides — copy `.env.example` to `.env` and edit, or export
# in your shell before running. All variables are documented in README.md.
# ============================================================================

set -euo pipefail

# ----------------------------------------------------------------------------
# Resolve paths relative to this script so it works from any cwd.
# ----------------------------------------------------------------------------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# ----------------------------------------------------------------------------
# Auto-load a .env file if present. The file is plain "KEY=value" lines,
# comments start with #. Export everything so child processes inherit it.
# ----------------------------------------------------------------------------
if [[ -f "$SCRIPT_DIR/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$SCRIPT_DIR/.env"
  set +a
fi

# ----------------------------------------------------------------------------
# Defaults — match backend/internal/config/config.go. Keep these aligned.
# ----------------------------------------------------------------------------
: "${PORT:=8080}"
: "${FRONTEND_PORT:=3000}"
: "${LLAMA_SERVER_PORT:=8081}"
: "${DATA_DIR:=./backend/data}"
: "${LLAMA_CPP_BINARY:=./llama.cpp/main}"
: "${MODEL_PATH:=./backend/models/gemma.gguf}"
: "${MODEL_NAME:=gemma}"
: "${LLM_MAX_TOKENS:=512}"
: "${LLM_TEMPERATURE:=0.2}"
: "${LLM_TOP_P:=0.9}"
: "${LLM_SEED:=42}"
: "${LLM_CONTEXT_SIZE:=4096}"
: "${LLM_TIMEOUT_SEC:=120}"
: "${LLM_THREADS:=0}"
: "${SQLITE_ENABLED:=true}"
: "${VECTOR_BACKEND:=memory}"
: "${LICENSE_FILE:=./license.lic}"
: "${LICENSE_STATE:=./backend/data/state/license.state.enc}"
: "${START_LLAMA_SERVER:=false}"   # the standalone Gemma chat UI on :8081
: "${LOG_DIR:=/tmp/cfo-logs}"

# Resolve to absolute paths so subprocess cwd changes don't break lookups.
# This is essential because we `cd backend` before launching the Go binary;
# a relative DATA_DIR like "./backend/data" would otherwise become
# "./backend/backend/data" once inside the subprocess.
abs_path() { python3 -c "import os,sys; print(os.path.abspath(sys.argv[1]))" "$1" 2>/dev/null || echo "$1"; }
DATA_DIR="$(abs_path "$DATA_DIR")"
LLAMA_CPP_BINARY="$(abs_path "$LLAMA_CPP_BINARY")"
MODEL_PATH="$(abs_path "$MODEL_PATH")"
LOG_DIR="$(abs_path "$LOG_DIR")"
LICENSE_FILE="$(abs_path "$LICENSE_FILE")"
LICENSE_STATE="$(abs_path "$LICENSE_STATE")"
: "${SQLITE_PATH:=$DATA_DIR/state/cfo.db}"
SQLITE_PATH="$(abs_path "$SQLITE_PATH")"

export PORT DATA_DIR LLAMA_CPP_BINARY MODEL_PATH MODEL_NAME \
       LLM_MAX_TOKENS LLM_TEMPERATURE LLM_TOP_P LLM_SEED \
       LLM_CONTEXT_SIZE LLM_TIMEOUT_SEC LLM_THREADS \
       SQLITE_ENABLED SQLITE_PATH VECTOR_BACKEND \
       QDRANT_URL QDRANT_COLLECTION QDRANT_API_KEY EMBEDDING_DIM \
       LICENSE_FILE LICENSE_STATE

mkdir -p "$LOG_DIR"

# ----------------------------------------------------------------------------
# Color helpers (only when stdout is a TTY).
# ----------------------------------------------------------------------------
if [[ -t 1 ]]; then
  C_RED=$'\033[31m';   C_GRN=$'\033[32m'; C_YEL=$'\033[33m'
  C_BLU=$'\033[34m';   C_BLD=$'\033[1m';  C_RST=$'\033[0m'
else
  C_RED=""; C_GRN=""; C_YEL=""; C_BLU=""; C_BLD=""; C_RST=""
fi

ok()   { echo "${C_GRN}OK${C_RST}    $*"; }
warn() { echo "${C_YEL}WARN${C_RST}  $*"; }
err()  { echo "${C_RED}ERROR${C_RST} $*" >&2; }
info() { echo "${C_BLU}INFO${C_RST}  $*"; }

# ============================================================================
# Prerequisite checks
# ============================================================================
check_deps() {
  local fail=0

  if ! command -v go >/dev/null 2>&1; then
    err "go is not installed. Install Go 1.21+ from https://go.dev/dl/"
    fail=1
  else
    ok "go: $(go version | awk '{print $3}')"
  fi

  if ! command -v node >/dev/null 2>&1; then
    err "node is not installed. Install Node 18+ from https://nodejs.org/"
    fail=1
  else
    ok "node: $(node --version)"
  fi

  if ! command -v npm >/dev/null 2>&1; then
    err "npm is not installed (comes with Node)."
    fail=1
  else
    ok "npm: $(npm --version)"
  fi

  if ! command -v python3 >/dev/null 2>&1; then
    warn "python3 not found — './run.sh test' will not work."
  else
    ok "python3: $(python3 --version)"
  fi

  if [[ ! -x "$LLAMA_CPP_BINARY" ]]; then
    err "llama.cpp binary missing or not executable: $LLAMA_CPP_BINARY"
    err "  Build it: git clone https://github.com/ggerganov/llama.cpp && cd llama.cpp && make"
    err "  Then symlink: ln -s build/bin/llama-completion main"
    fail=1
  else
    ok "llama.cpp binary: $LLAMA_CPP_BINARY"
  fi

  if [[ ! -f "$MODEL_PATH" ]]; then
    err "Gemma GGUF model missing: $MODEL_PATH"
    err "  Download with: hf download google/gemma-2-9b-it-GGUF gemma-2-9b-it-Q4_K_M.gguf --local-dir ./backend/models"
    err "  Then rename: mv ./backend/models/gemma-*.gguf ./backend/models/gemma.gguf"
    fail=1
  else
    local size_mb
    size_mb=$(du -m "$MODEL_PATH" | awk '{print $1}')
    ok "Gemma model: $MODEL_PATH (${size_mb} MB)"
  fi

  if [[ $fail -ne 0 ]]; then
    err "Prerequisites missing — fix the above and re-run."
    exit 1
  fi
}

# ============================================================================
# Port helpers
# ============================================================================
port_pid() {
  local p="$1"
  lsof -nP -iTCP:"$p" -sTCP:LISTEN 2>/dev/null | awk 'NR==2 {print $2}' || true
}

ensure_port_free() {
  local p="$1"; local name="$2"
  local pid; pid="$(port_pid "$p")"
  if [[ -n "$pid" ]]; then
    err "Port $p ($name) is in use by PID $pid. Run './run.sh stop' first or kill it manually."
    exit 1
  fi
}

wait_for_port() {
  local p="$1"; local name="$2"; local tries="${3:-30}"
  for ((i=0; i<tries; i++)); do
    if [[ -n "$(port_pid "$p")" ]]; then
      ok "$name listening on :$p"
      return 0
    fi
    sleep 1
  done
  err "$name did not come up on :$p within ${tries}s. Check $LOG_DIR/${name}.log"
  return 1
}

# ============================================================================
# start / stop / status / logs / test
# ============================================================================
cmd_start() {
  check_deps

  info "Starting AI CFO stack…"
  info "  data dir   : $DATA_DIR"
  info "  log dir    : $LOG_DIR"
  info "  llama bin  : $LLAMA_CPP_BINARY"
  info "  model      : $MODEL_PATH"
  info "  ports      : backend=$PORT  frontend=$FRONTEND_PORT  llama-server=$LLAMA_SERVER_PORT"
  info ""

  ensure_port_free "$PORT"          "backend"
  ensure_port_free "$FRONTEND_PORT" "frontend"
  if [[ "$START_LLAMA_SERVER" == "true" ]]; then
    ensure_port_free "$LLAMA_SERVER_PORT" "llama-server"
  fi

  # ---- Backend ----
  info "Launching Go backend…"
  (
    cd backend
    nohup go run ./cmd/server >"$LOG_DIR/backend.log" 2>&1 &
    echo $! >"$LOG_DIR/backend.pid"
  )
  wait_for_port "$PORT" "backend" 60

  # ---- Frontend (only if dependencies installed) ----
  if [[ ! -d frontend/node_modules ]]; then
    warn "frontend/node_modules missing — running 'npm install' (one-time, takes ~30s)"
    (cd frontend && npm install --silent)
  fi
  info "Launching Vite frontend…"
  (
    cd frontend
    nohup npm run dev >"$LOG_DIR/frontend.log" 2>&1 &
    echo $! >"$LOG_DIR/frontend.pid"
  )
  wait_for_port "$FRONTEND_PORT" "frontend" 30

  # ---- Optional standalone llama-server (chat UI for raw model debugging) ----
  if [[ "$START_LLAMA_SERVER" == "true" ]]; then
    info "Launching llama-server on :$LLAMA_SERVER_PORT (standalone Gemma chat UI)…"
    nohup ./llama.cpp/build/bin/llama-server \
      -m "$MODEL_PATH" -c "$LLM_CONTEXT_SIZE" --port "$LLAMA_SERVER_PORT" \
      >"$LOG_DIR/llama-server.log" 2>&1 &
    echo $! >"$LOG_DIR/llama-server.pid"
    wait_for_port "$LLAMA_SERVER_PORT" "llama-server" 60
  fi

  echo ""
  ok "Stack is up. Open ${C_BLD}http://localhost:$FRONTEND_PORT${C_RST} in your browser."
  if [[ "$START_LLAMA_SERVER" == "true" ]]; then
    info "Raw Gemma chat UI:  http://localhost:$LLAMA_SERVER_PORT"
  fi
  info "Backend API:        http://localhost:$PORT/health"
  info "Run tests:          ./run.sh test"
  info "Stop everything:    ./run.sh stop"
}

cmd_stop() {
  info "Stopping AI CFO stack…"

  # First, anything we have a pid file for. `go run` spawns a child binary,
  # so killing only the parent PID isn't enough — we kill the whole process
  # group (`-P` lists children; we kill both).
  for svc in backend frontend llama-server; do
    local pidfile="$LOG_DIR/${svc}.pid"
    if [[ -f "$pidfile" ]]; then
      local pid; pid="$(cat "$pidfile")"
      # Kill children first so they don't get reparented and survive.
      pgrep -P "$pid" 2>/dev/null | xargs -I{} kill {} 2>/dev/null || true
      if kill "$pid" 2>/dev/null; then
        ok "stopped $svc (pid $pid)"
      fi
      rm -f "$pidfile"
    fi
  done

  # Belt-and-suspenders cleanup: anything still bound to our ports. This
  # catches the Go-build child binary which lives under a random
  # /var/folders path that no static pkill pattern can match.
  for p in "$PORT" "$FRONTEND_PORT" "$LLAMA_SERVER_PORT"; do
    local pid; pid="$(port_pid "$p")"
    if [[ -n "$pid" ]]; then
      kill "$pid" 2>/dev/null && ok "stopped leftover on :$p (pid $pid)" || true
    fi
  done

  # Final cleanup of well-known process names.
  pkill -f 'go run ./cmd/server' 2>/dev/null || true
  pkill -f 'vite'                2>/dev/null || true
  pkill -f 'llama-server'        2>/dev/null || true

  ok "done."
}

cmd_status() {
  printf "%-15s %-6s %-8s %s\n" "SERVICE" "PORT" "PID" "STATE"
  for svc in backend:$PORT frontend:$FRONTEND_PORT llama-server:$LLAMA_SERVER_PORT; do
    local name="${svc%%:*}"; local p="${svc##*:}"
    local pid; pid="$(port_pid "$p")"
    if [[ -n "$pid" ]]; then
      printf "%-15s %-6s %-8s ${C_GRN}up${C_RST}\n"   "$name" "$p" "$pid"
    else
      printf "%-15s %-6s %-8s ${C_RED}down${C_RST}\n" "$name" "$p" "-"
    fi
  done
}

cmd_logs() {
  local svc="${1:-backend}"
  local f="$LOG_DIR/${svc}.log"
  if [[ ! -f "$f" ]]; then
    err "No log file for '$svc'. Try: backend | frontend | llama-server"
    exit 1
  fi
  tail -f "$f"
}

cmd_test() {
  if ! command -v python3 >/dev/null 2>&1; then
    err "python3 is required for the e2e test runner."
    exit 1
  fi
  if [[ -z "$(port_pid "$PORT")" ]]; then
    err "Backend is not running. Start it with: ./run.sh start"
    exit 1
  fi
  python3 scripts/e2e.py --skip-mutating --timeout 300
}

cmd_check() { check_deps; }

# ----------------------------------------------------------------------------
# License: thin wrapper that calls the on-device CLI with our environment
# pre-populated. This keeps the same UX whether the customer runs the
# bare binary or goes through run.sh.
# Usage:
#   ./run.sh license status
#   ./run.sh license deactivate
#   ./run.sh license activate <migration.dat>
#   ./run.sh license export-request
# ----------------------------------------------------------------------------
cmd_license() {
  local cli="$SCRIPT_DIR/backend/cfo-license"
  if [[ ! -x "$cli" ]]; then
    info "building cfo-license CLI…"
    (cd "$SCRIPT_DIR/backend" && go build -o cfo-license ./cmd/cfo-license)
  fi
  LICENSE_FILE="$LICENSE_FILE" LICENSE_STATE="$LICENSE_STATE" \
    DATA_DIR="$DATA_DIR" "$cli" "$@"
}

# ============================================================================
# Dispatch
# ============================================================================
main() {
  local cmd="${1:-start}"
  case "$cmd" in
    start)   cmd_start ;;
    stop)    cmd_stop ;;
    status)  cmd_status ;;
    logs)    shift; cmd_logs "$@" ;;
    test)    cmd_test ;;
    check)   cmd_check ;;
    license) shift; cmd_license "$@" ;;
    help|-h|--help)
      cat <<EOF
Usage: ./run.sh {start|stop|status|logs <svc>|check|test|license <subcommand>}

  start                 Start backend + frontend (+ optional llama-server)
  stop                  Stop everything started by this script
  status                Show port + pid for each service
  logs                  Tail logs:  ./run.sh logs backend|frontend|llama-server
  check                 Verify prerequisites (go, node, llama.cpp, model)
  test                  Run the python E2E test suite against the running stack
  license status        Show license binding, expiry, features, machine ID
  license deactivate    Generate migration.dat to move to another machine
  license activate F    Bind license to THIS machine using migration.dat
  license export-request  Generate request.dat for vendor renewal

See README.md for full env-var reference and feature list.
EOF
      ;;
    *)
      err "Unknown command: $cmd"
      err "Run './run.sh help' for usage."
      exit 1
      ;;
  esac
}

main "$@"
