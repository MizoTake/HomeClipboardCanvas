. (Join-Path $PSScriptRoot 'common.ps1')
& (Join-Path $PSScriptRoot 'build.ps1')
$exe = Join-Path $ProjectRoot 'dist\HomeClipboardCanvas.exe'
$previousToken = $env:HCC_TOKEN
try {
  if (-not $env:HCC_TOKEN) {
    $env:HCC_TOKEN = (& $exe generate-token).Trim()
    if ($LASTEXITCODE -ne 0) { throw 'Token generation failed' }
    New-Item -ItemType Directory -Path (Join-Path $ProjectRoot 'tools') -Force | Out-Null
    $tokenPath = Join-Path $ProjectRoot 'tools\dev-token.txt'
    [IO.File]::WriteAllText($tokenPath, $env:HCC_TOKEN, [Text.UTF8Encoding]::new($false))
    Write-Host 'Temporary development token: tools\dev-token.txt (ignored by Git).'
  }
  Write-Host 'Open http://127.0.0.1:8080 and paste the token. Ctrl+C stops the server.'
  & $exe serve --dev-http --bind 127.0.0.1
} finally {
  $env:HCC_TOKEN = $previousToken
  if ($tokenPath -and (Test-Path -LiteralPath $tokenPath)) { Remove-Item -LiteralPath $tokenPath -Force }
}
