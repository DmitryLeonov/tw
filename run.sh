#!/usr/bin/env bash
# Launch server + 5 Go + 5 Python + 5 C# clients. The server fills the
# remaining player slots with built-in bots (max_players - registered clients).
#
# Usage:
#   ./run.sh              # server + all 15 clients (+ bots fill the rest)
#   ./run.sh --no-go      # skip Go clients
#   ./run.sh --no-python  # skip Python clients
#   ./run.sh --no-csharp  # skip C# clients
#   ./run.sh --no-bots    # server without bots (clients play)

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SERVER_DIR="$SCRIPT_DIR/twserver"
SERVER_BIN="$SERVER_DIR/bin/server"
CLIENTS_DIR="$SCRIPT_DIR/twclients"

NUM_GO=5
NUM_PYTHON=5
NUM_CSHARP=5

# Find dotnet executable (Linux / macOS compatible)
DOTNET_BIN=""
if command -v dotnet > /dev/null 2>&1; then
  DOTNET_BIN="$(command -v dotnet)"
elif [ -f /opt/dotnet/dotnet ]; then
  DOTNET_BIN="/opt/dotnet/dotnet"
elif [ -f /usr/local/share/dotnet/dotnet ]; then
  DOTNET_BIN="/usr/local/share/dotnet/dotnet"
elif [ -f "$HOME/.dotnet/dotnet" ]; then
  DOTNET_BIN="$HOME/.dotnet/dotnet"
fi
if [ -n "$DOTNET_BIN" ]; then
  export PATH="$(dirname "$DOTNET_BIN"):$PATH"
fi

# Add uv to PATH if installed
UV_BIN=""
if command -v uv > /dev/null 2>&1; then
  UV_BIN="uv"
elif [ -d "$HOME/.local/bin" ] && command -v "$HOME/.local/bin/uv" > /dev/null 2>&1; then
  export PATH="$HOME/.local/bin:$PATH"
  UV_BIN="uv"
fi

# Defaults
RUN_BOTS=true
RUN_GO=true
RUN_PYTHON=true
RUN_CSHARP=true
RUN_FORCE_START=false

# Ensure logs directory exists
mkdir -p "$SCRIPT_DIR/logs"

# Arrays for PIDs
declare -a SERVER_PIDS=()
declare -a GO_PIDS=()
declare -a PYTHON_PIDS=()
declare -a CSHARP_PIDS=()
SERVER_IDX=0
GO_IDX=0
PYTHON_IDX=0
CSHARP_IDX=0

# Parse flags
for arg in "$@"; do
  case "$arg" in
    --no-go)       RUN_GO=false ;;
    --no-python)   RUN_PYTHON=false ;;
    --no-csharp)   RUN_CSHARP=false ;;
    --no-bots)     RUN_BOTS=false ;;
    --force-start) RUN_FORCE_START=true ;;
    *)             echo "Unknown: $arg"; exit 1 ;;
  esac
done

# Refuse to run if a server from a previous session is still listening:
# the server ignores SIGINT/SIGTERM by design, so Ctrl+C elsewhere won't stop it,
# and a second instance would silently fail to bind while run.sh drives the old one.
if curl -s -m 2 http://localhost:8080/health > /dev/null 2>&1; then
  echo "ERROR: a server is already listening on :8080 (likely from a previous run)."
  echo "Stop it first:   kill -9 \$(pgrep -f 'twserver/bin/server')"
  exit 1
fi

# Build Go binaries so we never launch a stale server/client.
if ! command -v go > /dev/null 2>&1; then
  echo "ERROR: 'go' not found in PATH."
  exit 1
fi
echo "=== Building server ==="
cd "$SERVER_DIR" && go build -o bin/server ./cmd/server || { echo "ERROR: server build failed"; exit 1; }
if [ "$RUN_GO" = true ]; then
  echo "=== Building Go client ==="
  cd "$CLIENTS_DIR/golang" && go build -o bin/client ./cmd/client || { echo "ERROR: Go client build failed"; exit 1; }
fi

