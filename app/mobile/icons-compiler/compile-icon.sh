#!/usr/bin/env bash
set -euo pipefail

if [[ "$(uname)" != "Darwin" ]]; then
  echo "Icon compilation skipped (macOS only)"
  exit 0
fi

# Requires Xcode 26+ for folder.iconcomposer.icon / AssetCatalogAgent-Runtime support.
#
# Capture the full output BEFORE extracting the version: piping xcodebuild
# straight into `head -1` under pipefail can kill xcodebuild with SIGPIPE and
# fail the whole pipeline even though the version was already printed — the
# `|| echo 0` fallback then APPENDED a second line, the numeric guard became a
# syntax error ("[[: 16\n0"), the skip branch was silently not taken, and the
# build died later at the Assets.car copy on Xcode <26 runners.
xcode_major() {
  local version_output
  version_output=$(xcodebuild -version 2>/dev/null || true)
  printf '%s\n' "$version_output" | sed -n '1s/[^0-9]*\([0-9][0-9]*\).*/\1/p'
}

XCODE_MAJOR=$(xcode_major)

# The selected Xcode is too old, but a 26+ toolchain may still be installed
# side by side: GitHub's macos-15 runners default xcode-select to 16.4 while
# shipping Xcode 26.x under /Applications. Prefer the newest one via
# DEVELOPER_DIR (step-scoped, no sudo xcode-select needed).
XCODE_SEARCH_DIR="${XCODE_SEARCH_DIR:-/Applications}"
if [[ "${XCODE_MAJOR:-0}" -lt 26 ]]; then
  CANDIDATE=$(ls -d "$XCODE_SEARCH_DIR"/Xcode_26*.app "$XCODE_SEARCH_DIR"/Xcode-26*.app 2>/dev/null | sort -V | tail -1 || true)
  if [[ -n "$CANDIDATE" ]]; then
    export DEVELOPER_DIR="$CANDIDATE/Contents/Developer"
    XCODE_MAJOR=$(xcode_major)
    echo "Selected $CANDIDATE via DEVELOPER_DIR (xcode-select default is older)"
  fi
fi

if [[ "${XCODE_MAJOR:-0}" -lt 26 ]]; then
  XCODE_VERSION_LINE=$(xcodebuild -version 2>/dev/null || true)
  XCODE_VERSION_LINE=${XCODE_VERSION_LINE%%$'\n'*}
  echo "Icon compilation skipped (requires Xcode 26+, found: ${XCODE_VERSION_LINE:-Xcode not found})"
  exit 0
fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BUILD_DIR="$SCRIPT_DIR/.build"
ICONS_DIR="$SCRIPT_DIR/../src-tauri/icons"

xcodebuild \
  -project "$SCRIPT_DIR/icons-compiler.xcodeproj" \
  -target icons-compiler \
  -configuration Release \
  CONFIGURATION_BUILD_DIR="$BUILD_DIR/products" \
  BUILD_DIR="$BUILD_DIR" \
  build

RESOURCES="$BUILD_DIR/products/icons-compiler.framework/Versions/A/Resources"
cp "$RESOURCES/Assets.car" "$ICONS_DIR/Assets.car"
cp "$RESOURCES/crowbar.icns" "$ICONS_DIR/crowbar.icns"

echo "Icons compiled successfully"
