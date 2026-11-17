# SPDX-License-Identifier: Apache-2.0
[CmdletBinding()]
param(
  [string]$MicroSDRoot,
  [string]$TestMSI,
  [string]$InteractiveInstaller,
  [switch]$NonInteractive,
  [switch]$SkipCleanup
)

$ErrorActionPreference = 'Stop'
$kitHash = '__KIT_SHA256__'
$kitURL = '__KIT_URL__'
$resultsURL = '__RESULTS_URL__'
$kitRoot = 'C:\LanbayM0'

function Require-Administrator {
  $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
  $principal = New-Object Security.Principal.WindowsPrincipal($identity)
  if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'Este script debe ejecutarse desde PowerShell abierto como Administrador.'
  }
}
function Confirm-Action([string]$Message) {
  if ($NonInteractive) { return $false }
  return (Read-Host "$Message Escriba SI para continuar") -ceq 'SI'
}
function Read-Required([string]$Prompt, [string]$Current) {
  if ($Current) { return $Current }
  if ($NonInteractive) { return '' }
  return Read-Host $Prompt
}
function Write-EnvironmentReport {
  $appInstaller = Get-AppxPackage Microsoft.DesktopAppInstaller -ErrorAction SilentlyContinue | Select-Object -First 1
  $microSDDriveLetter = if ($MicroSDRoot) { [string]$MicroSDRoot[0] } else { $null }
  $report = [ordered]@{
    generated_at = (Get-Date).ToUniversalTime().ToString('o')
    windows = @(Get-ComputerInfo | Select-Object WindowsProductName, WindowsVersion, OsBuildNumber, OsLanguage, OsLocale)
    app_installer = $(if ($appInstaller) { $appInstaller.Version.ToString() } else { $null })
    winget = $(try { (& winget --version 2>&1 | Out-String).Trim() } catch { $_.Exception.Message })
    powershell_5 = (& powershell.exe -NoProfile -Command '$PSVersionTable.PSVersion.ToString()' 2>$null | Out-String).Trim()
    powershell_7 = $(if (Get-Command pwsh.exe -ErrorAction SilentlyContinue) { (& pwsh.exe -NoProfile -Command '$PSVersionTable.PSVersion.ToString()' 2>$null | Out-String).Trim() } else { $null })
    micro_sd_file_system = $(if ($microSDDriveLetter) { (Get-Volume -DriveLetter $microSDDriveLetter -ErrorAction SilentlyContinue | Select-Object -ExpandProperty FileSystem) } else { $null })
  }
  $report | ConvertTo-Json -Depth 6 | Set-Content -Encoding utf8 (Join-Path $kitRoot 'out\environment.json')
}

Require-Administrator
New-Item -ItemType Directory -Force -Path $kitRoot | Out-Null
$zip = Join-Path $env:TEMP 'lanbay-m0-kit.zip'
Write-Host 'Descargando el kit M0…'
Invoke-WebRequest -UseBasicParsing -Uri $kitURL -OutFile $zip
$actualHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $zip).Hash.ToLowerInvariant()
if ($actualHash -ne $kitHash) { throw "El SHA-256 del kit no coincide. Esperado: $kitHash; recibido: $actualHash. No se ejecutó ningún spike." }
Expand-Archive -LiteralPath $zip -DestinationPath $kitRoot -Force
New-Item -ItemType Directory -Force -Path (Join-Path $kitRoot 'out') | Out-Null

if ($NonInteractive) {
  & (Join-Path $kitRoot 'volumes\probe.exe') -out (Join-Path $kitRoot 'out\with-microsd.json')
} else {
  $MicroSDRoot = Read-Required 'Letra de la microSD (por ejemplo E:)' $MicroSDRoot
  $TestMSI = Read-Required 'Ruta completa del MSI de prueba (Enter para omitir)' $TestMSI
  $InteractiveInstaller = Read-Required 'Ruta completa del EXE interactivo (Enter para omitir)' $InteractiveInstaller
  Write-Host 'Ejecutando inventario de volúmenes con la microSD insertada…'
  & (Join-Path $kitRoot 'volumes\probe.exe') -out (Join-Path $kitRoot 'out\with-microsd.json')
  Read-Host 'Retire la microSD de forma segura y presione Enter'
  & (Join-Path $kitRoot 'volumes\probe.exe') -out (Join-Path $kitRoot 'out\without-microsd.json')
  Read-Host 'Reinserte la microSD y presione Enter'

  & (Join-Path $kitRoot 'winget-backend\run.ps1')
  if (Confirm-Action 'El spike de Papelera crea y borra un archivo efímero en C: y la microSD.') {
    & (Join-Path $kitRoot 'recycle-bin\run.ps1') -Roots @('C:\', $MicroSDRoot)
  } else { Write-Host 'Spike de Papelera omitido por el usuario.' }

  if (Confirm-Action 'El spike de contexto ejecutará comprobaciones sin instalar software.') {
    $register = Join-Path $kitRoot 'execution-context\register-task.ps1'
    $registerArgs = @('-RunnerPath', (Join-Path $kitRoot 'execution-context\runner.exe'), '-MicroSDRoot', ($MicroSDRoot + '\\'))
    if ($TestMSI) { $registerArgs += @('-TestMSI', $TestMSI) }
    if ($InteractiveInstaller) { $registerArgs += @('-InteractiveInstaller', $InteractiveInstaller) }
    if (Confirm-Action '¿Permite instalar el paquete de prueba de winget?') { $registerArgs += '-AllowInstall' }
    if ($TestMSI -and (Confirm-Action '¿Permite instalar el MSI de prueba?')) { $registerArgs += '-AllowMSI' }
    if ($InteractiveInstaller -and (Confirm-Action '¿Permite abrir el EXE interactivo de prueba?')) { $registerArgs += '-AllowInteractive' }
    & $register @registerArgs
    Start-ScheduledTask -TaskName Lanbay-M0-ExecutionContext
    Start-Sleep -Seconds 20
  } else { Write-Host 'Spike de contexto omitido por el usuario.' }
}

Write-EnvironmentReport
$archive = Join-Path $env:TEMP ('lanbay-m0-results-' + (Get-Date -Format 'yyyyMMddHHmmss') + '.zip')
Compress-Archive -Path (Join-Path $kitRoot 'out\*') -DestinationPath $archive -Force
Invoke-WebRequest -UseBasicParsing -Method Post -Uri $resultsURL -InFile $archive -ContentType 'application/zip' -Headers @{ 'X-Lanbay-Result-Name' = [IO.Path]::GetFileName($archive) } | Out-Null
Write-Host 'Resultados recibidos por la Mac.'
if (-not $NonInteractive -and -not $SkipCleanup -and (Confirm-Action '¿Desea ejecutar la limpieza del spike de contexto?')) {
  & (Join-Path $kitRoot 'execution-context\cleanup.ps1')
}
