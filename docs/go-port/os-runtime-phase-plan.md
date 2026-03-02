# OpenClaw Go OS Runtime Track (Issue #22)

Issue: <https://github.com/adybag14-cyber/openclaw-go-port/issues/22>

## Release Gate Rule

No release for this OS-runtime track is allowed until Docker stability gating passes for the targeted phase scope.

Required gate command:

- PowerShell: `./scripts/docker-stability-gate.ps1`
- POSIX: `WITH_BRIDGE=0 ./scripts/docker-stability-gate.sh`

For bridge-enabled milestones:

- PowerShell: `./scripts/docker-stability-gate.ps1 -WithBridge`
- POSIX: `WITH_BRIDGE=1 ./scripts/docker-stability-gate.sh`

## Phase 0 - Architecture and Gate Setup

Scope:
- define hosted runtime targets and bootable path architecture.
- define stability criteria and probe cadence.
- add automated Docker stability gate scripts.
- map release decision to gate pass/fail.

Exit criteria:
- documented architecture split (`hosted` vs `bootable`).
- stability scripts committed and reproducible.
- issue checklist updated with gate rules.

## Phase 1 - Docker Stability Baseline

Scope:
- run baseline stability sweeps for core runtime (`docker-compose.yml`).
- run bridge-enabled sweeps for browser path (`docker-compose + bridge`).
- capture pass/fail evidence and failure signatures.
- only proceed to release packaging after passing sweeps.

Exit criteria:
- zero failed probes across agreed probe window for core runtime.
- zero failed probes across agreed probe window for bridge runtime.
- findings documented in phase log and issue.

## Target Matrix for this Track

- hosted mode: Linux, Windows, macOS, Android/Termux, Raspberry Pi.
- boot mode (future phases): Linux kernel + initramfs + OpenClaw Go service bootstrap.

Phase 0/1 focus is Docker stability and release gating, not boot image delivery yet.

## Current Status (2026-03-02)

- Phase 0: complete.
- Phase 1A (core profile): pass.
  - `./scripts/docker-stability-gate.ps1 -Probes 4 -IntervalSeconds 5 -StartupTimeoutSeconds 120`
- Phase 1B (bridge profile): blocked by upstream image pull failure.
  - `mcr.microsoft.com/playwright:v1.52.0-noble` returned manifest `EOF` across retries.
  - gate scripts now include bridge image pull retries and startup warmup to reduce false failures.

Release for this track remains blocked until Phase 1B passes.
