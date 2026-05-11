# Medições SNMP rápidas (Net-SNMP/net-snmp instalado no PATH).
# Ajuda a comparar com o tempo do backend (worker) quando houver timeouts na rede/agente.

param(
  [Parameter(Mandatory = $true)][string]$OltHost,
  [string]$Community = "public",
  [int]$SNMPPort = 161
)

$ErrorActionPreference = "Stop"

function Require-Cmd([string]$name) {
  if (-not (Get-Command $name -ErrorAction SilentlyContinue)) {
    Write-Host "Não encontrado '$name'. Instale Net-SNMP ou use: go run ./cmd/monitorprobe -host ..."
    exit 1
  }
}

Require-Cmd "snmpbulkwalk"
if (-not (Get-Command "Test-NetConnection" -ErrorAction SilentlyContinue)) {
  Write-Host "Test-NetConnection indisponível (PowerShell velho)."
}

Write-Host "=== ICMP (opcional; firewall pode bloquear) ===" 
try {
  $p = Test-Connection -ComputerName $OltHost -Count 2 -Quiet
  Write-Host "Ping OK: $p"
}
catch {
  Write-Host "Ping: $_"
}

Write-Host "`n=== TCP SNMP/161 ===" 
try {
  $t = Measure-Command { Test-NetConnection -ComputerName $OltHost -Port $SNMPPort -WarningAction SilentlyContinue }
  Write-Host "Test-NetConnection porta $SNMPPort levou $($t.TotalMilliseconds) ms"
}
catch {
  Write-Host "Test-NetConnection: $_"
}

Write-Host "`n=== TCP Telnet/23 (latência de abertura) ===" 
try {
  $tt = Measure-Command { Test-NetConnection -ComputerName $OltHost -Port 23 -WarningAction SilentlyContinue }
  Write-Host "Test-NetConnection porta 23 levou $($tt.TotalMilliseconds) ms"
}
catch {
  Write-Host "Telnet TCP: $_"
}

$oids = @(
  @{ Name = "IF-MIB ifTable"; Oid = "1.3.6.1.2.1.2.2.1" },
  @{ Name = "IF-MIB ifXTable"; Oid = "1.3.6.1.2.1.31.1.1.1" }
)

foreach ($o in $oids) {
  Write-Host "`n=== snmpbulkwalk $($o.Name) ===" 
  $args = @("-v2c", "-c", $Community, "-t", "30", "-r", "1", "${OltHost}:${SNMPPort}", $o.Oid)
  $sw = [System.Diagnostics.Stopwatch]::StartNew()
  try {
    $out = & snmpbulkwalk @args 2>&1
    $sw.Stop()
    $lines = ($out | Measure-Object -Line).Lines
    Write-Host "Linhas (aprox.): $lines | Wall clock: $($sw.ElapsedMilliseconds) ms"
  }
  catch {
    $sw.Stop()
    Write-Host "Erro: $_ | Após $($sw.ElapsedMilliseconds) ms"
  }
}

Write-Host "`nPróximo passo: na pasta quasar_backend, `go run ./cmd/monitorprobe -host $OltHost -community $Community`"
