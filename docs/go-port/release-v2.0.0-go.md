# OpenClaw Go `v2.0.0-go`

## Summary

`v2.0.0-go` finalizes the Go port phase program and closes the remaining v2 parity-depth slices implemented after `v1.0.1-go`:

- browser bridge hardening (`runtime.browser_bridge` controls, retries, circuit breaker)
- telegram command parity flow (`/model`, `/auth`, `/tts`)
- wasm runtime lifecycle + stricter sandbox policy checks
- vector + graph memory recall surfaces with persistence
- stateful edge contracts (finetune/enclave/compute depth)
- doctor/CLI diagnostics modes (`--doctor`, `--security-audit`, `--list-methods`, `--deep`)
- optimized cross-target build matrix including Android/Termux artifact target

## Artifacts

- `dist/release-v2.0.0-go-assets/openclaw-go-windows-amd64.exe`
- `dist/release-v2.0.0-go-assets/openclaw-go-linux-amd64`
- `dist/release-v2.0.0-go-assets/openclaw-go-android-arm64`
- `dist/release-v2.0.0-go-assets/SHA256SUMS.txt`

## Validation Gates

Final phase validation rerun (Dockerized Go toolchain):

- `go test ./...`
- `go vet ./...`

Binary smoke checks:

- Windows artifact CLI:
  - `--doctor` returns JSON diagnostics summary
  - `--list-methods` returns method catalog (`133` methods)
- Linux artifact runtime:
  - `GET /health` returns `status=ok`
  - `POST /rpc` with `status` returns `type=resp`, `result.status=ok`, and method surface count

Cross-build matrix smoke:

- `windows/amd64` build success
- `linux/amd64` build success
- `android/arm64` build success

## Checksums

See `dist/release-v2.0.0-go-assets/SHA256SUMS.txt`.

