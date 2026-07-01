#!/usr/bin/env bash
set -euo pipefail

if [[ "$(uname)" != "Darwin" ]]; then
  echo "Icon compilation skipped (macOS only)"
  exit 0
fi

# Requires Xcode 26+ for folder.iconcomposer.icon / AssetCatalogAgent-Runtime support
XCODE_MAJOR=$(xcodebuild -version 2>/dev/null | head -1 | grep -oE '[0-9]+' | head -1 || echo "0")
if [[ "$XCODE_MAJOR" -lt 26 ]]; then
  echo "Icon compilation skipped (requires Xcode 26+, found: $(xcodebuild -version 2>/dev/null | head -1 || echo 'Xcode not found'))"
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
