# Rust -> Go Parity Matrix

Status legend: `not-started`, `in-progress`, `done`, `deferred`

| Rust Module | Go Target Package | Status | Notes |
| --- | --- | --- | --- |
| `src/main.rs` | `go-agent/cmd/openclaw-go` | in-progress | CLI bootstrap started in phase 1 |
| `src/config.rs` | `go-agent/internal/config` | in-progress | Minimal config + env overrides in phase 1 |
| `src/gateway_server.rs` | `go-agent/internal/gateway` | in-progress | HTTP health endpoint + RPC envelope plumbing and status scaffold |
| `src/protocol.rs` | `go-agent/internal/protocol` | done | Framing, method-family classification, rpc request/response/error helpers, fixture corpus tests |
| `src/gateway.rs` | `go-agent/internal/rpc` + `internal/gateway` | in-progress | Method registry scaffold (supported list + canonical resolution), health/status RPC wiring |
| `src/scheduler.rs` | `go-agent/internal/scheduler` | not-started | Phase 3 target |
| `src/runtime.rs` | `go-agent/internal/runtime` | not-started | Depends on protocol + scheduler |
| `src/tool_registry.rs` | `go-agent/internal/tools` | not-started | Phase 4 target |
| `src/tool_runtime.rs` | `go-agent/internal/tools/runtime` | not-started | Phase 4 target |
| `src/telegram_bridge.rs` | `go-agent/internal/channels/telegram` | not-started | Phase 5 target |
| `src/channels/mod.rs` | `go-agent/internal/channels` | not-started | Phase 5 target |
| `src/memory.rs` | `go-agent/internal/memory` | not-started | Phase 5 target |
| `src/persistent_memory.rs` | `go-agent/internal/memory/persist` | not-started | Phase 5 target |
| `src/state.rs` | `go-agent/internal/state` | not-started | Phase 5 target |
| `src/session_key.rs` | `go-agent/internal/session` | not-started | Phase 5 target |
| `src/security/*` | `go-agent/internal/security/*` | not-started | Phase 6 target |
| `src/security_audit.rs` | `go-agent/internal/security/audit` | not-started | Phase 6 target |
| `src/routines.rs` | `go-agent/internal/routines` | not-started | Phase 7 target |
| `src/wasm_runtime.rs` | `go-agent/internal/wasm/runtime` | not-started | Phase 7 target |
| `src/wasm_sandbox.rs` | `go-agent/internal/wasm/sandbox` | not-started | Phase 7 target |
| `src/website_bridge.rs` | `go-agent/internal/bridge/web` | not-started | Phase 4 target |
| `src/bridge.rs` | `go-agent/internal/bridge` | not-started | Cross-phase orchestration |

## Current Gap Summary

- Completed: `1`
- In-progress: `4`
- Not-started: `17`
- Deferred: `0`
