@echo off
cd /d "%~dp0"
echo === Starting Territory Wars C# client ===
dotnet run --project . -- --config config.json --name CSharpClient --strategy simple
