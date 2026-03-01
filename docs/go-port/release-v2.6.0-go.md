# OpenClaw Go v2.6.0 Release Notes

Date: 2026-03-01
Tag: `v2.6.0-go`

## Scope

This release expands Go runtime channel breadth by incorporating the multi-channel adapter pattern used in `goclaw`, while preserving OpenClaw parity contracts.

1. Added broad channel adapter support in Go runtime.
2. Added reusable channel adapter config model (token/webhook/default target).
3. Added webhook-backed delivery path for non-Telegram channels.
4. Preserved existing Telegram command/auth/model/TTS flow behavior.

## Key Changes

- Added generic channel adapter driver with:
  - disabled/token-ready/webhook delivery modes,
  - auth header/prefix support,
  - custom static headers,
  - per-channel status and logout state.
- Expanded channels registry coverage to include:
  - `whatsapp`, `discord`, `slack`, `feishu`, `qq`, `wework`, `dingtalk`, `infoflow`, `googlechat`, `teams`, `matrix`, `signal`, `line`, `mattermost`, `imessage`,
  - while keeping `telegram`, `webchat`, and `cli`.
- Added channel adapter config schema in TOML:
  - `[channels.<name>] enabled, token, default_target, webhook_url, auth_header, auth_prefix, headers`.
- Added validation rule:
  - enabled adapters require either `token` or `webhook_url`.
- Added/updated tests:
  - broad channel catalog coverage,
  - webhook delivery mode,
  - token-ready delivery mode,
  - disabled-channel rejection,
  - channel adapter config validation.
- Updated docs/examples:
  - root README,
  - `go-agent/README.md`,
  - `openclaw-go.example.toml`.

## Validation

Executed with Dockerized Go toolchain (`golang:1.25`):

- `gofmt` on changed files (pass)
- `go test ./...` (pass)
- `go vet ./...` (pass)
- Dockerized release build:
  - `go-agent/scripts/build-matrix.sh 2.6.0 ../dist/release-v2.6.0-go-assets`

## Artifacts

Release assets in `dist/release-v2.6.0-go-assets`:

- `openclaw-go-windows-amd64.exe`
- `openclaw-go-linux-amd64`
- `openclaw-go-android-arm64`
- `SHA256SUMS.txt`

Checksums:

- `c8c757c39fe7fe905961e87ce164a2feaba75ef20f114ef5444b776e2f08bf42  openclaw-go-windows-amd64.exe`
- `8e588da32bef4fd4698f79d34d039f27531ed828340a847f9097b87728c798d7  openclaw-go-linux-amd64`
- `6103369e66b1fc1a09f332455bd2d939f59f97311181ed4a5e143b903efadd44  openclaw-go-android-arm64`
