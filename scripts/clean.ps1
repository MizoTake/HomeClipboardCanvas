. (Join-Path $PSScriptRoot 'common.ps1')
foreach ($path in @('dist', 'frontend\dist', 'backend\web\assets', 'frontend\test-results', 'frontend\playwright-report')) { Remove-ProjectDirectory $path }
New-Item -ItemType Directory -Path (Join-Path $ProjectRoot 'backend\web\assets') -Force | Out-Null
[IO.File]::WriteAllText((Join-Path $ProjectRoot 'backend\web\assets\.gitkeep'), '', [Text.UTF8Encoding]::new($false))
Write-Host 'Build and test outputs removed.'
