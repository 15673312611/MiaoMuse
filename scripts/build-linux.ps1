param(
  [string]$OutputDir = "dist",
  [string]$BinaryName = "miaoMuse"
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$backendDir = Join-Path $root "backend"
$frontendDir = Join-Path $root "frontend"
$embedDir = Join-Path $backendDir "web\dist"
$outputDir = Join-Path $root $OutputDir

Push-Location $frontendDir
npm run build
Pop-Location

New-Item -ItemType Directory -Force -Path $embedDir | Out-Null
Remove-Item -Recurse -Force -ErrorAction SilentlyContinue (Join-Path $embedDir "*")
Copy-Item -Recurse -Force (Join-Path $frontendDir "dist\*") $embedDir

New-Item -ItemType Directory -Force -Path $outputDir | Out-Null
$env:CGO_ENABLED = "0"
$env:GOOS = "linux"
$env:GOARCH = "amd64"

Push-Location $backendDir
go build -trimpath -ldflags "-s -w" -o (Join-Path $outputDir $BinaryName)
Pop-Location

Get-ChildItem -Force $embedDir | Where-Object { $_.Name -ne "placeholder.txt" } | Remove-Item -Recurse -Force
Set-Content -Path (Join-Path $embedDir "placeholder.txt") -Value "placeholder"

Write-Host "Built Linux binary: $(Join-Path $outputDir $BinaryName)"
