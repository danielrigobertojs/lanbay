# SPDX-License-Identifier: Apache-2.0
[CmdletBinding()]
param(
  [Parameter(Mandatory)][string]$RunnerPath,
  [Parameter(Mandatory)][string]$MicroSDRoot,
  [Parameter(Mandatory)][string]$TestMSI,
  [Parameter(Mandatory)][string]$InteractiveInstaller,
  [string]$WingetID = '7zip.7zip',
  [switch]$AllowInstall
)
$ErrorActionPreference = 'Stop'
$scriptDir = Split-Path -Parent $PSCommandPath
$out = Join-Path $scriptDir 'out'
New-Item -ItemType Directory -Force -Path $out | Out-Null
$arguments = @('-out', (Join-Path $out 'execution-context.json'), '-micro-sd', $MicroSDRoot,
  '-test-msi', $TestMSI, '-interactive-installer', $InteractiveInstaller, '-winget-id', $WingetID)
if ($AllowInstall) { $arguments += '-allow-install' }
$quoted = $arguments | ForEach-Object { '"' + ($_ -replace '"', '\"') + '"' }
$action = New-ScheduledTaskAction -Execute $RunnerPath -Argument ($quoted -join ' ')
$trigger = New-ScheduledTaskTrigger -AtLogOn -User "$env:USERDOMAIN\$env:USERNAME"
$principal = New-ScheduledTaskPrincipal -UserId "$env:USERDOMAIN\$env:USERNAME" -LogonType Interactive -RunLevel Highest
$settings = New-ScheduledTaskSettingsSet -Hidden -StartWhenAvailable -ExecutionTimeLimit (New-TimeSpan -Minutes 30)
Register-ScheduledTask -TaskName 'Lanbay-M0-ExecutionContext' -Action $action -Trigger $trigger -Principal $principal -Settings $settings -Force | Out-Null
Write-Host 'Tarea registrada. Iníciela una vez con Start-ScheduledTask y pegue el informe JSON.'
