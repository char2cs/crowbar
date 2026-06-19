#!/usr/bin/env bash
# Compiles crowbar.icon (macOS 26 Tahoe adaptive icon) into Assets.car and
# patches tauri.conf.json + Info.plist so the bundled .app picks it up.
#
# Requires full Xcode (not just Command Line Tools) — actool is Xcode-only.
# Run once after `tauri build`, or integrate into a post-build CI step.
#
# Usage: ./scripts/inject-liquid-icon.sh
set -euo pipefail

ICON_PATH="$(git rev-parse --show-toplevel)/crowbar.icon"
TAURI_DIR="$(cd "$(dirname "$0")/../src-tauri" && pwd)"
RESOURCES_DIR="$TAURI_DIR/icons"

if ! xcrun --find actool &>/dev/null; then
  echo "❌ actool not found. Install Xcode (not just Command Line Tools) and retry."
  echo "   Download: https://developer.apple.com/xcode/"
  exit 1
fi

if [ ! -d "$ICON_PATH" ]; then
  echo "❌ .icon bundle not found at: $ICON_PATH"
  exit 1
fi

echo "🎨 Compiling $(basename "$ICON_PATH") → Assets.car ..."
npx tauri-liquid-icon \
  --icon "$ICON_PATH" \
  --name "crowbar" \
  --output "$RESOURCES_DIR" \
  --tauri-dir "$TAURI_DIR"

echo "✅ Done. Rebuild with 'tauri build' to include the Tahoe adaptive icon."
