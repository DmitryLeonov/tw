@echo off
REM Launch server + 5 Go + 5 Python + 5 C# clients + 10 bots.
REM
REM Usage:
REM   run.cmd              ^| server + 10 bots + all 15 clients (default)
REM   run.cmd --no-go      ^| skip Go clients
REM   run.cmd --no-python  ^| skip Python clients
REM   run.cmd --no-csharp  ^| skip C# clients
REM   run.cmd --no-bots    ^| server without bots (clients play)
REM   run.cmd --force-start| start game after clients register

setlocal enabledelayedexpansion

set "SCRIPT_DIR=%~dp0"
set "SERVER_DIR=%SCRIPT_DIR%twserver"
set "SERVER_BIN=%SERVER_DIR%\bin\server.exe"
set "CLIENTS_DIR=%SCRIPT_DIR%twclients"
set "LOGS_DIR=%SCRIPT_DIR%logs"
if not exist "%LOGS_DIR%" mkdir "%LOGS_DIR%"

set NUM_GO=5
set NUM_PYTHON=5
set NUM_CSHARP=5
set NUM_BOTS=10

set RUN_BOTS=true
set RUN_GO=true
set RUN_PYTHON=true
set RUN_CSHARP=true
set RUN_FORCE_START=false

REM Parse flags (skip if no arguments)
if "%*" neq "" (
    for %%a in (%*) do (
        if "%%a"=="--no-go" set RUN_GO=false
        if "%%a"=="--no-python" set RUN_PYTHON=false
        if "%%a"=="--no-csharp" set RUN_CSHARP=false
        if "%%a"=="--no-bots" set RUN_BOTS=false
        if "%%a"=="--force-start" set RUN_FORCE_START=true
    )
)

REM Build server
echo === Building server ===
cd /d "%SERVER_DIR%"
go build -o bin\server.exe .\cmd\server

REM Start server
echo === Starting server ===
if "%RUN_BOTS%"=="true" (
    start "TW Server" cmd /c "bin\server.exe -config config.json"
    echo Server started [bots enabled]
) else (
    echo { "port": 8080, "max_players": 30, "max_turns": 200, "width": 40, "height": 40, "turn_timeout": "1s", "bots": { "enabled": false } } > %LOGS_DIR%\server_nobots.json
    start "TW Server" cmd /c "bin\server.exe -config %LOGS_DIR%\server_nobots.json"
    echo Server started [no bots — clients play]
)

REM Wait for server
echo Waiting for server to be ready...
for /l %%i in (1,1,20) do (
    curl -s http://localhost:8080/health >nul 2>&1 && echo Server is ready && goto :start_clients
    timeout /t 1 /nobreak >nul
)
echo Server may not be ready

:start_clients
REM Start Go clients
if "%RUN_GO%"=="true" (
    echo === Starting %NUM_GO% Go clients ===
    cd /d "%CLIENTS_DIR%\golang"
    for /l %%i in (1,1,%NUM_GO%) do (
        start "GoClient%%i" cmd /c "bin\client.exe --name GoClient%%i --config config.json"
        echo   GoClient%%i started
    )
)

REM Start Python clients
if "%RUN_PYTHON%"=="true" (
    echo === Starting %NUM_PYTHON% Python clients ===
    cd /d "%CLIENTS_DIR%\python"
    for /l %%i in (1,1,%NUM_PYTHON%) do (
        start "PythonClient%%i" cmd /c "uv run python main.py --name PythonClient%%i --config config.json"
        echo   PythonClient%%i started
    )
)

REM Start C# clients (delayed because dotnet takes time to load)
if "%RUN_CSHARP%"=="true" (
    echo === Starting %NUM_CSHARP% C# clients [delayed 15s] ===
    timeout /t 15 /nobreak >nul
    cd /d "%CLIENTS_DIR%\csharp"
    for /l %%i in (1,1,%NUM_CSHARP%) do (
        start "CSharpClient%%i" cmd /c "dotnet exec bin\Release\net10.0\publish\TerritoryWars.Client.dll --config config.json --name CSharpClient%%i --strategy simple"
        echo   CSharpClient%%i started
    )
)

REM If --force-start, wait for clients to register then start the game
if "%RUN_FORCE_START%"=="true" (
    echo === Waiting for clients to register [15s] ===
    timeout /t 15 /nobreak >nul
    echo === Starting game manually ===
    curl -s -X POST http://localhost:8080/api/v1/start
    echo.
)

REM If no bots, wait for clients to register then start the game
if "%RUN_BOTS%"=="false" (
    echo === Waiting for clients to register [15s] ===
    timeout /t 15 /nobreak >nul
    echo === Starting game manually ===
    curl -s -X POST http://localhost:8080/api/v1/start
    echo.
)

echo.
echo === All processes started ===
echo Server:        started
echo Go clients:    %NUM_GO% instances
echo Python clients: %NUM_PYTHON% instances
echo C# clients:    %NUM_CSHARP% instances
if "%RUN_BOTS%"=="true" echo Bots:          %NUM_BOTS% enabled
echo.
echo Viewer: http://localhost:8080/viewer
echo Logs:   %LOGS_DIR%\{server,go_client_*,python_client_*,csharp_client_*}.log
echo Press Ctrl+C to stop

REM Keep script alive
:wait_loop
timeout /t 5 /nobreak >nul
goto :wait_loop
