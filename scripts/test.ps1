param([switch]$SkipBuild)
. (Join-Path $PSScriptRoot 'common.ps1')
if (-not $SkipBuild) { & (Join-Path $PSScriptRoot 'build.ps1') }
$previousBrowsers = $env:PLAYWRIGHT_BROWSERS_PATH
try {
  $env:PLAYWRIGHT_BROWSERS_PATH = Join-Path $ProjectRoot 'tools\browsers'
  Push-Location (Join-Path $ProjectRoot 'frontend')
  try {
    Invoke-Checked 'npx.cmd' @('playwright', 'install', 'chromium')
    Invoke-Checked 'npm.cmd' @('test')
  } finally { Pop-Location }
} finally { $env:PLAYWRIGHT_BROWSERS_PATH = $previousBrowsers }
