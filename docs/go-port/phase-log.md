# Go Port Phase Log

## 2026-02-28

### Phase 0 Completed

- Created fork-equivalent repo: `adybag14-cyber/openclaw-go-port`.
- Wired remotes:
  - `origin -> openclaw-go-port`
  - `upstream -> openclaw-rust`
- Added planning/tracking artifacts:
  - `docs/GO_PORT_PLAN.md`
  - `docs/go-port/phase-checklist.md`
  - `docs/go-port/parity-matrix.md`
  - `docs/go-port/phase-log.md`
- Created local skill `openclaw-go-port` for repeatable migration workflow.
- Opened tracker issue: `https://github.com/adybag14-cyber/openclaw-go-port/issues/1`.

### Phase 1 Started

- Goal: establish runnable Go module with baseline config + health/control HTTP skeleton.
- Planned validation: `go test ./...` and `go vet ./...` via Dockerized Go toolchain.

### Phase 1 Bootstrap Delivered

- Added `go-agent` module with:
  - `cmd/openclaw-go` executable bootstrap.
  - Config loader (`internal/config`) with TOML + env override support.
  - Health/control HTTP skeleton (`internal/gateway`) with deterministic `/health` and phase-1 `/rpc` stub.
  - Unit tests for config and gateway health/RPC responses.
- Validation commands passed via Docker:
  - `gofmt -w ./cmd ./internal`
  - `go mod tidy`
  - `go test ./...`
  - `go vet ./...`

### Phase 2 Protocol + RPC Scaffold Delivered

- Added `internal/protocol` with:
  - frame kind detection (`req/resp/event/error`)
  - method family classification parity helpers
  - RPC request/response/error parsing helpers
  - Rust-style response envelope builders (`rpc_success_response_frame`/`rpc_error_response_frame` equivalent semantics)
- Added fixture-backed protocol compatibility tests:
  - `internal/protocol/testdata/frame-corpus.json`
  - corpus snapshot assertions for kind/family/request-id/error-presence.
- Added `internal/rpc` method registry scaffold:
  - canonical normalization and resolve behavior
  - supported-method list parity scaffold from Rust method surface.
- Updated gateway `/rpc` handling to:
  - validate `type: req` envelope
  - resolve canonical methods through registry
  - return Rust-style `resp` envelope results/errors
  - implement `health`/`status` RPC success path.
- Validation commands passed via Docker:
  - `gofmt -w ./cmd ./internal`
  - `go mod tidy`
  - `go test ./...`
  - `go vet ./...`

### Phase 3 Gateway + Scheduler Delivered

- Added gateway auth/connect lifecycle:
  - `connect` with auth mode resolution (`none`/`token`/`password`).
  - session registry with `sessions.list` + `session.status`.
- Added scheduler primitives:
  - queue/worker job execution model.
  - `agent.wait` job wait/status contract.
  - scheduler status in `status` payload.
- Added test coverage for connect auth and session lifecycle.

### Phase 4 Tool Runtime + Web/Auth Bridge Delivered

- Added tool runtime orchestration package with provider interface and builtin bridge provider.
- Added `tools.catalog` surface and runtime invocation path for browser tools.
- Added web auth/login manager with:
  - `web.login.start`
  - `web.login.wait`
  - `auth.oauth.complete`
  - `auth.oauth.logout`
- Added auth-gated browser bridge behavior:
  - browser methods blocked until login is authorized.
  - browser requests executed through scheduler + tool runtime and returned via `agent.wait`.
- Added end-to-end gateway test covering full login -> browser request -> wait-for-result path.
- Validation commands passed via Docker:
  - `gofmt -w ./cmd ./internal`
  - `go mod tidy`
  - `go test ./...`
  - `go vet ./...`

### Phase 5 Channels + Memory/State Delivered

