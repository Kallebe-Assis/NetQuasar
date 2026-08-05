@echo off
setlocal
cd /d "%~dp0"

echo === NetQuasar update ===
echo.

echo [1/2] git pull...
git pull
if errorlevel 1 (
  echo ERRO: git pull falhou.
  exit /b 1
)

echo.
echo [2/2] docker compose up --build -d...
docker compose up --build -d
if errorlevel 1 (
  echo ERRO: docker compose falhou.
  exit /b 1
)

echo.
echo Concluido.
endlocal
