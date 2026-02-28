# OpenClaw Go Port `v1.0.0-go`

## Scope

- Completes phase 8 cutover for the Go runtime port.
- Final parity sign-off: required module parity matrix is now `22/22` complete.
- Includes cross-platform artifacts for Windows and Linux.

## Artifacts

- `dist/release-v1.0.0-go/openclaw-go-windows-amd64.exe`
- `dist/release-v1.0.0-go/openclaw-go-linux-amd64`
- `dist/release-v1.0.0-go/SHA256SUMS.txt`

## Validation

- Dockerized Go validation:
  - `gofmt -w ./cmd ./internal`
  - `go mod tidy`
  - `go test ./...`
  - `go vet ./...`
- Cross-platform compile:
  - `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ...`
  - `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ...`
- Binary smoke:
  - Windows CLI usage output (`--help`) verified.
  - Linux runtime `/health` verified in container.
- Oracle VM smoke (`ubuntu@144.21.61.111`):
  - Linux binary uploaded and executed.
  - `GET /health` returns `HTTP 200` and `status=ok`.
  - `POST /rpc` with `status` request returns valid RPC response.
