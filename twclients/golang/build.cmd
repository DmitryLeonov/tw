@echo off
cd /d "%~dp0"
echo === Building Territory Wars Go client ===
go build -o client.exe .\cmd\client
echo Done: client.exe
