#!/usr/bin/env sh
set -eu

ENV_PATH="${1:-.env}"
EXAMPLE_PATH="${2:-.env.example}"

if [ ! -f "$EXAMPLE_PATH" ]; then
  echo "Missing $EXAMPLE_PATH" >&2
  exit 1
fi

if [ ! -f "$ENV_PATH" ]; then
  cp "$EXAMPLE_PATH" "$ENV_PATH"
  echo "Created $ENV_PATH from $EXAMPLE_PATH"
else
  echo "$ENV_PATH already exists. Updating empty required values only."
fi

generate_token() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -base64 32 | tr -d '\n' | tr '+/' '-_' | tr -d '='
    return
  fi
  # Fallback token generator if openssl is unavailable.
  if command -v sha256sum >/dev/null 2>&1; then
    date +%s%N | sha256sum | awk '{print $1}'
    return
  fi
  if command -v shasum >/dev/null 2>&1; then
    date +%s%N | shasum -a 256 | awk '{print $1}'
    return
  fi
  if command -v uuidgen >/dev/null 2>&1; then
    uuidgen | tr '[:upper:]' '[:lower:]' | tr -d '-'
    return
  fi
  echo "openclaw-go-token-$(date +%s)"
}

if grep -q '^OPENCLAW_GO_GATEWAY_TOKEN=$' "$ENV_PATH"; then
  token="$(generate_token)"
  sed "s|^OPENCLAW_GO_GATEWAY_TOKEN=$|OPENCLAW_GO_GATEWAY_TOKEN=$token|" "$ENV_PATH" > "${ENV_PATH}.tmp"
  mv "${ENV_PATH}.tmp" "$ENV_PATH"
elif ! grep -q '^OPENCLAW_GO_GATEWAY_TOKEN=' "$ENV_PATH"; then
  token="$(generate_token)"
  printf '\nOPENCLAW_GO_GATEWAY_TOKEN=%s\n' "$token" >> "$ENV_PATH"
fi

if [ ! -f "openclaw-go.toml" ] && [ -f "openclaw-go.example.toml" ]; then
  cp "openclaw-go.example.toml" "openclaw-go.toml"
  echo "Created openclaw-go.toml from openclaw-go.example.toml"
fi

echo ""
echo "Environment bootstrap complete."
echo "Next:"
echo "  docker compose up -d --build"
echo "  docker compose -f docker-compose.yml -f docker-compose.bridge.yml up -d"
