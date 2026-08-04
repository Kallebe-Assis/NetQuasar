#Requires -Version 5.1
<#
.SYNOPSIS
  Conta linhas em tabelas-chave na base local (e opcionalmente no Supabase).
#>
param(
  [switch]$CompareSupabase
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "_lib.ps1")
Import-OneShotEnv

Require-Command "psql" | Out-Null
$local = Require-Env "LOCAL_DATABASE_URL"
$outDir = Get-OneShotOutDir
$stamp = Get-Stamp
$report = Join-Path $outDir "verify-$stamp.txt"

$tables = @(
  "users",
  "pops",
  "devices",
  "alert_instances",
  "network_projects",
  "network_ctos",
  "network_splice_boxes",
  "network_cables",
  "network_poles",
  "client_connections",
  "goose_db_version"
)

function Get-TableCounts {
  param([string]$DatabaseUrl, [string[]]$TableList)
  $rows = @()
  foreach ($t in $TableList) {
    $sql = "SELECT COUNT(*)::bigint FROM $t;"
    $out = & psql --dbname=$DatabaseUrl -v ON_ERROR_STOP=1 -t -A -c $sql 2>&1
    if ($LASTEXITCODE -ne 0) {
      $rows += [pscustomobject]@{ table = $t; count = "ERR"; detail = ($out | Out-String).Trim() }
      continue
    }
    $n = ($out | Out-String).Trim()
    $rows += [pscustomobject]@{ table = $t; count = $n; detail = "" }
  }
  return $rows
}

Write-Host "Local:" (Redact-DatabaseUrl $local)
$localCounts = Get-TableCounts -DatabaseUrl $local -TableList $tables

$lines = New-Object System.Collections.Generic.List[string]
$lines.Add(("NetQuasar verify - {0}" -f (Get-Date -Format o)))
$lines.Add(("LOCAL {0}" -f ($local -replace ':[^:@/]+@', ':***@')))
$lines.Add("")
$lines.Add("--- Contagens locais ---")
foreach ($r in $localCounts) {
  $line = "{0,-28} {1}" -f $r.table, $r.count
  $lines.Add($line)
  Write-Host $line
  if ($r.detail) { Write-Warning "$($r.table): $($r.detail)" }
}

if ($CompareSupabase) {
  $src = Require-Env "SUPABASE_DATABASE_URL"
  Write-Host ""
  Write-Host "Supabase:" (Redact-DatabaseUrl $src)
  $srcCounts = Get-TableCounts -DatabaseUrl $src -TableList $tables
  $lines.Add("")
  $lines.Add("--- Contagens Supabase ---")
  $map = @{}
  foreach ($r in $localCounts) { $map[$r.table] = $r.count }
  foreach ($r in $srcCounts) {
    $localN = $map[$r.table]
    $mark = if ($localN -eq $r.count) { "OK" } else { "DIFF" }
    $line = "{0,-28} supabase={1,-10} local={2,-10} {3}" -f $r.table, $r.count, $localN, $mark
    $lines.Add($line)
    Write-Host $line
  }
}

$lines | Set-Content -LiteralPath $report -Encoding UTF8
Write-Host ""
Write-Host "Relatório:" $report
Write-Host "Se as contagens estiverem coerentes: .\05-dump-local-full.ps1"
