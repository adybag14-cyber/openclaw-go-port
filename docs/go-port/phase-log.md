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

### Post-v2 Continuation (Issue #6) - Slice 3: Security Engine Depth Parity

- Expanded Go security configuration/guard depth to close Rust-vs-Go parity gaps:
  - new security config fields for EDR telemetry feed and attestation posture:
    - `security.edr_telemetry_path`
    - `security.edr_telemetry_max_age_secs`
    - `security.edr_telemetry_risk_bonus`
    - `security.attestation_expected_sha256`
    - `security.attestation_report_path`
    - `security.attestation_mismatch_risk_bonus`
  - new env overrides:
    - `OPENCLAW_GO_EDR_TELEMETRY_PATH`
    - `OPENCLAW_GO_EDR_TELEMETRY_MAX_AGE_SECS`
    - `OPENCLAW_GO_EDR_TELEMETRY_RISK_BONUS`
    - `OPENCLAW_GO_ATTESTATION_EXPECTED_SHA256`
    - `OPENCLAW_GO_ATTESTATION_REPORT_PATH`
    - `OPENCLAW_GO_ATTESTATION_MISMATCH_RISK_BONUS`
- Guard runtime depth additions:
  - EDR telemetry feed scan with recency window + severity/tag/quarantine detection.
  - cached feed checks to reduce repeated file I/O churn.
  - runtime attestation digest snapshot with expected-hash mismatch risk scoring.
  - attestation snapshot/report surfaced in guard snapshot payload.
- Added regression tests:
  - `TestEDRTelemetryFeedReview`
  - `TestAttestationMismatchRaisesRisk`
  - config defaults/env/validation tests for new security fields.

### Post-v2 Continuation (Issue #6) - Slice 4: Edge Runtime Behavior Depth

- Reduced edge simulation depth with executable runtime paths:
  - `edge.voice.transcribe` now supports provider-order execution with local `tinywhisper` binary path + args:
    - `OPENCLAW_GO_TINYWHISPER_BIN`
    - `OPENCLAW_GO_TINYWHISPER_ARGS`
  - `edge.enclave.prove` now supports attestation-binary execution path with structured request/response fallback:
    - `OPENCLAW_GO_ENCLAVE_ATTEST_BIN`
    - `OPENCLAW_GO_ENCLAVE_ATTEST_ARGS`
    - `OPENCLAW_GO_ENCLAVE_ATTEST_TIMEOUT_MS`
  - `edge.finetune.run` now enforces deeper contracts and runtime execution:
    - dataset/auto-ingest memory requirement parity checks.
    - non-dry-run trainer binary requirement (`OPENCLAW_GO_LORA_TRAINER_BIN`).
    - trainer args/timeout envs:
      - `OPENCLAW_GO_LORA_TRAINER_ARGS`
      - `OPENCLAW_GO_LORA_TRAINER_TIMEOUT_MS`
    - manifest persistence and real process execution/log tail capture for non-dry-run jobs.
- Added regression/integration tests:
  - `TestEdgeVoiceTranscribeUsesTinyWhisperWhenConfigured`
  - `TestEdgeEnclaveProveUsesAttestationBinaryWhenConfigured`
  - `TestEdgeFinetuneRunRequiresTrainerWhenDryRunDisabled`
  - `TestEdgeFinetuneRunExecutesTrainerWhenConfigured`
- Validation completed (Dockerized Go toolchain):
  - `/usr/local/go/bin/gofmt -w ...`
  - `/usr/local/go/bin/go test ./...`
  - `/usr/local/go/bin/go vet ./...`

### Post-v2 Continuation (Issue #7) - Slice 1: Homomorphic Contract Parity Hardening

- Hardened `edge.homomorphic.compute` ciphertext contract behavior toward Rust parity:
  - ciphertext mode now rejects missing `keyId`.
  - ciphertext mode now rejects unsupported operations (only `sum|count|mean`).
  - ciphertext mode now rejects empty ciphertext sets.
  - ciphertext `mean` now requires `revealResult=true`.
- Preserved existing plaintext fallback flow for backward-compatible non-ciphertext calls.
- Added integration test coverage:
  - `TestEdgeHomomorphicCipherValidationParity`.
- Validation completed (Dockerized Go toolchain):
  - `/usr/local/go/bin/gofmt -w ...`
  - `/usr/local/go/bin/go test ./...`
  - `/usr/local/go/bin/go vet ./...`

