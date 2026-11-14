@echo off
cd /d "%~dp0"
echo === Starting Territory Wars C# client ===
dotnet exec bin\Release\net10.0\publish\TerritoryWars.Client.dll --config config.json --name CSharpClient --strategy simple
