# OpenClaw Go v2.7.0 Release Notes

Date: 2026-03-01
Tag: `v2.7.0-go`

## Scope

This release adds production-ready Docker deployment profiles and environment bootstrap tooling for the Go-only runtime.

1. Added first-class container build and compose deployment surfaces.
2. Added bootstrap scripts for `.env` and runtime config initialization.
3. Updated release docs/examples to standardize `v2.7.0-go` operations.

## Key Changes

- Added root runtime image build:
  - `Dockerfile`
- Added core compose profile:
  - `docker-compose.yml`
  - `openclaw-go` service with health check, persistent volume, port mapping, and restart policy.
- Added browser-bridge overlay profile:
  - `docker-compose.bridge.yml`
  - `openclaw-browser-bridge` Playwright container with persistent profile volume.
  - provider endpoint wiring into `openclaw-go`.
- Added environment bootstrap assets:
  - `.env.example`
  - `prepare-env.ps1`
  - `prepare-env.sh`
  - auto-populates `OPENCLAW_GO_GATEWAY_TOKEN` when missing.
- Stabilized finetune timeout regression behavior:
  - `TestEdgeFinetuneRunReportsExecutionTimeout` now uses a deterministic shell busy-loop trainer mock.
- Documentation updates:
  - `README.md`
  - `go-agent/README.md`
  - `CHANGELOG.md`

## Validation

Executed with Dockerized Go toolchain and compose validation:

- `go test ./...` (pass)
- `go vet ./...` (pass)
- `docker compose config` (pass)
- `docker compose -f docker-compose.yml -f docker-compose.bridge.yml config` (pass)
- build matrix:
  - `go-agent/scripts/build-matrix.sh 2.7.0 ../dist/release-v2.7.0-go-assets` (pass)

## Artifacts

Release assets in `dist/release-v2.7.0-go-assets`:

- `openclaw-go-windows-amd64.exe`
- `openclaw-go-linux-amd64`
- `openclaw-go-android-arm64`
- `SHA256SUMS.txt`

Checksums:

- `586c340206671fb0866eb4c7ab485aa344d55dec57945ff3efb1c7322e72e307  openclaw-go-windows-amd64.exe`
- `82c7b354b15c95e21407982fe3cbf975e6737482271c51d98f0bc9042ef31177  openclaw-go-linux-amd64`
- `04dd34039140ef981e001eea966feab2c63ef838aa8f8f8fe0a9fe07aa7381e1  openclaw-go-android-arm64`
