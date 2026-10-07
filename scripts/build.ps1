param([string]$Version = '0.1.0')
. (Join-Path $PSScriptRoot 'common.ps1')
if ($Version -notmatch '^\d+\.\d+\.\d+([-.][a-zA-Z0-9.]+)?$') { throw 'Invalid version' }
Push-Location (Join-Path $ProjectRoot 'frontend')
try {
  Invoke-Checked 'npm.cmd' @('ci')
  Invoke-Checked 'npm.cmd' @('run', 'build')
} finally { Pop-Location }

Remove-ProjectDirectory 'backend\web\assets'
New-Item -ItemType Directory -Path (Join-Path $ProjectRoot 'backend\web\assets') -Force | Out-Null
[IO.File]::WriteAllText((Join-Path $ProjectRoot 'backend\web\assets\.gitkeep'), '', [Text.UTF8Encoding]::new($false))
Copy-Item -Path (Join-Path $ProjectRoot 'frontend\dist\*') -Destination (Join-Path $ProjectRoot 'backend\web\assets') -Recurse
New-Item -ItemType Directory -Path (Join-Path $ProjectRoot 'dist') -Force | Out-Null
Push-Location (Join-Path $ProjectRoot 'backend')
$previousCGO = $env:CGO_ENABLED
try {
  $env:CGO_ENABLED = '0'
  Invoke-Checked $GoExecutable @('mod', 'tidy')
  Invoke-Checked $GoExecutable @('test', './...')
  Invoke-Checked $GoExecutable @('vet', './...')
  Invoke-Checked $GoExecutable @('build', '-trimpath', '-ldflags', "-s -w -X main.version=$Version", '-o', (Join-Path $ProjectRoot 'dist\HomeClipboardCanvas.exe'), '.')
} finally { $env:CGO_ENABLED = $previousCGO; Pop-Location }
Write-Host 'Built dist\HomeClipboardCanvas.exe (embedded Web UI, no runtime required).'
