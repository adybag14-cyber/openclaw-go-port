param(
  [string]$Version = "dev",
  [string]$OutputDir = "../dist/release-$Version-go"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$cmdPath = Join-Path $root "cmd/openclaw-go"
$outPath = Resolve-Path (Join-Path $root $OutputDir) -ErrorAction SilentlyContinue
if (-not $outPath) {
  $fullOutPath = Join-Path $root $OutputDir
  New-Item -ItemType Directory -Force -Path $fullOutPath | Out-Null
} else {
  $fullOutPath = $outPath.Path
}

$targets = @(
  @{ GOOS = "windows"; GOARCH = "amd64"; Name = "openclaw-go-windows-amd64.exe" },
  @{ GOOS = "windows"; GOARCH = "arm64"; Name = "openclaw-go-windows-arm64.exe" },
  @{ GOOS = "linux"; GOARCH = "amd64"; Name = "openclaw-go-linux-amd64" },
  @{ GOOS = "linux"; GOARCH = "arm64"; Name = "openclaw-go-linux-arm64" },
  @{ GOOS = "darwin"; GOARCH = "amd64"; Name = "openclaw-go-darwin-amd64" },
  @{ GOOS = "darwin"; GOARCH = "arm64"; Name = "openclaw-go-darwin-arm64" },
  @{ GOOS = "android"; GOARCH = "arm64"; Name = "openclaw-go-android-arm64" }
)

Push-Location $root
try {
  foreach ($target in $targets) {
    $outFile = Join-Path $fullOutPath $target.Name
    Write-Host "Building $($target.GOOS)/$($target.GOARCH) -> $outFile"
    $env:CGO_ENABLED = "0"
    $env:GOOS = $target.GOOS
    $env:GOARCH = $target.GOARCH
    go build -trimpath -ldflags "-s -w" -o $outFile ./cmd/openclaw-go
  }

  $checksumsPath = Join-Path $fullOutPath "SHA256SUMS.txt"
  Remove-Item -Force $checksumsPath -ErrorAction SilentlyContinue
  Get-ChildItem -File $fullOutPath | ForEach-Object {
    $hash = (Get-FileHash -Algorithm SHA256 $_.FullName).Hash.ToLower()
    "$hash  $($_.Name)" | Add-Content -Path $checksumsPath
  }

  Write-Host "Release artifacts:"
  Get-ChildItem -File $fullOutPath | Select-Object Name, Length
}
finally {
  Pop-Location
}
