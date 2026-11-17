# SPDX-License-Identifier: Apache-2.0
[CmdletBinding()]
param([string]$Query = '7zip', [int]$Iterations = 3)
$ErrorActionPreference = 'Continue'
$dir = Split-Path -Parent $PSCommandPath; $out = Join-Path $dir 'out'; New-Item -ItemType Directory -Force $out | Out-Null
& powershell.exe -NoProfile -ExecutionPolicy Bypass -File (Join-Path $dir 'measure-module.ps1') -Query $Query -Iterations $Iterations -Output (Join-Path $out 'powershell-5.json')
if (Get-Command pwsh.exe -ErrorAction SilentlyContinue) { & pwsh.exe -NoProfile -File (Join-Path $dir 'measure-module.ps1') -Query $Query -Iterations $Iterations -Output (Join-Path $out 'powershell-7.json') } else { @{ error = 'pwsh.exe no disponible' } | ConvertTo-Json | Set-Content (Join-Path $out 'powershell-7.json') }
& (Join-Path $dir 'probe.exe') -query $Query -iterations $Iterations -out (Join-Path $out 'winget-backend.json')
