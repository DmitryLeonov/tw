#!/usr/bin/env bash
set -e
cd "$(dirname "$0")"
echo "=== Building Territory Wars Go client ==="
mkdir -p bin
go build -o bin/client ./cmd/client
echo "Done: bin/client"
