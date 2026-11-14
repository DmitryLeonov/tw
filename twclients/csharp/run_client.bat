@echo off
cd /d "%~dp0"
dotnet exec bin\Release\net10.0\publish\TerritoryWars.Client.dll --name %*
