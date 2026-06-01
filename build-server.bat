@echo off
setlocal
set SERVER_DIR=%~dp0_build\Server

if not exist "%SERVER_DIR%" mkdir "%SERVER_DIR%"

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

echo === Server bauen ===
cd /d "%~dp0Development\server"
go build -o "%SERVER_DIR%\liedanzeige-server.exe" .
if %errorlevel% neq 0 (echo Server-Build fehlgeschlagen & pause & exit /b 1)

echo.
echo === Fertig ===
echo  Server: %SERVER_DIR%\liedanzeige-server.exe
echo.
pause