### Post-v2 Continuation (Issue #7) - Slice 2: Security Audit Telemetry/Attestation Parity Expansion

- Expanded security audit findings to cover telemetry + attestation posture:
  - `security.edr_telemetry.unset`
  - `security.edr_telemetry.stat_failed`
  - `security.edr_telemetry.is_dir`
  - `security.attestation.expected_sha_unset`
  - `security.attestation.report_path_unset`
- Expanded `--security-audit --fix` remediation to set/normalize:
  - `security.edr_telemetry_path`
  - `security.edr_telemetry_max_age_secs`
  - `security.edr_telemetry_risk_bonus`
  - `security.attestation_report_path`
  - `security.attestation_mismatch_risk_bonus`
- Added test coverage:
  - `TestRunReportsTelemetryAndAttestationPostureFindings`
  - remediation assertions for telemetry/attestation path population.
- Validation completed (Dockerized Go toolchain):
  - `/usr/local/go/bin/gofmt -w ...`
  - `/usr/local/go/bin/go test ./...`
  - `/usr/local/go/bin/go vet ./...`

### Post-v2 Continuation (Issue #7) - Slice 3: Enclave + Finetune Execution Parity Polish

- Enclave proof contract hardening:
  - `edge.enclave.prove` now returns `-32602` when statement/challenge input is missing (Rust-parity requirement).
  - `edge.enclave.status` now includes deeper runtime/attestation metadata surfaces (`runtime`, `attestationInfo`) while preserving existing compatibility fields.
- Finetune execution depth polishing:
  - execution payload now includes explicit `status` (`completed|failed|timeout`).
  - job payload now includes `statusReason`.
  - added integration coverage for non-dry-run failure and timeout flows.
- Added/expanded integration tests:
  - `TestEdgeValidationRejectsMissingRequiredInputs` now covers missing `edge.enclave.prove` statement.
  - `TestEdgeFinetuneRunReportsExecutionFailure`
  - `TestEdgeFinetuneRunReportsExecutionTimeout`
- Validation completed (Dockerized Go toolchain):
  - `/usr/local/go/bin/gofmt -w ...`
  - `/usr/local/go/bin/go test ./...`
  - `/usr/local/go/bin/go vet ./...`

### Post-v2 Continuation (Issue #8) - Slice 1: Session Lifecycle Runtime Parity Depth

- Replaced synthetic compat behavior for session lifecycle methods with runtime mutations:
  - `sessions.delete` now removes session registry entries, state entries, and session memory history (and preserves tombstone semantics).
  - `sessions.reset` now clears session tombstones and purges session-scoped state/history.
  - `sessions.compact` now executes real memory trimming through store compaction APIs.
- Added underlying runtime data-plane support:
  - memory store: `Trim(limit)` and `RemoveSession(sessionID)` with persistence-safe snapshots.
  - state store: `Delete(sessionID)` lifecycle operation.
  - session registry: `Delete(sessionID)` lifecycle operation.
- Expanded tests:
  - memory/store tests for trim/remove persistence.
  - state store delete lifecycle test.
  - gateway session registry delete test.
  - integration tests for `sessions.delete`, `sessions.reset`, and `sessions.compact` end-to-end behavior.
- Validation completed (Dockerized Go toolchain):
  - `/usr/local/go/bin/gofmt -w ...`
  - `/usr/local/go/bin/go test ./...`
  - `/usr/local/go/bin/go vet ./...`

### Post-v2 Continuation (Issue #8) - Slice 2: Operational Status + Update Lifecycle Depth

- Removed residual scaffold marker from runtime status contracts:
  - `status.phase` now reports `phase-8-cutover-ready`.
- Expanded `update.run` from static queued envelope into lifecycle-tracked update jobs:
  - queue/run/apply/finalize status progression with transition counters and progress.
  - failure path support via simulation toggle and env gate.
  - deterministic job metadata (`jobId`, `targetVersion`, `phase`, `progress`, timestamps, release notes).
- Expanded `poll` behavior to support update lifecycle polling:
  - `poll(jobId=...)` now resolves update job state directly.
  - generic poll now includes update job summaries (`updateCount`, `updateJobs`, `hasUpdateJobs`) in addition to event items.
- Added integration coverage:
  - status phase marker validation.
  - `update.run` + `poll` lifecycle progression and completion assertions.
