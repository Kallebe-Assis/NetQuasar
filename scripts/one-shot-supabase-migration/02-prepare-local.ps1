#Requires -Version 5.1
<#
.SYNOPSIS
  Prepara a base local: aplica migrations Goose (schema vazio de negócio).
#>
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "_lib.ps1")
Import-OneShotEnv

$local = Require-Env "LOCAL_DATABASE_URL"
$repo = Get-RepoRoot
$backend = Join-Path $repo "quasar_backend"

if (-not (Test-Path -LiteralPath (Join-Path $backend "cmd\migrate"))) {
  throw "Não encontrei quasar_backend/cmd/migrate em $repo"
}

Require-Command "go" | Out-Null

Write-Host "Destino local:" (Redact-DatabaseUrl $local)
Write-Host "A aplicar Goose migrations em quasar_backend (NETQUASAR_DATABASE_URL = LOCAL)..."

Push-Location $backend
try {
  $env:NETQUASAR_DATABASE_URL = $local
  # Evitar que .env do Supabase sobrescreva via loadEnvFiles: netquasar/migrate
  # só preenche se a env do processo estiver vazia — já está definida, OK.
  & go run .\cmd\migrate\
  if ($LASTEXITCODE -ne 0) {
    throw "go run ./cmd/migrate falhou com código $LASTEXITCODE"
  }
} finally {
  Pop-Location
}

Write-Host "OK: schema local actualizado via Goose."
Write-Host "Próximo: .\03-restore-local.ps1  (opcional: -DumpFile caminho)"
