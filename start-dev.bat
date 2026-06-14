@echo off
setlocal

set "ROOT=%~dp0"
set "BACKEND_DIR=%ROOT%backend"
set "FRONTEND_DIR=%ROOT%frontend"
set "BACKEND_PORT=18080"
set "FRONTEND_PORT=5173"

title 剧本工坊 Dev

echo ========================================
echo 剧本工坊 dev startup
echo ========================================
echo.

where go >nul 2>nul
if errorlevel 1 (
  echo [ERROR] Go was not found in PATH.
  pause
  exit /b 1
)

where npm >nul 2>nul
if errorlevel 1 (
  echo [ERROR] npm was not found in PATH.
  pause
  exit /b 1
)

if not exist "%BACKEND_DIR%\main.go" (
  echo [ERROR] Backend folder not found: %BACKEND_DIR%
  pause
  exit /b 1
)

if not exist "%FRONTEND_DIR%\package.json" (
  echo [ERROR] Frontend folder not found: %FRONTEND_DIR%
  pause
  exit /b 1
)

if not exist "%FRONTEND_DIR%\node_modules" (
  echo [FRONTEND] Installing dependencies...
  pushd "%FRONTEND_DIR%"
  call npm install
  if errorlevel 1 (
    popd
    echo [ERROR] npm install failed.
    pause
    exit /b 1
  )
  popd
)

echo [BACKEND] Starting http://localhost:%BACKEND_PORT%
pushd "%BACKEND_DIR%"
start /b "" cmd /c "set PORT=%BACKEND_PORT%&& go run ."
popd

echo [FRONTEND] Starting http://localhost:%FRONTEND_PORT%
pushd "%FRONTEND_DIR%"
start /b "" cmd /c "set VITE_API_BASE=http://localhost:%BACKEND_PORT%&& npm run dev -- --port %FRONTEND_PORT%"
popd

echo.
echo Frontend: http://localhost:%FRONTEND_PORT%
echo Backend:  http://localhost:%BACKEND_PORT%
echo.
echo Keep this window open while developing.
echo Press Ctrl+C, then Y, to stop this dev window.
echo.

:keep_alive
timeout /t 3600 >nul
goto keep_alive