- Validation completed (Dockerized Go toolchain):
  - `/usr/local/go/bin/gofmt -w ...`
  - `/usr/local/go/bin/go test ./...`
  - `/usr/local/go/bin/go vet ./...`

### Post-v2 Continuation (Issue #9) - Slice 1: Residual Simulated/Scaffold Semantics Cleanup

- Enclave runtime semantics hardened:
  - replaced default `simulated-enclave` mode paths with deterministic enclave mode resolution (`tpm|sgx|sev|software-attestation`) driven by env capability signals and optional explicit mode override.
  - `edge.enclave.status` and `edge.enclave.prove` now share resolved active mode/signals without simulated labels.
- Voice fallback semantics hardened:
  - replaced `simulated` transcription labels/content with deterministic local heuristic semantics (`source=local-heuristic`, heuristic transcript wording).
- Gateway wording cleanup:
  - removed residual scaffold wording from method-not-implemented error message.
  - wasm marketplace builder metadata renamed from `scaffoldHints` to `builderHints`.
- Added/expanded integration coverage:
  - status/voice assertions now validate local heuristic source.
  - enclave status/proof tests assert non-simulated mode semantics and override behavior.
- Validation completed (Dockerized Go toolchain):
  - `/usr/local/go/bin/gofmt -w ...`
  - `/usr/local/go/bin/go test ./...`
  - `/usr/local/go/bin/go vet ./...`

### Post-v2 Continuation (Issue #10) - Slice 1: Compat Dispatch Fallback Elimination

- Removed generic compat success fallback path:
  - `handleCompatMethod` default branch now returns strict `-32601` (`compat method not implemented`) instead of synthetic `compat-fallback` success envelopes.
- Added supported-method coverage guard:
  - expanded full-method dispatch test now fails if any supported method resolves to `status=compat-fallback`.
  - preserves strict expectation that all advertised methods are explicitly wired.
- Validation completed (Dockerized Go toolchain):
  - `/usr/local/go/bin/gofmt -w ...`
  - `/usr/local/go/bin/go test ./...`
  - `/usr/local/go/bin/go vet ./...`

### Post-v2 Continuation (Issue #11) - Slice 1: Tool Runtime Family-Depth Expansion

- Expanded Go runtime to accept Rust-style top-level tool families:
  - `read`, `write`, `edit`, `apply_patch`, `exec`, `process`, `gateway`, `sessions`, `message`, `browser`, `canvas`, `nodes`, `wasm`, `routines`, `system`.
- Added message runtime lifecycle behavior with in-memory state:
  - send/poll/read/edit/delete/react/reactions/search actions.
  - deterministic message ids, reaction mutation, and bounded runtime retention.
- Added sessions runtime lifecycle behavior backed by runtime message state:
  - list/history/reset/usage actions with per-session aggregation and token counting.
- Added family routing for browser/process/nodes/canvas/wasm/routines/gateway surfaces so top-level tool contracts execute concrete behavior rather than unsupported errors.
- Expanded runtime catalog to advertise both family-level and dot-tool surfaces.
- Added test coverage:
  - alias/family routing validation (`read/write/edit/browser`).
  - message + sessions lifecycle integration (`send/react/search/usage/reset`).
  - gateway/canvas/wasm/routines family smoke coverage.
- Validation completed (Dockerized Go toolchain):
  - `/usr/local/go/bin/gofmt -w ...`
  - `/usr/local/go/bin/go test ./...`
  - `/usr/local/go/bin/go vet ./...`

### Post-v2 Continuation (Issue #12) - Slice 1: Provider/Auth Catalog Widening (Qwen Copaw Compatibility)

- Expanded provider alias normalization in Go gateway parity surfaces:
  - added Rust-aligned alias handling for major auth/runtime providers (`openai/chatgpt`, `codex`, `claude`, `gemini`, `qwen`, `minimax`, `kimi`, `opencode`, `zhipuai`, `zai`, `inception`).
  - added explicit Qwen Copaw compatibility aliases (`copaw`, `qwen-copaw`, `qwen-agent`) mapping to canonical `qwen`.
- Added OAuth provider catalog contract in Go gateway compat layer:
  - canonical provider metadata now includes `id`, `providerId`, `name/displayName`, `aliases`, `verificationUrl`, `supportsBrowserSession`, and `apiKeyConfigured`.
  - wired expanded catalog into `auth.oauth.providers`.
