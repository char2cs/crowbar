#!/usr/bin/env bash
set -euo pipefail

BINARIES_DIR="$(cd "$(dirname "$0")/.." && pwd)/src-tauri/binaries"
mkdir -p "$BINARIES_DIR"

# Detect target triple
ARCH=$(uname -m)
OS=$(uname -s | tr '[:upper:]' '[:lower:]')

case "$ARCH" in
  arm64|aarch64) ARCH="arm64" ;;
  x86_64)        ARCH="amd64" ;;
  *) echo "Unsupported arch: $ARCH"; exit 1 ;;
esac

case "$OS" in
  darwin|linux) ;;
  *) echo "Unsupported OS: $OS"; exit 1 ;;
esac

PLATFORM="${OS}-${ARCH}"

# Tauri expects binaries named: <name>-<target-triple>
# https://tauri.app/develop/sidecar/
case "$OS-$ARCH" in
  darwin-arm64)  TAURI_TARGET_TRIPLE="aarch64-apple-darwin" ;;
  darwin-amd64)  TAURI_TARGET_TRIPLE="x86_64-apple-darwin" ;;
  linux-amd64)   TAURI_TARGET_TRIPLE="x86_64-unknown-linux-gnu" ;;
  linux-arm64)   TAURI_TARGET_TRIPLE="aarch64-unknown-linux-gnu" ;;
  *) echo "Unsupported platform: $OS-$ARCH"; exit 1 ;;
esac

BINARY_NAME="crowbar-api-${TAURI_TARGET_TRIPLE}"
DEST="${BINARIES_DIR}/${BINARY_NAME}"

# For development: build the Go daemon locally from source
echo "Building crowbar-api for ${PLATFORM} (${TAURI_TARGET_TRIPLE})..."
REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "${REPO_ROOT}/api"
go build -tags noEmbed -o "$DEST" ./cmd/crowbar

echo "Sidecar ready at: $DEST"
