# go-agent

Go port runtime for OpenClaw with parity-focused gateway, runtime, bridge, security, and diagnostics surfaces.

Channel adapters include:
- `telegram`, `whatsapp`, `discord`, `slack`, `feishu`, `qq`, `wework`, `dingtalk`, `infoflow`, `googlechat`, `teams`, `matrix`, `signal`, `line`, `mattermost`, `imessage`, `webchat`, `cli`.

## Validate with Dockerized Go

```bash
docker run --rm -v "$PWD/go-agent:/work" -w /work golang:1.25 sh -lc "export PATH=/usr/local/go/bin:$PATH; go test ./... && go vet ./..."
```

## CLI Diagnostics

- `openclaw-go --doctor`
- `openclaw-go --security-audit`
- `openclaw-go --list-methods`
- Add `--deep` to include deep probe checks in doctor/audit output.

## Release Matrix Build

Windows PowerShell:

```powershell
pwsh ./scripts/build-matrix.ps1 -Version 2.6.1
```

POSIX shell:

```bash
sh ./scripts/build-matrix.sh 2.6.1
```

Artifacts include:
- `openclaw-go-windows-amd64.exe`
- `openclaw-go-linux-amd64`
- `openclaw-go-android-arm64`
- `SHA256SUMS.txt`

The build scripts enforce:
- `CGO_ENABLED=0`
- stripped binaries (`-ldflags "-s -w"`)
- deterministic matrix output suitable for Termux/Android deployment flows.