- Expanded Telegram auth provider visibility:
  - `/auth providers` now uses the catalog payload (instead of a minimal hard-coded list), preserving API key configured state per canonical provider.
- Expanded browser login provider routing:
  - login manager now normalizes provider aliases before session creation and resolves provider-specific verification URIs for expanded providers, including Qwen Copaw aliases.
- Added test coverage:
  - provider alias normalization includes Copaw/Qwen mappings.
  - auth provider payload verifies Qwen alias metadata and configured-key propagation from Copaw alias.
  - compat `auth.oauth.providers` returns expanded catalog.
  - `/auth start` parser accepts Copaw alias and resolves to canonical Qwen provider.
  - browser login verification URI tests cover extended provider aliases.
- Validation completed (Dockerized Go toolchain):
  - `/usr/local/go/bin/gofmt -w ...`
  - `/usr/local/go/bin/go test ./...`
  - `/usr/local/go/bin/go vet ./...`

### Post-v2 Continuation (Issue #13) - Slice 1: Model Catalog Widening + Slash-Scoped Resolution

- Expanded compat model catalog beyond ChatGPT-only defaults to cover parity-critical provider models:
  - Codex: `gpt-5.3-codex`.
  - Qwen: `qwen3.5-397b-a17b`, `qwen3.5-plus`, `qwen3.5-flash`.
  - Inception: `inception/mercury`.
  - OpenCode: `opencode/glm-5-free`, `opencode/kimi-k2.5-free`.
  - OpenRouter: `openrouter/qwen/qwen3-coder:free`, `openrouter/google/gemini-2.0-flash-exp:free`.
- Hardened `resolveModelChoice` matching semantics:
  - supports deterministic matching for slash-scoped provider/model IDs by comparing normalized forms of catalog IDs.
  - supports alias-based matching from model descriptor `aliases` arrays.
- Added parity-focused test coverage:
  - slash-scoped model IDs resolve directly.
  - slash-scoped model aliases resolve correctly.
  - provider-scoped model listing includes expanded Qwen/OpenRouter catalog entries.
- Validation completed (Dockerized Go toolchain):
  - `/usr/local/go/bin/gofmt -w ...`
  - `/usr/local/go/bin/go test ./...`
  - `/usr/local/go/bin/go vet ./...`

### Post-v2 Continuation (Issue #14) - Slice 1: Provider-Aware Browser Runtime Bridge

- Expanded runtime browser completion payload contract:
  - `browser.request` completion path now passes canonical `provider` in payload alongside `model/messages`.
  - defaults remain backward-compatible (`provider=chatgpt`, `model=gpt-5.2`) when provider is omitted.
- Added provider alias normalization for browser runtime surfaces:
  - includes Rust-aligned aliases and explicit Qwen Copaw support (`copaw`, `qwen-copaw`, `qwen-agent` -> `qwen`).
- Expanded browser runtime response metadata:
  - completion responses now report normalized provider in `provider` with explicit bridge marker (`bridge=browser`) rather than fixed provider label.
  - non-completion browser request responses also include normalized provider metadata.
- Added test coverage:
  - default non-completion browser provider behavior (`chatgpt`).
  - completion payload provider passthrough (`copaw` input emits `qwen` payload provider).
  - completion response provider metadata asserts normalized provider.
  - alias normalization unit checks for Copaw and Codex alias paths.
- Validation completed (Dockerized Go toolchain):
  - `/usr/local/go/bin/gofmt -w ...`
  - `/usr/local/go/bin/go test ./...`
  - `/usr/local/go/bin/go vet ./...`

### Post-v2 Continuation (Issue #15) - Slice 1: `models.list` Contract Depth

- Replaced compat `models.list` inline stub with structured handler semantics:
  - strict parameter validation with deterministic `-32602` for unknown fields.
  - provider-scoped filtering with canonical alias normalization (including `copaw -> qwen`).
  - deterministic model ordering (`provider`, then `id`) for stable list outputs.
  - provider summary metadata in payload (`providers`, `providerRequested`).
- Added parity-focused tests:
  - invalid params rejection for `models.list`.
  - provider filter behavior via Copaw alias ensuring returned models are Qwen-scoped.
