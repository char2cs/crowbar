#!/usr/bin/env bash
# cargo-packager runs this before each format. Without a signing identity it signs nothing, so the
# bundle is ad-hoc signed here before the DMG wraps it, or macOS reports the download as damaged.
set -euo pipefail
[[ $CARGO_PACKAGER_FORMAT == dmg ]] || exit 0
codesign --force --sign - "$APP_BUNDLE/Contents/MacOS/crowbar-api"
codesign --force --sign - "$APP_BUNDLE"
