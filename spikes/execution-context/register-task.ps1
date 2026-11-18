# SPDX-License-Identifier: Apache-2.0
[CmdletBinding()]
param(
  [Parameter(Mandatory)][string]$RunnerPath,
  [Parameter(Mandatory)][string]$MicroSDRoot,
  [string]$TestMSI,
  [string]$InteractiveInstaller,
  [string]$WingetID = '7zip.7zip',
  [switch]$AllowInstall,
  [switch]$AllowMSI,
  [switch]$AllowInteractive
)
$ErrorActionPreference = 'Stop'
function ConvertTo-WindowsCommandLineArgument([string]$Argument) {
  if ($Argument -notmatch '[\s"]') { return $Argument }
  $escaped = $Argument -replace '(\\*)"', '$1$1\"'
  $escaped = $escaped -replace '(\\+)$', '$1$1'
  return '"' + $escaped + '"'
}
$scriptDir = Split-Path -Parent $PSCommandPath
$out = Join-Path $scriptDir 'out'
New-Item -ItemType Directory -Force -Path $out | Out-Null
$arguments = @('-out', (Join-Path $out 'execution-context.json'), '-micro-sd', $MicroSDRoot, '-winget-id', $WingetID)
if ($TestMSI) { $arguments += @('-test-msi', $TestMSI) }
if ($InteractiveInstaller) { $arguments += @('-interactive-installer', $InteractiveInstaller) }
if ($AllowInstall) { $arguments += '-allow-install' }
if ($AllowMSI) { $arguments += '-allow-msi' }
if ($AllowInteractive) { $arguments += '-allow-interactive' }
$quoted = $arguments | ForEach-Object { ConvertTo-WindowsCommandLineArgument $_ }
$action = New-ScheduledTaskAction -Execute $RunnerPath -Argument ($quoted -join ' ')
$trigger = New-ScheduledTaskTrigger -AtLogOn -User "$env:USERDOMAIN\$env:USERNAME"
$principal = New-ScheduledTaskPrincipal -UserId "$env:USERDOMAIN\$env:USERNAME" -LogonType Interactive -RunLevel Highest
$settings = New-ScheduledTaskSettingsSet -Hidden -StartWhenAvailable -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -ExecutionTimeLimit (New-TimeSpan -Minutes 30)
Register-ScheduledTask -TaskName 'Lanbay-M0-ExecutionContext' -Action $action -Trigger $trigger -Principal $principal -Settings $settings -Force | Out-Null
Write-Host 'Tarea registrada. Iníciela una vez con Start-ScheduledTask y pegue el informe JSON.'
