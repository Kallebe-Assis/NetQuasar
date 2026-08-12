@echo off
setlocal EnableExtensions EnableDelayedExpansion
chcp 65001 >nul 2>&1

rem NetQuasar — actualiza o código (git pull) e reconstroi/sobe o stack Docker.
rem Uso:  update.bat
rem        update.bat --skip-pull     (só Docker)
rem        update.bat --no-pause      (não espera tecla no fim)
rem        update.bat --no-caddy      (sobe só a app, sem HTTPS / Caddy)

cd /d "%~dp0"
set "ROOT=%CD%"
set "SKIP_PULL=0"
set "NO_PAUSE=0"
set "COMPOSE=docker compose"
set "COMPOSE_ARGS=-f docker-compose.yml -f deploy/linux-debian/docker-compose.caddy.yml"
set "USE_CADDY=1"
set "APP_PORT=8080"
set "CACHEBUST=0"
set "CADDY_FILE=deploy\linux-debian\docker-compose.caddy.yml"

for %%A in (%*) do (
  if /i "%%~A"=="--skip-pull" set "SKIP_PULL=1"
  if /i "%%~A"=="--no-pause" set "NO_PAUSE=1"
  if /i "%%~A"=="--no-caddy" set "USE_CADDY=0"
)

echo.
echo ============================================
echo   NetQuasar — actualizacao
echo   Pasta: %ROOT%
echo   Data:  %DATE% %TIME%
echo ============================================
echo.

call :step "Ferramentas (git, docker, compose)"
call :require_cmd git
if errorlevel 1 goto :fail
call :require_cmd docker
if errorlevel 1 goto :fail

docker compose version >nul 2>&1
if errorlevel 1 (
  docker-compose version >nul 2>&1
  if errorlevel 1 (
    echo ERRO: nem "docker compose" nem "docker-compose" estao disponiveis.
    goto :fail
  )
  set "COMPOSE=docker-compose"
)
echo   Compose: 
%COMPOSE% version
echo.

call :step "Motor Docker"
docker info >nul 2>&1
if errorlevel 1 (
  echo ERRO: o Docker nao esta a correr. Inicie o Docker Desktop / servico e tente de novo.
  goto :fail
)
echo   Docker OK.
echo.

call :step "Ficheiros do projecto"
if not exist "%ROOT%\docker-compose.yml" (
  echo ERRO: docker-compose.yml nao encontrado em "%ROOT%".
  goto :fail
)
if not exist "%ROOT%\Dockerfile" (
  echo ERRO: Dockerfile nao encontrado em "%ROOT%".
  goto :fail
)
if not exist "%ROOT%\.env" (
  echo ERRO: falta o ficheiro .env na raiz.
  echo       Copie:  copy .env.example .env
  echo       Depois edite POSTGRES_PASSWORD e NETQUASAR_SESSION_SECRET.
  goto :fail
)

call :require_env POSTGRES_USER
if errorlevel 1 goto :fail
call :require_env POSTGRES_PASSWORD
if errorlevel 1 goto :fail
call :require_env POSTGRES_DB
if errorlevel 1 goto :fail

call :read_env NETQUASAR_PUBLISH_PORT
if defined NETQUASAR_PUBLISH_PORT if not "!NETQUASAR_PUBLISH_PORT!"=="" set "APP_PORT=!NETQUASAR_PUBLISH_PORT!"

if "%USE_CADDY%"=="0" (
  set "COMPOSE_ARGS=-f docker-compose.yml"
  echo   [--no-caddy] A subir sem Caddy ^(so HTTP na porta %APP_PORT%^).
  echo.
) else (
  if not exist "%ROOT%\%CADDY_FILE%" (
    echo ERRO: falta "%CADDY_FILE%" para subir o Caddy.
    goto :fail
  )
  set "COMPOSE_ARGS=-f docker-compose.yml -f deploy/linux-debian/docker-compose.caddy.yml"
  call :read_env NETQUASAR_DOMAIN
  if "!NETQUASAR_DOMAIN!"=="" (
    echo ERRO: NETQUASAR_DOMAIN e obrigatorio para o Caddy.
    echo       No .env: NETQUASAR_DOMAIN=netquasar.seudominio.com
    echo       Ou: update.bat --no-caddy
    goto :fail
  )
)

%COMPOSE% !COMPOSE_ARGS! config -q
if errorlevel 1 (
  echo ERRO: docker-compose.yml / .env invalidos ^(compose config falhou^).
  if "!USE_CADDY!"=="1" echo       Confirme NETQUASAR_DOMAIN no .env ^(obrigatorio para o Caddy^).
  goto :fail
)
echo   .env e compose OK. Porta da app: %APP_PORT%
if "!USE_CADDY!"=="1" (
  echo   Caddy HTTPS: https://!NETQUASAR_DOMAIN!
) else (
  echo   Sem Caddy: http://127.0.0.1:%APP_PORT%
)
echo.

