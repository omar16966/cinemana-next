# update-icon.ps1
# Copies the app icon from the icons/ folder to the correct build
# locations, then you rebuild with: wails build
#
# Accepted files in icons/ (first match wins):
#   icon.ico / app.ico / any *.ico  -> build\windows\icon.ico
#   icon.png / app.png / any *.png  -> build\appicon.png

$ErrorActionPreference = "Stop"

$root   = Split-Path -Parent $MyInvocation.MyCommand.Path
$icons  = Join-Path $root "icons"
$winDir = Join-Path $root "build\windows"

if (-not (Test-Path $winDir)) {
    New-Item -ItemType Directory -Path $winDir -Force | Out-Null
}

$done = $false

# --- .ico: preferred names first, then any *.ico ---
$icoCandidates = @("icon.ico", "app.ico") +
    (Get-ChildItem $icons -Filter *.ico -ErrorAction SilentlyContinue |
        ForEach-Object { $_.Name })

foreach ($name in $icoCandidates | Select-Object -Unique) {
    $src = Join-Path $icons $name
    if (Test-Path $src) {
        Copy-Item $src (Join-Path $winDir "icon.ico") -Force
        Write-Host "OK: $name -> build\windows\icon.ico"
        $done = $true
        break
    }
}

# --- .png: preferred names first, then any *.png ---
$pngCandidates = @("icon.png", "app.png") +
    (Get-ChildItem $icons -Filter *.png -ErrorAction SilentlyContinue |
        ForEach-Object { $_.Name })

foreach ($name in $pngCandidates | Select-Object -Unique) {
    $src = Join-Path $icons $name
    if (Test-Path $src) {
        Copy-Item $src (Join-Path $root "build\appicon.png") -Force
        Write-Host "OK: $name -> build\appicon.png"
        $done = $true
        break
    }
}

if (-not $done) {
    Write-Host "No icon found! Place icons\icon.ico (or icon.png) first."
    exit 1
}

Write-Host "Done - now rebuild with: wails build"
