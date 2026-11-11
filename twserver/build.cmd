@echo off
cd /d "%~dp0"
echo === Building Territory Wars server ===
go build -o bin\server.exe .\cmd\server
echo Done: bin\server.exe
