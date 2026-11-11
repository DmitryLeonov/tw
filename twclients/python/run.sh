#!/usr/bin/env bash
set -e
cd "$(dirname "$0")"
echo "=== Starting Territory Wars Python client ==="
uv run python main.py --name PythonClient --config config.json
