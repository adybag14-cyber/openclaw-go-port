# OpenClaw Go v2.0.3 Release Notes

Date: 2026-03-01
Tag: `v2.0.3-go`

## Scope

This release completes the requested parity slices:

1. Edge handler depth upgrades (router, acceleration, WASM marketplace, mesh, enclave, homomorphic, finetune status/run, trust, revenue preview, finetune cluster plan, alignment evaluation).
2. Telegram `/model` provider/mode matrix parity (`list <provider>`, `<provider>/<model>`, `<provider> <model>`, provider default selection, and custom override metadata).

## Key Changes

- Added provider-aware Telegram model selection state:
  - per-target provider + model persistence
  - provider alias normalization (including `openai -> chatgpt`)
  - provider catalog listing and default provider model resolution
- Expanded Telegram model command contracts:
  - `/model list`
  - `/model list <provider>`
  - `/model <provider>/<model>`
  - `/model <provider> <model>`
  - provider-only default selection
  - custom model override annotations
- Enriched edge RPC payloads while preserving legacy compatibility keys used by existing clients/tests.
- Added parity-focused integration tests for:
  - Telegram provider-mode model flows
  - Rich edge payload contracts across the updated methods

## Validation

Executed with Dockerized Go toolchain:

- `go test ./...` (pass)
- `go vet ./...` (pass)

## Artifacts

Expected release artifacts:

- `openclaw-go-windows-amd64.exe`
- `openclaw-go-linux-amd64`
- `openclaw-go-android-arm64`
- `SHA256SUMS.txt`
