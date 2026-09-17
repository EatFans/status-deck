$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$scriptDir = Split-Path -Parent $PSCommandPath
$desktopDir = Split-Path -Parent $scriptDir
$distDir = Join-Path $desktopDir "dist"
$exePath = Join-Path $distDir "Status Deck.exe"
$zipPath = Join-Path $distDir "Status-Deck-windows.zip"

New-Item -ItemType Directory -Force -Path $distDir | Out-Null
Push-Location $desktopDir
try {
    go build -trimpath -ldflags "-s -w -H=windowsgui" -o $exePath ./cmd/status-deck
}
finally {
    Pop-Location
}

Compress-Archive -Path $exePath -DestinationPath $zipPath -Force
Write-Output "Built $exePath"
Write-Output "Built $zipPath"
