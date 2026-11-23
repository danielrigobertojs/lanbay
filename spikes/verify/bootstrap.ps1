# SPDX-License-Identifier: Apache-2.0
[CmdletBinding()]
param(
  [string]$LanbayURL,
  [string]$Password,
  [string]$MicroSDRoot,
  [switch]$NonInteractive,
  [switch]$SkipCleanup
)

$ErrorActionPreference = 'Stop'
$kitHash = '__KIT_SHA256__'
$kitURL = '__KIT_URL__'
$resultsURL = '__RESULTS_URL__'
$kitRoot = 'C:\LanbayVerify'

function Require-Administrator {
  $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
  $principal = New-Object Security.Principal.WindowsPrincipal($identity)
  if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'Este script debe ejecutarse desde PowerShell abierto como Administrador.'
  }
}
function Write-Result([string]$Section, [string]$Name, [string]$Status, $Evidence) {
  $dir = Join-Path (Join-Path $kitRoot 'out') $Section
  New-Item -ItemType Directory -Force -Path $dir | Out-Null
  $obj = [ordered]@{
    section = $Section
    name = $Name
    status = $Status
    evidence = $Evidence
    at = (Get-Date).ToUniversalTime().ToString('o')
  }
  $obj | ConvertTo-Json -Depth 8 | Set-Content -Encoding utf8 (Join-Path $dir ($Name + '.json'))
}

Require-Administrator
New-Item -ItemType Directory -Force -Path $kitRoot | Out-Null
$zip = Join-Path $env:TEMP 'lanbay-verify-kit.zip'
Write-Host 'Descargando el kit de verificación…'
Invoke-WebRequest -UseBasicParsing -Uri $kitURL -OutFile $zip
$actualHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $zip).Hash.ToLowerInvariant()
if ($actualHash -ne $kitHash) { throw "El SHA-256 del kit no coincide. Esperado: $kitHash; recibido: $actualHash." }
Expand-Archive -LiteralPath $zip -DestinationPath $kitRoot -Force
New-Item -ItemType Directory -Force -Path (Join-Path $kitRoot 'out') | Out-Null

# Run verify.ps1
$verify = Join-Path $kitRoot 'verify.ps1'
$params = @{}
if ($LanbayURL) { $params['LanbayURL'] = $LanbayURL }
if ($Password) { $params['Password'] = $Password }
if ($MicroSDRoot) { $params['MicroSDRoot'] = $MicroSDRoot }
if ($NonInteractive) { $params['NonInteractive'] = $true }
if ($SkipCleanup) { $params['SkipCleanup'] = $true }
& $verify @params

# Package results and POST
$resultsZip = Join-Path $env:TEMP 'lanbay-verify-results.zip'
if (Test-Path $resultsZip) { Remove-Item $resultsZip -Force }
Compress-Archive -Path (Join-Path $kitRoot 'out\*') -DestinationPath $resultsZip -Force
Invoke-WebRequest -UseBasicParsing -Uri $resultsURL -Method POST -InFile $resultsZip -ContentType 'application/zip' | Out-Null
Write-Host 'Resultados enviados a la Mac.'