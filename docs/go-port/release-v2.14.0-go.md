# OpenClaw Go v2.14.0 Release Notes

## Highlights

- Refreshed full cross-port parity evidence against latest references:
  - Upstream OpenClaw `origin/main` `0954b6bf5`
  - OpenClaw Rust `b2abb0d`
  - goclaw `origin/master` `0ee0f6d`
- Confirmed method-surface parity remains complete:
  - Go vs upstream base: `100%`
  - Go vs upstream handlers: `100%`
  - Go vs Rust: `100%`
- Telegram runtime provider failover hardening:
  - when the selected provider path fails, runtime retries via latest authorized provider session.
- Browser bridge key propagation hardening:
  - provider API keys now propagate as both `apiKey` and `api_key` payload fields.
- Telegram prompt-budget reliability:
  - newest user turn is preserved under budget pressure via truncation, not dropped.

## Validation

- Dockerized formatting + validation:
  - `gofmt -w ./internal/gateway/telegram_runtime.go ./internal/gateway/telegram_runtime_test.go ./internal/gateway/server.go ./internal/tools/runtime/runtime.go ./internal/tools/runtime/runtime_test.go`
  - `go test ./...`
  - `go vet ./...`
- Race validation:
  - `go test -race ./...` attempted in `golang:1.25` container; blocked because the image lacks `gcc` (`cgo: C compiler "gcc" not found`).

## Release Artifacts

Generated under `dist/release-v2.14.0-go-assets`:

- `openclaw-go-windows-amd64.exe`
- `openclaw-go-windows-arm64.exe`
- `openclaw-go-linux-amd64`
- `openclaw-go-linux-arm64`
- `openclaw-go-darwin-amd64`
- `openclaw-go-darwin-arm64`
- `openclaw-go-android-arm64`
- `SHA256SUMS.txt`

## Deployment

- Oracle VM target migrated to `v2.14.0-go` binary and restarted service.
- Existing runtime config and Telegram credentials retained in-place.
