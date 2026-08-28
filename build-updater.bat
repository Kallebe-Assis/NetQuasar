@echo off
setlocal
chcp 65001 >nul 2>&1

rem Recompila o NetQuasarUpdater.exe (raiz do projecto) a partir de quasar_backend/cmd/updater.
rem *.exe esta no .gitignore (nao versionado) — corra este script depois de um git pull para
rem obter o executavel actualizado, ou sempre que mudar o codigo do updater.

cd /d "%~dp0quasar_backend"
echo A compilar NetQuasarUpdater.exe...
go build -trimpath -ldflags="-s -w -H=windowsgui" -o "..\NetQuasarUpdater.exe" ./cmd/updater
if errorlevel 1 (
  echo ERRO: falha ao compilar. Confirme que o Go esta instalado e no PATH.
  pause
  exit /b 1
)
echo OK: ..\NetQuasarUpdater.exe pronto.
pause
