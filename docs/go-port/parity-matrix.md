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
- Slice 10 complete (Issue #7): homomorphic ciphertext contract hardening (`keyId` requirement, strict op set, `mean` reveal requirement, and empty-ciphertext rejection with integration coverage).
- Slice 11 complete (Issue #7): security audit parity expansion for telemetry/attestation posture findings plus remediation normalization.
- Slice 12 complete (Issue #7): enclave proof input strictness + enriched enclave status metadata + finetune execution status/failure/timeout contract depth.
- Slice 13 complete (Issue #8): session lifecycle parity depth (`sessions.delete/reset/compact` now mutate session registry + state + memory with durable compaction/removal behavior).
- Slice 14 complete (Issue #8): operational parity cleanup (status phase marker no longer scaffolded + `update.run`/`poll` lifecycle contracts with transition-aware job tracking).
- Slice 15 complete (Issue #9): residual simulated/scaffold semantic cleanup (deterministic enclave mode resolution, local-heuristic voice fallback semantics, and scaffold wording removal in gateway/runtime metadata).
- Slice 16 complete (Issue #10): compat dispatch tightening (generic fallback removed; unsupported compat methods now return strict `-32601`, with supported-method coverage guard against silent fallback drift).
- Slice 17 complete (Issue #11): tool runtime depth expansion (Rust-style top-level tool families + in-memory message/session lifecycle actions + family integration coverage in Go runtime).
- Slice 18 complete (Issue #12): provider/auth catalog widening (Rust-aligned provider alias normalization, expanded OAuth provider catalog payloads, and Qwen `copaw` alias support across `/auth providers`, `auth.oauth.providers`, and browser-login verification routing).
- Slice 19 complete (Issue #13): model catalog widening (Qwen/OpenCode/Inception/OpenRouter/Codex model descriptors) plus slash-scoped provider/model resolution hardening and alias-backed model matching in `resolveModelChoice`.
- Slice 20 complete (Issue #14): provider-aware browser runtime bridge semantics (provider passthrough + alias normalization including `copaw`) for `browser.request` completion payloads and response metadata.
