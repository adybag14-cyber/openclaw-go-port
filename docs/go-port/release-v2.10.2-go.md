# OpenClaw Go v2.10.2 Release Notes

## Highlights

- Provider-aware browser auth gate hardening:
  - `browser.request` now requires active web login only for browser-session providers.
  - provider detection covers explicit provider values, model-prefix provider values, and URL-host inference.
- Expanded keyless browser-provider alias coverage:
  - `glm`, `glm5`, `glm-5` map to `zai` (GLM browser path).
  - `mercury`, `mercury2`, `mercury-2` map to `inception`.
  - aliases are aligned across gateway catalog, web login manager, and tool runtime.
- Browser bridge improvements:
  - provider profiles now route ChatGPT/Qwen/Z.ai/Inception domains in `scripts/chatgpt-browser-bridge.mjs`.
  - bridge `/health` exposes provider list plus engine order and Lightpanda readiness flags.
  - fixed page-context error shaping in session probe fallback path.
- Auth/ops reliability improvements:
  - configurable login session TTL (`runtime.web_login_ttl_minutes`, `OPENCLAW_GO_WEB_LOGIN_TTL_MINUTES`) with default 24h.
  - deep security audit browser probe now verifies `/health` and reports HTTP status/body context.
  - doctor detects systemd user/system unit conflicts for runtime/bridge services.

## Validation

- Node syntax checks:
  - `node --check scripts/chatgpt-browser-bridge.mjs`
- Dockerized Go validation:
  - `go fmt ./...`
  - `go test ./...`
  - `go vet ./...`
- Bridge smoke:
  - default `/health` probe: providers + `playwright,puppeteer`
  - Lightpanda-configured `/health` probe: providers + `lightpanda-playwright,lightpanda-puppeteer,playwright,puppeteer`

## Release Artifacts

Generated under `dist/release-v2.10.2-go-assets`:

- `openclaw-go-windows-amd64.exe`
- `openclaw-go-windows-arm64.exe`
- `openclaw-go-linux-amd64`
- `openclaw-go-linux-arm64`
- `openclaw-go-darwin-amd64`
- `openclaw-go-darwin-arm64`
- `openclaw-go-android-arm64`
- `SHA256SUMS.txt`
