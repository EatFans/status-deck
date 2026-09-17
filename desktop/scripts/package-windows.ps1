$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$scriptDir = Split-Path -Parent $PSCommandPath
$desktopDir = Split-Path -Parent $scriptDir
$distDir = Join-Path $desktopDir "dist"
$exePath = Join-Path $distDir "Status Deck.exe"
$zipPath = Join-Path $distDir "Status-Deck-windows.zip"
$iconPath = Join-Path $distDir "StatusDeck.ico"
$resourcePath = Join-Path $desktopDir "cmd\status-deck\status-deck_windows_amd64.syso"

if ((& go env GOARCH).Trim() -ne "amd64") {
    throw "package-windows.ps1 currently packages Windows amd64 only."
}

New-Item -ItemType Directory -Force -Path $distDir | Out-Null
Push-Location $desktopDir
try {
    go run ./cmd/icon-gen --ico $iconPath
    go run github.com/akavel/rsrc@v0.10.2 -arch amd64 -ico $iconPath -o $resourcePath
    go build -trimpath -ldflags "-s -w -H=windowsgui" -o $exePath ./cmd/status-deck
}
finally {
    Remove-Item -Force -ErrorAction SilentlyContinue $resourcePath
    Pop-Location
}

Compress-Archive -Path $exePath -DestinationPath $zipPath -Force
Write-Output "Built $exePath"
Write-Output "Built $zipPath"
