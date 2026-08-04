#Requires -Version 5.1
<#
.SYNOPSIS
  Dump completo (schema+dados) da base local para out/netquasar-full-<stamp>.pgdump
#>
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "_lib.ps1")
Import-OneShotEnv

Require-Command "pg_dump" | Out-Null
$local = Require-Env "LOCAL_DATABASE_URL"
$outDir = Get-OneShotOutDir
$stamp = Get-Stamp
$outFile = Join-Path $outDir "netquasar-full-$stamp.pgdump"

Write-Host "Origem local:" (Redact-DatabaseUrl $local)
Write-Host "Destino:" $outFile

$args = @(
  "--dbname=$local",
  "--format=custom",
  "--compress=9",
  "--no-owner",
  "--no-acl",
  "--file=$outFile",
  "--verbose"
)

& pg_dump @args
if ($LASTEXITCODE -ne 0) {
  throw "pg_dump (full) falhou com código $LASTEXITCODE"
}

$info = Get-Item -LiteralPath $outFile
Write-Host ("OK: dump full ({0:N1} MB)" -f ($info.Length / 1MB))
Write-Host "Próximo: configure b2.env e corra .\06-upload-b2.ps1"
Write-Host "  (ou: .\06-upload-b2.ps1 -DumpFile `"$outFile`")"
