@echo off
cd /d "%~dp0"
echo === Starting Territory Wars C# client ===
dotnet run --project . -- --config appsettings.example.json --name CSharpClient --strategy simple
