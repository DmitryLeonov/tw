@echo off
cd /d "%~dp0"
echo === Building Territory Wars C# client ===
dotnet publish --configuration Release
echo Done: bin\Release\net10.0\publish\TerritoryWars.Client.dll
