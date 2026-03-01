# OpenClaw Go v2.10.0 Release Notes

Date: `2026-03-01`  
Tag: `v2.10.0-go`

## Summary

`v2.10.0-go` hardens Telegram reply reliability and broadens the release artifact matrix so operators can deploy consistently across more host platforms.

## Highlights

1. Telegram delivery hardening
- Outbound Telegram replies are now automatically split into Telegram-safe chunks.
- Multipart delivery metadata is preserved:
  - `chunked`
  - `chunkCount`
  - `messageIds`
- Prevents reply-drop failures when assistant output exceeds Telegram message length limits.

2. Telegram command compatibility upgrades
- Group-style bot-suffixed commands now work:
  - `/model@your_bot`
  - `/auth@your_bot`
  - `/tts@your_bot`
  - `/set@your_bot`
- Added first-class `/start` and `/help` response path for smoother first interaction.

3. Cross-platform release matrix expansion
- Added new build targets:
  - `windows/arm64`
  - `linux/arm64`
  - `darwin/amd64`
  - `darwin/arm64`
- Existing targets remain:
  - `windows/amd64`
  - `linux/amd64`
  - `android/arm64`

## Validation

- Dockerized formatting + test/vet:
  - `/usr/local/go/bin/gofmt -w ./internal/channels/telegram_driver.go ./internal/channels/registry_test.go ./internal/gateway/telegram_commands.go ./internal/gateway/telegram_runtime_test.go`
  - `/usr/local/go/bin/go test ./...`
  - `/usr/local/go/bin/go vet ./...`
- Release build matrix:
  - `go-agent/scripts/build-matrix.sh 2.10.0 ../dist/release-v2.10.0-go-assets`

## Assets

Release assets in `dist/release-v2.10.0-go-assets`:
- `openclaw-go-windows-amd64.exe`
- `openclaw-go-windows-arm64.exe`
- `openclaw-go-linux-amd64`
- `openclaw-go-linux-arm64`
- `openclaw-go-darwin-amd64`
- `openclaw-go-darwin-arm64`
- `openclaw-go-android-arm64`
- `SHA256SUMS.txt`