- Added channel abstraction and telegram bridge scaffold:
  - `internal/channels` registry with channel alias normalization and driver resolution.
  - built-in `webchat` and `cli` drivers.
  - telegram driver scaffold with token/default-target config wiring.
  - gateway routes: `channels.status`, `channels.logout`.
- Added memory + state surfaces:
  - `internal/memory` message history store with optional JSON persistence via `runtime.state_path`.
  - `chat.history` and `sessions.history` RPC routes.
  - `internal/state` session state tracker (last channel, last method, counters).
- Extended gateway runtime execution:
  - `send/chat.send/sessions.send` now route through channel registry.
  - outbound message events are recorded to memory/state.
  - session channel defaults are reused for sends when channel is omitted.
- Added tests:
  - channels package tests.
  - memory and state store tests.
  - end-to-end gateway flow for connect + send + wait + session/chat history + channel logout.
- Validation commands passed via Docker:
  - `gofmt -w ./cmd ./internal`
  - `go mod tidy`
  - `go test ./...`
  - `go vet ./...`

### Phase 6 Security/Policy Slice Delivered (Partial)

- Added `internal/security` guard layer with:
  - default action policy (`allow/review/block`)
  - per-method tool policies
  - blocked message pattern enforcement
  - optional JSON policy bundle loading (`security.policy_bundle_path`)
- Wired security guard into gateway dispatch:
  - mutating methods are evaluated before execution
  - blocked methods return deterministic policy error (`-32050`)
  - `config.get` exposes current security snapshot
- Added tests:
  - security guard unit tests (tool policy, pattern block, bundle load)
  - gateway integration test for policy-enforced blocking
- Validation commands passed via Docker:
  - `gofmt -w ./cmd ./internal`
  - `go mod tidy`
  - `go test ./...`
  - `go vet ./...`

### Phase 6 Security/Policy Stack Delivered

- Expanded security configuration and guard behavior:
  - telemetry high-risk tags and action policy wiring.
  - credential-sensitive key scanning and leak policy actions.
  - auth-handshake safe handling so `connect` auth payloads are validated without false-positive leak blocking.
- Added/extended tests:
  - credential leak detection tests.
  - telemetry high-risk review tests.
  - auth handshake allowlist test for `connect`.
  - gateway integration coverage for credential and telemetry policy outcomes.
- Validation commands passed via Docker:
  - `gofmt -w ./cmd ./internal`
  - `go mod tidy`
  - `go test ./...`
  - `go vet ./...`

### Phase 7 Advanced Runtime Features Delivered

- Added advanced runtime packages:
  - `internal/routines` with routine registry and deterministic run contracts.
  - `internal/wasm/runtime` marketplace and sandbox-gated execution.
  - `internal/wasm/sandbox` policy evaluation for capability controls.
- Wired edge/runtime parity methods in gateway dispatch:
  - wasm marketplace, router/swam/multimodal, enclave/mesh, homomorphic compute, finetune run/status, identity/personality/handoff, marketplace preview, cluster planning, alignment, quantum status, collaboration, and voice transcription.
  - `config.get` now exposes routines and wasm runtime snapshots.
- Added advanced integration coverage:
  - edge wasm + routines execution flow checks.
  - edge method matrix replay-style gateway test covering full new `edge.*` method set.
- Validation commands passed via Docker:
  - `gofmt -w ./cmd ./internal`
  - `go mod tidy`
  - `go test ./...`
  - `go vet ./...`

### Phase 8 Cutover Readiness + v1.0.0-go Delivered

- Closed remaining parity modules:
  - Added `internal/runtime` profile snapshot package (`core`/`edge`, audit/enforcing modes).
  - Added `internal/session` session-key parser parity port.
  - Added `internal/security/audit` package with summary findings + deep gateway probe.
  - Added gateway `security.audit` RPC route and runtime snapshot integration in `status`/`config.get`.
- Cross-platform build validation:
  - Dockerized Windows build: `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ...`
  - Dockerized Linux build: `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ...`
  - Windows binary CLI smoke: `openclaw-go-windows-amd64.exe --help`
  - Linux binary runtime smoke in container:
    - `GET /health` returns `status=ok`, `version=v1.0.0-go`.
