# OpenClaw Go Port `v1.0.1-go`

## Scope

- Aligns Go RPC contract to exact Rust parity method surface (`133/133`).
- Preserves full dispatch coverage for every advertised Go method.
- Ships refreshed Windows/Linux release artifacts.

## Artifacts

- `dist/release-v1.0.1-go/openclaw-go-windows-amd64.exe`
- `dist/release-v1.0.1-go/openclaw-go-linux-amd64`
- `dist/release-v1.0.1-go/SHA256SUMS.txt`

## Validation

- Dockerized Go validation:
  - `gofmt -w ./internal/rpc/registry.go ./internal/gateway/server_test.go`
  - `go test ./...`
  - `go vet ./...`
  - `go test ./internal/gateway -run TestAllSupportedMethodsDispatchWithoutNotImplemented -v`
- Method-surface parity diff:
  - Rust count: `133`
  - Go count: `133`
  - Missing in Go: `0`
  - Extra in Go: `0`
- Cross-platform compile:
  - `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ...`
  - `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ...`
