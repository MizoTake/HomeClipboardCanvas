$ErrorActionPreference = 'Stop'
$ProjectRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$goCommand = Get-Command go -ErrorAction SilentlyContinue
if ($goCommand) { $GoExecutable = $goCommand.Source }
else {
  $GoExecutable = Join-Path $ProjectRoot 'tools\go\bin\go.exe'
  if (-not (Test-Path -LiteralPath $GoExecutable)) { throw 'Go is required. Install Go or unpack its official Windows ZIP into tools\go.' }
}

function Invoke-Checked {
  param([string]$Command, [string[]]$Arguments)
  & $Command @Arguments
  if ($LASTEXITCODE -ne 0) { throw "$Command failed with exit code $LASTEXITCODE" }
}

function Remove-ProjectDirectory {
  param([string]$RelativePath)
  $target = [IO.Path]::GetFullPath((Join-Path $ProjectRoot $RelativePath))
  if (-not $target.StartsWith($ProjectRoot + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) { throw 'Deletion target is outside project' }
  if (Test-Path -LiteralPath $target) {
    $item = Get-Item -LiteralPath $target -Force
    if ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Refusing to delete a linked directory' }
    Remove-Item -LiteralPath $target -Recurse -Force
  }
}
