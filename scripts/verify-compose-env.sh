#!/usr/bin/env bash
# Verifica ficheiros e variáveis mínimas antes de `docker compose up`.
# Uso (na raiz do repositório): bash scripts/verify-compose-env.sh

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

err() { echo "Erro: $*" >&2; exit 1; }

[[ -f docker-compose.yml ]] || err "docker-compose.yml não encontrado em $ROOT (execute na raiz do repo)."
[[ -f Dockerfile ]] || err "Dockerfile não encontrado em $ROOT."
[[ -f .env ]] || err "Falta .env — copie: cp .env.example .env e edite POSTGRES_PASSWORD."

required_vars=(POSTGRES_USER POSTGRES_PASSWORD POSTGRES_DB)
for v in "${required_vars[@]}"; do
  if ! grep -E "^${v}=" .env >/dev/null 2>&1; then
    err "Variável obrigatória ausente ou mal formatada no .env: $v (esperada linha ${v}=...)"
  fi
  val="$(grep -E "^${v}=" .env | head -1 | cut -d= -f2-)"
  if [[ -z "${val// /}" ]]; then
    err "Variável $v está vazia no .env."
  fi
done

if grep -E '^NETQUASAR_DATABASE_URL=' .env >/dev/null 2>&1; then
  echo "Aviso: NETQUASAR_DATABASE_URL está definido no .env — a app usará essa URL (ex.: Supabase :6543)." >&2
  echo "  O contentor Postgres do Compose continua a subir, mas o NetQuasar pode não o utilizar." >&2
fi

echo "OK: .env e ficheiros Compose parecem consistentes em $ROOT"
