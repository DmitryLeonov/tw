@echo off
cd /d "%~dp0"
echo === Starting Territory Wars server ===
bin\server.exe -config config.json
