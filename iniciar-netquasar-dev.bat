@echo off
setlocal

set "ROOT=%~dp0"
set "BACKEND_DIR=%ROOT%quasar_backend"
set "FRONTEND_DIR=%ROOT%quasar_frontend"

if not exist "%BACKEND_DIR%" (
  echo Pasta do backend nao encontrada: "%BACKEND_DIR%"
  pause
  exit /b 1
)

if not exist "%FRONTEND_DIR%" (
  echo Pasta do frontend nao encontrada: "%FRONTEND_DIR%"
  pause
  exit /b 1
)

echo Abrindo backend...
start "NetQuasar Backend" cmd /k "cd /d ""%BACKEND_DIR%"" && go run .\cmd\netquasar\"

timeout /t 1 >nul

echo Abrindo frontend...
start "NetQuasar Frontend" cmd /k "cd /d ""%FRONTEND_DIR%"" && npm run dev"

echo.
echo Backend e frontend iniciados em janelas separadas.
echo Backend:  http://localhost:8080
echo Frontend: http://localhost:5173
echo.
echo Pode fechar esta janela.

endlocal
