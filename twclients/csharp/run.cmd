@echo off
cd /d "%~dp0"
echo === Building Territory Wars C# client ===
dotnet publish --configuration Release >nul 2>&1
if errorlevel 1 (
    echo Build failed.
    pause
    exit /b 1
)
echo === Starting Territory Wars C# client ===
dotnet bin\publish\TerritoryWars.Client.dll --config config.json --name CSharpClient --strategy simple
