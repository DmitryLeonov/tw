#!/usr/bin/env bash
set -e
cd "$(dirname "$0")"
echo "=== Building Territory Wars C# client ==="
dotnet build --configuration Release
echo "Done"
