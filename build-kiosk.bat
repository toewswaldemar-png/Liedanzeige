@echo off
setlocal
set KIOSK_DIR=%~dp0_build\Kiosk

if not exist "%KIOSK_DIR%" mkdir "%KIOSK_DIR%"

echo === Kiosk bauen (Go) ===
cd /d "%~dp0Development\kiosk"

echo [1/4] Abhaengigkeiten laden...
go get github.com/jchv/go-webview2@latest
if %errorlevel% neq 0 (echo go get fehlgeschlagen & pause & exit /b 1)

echo [2/4] Module aufraumen...
go mod tidy
if %errorlevel% neq 0 (echo go mod tidy fehlgeschlagen & pause & exit /b 1)

echo [3/4] Windows-Ressourcen generieren (Icon + Manifest)...
goversioninfo -o resource.syso versioninfo.json
if %errorlevel% neq 0 (echo goversioninfo fehlgeschlagen - ist goversioninfo installiert? & pause & exit /b 1)

echo [4/4] Kompilieren...
go build -ldflags="-H windowsgui" -o "%KIOSK_DIR%\Kiosk.exe" .
if %errorlevel% neq 0 (echo Kiosk-Build fehlgeschlagen & pause & exit /b 1)

echo.
echo === Fertig ===
echo  Kiosk: %KIOSK_DIR%\Kiosk.exe
echo.
pause
