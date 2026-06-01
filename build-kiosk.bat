@echo off
setlocal
set KIOSK_DIR=%~dp0_build\Kiosk

if not exist "%KIOSK_DIR%" mkdir "%KIOSK_DIR%"

echo === Kiosk bauen (Wails) ===
cd /d "%~dp0Development\kiosk"
wails build -skipbindings
if %errorlevel% neq 0 (echo Kiosk-Build fehlgeschlagen & pause & exit /b 1)
copy /y "build\bin\liedanzeige-kiosk.exe" "%KIOSK_DIR%\liedanzeige-kiosk.exe"
if %errorlevel% neq 0 (echo Kiosk-Kopieren fehlgeschlagen & pause & exit /b 1)
rmdir /s /q "build\bin"

echo.
echo === Fertig ===
echo  Kiosk: %KIOSK_DIR%\liedanzeige-kiosk.exe
echo.
pause
