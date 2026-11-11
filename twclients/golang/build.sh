#!/usr/bin/env bash
set -e
cd "$(dirname "$0")"
echo "=== Building Territory Wars Go client ==="
go build -o client ./cmd/client
echo "Done: client"
