@echo off
cd /d "%~dp0"
echo === Building Territory Wars C# client ===
dotnet build --configuration Release
echo Done
