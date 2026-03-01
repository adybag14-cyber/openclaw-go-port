#!/usr/bin/env sh
set -eu

VERSION="${1:-dev}"
OUT_DIR="${2:-../dist/release-${VERSION}-go}"

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
FULL_OUT_DIR="${ROOT_DIR}/${OUT_DIR}"
mkdir -p "${FULL_OUT_DIR}"

build_target() {
  GOOS="$1"
  GOARCH="$2"
  NAME="$3"
  echo "Building ${GOOS}/${GOARCH} -> ${FULL_OUT_DIR}/${NAME}"
  CGO_ENABLED=0 GOOS="${GOOS}" GOARCH="${GOARCH}" \
    go build -trimpath -ldflags "-s -w" -o "${FULL_OUT_DIR}/${NAME}" ./cmd/openclaw-go
}

cd "${ROOT_DIR}"
build_target windows amd64 openclaw-go-windows-amd64.exe
build_target windows arm64 openclaw-go-windows-arm64.exe
build_target linux amd64 openclaw-go-linux-amd64
build_target linux arm64 openclaw-go-linux-arm64
build_target darwin amd64 openclaw-go-darwin-amd64
build_target darwin arm64 openclaw-go-darwin-arm64
build_target android arm64 openclaw-go-android-arm64

(
  cd "${FULL_OUT_DIR}"
  sha256sum openclaw-go-* > SHA256SUMS.txt
)

echo "Release artifacts:"
ls -lh "${FULL_OUT_DIR}"
