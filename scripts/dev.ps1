. (Join-Path $PSScriptRoot 'common.ps1')
& (Join-Path $PSScriptRoot 'build.ps1')
$exe = Join-Path $ProjectRoot 'dist\HomeClipboardCanvas.exe'
Write-Host 'Open http://127.0.0.1:8080. Ctrl+C stops the server.'
Invoke-Checked $exe @('serve', '--dev-http', '--bind', '127.0.0.1')