- Validation completed (Dockerized Go toolchain):
  - `/usr/local/go/bin/gofmt -w ...`
  - `/usr/local/go/bin/go test ./...`
  - `/usr/local/go/bin/go vet ./...`

### Post-v2 Continuation (Issue #16) - Slice 1: `auth.oauth.providers` Contract Depth

- Replaced inline compat `auth.oauth.providers` payload path with structured handler semantics:
  - strict parameter validation with deterministic `-32602` for unknown fields.
  - provider-scoped filtering using canonical alias normalization (`openai-codex -> codex`, etc.).
  - deterministic provider ordering and explicit metadata fields (`count`, `providerRequested`).
- Added parity-focused tests:
  - invalid params rejection for `auth.oauth.providers`.
  - alias-based provider filter behavior (`openai-codex` request resolves to canonical `codex` entry).
- Validation completed (Dockerized Go toolchain):
  - `/usr/local/go/bin/gofmt -w ...`
  - `/usr/local/go/bin/go test ./...`
  - `/usr/local/go/bin/go vet ./...`

### Post-v2 Continuation (Issue #17) - Slice 1: `auth.oauth.import` Provider Hardening

- Hardened `auth.oauth.import` compatibility contract:
  - strict unknown-field validation with deterministic `-32602`.
  - provider now resolves through OAuth provider catalog with alias canonicalization.
  - unknown providers now fail fast with explicit known-provider hints.
- Improved import response metadata:
  - added canonical provider fields: `providerId`, `providerDisplayName`.
  - preserved existing `login` payload and import flow behavior.
- Model defaults during import now use provider-scoped defaults when model is omitted.
- Added parity-focused tests:
  - unknown provider rejection path.
  - alias canonicalization path (`openai-codex` resolves to `codex`) with authorized login assertion.
- Validation completed (Dockerized Go toolchain):
  - `/usr/local/go/bin/gofmt -w ...`
  - `/usr/local/go/bin/go test ./...`
  - `/usr/local/go/bin/go vet ./...`

### Post-v2 Continuation (Issue #18) - Slice 1: Provider-Specific Browser Bridge Endpoint Routing

- Expanded browser runtime bridge options:
  - added provider-scoped endpoint map support (`EndpointByProvider`) with canonical provider alias normalization.
  - preserved backward-compatible fallback to default bridge endpoint (`Endpoint`) when no provider override is configured.
- Runtime completion path now resolves endpoint per canonical provider before dispatch:
  - supports Copaw/Qwen alias normalization in endpoint selection.
  - completion response metadata now includes selected endpoint for observability.
- Added parity-focused runtime tests:
  - provider-scoped endpoint override is selected for Qwen/Copaw requests.
  - default endpoint remains unused when provider override is configured.
- Validation completed (Dockerized Go toolchain):
  - `/usr/local/go/bin/gofmt -w ...`
  - `/usr/local/go/bin/go test ./...`
  - `/usr/local/go/bin/go vet ./...`

### Post-v2 Continuation (Issue #19) - Slice 1: Telegram Auth Code Extraction Robustness

- Expanded `/auth complete` callback code extraction semantics:
  - supports query-based code/token extraction (`openclaw_code`, `code`, `device_code`, `auth_code`, `token`, `oauth_token`).
  - supports fragment-based extraction (`#code=...`, `#token=...`, raw fragment tokens).
  - supports path-style callback tokens (e.g., `/oauth/complete/<code>`).
  - preserves existing plain-code passthrough behavior.
- Added parser coverage for provider alias completion flows:
  - Copaw alias in `parseAuthCompleteScope` resolves to canonical `qwen`.
- Added unit tests:
  - query/fragment/path extraction matrix.
  - Copaw completion scope parsing + extracted code assertion.
- Validation completed (Dockerized Go toolchain):
  - `/usr/local/go/bin/gofmt -w ...`
  - `/usr/local/go/bin/go test ./...`
  - `/usr/local/go/bin/go vet ./...`

### Post-v2 Continuation - v2.5.0-go Parity Closure + Release Cut

- Re-ran full parity validation gate sequence (`CP0` through `CP9`) and refreshed generated parity artifacts:
  - `parity/generated/method-surface-diff.json`
  - `parity/generated/parity-scoreboard.json`
  - `parity/generated/parity-scoreboard.md`
  - `parity/method-surface-report.md`
  - `parity/generated/cp*/...` summaries, metrics, and logs
