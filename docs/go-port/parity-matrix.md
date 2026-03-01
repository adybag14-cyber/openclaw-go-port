# Rust -> Go Parity Matrix

Status legend: `not-started`, `in-progress`, `done`, `deferred`

| Rust Module | Go Target Package | Status | Notes |
| --- | --- | --- | --- |
| `src/main.rs` | `go-agent/cmd/openclaw-go` | done | CLI bootstrap delivered with config/http-bind overrides and signal-safe runtime startup |
| `src/config.rs` | `go-agent/internal/config` | done | TOML + env loading, runtime profile validation, and security defaults are complete |
| `src/gateway_server.rs` | `go-agent/internal/gateway` | done | HTTP health/RPC server, auth/connect/session lifecycle, scheduler routing, and edge handlers shipped |
| `src/protocol.rs` | `go-agent/internal/protocol` | done | Framing, method-family classification, rpc request/response/error helpers, fixture corpus tests |
| `src/gateway.rs` | `go-agent/internal/rpc` + `internal/gateway` | done | Canonical method registry and gateway dispatch parity surface delivered (`133/133` method match with Rust); `security.audit` remains callable but intentionally non-advertised |
| `src/scheduler.rs` | `go-agent/internal/scheduler` | done | Queue/worker execution + `agent.wait` status contracts and scheduler metrics delivered |
| `src/runtime.rs` | `go-agent/internal/runtime` | done | Runtime snapshot/profile model implemented (`core`/`edge`, `audit-only` vs enforcing mode) |
| `src/tool_registry.rs` | `go-agent/internal/tools` | done | Tool catalog and runtime provider registry flow delivered |
| `src/tool_runtime.rs` | `go-agent/internal/tools/runtime` | done | Runtime invoke path for browser bridge and agent/tool orchestration delivered |
| `src/telegram_bridge.rs` | `go-agent/internal/channels/telegram` | done | Telegram channel driver send/status/logout semantics shipped |
| `src/channels/mod.rs` | `go-agent/internal/channels` | done | Channel registry abstraction + canonical alias routing delivered |
| `src/memory.rs` | `go-agent/internal/memory` | done | Message history storage and channel/session query surfaces delivered |
| `src/persistent_memory.rs` | `go-agent/internal/memory/persist` | done | JSON-backed persistence via `runtime.state_path` delivered in store layer |
| `src/state.rs` | `go-agent/internal/state` | done | Session state tracking and touch counters are integrated |
| `src/session_key.rs` | `go-agent/internal/session` | done | Session-key descriptor parser ported with main/direct/group/channel/cron/hook/node coverage |
| `src/security/*` | `go-agent/internal/security/*` | done | Policy guard parity includes bundle loading, telemetry high-risk handling, credential leak detection, and auth-handshake-safe enforcement |
| `src/security_audit.rs` | `go-agent/internal/security/audit` | done | Security audit package delivered with summary findings and deep gateway probe support |
| `src/routines.rs` | `go-agent/internal/routines` | done | Routine registry + deterministic execution contract + tests |
| `src/wasm_runtime.rs` | `go-agent/internal/wasm/runtime` | done | WASM module marketplace + sandbox-gated execution + tests |
| `src/wasm_sandbox.rs` | `go-agent/internal/wasm/sandbox` | done | Default sandbox policy + capability evaluation + tests |
| `src/website_bridge.rs` | `go-agent/internal/bridge/web` | done | Browser auth login manager (`start/wait/complete/logout`) integrated and validated end-to-end |
| `src/bridge.rs` | `go-agent/internal/bridge` | done | Bridge orchestration through gateway + scheduler + tool runtime is complete |

## Current Gap Summary

- Completed: `22`
- In-progress: `0`
- Not-started: `0`
- Deferred: `0`

## Post-v2 Continuation Status (Issue #3)

- Slice 1 complete: edge contract hardening (`edge.swarm.plan`, `edge.multimodal.inspect`, `edge.voice.transcribe`, `edge.quantum.status`).
- Slice 2 complete: security policy expressiveness (`group:*` tool policy matching with deterministic precedence).
- Slice 3 complete: WASM runtime depth (`wazero` compiled execution path under policy gates).
- Slice 4 complete: doctor/security audit depth (expanded audit checks, deep probes, parity corpus tests, and `doctor.checks` diagnostics output).
- Slice 5 complete: security audit remediation flow (`--security-audit --fix`) with persisted config fixes, policy bundle bootstrap, idempotency tests, and action-level fix reporting.
- Slice 6 complete (Issue #5): websocket gateway parity (`/ws` request/response loop) and telegram control parity expansion (`/set api key`, `/auth providers|wait|bridge|help`, `/tts providers|help`) with integration tests.
- Slice 7 complete (Issue #6): provider/account-scoped Telegram auth parity (`/auth start|status|wait|complete|cancel` scoped by provider/account/session) with backward-compatible short forms and integration coverage.
- Slice 8 complete (Issue #6): security engine depth parity (EDR telemetry feed parsing/caching + runtime attestation mismatch scoring/reporting + config/env validation surfaces).
- Slice 9 complete (Issue #6): edge runtime behavior depth (`tinywhisper` local STT execution path, attestation-binary enclave proof path, and non-dry-run finetune trainer execution/manifest contracts).
