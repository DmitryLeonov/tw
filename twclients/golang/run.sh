#!/usr/bin/env bash
set -e
cd "$(dirname "$0")"
echo "=== Starting Territory Wars Go client ==="
go run ./cmd/client --name GoClient --config config.json
