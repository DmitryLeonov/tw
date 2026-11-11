@echo off
cd /d "%~dp0"
echo === Starting Territory Wars Go client ===
go run .\cmd\client --name GoClient --config config.json
