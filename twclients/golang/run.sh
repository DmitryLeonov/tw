#!/usr/bin/env bash
set -e
cd "$(dirname "$0")"
echo "=== Starting Territory Wars Go client ==="
./bin/client --name GoClient --config config.json
