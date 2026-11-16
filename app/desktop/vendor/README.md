# Vendored crates

Three crates are patched copies of the crates.io release, wired in through `[patch.crates-io]` in `app/Cargo.toml`. The directory is excluded from the workspace, so it is not linted, formatted or tested by CI. Each patch is stored next to its crate as the exact diff against the published tarball (generated with `diff -ru` against the `.crate` from crates.io, ignoring `.cargo_vcs_info.json`, `Cargo.lock` and `Cargo.toml.orig`).

## gpui 0.2.2

`gpui-x11-window-handle.patch` implements `HasWindowHandle` and `HasDisplayHandle` for `X11Window` (`src/platform/linux/x11/window.rs`). Upstream leaves both as `unimplemented!()`, and `wry` needs the handles to parent a web view on X11 (Linux runs through XWayland). Everything else is byte-identical to the 0.2.2 release.

Drop the vendored copy when upstream gpui implements the X11 handles in a published release. Tracking: zed-industries/zed, `crates/gpui/src/platform/linux/x11/window.rs`.

`gpui-variable-weight.patch` (`src/platform/linux/text_system.rs`) makes the Linux text backend honour the `wght` axis of variable faces (Cal Sans UI): gpui keeps one `LoadedFont` per requested weight of a variable face, passes that weight to shaping as a `wght` font feature, and rasterizes those glyphs with swash at that `wght` instance. Upstream always draws the default instance. macOS uses CoreText and is untouched.

`gpui-svg-image-bgra.patch` (`src/platform.rs`): an SVG decoded through `Image::to_image_data` keeps tiny-skia's RGBA order while every other format is converted to the BGRA the renderer expects, so coloured SVG images drew with red and blue swapped. The shell draws provider icons this way.

Drop it when a published gpui converts SVG images to BGRA. Tracking: zed-industries/zed, `Image::to_image_data`.

`gpui-external-paths.patch` (`src/interactive.rs`): `ExternalPaths::from_paths` builds the file-drop payload `ExternalPaths` otherwise only gets from a platform, so the sidebar's file-drop test can drive its `on_drop` handler. Drop it when a published gpui exposes a constructor.

## cosmic-text 0.14.2

`cosmic-text-variable-weight.patch`, reached through `gpui` on Linux and wired in through `[patch.crates-io]` in `app/Cargo.toml`:

- `src/shape.rs`: `shape_fallback` clones the rustybuzz face and applies the `wght` variation when the run carries a `wght` font feature, so advances follow the weight; a glyph whose varied advance comes back 0 (no HVAR table, empty glyphs such as the space) keeps its `hmtx` advance. The weight cannot travel in `Attrs::weight`, because cosmic-text matches faces on it and a variable face only matches its default weight.
- `src/swash.rs`: re-exports the swash types gpui needs to scale a glyph at a variation instance.

The copy is trimmed to what the build needs: `tests/`, `benches/`, `fonts/`, the helper scripts, `deny.toml` and the changelog are deleted, and `Cargo.toml` loses its `[[test]]`, `[[bench]]` and `[dev-dependencies]` tables (not part of the patch file).

Drop the vendored copy when upstream cosmic-text or gpui applies variable-font axes. Tracking: pop-os/cosmic-text.

## xattr 0.2.3

`xattr-libc.patch` (reached through `tar` via `zbus`/`gpui`):

- `src/sys/linux_macos/macos.rs`: `libc::uint32_t` is gone from current `libc`; the type becomes `u32`.
- `src/sys/mod.rs`: `libc::ENOATTR` does not exist on Linux in current `libc`, so `ENOATTR` is `ENODATA` on Linux and Android and stays `libc::ENOATTR` on macOS and the BSDs.

Drop the vendored copy when a published xattr release builds against current `libc` on Linux. Tracking: Stebalien/xattr.

## Updating

Replace the crate directory with the new release, re-apply the patch (`patch -p0 -d <crate>/src < <name>.patch` after adjusting the prefixes), regenerate the patch file, bump `=0.2.2` in `app/Cargo.toml` and refresh `app/Cargo.lock` with `cargo update -p gpui`.
