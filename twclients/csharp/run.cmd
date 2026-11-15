@echo off
cd /d "%~dp0"
echo === Starting Territory Wars C# client ===
set NAME=CSharpClient
set CONFIG=config.json
set STRATEGY=simple
set LOG_FILE=

:loop
if "%~1"=="" goto endloop
if "%~1"=="--name" set NAME=%~2
if "%~1"=="--config" set CONFIG=%~2
if "%~1"=="--strategy" set STRATEGY=%~2
if "%~1"=="--log-file" set LOG_FILE=%~2
shift
shift
if "%~1" neq "" goto loop

:endloop
dotnet exec bin\Release\net10.0\publish\TerritoryWars.Client.dll --name %NAME% --config %CONFIG% --strategy %STRATEGY% --log-file %LOG_FILE%
