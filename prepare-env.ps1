param(
  [string]$EnvPath = ".env",
  [string]$ExamplePath = ".env.example"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

if (-not (Test-Path $ExamplePath)) {
  throw "Missing $ExamplePath"
}

if (-not (Test-Path $EnvPath)) {
  Copy-Item $ExamplePath $EnvPath
  Write-Host "Created $EnvPath from $ExamplePath"
} else {
  Write-Host "$EnvPath already exists. Updating empty required values only."
}

function New-Token([int]$bytes = 32) {
  $buffer = New-Object byte[] $bytes
  [System.Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($buffer)
  return ([Convert]::ToBase64String($buffer)).TrimEnd("=") -replace "\+","-" -replace "/","_"
}

$lines = Get-Content $EnvPath
$updated = @()
$tokenFilled = $false
foreach ($line in $lines) {
  if ($line -match '^OPENCLAW_GO_GATEWAY_TOKEN\s*=\s*$') {
    $updated += "OPENCLAW_GO_GATEWAY_TOKEN=$(New-Token 32)"
    $tokenFilled = $true
    continue
  }
  $updated += $line
}

if (-not $tokenFilled -and -not ($updated -match '^OPENCLAW_GO_GATEWAY_TOKEN=')) {
  $updated += "OPENCLAW_GO_GATEWAY_TOKEN=$(New-Token 32)"
  $tokenFilled = $true
}

Set-Content -Path $EnvPath -Value $updated

if (-not (Test-Path "openclaw-go.toml")) {
  Copy-Item "openclaw-go.example.toml" "openclaw-go.toml"
  Write-Host "Created openclaw-go.toml from openclaw-go.example.toml"
}

Write-Host ""
Write-Host "Environment bootstrap complete."
Write-Host "Next:"
Write-Host "  docker compose up -d --build"
Write-Host "  docker compose -f docker-compose.yml -f docker-compose.bridge.yml up -d"