- Oracle VM smoke validation (`ubuntu@144.21.61.111`):
  - Uploaded Linux artifact via `scp`.
  - Executed binary and verified:
    - `GET /health` -> HTTP 200 with `status=ok`.
    - `POST /rpc` (`status` method) -> HTTP 200 with valid RPC response payload.
- Final parity sign-off:
  - `docs/go-port/parity-matrix.md` now reflects `22/22` required modules complete.
- Release packaging:
  - Release artifact directory: `dist/release-v1.0.0-go/`
  - Built artifacts:
    - `openclaw-go-windows-amd64.exe`
    - `openclaw-go-linux-amd64`

### Post-v1.0.0 Parity Alignment + v1.0.1-go

- Closed method-contract parity delta against `openclaw-rust` by aligning advertised Go RPC surface to exact Rust method count (`133/133`).
- Removed only the non-parity advertised extra (`security.audit`) from Go method registry while keeping runtime handler availability for diagnostics.
- Added hard dispatch parity gate in gateway tests ensuring all advertised methods resolve without `-32601`.
- Revalidated Dockerized Go matrix (`gofmt`, `go test ./...`, `go vet ./...`) and method-surface diff (`missing=0`, `extra=0`).

### v2.0 Program Phase 3 Slice: Browser Bridge Runtime Hardening

- Added configurable browser bridge runtime controls under `runtime.browser_bridge`:
  - `enabled`, `endpoint`, `request_timeout_ms`, `retries`, `retry_backoff_ms`, `circuit_fail_threshold`, `circuit_cooldown_ms`.
  - Environment overrides for all browser-bridge settings.
- Upgraded Go tool runtime browser path:
  - `browser.request` now detects chat-completion payloads (`messages` or prompt/message text) and calls the local browser bridge `/v1/chat/completions`.
  - Added retry/backoff handling and circuit-breaker protection for bridge instability.
  - Preserved legacy compatibility for URL/method probe-style `browser.request` calls.
- Added tests:
  - Runtime tests for bridge completion success, retry recovery, circuit-breaker open behavior, and disabled-bridge validation.
  - Gateway integration test validating end-to-end `web.login.start -> auth.oauth.complete -> browser.request -> agent.wait` with a real assistant response payload.
- Validation completed (Dockerized Go toolchain):
  - `gofmt -w ./cmd ./internal`
  - `go test ./...`
  - `go vet ./...`

### v2.0 Program Phase 4 Slice: Telegram Command Parity (`/model`, `/auth`, `/tts`)

- Added Telegram command execution path in Go gateway runtime:
  - `send`/`chat.send` requests on `telegram` channel now intercept slash commands before generic send dispatch.
  - Commands return structured `channels.SendReceipt` with command metadata payload.
- Implemented `/model` command behavior:
  - `/model` and `/model list|status` return current + available models.
  - `/model <id>` validates against known model list and sets target-scoped active model.
- Implemented `/auth` command behavior:
  - `/auth` starts a browser login session and returns code + verification URI.
  - `/auth status` reports current session state.
  - `/auth complete <CODE>` completes the pending target-scoped login session.
- Implemented `/tts` command behavior:
  - `/tts status|on|off`
  - `/tts provider <name>`
  - `/tts say <text>` with synthesized audio metadata (`audioRef`, bytes).
- Added compatibility state tracking for telegram target-scoped model/auth session mappings.
- Added end-to-end gateway test coverage:
  - connect telegram session
  - `/model gpt-5.2-pro`
  - `/auth` + `/auth complete <code>`
  - `/tts provider openai-voice`
  - `/tts say ...`
  - validates command metadata and authorized auth completion.
- Validation completed (Dockerized Go toolchain):
  - `gofmt -w ./cmd ./internal`
  - `go test ./...`
  - `go vet ./...`

