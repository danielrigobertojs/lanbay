# SPDX-License-Identifier: Apache-2.0
[CmdletBinding()]
param([string]$WingetID = '7zip.7zip')
$ErrorActionPreference = 'Stop'
Unregister-ScheduledTask -TaskName 'Lanbay-M0-ExecutionContext' -Confirm:$false -ErrorAction SilentlyContinue
& winget uninstall --id $WingetID --exact --source winget --silent --disable-interactivity
Write-Host 'Tarea eliminada. Verifique manualmente el MSI y EXE de prueba antes de borrarlos o desinstalarlos.'
