# OpenClaw Go Port

OpenClaw is now fully ported to Go in this repository.

Current release: `v2.10.2-go`

## Status

- Go runtime parity program: complete.
- Rust/Go RPC contract parity: `133/133` methods.
- CP gate suite: `CP0` through `CP9` passing.
- Cross-platform artifacts published for Windows, Linux, macOS, and Android arm64.
- TTS parity depth expanded (`v2.8.0-go`) with provider-aware cataloging, `kittentts` runtime adapter support, and Telegram `/tts` metadata parity.
- Telegram runtime upgraded to real Bot API delivery + inbound long-poll auto-reply (`v2.9.0-go`).
- Telegram delivery hardening added (`v2.10.0-go`):
  - long replies are auto-chunked to Telegram-safe message sizes.
  - group-style commands with bot suffix (`/cmd@bot`) are fully supported.
  - `/start` and `/help` command responses are now first-class.
- Multi-channel adapter breadth expanded in Go runtime (`v2.6.x-go` scope):
  - `telegram`, `whatsapp`, `discord`, `slack`, `feishu`, `qq`, `wework`, `dingtalk`, `infoflow`, `googlechat`, `teams`, `matrix`, `signal`, `line`, `mattermost`, `imessage`, plus `webchat` and `cli`.

Parity evidence:
- `parity/generated/parity-scoreboard.md`
- `parity/generated/method-surface-diff.json`
- `parity/generated/cp8/cp8-gate-summary.md`
- `parity/generated/cp9/cp9-gate-summary.md`

## Rust Code Requirement

Short answer: for running and releasing the Go port, Rust is no longer required.

- Required for production/runtime: No.
- Required for Go builds/tests/releases: No.
- Rust runtime/code status on `main`: removed.
- Archive reference for full Rust-era tree: branch/tag `rust-archive-pre-go-only-v2.6.0-go`.

## Quick Start

### 1) Configure

From repo root:

```powershell
Copy-Item openclaw-go.example.toml openclaw-go.toml
```

### 2) Run from source (Go)

```powershell
Set-Location go-agent
go run ./cmd/openclaw-go --config ../openclaw-go.toml
```

### 3) Health check

```powershell
Invoke-WebRequest -UseBasicParsing http://127.0.0.1:8080/health
```

## CLI Diagnostics

From `go-agent`:

```powershell
go run ./cmd/openclaw-go -doctor -config ../openclaw-go.toml
go run ./cmd/openclaw-go -security-audit -config ../openclaw-go.toml
go run ./cmd/openclaw-go -security-audit -fix -config ../openclaw-go.toml
go run ./cmd/openclaw-go -list-methods
```

Deep probes:

```powershell
go run ./cmd/openclaw-go -doctor -deep -config ../openclaw-go.toml
go run ./cmd/openclaw-go -security-audit -deep -config ../openclaw-go.toml
```

## Dockerized Validation

If Go is not installed locally, run tests with Docker:

```powershell
docker run --rm -v "${PWD}/go-agent:/work" -w /work golang:1.25 sh -lc "export PATH=/usr/local/go/bin:$PATH; go test ./... && go vet ./..."
```

## Docker Deployment Profiles

From repo root:

1. Bootstrap env/config:

```powershell
./prepare-env.ps1
```

```bash
sh ./prepare-env.sh
```

2. Core gateway (HTTP + RPC + channels):

```bash
docker compose up -d --build
```

3. Gateway + browser auth bridge (Playwright container):

```bash
docker compose -f docker-compose.yml -f docker-compose.bridge.yml up -d --build
```

Compose files:
- `docker-compose.yml`: Go runtime service.
- `docker-compose.bridge.yml`: ChatGPT browser bridge service + endpoint wiring.

Optional Lightpanda backend (CDP) for browser bridge:

```powershell
$env:OPENCLAW_CHATGPT_LIGHTPANDA_WS_ENDPOINT="ws://127.0.0.1:9222"
$env:OPENCLAW_CHATGPT_BRIDGE_ENGINES="lightpanda-playwright,lightpanda-puppeteer,playwright,puppeteer"
```