- Confirmed deterministic parity closure:
  - Rust RPC methods: `133`
  - Go RPC methods: `133`
  - Missing in Go: `0`
  - Extra in Go: `0`
  - Scoreboard: `22/22` implemented, `0` partial/deferred/not started
- Built cross-platform release artifacts in Docker (`go1.25.7`) via build matrix:
  - `openclaw-go-windows-amd64.exe`
  - `openclaw-go-linux-amd64`
  - `openclaw-go-android-arm64`
  - `SHA256SUMS.txt`

### Post-v2 Continuation - v2.6.0-go Multi-Channel Adapter Breadth Upgrade

- Expanded Go runtime channel adapter surfaces to incorporate broad multi-channel coverage pattern:
  - added adapters: `whatsapp`, `discord`, `slack`, `feishu`, `qq`, `wework`, `dingtalk`, `infoflow`, `googlechat`, `teams`, `matrix`, `signal`, `line`, `mattermost`, `imessage`.
- Added reusable channel adapter config model in runtime config:
  - `enabled`, `token`, `default_target`, `webhook_url`, `auth_header`, `auth_prefix`, `headers`.
- Implemented generic adapter driver behavior:
  - disabled-mode rejection,
  - token-ready delivery mode,
  - webhook POST delivery mode with auth header/prefix and static header overrides,
  - status/logout state integration.
- Preserved Telegram command/auth/model/TTS runtime flow integration while widening channel breadth.
- Added test coverage:
  - broad channel status catalog,
  - webhook delivery and auth header propagation,
  - token-ready adapter mode,
  - disabled-channel send rejection,
  - config validation for enabled adapters without token/webhook.
- Validation completed (Dockerized Go toolchain):
  - `/usr/local/go/bin/gofmt -w ...`
  - `/usr/local/go/bin/go test ./...`
  - `/usr/local/go/bin/go vet ./...`
- Built release artifacts in Docker:
  - `openclaw-go-windows-amd64.exe`
  - `openclaw-go-linux-amd64`
  - `openclaw-go-android-arm64`
  - `SHA256SUMS.txt`

### Post-v2 Continuation - v2.6.1-go Go-Only Mainline Cleanup

- Created immutable Rust archive refs before cleanup:
  - branch: `rust-archive-pre-go-only-v2.6.0-go`
  - tag: `rust-archive-pre-go-only-v2.6.0-go`
- Removed Rust payload from `main`:
  - Rust runtime source tree (`src/*.rs`)
  - Rust manifests/toolchain files (`Cargo.toml`, `Cargo.lock`, `rust-toolchain.toml`, `deny.toml`, `openclaw-rs.example.toml`)
  - Rust-only CI/workflow + release-plan issue template
  - Rust-only deploy parity stack and parity gate scripts
  - Rust-era planning docs that were tied to pre-Go runtime shipping
- Updated mainline docs to reflect Go-only runtime path and archive ref.

### Post-v2 Continuation - v2.7.0-go Dockerized Deployment Profile Expansion

- Added Dockerized runtime deployment surfaces for the Go-only repo:
  - root `Dockerfile` to build `openclaw-go` Linux runtime image.
  - `docker-compose.yml` for core gateway service.
  - `docker-compose.bridge.yml` overlay for browser-bridge sidecar integration.
- Added operator bootstrap artifacts:
  - `.env.example`
  - `prepare-env.ps1`
  - `prepare-env.sh`
- Updated docs/version references for next release cut:
  - `README.md`
  - `go-agent/README.md`
  - `CHANGELOG.md`
- Stabilized gateway finetune timeout regression test for deterministic containerized gate behavior:
  - `go-agent/internal/gateway/server_test.go` (`TestEdgeFinetuneRunReportsExecutionTimeout`).
- Validation completed:
  - `go test ./...`
  - `go vet ./...`
  - `docker compose config`
  - `docker compose -f docker-compose.yml -f docker-compose.bridge.yml config`
- Built release assets:
  - `dist/release-v2.7.0-go-assets/openclaw-go-windows-amd64.exe`
  - `dist/release-v2.7.0-go-assets/openclaw-go-linux-amd64`
  - `dist/release-v2.7.0-go-assets/openclaw-go-android-arm64`
  - `dist/release-v2.7.0-go-assets/SHA256SUMS.txt`

### Post-v2 Continuation - v2.8.0-go TTS Runtime Depth + Telegram Contract Alignment

