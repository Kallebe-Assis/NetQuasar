#Requires -Version 5.1
<#
.SYNOPSIS
  Envia um dump .pgdump para o bucket Backblaze B2.
  Preferência: AWS CLI (S3-compatible). Fallback: API nativa B2 (PowerShell).
.PARAMETER DumpFile
  Ficheiro a enviar. Default: netquasar-full-*.pgdump mais recente em out/.
#>
param(
  [string]$DumpFile = ""
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "_lib.ps1")
Import-OneShotEnv

$outDir = Get-OneShotOutDir

$keyId = Require-Env "B2_KEY_ID"
$appKey = Require-Env "B2_APPLICATION_KEY"
$bucket = Require-Env "B2_BUCKET"
$endpoint = if ($env:B2_ENDPOINT) { $env:B2_ENDPOINT.Trim() } else { "" }
$region = if ($env:B2_REGION -and $env:B2_REGION.Trim() -ne "") { $env:B2_REGION.Trim() } else { "auto" }
$prefix = if ($env:B2_PREFIX -and $env:B2_PREFIX.Trim() -ne "") {
  $env:B2_PREFIX.Trim().TrimEnd("/")
} else {
  "netquasar/postgres"
}

if ([string]::IsNullOrWhiteSpace($DumpFile)) {
  $latest = Get-ChildItem -LiteralPath $outDir -Filter "netquasar-full-*.pgdump" -ErrorAction SilentlyContinue |
    Sort-Object LastWriteTime -Descending |
    Select-Object -First 1
  if (-not $latest) {
    throw "Nenhum out/netquasar-full-*.pgdump. Corra .\05-dump-local-full.ps1 primeiro."
  }
  $DumpFile = $latest.FullName
}

if (-not (Test-Path -LiteralPath $DumpFile)) {
  throw "Ficheiro não encontrado: $DumpFile"
}

$fileName = [System.IO.Path]::GetFileName($DumpFile)
$objectKey = "$prefix/$fileName"
$fileInfo = Get-Item -LiteralPath $DumpFile

Write-Host "Ficheiro:" $DumpFile
Write-Host ("Tamanho: {0:N1} MB" -f ($fileInfo.Length / 1MB))
Write-Host "Object key:" $objectKey
Write-Host "Bucket:" $bucket

function Upload-B2Native {
  param(
    [string]$KeyId,
    [string]$ApplicationKey,
    [string]$BucketName,
    [string]$FilePath,
    [string]$RemoteName
  )

  $pair = "${KeyId}:${ApplicationKey}"
  $b64 = [Convert]::ToBase64String([Text.Encoding]::ASCII.GetBytes($pair))
  $auth = Invoke-RestMethod -Uri "https://api.backblazeb2.com/b2api/v2/b2_authorize_account" `
    -Headers @{ Authorization = "Basic $b64" } -Method Get

  $bucketId = $auth.allowed.bucketId
  if (-not $bucketId) {
    $list = Invoke-RestMethod -Uri "$($auth.apiUrl)/b2api/v2/b2_list_buckets" `
      -Headers @{ Authorization = $auth.authorizationToken } `
      -Method Post -ContentType "application/json" `
      -Body (@{ accountId = $auth.accountId; bucketName = $BucketName } | ConvertTo-Json)
    $bucketId = $list.buckets[0].bucketId
  }
  if (-not $bucketId) {
    throw "Não foi possível resolver bucketId para '$BucketName'."
  }

  $fi = Get-Item -LiteralPath $FilePath
  $uploadUrlResp = Invoke-RestMethod -Uri "$($auth.apiUrl)/b2api/v2/b2_get_upload_url" `
    -Headers @{ Authorization = $auth.authorizationToken } `
    -Method Post -ContentType "application/json" `
    -Body (@{ bucketId = $bucketId } | ConvertTo-Json)

  # SHA1 do ficheiro (B2 exige no header)
  $sha1 = (Get-FileHash -LiteralPath $FilePath -Algorithm SHA1).Hash.ToLowerInvariant()
  $bytes = [System.IO.File]::ReadAllBytes($FilePath)
  $encodedName = [Uri]::EscapeDataString($RemoteName)

  $headers = @{
    Authorization                      = $uploadUrlResp.authorizationToken
    "X-Bz-File-Name"                   = $encodedName
    "Content-Type"                     = "application/octet-stream"
    "Content-Length"                   = $bytes.Length
    "X-Bz-Content-Sha1"                = $sha1
    "X-Bz-Info-src_last_modified_millis" = ([DateTimeOffset]$fi.LastWriteTimeUtc).ToUnixTimeMilliseconds().ToString()
  }

  # Upload binário via HttpWebRequest (Invoke-RestMethod com -Body byte[] pode falhar em PS 5.1)
  $req = [System.Net.HttpWebRequest]::Create($uploadUrlResp.uploadUrl)
  $req.Method = "POST"
  foreach ($k in $headers.Keys) {
    if ($k -eq "Content-Type") { $req.ContentType = $headers[$k]; continue }
    if ($k -eq "Content-Length") { $req.ContentLength = [int64]$headers[$k]; continue }
    $req.Headers[$k] = [string]$headers[$k]
  }
  $stream = $req.GetRequestStream()
  $stream.Write($bytes, 0, $bytes.Length)
  $stream.Close()
  try {
    $resp = $req.GetResponse()
    $reader = New-Object System.IO.StreamReader($resp.GetResponseStream())
    $body = $reader.ReadToEnd()
    $reader.Close()
    $resp.Close()
    return ($body | ConvertFrom-Json)
  } catch [System.Net.WebException] {
    $errResp = $_.Exception.Response
    if ($errResp) {
      $reader = New-Object System.IO.StreamReader($errResp.GetResponseStream())
      $errBody = $reader.ReadToEnd()
      $reader.Close()
      throw "Upload B2 nativo falhou: $errBody"
    }
    throw
  }
}

$awsCmd = Get-Command "aws" -ErrorAction SilentlyContinue
if ($awsCmd -and $endpoint) {
  Write-Host "Modo: AWS CLI -> $endpoint"
  $env:AWS_ACCESS_KEY_ID = $keyId
  $env:AWS_SECRET_ACCESS_KEY = $appKey
  $env:AWS_DEFAULT_REGION = $region
  $s3Uri = "s3://$bucket/$objectKey"
  & aws s3 cp $DumpFile $s3Uri --endpoint-url $endpoint --only-show-errors
  if ($LASTEXITCODE -ne 0) {
    throw "aws s3 cp falhou com codigo $LASTEXITCODE"
  }
} else {
  if (-not $awsCmd) {
    Write-Host "AWS CLI nao encontrado - a usar API nativa B2."
  }
  $result = Upload-B2Native -KeyId $keyId -ApplicationKey $appKey -BucketName $bucket `
    -FilePath $DumpFile -RemoteName $objectKey
  Write-Host "fileId:" $result.fileId
  Write-Host "sha1:" $result.contentSha1
}

Write-Host "OK: upload concluido."
Write-Host ("Confirme na consola B2 Browse Files: bucket={0} prefix={1}/" -f $bucket, $prefix)
Write-Host ("Detalhes esperados: Name={0}, Size coerente, Bucket Type=private" -f $fileName)
