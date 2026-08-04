#Requires -Version 5.1
<#
.SYNOPSIS
  Restaura o dump data-only do Supabase na base local.
.PARAMETER DumpFile
  Caminho do .pgdump. Se omitido, usa o supabase-data-*.pgdump mais recente em out/.
#>
param(
  [string]$DumpFile = ""
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "_lib.ps1")
Import-OneShotEnv

Require-Command "pg_restore" | Out-Null
Require-Command "psql" | Out-Null
$local = Require-Env "LOCAL_DATABASE_URL"
$outDir = Get-OneShotOutDir

if ([string]::IsNullOrWhiteSpace($DumpFile)) {
  $latest = Get-ChildItem -LiteralPath $outDir -Filter "supabase-data-*.pgdump" -ErrorAction SilentlyContinue |
    Sort-Object LastWriteTime -Descending |
    Select-Object -First 1
  if (-not $latest) {
    throw "Nenhum out/supabase-data-*.pgdump encontrado. Corra .\01-dump-supabase.ps1 primeiro."
  }
  $DumpFile = $latest.FullName
}

if (-not (Test-Path -LiteralPath $DumpFile)) {
  throw "Dump não encontrado: $DumpFile"
}

Write-Host "Dump:" $DumpFile
Write-Host "Destino:" (Redact-DatabaseUrl $local)

# Goose deixa seeds (settings, profiles, etc.) que colidem com o dump do Supabase.
# Truncamos public (exceto goose_db_version) antes do data-only restore.
Write-Host "A truncar tabelas public (exceto goose_db_version)..."
$truncateSql = @"
DO `$`$
DECLARE r RECORD;
BEGIN
  FOR r IN (
    SELECT tablename FROM pg_tables
    WHERE schemaname = 'public' AND tablename <> 'goose_db_version'
  ) LOOP
    EXECUTE format('TRUNCATE TABLE public.%I CASCADE', r.tablename);
  END LOOP;
END
`$`$;
"@
& psql --dbname=$local -v ON_ERROR_STOP=1 -c $truncateSql
if ($LASTEXITCODE -ne 0) {
  throw "TRUNCATE pré-restore falhou com código $LASTEXITCODE"
}

# Só public: auth/storage/realtime/vault são do Supabase e não existem no Postgres local.
# --disable-triggers ajuda com ordem de FKs (requer privilégios de table owner / superuser).
$args = @(
  "--dbname=$local",
  "--data-only",
  "--schema=public",
  "--no-owner",
  "--no-acl",
  "--disable-triggers",
  "--verbose",
  $DumpFile
)

& pg_restore @args
$code = $LASTEXITCODE
# pg_restore devolve 1 se houve warnings (ex.: tabela em falta); 0 = limpo.
if ($code -gt 1) {
  throw "pg_restore falhou com código $code"
}
if ($code -eq 1) {
  Write-Warning "pg_restore terminou com avisos (código 1). Reveja o log; depois corra .\04-verify.ps1"
} else {
  Write-Host "OK: restore concluído."
}
Write-Host "Próximo: .\04-verify.ps1"
