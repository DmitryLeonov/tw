#!/usr/bin/env bash
set -e
cd "$(dirname "$0")"
echo "=== Starting Territory Wars C# client ==="
dotnet run --project . -- --config config.json --name CSharpClient --strategy simple
