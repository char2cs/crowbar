# Icon sources

Master artwork. Everything under `../src-tauri/icons/` is generated from here or
from the Icon Composer bundle — edit these, never the generated output.

Crowbar wears three different faces, on purpose:

| Where | Source | Path |
| --- | --- | --- |
| macOS, release | `../src-tauri/icons/crowbar.icon` (Icon Composer) | compiled to `Assets.car` + `crowbar.icns` by `../icons-compiler/compile-icon.sh` at build time |
| Everything else (Windows, Linux, Android, iOS) | `crowbar-dark.png` | `tauri icon`, see below |
| Any debug build | `crowbar-dev.png` | embedded in the binary, applied at runtime |

## `crowbar-dev.png` — debug builds only

The badged variant, so a Crowbar running out of a worktree is not mistaken for
the installed one in the Dock or app switcher. `lib.rs` embeds it under
`cfg(debug_assertions)` and sets it via `-setApplicationIconImage:` at startup;
release binaries do not even link the bytes.

**Keep the ~10% transparent margin.** AppKit draws a runtime-set icon
edge-to-edge with no inset of its own, so cropping the padding makes the Dock
icon render visibly larger than its neighbours.

## `crowbar-dark.png` — every non-macOS platform

Regenerate the whole set after changing it:

```sh
cd desktop
bunx @tauri-apps/cli icon icons-src/crowbar-dark.png
git checkout -- src-tauri/icons/icon.icns   # macOS keeps the Icon Composer output
```

The `git checkout` is not optional: `tauri icon` also emits an `icon.icns`, and
letting it land would quietly replace the adaptive macOS icon with a flat
rasterised one on any host where `compile-icon.sh` skips (Xcode < 26).

> **Known limitation:** the current export is only 157×157, so tiers above that
> (`icon.png`, the 1024 iOS/Android assets) are upscaled and soft. A 1024×1024
> export would fix them; the sizes actually shipped on Windows and Linux
> (≤ 256px) resample cleanly and are unaffected.