# Build the C# client if dotnet is available (run.sh launches the published DLL).
if [ "$RUN_CSHARP" = true ] && [ -n "$DOTNET_BIN" ]; then
  echo "=== Building C# client ==="
  cd "$CLIENTS_DIR/csharp"
  "$DOTNET_BIN" publish -c Release > /dev/null 2>&1 || echo "WARNING: C# build failed, C# clients will be skipped"
fi

# Start server
echo "=== Starting server ==="
cd "$SERVER_DIR"
if [ "$RUN_BOTS" = true ]; then
  nohup "$SERVER_BIN" -config config.json &
  SERVER_PIDS[$SERVER_IDX]=$!
  disown $!
  echo "Server PID: ${SERVER_PIDS[$SERVER_IDX]} (bots enabled)"
  SERVER_IDX=$((SERVER_IDX + 1))
else
  # No bots: start server without bots so external clients can join first
  cat > /tmp/server_nobots.json <<EOF
{
  "port": 8080,
  "max_players": 30,
  "max_turns": 200,
  "width": 40,
  "height": 40,
  "turn_delay": "1s",
  "bots": { "enabled": false }
}
EOF
  nohup "$SERVER_BIN" -config /tmp/server_nobots.json &
  SERVER_PIDS[$SERVER_IDX]=$!
  disown $!
  echo "Server PID: ${SERVER_PIDS[$SERVER_IDX]} (no bots — clients play)"
  SERVER_IDX=$((SERVER_IDX + 1))
fi

# Wait for server to be ready
echo "Waiting for server to be ready..."
READY=false
for i in $(seq 1 20); do
  if curl -s -m 2 http://localhost:8080/health > /dev/null 2>&1; then
    echo "Server is ready"
    READY=true
    break
  fi
  sleep 0.5
done
if [ "$READY" != true ]; then
  echo "WARNING: server did not answer /health within 10s — it may have failed to start or bind the port."
fi

# Start external clients
if [ "$RUN_GO" = true ]; then
  echo "=== Starting $NUM_GO Go clients ==="
  cd "$CLIENTS_DIR/golang"
  for i in $(seq 1 $NUM_GO); do
    nohup ./bin/client --name "GoClient$i" --config config.json &
    GO_PIDS[$GO_IDX]=$!
    disown $!
    echo "  GoClient$i PID: ${GO_PIDS[$GO_IDX]}"
    GO_IDX=$((GO_IDX + 1))
  done
fi

# Start Python clients (5 copies)
if [ "$RUN_PYTHON" = true ]; then
  echo "=== Starting $NUM_PYTHON Python clients ==="
  cd "$CLIENTS_DIR/python"
  if [ -n "$UV_BIN" ]; then
    for i in $(seq 1 $NUM_PYTHON); do
      PATH="$HOME/.local/bin:$PATH" nohup $UV_BIN run python main.py --name "PythonClient$i" --config config.json &
      PYTHON_PIDS[$PYTHON_IDX]=$!
      disown $!
      echo "  PythonClient$i PID: ${PYTHON_PIDS[$PYTHON_IDX]}"
      PYTHON_IDX=$((PYTHON_IDX + 1))
    done
  else
    echo "=== Skipping Python clients (uv not found) ==="
  fi
fi

# Start C# clients (5 copies) — delayed because dotnet takes time to load
if [ "$RUN_CSHARP" = true ]; then
  if [ -f "$CLIENTS_DIR/csharp/bin/Release/net10.0/publish/TerritoryWars.Client.dll" ]; then
    echo "=== Starting $NUM_CSHARP C# clients (delayed 5s) ==="
    sleep 15
    cd "$CLIENTS_DIR/csharp"
    for i in $(seq 1 $NUM_CSHARP); do
      nohup "$DOTNET_BIN" exec bin/Release/net10.0/publish/TerritoryWars.Client.dll \
        --config config.json \
        --name "CSharpClient$i" \
        --strategy simple \
        &
      CSHARP_PIDS[$CSHARP_IDX]=$!
      disown $!
      echo "  CSharpClient$i PID: ${CSHARP_PIDS[$CSHARP_IDX]}"
      CSHARP_IDX=$((CSHARP_IDX + 1))
    done
  else
    echo "=== Skipping C# clients (binary not found) ==="
  fi
fi

