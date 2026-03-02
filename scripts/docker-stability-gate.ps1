param(
    [int]$Probes = 12,
    [int]$IntervalSeconds = 10,
    [int]$StartupTimeoutSeconds = 120,
    [int]$BridgeImagePullRetries = 3,
    [switch]$WithBridge,
    [switch]$NoCleanup
)

$ErrorActionPreference = "Stop"

if ($Probes -le 0) {
    throw "Probes must be > 0"
}
if ($IntervalSeconds -le 0) {
    throw "IntervalSeconds must be > 0"
}
if ($StartupTimeoutSeconds -le 0) {
    throw "StartupTimeoutSeconds must be > 0"
}
if ($BridgeImagePullRetries -le 0) {
    throw "BridgeImagePullRetries must be > 0"
}

function Invoke-HealthCheck {
    param(
        [Parameter(Mandatory = $true)][string]$Url,
        [Parameter(Mandatory = $true)][string]$Name
    )

    try {
        $response = Invoke-WebRequest -UseBasicParsing -Uri $Url -TimeoutSec 8
        if ($response.StatusCode -lt 200 -or $response.StatusCode -ge 300) {
            return @{
                ok      = $false
                reason  = "$Name returned HTTP $($response.StatusCode)"
            }
        }
        return @{
            ok      = $true
            reason  = "$Name returned HTTP $($response.StatusCode)"
        }
    } catch {
        return @{
            ok      = $false
            reason  = "$Name request failed: $($_.Exception.Message)"
        }
    }
}

$repoRoot = Split-Path -Parent $PSScriptRoot
Set-Location $repoRoot

$composeArgs = @("-f", "docker-compose.yml")
if ($WithBridge) {
    $composeArgs += @("-f", "docker-compose.bridge.yml")
}

Write-Host "Using compose files: $($composeArgs -join ' ')" -ForegroundColor Cyan
docker compose @composeArgs config | Out-Null

if ($WithBridge) {
    $bridgeImage = "mcr.microsoft.com/playwright:v1.52.0-noble"
    $pulled = $false
    for ($attempt = 1; $attempt -le $BridgeImagePullRetries; $attempt++) {
        Write-Host "Pulling bridge image ($attempt/$BridgeImagePullRetries): $bridgeImage" -ForegroundColor DarkGray
        docker pull $bridgeImage
        if ($LASTEXITCODE -eq 0) {
            $pulled = $true
            break
        }
        Start-Sleep -Seconds 5
    }
    if (-not $pulled) {
        throw "Unable to pull bridge image after $BridgeImagePullRetries attempts: $bridgeImage"
    }
}

docker compose @composeArgs up -d --build

$failures = @()

try {
    $warmupDeadline = (Get-Date).AddSeconds($StartupTimeoutSeconds)
    $warmupReady = $false
    while ((Get-Date) -lt $warmupDeadline) {
        $mainWarm = Invoke-HealthCheck -Url "http://127.0.0.1:8766/health" -Name "openclaw-go"
        $bridgeWarm = @{ ok = $true }
        if ($WithBridge) {
            $bridgeWarm = Invoke-HealthCheck -Url "http://127.0.0.1:43010/health" -Name "openclaw-browser-bridge"
        }
        if ($mainWarm.ok -and $bridgeWarm.ok) {
            $warmupReady = $true
            break
        }
        Start-Sleep -Seconds 2
    }
    if (-not $warmupReady) {
        throw "Warmup timeout: services did not become healthy within ${StartupTimeoutSeconds}s"
    }

    for ($i = 1; $i -le $Probes; $i++) {
        $main = Invoke-HealthCheck -Url "http://127.0.0.1:8766/health" -Name "openclaw-go"
        $bridge = @{
            ok = $true
            reason = "bridge check skipped"
        }
        if ($WithBridge) {
            $bridge = Invoke-HealthCheck -Url "http://127.0.0.1:43010/health" -Name "openclaw-browser-bridge"
        }

        $ok = $main.ok -and $bridge.ok
        $stamp = Get-Date -Format "yyyy-MM-ddTHH:mm:ssK"
        $line = "[$stamp] probe $i/$Probes openclaw-go=$($main.ok) bridge=$($bridge.ok)"
        Write-Host $line

        if (-not $ok) {
            $reason = "$line | main=[$($main.reason)] bridge=[$($bridge.reason)]"
            $failures += $reason
        }

        if ($i -lt $Probes) {
            Start-Sleep -Seconds $IntervalSeconds
        }
    }

    if ($failures.Count -gt 0) {
        Write-Host "`nStability gate failed with $($failures.Count) probe failures:" -ForegroundColor Red
        $failures | ForEach-Object { Write-Host " - $_" -ForegroundColor Red }
        exit 1
    }

    Write-Host "`nDocker stability gate passed: $Probes probes, interval ${IntervalSeconds}s, withBridge=$WithBridge" -ForegroundColor Green
    exit 0
} finally {
    if (-not $NoCleanup) {
        Write-Host "Stopping compose services..." -ForegroundColor DarkGray
        docker compose @composeArgs down --remove-orphans
    }
}
