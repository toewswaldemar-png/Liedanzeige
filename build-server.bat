@echo off
setlocal
set SERVER_DIR=%~dp0_build\Server

if not exist "%SERVER_DIR%" mkdir "%SERVER_DIR%"

echo === Version ermitteln ===
for /f "delims=" %%v in ('git describe --tags --always 2^>nul') do set VERSION=%%v
if "%VERSION%"=="" set VERSION=dev
echo  Version: %VERSION%

echo === npm install pruefen ===
if not exist "%~dp0Development\frontend\node_modules" (
    echo node_modules fehlt - installiere...
    cd /d "%~dp0Development\frontend"
    call npm install
    if %errorlevel% neq 0 (echo npm install fehlgeschlagen & pause & exit /b 1)
)

echo === Frontend bauen ===
cd /d "%~dp0Development\frontend"
call npm run build
if %errorlevel% neq 0 (echo Frontend-Build fehlgeschlagen & pause & exit /b 1)

echo === Windows-Ressourcen generieren (Icon + Manifest) ===
cd /d "%~dp0Development\server"
goversioninfo -o resource.syso versioninfo.json
if %errorlevel% neq 0 (echo goversioninfo fehlgeschlagen - ist goversioninfo installiert? & pause & exit /b 1)

echo === Server bauen ===
go build -ldflags="-X main.version=%VERSION%" -o "%SERVER_DIR%\Liedanzeige.exe" .
if %errorlevel% neq 0 (echo Server-Build fehlgeschlagen & pause & exit /b 1)

echo.
echo === Fertig ===
echo  Server:  %SERVER_DIR%\Liedanzeige.exe
echo  Version: %VERSION%
echo.
pause
