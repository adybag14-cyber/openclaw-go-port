# OpenClaw Go v2.12.0 Release Notes

## Highlights

- Fixed Telegram assistant context depth:
  - completions now receive system context + session history + current user turn.
  - avoids single-turn behavior that caused lost conversational continuity.
- Exposed Zvec + GraphLite memory context directly to agent prompts:
  - semantic recall + graph neighbor summaries are injected into Telegram assistant system context.
  - assistant receives actionable long-term memory hints per request.
- Fixed auth session propagation for Telegram browser flows:
  - scoped `/auth` login session IDs are forwarded into browser completion payloads.
  - stale scoped login IDs are auto-cleared when no longer authorized.
- Improved tool capability exposure:
  - runtime tool catalog is summarized into system context so agent responses no longer default to "no tools".
- Updated bridge prompt shaping:
  - `scripts/chatgpt-browser-bridge.mjs` now builds prompt context from the full `messages` array instead of only the last user message.
- Updated config template defaults:
  - `openclaw-go.example.toml` now defaults to persistent state path (`.openclaw-go/state/memory.json`).
  - example memory retention default set to unlimited (`memory_max_entries = 0`).

## Validation

- Node syntax check:
  - `node --check scripts/chatgpt-browser-bridge.mjs`
- Dockerized formatting + test + vet:
  - `gofmt -w ./internal/gateway/telegram_runtime.go ./internal/gateway/telegram_runtime_test.go ./internal/tools/runtime/runtime.go`
  - `go test ./...`
  - `go vet ./...`
- Release matrix build:
  - `go-agent/scripts/build-matrix.sh 2.12.0 ../dist/release-v2.12.0-go-assets`

## Release Artifacts

Generated under `dist/release-v2.12.0-go-assets`:

- `openclaw-go-windows-amd64.exe`
- `openclaw-go-windows-arm64.exe`
- `openclaw-go-linux-amd64`
- `openclaw-go-linux-arm64`
- `openclaw-go-darwin-amd64`
- `openclaw-go-darwin-arm64`
- `openclaw-go-android-arm64`
- `SHA256SUMS.txt`
