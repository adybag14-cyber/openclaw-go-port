# OpenClaw Go `v2.0.2-go`

## Summary

`v2.0.2-go` continues post-v2 parity closure with Rust-style security audit remediation support.

- Added `--security-audit --fix` remediation flow.
- Added fix-result payload in audit output (`securityAudit.fix`) with:
  - `ok`
  - `changes`
  - `actions`
  - `errors`
- Implemented safe config remediation + persistence:
  - auth mode hardening (`none` -> `auto`)
  - loopback bind normalization
  - loop-guard enablement and threshold normalization
  - blocked pattern and credential-key baseline restoration
  - risk threshold normalization
  - persisted state path remediation
  - policy bundle path remediation + baseline JSON bootstrap
- Added idempotency and app integration tests for fix mode.

## Artifacts

- `dist/release-v2.0.2-go-assets/openclaw-go-windows-amd64.exe`
- `dist/release-v2.0.2-go-assets/openclaw-go-linux-amd64`
- `dist/release-v2.0.2-go-assets/openclaw-go-android-arm64`
- `dist/release-v2.0.2-go-assets/SHA256SUMS.txt`

## Validation

Dockerized Go validation:

- `gofmt -w ./cmd ./internal`
- `go test ./...`
- `go vet ./...`

Binary smoke checks:

- Linux artifact:
  - `--security-audit --fix` output includes `securityAudit.fix`
  - `/health` returns `status=ok`
  - `/rpc status` returns RPC `ok=true`
- Windows artifact:
  - `--list-methods` reports `133` methods
  - `--security-audit --fix` returns `fix.ok=true`

## Checksums

See `dist/release-v2.0.2-go-assets/SHA256SUMS.txt`.
