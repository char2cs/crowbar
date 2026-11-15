# Vendored crates

Both crates are patched copies of the crates.io release, wired in through `[patch.crates-io]` in `app/Cargo.toml`. The directory is excluded from the workspace, so it is not linted, formatted or tested by CI. Each patch is stored next to its crate as the exact diff against the published tarball (generated with `diff -ru` against the `.crate` from crates.io, ignoring `.cargo_vcs_info.json`, `Cargo.lock` and `Cargo.toml.orig`).

## gpui 0.2.2

`gpui-x11-window-handle.patch` implements `HasWindowHandle` and `HasDisplayHandle` for `X11Window` (`src/platform/linux/x11/window.rs`). Upstream leaves both as `unimplemented!()`, and `wry` needs the handles to parent a web view on X11 (Linux runs through XWayland). Everything else is byte-identical to the 0.2.2 release.

Drop the vendored copy when upstream gpui implements the X11 handles in a published release. Tracking: zed-industries/zed, `crates/gpui/src/platform/linux/x11/window.rs`.

## xattr 0.2.3

`xattr-libc.patch` (reached through `tar` via `zbus`/`gpui`):

- `src/sys/linux_macos/macos.rs`: `libc::uint32_t` is gone from current `libc`; the type becomes `u32`.
- `src/sys/mod.rs`: `libc::ENOATTR` does not exist on Linux in current `libc`, so `ENOATTR` is `ENODATA` on Linux and Android and stays `libc::ENOATTR` on macOS and the BSDs.

Drop the vendored copy when a published xattr release builds against current `libc` on Linux. Tracking: Stebalien/xattr.

## Updating

Replace the crate directory with the new release, re-apply the patch (`patch -p0 -d <crate>/src < <name>.patch` after adjusting the prefixes), regenerate the patch file, bump `=0.2.2` in `app/Cargo.toml` and refresh `app/Cargo.lock` with `cargo update -p gpui`.
