# SPDX-License-Identifier: Apache-2.0
[CmdletBinding()]
param([Parameter(Mandatory)][string[]]$Roots)
$dir = Split-Path -Parent $PSCommandPath; $out = Join-Path $dir 'out'; New-Item -ItemType Directory -Force $out | Out-Null
& (Join-Path $dir 'probe.exe') -roots ($Roots -join ';') -script (Join-Path $dir 'recycle.ps1') -out (Join-Path $out 'recycle-bin.json')
