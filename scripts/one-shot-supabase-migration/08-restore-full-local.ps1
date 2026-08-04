#Requires -Version 5.1
<#
.SYNOPSIS
  Restaura um dump FULL (schema + dados) no Postgres local — cenario de disaster recovery.
.PARAMETER DumpFile
  .pgdump a restaurar. Default: from-b2-netquasar-full-*.pgdump mais recente; senao netquasar-full-*.
.PARAMETER SkipWipe
  Se definido, nao faz DROP SCHEMA public (usa pg_restore --clean --if-exists).
#>
param(
  [string]$DumpFile = "",
  [switch]$SkipWipe
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "_lib.ps1")
Import-OneShotEnv

Require-Command "pg_restore" | Out-Null
Require-Command "psql" | Out-Null
$local = Require-Env "LOCAL_DATABASE_URL"
$outDir = Get-OneShotOutDir

if ([string]::IsNullOrWhiteSpace($DumpFile)) {
  $latest = Get-ChildItem -LiteralPath $outDir -Filter "from-b2-netquasar-full-*.pgdump" -ErrorAction SilentlyContinue |
    Sort-Object LastWriteTime -Descending |
    Select-Object -First 1
  if (-not $latest) {
    $latest = Get-ChildItem -LiteralPath $outDir -Filter "netquasar-full-*.pgdump" -ErrorAction SilentlyContinue |
      Sort-Object LastWriteTime -Descending |
      Select-Object -First 1
  }
  if (-not $latest) {
    throw "Nenhum dump full em out/. Corra .\07-download-b2.ps1 ou .\05-dump-local-full.ps1."
  }
  $DumpFile = $latest.FullName
}

if (-not (Test-Path -LiteralPath $DumpFile)) {
  throw "Dump nao encontrado: $DumpFile"
}

Write-Host "Dump:" $DumpFile
Write-Host ("Tamanho: {0:N1} MB" -f ((Get-Item -LiteralPath $DumpFile).Length / 1MB))
Write-Host "Destino:" (Redact-DatabaseUrl $local)

if (-not $SkipWipe) {
  Write-Host "A limpar schema public (DROP CASCADE) para restore completo..."
  $wipeSql = @"
DROP SCHEMA IF EXISTS public CASCADE;
CREATE SCHEMA public;
GRANT ALL ON SCHEMA public TO CURRENT_USER;
GRANT ALL ON SCHEMA public TO public;
GRANT USAGE ON SCHEMA public TO public;
"@
  & psql --dbname=$local -v ON_ERROR_STOP=1 -c $wipeSql
  if ($LASTEXITCODE -ne 0) {
    throw "Wipe do schema public falhou com codigo $LASTEXITCODE"
  }
}

$args = @(
  "--dbname=$local",
  "--no-owner",
  "--no-acl",
  "--verbose",
  $DumpFile
)
if ($SkipWipe) {
  $args = @("--clean", "--if-exists") + $args
}

& pg_restore @args
$code = $LASTEXITCODE
if ($code -gt 1) {
  throw "pg_restore falhou com codigo $code"
}
if ($code -eq 1) {
  Write-Warning "pg_restore terminou com avisos (codigo 1). Corra .\04-verify.ps1"
} else {
  Write-Host "OK: restore FULL concluido."
}
Write-Host "Proximo: .\04-verify.ps1"
