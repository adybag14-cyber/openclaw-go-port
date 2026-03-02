#!/usr/bin/env sh
set -eu

PROBES="${PROBES:-12}"
INTERVAL_SECONDS="${INTERVAL_SECONDS:-10}"
STARTUP_TIMEOUT_SECONDS="${STARTUP_TIMEOUT_SECONDS:-120}"
BRIDGE_IMAGE_PULL_RETRIES="${BRIDGE_IMAGE_PULL_RETRIES:-3}"
WITH_BRIDGE="${WITH_BRIDGE:-0}"
NO_CLEANUP="${NO_CLEANUP:-0}"

if [ "$PROBES" -le 0 ]; then
  echo "PROBES must be > 0" >&2
  exit 2
fi
if [ "$INTERVAL_SECONDS" -le 0 ]; then
  echo "INTERVAL_SECONDS must be > 0" >&2
  exit 2
fi
if [ "$STARTUP_TIMEOUT_SECONDS" -le 0 ]; then
  echo "STARTUP_TIMEOUT_SECONDS must be > 0" >&2
  exit 2
fi
if [ "$BRIDGE_IMAGE_PULL_RETRIES" -le 0 ]; then
  echo "BRIDGE_IMAGE_PULL_RETRIES must be > 0" >&2
  exit 2
fi

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
REPO_ROOT="$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

COMPOSE_FILES="-f docker-compose.yml"
if [ "$WITH_BRIDGE" = "1" ]; then
  COMPOSE_FILES="$COMPOSE_FILES -f docker-compose.bridge.yml"
fi

echo "Using compose files: $COMPOSE_FILES"
docker compose $COMPOSE_FILES config >/dev/null

if [ "$WITH_BRIDGE" = "1" ]; then
  bridge_image="mcr.microsoft.com/playwright:v1.52.0-noble"
  pulled=0
  attempt=1
  while [ "$attempt" -le "$BRIDGE_IMAGE_PULL_RETRIES" ]; do
    echo "Pulling bridge image ($attempt/$BRIDGE_IMAGE_PULL_RETRIES): $bridge_image"
    if docker pull "$bridge_image"; then
      pulled=1
      break
    fi
    sleep 5
    attempt=$((attempt + 1))
  done
  if [ "$pulled" -ne 1 ]; then
    echo "Unable to pull bridge image after $BRIDGE_IMAGE_PULL_RETRIES attempts: $bridge_image" >&2
    exit 1
  fi
fi

docker compose $COMPOSE_FILES up -d --build

cleanup() {
  if [ "$NO_CLEANUP" != "1" ]; then
    echo "Stopping compose services..."
    docker compose $COMPOSE_FILES down --remove-orphans
  fi
}
trap cleanup EXIT INT TERM

failures=0

warmup_deadline=$(( $(date +%s) + STARTUP_TIMEOUT_SECONDS ))
warmup_ready=0
while [ "$(date +%s)" -lt "$warmup_deadline" ]; do
  main_warm=0
  bridge_warm=1
  if curl -fsS --max-time 8 "http://127.0.0.1:8766/health" >/dev/null 2>&1; then
    main_warm=1
  fi
  if [ "$WITH_BRIDGE" = "1" ]; then
    if curl -fsS --max-time 8 "http://127.0.0.1:43010/health" >/dev/null 2>&1; then
      bridge_warm=1
    else
      bridge_warm=0
    fi
  fi
  if [ "$main_warm" -eq 1 ] && [ "$bridge_warm" -eq 1 ]; then
    warmup_ready=1
    break
  fi
  sleep 2
done

if [ "$warmup_ready" -ne 1 ]; then
  echo "Warmup timeout: services did not become healthy within ${STARTUP_TIMEOUT_SECONDS}s" >&2
  exit 1
fi

i=1
while [ "$i" -le "$PROBES" ]; do
  ts="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
  main_ok=0
  bridge_ok=0

  if curl -fsS --max-time 8 "http://127.0.0.1:8766/health" >/dev/null 2>&1; then
    main_ok=1
  fi

  if [ "$WITH_BRIDGE" = "1" ]; then
    if curl -fsS --max-time 8 "http://127.0.0.1:43010/health" >/dev/null 2>&1; then
      bridge_ok=1
    fi
  else
    bridge_ok=1
  fi

  echo "[$ts] probe $i/$PROBES openclaw-go=$main_ok bridge=$bridge_ok"
  if [ "$main_ok" -ne 1 ] || [ "$bridge_ok" -ne 1 ]; then
    failures=$((failures + 1))
  fi

  if [ "$i" -lt "$PROBES" ]; then
    sleep "$INTERVAL_SECONDS"
  fi
  i=$((i + 1))
done

if [ "$failures" -gt 0 ]; then
  echo "Docker stability gate failed with $failures failed probes" >&2
  exit 1
fi

echo "Docker stability gate passed: probes=$PROBES interval=${INTERVAL_SECONDS}s with_bridge=$WITH_BRIDGE"
