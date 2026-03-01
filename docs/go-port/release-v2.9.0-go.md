# OpenClaw Go v2.9.0 Release Notes

Date: `2026-03-01`  
Tag: `v2.9.0-go`

## Summary

`v2.9.0-go` fixes real-world Telegram runtime behavior by replacing the prior stub with real Bot API delivery and adding inbound polling/reply processing in the Go gateway.

## Highlights

1. Real Telegram Bot API delivery
- `channels.telegram` now uses live Telegram API `sendMessage` calls.
- Delivery metadata includes telegram `messageId` and chat fields.
- Channel status reflects connectivity and latest API errors.
- Supports API base override for test/proxy environments:
  - `OPENCLAW_GO_TELEGRAM_API_BASE`

2. Telegram inbound runtime loop
- Go gateway now starts a background Telegram polling runtime when `channels.telegram.bot_token` is configured.
- Inbound slash commands are handled end-to-end:
  - `/model`, `/auth`, `/tts`, `/set`
  - command responses are sent back to the originating Telegram chat.
- Inbound plain-text messages are bridged to browser-completion runtime and assistant replies are sent back to Telegram.

3. Multi-channel stability
- Existing webhook/token-ready channel adapters remain intact for non-Telegram channels:
  - `whatsapp`, `discord`, `slack`, `feishu`, `qq`, `wework`, `dingtalk`, `infoflow`, `googlechat`, `teams`, `matrix`, `signal`, `line`, `mattermost`, `imessage`.

## Validation

- Dockerized formatting + test/vet:
  - `gofmt -w ./internal/channels/telegram_driver.go ./internal/channels/registry_test.go ./internal/gateway/server.go ./internal/gateway/server_test.go ./internal/gateway/telegram_runtime.go ./internal/gateway/telegram_runtime_test.go`
  - `go test ./...`
  - `go vet ./...`
- Release build matrix:
  - `go-agent/scripts/build-matrix.sh 2.9.0 ../dist/release-v2.9.0-go-assets`

## Assets

Release assets in `dist/release-v2.9.0-go-assets`:
- `openclaw-go-windows-amd64.exe`
- `openclaw-go-linux-amd64`
- `openclaw-go-android-arm64`
- `SHA256SUMS.txt`
