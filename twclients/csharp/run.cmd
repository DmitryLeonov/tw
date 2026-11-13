@echo off
cd /d "%~dp0"
echo === Starting Territory Wars C# client ===
dotnet bin\publish\TerritoryWars.Client.dll --config config.json --name CSharpClient --strategy simple
