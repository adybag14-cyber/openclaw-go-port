# OpenClaw Go v2.10.1 Release Notes

## Highlights

- Added optional Lightpanda CDP backend support for browser auth and browser completion bridge helpers:
  - `scripts/chatgpt-browser-auth.mjs`
  - `scripts/chatgpt-browser-bridge.mjs`
- Preserved existing Playwright/Puppeteer bridge behavior as fallback.
- Added engine-order and Lightpanda readiness metadata to bridge `/health`.
- Added Docker bridge env wiring:
  - `OPENCLAW_CHATGPT_LIGHTPANDA_WS_ENDPOINT`
  - `OPENCLAW_CHATGPT_BRIDGE_ENGINES`
- Updated operator docs for Lightpanda configuration and fallback behavior.

## Validation

- `node --check scripts/chatgpt-browser-auth.mjs`
- `node --check scripts/chatgpt-browser-bridge.mjs`
- Dockerized Go validation/build:
  - `go test ./...`
  - `go vet ./...`
  - `go-agent/scripts/build-matrix.sh 2.10.1 ../dist/release-v2.10.1-go-assets`

## Release Artifacts

Generated under `dist/release-v2.10.1-go-assets`:

- `openclaw-go-windows-amd64.exe`
- `openclaw-go-windows-arm64.exe`
- `openclaw-go-linux-amd64`
- `openclaw-go-linux-arm64`
- `openclaw-go-darwin-amd64`
- `openclaw-go-darwin-arm64`
- `openclaw-go-android-arm64`
- `SHA256SUMS.txt`
