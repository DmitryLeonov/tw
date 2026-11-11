#!/usr/bin/env bash
# Launch server + external clients (Go, Python, C#).
#
# Usage:
#   ./run.sh              # server + all 3 clients (no bots — clients play)
#   ./run.sh --bots       # server with bots (bots play, no external clients)
#   ./run.sh --mixed      # server with bots + external clients
#   ./run.sh --no-go      # skip Go client (only with --bots or --mixed)
#   ./run.sh --no-python  # skip Python client
#   ./run.sh --no-csharp  # skip C# client

set +e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SERVER_DIR="$SCRIPT_DIR/../twserver"
SERVER_BIN="$SERVER_DIR/bin/server"

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
MODE="normal"  # normal | bots | mixed
BOTS=false
RUN_GO=true
RUN_PYTHON=true
RUN_CSHARP=true
GO_PID=""
PYTHON_PID=""
CSHARP_PID=""

# Parse flags
for arg in "$@"; do
  case "$arg" in
    --no-go)      RUN_GO=false ;;
    --no-python)  RUN_PYTHON=false ;;
    --no-csharp)  RUN_CSHARP=false ;;
    --bots)       MODE="bots"; BOTS=true ;;
    --mixed)      MODE="mixed"; BOTS=true ;;
    *)            echo "Unknown: $arg"; exit 1 ;;
  esac
done

# Build server
echo "=== Building server ==="
cd "$SERVER_DIR"
go build -o bin/server ./cmd/server 2>&1

# Start server
echo "=== Starting server ==="
if [ "$MODE" = "bots" ]; then
  nohup "$SERVER_BIN" -config config.json > /tmp/server.log 2>&1 &
  SERVER_PID=$!
  echo "Server PID: $SERVER_PID (bots mode)"
elif [ "$MODE" = "mixed" ]; then
  nohup "$SERVER_BIN" -config config.json > /tmp/server.log 2>&1 &
  SERVER_PID=$!
  echo "Server PID: $SERVER_PID (mixed mode)"
else
  # Normal mode: start server without bots config so external clients can join first
  cat > /tmp/server_nobots.json <<EOF
{
  "port": 8080,
  "max_players": 20,
  "max_turns": 200,
  "width": 40,
  "height": 40,
  "turn_timeout": "15s",
  "bots": { "enabled": false }
}
EOF
  nohup "$SERVER_BIN" -config /tmp/server_nobots.json > /tmp/server.log 2>&1 &
  SERVER_PID=$!
  echo "Server PID: $SERVER_PID (normal mode — clients play)"
fi

# Wait for server to be ready
for i in $(seq 1 10); do
  if curl -s http://localhost:8080/health > /dev/null 2>&1; then
    echo "Server is ready"
    break
  fi
  sleep 0.5
done

# Start external clients (only in normal or mixed mode)
if [ "$MODE" != "bots" ]; then
  # Start Go client
  if [ "$RUN_GO" = true ]; then
    echo "=== Starting Go client ==="
    cd "$SCRIPT_DIR/golang"
    nohup go run ./cmd/client --name GoClient --config config.json > /tmp/go_client.log 2>&1 &
    GO_PID=$!
    echo "Go client PID: $GO_PID"
  fi

  # Start Python client
  if [ "$RUN_PYTHON" = true ]; then
    echo "=== Starting Python client ==="
    cd "$SCRIPT_DIR/python"
    if [ -n "$UV_BIN" ]; then
      nohup $UV_BIN run python main.py --name PythonClient --config config.json > /tmp/python_client.log 2>&1 &
    else
      echo "=== Skipping Python client (uv not found) ==="
    fi
    PYTHON_PID=$!
    echo "Python client PID: $PYTHON_PID"
  fi

  # Start C# client
  if [ "$RUN_CSHARP" = true ]; then
    if command -v dotnet > /dev/null 2>&1; then
      echo "=== Starting C# client ==="
      cd "$SCRIPT_DIR/csharp"
      cp config.json config.local.json 2>/dev/null || true
      dotnet run --project . -- \
        --config config.json \
        --name CSharpClient \
        --strategy simple \
        > /tmp/csharp_client.log 2>&1 &
      CSHARP_PID=$!
      echo "C# client PID: $CSHARP_PID"
    else
      echo "=== Skipping C# client (dotnet not found) ==="
    fi
  fi

  # In mixed mode, wait for clients to register then start the game
  if [ "$MODE" = "mixed" ]; then
    echo "=== Waiting for clients to register (10s) ==="
    sleep 10
    echo "=== Starting game manually ==="
    curl -s -X POST http://localhost:8080/api/v1/start && echo "" || true
  fi
fi

echo ""
echo "=== All processes started ==="
echo "Server:      PID $SERVER_PID"
if [ "$MODE" != "bots" ]; then
  [ -n "${GO_PID:-}" ]      && echo "Go client:   PID $GO_PID"
  [ -n "${PYTHON_PID:-}" ]  && echo "Python:      PID $PYTHON_PID"
  [ -n "${CSHARP_PID:-}" ]  && echo "C#:          PID $CSHARP_PID"
fi
if [ "$BOTS" = true ]; then
  echo "Bots:        enabled"
fi
echo ""
echo "Viewer: http://localhost:8080/viewer"
echo "Press Ctrl+C to stop all"

# Trap SIGINT to kill all children
cleanup() {
  echo ""
  echo "Shutting down..."
  kill $SERVER_PID 2>/dev/null
  if [ "$MODE" != "bots" ]; then
    [ -n "${GO_PID:-}" ]      && kill $GO_PID 2>/dev/null
    [ -n "${PYTHON_PID:-}" ]  && kill $PYTHON_PID 2>/dev/null
    [ -n "${CSHARP_PID:-}" ]  && kill $CSHARP_PID 2>/dev/null
  fi
  echo "Done"
}
trap cleanup INT TERM

# Keep script alive without blocking on child exit
while true; do
  sleep 1
done
