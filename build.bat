@echo off
setlocal
set BUILD_DIR=%~dp0_build
set SERVER_DIR=%BUILD_DIR%\Server
set KIOSK_DIR=%BUILD_DIR%\Kiosk

if not exist "%SERVER_DIR%" mkdir "%SERVER_DIR%"
if not exist "%KIOSK_DIR%"  mkdir "%KIOSK_DIR%"

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

echo === Kiosk bauen (Wails) ===
cd /d "%~dp0Development\kiosk"
wails build
if %errorlevel% neq 0 (echo Kiosk-Build fehlgeschlagen & pause & exit /b 1)
copy /y "build\bin\liedanzeige-kiosk.exe" "%KIOSK_DIR%\liedanzeige-kiosk.exe"
if %errorlevel% neq 0 (echo Kiosk-Kopieren fehlgeschlagen & pause & exit /b 1)

echo.
echo === Fertig ===
echo  Server:  %SERVER_DIR%
echo  Kiosk:   %KIOSK_DIR%
echo.
pause
