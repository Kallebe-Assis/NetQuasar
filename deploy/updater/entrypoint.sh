#!/bin/sh
# updater — faz polling de system_update_state (Postgres) e executa git pull + docker compose
# build/up quando a aba Sobre -> Versão do sistema pede uma verificação ou atualização. É o
# único serviço com acesso ao socket do Docker e ao checkout do repositório do host (ver
# docker-compose.yml) -- o netquasar nunca fala com Docker/git directamente.
set -u

REPO=/workspace
cd "$REPO" || { echo "[updater] não consegui entrar em $REPO"; exit 1; }

export PGPASSWORD="${NETQUASAR_DB_PASSWORD:-}"
DB_HOST="${NETQUASAR_DB_HOST:-postgres}"
DB_PORT="${NETQUASAR_DB_PORT:-5432}"
DB_USER="${NETQUASAR_DB_USER:-quasar}"
DB_NAME="${NETQUASAR_DB_NAME:-netquasar}"

psql_run() {
  psql -X -q -v ON_ERROR_STOP=1 -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" "$@"
}

# psql_var — para SQL que usa substituição de variável (:'nome'), necessária para escapar com
# segurança valores que podem ter aspas/quebras de linha (mensagens de erro, JSON). A
# interpolação :'nome' do psql só funciona quando o SQL vem pelo stdin — NÃO funciona com -c
# (confirmado ao vivo: "-c" falha com "syntax error at or near :" mesmo num caso trivial).
# $1 = SQL terminado em ";", restantes args = -v nome=valor ...
psql_var() {
  sql=$1
  shift
  printf '%s\n' "$sql" | psql_run "$@"
}

log() {
  echo "[updater] $(date -u +%Y-%m-%dT%H:%M:%SZ) $*"
}

get_status() {
  psql_run -tAc "SELECT status FROM system_update_state WHERE id=1" 2>/dev/null | tr -d '[:space:]'
}

get_field() {
  psql_run -tAc "SELECT coalesce($1::text,'') FROM system_update_state WHERE id=1" 2>/dev/null | tr -d '\r'
}

mark_failed() {
  msg=$1
  psql_var "UPDATE system_update_state SET status='failed', error_message=:'msg', updated_at=now() WHERE id=1;" -v msg="$msg" >/dev/null 2>&1
  log "FAILED: $msg"
}

# mark_update_failed — como mark_failed, mas usada só em do_update: também restaura
# monitoring_runtime a partir de previous_is_running/previous_monitoring_mode (gravados por
# POST /api/v1/system/update antes de pedir a atualização). do_check nunca mexe no monitoramento,
# por isso nunca deve chamar esta versão (os previous_* podem estar desactualizados de uma
# atualização anterior). Cobre o caso descoberto ao testar: se a atualização falha ANTES de tocar
# no container (git sujo/fetch/merge/build), o processo antigo continua vivo e o
# monitorworker.Run() dele lê monitoring_runtime a cada tick — sem isto, o monitoramento ficava
# parado para sempre porque só o arranque de um processo NOVO (bootstrap.ResumeAfterUpdate)
# sabia restaurá-lo, e nenhum processo novo chega a nascer quando a falha é tão cedo.
mark_update_failed() {
  msg=$1
  psql_var "
    UPDATE system_update_state SET status='failed', error_message=:'msg', updated_at=now() WHERE id=1;
    UPDATE monitoring_runtime m SET
      is_running = COALESCE(s.previous_is_running, false),
      monitoring_mode = CASE WHEN COALESCE(s.previous_is_running, false)
        THEN COALESCE(NULLIF(s.previous_monitoring_mode, ''), 'simple_ping') ELSE 'off' END,
      updated_at = now()
    FROM system_update_state s WHERE m.id = 1 AND s.id = 1;
  " -v msg="$msg" >/dev/null 2>&1
  log "FAILED: $msg"
}

