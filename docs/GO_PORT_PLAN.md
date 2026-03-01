# OpenClaw Go Port Master Plan

> Status update (2026-03-01): Rust cutover is complete, and Rust code has been archived from `main`.
> Archive ref: `rust-archive-pre-go-only-v2.6.0-go`.

## Objective

Port the full `openclaw-rust` runtime to Go with end-to-end parity, no feature regressions, and production-grade validation gates.
This objective is now complete on `main`, with ongoing Go-only hardening and feature expansion.

## Critical Points

1. Contract parity is mandatory
- Preserve RPC method names, payload fields, default values, and error envelopes.
- Keep behavior-compatible auth, session routing, bridge flows, and transport semantics.

2. Security posture must not regress
- Preserve current hardening level and secure defaults from Rust.
- Keep policy, credential, and guard surfaces at least equivalent.

3. Incremental delivery only
- Ship in narrow vertical slices (config + transport + handler + tests + docs).
- No broad untested rewrites across multiple domains in one commit.

4. Validation must be deterministic
- Every phase must include explicit test and verification commands.
- Parity artifacts and checklists must be updated as part of done criteria.

5. No v1.0 without hard evidence
- `docs/go-port/parity-matrix.md` and phase checklist must show all complete.
- Smoke + integration + replay coverage must pass on target environments.

## Phase Model

## Phase 0: Program Setup and Baseline Lock

### Scope
- Fork-equivalent repository setup.
- Planning artifacts, issue tracking, and skill workflow.
- Baseline capture of Rust module boundaries and release reference point.

### Exit Criteria
- Plan/checklist/matrix/log files exist and are committed.
- Tracking issue with checklists exists.
- Go port workspace directory exists and is wired to the fork.

## Phase 1: Go Runtime Bootstrap and Core Transport Skeleton

### Scope
- Create `go-agent` module.
- Implement minimal executable, config loader, and health/control HTTP surface.
- Establish base package boundaries that map to Rust runtime architecture.

### Exit Criteria
- `go test ./...` and `go vet ./...` pass for `go-agent`.
- Health endpoint returns deterministic JSON and HTTP 200.
- Initial phase log and parity matrix entries updated.

## Phase 2: Protocol and RPC Envelope Parity

### Scope
- Port request/response framing and RPC envelope helpers.
- Implement method registry scaffold and canonical method resolution.
- Match Rust error envelope patterns.

### Exit Criteria
- Golden tests for request parsing and RPC responses pass.
- Protocol fixtures are ported and stable.

## Phase 3: Gateway Server and Session Scheduling Core

### Scope
- Port gateway server auth modes, connection lifecycle, queueing, and scheduler primitives.
- Port session and event routing skeleton with deterministic ordering.

### Exit Criteria
- Gateway connect/status/health/control flows pass parity tests.
- Session queue behavior is validated against Rust reference scenarios.

## Phase 4: Tool Runtime and Bridge Flows

### Scope
- Port tool runtime orchestration, registry, and provider routing hooks.
- Port website bridge and browser-auth bridge control semantics.
- Preserve key operational flows used in Telegram and gateway operation.

### Exit Criteria
- End-to-end tool call path is operational in Go.
- Bridge auth flow contracts pass smoke tests.

## Phase 5: Channel Integrations and Memory Surfaces

### Scope
- Port channel abstraction and Telegram bridge behavior.
- Port in-memory and persistent memory behavior.
- Port state/session key handling and replay-sensitive logic.

### Exit Criteria
- Telegram and channel flows pass integration tests.
- Memory persistence and recall tests pass.

## Phase 6: Security and Policy Stack Port

### Scope
- Port command/prompt/tool guards, policy bundle handling, and safety-layer controls.
- Port telemetry-based risk hooks and credential injector behavior.

### Exit Criteria
- Security checks and policy outcomes match Rust baseline tests.
- No reduction in enforced controls for default profiles.

## Phase 7: Advanced Runtime Features

### Scope
- Port wasm runtime/sandbox controls, routines, and remaining edge capabilities.
- Close all remaining method parity gaps.

### Exit Criteria
- Parity matrix shows all critical features complete.
- Replay harness and advanced smoke tests pass.

## Phase 8: Cutover Readiness and v1.0 Release Candidate

### Scope
- Full regression matrix, packaging, and deployment validation (Windows/Linux/VM).
- Operational runbooks and migration docs finalized.

### Exit Criteria
- 100% parity tracked as complete in docs and issue checklist.
- Release candidate artifacts validated and signed off.
- Tag and release `v1.0.0-go`.

## Validation Strategy

1. Unit tests by package for every new capability.
2. Golden/fixture tests for protocol and response compatibility.
3. Integration tests for gateway + bridge flows.
4. Replay tests against captured traffic/corpora where available.
5. Environment smoke tests (local + VM).

## Risk Register

1. RPC drift risk
- Mitigation: fixture-based protocol tests and explicit parity matrix fields.

2. Behavioral drift risk (scheduler/session/auth)
- Mitigation: scenario tests copied from Rust semantics and golden outcomes.

3. Hidden dependency risk
- Mitigation: phase logs include unresolved blockers and required follow-up.

4. Toolchain drift risk
- Mitigation: pin Go version in module docs and CI config when added.

## Definition of Done for v1.0

1. All phase checklists complete.
2. Parity matrix indicates no unresolved required feature gaps.
3. Validation gates pass in CI-equivalent local runs and target VM smoke.
4. Issue tracker checklist fully checked and linked to commits/PRs.
