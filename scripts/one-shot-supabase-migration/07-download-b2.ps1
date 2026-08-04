#Requires -Version 5.1
<#
.SYNOPSIS
  Baixa o dump full mais recente do Backblaze B2 (API nativa) para out/.
.PARAMETER FileName
  Nome do objecto sob B2_PREFIX (ex.: netquasar-full-20260803-1203.pgdump).
  Se omitido, escolhe o netquasar-full-*.pgdump mais recente no prefixo.
#>
param(
  [string]$FileName = ""
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "_lib.ps1")
Import-OneShotEnv

$keyId = Require-Env "B2_KEY_ID"
$appKey = Require-Env "B2_APPLICATION_KEY"
$bucket = Require-Env "B2_BUCKET"
$prefix = if ($env:B2_PREFIX -and $env:B2_PREFIX.Trim() -ne "") {
  $env:B2_PREFIX.Trim().TrimEnd("/")
} else {
  "netquasar/postgres"
}
$outDir = Get-OneShotOutDir

$pair = "${keyId}:${appKey}"
$b64 = [Convert]::ToBase64String([Text.Encoding]::ASCII.GetBytes($pair))
$auth = Invoke-RestMethod -Uri "https://api.backblazeb2.com/b2api/v2/b2_authorize_account" `
  -Headers @{ Authorization = "Basic $b64" } -Method Get

$bucketId = $auth.allowed.bucketId
if (-not $bucketId -and $env:B2_BUCKET_ID) {
  $bucketId = $env:B2_BUCKET_ID.Trim()
}
if (-not $bucketId) {
  $list = Invoke-RestMethod -Uri "$($auth.apiUrl)/b2api/v2/b2_list_buckets" `
    -Headers @{ Authorization = $auth.authorizationToken } `
    -Method Post -ContentType "application/json" `
    -Body (@{ accountId = $auth.accountId; bucketName = $bucket } | ConvertTo-Json)
  $bucketId = $list.buckets[0].bucketId
}
if (-not $bucketId) {
  throw "Nao foi possivel resolver bucketId para '$bucket'."
}

$remoteName = $null
$fileId = $null
$contentLength = 0

if ($FileName -and $FileName.Trim() -ne "") {
  $remoteName = "$prefix/$($FileName.Trim())"
} else {
  Write-Host "A listar ficheiros em $prefix/ ..."
  $start = $null
  $best = $null
  do {
    $body = @{
      bucketId     = $bucketId
      prefix       = "$prefix/"
      maxFileCount = 1000
    }
    if ($start) { $body["startFileName"] = $start }
    $listed = Invoke-RestMethod -Uri "$($auth.apiUrl)/b2api/v2/b2_list_file_names" `
      -Headers @{ Authorization = $auth.authorizationToken } `
      -Method Post -ContentType "application/json" `
      -Body ($body | ConvertTo-Json)
    foreach ($f in @($listed.files)) {
      if ($f.action -ne "upload") { continue }
      $bn = [System.IO.Path]::GetFileName($f.fileName)
      if ($bn -notlike "netquasar-full-*.pgdump") { continue }
      if (-not $best -or $f.uploadTimestamp -gt $best.uploadTimestamp) {
        $best = $f
      }
    }
    $start = $listed.nextFileName
  } while ($start)

  if (-not $best) {
    throw "Nenhum netquasar-full-*.pgdump encontrado em $prefix/ no bucket $bucket."
  }
  $remoteName = $best.fileName
  $fileId = $best.fileId
  $contentLength = [int64]$best.contentLength
}

Write-Host "Objecto B2:" $remoteName
$localName = [System.IO.Path]::GetFileName($remoteName)
$outFile = Join-Path $outDir ("from-b2-" + $localName)
Write-Host "Destino local:" $outFile

# Download autenticado (bucket private)
$encodedPath = ($remoteName -split "/" | ForEach-Object { [Uri]::EscapeDataString($_) }) -join "/"
$url = "$($auth.downloadUrl)/file/$([Uri]::EscapeDataString($bucket))/$encodedPath"

$req = [System.Net.HttpWebRequest]::Create($url)
$req.Method = "GET"
$req.Headers["Authorization"] = $auth.authorizationToken
$req.Timeout = 600000
$req.ReadWriteTimeout = 600000

$resp = $null
try {
  $resp = $req.GetResponse()
  $inStream = $resp.GetResponseStream()
  $outStream = [System.IO.File]::Create($outFile)
  try {
    $buffer = New-Object byte[] (1024 * 1024)
    $total = [int64]0
    while (($read = $inStream.Read($buffer, 0, $buffer.Length)) -gt 0) {
      $outStream.Write($buffer, 0, $read)
      $total += $read
    }
  } finally {
    $outStream.Close()
    $inStream.Close()
  }
} catch [System.Net.WebException] {
  $errResp = $_.Exception.Response
  if ($errResp) {
    $reader = New-Object System.IO.StreamReader($errResp.GetResponseStream())
    $errBody = $reader.ReadToEnd()
    $reader.Close()
    throw "Download B2 falhou: $errBody"
  }
  throw
} finally {
  if ($resp) { $resp.Close() }
}

$info = Get-Item -LiteralPath $outFile
if ($contentLength -gt 0 -and $info.Length -ne $contentLength) {
  throw ("Tamanho divergente: B2={0} local={1}" -f $contentLength, $info.Length)
}

Write-Host ("OK: download concluido ({0:N1} MB)" -f ($info.Length / 1MB))
if ($fileId) { Write-Host "fileId:" $fileId }
Write-Host "Proximo: .\08-restore-full-local.ps1 -DumpFile `"$outFile`""