if "%SKIP_PULL%"=="1" (
  echo [--skip-pull] A ignorar git pull.
  echo.
  goto :docker
)

call :step "Repositorio Git"
git -C "%ROOT%" rev-parse --is-inside-work-tree >nul 2>&1
if errorlevel 1 (
  echo ERRO: "%ROOT%" nao e um repositorio git.
  goto :fail
)

git -C "%ROOT%" remote get-url origin >nul 2>&1
if errorlevel 1 (
  echo ERRO: falta o remoto "origin".
  echo       git remote add origin https://github.com/Kallebe-Assis/NetQuasar.git
  goto :fail
)

echo   Remoto:
git -C "%ROOT%" remote -v
echo.

echo   git fetch --prune...
git -C "%ROOT%" fetch --prune
if errorlevel 1 (
  echo ERRO: git fetch falhou ^(rede, credenciais ou remoto^).
  goto :fail
)

set "REMOTE_REF="
git -C "%ROOT%" show-ref --verify --quiet refs/remotes/origin/main
if not errorlevel 1 set "REMOTE_REF=origin/main"
if not defined REMOTE_REF (
  git -C "%ROOT%" show-ref --verify --quiet refs/remotes/origin/master
  if not errorlevel 1 set "REMOTE_REF=origin/master"
)
if not defined REMOTE_REF (
  echo ERRO: o remoto nao tem ramo main nem master.
  goto :fail
)

rem Repo acabado de iniciar (git init) ainda nao tem commits — alinha com o GitHub.
git -C "%ROOT%" rev-parse --verify HEAD >nul 2>&1
if errorlevel 1 (
  echo   Pasta sem commits locais. A ligar ao %REMOTE_REF%...
  git -C "%ROOT%" checkout -f -B main "%REMOTE_REF%"
  if errorlevel 1 (
    echo ERRO: nao foi possivel fazer checkout de %REMOTE_REF%.
    goto :fail
  )
  git -C "%ROOT%" branch --set-upstream-to="%REMOTE_REF%" main >nul 2>&1
)

for /f "delims=" %%B in ('git -C "%ROOT%" rev-parse --abbrev-ref HEAD') do set "BRANCH=%%B"
echo   Ramo: %BRANCH%

git -C "%ROOT%" rev-parse --abbrev-ref --symbolic-full-name "@{u}" >nul 2>&1
if errorlevel 1 (
  echo   Sem upstream. A seguir %REMOTE_REF%...
  git -C "%ROOT%" branch --set-upstream-to="%REMOTE_REF%" "%BRANCH%"
  if errorlevel 1 (
    git -C "%ROOT%" checkout -B main "%REMOTE_REF%"
    if errorlevel 1 (
      echo ERRO: nao foi possivel definir upstream para %REMOTE_REF%.
      goto :fail
    )
    git -C "%ROOT%" branch --set-upstream-to="%REMOTE_REF%" main >nul 2>&1
    set "BRANCH=main"
  )
)

git -C "%ROOT%" status -sb
echo.

git -C "%ROOT%" diff --quiet --ignore-submodules HEAD
if errorlevel 1 (
  echo AVISO: ha alteracoes locais por commitar. O pull pode falhar se o remoto tocar nos mesmos ficheiros.
  git -C "%ROOT%" status --short
  echo.
)

for /f %%C in ('git -C "%ROOT%" rev-list --count "HEAD..@{u}"') do set "BEHIND=%%C"
for /f %%C in ('git -C "%ROOT%" rev-list --count "@{u}..HEAD"') do set "AHEAD=%%C"
echo   Relativo ao remoto: %BEHIND% commit(s) atras / %AHEAD% commit(s) a frente.

if not "%AHEAD%"=="0" if not "%BEHIND%"=="0" (
  echo ERRO: historico divergiu do remoto. Resolva o Git manualmente antes de actualizar.
  goto :fail
)
if not "%AHEAD%"=="0" (
  echo ERRO: ha commits locais que ainda nao foram enviados. Use --skip-pull ou faca push/rebase.
  goto :fail
)

if "%BEHIND%"=="0" (
  echo   Ja esta actualizado com o remoto.
) else (
  echo   git pull --ff-only...
  git -C "%ROOT%" pull --ff-only
  if errorlevel 1 (
    echo ERRO: git pull --ff-only falhou.
    goto :fail
  )
)

echo   Commit actual:
git -C "%ROOT%" log -1 --oneline
echo.

:docker
call :step "Imagens e stack Docker"
set "DOCKER_BUILDKIT=1"
set "COMPOSE_DOCKER_CLI_BUILD=1"

for /f %%T in ('powershell -NoProfile -Command "[DateTimeOffset]::UtcNow.ToUnixTimeSeconds()"') do set "CACHEBUST=%%T"
if "%CACHEBUST%"=="0" set "CACHEBUST=%RANDOM%%RANDOM%"
echo   CACHEBUST=%CACHEBUST%  ^(forca rebuild da UI no Docker^)
echo.

