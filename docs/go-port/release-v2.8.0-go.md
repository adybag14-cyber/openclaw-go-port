# OpenClaw Go v2.8.0 Release Notes

Date: `2026-03-01`  
Tag: `v2.8.0-go`

## Summary

`v2.8.0-go` deepens Go runtime TTS parity by adding provider-aware runtime behavior, `kittentts` adapter execution support, and Telegram `/tts` command contract alignment.

## Highlights

1. Compat TTS runtime depth
- `tts.providers` now returns a unified provider catalog with availability and reason metadata.
- Added `kittentts` provider detection and execution adapter path:
  - binary discovery from `OPENCLAW_GO_KITTENTTS_BIN`, `OPENCLAW_GO_TTS_KITTENTTS_BIN`, or `PATH`.
  - optional args support via `OPENCLAW_GO_KITTENTTS_ARGS`.
  - timeout control via `OPENCLAW_GO_KITTENTTS_TIMEOUT_MS`.
- `tts.convert` now supports:
  - `requireRealAudio` / `require_real_audio` enforcement.
  - richer result contract (`outputFormat`, `realAudio`, `fallback`, `audioBase64`, `synthesisError`, debug metadata).
  - deterministic synthetic WAV fallback when real synthesis is unavailable and strict mode is not requested.

2. Telegram command parity alignment
- `/tts providers` now uses the same provider catalog as compat RPC.
- `/tts status|on|off|provider` now returns availability/reason metadata.
- `/tts say` now includes runtime synthesis metadata (`realAudio`, `outputFormat`, `fallback`, `engine`).

3. Regression coverage
- Added `compat_tts_test.go` to validate:
  - provider catalog includes core providers.
  - fallback behavior for unavailable providers.
  - strict real-audio failure path for unavailable `kittentts`.
  - command-adapter success path for `kittentts`.
- Extended gateway Telegram integration assertions for TTS metadata contract depth.

## Validation

- Dockerized formatting + validation:
  - `gofmt -w ./internal/gateway/compat.go ./internal/gateway/telegram_commands.go ./internal/gateway/server_test.go ./internal/gateway/compat_tts_test.go`
  - `go test ./...`
  - `go vet ./...`
- Release build matrix:
  - `go-agent/scripts/build-matrix.sh 2.8.0 ../dist/release-v2.8.0-go-assets`

## Assets

Release assets in `dist/release-v2.8.0-go-assets`:
- `openclaw-go-windows-amd64.exe`
- `openclaw-go-linux-amd64`
- `openclaw-go-android-arm64`
- `SHA256SUMS.txt`
