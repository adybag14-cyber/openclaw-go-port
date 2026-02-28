# go-agent (Phase 1 Bootstrap)

This directory contains the initial Go port bootstrap for OpenClaw.

## Current Scope

- Config loading from TOML with environment variable overrides.
- HTTP control surface skeleton:
  - `GET /health` -> runtime metadata and uptime.
  - `POST /rpc` -> phase-1 stub (`-32601` not implemented).

## Run with Dockerized Go

```bash
docker run --rm -v "$PWD/go-agent:/work" -w /work golang:1.22 sh -lc "go test ./... && go vet ./..."
```
