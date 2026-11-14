@echo off
cd /d "%~dp0"
echo === Starting Territory Wars Python client ===
set NAME=PythonClient
set CONFIG=config.json
set LOG_FILE=

:loop
if "%~1"=="" goto endloop
if "%~1"=="--name" set NAME=%~2
if "%~1"=="--config" set CONFIG=%~2
if "%~1"=="--log-file" set LOG_FILE=%~2
shift
shift
if "%~1" neq "" goto loop

:endloop
uv run python main.py --name %NAME% --config %CONFIG% --log-file %LOG_FILE%
