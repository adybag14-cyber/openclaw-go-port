# OpenClaw Go v2.6.1 Release Notes

Date: 2026-03-01
Tag: `v2.6.1-go`

## Scope

Go-only mainline cleanup after full Rust-to-Go cutover.

1. Archived Rust tree with immutable refs.
2. Removed Rust runtime/manifests/workflow/deploy/parity scripts from `main`.
3. Kept Go runtime and Go release flow as the only active implementation path.

## Archive Safety

- Rust archive branch: `rust-archive-pre-go-only-v2.6.0-go`
- Rust archive tag: `rust-archive-pre-go-only-v2.6.0-go`

## Key Changes

- Removed from `main`:
  - Rust runtime source: `src/*.rs`
  - Rust manifests/toolchain: `Cargo.toml`, `Cargo.lock`, `rust-toolchain.toml`, `deny.toml`
  - Rust config sample: `openclaw-rs.example.toml`
  - Rust-only workflow/template assets
  - Rust deploy parity compose stack
  - Rust parity and replay shell/PowerShell scripts
  - Rust-era planning docs tied to pre-Go runtime shipping
- Added Go CI workflow:
  - `.github/workflows/go-ci.yml`
- Updated docs:
  - `README.md`
  - `docs/GO_PORT_PLAN.md`
  - `docs/go-port/phase-checklist.md`
  - `docs/go-port/phase-log.md`
  - `CHANGELOG.md`

## Validation

Executed with Dockerized Go toolchain:

- `go test ./...` (pass)
- `go vet ./...` (pass)

## Artifacts

Release assets in `dist/release-v2.6.1-go-assets`:

- `openclaw-go-windows-amd64.exe`
- `openclaw-go-linux-amd64`
- `openclaw-go-android-arm64`
- `SHA256SUMS.txt`

Checksums:

- `147f3cd57ec922450658d10776e0f65e25ba03a3c512ceaab4878c66ae7aac5e  openclaw-go-windows-amd64.exe`
- `c928f75c93db187815bbf6fa9bd64891d86f0ac57bf074b1a4f61202c2fad16d  openclaw-go-linux-amd64`
- `9be114c7e6f2f4b02aa9189575ae20ac0f98db11ca7af465d48bcba5d4d89801  openclaw-go-android-arm64`
