# Cross-Port Parity Diff (2026-03-02)

## Scope

Comparison baseline for this release cycle:

- Upstream OpenClaw: `openclaw-upstream-main` @ `origin/main` `0954b6bf5`
- OpenClaw Rust: `openclaw-rust` @ `b2abb0d`
- OpenClaw Go: `openclaw-go-port` @ `23e75bf` (pre-release state)
- goclaw reference: `goclaw-smallnest` @ `origin/master` `0ee0f6d`

## Method Surface Parity

Sources:

- `parity/generated/upstream-methods.base.latest.json`
- `parity/generated/upstream-methods.handlers.latest.json`
- `parity/generated/rust-methods.latest.json`
- `parity/generated/go-methods.latest.json`
- `parity/generated/go-method-surface-diff.latest.json`

Results:

- Upstream base methods: `89`
- Upstream handler methods: `99`
- Rust methods: `133`
- Go methods: `134`

Coverage:

- Go vs upstream base: `100%` (missing `0`)
- Go vs upstream handlers: `100%` (missing `0`)
- Go vs Rust: `100%` (missing `0`)
- Go-only method delta vs Rust: `node.canvas.capability.refresh`

## Feature-Level Diff

### Upstream OpenClaw vs Go Port

- RPC contract parity: complete on base + handler surfaces.
- Go still carries extended edge/runtime methods that are intentionally beyond upstream core.

### Rust vs Go Port

- Required method parity: complete (`133/133`).
- Go retains additional method `node.canvas.capability.refresh`.
- This release adds reliability depth on Telegram/browser bridge paths (see Improvements).

### goclaw vs Go Port

goclaw strengths reviewed:

- broad channel ecosystem + multi-adapter emphasis
- provider failover posture
- skill-heavy ecosystem packaging

Go port status against those strengths:

- channel adapter breadth: already broader than goclaw config baseline in core runtime (`telegram`, `whatsapp`, `discord`, `slack`, `feishu`, `qq`, `wework`, `dingtalk`, `infoflow`, `googlechat`, `teams`, `matrix`, `signal`, `line`, `mattermost`, `imessage`)
- provider failover depth: improved in this release for Telegram completion runtime (cross-provider fallback)
- provider key handling: improved in this release (API key propagation to bridge payload)

## Improvements Implemented For This Release

1. Telegram provider failover hardening
- If the selected provider path fails, Telegram runtime now retries through the latest authorized browser session provider instead of returning immediate failure.
- File: `go-agent/internal/gateway/telegram_runtime.go`

2. Provider API key propagation into browser bridge requests
- Browser completion payloads now carry `apiKey` and `api_key` when configured.
- Applies to Telegram runtime and direct scheduled `browser.request`/`browser.open` calls.
- Files:
  - `go-agent/internal/gateway/server.go`
  - `go-agent/internal/gateway/telegram_runtime.go`
  - `go-agent/internal/tools/runtime/runtime.go`

3. Telegram context-budget safety
- Budget trimming now guarantees the newest user turn is retained (truncated when needed) instead of being dropped.
- File: `go-agent/internal/gateway/telegram_runtime.go`

4. Test coverage added for the above
- cross-provider Telegram failover
- API key forwarding in Telegram and bridge runtime
- newest-user-turn retention under tight budget
- Files:
  - `go-agent/internal/gateway/telegram_runtime_test.go`
  - `go-agent/internal/tools/runtime/runtime_test.go`
