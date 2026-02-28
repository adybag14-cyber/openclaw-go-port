# OpenClaw Go `v2.0.1-go`

## Summary

`v2.0.1-go` is the post-`v2.0.0-go` continuation patch release that closes issue `#3` depth-parity work:

- Security audit depth expansion:
  - gateway/public bind checks
  - loop-guard/risk-threshold posture checks
  - browser bridge endpoint exposure checks
  - policy bundle file/stat/read/parse checks
- Deep security probes expanded to include:
  - gateway probe
  - browser bridge probe
  - policy bundle probe
- Doctor diagnostics expanded with structured `doctor.checks` output:
  - auth secret readiness
  - endpoint scope posture
  - state persistence posture
  - policy bundle posture
  - loop/risk posture
  - deep probe outcomes
  - binary availability (`docker`, `wasmtime`)
- Added parity corpus-style audit tests and gateway deep-audit integration coverage.

## Artifacts

- `dist/release-v2.0.1-go-assets/openclaw-go-windows-amd64.exe`
- `dist/release-v2.0.1-go-assets/openclaw-go-linux-amd64`
- `dist/release-v2.0.1-go-assets/openclaw-go-android-arm64`
- `dist/release-v2.0.1-go-assets/SHA256SUMS.txt`

## Validation Gates

Dockerized Go toolchain:

- `gofmt -w ./cmd ./internal`
- `go test ./...`
- `go vet ./...`

Binary smoke checks:

- Windows artifact:
  - `--doctor` includes `doctor.checks`
  - `--list-methods` reports `133` methods
- Linux artifact:
  - `GET /health` returns `status=ok`
  - `POST /rpc` `status` returns RPC `ok=true`

## Checksums

See `dist/release-v2.0.1-go-assets/SHA256SUMS.txt`.