- Opened tracking issue for this slice:
  - `https://github.com/adybag14-cyber/openclaw-go-port/issues/20`
- Expanded compat TTS runtime behavior:
  - unified `tts.providers` catalog with runtime availability/reason metadata.
  - added `kittentts` binary discovery (`OPENCLAW_GO_KITTENTTS_BIN`, `OPENCLAW_GO_TTS_KITTENTTS_BIN`, `PATH`).
  - added `kittentts` execution adapter support with env-driven args (`OPENCLAW_GO_KITTENTTS_ARGS`) and timeout control (`OPENCLAW_GO_KITTENTTS_TIMEOUT_MS`).
  - upgraded `tts.convert` contract with strict real-audio mode (`requireRealAudio`), structured audio metadata, and deterministic synthetic WAV fallback behavior.
- Aligned Telegram `/tts` command surfaces with compat contracts:
  - `/tts providers` now uses compat provider catalog directly.
  - `/tts status|on|off|provider` includes availability/reason metadata.
  - `/tts say` now surfaces `realAudio`, `outputFormat`, `fallback`, and engine metadata.
- Added/expanded tests:
  - `go-agent/internal/gateway/compat_tts_test.go`
  - `go-agent/internal/gateway/server_test.go` TTS assertions for provider presence (`kittentts`) and metadata depth.
- Validation completed (Dockerized Go toolchain):
  - `/usr/local/go/bin/gofmt -w ./internal/gateway/compat.go ./internal/gateway/telegram_commands.go ./internal/gateway/server_test.go ./internal/gateway/compat_tts_test.go`
  - `/usr/local/go/bin/go test ./...`
  - `/usr/local/go/bin/go vet ./...`

### Post-v2 Continuation - v2.9.0-go Telegram Runtime Live Delivery + Inbound Reply Loop

- Opened tracking issue for this slice:
  - `https://github.com/adybag14-cyber/openclaw-go-port/issues/21`
- Replaced Telegram stub driver behavior with real Bot API delivery:
  - `sendMessage` HTTP path is now used for `channels.telegram` sends.
  - channel status now reflects live delivery connectivity and last API error surfaces.
  - added test/proxy override support through `OPENCLAW_GO_TELEGRAM_API_BASE`.
- Added gateway background Telegram runtime:
  - long-poll loop via `getUpdates` starts automatically when bot token is configured.
  - inbound slash command messages are routed through existing Telegram command handlers and delivered back to the originating chat.
  - inbound plain-text messages are bridged through browser completion runtime and replies are posted back to Telegram.
- Preserved non-Telegram channel behavior:
  - existing generic multi-channel webhook/token-ready adapters remain unchanged and operational.
- Added/expanded tests:
  - `go-agent/internal/channels/registry_test.go` now validates real Telegram send semantics through mocked API.
  - new `go-agent/internal/gateway/telegram_runtime_test.go` covers inbound command handling and inbound plain-text bridged replies.
  - `go-agent/internal/gateway/server_test.go` updated to use mocked Telegram API in channel send integration flow.
- Validation completed (Dockerized Go toolchain):
  - `/usr/local/go/bin/gofmt -w ./internal/channels/telegram_driver.go ./internal/channels/registry_test.go ./internal/gateway/server.go ./internal/gateway/server_test.go ./internal/gateway/telegram_runtime.go ./internal/gateway/telegram_runtime_test.go`
  - `/usr/local/go/bin/go test ./...`
  - `/usr/local/go/bin/go vet ./...`

### Post-v2 Continuation - v2.10.0-go Telegram Reliability Hardening + Expanded Platform Matrix

- Hardened Telegram delivery path for real-world message sizes:
  - `channels.telegram` now auto-chunks oversized outbound replies to stay within Telegram message limits.
  - multipart sends keep explicit metadata (`chunked`, `chunkCount`, `messageIds`) for auditability.
- Expanded Telegram command handling compatibility:
  - supports `/command@bot` format commonly used in group chats.
  - added explicit `/start` and `/help` responses to reduce first-message failure cases.
- Added/expanded tests:
  - multipart Telegram send regression (`registry_test`).
  - `/tts@bot` command handling regression (`telegram_runtime_test`).
  - `/start` command reply regression (`telegram_runtime_test`).