### v2.0 Program Phase 5 Slice: WASM Runtime Lifecycle + Policy Depth

- Hardened WASM sandbox decision model:
  - `DeniedCapabilities` now included in sandbox decisions.
  - policy checks aggregate all denied capabilities rather than returning on first failure.
- Expanded WASM runtime model behavior:
  - module lifecycle operations: `InstallModule`, `RemoveModule`.
  - runtime policy mutation: `SetPolicy`.
  - deterministic marketplace ordering.
  - module metadata expanded with `WIT` and `EntryPoint`.
- Added stricter execution policy enforcement:
  - per-call `timeoutMs` and `memoryMB` checks against sandbox limits.
  - capability union between module capabilities and `requiredCapabilities` execution request.
  - explicit sandbox denials for policy-limit breaches.
- Added runtime test coverage for:
  - deterministic marketplace order,
  - lifecycle install/remove flow,
  - timeout/memory policy denials,
  - policy override allowing network capability for approved modules.
- Added sandbox test coverage for multi-capability denial aggregation.
- Validation completed (Dockerized Go toolchain):
  - `gofmt -w ./cmd ./internal`
  - `go test ./...`
  - `go vet ./...`

### v2.0 Program Phase 6 Slice: Memory Vector + Graph Recall Surfaces

- Upgraded memory store internals with embedded vector + graph indexing:
  - vector embeddings per message entry (normalized token vectors).
  - graph edges across session/channel/role/method/term nodes.
  - deterministic index rebuild on append/load for consistency.
- Added memory depth APIs in store:
  - `SemanticRecall(query, limit)`
  - `GraphNeighbors(node, limit)`
  - `RecallSynthesis(query, limit)`
  - `Stats()` (entries/vectors/graph nodes/graph edges/persistence).
- Extended persisted memory snapshot to include vector + graph structures.
- Integrated memory stats exposure into gateway responses:
  - `config.get` now includes `memory` stats object.
  - `status` and `doctor.memory.status` include deep memory stats.
- Added tests:
  - semantic recall relevance
  - graph neighbor retrieval + recall synthesis
  - stats + persistence/recovery verification
  - gateway `config.get` memory stats contract check
- Validation completed (Dockerized Go toolchain):
  - `gofmt -w ./cmd ./internal`
  - `go test ./...`
  - `go vet ./...`

### v2.0 Program Phase 7 Slice: Edge Handler Statefulness + Contract Depth

- Replaced static edge placeholders with stateful behavior:
  - added internal edge-state tracker for finetune jobs and enclave proof history.
  - `edge.finetune.run` now records stateful job entries with runtime payload references.
  - `edge.finetune.status` returns tracked jobs instead of fixed static placeholders.
  - `edge.enclave.prove` now generates deterministic hashed proof artifacts (non-placeholder).
  - `edge.enclave.status` now reports proof count + last challenge/proof time.
- Expanded compute contract:
  - `edge.homomorphic.compute` now supports `sum|mean|max|min`.
- Expanded collaboration contract:
  - `edge.collaboration.plan` now includes `goal` and checkpoint objects for deterministic planning state.
- Added edge integration test coverage (`TestEdgeStatefulContracts`) validating:
  - finetune run/status state propagation,
  - non-placeholder enclave proof issuance and status retention,
  - homomorphic `mean` operation correctness.
- Validation completed (Dockerized Go toolchain):
  - `gofmt -w ./cmd ./internal`
  - `go test ./...`
  - `go vet ./...`

### v2.0 Program Phase 8 Slice: Doctor + CLI Diagnostics Depth

- Expanded CLI diagnostic surfaces in `openclaw-go`:
  - `--doctor` (security/health diagnostics snapshot, JSON output)
  - `--security-audit` (audit report output without starting gateway server)
  - `--list-methods` (full RPC method catalog output)
  - `--deep` toggle for deep audit probes.