echo   A puxar imagens de postgres/redis...
%COMPOSE% !COMPOSE_ARGS! pull postgres redis
if errorlevel 1 (
  echo AVISO: pull das imagens base falhou. A continuar com o que ja existir em cache.
  echo.
)
if "!USE_CADDY!"=="1" (
  echo   A puxar imagem do Caddy...
  %COMPOSE% !COMPOSE_ARGS! pull caddy
  if errorlevel 1 echo AVISO: pull do Caddy falhou. A continuar com a imagem em cache.
  echo.
)

echo   docker compose !COMPOSE_ARGS! up --build -d --remove-orphans
echo   ^(pode demorar varios minutos na primeira vez ou apos mudancas^)
echo.
%COMPOSE% !COMPOSE_ARGS! up --build -d --remove-orphans
if errorlevel 1 (
  echo ERRO: docker compose up falhou.
  echo.
  echo --- ultimos logs netquasar ---
  %COMPOSE% !COMPOSE_ARGS! logs --tail=80 netquasar
  if "!USE_CADDY!"=="1" (
    echo --- ultimos logs caddy ---
    %COMPOSE% !COMPOSE_ARGS! logs --tail=80 caddy
  )
  goto :fail
)
echo.

call :step "Saude dos contentores"
%COMPOSE% !COMPOSE_ARGS! ps
echo.

echo   A aguardar Postgres healthy...
set "PG_OK=0"
for /L %%I in (1,1,30) do (
  if "!PG_OK!"=="0" (
    %COMPOSE% !COMPOSE_ARGS! exec -T postgres pg_isready -U !POSTGRES_USER! -d !POSTGRES_DB! >nul 2>&1
    if not errorlevel 1 (
      set "PG_OK=1"
    ) else (
      timeout /t 2 /nobreak >nul
    )
  )
)
if "!PG_OK!"=="0" (
  echo ERRO: Postgres nao ficou ready a tempo.
  %COMPOSE% !COMPOSE_ARGS! logs --tail=40 postgres
  goto :fail
)
echo   Postgres OK.

echo   A aguardar http://127.0.0.1:%APP_PORT%/health ...
set "APP_OK=0"
for /L %%I in (1,1,45) do (
  if "!APP_OK!"=="0" (
    curl.exe -fsS --max-time 3 "http://127.0.0.1:%APP_PORT%/health" >nul 2>&1
    if not errorlevel 1 (
      set "APP_OK=1"
    ) else (
      timeout /t 2 /nobreak >nul
    )
  )
)
if "!APP_OK!"=="0" (
  echo ERRO: a API nao respondeu em /health na porta %APP_PORT%.
  echo       Confirme NETQUASAR_PUBLISH_PORT no .env e os logs abaixo.
  echo.
  %COMPOSE% !COMPOSE_ARGS! logs --tail=80 netquasar
  goto :fail
)

if "!USE_CADDY!"=="1" (
  echo   A confirmar contentor Caddy...
  %COMPOSE% !COMPOSE_ARGS! ps --status running caddy 2>nul | findstr /i caddy >nul
  if errorlevel 1 (
    echo ERRO: o Caddy nao esta a correr.
    %COMPOSE% !COMPOSE_ARGS! logs --tail=80 caddy
    goto :fail
  )
  echo   Caddy OK.
)

echo.
echo ============================================
echo   Concluido.
echo   UI/API:  http://127.0.0.1:%APP_PORT%
echo   Health:  http://127.0.0.1:%APP_PORT%/health
if "!USE_CADDY!"=="1" (
  echo   HTTPS:   https://!NETQUASAR_DOMAIN!
)
echo ============================================
echo.
if "%NO_PAUSE%"=="0" pause
endlocal
exit /b 0

:fail
echo.
echo ============================================
echo   FALHOU. Veja as mensagens acima.
echo ============================================
echo.
if "%NO_PAUSE%"=="0" pause
endlocal
exit /b 1

:step
echo --------------------------------------------
echo ^> %~1
echo --------------------------------------------
exit /b 0

:require_cmd
where %~1 >nul 2>&1
if errorlevel 1 (
  echo ERRO: comando "%~1" nao encontrado no PATH.
  exit /b 1
)
exit /b 0

:require_env
call :read_env %~1
if "!%~1!"=="" (
  echo ERRO: variavel %~1 ausente ou vazia no .env
  exit /b 1
)
exit /b 0

:read_env
set "_KEY=%~1"
set "_VAL="
for /f "usebackq tokens=1,* delims==" %%A in (`findstr /b /c:"%_KEY%=" "%ROOT%\.env"`) do (
  set "_VAL=%%B"
)
if defined _VAL (
  set "_VAL=!_VAL:"=!"
  set "%_KEY%=!_VAL!"
)
exit /b 0
