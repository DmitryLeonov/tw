#!/usr/bin/env bash
set -e
cd "$(dirname "$0")"
echo "=== Starting Territory Wars C# client ==="
dotnet run --project . -- --config appsettings.example.json --name CSharpClient --strategy simple