- Expanded release build matrix for broader deployment parity:
  - `windows/amd64`, `windows/arm64`
  - `linux/amd64`, `linux/arm64`
  - `darwin/amd64`, `darwin/arm64`
  - `android/arm64`
- Validation completed (Dockerized Go toolchain):
  - `/usr/local/go/bin/gofmt -w ./internal/channels/telegram_driver.go ./internal/channels/registry_test.go ./internal/gateway/telegram_commands.go ./internal/gateway/telegram_runtime_test.go`
  - `/usr/local/go/bin/go test ./...`
  - `/usr/local/go/bin/go vet ./...`

### Post-v2 Continuation - Provider Parity Closure + Legacy WS Bridge Compatibility

- Closed Rust->Go model provider parity gaps in Go compat model catalog:
  - added Rust-matched providers in `models.list`/Telegram model surfaces:
    - `claude`
    - `groq`
    - `zhipuai`
    - `zai`
- Added regression coverage:
  - `TestModelCatalogIncludesRustProviderParitySet`
- Added websocket root compatibility route in gateway handler:
  - supports legacy bridge clients that dial `ws://host:port` without `/ws`.
  - regression test added: `TestWebSocketRPCDispatchRootCompatibility`.
- Validation completed (Dockerized Go toolchain):
  - `/usr/local/go/bin/gofmt -w ./internal/gateway/compat.go ./internal/gateway/provider_catalog_test.go ./internal/gateway/server.go ./internal/gateway/server_test.go`
  - `/usr/local/go/bin/go test ./...`
  - `/usr/local/go/bin/go vet ./...`

### Post-v2 Continuation - Lightpanda Browser Bridge Backend (Optional CDP Path)

- Added Lightpanda-compatible browser bridge execution path in Node bridge helpers used by Go runtime operator flows:
  - `scripts/chatgpt-browser-bridge.mjs`:
    - added CDP endpoint support via `OPENCLAW_CHATGPT_LIGHTPANDA_WS_ENDPOINT`.
    - added engine routing control via `OPENCLAW_CHATGPT_BRIDGE_ENGINES`.
    - added Lightpanda Playwright/Puppeteer connection attempts with fallback to local Playwright/Puppeteer.
    - extended `/health` payload with bridge engine order and Lightpanda readiness fields.
  - `scripts/chatgpt-browser-auth.mjs`:
    - added `--engine lightpanda` and `--lightpanda-endpoint` support.
    - added Lightpanda Playwright/Puppeteer auth-capture attempts and structured error shaping.
- Updated operator docs and compose wiring:
  - `README.md`: Lightpanda environment examples and explicit fallback behavior notes.
  - `docker-compose.bridge.yml`: plumbed Lightpanda endpoint + engine-order env passthrough.
- Validation completed:
  - `node --check scripts/chatgpt-browser-auth.mjs`
  - `node --check scripts/chatgpt-browser-bridge.mjs`
  - bridge `/health` smoke in both default and Lightpanda-configured modes.

### Post-v2 Continuation - v2.10.2-go Provider-Aware Keyless Bridge Hardening

- Hardened gateway browser auth gating to only require active login sessions for browser-session providers:
  - `chatgpt`
  - `qwen`
  - `zai`
  - `inception`
- Expanded provider inference and alias normalization depth:
  - model-prefix and URL-host provider detection in gateway/browser bridge paths.
  - `glm/glm5/glm-5 -> zai`
  - `mercury/mercury2/mercury-2 -> inception`
- Added configurable web-login TTL:
  - config: `runtime.web_login_ttl_minutes`
  - env: `OPENCLAW_GO_WEB_LOGIN_TTL_MINUTES`
  - default: `1440` minutes
- Deep diagnostics parity improvements:
  - security audit deep browser probe now performs `/health` HTTP checks and includes status/body details.
  - doctor deep checks now detect conflicting active systemd units across user/system scopes for runtime and bridge services.
- Browser bridge script improvements:
  - provider profiles for `chatgpt`, `qwen`, `zai`, `inception`.
  - `/health` now reports provider catalog plus engine and Lightpanda readiness metadata.
- Validation completed:
  - `node --check scripts/chatgpt-browser-bridge.mjs`
  - `go fmt ./...` (Dockerized)
  - `go test ./...` (Dockerized)
  - `go vet ./...` (Dockerized)
  - bridge `/health` smoke in default and Lightpanda-configured modes.
