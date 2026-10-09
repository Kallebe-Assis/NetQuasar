#!/usr/bin/env bash
# purge-old-data.sh — apaga TUDO que tem mais de N dias nas tabelas de histórico do NetQuasar e DEVOLVE o espaço ao disco.
#
# Uso (no servidor, dentro da pasta do projeto, com a stack no ar):
#   bash scripts/purge-old-data.sh            # 30 dias (padrão), pede confirmação
#   bash scripts/purge-old-data.sh 30 --yes   # sem perguntar
#   bash scripts/purge-old-data.sh 7  --dry-run   # só mostra o que seria apagado
#   bash scripts/purge-old-data.sh 30 --no-compact # só apaga (o disco NÃO libera — o espaço fica reservado no banco)
#   bash scripts/purge-old-data.sh 1 --yes --tables=ping_history   # só estas tabelas (separadas por vírgula)
#
# Por que não basta um DELETE: no PostgreSQL ele só marca as linhas como mortas; o arquivo da tabela não encolhe e o disco do
# servidor continua cheio. Aqui, por tabela:
#   • tudo antigo            → TRUNCATE (instantâneo, devolve tudo);
#   • maioria antiga (≥ 50%) → copia só o que FICA, TRUNCATE e devolve (rápido; precisa de espaço ≈ o que fica);
#   • poucas antigas         → DELETE em lotes (sem pico de log) + VACUUM [+ VACUUM FULL se houver folga de disco].
# É a mesma lógica do botão «Limpeza de dados históricos» em Configurações → Base de dados.
#
# Variáveis: NQ_DB (banco; padrão $POSTGRES_DB do container), NQ_COMPOSE_DIR (pasta do docker-compose.yml; padrão: a do script).
set -euo pipefail

DAYS=30
YES=0
DRY=0
COMPACT=1
ONLY=""
for a in "$@"; do
  case "$a" in
    --yes|-y) YES=1 ;;
    --dry-run) DRY=1 ;;
    --no-compact) COMPACT=0 ;;
    --tables=*) ONLY=",${a#--tables=}," ;;
    --help|-h) sed -n '2,20p' "$0"; exit 0 ;;
    ''|*[!0-9]*) echo "argumento inválido: $a (use um número de dias, --yes, --dry-run ou --no-compact)" >&2; exit 2 ;;
    *) DAYS="$a" ;;
  esac
done
if [ "$DAYS" -lt 1 ]; then echo "o mínimo é 1 dia" >&2; exit 2; fi

cd "${NQ_COMPOSE_DIR:-$(cd "$(dirname "$0")/.." && pwd)}"

# tabela:coluna_de_data:guarda   (guarda "closed" = só linhas com a data preenchida, ex.: alertas encerrados)
TABLES=(
  "ping_history:checked_at:"
  "telemetry_samples:collected_at:"
  "interface_snapshots:collected_at:"
  "olt_onu_samples:recorded_at:"
  "olt_pon_samples:recorded_at:"
  "bng_stats_samples:collected_at:"
  "bng_session_snapshots:captured_at:"
  "bng_login_events:disconnected_at:closed"
  "events:created_at:"
  "snmp_walk_jobs:created_at:"
  "onu_report_runs:started_at:"
  "automation_execution_log:started_at:"
  "integration_run_logs:created_at:"
  "settings_connection_audit:created_at:"
  "alert_instances:closed_at:closed"
)

MB=$((1024 * 1024))
MIN_COMPACT=$((64 * MB))
MARGIN=$((256 * MB))
UNKNOWN_DISK_MAX_KEEP=$((512 * MB))

# psql dentro do container; o SQL vem pelo stdin (NQ_DB opcional escolhe outro banco)
psqlq() { docker compose exec -T -e NQ_DB="${NQ_DB:-}" postgres sh -c 'psql -U "$POSTGRES_USER" -d "${NQ_DB:-$POSTGRES_DB}" -X -At -v ON_ERROR_STOP=1'; }
q() { printf '%s\n' "$1" | psqlq; }

human() { awk -v b="$1" 'BEGIN{split("B KB MB GB TB",u," ");i=1;while(b>=1000&&i<5){b/=1000;i++}printf "%.1f %s",b,u[i]}'; }

root_dir="$(docker info -f '{{.DockerRootDir}}' 2>/dev/null || echo /var/lib/docker)"
disk_free() { df -B1 --output=avail "$root_dir" 2>/dev/null | tail -1 | tr -d ' ' || true; }
has_room() { # $1 = bytes extras necessários
  local need="$1" free; free="$(disk_free)"
  if [[ "$free" =~ ^[0-9]+$ ]]; then
    [ "$free" -ge $(( need * 12 / 10 + MARGIN )) ]
  else
    [ "$need" -le "$UNKNOWN_DISK_MAX_KEEP" ]
  fi
}

CUTOFF="$(q "SELECT to_char((now() - interval '${DAYS} days') AT TIME ZONE 'UTC', 'YYYY-MM-DD\"T\"HH24:MI:SS\"Z\"')")"
echo "== Limpeza: dados com mais de ${DAYS} dia(s) (anteriores a ${CUTOFF}) =="
f0="$(disk_free)"; [[ "$f0" =~ ^[0-9]+$ ]] && echo "   disco livre agora: $(human "$f0")"