The bridge scripts also support direct auth helper execution:

```powershell
node scripts/chatgpt-browser-auth.mjs --engine lightpanda --lightpanda-endpoint ws://127.0.0.1:9222
```

Lightpanda is wired as an optional backend with automatic fallback to Playwright/Puppeteer.

## Build Release Artifacts

### PowerShell

```powershell
Set-Location go-agent
./scripts/build-matrix.ps1 -Version 2.10.2 -OutputDir ../dist/release-v2.10.2-go-assets
```

### POSIX shell

```bash
cd go-agent
sh ./scripts/build-matrix.sh 2.10.2 ../dist/release-v2.10.2-go-assets
```

Outputs:
- `openclaw-go-windows-amd64.exe`
- `openclaw-go-windows-arm64.exe`
- `openclaw-go-linux-amd64`
- `openclaw-go-linux-arm64`
- `openclaw-go-darwin-amd64`
- `openclaw-go-darwin-arm64`
- `openclaw-go-android-arm64`
- `SHA256SUMS.txt`

Build defaults:
- `CGO_ENABLED=0`
- stripped binaries (`-ldflags "-s -w"`)

## Release Process (GitHub)

```powershell
git push origin main
git tag v2.10.2-go
git push origin v2.10.2-go
gh release create v2.10.2-go dist/release-v2.10.2-go-assets/openclaw-go-windows-amd64.exe dist/release-v2.10.2-go-assets/openclaw-go-windows-arm64.exe dist/release-v2.10.2-go-assets/openclaw-go-linux-amd64 dist/release-v2.10.2-go-assets/openclaw-go-linux-arm64 dist/release-v2.10.2-go-assets/openclaw-go-darwin-amd64 dist/release-v2.10.2-go-assets/openclaw-go-darwin-arm64 dist/release-v2.10.2-go-assets/openclaw-go-android-arm64 dist/release-v2.10.2-go-assets/SHA256SUMS.txt -R adybag14-cyber/openclaw-go-port --title "OpenClaw Go v2.10.2" --notes-file docs/go-port/release-v2.10.2-go.md
```

## Telegram and Auth Flows

Go port includes provider-aware Telegram command handling and browser-session auth flow support.

Main command families:
- `/model ...`
- `/auth start|wait|complete|providers|status|bridge`
- `/tts status|providers|provider|on|off|speak`

Browser session bridge and provider alias handling (including Copaw/Qwen aliases) are implemented in Go runtime surfaces.
Bridge engine selection now supports `lightpanda-playwright` and `lightpanda-puppeteer` through
`OPENCLAW_CHATGPT_LIGHTPANDA_WS_ENDPOINT` + `OPENCLAW_CHATGPT_BRIDGE_ENGINES`.

## Multi-Channel Adapters

Channel adapters are configured in `openclaw-go.toml` under `[channels.<name>]`.

Each adapter accepts:
- `enabled` (`true|false`)
- `token`
- `default_target`
- `webhook_url`
- `auth_header`
- `auth_prefix`
- `headers` (optional map)

Behavior:
- If `webhook_url` is set, outbound `send` posts JSON to that endpoint.
- If no webhook is set and `token` is present, runtime treats it as token-ready delivery mode.
- If `enabled=false`, send is rejected for that channel.

Minimal example:

```toml
[channels.slack]
enabled = true
token = "xoxb-..."
default_target = "C123456"
webhook_url = "https://example.internal/slack/send"
auth_header = "Authorization"
auth_prefix = "Bearer"
```

## Repository Layout

- `go-agent/`: Go runtime, gateway, security, tool runtime, integrations.
- `docs/go-port/`: phase plans, logs, and release notes.
- `parity/`: parity harness, CP gates, generated scoreboards/reports.
- `dist/`: built artifacts (not usually committed).
- `wit/`: interface definitions used by wasm/runtime surfaces.

## Additional Docs

- Go release notes: `docs/go-port/release-v2.10.2-go.md`
- Port plan: `docs/GO_PORT_PLAN.md`
- Phase checklist: `docs/go-port/phase-checklist.md`
- Go changelog entries: `CHANGELOG.md`
