@echo off
cd /d "%~dp0"
echo === Starting Territory Wars Python client ===
uv run python main.py --name %*
