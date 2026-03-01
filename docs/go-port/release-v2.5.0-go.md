# OpenClaw Go v2.5.0 Release Notes

Date: 2026-03-01
Tag: `v2.5.0-go`

## Scope

This release is the full parity closure cut for the Go port:

1. Completed full CP0-CP9 parity gate pass set and refreshed parity artifacts.
2. Verified exact Rust/Go RPC method-set parity at `133/133` with zero missing and zero extra methods.
3. Published cross-platform release artifacts for Windows, Linux, and Android.

## Key Outcomes

- Full gate pass status:
  - CP0 through CP9: all passed.
  - CP8 hardening gate: `15/15` fixtures passed.
  - CP9 Docker E2E gate: `4/4` checks passed.
- Method-set parity:
  - Rust supported methods: `133`
  - Go supported methods: `133`
  - Missing in Go: `0`
  - Extra in Go: `0`
- Scoreboard status:
  - Audit Implemented: `22`
  - Audit Partial: `0`
  - Audit Deferred: `0`
  - Audit Not Started: `0`
  - Base coverage: `100%`
  - Handler coverage: `100%`

## Validation

Executed with Dockerized toolchains and parity gates:

- CP gates:
  - `parity/run-cp0-gate.ps1`
  - `parity/run-cp1-gate.ps1`
  - `parity/run-cp2-gate.ps1`
  - `parity/run-cp3-gate.ps1`
  - `parity/run-cp4-gate.ps1`
  - `parity/run-cp5-gate.ps1`
  - `parity/run-cp6-gate.ps1`
  - `parity/run-cp7-gate.ps1`
  - `parity/run-cp8-gate.ps1`
  - `parity/run-cp9-gate.ps1`
- Dockerized build matrix:
  - `go-agent/scripts/build-matrix.sh 2.5.0 ../dist/release-v2.5.0-go-assets`

## Artifacts

Release assets in `dist/release-v2.5.0-go-assets`:

- `openclaw-go-windows-amd64.exe`
- `openclaw-go-linux-amd64`
- `openclaw-go-android-arm64`
- `SHA256SUMS.txt`

Checksums:

- `9ea73b03540cfb306aed81eef3ca41c28c97b7a985a2e949cb6e4557f9920966  openclaw-go-windows-amd64.exe`
- `bc7ed13e101ab540642a400bb1716d8d336c52b5069c6bba5334b68550d1ea07  openclaw-go-linux-amd64`
- `e0e27ee2e1bf74ce66e65f351f784dc7849a1ff902ba1d7cc501e32c8315426f  openclaw-go-android-arm64`
