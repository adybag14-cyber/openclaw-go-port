# Rust -> Go Parity Matrix

Status legend: `not-started`, `in-progress`, `done`, `deferred`

| Rust Module | Go Target Package | Status | Notes |
| --- | --- | --- | --- |
| `src/main.rs` | `go-agent/cmd/openclaw-go` | in-progress | CLI bootstrap started in phase 1 |
| `src/config.rs` | `go-agent/internal/config` | in-progress | Minimal config + env overrides in phase 1 |
| `src/gateway_server.rs` | `go-agent/internal/gateway` | in-progress | HTTP health endpoint + auth/connect/session lifecycle + scheduler-backed RPC plumbing |
| `src/protocol.rs` | `go-agent/internal/protocol` | done | Framing, method-family classification, rpc request/response/error helpers, fixture corpus tests |
| `src/gateway.rs` | `go-agent/internal/rpc` + `internal/gateway` | in-progress | Method registry scaffold (supported list + canonical resolution), health/status/connect/session/tool routes wired |
| `src/scheduler.rs` | `go-agent/internal/scheduler` | in-progress | Queue/worker primitives, job wait/status, scheduler stats integrated in gateway status |
| `src/runtime.rs` | `go-agent/internal/runtime` | not-started | Depends on protocol + scheduler |
| `src/tool_registry.rs` | `go-agent/internal/tools` | in-progress | `tools.catalog` runtime provider catalog scaffold |
| `src/tool_runtime.rs` | `go-agent/internal/tools/runtime` | in-progress | Provider interface + builtin browser bridge runtime execution |
| `src/telegram_bridge.rs` | `go-agent/internal/channels/telegram` | in-progress | Telegram channel driver scaffold with send/logout/status and alias mapping |
| `src/channels/mod.rs` | `go-agent/internal/channels` | in-progress | Channel registry abstraction + `channels.status` and `channels.logout` wiring |
| `src/memory.rs` | `go-agent/internal/memory` | in-progress | Message history store + chat/session history query surface |
| `src/persistent_memory.rs` | `go-agent/internal/memory/persist` | in-progress | JSON persistence path support for memory store (`runtime.state_path`) |
| `src/state.rs` | `go-agent/internal/state` | in-progress | Session state tracker (last channel/method/message counters) |
| `src/session_key.rs` | `go-agent/internal/session` | not-started | Phase 5 target |
| `src/security/*` | `go-agent/internal/security/*` | not-started | Phase 6 target |
| `src/security_audit.rs` | `go-agent/internal/security/audit` | not-started | Phase 6 target |
| `src/routines.rs` | `go-agent/internal/routines` | not-started | Phase 7 target |
| `src/wasm_runtime.rs` | `go-agent/internal/wasm/runtime` | not-started | Phase 7 target |
| `src/wasm_sandbox.rs` | `go-agent/internal/wasm/sandbox` | not-started | Phase 7 target |
| `src/website_bridge.rs` | `go-agent/internal/bridge/web` | in-progress | Browser auth login manager (`start/wait/complete/logout`) integrated via RPC |
| `src/bridge.rs` | `go-agent/internal/bridge` | in-progress | Bridge orchestration scaffold through gateway + scheduler + tool runtime |

## Current Gap Summary

- Completed: `1`
- In-progress: `14`
- Not-started: `7`
- Deferred: `0`
