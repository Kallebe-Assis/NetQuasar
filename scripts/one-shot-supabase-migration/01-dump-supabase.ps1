#Requires -Version 5.1
<#
.SYNOPSIS
  Dump data-only do Supabase (formato custom) para out/supabase-data-<stamp>.pgdump
#>
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "_lib.ps1")
Import-OneShotEnv

Require-Command "pg_dump" | Out-Null
$src = Require-Env "SUPABASE_DATABASE_URL"
$outDir = Get-OneShotOutDir
$stamp = Get-Stamp
$outFile = Join-Path $outDir "supabase-data-$stamp.pgdump"

Write-Host "Origem:" (Redact-DatabaseUrl $src)
Write-Host "Destino dump:" $outFile

# Schema virá do Goose no destino. Excluímos goose_db_version do data dump
# (versão local será a das migrations embutidas).
$pgDumpArgs = @(
  "--dbname=$src",
  "--format=custom",
  "--data-only",
  "--schema=public",
  "--no-owner",
  "--no-acl",
  "--exclude-table-data=goose_db_version",
  "--exclude-table-data=public.goose_db_version",
  "--file=$outFile",
  "--verbose"
)

& pg_dump @pgDumpArgs
if ($LASTEXITCODE -ne 0) {
  throw "pg_dump falhou com código $LASTEXITCODE. Tente URL directa db.<ref>.supabase.co:5432 em vez do pooler."
}

$info = Get-Item -LiteralPath $outFile
Write-Host ("OK: dump criado ({0:N1} MB)" -f ($info.Length / 1MB))
Write-Host "Próximo: .\02-prepare-local.ps1"
