# Vendored crates

Three crates are patched copies of the crates.io release, wired in through `[patch.crates-io]` in `app/Cargo.toml`. The directory is excluded from the workspace, so it is not linted, formatted or tested by CI. Each patch is stored next to its crate as the exact diff against the published tarball (generated with `diff -ru` against the `.crate` from crates.io, ignoring `.cargo_vcs_info.json`, `Cargo.lock` and `Cargo.toml.orig`).

## gpui 0.2.2

`gpui-x11-window-handle.patch` implements `HasWindowHandle` and `HasDisplayHandle` for `X11Window` (`src/platform/linux/x11/window.rs`). Upstream leaves both as `unimplemented!()`, and `wry` needs the handles to parent a web view on X11 (Linux runs through XWayland). Everything else is byte-identical to the 0.2.2 release.

Drop the vendored copy when upstream gpui implements the X11 handles in a published release. Tracking: zed-industries/zed, `crates/gpui/src/platform/linux/x11/window.rs`.

`gpui-variable-weight.patch` (`src/platform/linux/text_system.rs`) makes the Linux text backend honour the `wght` axis of variable faces (Cal Sans UI): gpui keeps one `LoadedFont` per requested weight of a variable face, passes that weight to shaping as a `wght` font feature, and rasterizes those glyphs with swash at that `wght` instance. Upstream always draws the default instance. macOS uses CoreText and is untouched. The same patch synthesizes italic: when italic is asked of a family with no italic face (Cal Sans UI has none), gpui keeps a second `LoadedFont` for that face that rasterizes slanted 14 degrees, as WebKit does for the Tauri build's provisional titles. Upstream draws such text upright.

`gpui-svg-image-bgra.patch` (`src/platform.rs`): an SVG decoded through `Image::to_image_data` keeps tiny-skia's RGBA order while every other format is converted to the BGRA the renderer expects, so coloured SVG images drew with red and blue swapped. The shell draws provider icons this way.

Drop it when a published gpui converts SVG images to BGRA. Tracking: zed-industries/zed, `Image::to_image_data`.

`gpui-svg-inherit-color.patch` (`src/elements/svg.rs`): an svg with no colour of its own paints in the text colour in force where it is painted, as an inline SVG on `currentColor` does in the browser. Upstream paints nothing, so `ui::Icon` had to pin the colour while the tree was built, before a parent's `.hover()` text colour applies; the tab strip's close glyph darkens on hover through this.

`gpui-external-paths.patch` (`src/interactive.rs`): `ExternalPaths::from_paths` builds the file-drop payload `ExternalPaths` otherwise only gets from a platform, so the sidebar's file-drop test can drive its `on_drop` handler. Drop it when a published gpui exposes a constructor.

`gpui-text-truncation.patch` (`src/elements/text.rs`): a text element caches its measured size within a frame and reused it whenever its wrap width matched, so a nowrap text first measured at its max-content width (as a flex item is) kept that size when laid out at its real width, and `truncate()` clipped it with no ellipsis. The cache is now also keyed on the truncation width. Drop it when a published gpui keys the cache on it.

`gpui-dispatch-event-result-pub.patch` (`src/window.rs`): `DispatchEventResult` is `pub`, as upstream has it, so the shell can call `Window::dispatch_event` to replay a pointer move reported by a web view, which gpui cannot see. Drop it with the move to a gpui that has it public.

`gpui-overlay-window.patch` gives a window an overlay window: the shell's top layer, a transparent window kept over the owner's content area, so floating elements paint above the owner's native web views.

- `src/platform.rs`, `src/window.rs`, `src/app/context.rs`: `WindowOptions::overlay_of` names the owner; `Window::owner_bounds`, `input_region`, `content_region` and `takes_keys` describe the overlay to the platform, and `Window::set_overlay_shown` maps it only while it has something to show (it starts hidden). The input region is the union of the hitboxes painted in a frame; the content region is the union of what a frame paints opaque, leaving out shadows and translucent fills. The overlay takes the keyboard only while one of its elements is focused, and `Context::observe_overlay_keys` tells the owner when that changes. Focus given to an element the owner renders goes back to the owner; focus on an element the overlay stopped rendering is dropped. Moves and the release of a drag that started in an overlay, and file drops, go to the owner, offset by its bounds, so the owner's drop targets receive them.
- `src/platform/linux/x11/overlay.rs` (new), `x11/window.rs`, `x11/client.rs`, `Cargo.toml` (x11rb `shape`, `xfixes`): an ARGB dialog window, transient for its owner and asking for no decorations (`_MOTIF_WM_HINTS`, static gravity), kept above its owner and moved with it; its own configure events, not the requests, say where it is, and the owner's configure events and the cached RandR monitors place it without round trips. Its input shape is the input region. Without a compositor (no owner of `_NET_WM_CM_S<screen>`, followed through XFixes selection events) its bounding shape is cut to the content region, and an input-only child of the owner takes the input region's invisible part, such as a menu's click-away layer. While a button is held (a drag), the owner's pointer leave is ignored while the pointer is inside its overlay's input region; otherwise the owner hears it, so nothing under a dialog or menu stays hovered. The owner gets the leave when the pointer leaves the overlay outside the owner's bounds.
- `src/platform/mac/window.rs`: a borderless child `NSWindow` above the owner; it ignores the mouse when its input region is empty, and hides by its alpha, since ordering it out would detach it from the owner.
- `src/platform/test/window.rs`: the test window answers `HandleError::NotSupported` for its raw handles instead of panicking, so a view that parents web views can be opened in a test, and an overlay test window reports its owner's bounds.
- `src/window.rs` (`Frame::clear`): test builds clear the frame's debug bounds, so an element that stopped painting is no longer found by `debug_bounds`.
- `src/window.rs`, `src/elements/div.rs`: a window the pointer left (`MouseExited`, as when it moves onto the overlay or a native child view) hit-tests nothing until the pointer comes back, so hover styles clear and `on_hover` listeners hear `false`. Upstream kept the last position hovered, leaving rows lit under a dialog.

Drop it when upstream gpui offers windows above a native child view. Tracking: zed-industries/zed.

`gpui-owner-capture.patch`, applied on top of `gpui-overlay-window.patch`, lets the top layer paint the window beneath a modal scrim: a compositor blends a translucent scrim over the owner but cannot blur through it (develop's dialogs are `backdrop-blur-sm`), and without one only opaque pixels show. Blurring, caching and painting live in `ui::Backdrop`; gpui only reads the pixels.

- `src/platform.rs`, `src/window.rs`: `Window::capture_owner` returns the owner's content area as BGRA at device pixels, `Window::owner_version` changes whenever that content does, and `Window::is_composited` says whether a compositor blends the window.
- `src/platform/linux/x11/overlay.rs`, `x11/window.rs`, `x11/client.rs`, `Cargo.toml` (x11rb `composite`, `damage`): creating an overlay redirects its owner automatically (Composite), so the server keeps the owner's content, child web views included, off screen, and watches it with a Damage object. `capture_owner` reads it with one `GetImage`, unaffected by what the overlay paints above it; each `DamageNotify` bumps the version and, once the overlay has captured its owner and while it is shown, redraws the overlay so the capture follows the owner.
- `src/platform/test/window.rs`: an overlay test window captures an all-black owner and reports no compositor.

Drop it with `gpui-overlay-window.patch`.

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
