# OpenClaw Go v2.0.4 Release Notes

Date: 2026-03-01
Tag: `v2.0.4-go`

## Scope

This release completes the issue-#5 parity slice:

1. WebSocket gateway parity (`/ws`) with RPC request/response loop behavior.
2. Telegram control command parity expansion for auth/model/tts operator workflows.

## Key Changes

- Added `/ws` gateway endpoint with upgrade + frame loop handling:
  - validates `type=req` request envelopes
  - routes methods through canonical dispatch
  - returns Rust-style success/error RPC envelopes
- Expanded telegram command set:
  - `/set api key <provider> <key>`
  - `/auth help`
  - `/auth providers`
  - `/auth wait [session <id>|<id>] [--timeout <seconds>]`
  - `/auth bridge`
  - `/tts providers`
  - `/tts help`
- Improved `/auth complete` behavior:
  - accepts direct code token or callback URL
  - extracts `openclaw_code|code|device_code` query params automatically
- Added compatibility state for provider API keys.
- Added integration coverage:
  - websocket RPC dispatch test
  - extended telegram command parity test matrix

## Validation

Executed with Dockerized Go toolchain:

- `/usr/local/go/bin/go mod tidy` (pass)
- `/usr/local/go/bin/gofmt -w ./cmd ./internal` (pass)
- `/usr/local/go/bin/go test ./...` (pass)
- `/usr/local/go/bin/go vet ./...` (pass)

## Artifacts

Release assets in `dist/release-v2.0.4-go-assets`:

- `openclaw-go-windows-amd64.exe`
- `openclaw-go-linux-amd64`
- `openclaw-go-android-arm64`
- `SHA256SUMS.txt`

Checksums:

- `5317b6172fac1bafd3dae54343c12553b21d51a01c141e44218df38984d6b257  openclaw-go-windows-amd64.exe`
- `c6a5654797e6c045990e796a56578cdeec970a57a0bfe31e92cb400f5db4f6b9  openclaw-go-linux-amd64`
- `081bda4f26efcc9374f995f3c2ca772f246243d7e8617bdcea6d5f459d68129d  openclaw-go-android-arm64`
