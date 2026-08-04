# Migração pontual Supabase → Postgres local → Backblaze B2
#
# Scripts descartáveis (não fazem parte do produto). Correr **por ordem** a partir desta pasta.
#
# Pré-requisitos
# - PostgreSQL 18+ instalado (binários em C:\Program Files\PostgreSQL\18\bin).
#   Os scripts adicionam essa pasta ao PATH automaticamente se pg_dump não estiver no PATH.
#   No PowerShell (fora do prompt SQL do psql):  pg_dump --version
# - Go no PATH (para Goose via quasar_backend/cmd/migrate)
# - Upload B2: API nativa (AWS CLI / S3 da AWS NÃO é necessário)
# - Ficheiro `.env` (a partir de `.env.example`) com SUPABASE_DATABASE_URL e LOCAL_DATABASE_URL
# - Para B2: `b2.env` (a partir de `b2.env.example`) — bucket name (ex.: NetQuasar), não só o bucket ID
#
# Ordem
# 1. 01-dump-supabase.ps1     → out/supabase-data-*.pgdump
# 2. 02-prepare-local.ps1     → schema Goose na base local
# 3. 03-restore-local.ps1     → dados do dump na local
# 4. 04-verify.ps1            → contagens
# 5. 05-dump-local-full.ps1   → out/netquasar-full-*.pgdump
# 6. 06-upload-b2.ps1         → objecto no bucket B2 (prefixo netquasar/postgres/)
#
# Disaster recovery (B2 → Postgres local)
# 7. 07-download-b2.ps1       → out/from-b2-netquasar-full-*.pgdump
# 8. 08-restore-full-local.ps1 → DROP SCHEMA public + restore FULL (schema+dados)
# 9. 04-verify.ps1            → confirmar contagens
#
# Notas
# - Dump do Supabase é **data-only**; o schema vem das migrations do repositório.
# - Preferir URL directa `db.<ref>.supabase.co:5432` se o Session pooler falhar no pg_dump.
# - Passwords com *, $, @ → URL-encode na DSN.
# - Nunca commitir `.env`, `b2.env`, nem ficheiros em `out/`.
# - Não apagar o projeto Supabase até verificar local + ficheiro no B2 (Browse Files).
# - Bucket B2 deve ser **private**. Uploads >500 MB: use o script (não a UI web).

## Exemplo rápido

```powershell
cd scripts\one-shot-supabase-migration
copy .env.example .env
# editar .env

.\01-dump-supabase.ps1
.\02-prepare-local.ps1
.\03-restore-local.ps1
.\04-verify.ps1

copy b2.env.example b2.env
# editar b2.env

.\05-dump-local-full.ps1
.\06-upload-b2.ps1
```

## Restaurar do Backblaze (teste local)

```powershell
cd scripts\one-shot-supabase-migration
.\07-download-b2.ps1
.\08-restore-full-local.ps1
.\04-verify.ps1
```

Isto limpa o schema `public` local e recarrega **tudo** a partir do dump full no B2.

## Confirmacao no Backblaze

1. Consola B2 → B2 Cloud Storage → Browse Files  
2. Abrir o bucket → prefixo `netquasar/postgres/`  
3. Verificar Name, Size, Sha1
