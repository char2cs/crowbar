# Icon sources

Master artwork. Everything under `../src-tauri/icons/` is generated from here or
from the Icon Composer bundle — edit these, never the generated output.

| Where | Source | Path |
| --- | --- | --- |
| macOS app and iOS app | `../src-tauri/icons/crowbar.icon` (Icon Composer) | macOS: compiled to `Assets.car` by `../icons-compiler/compile-icon.sh` when `app/desktop/packaging/package.sh` runs; iOS: compiled by Xcode |
| Linux packages, fallback PNGs | `crowbar-dark.png` | `tauri icon`, see below |

## `crowbar-dark.png`

Regenerate the whole set after changing it:

```sh
cd app/mobile
bunx @tauri-apps/cli icon icons-src/crowbar-dark.png
git checkout -- src-tauri/icons/icon.icns   # macOS keeps the Icon Composer output
git clean -f src-tauri/icons                # Windows and Android sizes: no such targets
rm -rf src-tauri/icons/android
```

The `git checkout` is not optional: `tauri icon` also emits an `icon.icns`, and
letting it land would quietly replace the adaptive macOS icon with a flat
rasterised one on any host where `compile-icon.sh` skips (Xcode < 26).

> **Known limitation:** the current export is only 157×157, so tiers above that
> (`icon.png`, the 1024 iOS asset) are upscaled and soft. A 1024×1024 export
> would fix them; the sizes shipped on Linux (≤ 256px) resample cleanly.