- Added diagnostics execution path in app layer:
  - outputs structured JSON with service/version/config snapshot.
  - includes security audit summary when doctor/audit mode is selected.
  - includes sorted method catalog for list-methods mode.
- Added app-level tests validating doctor and method-list JSON output contracts.
- Validation completed (Dockerized Go toolchain):
  - `gofmt -w ./cmd ./internal`
  - `go test ./...`
  - `go vet ./...`

### v2.0 Program Phase 9 Slice: Android/Termux + Optimization Build Matrix

- Added release matrix build scripts:
  - `scripts/build-matrix.ps1`
  - `scripts/build-matrix.sh`
- Build matrix now targets:
  - `windows/amd64`
  - `linux/amd64`
  - `android/arm64` (Termux-ready artifact target)
- Enforced optimization defaults in build scripts:
  - `CGO_ENABLED=0`
  - `-trimpath`
  - stripped binaries via `-ldflags "-s -w"`
  - SHA256 checksum manifest generation (`SHA256SUMS.txt`).
- Updated `go-agent/README.md` with diagnostics usage and matrix build commands.
- Validation completed:
  - full Go validation matrix (`go test ./...`, `go vet ./...`)
  - cross-build smoke for `windows/amd64`, `linux/amd64`, `android/arm64` inside Docker.

### v2.0 Program Phase 10 Completed: Final Validation + Release Packaging

- Executed final gate rerun for full Go validation matrix:
  - `go test ./...`
  - `go vet ./...`
- Built final release artifact set in `dist/release-v2.0.0-go-assets/`:
  - `openclaw-go-windows-amd64.exe`
  - `openclaw-go-linux-amd64`
  - `openclaw-go-android-arm64`
  - `SHA256SUMS.txt`
- Completed runtime smoke checks:
  - Windows binary diagnostics (`--doctor`, `--list-methods`) verified.
  - Linux binary gateway smoke (`/health`, `/rpc status`) verified.
- Added release notes:
  - `docs/go-port/release-v2.0.0-go.md`

### Post-v2 Continuation (Issue #3) - Slice 1: Edge Contract Hardening

- Opened continuation tracker issue:
  - `https://github.com/adybag14-cyber/openclaw-go-port/issues/3`
