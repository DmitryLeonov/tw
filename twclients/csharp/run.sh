#!/usr/bin/env bash
set -e
cd "$(dirname "$0")"
echo "=== Starting Territory Wars C# client ==="
dotnet bin/publish/TerritoryWars.Client.dll --config config.json --name CSharpClient --strategy simple
