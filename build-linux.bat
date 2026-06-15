@echo off
setlocal EnableExtensions

set "ROOT=%~dp0"
set "BACKEND_DIR=%ROOT%backend"
set "FRONTEND_DIR=%ROOT%frontend"
set "EMBED_DIR=%BACKEND_DIR%\web\dist"
set "OUTPUT_DIR=%ROOT%dist"
set "BINARY_NAME=miaoMuse"

if not "%~1"=="" set "BINARY_NAME=%~1"

echo ========================================
echo Build Linux binary with embedded frontend
echo ========================================
echo.

where go >nul 2>nul
if errorlevel 1 (
  echo [ERROR] Go was not found in PATH.
  exit /b 1
)

where npm >nul 2>nul
if errorlevel 1 (
  echo [ERROR] npm was not found in PATH.
  exit /b 1
)

if not exist "%BACKEND_DIR%\main.go" (
  echo [ERROR] Backend folder not found: %BACKEND_DIR%
  exit /b 1
)

if not exist "%FRONTEND_DIR%\package.json" (
  echo [ERROR] Frontend folder not found: %FRONTEND_DIR%
  exit /b 1
)

echo [1/5] Installing frontend dependencies if needed...
pushd "%FRONTEND_DIR%"
if not exist "node_modules" (
  call npm install
  if errorlevel 1 (
    popd
    echo [ERROR] npm install failed.
    exit /b 1
  )
)

echo [2/5] Building latest frontend...
call npm run build
if errorlevel 1 (
  popd
  echo [ERROR] frontend build failed.
  exit /b 1
)
popd

echo [3/5] Refreshing embedded frontend files...
if exist "%EMBED_DIR%" rmdir /s /q "%EMBED_DIR%"
mkdir "%EMBED_DIR%"
xcopy "%FRONTEND_DIR%\dist\*" "%EMBED_DIR%\" /e /i /y >nul
if errorlevel 1 (
  echo [ERROR] failed to copy frontend dist into backend embed directory.
  exit /b 1
)

if not exist "%EMBED_DIR%\index.html" (
  echo [ERROR] embedded frontend is missing index.html.
  exit /b 1
)

echo [4/5] Running backend tests...
pushd "%BACKEND_DIR%"
go test ./...
if errorlevel 1 (
  popd
  echo [ERROR] backend tests failed.
  exit /b 1
)

echo [5/5] Building Linux amd64 executable...
if not exist "%OUTPUT_DIR%" mkdir "%OUTPUT_DIR%"
set "CGO_ENABLED=0"
set "GOOS=linux"
set "GOARCH=amd64"
go build -trimpath -ldflags "-s -w" -o "%OUTPUT_DIR%\%BINARY_NAME%" .
if errorlevel 1 (
  popd
  echo [ERROR] Linux build failed.
  exit /b 1
)
popd

echo.
echo [OK] Built: %OUTPUT_DIR%\%BINARY_NAME%
echo [OK] Frontend embedded from: %FRONTEND_DIR%\dist
echo.