- Linked closed v2 program issue (#2) to continuation tracker.
- Implemented first depth-parity slice in Go gateway edge handlers:
  - `edge.swarm.plan` now enforces required input (`tasks` or `goal`) and returns deterministic task graph contracts.
  - `edge.multimodal.inspect` now enforces required input and returns inferred modalities + media metadata summary.
  - `edge.voice.transcribe` now removes placeholder transcript behavior and supports `audioPath|audioRef` + `hintText` synthesis with provider/source metadata.
  - `edge.quantum.status` now reports env-driven PQC posture (`off|hybrid|strict-pqc`) with algorithm metadata instead of fixed `simulated` mode.
- Added/updated integration tests:
  - matrix assertions for quantum mode contract and non-placeholder voice transcript
  - new validation tests for required edge inputs
  - new voice synthesis behavior test (hint passthrough + audio-stem synthesis)
  - new PQC env-driven quantum status test
- Validation completed (Dockerized Go toolchain):
  - `gofmt -w ./cmd ./internal`
  - `go test ./...`
  - `go vet ./...`

### Post-v2 Continuation (Issue #3) - Slice 2: Security Policy Expressiveness

- Extended Go guard policy matching with group selectors (`group:*`) to align closer to Rust-style policy ergonomics.
- Added supported groups:
  - `group:edge`
  - `group:browser`
  - `group:messaging`
  - `group:sessions`
  - `group:system`
  - `group:nodes`
- Preserved deterministic precedence:
  - exact method policy entries still override group-expanded wildcard policies.
- Added security regression tests:
  - `TestToolPolicyGroupBlock`
  - `TestToolPolicySpecificOverrideGroup`
- Validation completed (Dockerized Go toolchain):
  - `gofmt -w ./cmd ./internal`
  - `go test ./...`
  - `go vet ./...`

### Post-v2 Continuation (Issue #3) - Slice 3: WASM Engine Depth (`wazero`)

- Added real WASM engine execution path to Go runtime using `github.com/tetratelabs/wazero`.
- Extended module install schema to support compiled module sources:
  - `wasmBase64`
  - `wasmPath`
  - in-memory `Binary` bytes (internal)
- Runtime execution behavior:
  - modules with compiled bytes run through `wazero` exported function calls.
  - modules without bytes retain prior synthetic execution path for backward compatibility.
  - output now reports execution engine (`wazero` vs `synthetic`).
- Added regression tests:
  - compiled module execution via `wasmBase64`
  - invalid base64 rejection on install
  - missing entrypoint error
  - unsupported wasm argument type error
- Added Go module dependency + lock updates:
  - `go.mod` / `go.sum` include `wazero`.
- Validation completed (Dockerized Go toolchain):
  - `go mod tidy`
  - `gofmt -w ./cmd ./internal`
  - `go test ./...`
  - `go vet ./...`

### Post-v2 Continuation (Issue #3) - Slice 4: Doctor + Security Audit Depth

- Expanded Go security audit finding coverage to better match Rust depth posture:
  - gateway exposure checks:
    - `gateway.bind.public`
    - `gateway.http_bind.public`
  - runtime posture checks:
    - `runtime.state_path.in_memory`
    - `runtime.browser_bridge.endpoint.public`
  - security posture checks:
    - `security.loop_guard.disabled`
    - `security.loop_guard.thresholds.invalid`
    - `security.risk_thresholds.permissive`
  - policy bundle validation checks:
    - `security.policy_bundle.stat_failed`
    - `security.policy_bundle.is_dir`
    - `security.policy_bundle.read_failed`
    - `security.policy_bundle.parse_failed`
- Expanded deep audit report shape:
  - `deep.gateway` probe (existing)
  - `deep.browserBridge` probe (new)
  - `deep.policyBundle` probe (new)
- Added deep-probe surfaced findings:
  - `browser_bridge.deep_probe`
  - `security.policy_bundle.deep_probe`
- Added parity corpus-style audit tests to lock deterministic check-id coverage for hardened/misconfigured profiles.
- Expanded CLI doctor diagnostics payload with structured `doctor.checks`:
  - auth secret readiness
  - gateway bind scope
  - browser bridge endpoint posture
  - state path persistence posture
  - policy bundle readiness
  - loop guard + risk-threshold posture
  - security audit summary projection
  - deep probe status entries (when `--deep`)
  - binary availability checks (`docker`, `wasmtime`)
- Validation completed (Dockerized Go toolchain):
  - `gofmt -w ./cmd ./internal`
  - `go test ./...`
  - `go vet ./...`

### Post-v2 Continuation (Issue #4) - Slice 1: Security Audit Remediation (`--fix`)

- Added Rust-style security audit remediation support in Go audit package:
  - `securityaudit.Options{Fix, ConfigPath}` support.
  - `report.fix` payload with deterministic action/change/error model.
- Implemented safe config remediation flow for `--security-audit --fix`:
  - auth posture:
    - `gateway.server.auth_mode` from `none` -> `auto`.
  - bind posture:
    - reset non-loopback `gateway.server.bind` / `gateway.server.http_bind` to loopback defaults.
  - runtime posture:
    - convert `runtime.state_path` from `memory://...` to persisted state file path.
    - normalize non-loopback browser bridge endpoint to default loopback endpoint.
  - security posture:
    - enable loop guard and restore positive thresholds.
    - restore default blocked message patterns when empty.
    - restore default credential-sensitive keys when empty.
    - normalize permissive risk thresholds to defaults.
  - policy bundle posture:
    - set persisted `security.policy_bundle_path` when unset/in-memory.
    - auto-create baseline JSON bundle when missing.
  - persistence:
    - writes remediated TOML config to `--config` target.
    - includes write/chmod action outcomes in fix report.
- Integration wiring:
  - Added CLI flag `--fix` and constrained it to `--security-audit` mode.
  - `app` diagnostics now emits fix report in `securityAudit.fix` when enabled.
- Added regression coverage:
  - remediation persistence test
  - idempotent second fix-run test
  - app-level security-audit fix JSON contract test
- Validation completed (Dockerized Go toolchain):
  - `gofmt -w ./cmd ./internal`
  - `go test ./...`
  - `go vet ./...`

### Post-v2 Continuation (Issue #5) - Slice 1: WebSocket Gateway + Telegram Control Parity

- Added WebSocket RPC gateway endpoint parity:
  - `/ws` route now upgrades and handles request/response loops with Rust-compatible RPC envelopes.
  - validates frame kind (`type=req`) and returns canonical parse/invalid request errors.
- Expanded Telegram parity command surface:
  - new `/set api key <provider> <key>` flow with provider key storage metadata.
  - new auth actions: `/auth help`, `/auth providers`, `/auth wait ...`, `/auth bridge`.
  - auth completion now accepts callback URLs and extracts code query params.
  - new TTS actions: `/tts providers`, `/tts help`.
- Added compatibility state storage for provider API keys.
- Added/expanded tests:
  - `TestWebSocketRPCDispatch`
  - expanded telegram command integration coverage in `TestTelegramCommandFlowModelAuthTTS`.
- Validation completed (Dockerized Go toolchain with explicit Go path):
  - `/usr/local/go/bin/go mod tidy`
  - `/usr/local/go/bin/gofmt -w ./cmd ./internal`
  - `/usr/local/go/bin/go test ./...`
  - `/usr/local/go/bin/go vet ./...`
- Release artifacts built for `v2.0.4-go`:
  - `openclaw-go-windows-amd64.exe`
  - `openclaw-go-linux-amd64`
  - `openclaw-go-android-arm64`
  - `SHA256SUMS.txt`

### Post-v2 Continuation (Issue #6) - Slice 1: Provider-Scoped Telegram Auth Command Parity

- Expanded Telegram auth flow semantics to support scoped auth contexts:
  - provider/account-scoped auth mapping in compat state (`target + provider + account -> loginSessionId`), with safe fallback to target-level legacy mapping.
  - retained backward compatibility with existing short forms (`/auth`, `/auth complete <code>`).
- Expanded `/auth` command contracts:
  - `/auth start <provider> [account] [--force]`
  - `/auth status [provider] [account] [session_id]`
  - `/auth wait <provider> [session_id] [account] [--timeout <seconds>]`
  - `/auth complete <provider> <callback_url_or_code> [session_id] [account]`
  - `/auth cancel [provider] [account] [session_id]`
- Added richer auth metadata payloads for command results:
  - `provider`, `account`, `scope`, and resolved session ids where applicable.
- Added provider-specific browser verification URI routing in web login manager:
  - `chatgpt|codex -> https://chatgpt.com/`
  - `openrouter -> https://openrouter.ai/`
  - `kimi -> https://kimi.com/`
  - `qwen -> https://chat.qwen.ai/`
  - plus login manager test coverage for provider verification URL outputs.
- Added richer `/auth bridge` diagnostics metadata:
  - bridge payload now includes web-login session summary (`total`, status counters, `byProvider` buckets).
  - gateway integration assertion updated to verify diagnostics summary is present.
- Added integration test coverage for provider/account auth flow:
  - provider-scoped start/status/wait/complete/cancel path with callback URL code extraction.
- Validation completed (Dockerized Go toolchain):
  - `/usr/local/go/bin/gofmt -w ...`
  - `/usr/local/go/bin/go mod tidy`
  - `/usr/local/go/bin/go test ./...`
  - `/usr/local/go/bin/go vet ./...`
