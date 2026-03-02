# OpenClaw Go v2.13.0 Release Notes

## Highlights

- Fixed scheduler race behavior under concurrent submit/execute load:
  - `scheduler.Submit` now returns an immutable queued snapshot.
  - race regression test added to protect submit semantics.
- Hardened provider-aware auth fallback for browser bridge requests:
  - browser requests now resolve authorized sessions by provider.
  - when `loginSessionId` is omitted, runtime auto-selects a provider-matching authorized session.
- Hardened Telegram auth recovery:
  - stale scoped auth IDs are cleared automatically.
  - Telegram reply runtime falls back to the latest authorized provider session.
- Improved resilience for long Telegram conversations:
  - completion message budget trimming preserves system + newest turns while preventing oversized prompt payloads.
- Improved persistence defaults:
  - runtime default `state_path` is now persistent (`.openclaw-go/state/memory.json`) instead of volatile memory-only storage.

## Validation

- Dockerized formatting + validation:
  - `gofmt -w ./internal/scheduler/scheduler.go ./internal/scheduler/scheduler_test.go ./internal/config/config.go ./internal/config/config_test.go ./internal/bridge/web/login.go ./internal/bridge/web/login_test.go ./internal/gateway/server.go ./internal/gateway/telegram_runtime.go ./internal/gateway/telegram_runtime_test.go`
  - `go test ./...`
  - `go vet ./...`
  - `go test -race ./...`
- Release matrix build:
  - `go-agent/scripts/build-matrix.sh 2.13.0 ../dist/release-v2.13.0-go-assets`

## Release Artifacts

Generated under `dist/release-v2.13.0-go-assets`:

- `openclaw-go-windows-amd64.exe`
- `openclaw-go-windows-arm64.exe`
- `openclaw-go-linux-amd64`
- `openclaw-go-linux-arm64`
- `openclaw-go-darwin-amd64`
- `openclaw-go-darwin-arm64`
- `openclaw-go-android-arm64`
- `SHA256SUMS.txt`
