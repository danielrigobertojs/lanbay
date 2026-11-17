# SPDX-License-Identifier: Apache-2.0
[CmdletBinding()]
param([Parameter(Mandatory)][string]$Query, [int]$Iterations = 3, [Parameter(Mandatory)][string]$Output)
$ErrorActionPreference = 'Stop'
$report = [ordered]@{ engine = $PSVersionTable.PSVersion.ToString(); edition = $PSVersionTable.PSEdition; culture = [cultureinfo]::CurrentCulture.Name; module = $null; runs = @(); error = $null }
try {
  $module = Get-Module -ListAvailable Microsoft.WinGet.Client | Sort-Object Version -Descending | Select-Object -First 1
  if (-not $module) { throw 'Microsoft.WinGet.Client no está instalado.' }
  Import-Module $module.Path -ErrorAction Stop
  $report.module = @{ version = $module.Version.ToString(); path = $module.Path }
  1..$Iterations | ForEach-Object {
    $sw = [Diagnostics.Stopwatch]::StartNew()
    $value = Find-WinGetPackage -Query $Query -Source winget -ErrorAction Stop
    $sw.Stop()
    $report.runs += @{ milliseconds = $sw.ElapsedMilliseconds; result = @($value | Select-Object -First 10 | ConvertTo-Json -Depth 6 | ConvertFrom-Json) }
  }
} catch { $report.error = $_.Exception.ToString() }
$report | ConvertTo-Json -Depth 12 | Set-Content -Encoding utf8 $Output
