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
XCODE_VERSION_OUTPUT=$(xcodebuild -version 2>/dev/null || true)
XCODE_MAJOR=$(printf '%s\n' "$XCODE_VERSION_OUTPUT" | sed -n '1s/[^0-9]*\([0-9][0-9]*\).*/\1/p')
if [[ "${XCODE_MAJOR:-0}" -lt 26 ]]; then
  XCODE_VERSION_LINE=${XCODE_VERSION_OUTPUT%%$'\n'*}
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