# If --force-start, wait for clients to register then start the game
if [ "$RUN_FORCE_START" = true ]; then
  echo "=== Waiting for clients to register (15s) ==="
  sleep 15
  echo "=== Starting game manually ==="
  curl -s -m 5 -X POST http://localhost:8080/api/v1/start && echo "" || true
fi

echo ""
echo "=== All processes started ==="
echo "Server:        PID ${SERVER_PIDS[0]}"
[ ${#GO_PIDS[@]} -gt 0 ]      && echo "Go clients:    ${#GO_PIDS[@]} instances (PIDs: ${GO_PIDS[*]})"
[ ${#PYTHON_PIDS[@]} -gt 0 ]  && echo "Python clients: ${#PYTHON_PIDS[@]} instances (PIDs: ${PYTHON_PIDS[*]})"
[ ${#CSHARP_PIDS[@]} -gt 0 ]  && echo "C# clients:    ${#CSHARP_PIDS[@]} instances (PIDs: ${CSHARP_PIDS[*]})"
if [ "$RUN_BOTS" = true ]; then
  echo "Bots:          server fills the remaining slots (max_players - registered clients)"
else
  echo "Bots:          disabled (--no-bots)"
fi
echo ""
echo "Viewer: http://localhost:8080/viewer"
echo "Logs:   twserver/logs/server.log, twclients/{golang,python,csharp}/logs/<client_name>.log"
echo "Press Ctrl+C to stop all"

# Interactive console — sends commands to the server via REST API.
# Mirrors the built-in console commands: start, stop, pause, resume.
SERVER_URL="http://localhost:8080"

while true; do
  echo ""
  echo "=== Server Console ==="
  echo "  1) start  — start the game"
  echo "  2) stop   — stop the game"
  echo "  3) pause  — pause the game"
  echo "  4) resume — resume the game"
  echo "  5) state  — show game state"
  echo "  6) leaderboard — show leaderboard"
  echo "  q) quit   — stop all processes"
  echo ""
  read -r -p "Command [1-6/q]: " cmd

  case "$cmd" in
    1)
      echo -n "Starting game... "
      curl -s -m 10 -X POST "$SERVER_URL/api/v1/start" && echo " OK" || echo " FAIL"
      ;;
    2)
      echo -n "Stopping game... "
      curl -s -m 10 -X POST "$SERVER_URL/api/v1/stop" && echo " OK" || echo " FAIL"
      ;;
    3)
      echo -n "Pausing game... "
      curl -s -m 10 -X POST "$SERVER_URL/api/v1/pause" && echo " OK" || echo " FAIL"
      ;;
    4)
      echo -n "Resuming game... "
      curl -s -m 10 -X POST "$SERVER_URL/api/v1/resume" && echo " OK" || echo " FAIL"
      ;;
    5)
      curl -s -m 10 "$SERVER_URL/api/v1/state" 2>/dev/null | python3 -m json.tool 2>/dev/null || curl -s -m 10 "$SERVER_URL/api/v1/state"
      ;;
    6)
      curl -s -m 10 "$SERVER_URL/api/v1/leaderboard" 2>/dev/null | python3 -m json.tool 2>/dev/null || curl -s -m 10 "$SERVER_URL/api/v1/leaderboard"
      ;;
    q|quit|Q|QUIT)
      echo "Stopping all processes..."
      # Stop server
      curl -s -m 5 -X POST "$SERVER_URL/api/v1/stop" > /dev/null 2>&1
      # Kill all client PIDs
      for pid in "${GO_PIDS[@]}" "${PYTHON_PIDS[@]}" "${CSHARP_PIDS[@]}"; do
        kill "$pid" 2>/dev/null
      done
      for pid in "${SERVER_PIDS[@]}"; do
        kill "$pid" 2>/dev/null
      done
      # The server ignores SIGTERM by design; make sure it is really gone.
      sleep 1
      for pid in "${SERVER_PIDS[@]}"; do
        kill -0 "$pid" 2>/dev/null && kill -9 "$pid" 2>/dev/null
      done
      echo "All processes stopped."
      exit 0
      ;;
    *)
      echo "Unknown command: $cmd"
      ;;
  esac
done
