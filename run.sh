#!/usr/bin/env bash
# Launch server + 5 Go + 5 Python + 5 C# clients + 10 bots.
#
# Usage:
#   ./run.sh              # server + 10 bots + all 15 clients (default)
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
NUM_BOTS=10

# Add dotnet to PATH if installed
if [ -d /opt/dotnet ]; then
  export PATH="/opt/dotnet:$PATH"
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

# Log directory (relative to project root)
LOGS_DIR="$SCRIPT_DIR/logs"
mkdir -p "$LOGS_DIR"

# Arrays for PIDs
declare -a SERVER_PIDS=()
declare -a GO_PIDS=()
declare -a PYTHON_PIDS=()
declare -a CSHARP_PIDS=()

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

# Start server
echo "=== Starting server ==="
cd "$SERVER_DIR"
if [ "$RUN_BOTS" = true ]; then
  nohup "$SERVER_BIN" -config config.json &
  SERVER_PIDS+=($!)
  disown $!
  echo "Server PID: ${SERVER_PIDS[-1]} (bots enabled)"
else
  # No bots: start server without bots so external clients can join first
  cat > /tmp/server_nobots.json <<EOF
{
  "port": 8080,
  "max_players": 30,
  "max_turns": 200,
  "width": 40,
  "height": 40,
  "turn_timeout": "1s",
  "bots": { "enabled": false }
}
EOF
  nohup "$SERVER_BIN" -config /tmp/server_nobots.json &
  SERVER_PIDS+=($!)
  echo "Server PID: ${SERVER_PIDS[-1]} (no bots — clients play)"
fi

# Wait for server to be ready
echo "Waiting for server to be ready..."
for i in $(seq 1 20); do
  if curl -s http://localhost:8080/health > /dev/null 2>&1; then
    echo "Server is ready"
    break
  fi
  sleep 0.5
done

# Start external clients
if [ "$RUN_GO" = true ]; then
  echo "=== Starting $NUM_GO Go clients ==="
  cd "$CLIENTS_DIR/golang"
  for i in $(seq 1 $NUM_GO); do
    nohup ./bin/client --name "GoClient$i" --config config.json &
    GO_PIDS+=($!)
    disown $!
    echo "  GoClient$i PID: ${GO_PIDS[-1]}"
  done
fi

# Start Python clients (5 copies)
if [ "$RUN_PYTHON" = true ]; then
  echo "=== Starting $NUM_PYTHON Python clients ==="
  cd "$CLIENTS_DIR/python"
  if [ -n "$UV_BIN" ]; then
    for i in $(seq 1 $NUM_PYTHON); do
      PATH="$HOME/.local/bin:$PATH" nohup $UV_BIN run python main.py --name "PythonClient$i" --config config.json &
      PYTHON_PIDS+=($!)
      disown $!
      echo "  PythonClient$i PID: ${PYTHON_PIDS[-1]}"
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
      nohup /opt/dotnet/dotnet exec bin/Release/net10.0/publish/TerritoryWars.Client.dll \
        --config config.json \
        --name "CSharpClient$i" \
        --strategy simple \
        &
      CSHARP_PIDS+=($!)
      disown $!
      echo "  CSharpClient$i PID: ${CSHARP_PIDS[-1]}"
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
  curl -s -X POST http://localhost:8080/api/v1/start && echo "" || true
fi

echo ""
echo "=== All processes started ==="
echo "Server:        PID ${SERVER_PIDS[0]}"
[ ${#GO_PIDS[@]} -gt 0 ]      && echo "Go clients:    ${#GO_PIDS[@]} instances (PIDs: ${GO_PIDS[*]})"
[ ${#PYTHON_PIDS[@]} -gt 0 ]  && echo "Python clients: ${#PYTHON_PIDS[@]} instances (PIDs: ${PYTHON_PIDS[*]})"
[ ${#CSHARP_PIDS[@]} -gt 0 ]  && echo "C# clients:    ${#CSHARP_PIDS[@]} instances (PIDs: ${CSHARP_PIDS[*]})"
if [ "$RUN_BOTS" = true ]; then
  echo "Bots:          $NUM_BOTS enabled"
fi
echo ""
echo "Viewer: http://localhost:8080/viewer"
echo "Logs:   logs/{server,go_client_*,python_client_*,csharp_client_*}.log"
echo "Press Ctrl+C to stop all"

# Keep script alive without blocking on child exit
while true; do
  sleep 1
done
