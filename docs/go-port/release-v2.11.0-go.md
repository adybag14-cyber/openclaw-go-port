# OpenClaw Go v2.11.0 Release Notes

## Highlights

- Expanded Go memory parity depth for Zvec/GraphLite:
  - `runtime.memory_max_entries` now drives memory retention.
  - `0` or negative values enable unlimited retention mode.
  - memory stats now include retention mode metadata.
- Added live Telegram response streaming and typing indicators:
  - progressive chunk streaming for long assistant replies.
  - periodic typing status while generation/sending is in flight.
  - configurable chunk size, chunk delay, and typing pulse interval.
- Added dynamic provider model-catalog refresh:
  - OpenRouter models are fetched/merged from provider API with TTL caching.
  - OpenCode models are fetched/merged from provider API with TTL caching.
  - expanded default latest Qwen small aliases:
    - `qwen3-0.6b`
    - `qwen3-1.7b`
    - `qwen3-4b`
    - `qwen3-8b`
- Added runtime/env controls:
  - `OPENCLAW_GO_MEMORY_MAX_ENTRIES`
  - `OPENCLAW_GO_MODEL_CATALOG_REFRESH_TTL_SECONDS`
  - `OPENCLAW_GO_TELEGRAM_LIVE_STREAMING`
  - `OPENCLAW_GO_TELEGRAM_STREAM_CHUNK_CHARS`
  - `OPENCLAW_GO_TELEGRAM_STREAM_CHUNK_DELAY_MS`
  - `OPENCLAW_GO_TELEGRAM_TYPING_INDICATORS`
  - `OPENCLAW_GO_TELEGRAM_TYPING_INTERVAL_MS`

## Validation

- Dockerized formatting:
  - `gofmt -w ./internal/channels/registry.go ./internal/channels/registry_test.go ./internal/channels/telegram_driver.go ./internal/config/config.go ./internal/config/config_test.go ./internal/gateway/compat.go ./internal/gateway/provider_catalog_test.go ./internal/gateway/server.go ./internal/gateway/telegram_commands.go ./internal/gateway/telegram_runtime.go ./internal/gateway/telegram_runtime_test.go ./internal/memory/store.go ./internal/memory/store_test.go ./internal/gateway/model_catalog_dynamic.go`
- Dockerized test + vet:
  - `go test ./...`
  - `go vet ./...`
- Release matrix build:
  - `go-agent/scripts/build-matrix.sh 2.11.0 ../dist/release-v2.11.0-go-assets`

## Release Artifacts

Generated under `dist/release-v2.11.0-go-assets`:

- `openclaw-go-windows-amd64.exe`
- `openclaw-go-windows-arm64.exe`
- `openclaw-go-linux-amd64`
- `openclaw-go-linux-arm64`
- `openclaw-go-darwin-amd64`
- `openclaw-go-darwin-arm64`
- `openclaw-go-android-arm64`
- `SHA256SUMS.txt`
