@echo off
cd /d "%~dp0"
echo === Building Territory Wars Go client ===
mkdir bin 2>nul
go build -o bin\client.exe .\cmd\client
echo Done: bin\client.exe