do_check() {
  base=$(get_field base_commit)
  branch=$(get_field branch)
  [ -z "$branch" ] && branch=main
  psql_run -c "UPDATE system_update_state SET status='checking', updated_at=now() WHERE id=1" >/dev/null 2>&1

  if [ -z "$base" ] || [ "$base" = "unknown" ]; then
    mark_failed "Este build não tem informação de commit (base_commit desconhecido) — não é possível comparar com o GitHub."
    return
  fi

  if ! fetch_out=$(git fetch --quiet origin "$branch" 2>&1); then
    mark_failed "git fetch falhou: $(printf '%s' "$fetch_out" | tail -c 500)"
    return
  fi

  if ! counts=$(git rev-list --left-right --count "${base}...origin/${branch}" 2>&1); then
    mark_failed "Commit actual ($base) não encontrado no histórico local: $(printf '%s' "$counts" | tail -c 300)"
    return
  fi
  ahead=$(printf '%s' "$counts" | awk '{print $1}')
  behind=$(printf '%s' "$counts" | awk '{print $2}')

  commits_json=$(git log "${base}..origin/${branch}" --format='%H%x09%h%x09%an%x09%aI%x09%s' 2>/dev/null \
    | jq -R -s '
        split("\n") | map(select(length > 0)) | map(split("\t")) |
        map({sha: .[0], short: .[1], author: .[2], date: .[3], message: (.[4:] | join("\t"))})
      ')
  [ -z "$commits_json" ] && commits_json='[]'

  result_json=$(jq -n --argjson ahead "${ahead:-0}" --argjson behind "${behind:-0}" \
    --arg checked_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" --argjson commits "$commits_json" \
    '{ahead_by:$ahead, behind_by:$behind, checked_at:$checked_at, commits:$commits}')

  psql_var "UPDATE system_update_state SET status='check_done', check_result=:'json'::jsonb, updated_at=now() WHERE id=1;" -v json="$result_json" >/dev/null 2>&1
  log "check concluído: ahead=$ahead behind=$behind"
}

do_update() {
  branch=$(get_field branch)
  [ -z "$branch" ] && branch=main
  psql_run -c "UPDATE system_update_state SET status='updating', started_at=now(), updated_at=now() WHERE id=1" >/dev/null 2>&1

  if [ -n "$(git status --porcelain)" ]; then
    mark_update_failed "Repositório com alterações locais não commitadas — não é seguro atualizar automaticamente. Resolva manualmente (git status) e tente de novo."
    return
  fi

  if ! fetch_out=$(git fetch --quiet origin "$branch" 2>&1); then
    mark_update_failed "git fetch falhou: $(printf '%s' "$fetch_out" | tail -c 500)"
    return
  fi

  if ! merge_out=$(git merge --ff-only "origin/${branch}" 2>&1); then
    mark_update_failed "Não foi possível avançar (fast-forward) — histórico local divergiu de origin/${branch}: $(printf '%s' "$merge_out" | tail -c 500)"
    return
  fi

  commit=$(git rev-parse HEAD)
  short=$(git rev-parse --short HEAD)
  buildtime=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  log "a construir netquasar @ $short"

  if ! build_out=$(DOCKER_BUILDKIT=1 docker compose build netquasar \
      --build-arg CACHEBUST="$(date +%s)" \
      --build-arg GIT_COMMIT="$commit" \
      --build-arg GIT_COMMIT_SHORT="$short" \
      --build-arg BUILD_TIME="$buildtime" 2>&1); then
    mark_update_failed "Falha ao construir a imagem: $(printf '%s' "$build_out" | tail -c 1500)"
    return
  fi

  log "a recriar o container netquasar"
  if ! up_out=$(docker compose up -d --no-deps netquasar 2>&1); then
    mark_update_failed "Falha ao subir o container: $(printf '%s' "$up_out" | tail -c 1500)"
    return
  fi

  log "a aguardar /health responder..."
  ok=0
  i=0
  while [ "$i" -lt 30 ]; do
    sleep 2
    if curl -fsS -m 3 "http://netquasar:8080/health" >/dev/null 2>&1; then
      ok=1
      break
    fi
    i=$((i + 1))
  done

  if [ "$ok" != "1" ]; then
    mark_update_failed "Container reiniciou mas /health não respondeu em 60s — verifique os logs do netquasar."
    return
  fi
  # Não marca "completed" aqui de propósito: quem fecha o estado é o processo NOVO, no arranque
  # (internal/bootstrap.ResumeAfterUpdate) — evita corrida entre os dois a escrever o estado final.
  log "netquasar respondeu — a aguardar que o processo novo confirme (bootstrap.ResumeAfterUpdate)"
}

log "updater iniciado — repositório=$REPO"
while true; do
  status=$(get_status)
  case "$status" in
    check_requested) do_check ;;
    update_requested) do_update ;;
  esac
  sleep 3
done
