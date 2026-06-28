#!/usr/bin/env bash
set -euo pipefail

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
