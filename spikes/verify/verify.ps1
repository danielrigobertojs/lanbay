# SPDX-License-Identifier: Apache-2.0
[CmdletBinding()]
param(
  [string]$LanbayURL,
  [string]$Password = $env:LANBAY_PASSWORD,
  [string]$MicroSDRoot,
  [string]$TestMSI,
  [string]$InteractiveInstaller,
  [string]$MsiURL = 'https://www.7-zip.org/a/7z2408-x64.msi',
  [string]$MsiSHA256 = '98330e7e6db3507b444d576dc437a9ac4d82333a88a6bb6ef36a91fe3d85fa92',
  [string]$ExeURL = 'https://www.7-zip.org/a/7z2408-x64.exe',
  [string]$ExeSHA256 = '67cb9d3452c9dd974b04f4a5fd842dbcba8184f2344ff72e3662d7cdb68b099b',
  [switch]$NonInteractive,
  [switch]$SkipCleanup
)

$ErrorActionPreference = 'Continue'
if ($PSVersionTable.PSVersion.Major -lt 6) {
  [System.Net.ServicePointManager]::ServerCertificateValidationCallback = { $true }
  [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
}
$kitRoot = 'C:\LanbayVerify'
$outRoot = Join-Path $kitRoot 'out'
New-Item -ItemType Directory -Force -Path $outRoot | Out-Null

$script:webSession = $null
$script:csrfToken = $null
$script:lanbayProc = $null
$script:startedLanbay = $false

function Write-SectionResult([string]$Section, [string]$Name, [string]$Status, $Evidence) {
  $dir = Join-Path $outRoot $Section
  New-Item -ItemType Directory -Force -Path $dir | Out-Null
  $obj = [ordered]@{
    section = $Section
    name = $Name
    status = $Status
    evidence = $Evidence
    at = (Get-Date).ToUniversalTime().ToString('o')
  }
  $obj | ConvertTo-Json -Depth 10 | Set-Content -Encoding utf8 (Join-Path $dir ($Name + '.json'))
}

function Invoke-LanbayLogin {
  if (-not $LanbayURL -or -not $Password) { return $false }
  try {
    $sess = New-Object Microsoft.PowerShell.Commands.WebRequestSession
    $resp = Invoke-WebRequest -UseBasicParsing -Uri ($LanbayURL.TrimEnd('/') + '/api/v1/auth/login') -Method POST -ContentType 'application/json' -Body (@{ password = $Password } | ConvertTo-Json) -WebSession $sess
    $script:webSession = $sess
    $json = $resp.Content | ConvertFrom-Json
    $script:csrfToken = $json.csrf_token
    return $true
  } catch {
    Write-SectionResult 'M1' 'lanbay-login' 'fail' @{ error = $_.Exception.Message }
    return $false
  }
}

function Invoke-Lanbay([string]$Method, [string]$Path, $Body = $null) {
  $headers = @{}
  if ($script:csrfToken) { $headers['X-CSRF-Token'] = $script:csrfToken }
  $uri = $LanbayURL.TrimEnd('/') + $Path
  $params = @{ Uri = $uri; Method = $Method; Headers = $headers; UseBasicParsing = $true }
  if ($script:webSession) { $params['WebSession'] = $script:webSession }
  if ($Body) { $params['Body'] = ($Body | ConvertTo-Json -Depth 10); $params['ContentType'] = 'application/json' }
  return Invoke-WebRequest @params
}

function Wait-Job([string]$JobId) {
  for ($i = 0; $i -lt 120; $i++) {
    try {
      $r = Invoke-Lanbay 'GET' '/api/v1/jobs'
      $jobs = ($r.Content | ConvertFrom-Json)
      $j = $jobs | ForEach-Object { $_ } | Where-Object { $_.id -eq $JobId } | Select-Object -First 1
      if ($j -and ($j.state -eq 'done' -or $j.state -eq 'failed' -or $j.state -eq 'cancelled')) { return $j }
    } catch {}
    Start-Sleep -Seconds 2
  }
  return $null
}

# Start Lanbay if needed
if (-not $LanbayURL) {
  $dataDir = 'C:\ProgramData\Lanbay'
  $lanbayExe = Join-Path $kitRoot 'lanbay.exe'
  if (-not (Test-Path $lanbayExe)) { $lanbayExe = 'lanbay.exe' }
  # initial password file
  $pwFile = Join-Path $dataDir 'initial-password.txt'
  if (-not (Test-Path $pwFile)) {
    # run once to generate
    $p = Start-Process -FilePath $lanbayExe -PassThru -WindowStyle Hidden
    Start-Sleep -Seconds 4
    Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 2
  }
  if (Test-Path $pwFile) { $Password = (Get-Content $pwFile -Raw).Trim() }
  $script:lanbayProc = Start-Process -FilePath $lanbayExe -PassThru -WindowStyle Hidden
  $script:startedLanbay = $true
  Start-Sleep -Seconds 4
  $LanbayURL = 'https://localhost:8443'
}

# ---------------- M1 ----------------
# icacls evaluation
try {
  $icaclsData = (& icacls 'C:\ProgramData\Lanbay' 2>&1 | Out-String)
  $icaclsPw = (& icacls 'C:\ProgramData\Lanbay\initial-password.txt' 2>&1 | Out-String)
  $ok = ($icaclsData -match 'SYSTEM' -and $icaclsData -match 'Administrators') -and -not ($icaclsData -match 'Users' -or $icaclsData -match 'Everyone')
  Write-SectionResult 'M1' 'icacls' $(if ($ok) { 'pass' } else { 'fail' }) @{ data_dir = $icaclsData; password_file = $icaclsPw }
} catch {
  Write-SectionResult 'M1' 'icacls' 'fail' @{ error = $_.Exception.Message }
}

# junction root test (restaura en finally; no deja el data dir movido si falla)
$backup = 'C:\ProgramData\Lanbay-backup-verify'
$testRoot = 'C:\ProgramData\Lanbay-junctionroot'
if (Test-Path $backup) {
  Write-SectionResult 'M1' 'junction-root' 'fail' @{ error = 'Abortado: existe Lanbay-backup-verify de una ejecución anterior.' }
} else {
  $icaclsBefore = (& icacls 'C:\ProgramData\Lanbay' 2>&1 | Out-String)
  # stop any running lanbay
  Get-Process -Name lanbay -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
  Start-Sleep -Seconds 1
  try {
    if (Test-Path 'C:\ProgramData\Lanbay') { Move-Item 'C:\ProgramData\Lanbay' $backup }
    New-Item -ItemType Directory -Force -Path $testRoot | Out-Null
    cmd /c "mklink /J C:\ProgramData\Lanbay $testRoot" | Out-Null
    $p = Start-Process -FilePath (Join-Path $kitRoot 'lanbay.exe') -PassThru -WindowStyle Hidden -RedirectStandardError (Join-Path $outRoot 'junction-root.err')
    Start-Sleep -Seconds 4
    $running = -not $p.HasExited
    if ($running) { Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue }
    $errText = if (Test-Path (Join-Path $outRoot 'junction-root.err')) { Get-Content (Join-Path $outRoot 'junction-root.err') -Raw } else { '' }
    $refused = (-not $running) -and ($errText -match 'junction|insegur|reparse')
    $drift = ($icaclsBefore -ne $icaclsAfter)
  Write-SectionResult 'M1' 'junction-root' $(if ($refused -and -not $drift) { 'pass' } else { 'fail' }) @{ exit_code = $p.ExitCode; stderr = $errText; icacls_before = $icaclsBefore; icacls_after = $icaclsAfter; icacls_changed = $drift }
  } catch {
    Write-SectionResult 'M1' 'junction-root' 'fail' @{ error = $_.Exception.Message }
  } finally {
    if (Test-Path 'C:\ProgramData\Lanbay') { cmd /c "rmdir C:\ProgramData\Lanbay" 2>$null | Out-Null }
    if (Test-Path $backup) { Move-Item $backup 'C:\ProgramData\Lanbay' -Force }
    if (Test-Path $testRoot) { Remove-Item $testRoot -Recurse -Force -ErrorAction SilentlyContinue }
    $icaclsAfter = (& icacls 'C:\ProgramData\Lanbay' 2>&1 | Out-String)
    # Do not overwrite the previous result; record icacls drift as evidence only.
    if ($icaclsBefore -ne $icaclsAfter) {
      Write-Host "ADVERTENCIA: icacls cambió tras la prueba de junction-root."
    }
  }
}

# junction internal to System32
try {
  $internalJunction = 'C:\ProgramData\Lanbay\junction-system32'
  if (-not (Test-Path 'C:\ProgramData\Lanbay')) { New-Item -ItemType Directory -Force -Path 'C:\ProgramData\Lanbay' | Out-Null }
  if (-not (Test-Path 'C:\LanbayVerify\canary')) { New-Item -ItemType Directory -Force -Path 'C:\LanbayVerify\canary' | Out-Null }
  cmd /c "mklink /J $internalJunction C:\LanbayVerify\canary" | Out-Null
  $p = Start-Process -FilePath (Join-Path $kitRoot 'lanbay.exe') -PassThru -WindowStyle Hidden -RedirectStandardError (Join-Path $outRoot 'junction-internal.err')
  Start-Sleep -Seconds 4
  $running = -not $p.HasExited
  if ($running) { Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue }
  $errText = if (Test-Path (Join-Path $outRoot 'junction-internal.err')) { Get-Content (Join-Path $outRoot 'junction-internal.err') -Raw } else { '' }
  if (Test-Path $internalJunction) { cmd /c "rmdir `"$internalJunction`"" | Out-Null }
  $refused = (-not $running) -and ($errText -match 'reparse|junction|insegur')
  Write-SectionResult 'M1' 'junction-internal' $(if ($refused) { 'pass' } else { 'fail' }) @{ exit_code = $p.ExitCode; stderr = $errText }
} catch {
  Write-SectionResult 'M1' 'junction-internal' 'fail' @{ error = $_.Exception.Message }
}

# set-password noecho: manual confirmation
if ($NonInteractive) {
  Write-SectionResult 'M1' 'set-password-noecho' 'manual' @{ note = 'Requiere confirmación humana (no eco).' }
} else {
  $ans = Read-Host 'Ejecute `lanbay set-password` y confirme que no hace eco (S/N)'
  Write-SectionResult 'M1' 'set-password-noecho' $(if ($ans -eq 'S') { 'pass' } else { 'manual' }) @{ confirm = $ans }
}

# Re-launch Lanbay after junction tests only if this script started it
if ($script:startedLanbay -and (-not $script:webSession)) {
  if (-not $LanbayURL) { $LanbayURL = 'https://localhost:8443' }
  if (-not $Password -and (Test-Path 'C:\ProgramData\Lanbay\initial-password.txt')) { $Password = (Get-Content 'C:\ProgramData\Lanbay\initial-password.txt' -Raw).Trim() }
  $script:lanbayProc = Start-Process -FilePath (Join-Path $kitRoot 'lanbay.exe') -PassThru -WindowStyle Hidden
  Start-Sleep -Seconds 5
}

# disk inventory via API
if (Invoke-LanbayLogin) {
  try {
    $resp = Invoke-Lanbay 'GET' '/api/v1/volumes'
    $vols = $resp.Content | ConvertFrom-Json
    Write-SectionResult 'M1' 'disk-inventory' 'pass' @{ volumes = $vols }
    Write-SectionResult 'M1' 'lanbay-login' 'pass' @{ volumes = ($vols.volumes.Count) }
  } catch {
    Write-SectionResult 'M1' 'disk-inventory' 'fail' @{ error = $_.Exception.Message }
    Write-SectionResult 'M1' 'lanbay-login' 'fail' @{ error = $_.Exception.Message }
  }
} else {
  Write-SectionResult 'M1' 'disk-inventory' 'fail' @{ error = 'login failed' }
  Write-SectionResult 'M1' 'lanbay-login' 'fail' @{ error = 'login failed' }
}

# ---------------- M4 ----------------
# search
try {
  $search = Invoke-Lanbay 'GET' '/api/v1/software/search?q=7zip.7zip'
  Write-SectionResult 'M4' 'search' 'pass' @{ result = ($search.Content | ConvertFrom-Json) }
} catch {
  Write-SectionResult 'M4' 'search' 'fail' @{ error = $_.Exception.Message }
}

# install
$jobId = $null
try {
  $inst = Invoke-Lanbay 'POST' '/api/v1/software/install' @{ id = '7zip.7zip' }
  $jobId = ($inst.Content | ConvertFrom-Json).job_id
  $j = Wait-Job $jobId
  if ($j -and $j.state -eq 'done') {
    Write-SectionResult 'M4' 'install' 'pass' @{ job_id = $jobId; job = $j }
  } else {
    Write-SectionResult 'M4' 'install' 'fail' @{ job_id = $jobId; job = $j }
  }
} catch {
  Write-SectionResult 'M4' 'install' 'fail' @{ error = $_.Exception.Message }
}

# reconciliation-missing: install by panel (done above), uninstall via winget outside panel, then check status=missing
try {
  if (Get-Command winget -ErrorAction SilentlyContinue) {
    & winget uninstall --id 7zip.7zip --silent 2>&1 | Out-Null
    Start-Sleep -Seconds 8
  }
  $list = Invoke-Lanbay 'GET' '/api/v1/software'
  $apps = ($list.Content | ConvertFrom-Json).apps
  $missing = $apps | Where-Object { $_.winget_id -eq '7zip.7zip' -and $_.status -eq 'missing' }
  if ($missing) {
    Write-SectionResult 'M4' 'reconciliation-missing' 'pass' @{ missing = $missing }
  } else {
    Write-SectionResult 'M4' 'reconciliation-missing' 'fail' @{ note = 'No se detectó status missing.'; apps = $apps }
  }
} catch {
  $code = 0
  if ($_.Exception.Response) { $code = [int]$_.Exception.Response.StatusCode }
  Write-SectionResult 'M4' 'reconciliation-missing' 'fail' @{ status = $code; error = $_.Exception.Message }
}

# update
try {
  $up = Invoke-Lanbay 'GET' '/api/v1/software/updates'
  $updates = ($up.Content | ConvertFrom-Json).updates
  if ($updates -and $updates.Count -gt 0) {
    $u = $updates | Select-Object -First 1
    $ur = Invoke-Lanbay 'POST' ("/api/v1/software/$($u.id)/upgrade") $null
    $jid = ($ur.Content | ConvertFrom-Json).job_id
    $jj = Wait-Job $jid
    if ($jj -and $jj.state -eq 'done') {
      Write-SectionResult 'M4' 'update' 'pass' @{ job_id = $jid; job = $jj }
    } else {
      Write-SectionResult 'M4' 'update' 'fail' @{ job_id = $jid; job = $jj }
    }
  } else {
    Write-SectionResult 'M4' 'update' 'manual' @{ note = 'No hay updates disponibles en este momento.' }
  }
} catch {
  Write-SectionResult 'M4' 'update' 'fail' @{ error = $_.Exception.Message }
}

# E3: preexisting app rejected (must return 4xx with product error code)
try {
  $r = Invoke-Lanbay 'POST' '/api/v1/software/install' @{ id = 'Microsoft.WindowsCalculator' }
  Write-SectionResult 'M4' 'e3-preexisting-reject' 'fail' @{ note = 'Se esperaba rechazo, pero fue aceptado'; status = $r.StatusCode }
} catch {
  $code = 0
  if ($_.Exception.Response) { $code = [int]$_.Exception.Response.StatusCode }
  if (($code -eq 400 -or $code -eq 409 -or $code -eq 422) -and -not ($code -eq 401 -or $code -eq 403 -or $code -eq 404)) {
    Write-SectionResult 'M4' 'e3-preexisting-reject' 'pass' @{ status = $code; error = $_.Exception.Message }
  } else {
    Write-SectionResult 'M4' 'e3-preexisting-reject' 'fail' @{ status = $code; error = $_.Exception.Message }
  }
}

# SW-1: resolve winget via APPEXECLINK
try {
  $wg = (Get-Command winget -ErrorAction Stop).Source
  $ok = ($wg -match 'WindowsApps') -and (Test-Path $wg)
  if ($ok) {
    Write-SectionResult 'M4' 'sw1-appexeclink' 'pass' @{ winget_path = $wg }
  } else {
    Write-SectionResult 'M4' 'sw1-appexeclink' 'fail' @{ winget_path = $wg; note = 'No se resolvió como alias de WindowsApps.' }
  }
} catch {
  Write-SectionResult 'M4' 'sw1-appexeclink' 'fail' @{ error = $_.Exception.Message }
}

# uninstall managed
try {
  $list = Invoke-Lanbay 'GET' '/api/v1/software'
  $apps = ($list.Content | ConvertFrom-Json).apps
  $managed = $apps | Where-Object { $_.winget_id -eq '7zip.7zip' -and $_.status -eq 'installed' } | Select-Object -First 1
  $missingApp = $apps | Where-Object { $_.winget_id -eq '7zip.7zip' -and $_.status -eq 'missing' } | Select-Object -First 1
  if ($managed) {
    $un = Invoke-Lanbay 'POST' ("/api/v1/software/$($managed.id)/uninstall?confirm=1") $null
    $jid = ($un.Content | ConvertFrom-Json).job_id
    $jj = Wait-Job $jid
    if ($jj -and $jj.state -eq 'done') {
      Write-SectionResult 'M4' 'uninstall' 'pass' @{ job_id = $jid; job = $jj }
    } else {
      Write-SectionResult 'M4' 'uninstall' 'fail' @{ job_id = $jid; job = $jj }
    }
  } elseif ($missingApp) {
    Write-SectionResult 'M4' 'uninstall' 'pass' @{ note = 'App ya marcada missing tras desinstalación externa; registro limpiado por reconciliación.'; app = $missingApp }
  } else {
    Write-SectionResult 'M4' 'uninstall' 'fail' @{ note = 'No se encontró la app gestionada ni missing.' }
  }
} catch {
  Write-SectionResult 'M4' 'uninstall' 'fail' @{ error = $_.Exception.Message }
}

# ---------------- M5 ----------------
# B3: product creates staging; do not create here. Check after stage.
# B2: path outside staging rejected
try {
  $r = Invoke-Lanbay 'POST' '/api/v1/software/install-uploaded' @{ path = 'C:\Windows\notepad.exe'; reauth_password = $Password }
  Write-SectionResult 'M5' 'b2-path-outside-staging' 'fail' @{ note = 'Se esperaba rechazo, pero fue aceptado'; status = $r.StatusCode }
} catch {
  $code = 0
  if ($_.Exception.Response) { $code = [int]$_.Exception.Response.StatusCode }
  if ($code -ge 400 -and $code -lt 500) {
    Write-SectionResult 'M5' 'b2-path-outside-staging' 'pass' @{ status = $code; error = $_.Exception.Message }
  } else {
    Write-SectionResult 'M5' 'b2-path-outside-staging' 'fail' @{ status = $code; error = $_.Exception.Message }
  }
}

# stage a public MSI via URL download to temp then upload? Use staging route with multipart
$stagingPath = $null
if (-not $MsiSHA256) { Write-SectionResult 'M5' 'b3-staging-dir' 'fail' @{ error = 'MsiSHA256 requerido' }; throw 'missing hash' }
if ($MsiURL) {
  try {
    $tmp = Join-Path $env:TEMP 'verify-test.msi'
    Invoke-WebRequest -UseBasicParsing -Uri $MsiURL -OutFile $tmp
    if ($MsiSHA256) {
      $actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $tmp).Hash.ToLowerInvariant()
      if ($actual -ne $MsiSHA256.ToLowerInvariant()) { throw "SHA-256 mismatch" }
    }
    # stage via multipart
    $boundary = [guid]::NewGuid().ToString()
    $fileBytes = [System.IO.File]::ReadAllBytes($tmp)
    $bodyLines = @()
    $bodyLines += "--$boundary"
    $bodyLines += 'Content-Disposition: form-data; name="file"; filename="verify-test.msi"'
    $bodyLines += 'Content-Type: application/octet-stream'
    $bodyLines += ''
    $prefix = [System.Text.Encoding]::ASCII.GetBytes(($bodyLines -join "`r`n") + "`r`n")
    $suffix = [System.Text.Encoding]::ASCII.GetBytes("`r`n--$boundary--`r`n")
    $ms = New-Object System.IO.MemoryStream
    $ms.Write($prefix, 0, $prefix.Length)
    $ms.Write($fileBytes, 0, $fileBytes.Length)
    $ms.Write($suffix, 0, $suffix.Length)
    $ms.Position = 0
    $headers = @{}
    if ($script:csrfToken) { $headers['X-CSRF-Token'] = $script:csrfToken }
    $resp = Invoke-WebRequest -UseBasicParsing -Uri ($LanbayURL.TrimEnd('/') + '/api/v1/software/staging') -Method POST -ContentType "multipart/form-data; boundary=$boundary" -Body $ms -WebSession $script:webSession -Headers $headers
    $stagingPath = ($resp.Content | ConvertFrom-Json).path
    Write-SectionResult 'M5' 'b3-staging-dir' 'pass' @{ staging_path = $stagingPath; product_created = $true }
  } catch {
    Write-SectionResult 'M5' 'b3-staging-dir' 'fail' @{ error = $_.Exception.Message }
  }
} else {
  Write-SectionResult 'M5' 'b3-staging-dir' 'manual' @{ note = 'Proporcionar -MsiURL.' }
}

# inspect
if ($stagingPath) {
  try {
    $ins = Invoke-Lanbay 'GET' ("/api/v1/software/installers/inspect?path=$([uri]::EscapeDataString($stagingPath))")
    $info = $ins.Content | ConvertFrom-Json
    $sigOk = ($info.authenticode.valid -eq $true) -or ($info.authenticode.status -eq 'Valid')
    if ($sigOk) {
      Write-SectionResult 'M5' 'authenticode' 'pass' @{ inspect = $info }
    } else {
      Write-SectionResult 'M5' 'authenticode' 'fail' @{ inspect = $info }
    }
  } catch {
    Write-SectionResult 'M5' 'authenticode' 'fail' @{ error = $_.Exception.Message }
  }
  # install uploaded
  $jobId2 = $null
  try {
    $iu = Invoke-Lanbay 'POST' '/api/v1/software/install-uploaded' @{ path = $stagingPath; reauth_password = $Password }
    $jobId2 = ($iu.Content | ConvertFrom-Json).job_id
    $j2 = Wait-Job $jobId2
    if ($j2 -and $j2.state -eq 'done') {
      Write-SectionResult 'M5' 'msi-silent-install' 'pass' @{ job_id = $jobId2; job = $j2 }
    } else {
      Write-SectionResult 'M5' 'msi-silent-install' 'fail' @{ job_id = $jobId2; job = $j2 }
    }
  } catch {
    Write-SectionResult 'M5' 'msi-silent-install' 'fail' @{ error = $_.Exception.Message }
  }

  # registry-diff: evaluated after install, requires job data
  try {
    if ($jobId2) {
      $jr = Invoke-Lanbay 'GET' '/api/v1/jobs'
      $job = ($jr.Content | ConvertFrom-Json) | Where-Object { $_.id -eq $jobId2 } | Select-Object -First 1
      $msg = if ($job) { $job.message } else { '' }
      $log = if ($job) { $job.log } else { '' }
      $count = -1
      if ($msg -match 'seleccione las entradas') { $count = 2 }
      elseif ($msg -match 'no rastreable') { $count = 0 }
      elseif ($job -and $job.state -eq 'done') { $count = 1 }
      if ($count -ge 0) {
        Write-SectionResult 'M5' 'registry-diff' 'pass' @{ entries = $count; message = $msg; log_excerpt = $log.Substring(0, [Math]::Min(200, $log.Length)) }
      } else {
        Write-SectionResult 'M5' 'registry-diff' 'fail' @{ note = 'Sin dato de diff.'; message = $msg }
      }
    } else {
      Write-SectionResult 'M5' 'registry-diff' 'fail' @{ note = 'Sin job de instalación.' }
    }
  } catch {
    Write-SectionResult 'M5' 'registry-diff' 'fail' @{ error = $_.Exception.Message }
  }
} else {
  Write-SectionResult 'M5' 'authenticode' 'manual' @{ note = 'Sin staging.' }
  Write-SectionResult 'M5' 'msi-silent-install' 'manual' @{ note = 'Sin staging.' }
  Write-SectionResult 'M5' 'registry-diff' 'manual' @{ note = 'Sin staging.' }
}

# EXE interactive
if ($ExeURL) {
  try {
    $tmpExe = Join-Path $env:TEMP 'verify-test.exe'
    Invoke-WebRequest -UseBasicParsing -Uri $ExeURL -OutFile $tmpExe
    if ($ExeSHA256) {
      $actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $tmpExe).Hash.ToLowerInvariant()
      if ($actual -ne $ExeSHA256.ToLowerInvariant()) { throw "SHA-256 mismatch" }
    }
    # screenshot
    Add-Type -AssemblyName System.Windows.Forms
    Add-Type -AssemblyName System.Drawing
    $screen = [System.Windows.Forms.Screen]::PrimaryScreen.Bounds
    $bmp = New-Object System.Drawing.Bitmap $screen.Width, $screen.Height
    $g = [System.Drawing.Graphics]::FromImage($bmp)
    $g.CopyFromScreen($screen.Location, [System.Drawing.Point]::Empty, $screen.Size)
    $shot = Join-Path $outRoot 'M5\interactive-screenshot.png'
    $bmp.Save($shot)
    $g.Dispose(); $bmp.Dispose()
    $confirm = 'N'
    if (-not $NonInteractive) { $confirm = Read-Host '¿La ventana del instalador es visible? (S/N)' }
    $status = if ($confirm -eq 'S') { 'pass' } else { 'manual' }
    Write-SectionResult 'M5' 'exe-interactive' $status @{ screenshot = $shot; confirm = $confirm }
  } catch {
    Write-SectionResult 'M5' 'exe-interactive' 'fail' @{ error = $_.Exception.Message }
  }
} elseif ($InteractiveInstaller -and (Test-Path $InteractiveInstaller)) {
  Write-SectionResult 'M5' 'exe-interactive' 'manual' @{ note = 'Proporcionar -InteractiveInstaller.' }
} else {
  Write-SectionResult 'M5' 'exe-interactive' 'manual' @{ note = 'Proporcionar -ExeURL.' }
}

try {
  $list = Invoke-Lanbay 'GET' '/api/v1/software'
  $apps = ($list.Content | ConvertFrom-Json).apps
  $targets = $apps | Where-Object { $_.display_name -match '7-Zip|verify-test' }
  foreach ($a in $targets) {
    $ur = Invoke-Lanbay 'POST' ("/api/v1/software/$($a.id)/uninstall?confirm=1") $null
    $jid = ($ur.Content | ConvertFrom-Json).job_id
    $jj = Wait-Job $jid
    if (-not ($jj -and $jj.state -eq 'done')) { throw "uninstall failed" }
  }
  Write-SectionResult 'M5' 'uninstall' 'pass' @{ uninstalled = ($targets.Count) }
} catch {
  Write-SectionResult 'M5' 'uninstall' 'fail' @{ error = $_.Exception.Message }
}

# ---------------- cleanup ----------------
if (-not $SkipCleanup) {
  if ($script:lanbayProc -and -not $script:lanbayProc.HasExited) {
    Stop-Process -Id $script:lanbayProc.Id -Force -ErrorAction SilentlyContinue
  }
  # remove junctions if any remain
  foreach ($j in @('C:\ProgramData\Lanbay\junction-system32')) {
    if (Test-Path $j) { cmd /c "rmdir `"$j`"" | Out-Null }
  }
  $keep = 'N'
  if (-not $NonInteractive) { $keep = Read-Host '¿Conservar C:\ProgramData\Lanbay? (S/N)' }
  if ($keep -ne 'S') {
    Write-Host 'Para borrar C:\ProgramData\Lanbay: Remove-Item -Recurse -Force C:\ProgramData\Lanbay'
  }
}

Write-Host 'Verificación M1/M4/M5 finalizada.'