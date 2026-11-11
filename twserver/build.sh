#!/usr/bin/env bash
set -e
cd "$(dirname "$0")"
echo "=== Building Territory Wars server ==="
go build -o bin/server ./cmd/server
echo "Done: bin/server"
