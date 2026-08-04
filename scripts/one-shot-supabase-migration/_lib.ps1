# Helpers partilhados pelos scripts one-shot (dot-source).

$script:OneShotRoot = $PSScriptRoot

function Import-DotEnvFile {
  param([Parameter(Mandatory = $true)][string]$Path)
  if (-not (Test-Path -LiteralPath $Path)) { return }
  Get-Content -LiteralPath $Path -Encoding UTF8 | ForEach-Object {
    $line = $_.Trim()
    if ($line -eq "" -or $line.StartsWith("#")) { return }
    $i = $line.IndexOf("=")
    if ($i -le 0) { return }
    $k = $line.Substring(0, $i).Trim()
    $v = $line.Substring($i + 1).Trim()
    if ($k -eq "") { return }
    # Não sobrescrever variáveis já definidas no processo.
    $existing = [Environment]::GetEnvironmentVariable($k, "Process")
    if (-not [string]::IsNullOrEmpty($existing)) { return }
    [Environment]::SetEnvironmentVariable($k, $v, "Process")
    Set-Item -Path "Env:$k" -Value $v
  }
}

function Import-OneShotEnv {
  Import-DotEnvFile (Join-Path $script:OneShotRoot ".env")
  Import-DotEnvFile (Join-Path $script:OneShotRoot "b2.env")
}

function Get-OneShotOutDir {
  if ($env:OUT_DIR -and $env:OUT_DIR.Trim() -ne "") {
    $dir = $env:OUT_DIR.Trim()
  } else {
    $dir = Join-Path $script:OneShotRoot "out"
  }
  if (-not (Test-Path -LiteralPath $dir)) {
    New-Item -ItemType Directory -Path $dir -Force | Out-Null
  }
  return (Resolve-Path -LiteralPath $dir).Path
}

function Ensure-PostgresClientPath {
  # Instalação Windows típica não adiciona bin/ ao PATH do utilizador.
  if (Get-Command pg_dump -ErrorAction SilentlyContinue) { return }
  $candidates = @(
    "C:\Program Files\PostgreSQL\18\bin",
    "C:\Program Files\PostgreSQL\17\bin",
    "C:\Program Files\PostgreSQL\16\bin",
    "C:\Program Files\PostgreSQL\15\bin",
    "C:\Program Files\PostgreSQL\18\pgAdmin 4\runtime"
  )
  foreach ($dir in $candidates) {
    $dump = Join-Path $dir "pg_dump.exe"
    if (Test-Path -LiteralPath $dump) {
      $env:Path = "$dir;$env:Path"
      Write-Host "PATH: adicionou clientes Postgres em $dir"
      return
    }
  }
}

Ensure-PostgresClientPath

function Require-Command {
  param([Parameter(Mandatory = $true)][string]$Name)
  Ensure-PostgresClientPath
  $cmd = Get-Command $Name -ErrorAction SilentlyContinue
  if (-not $cmd) {
    throw @"
Comando '$Name' não encontrado no PATH.
Os binários costumam estar em: C:\Program Files\PostgreSQL\18\bin
No PowerShell (sessão actual):
  `$env:Path = 'C:\Program Files\PostgreSQL\18\bin;' + `$env:Path
Ou adicione essa pasta ao PATH do utilizador nas Variáveis de Ambiente do Windows.
"@
  }
  return $cmd.Source
}

function Require-Env {
  param([Parameter(Mandatory = $true)][string]$Name)
  $v = [Environment]::GetEnvironmentVariable($Name, "Process")
  if ([string]::IsNullOrWhiteSpace($v)) {
    throw "Variável $Name em falta. Copie .env.example → .env (e b2.env.example → b2.env se for upload) e preencha."
  }
  return $v.Trim()
}

function Redact-DatabaseUrl {
  param([string]$Url)
  try {
    $u = [Uri]$Url
    if ($u.UserInfo -match ":") {
      $user = $u.UserInfo.Split(":")[0]
      return ($Url -replace [regex]::Escape($u.UserInfo), ("{0}:***" -f $user))
    }
  } catch {}
  return $Url
}

function Get-RepoRoot {
  return (Resolve-Path (Join-Path $script:OneShotRoot "..\..")).Path
}

function Get-Stamp {
  return (Get-Date -Format "yyyyMMdd-HHmm")
}
