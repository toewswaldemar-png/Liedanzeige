@echo off
setlocal
set KIOSK_DIR=%~dp0_build\kiosk

if not exist "%KIOSK_DIR%" mkdir "%KIOSK_DIR%"

echo === Watchdog bauen ===
cd /d "%~dp0watchdog"
go build -o "%KIOSK_DIR%\liedanzeige-watchdog.exe" .
if %errorlevel% neq 0 (echo Watchdog-Build fehlgeschlagen & pause & exit /b 1)

echo === Kiosk bauen (Wails) ===
cd /d "%~dp0kiosk"
wails build
if %errorlevel% neq 0 (echo Kiosk-Build fehlgeschlagen & pause & exit /b 1)
copy /y "build\bin\liedanzeige-kiosk.exe" "%KIOSK_DIR%\liedanzeige-kiosk.exe"
if %errorlevel% neq 0 (echo Kiosk-Kopieren fehlgeschlagen & pause & exit /b 1)

echo.
echo === Fertig ===
echo  Kiosk: %KIOSK_DIR%\liedanzeige-kiosk.exe
echo.
pause