# ---- relatório prévio ----
plan=()
total_elig=0
for spec in "${TABLES[@]}"; do
  IFS=: read -r t col guard <<<"$spec"
  if [ -n "$ONLY" ] && [[ "$ONLY" != *",${t},"* ]]; then continue; fi
  [ "$(q "SELECT to_regclass('public.${t}') IS NOT NULL")" = "t" ] || continue
  cond="${col} < '${CUTOFF}'::timestamptz"
  [ "$guard" = "closed" ] && cond="${col} IS NOT NULL AND ${cond}"
  total="$(q "SELECT COUNT(*) FROM ${t}")"
  elig="$(q "SELECT COUNT(*) FROM ${t} WHERE ${cond}")"
  size="$(q "SELECT pg_total_relation_size('${t}')")"
  printf '   %-26s %12s linhas | %12s antigas | %10s\n' "$t" "$total" "$elig" "$(human "$size")"
  if [ "$elig" -gt 0 ]; then plan+=("${t}|${col}|${cond}|${total}|${elig}|${size}"); total_elig=$((total_elig + elig)); fi
done
if [ "${#plan[@]}" -eq 0 ]; then echo "Nada a apagar."; exit 0; fi
echo "   total elegível: ${total_elig} linha(s)"
[ "$DRY" = 1 ] && { echo "(--dry-run: nada foi alterado)"; exit 0; }

if [ "$YES" != 1 ]; then
  read -r -p "Apagar PERMANENTEMENTE essas linhas? Digite SIM: " ans
  [ "$ans" = "SIM" ] || { echo "cancelado."; exit 1; }
fi

# procedimento que apaga em lotes com COMMIT a cada lote (sem transação gigante)
q "CREATE OR REPLACE PROCEDURE nq_purge_batches(tbl text, cond text, batch int)
LANGUAGE plpgsql AS \$\$
DECLARE n bigint; done bigint := 0;
BEGIN
  LOOP
    EXECUTE format('DELETE FROM %I WHERE ctid = ANY(ARRAY(SELECT ctid FROM %I WHERE %s LIMIT %s))', tbl, tbl, cond, batch);
    GET DIAGNOSTICS n = ROW_COUNT;
    EXIT WHEN n = 0;
    done := done + n;
    COMMIT;
    IF done % 200000 = 0 THEN RAISE NOTICE '  % apagados em %', done, tbl; END IF;
  END LOOP;
END \$\$" >/dev/null
trap 'q "DROP PROCEDURE IF EXISTS nq_purge_batches(text,text,int)" >/dev/null 2>&1 || true' EXIT

for item in "${plan[@]}"; do
  IFS='|' read -r t col cond total elig size <<<"$item"
  kept=$((total - elig))
  kept_est=0; [ "$total" -gt 0 ] && kept_est=$(( size * kept / total ))   # estimativa proporcional
  echo "-> ${t}"

  # sobra de uma reescrita interrompida: devolve antes de continuar
  if [ "$(q "SELECT to_regclass('public._nq_keep_${t}') IS NOT NULL")" = "t" ]; then
    echo "   restaurando dados de uma limpeza interrompida..."
    q "BEGIN; INSERT INTO ${t} OVERRIDING SYSTEM VALUE SELECT * FROM _nq_keep_${t}; DROP TABLE _nq_keep_${t}; COMMIT;" >/dev/null
  fi
  gen="$(q "SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='${t}' AND is_generated='ALWAYS')")"

  if [ "$kept" -le 0 ]; then
    echo "   tudo antigo: TRUNCATE"
    if q "TRUNCATE TABLE ${t}" >/dev/null 2>&1; then continue; fi
    echo "   (TRUNCATE não foi possível — usando lotes)"
  elif [ "$COMPACT" = 1 ] && [ "$gen" != "t" ] && [ $((elig * 2)) -ge "$total" ] && [ "$size" -ge "$MIN_COMPACT" ] && has_room "$kept_est"; then
    echo "   maioria antiga: reescrevendo só o que fica (~$(human "$kept_est"))"
    if printf 'BEGIN;\nLOCK TABLE %s IN ACCESS EXCLUSIVE MODE;\nCREATE TABLE _nq_keep_%s AS SELECT * FROM %s WHERE NOT COALESCE((%s), false);\nTRUNCATE TABLE %s;\nCOMMIT;\n' "$t" "$t" "$t" "$cond" "$t" | psqlq >/dev/null; then
      q "BEGIN; INSERT INTO ${t} OVERRIDING SYSTEM VALUE SELECT * FROM _nq_keep_${t}; DROP TABLE _nq_keep_${t}; COMMIT; ANALYZE ${t};" >/dev/null \
        || { echo "ERRO ao devolver os dados: eles estão em _nq_keep_${t} e serão restaurados na próxima execução." >&2; exit 1; }
      continue
    fi
    echo "   (reescrita não foi possível — usando lotes)"
  fi

  echo "   apagando em lotes de 20000..."
  printf 'CALL nq_purge_batches(%s, %s, 20000);\n' "'${t}'" "\$q\$${cond}\$q\$" | psqlq 2>&1 | grep -v '^$' || true
  q "VACUUM (ANALYZE) ${t}" >/dev/null
  if [ "$COMPACT" = 1 ] && [ "$gen" != "t" ] && [ "$size" -ge "$MIN_COMPACT" ] && [ $((elig * 5)) -ge "$total" ]; then
    if has_room "$kept_est"; then
      echo "   compactando (VACUUM FULL) — bloqueia gravações nesta tabela até terminar..."
      q "VACUUM (FULL, ANALYZE) ${t}" >/dev/null
    else
      echo "   sem espaço livre suficiente para compactar: o espaço ficou reservado dentro do banco. Libere disco e rode de novo."
    fi
  fi
done

echo "== Resultado =="
for item in "${plan[@]}"; do
  IFS='|' read -r t col cond total elig size <<<"$item"
  printf '   %-26s %10s -> %10s\n' "$t" "$(human "$size")" "$(human "$(q "SELECT pg_total_relation_size('${t}')")")"
done
f1="$(disk_free)"; [[ "$f1" =~ ^[0-9]+$ ]] && echo "   disco livre agora: $(human "$f1")"
echo "Concluído."
