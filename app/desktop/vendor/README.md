# Vendored crates

Five crates are patched copies of a crates.io release, wired in through `[patch.crates-io]` in `app/Cargo.toml`. The directory is excluded from the workspace, so it is not linted, formatted or tested by CI. Each copy is the release pinned in `upstream.txt` (name, version and the sha256 of the `.crate`), without cargo's packaging files (`.cargo_vcs_info.json`, `.cargo-ok`, `Cargo.lock`, `Cargo.toml.orig`) and the paths `patches/<dir>/trim` lists, with the patches `patches/<dir>/series` lists applied in order (`patch -p1 --fuzz=0`). Patches are named `patches/<dir>/NNNN-<name>.patch`; the sections below use the `<name>`.

## gpui-pre 0.3.12

gpui-pre is a crates.io snapshot of zed's gpui (zed@d2a4fb6d) cut into zed's crates: `gpui-pre` (lib `gpui`: app, window, elements, the `PlatformWindow` trait and the test platform), `gpui-pre-linux` (X11 and Wayland), `gpui-pre-wgpu` (the Linux renderer and the cosmic-text text system) and `gpui-pre-macos`. The shell depends on `gpui = { package = "gpui-pre" }` and `gpui_platform` (unpatched); `gpui_platform` needs its `font-kit` feature, without which macOS draws no text.

`svg-inherit-color` (`gpui-pre`, `src/elements/svg.rs`): an svg with no colour of its own paints in the text colour in force where it is painted, as an inline SVG on `currentColor` does in the browser. Upstream paints nothing, so `ui::Icon` had to pin the colour while the tree was built, before a parent's `.hover()` text colour applies; the tab strip's close glyph darkens on hover through this.

`variable-weight` (`gpui-pre-wgpu`, `src/cosmic_text_system.rs`) makes the Linux text system honour the `wght` axis of variable faces (Cal Sans UI): gpui keeps one `LoadedFont` per requested weight of a variable face, matches and shapes it at that weight (cosmic-text 0.19 varies the face by `Attrs::weight`), and rasterizes its glyphs with swash at that `wght` instance. Upstream passes the face's default weight and draws the default instance. The same patch synthesizes italic: when italic is asked of a family with no italic face (Cal Sans UI has none), gpui keeps a second `LoadedFont` for that face that rasterizes slanted 14 degrees, as WebKit does for the Tauri build's provisional titles. Upstream draws such text upright. macOS uses CoreText and is untouched.

`whole-pixel-mono` (`gpui-pre-wgpu`, `src/cosmic_text_system.rs`) advances each glyph of a fixed-pitch face (`post.isFixedPitch`, Geist Mono) by a whole pixel, carrying the rounding on to the glyphs after it, as WebKitGTK lays out the Tauri build's mono text: Geist Mono's 0.6em advance is 6.3px at 10.5px and 7.8px at 13px, which WebKitGTK draws 6 and 8 wide. Proportional faces keep fractional advances there too, so they are untouched. The terminal places glyphs on its own cell grid and is unaffected. macOS uses CoreText and is untouched.

`overlay-window` gives a window an overlay window: the shell's top layer, a transparent window kept over the owner's content area, so floating elements paint above the owner's native web views. Upstream's `WindowKind::AnchoredPopup` is one window per popup and X11 rejects it.

- `gpui-pre`, `src/platform.rs`, `src/window.rs`, `src/app/context.rs`: `WindowOptions::overlay_of` names the owner; `Window::owner_bounds`, `input_region`, `content_region` and `takes_keys` describe the overlay to the platform (the input region goes through upstream's `PlatformWindow::set_input_region`), and `Window::set_overlay_shown` maps it only while it has something to show (it starts hidden). The input region is the union of the hitboxes painted in a frame; the content region is the union of what a frame paints opaque, leaving out shadows and translucent fills. The overlay takes the keyboard only while one of its elements is focused, and `Context::observe_overlay_keys` tells the owner when that changes. Focus given to an element the owner renders goes back to the owner; focus on an element the overlay stopped rendering is dropped. When the owner focuses an element it renders itself, its overlays drop their focus, and a key an overlay with no focus still receives goes to the owner; keys the owner held for an unrendered focus reach that focus once a frame renders it, or the overlay once it takes the keyboard. Moves and the release of a drag that started in an overlay go to the owner, offset by its bounds, while the pointer is outside the overlay's input region, so the owner's drop targets receive them; over the overlay's own content they stay in the overlay for its drop targets. File drops always go to the owner. A window the pointer left, or has not entered yet, hit-tests nothing until the pointer comes in. Entities an overlay draws keep the window they were first drawn in (`src/app.rs`), so the owner's async tasks and `update_in` calls still reach the owner's window.
- `gpui-pre-linux`, `src/linux/x11/overlay.rs` (new), `x11/window.rs`, `x11/client.rs`, `Cargo.toml` (x11rb `shape`, `xfixes`): an ARGB dialog window, transient for its owner and asking for no decorations (`_MOTIF_WM_HINTS`, static gravity), kept above its owner and moved with it; its own configure events, not the requests, say where it is, and the owner's configure events and the cached RandR monitors place it without round trips. Its input shape is the input region. Without a compositor (no owner of `_NET_WM_CM_S<screen>`, followed through XFixes selection events) its bounding shape is cut to the content region, and an input-only child of the owner takes the input region's invisible part, such as a menu's click-away layer. While a button is held (a drag), the owner's pointer leave is ignored while the pointer is inside its overlay's input region; otherwise the owner hears it, so nothing under a dialog or menu stays hovered.
- `gpui-pre-macos`, `src/window.rs`: a borderless child `NSWindow` above the owner; it ignores the mouse when its input region is empty, and hides by its alpha, since ordering it out would detach it from the owner.
- `gpui-pre`, `src/platform/test/window.rs`: an overlay test window reports its owner's bounds.

Drop it when upstream gpui offers windows above a native child view. Tracking: zed-industries/zed.

`owner-capture`, applied on top of `overlay-window`, lets the top layer paint the window beneath a modal scrim: a compositor blends a translucent scrim over the owner but cannot blur through it (develop's dialogs are `backdrop-blur-sm`), and without one only opaque pixels show. Blurring, caching and painting live in `ui::Backdrop`; gpui only reads the pixels.

- `gpui-pre`, `src/platform.rs`, `src/window.rs`: `Window::capture_owner` returns the owner's content area as BGRA at device pixels, `Window::owner_version` changes whenever that content does, and `Window::is_composited` says whether a compositor blends the window.
- `gpui-pre-linux`, `src/linux/x11/overlay.rs`, `x11/window.rs`, `x11/client.rs`, `Cargo.toml` (x11rb `composite`, `damage`; `image`): creating an overlay redirects its owner automatically (Composite), so the server keeps the owner's content, child web views included, off screen, and watches it with a Damage object. `capture_owner` reads it with one `GetImage`, unaffected by what the overlay paints above it; each `DamageNotify` bumps the version and, once the overlay has captured its owner and while it is shown, redraws the overlay so the capture follows the owner.
- `gpui-pre`, `src/platform/test/window.rs`: an overlay test window captures an all-black owner and reports no compositor.

Drop it with `overlay-window`.

`inferior-focus` (`gpui-pre-linux`, `src/linux/x11/client.rs`), applied on top of `owner-capture`: a `FocusOut` whose detail is `NotifyInferior` (the keyboard went into one of the window's child windows, such as a web view the shell hosts) leaves the window active. Upstream deactivates the window, so everything that waits for the window to be looked at (toast expiry, the caret) stalls while a web view holds the keyboard. The window still deactivates when the keyboard leaves it, from itself or from a child.

Drop it when upstream gpui reads the detail of a focus change. Tracking: zed-industries/zed.

`physical-key`, applied on top of `inferior-focus`, gives each keystroke the key's position as well as the character it typed, as DOM `KeyboardEvent.code` does: develop matches ctrl+shift+digit by `Digit1`…`Digit9`, but on X11 shift+2 types `@` and upstream drops the shift.

- `gpui-pre`, `src/platform/keystroke.rs`, `src/window.rs`, `src/app/visual_test_context.rs`: `Keystroke.physical: Option<PhysicalKey { code, modifiers }>`, skipped by serde; `code` is the unshifted US-layout name of the key position and `modifiers` holds every modifier, shift included.
- `gpui-pre-linux`, `src/linux/platform.rs`, `src/linux/wayland/client.rs`: xkb fills it from the keycode through `us_unshifted_key`.
- `gpui-pre-macos`, `src/events.rs`: left `None` for now; the key code (`kVK_ANSI_*`) gives the position there.

Drop it when upstream gpui reports the physical key. Tracking: zed-industries/zed.

`unpainted-exposure` (`gpui-pre-linux`, `src/linux/x11/overlay.rs`, `x11/window.rs`), applied on top of `physical-key`: without a compositor an overlay has no background, so X leaves the pixels beneath in place where the window is exposed; with one its background stays transparent. A frame's content region grows the bounding shape before that frame's pixels land, and the uncovered area was filled with the black background pixel until they did, so a drag ghost showed black for a frame each time it appeared or moved.

Drop it with `overlay-window`.

`overlay-focus-owner` (`gpui-pre`, `src/window.rs`), applied on top of `physical-key`: focusing an element in an overlay also focuses it in the owner. The owner hands its focus to its overlays on every frame, so with only the overlay's focus moved (a click into a dialog's field, a select opening its list) the owner's next frame, such as the one the window's activation draws, put the keyboard back on what the owner focused before.

Drop it with `overlay-window`.

`letter-spacing`, applied on top of `overlay-focus-owner`, spaces text as CSS `letter-spacing` does (develop's `tracking-*`): every glyph after the first moves by the spacing of the glyphs before it, and the line grows by all of it, the last glyph's included. Upstream gpui has no letter spacing, so the sidebar's 19px space name set `tracking-tight` was 5px wider than develop's.

- `gpui-pre`, `src/style.rs`, `src/styled.rs`, `src/text_system.rs`, `src/text_system/line_layout.rs`: `TextStyle::letter_spacing`, `Styled::letter_spacing`, `TextRun::letter_spacing` and `FontRun::letter_spacing` carry the spacing; runs with different spacing do not merge, and `LineLayoutCache::layout_line` applies it after the platform shapes the line, so wrapping and truncation measure the spaced width. `LineWrapper`'s per-character widths do not include it.
- `gpui-pre-wgpu` (`0003`), `gpui-base` (`0001`), `gpui-component` (`0002`), and the `TextRun` literals in `gpui-pre`'s examples: the new field on their `TextRun` and `FontRun` literals, at zero; the wgpu text system keeps a run's spacing when it clips runs to a line.

Drop it when upstream gpui offers letter spacing. Tracking: zed-industries/zed.

`webkit-wheel-step` (`gpui-pre`, `src/elements/div.rs`), applied on top of `letter-spacing`: on Linux a wheel click (three lines from the X11 and Wayland platforms) scrolls a div `pow(h, 2/3)` pixels, rounded down, for a window `h` high, as WebKitGTK's `Scrollbar::pixelsPerLineStep` scrolls develop's pages (86px at 800, 71px at 600, measured on the rig). Upstream scrolls three line heights (78px). Precise (pixel) deltas, the terminal and the editor keep their own rules; other platforms keep upstream's.

Drop it when the shell no longer needs to scroll as WebKitGTK does.

`painted-shadows` (`gpui-pre`, `src/window.rs`), applied on top of `webkit-wheel-step`: with `test-support`, `Window::painted_shadows` returns the last frame's shadows, as `painted_quads` returns its quads, so a test can see whether a focus ring is drawn.

Drop it when upstream has a shadow accessor.

`overlay-input-modality` (`gpui-pre`, `src/window.rs`), applied on top of `painted-shadows`: an owner and its overlays share one input modality, as one page has one in the browser. Upstream keeps it per window, so the Escape typed in the Settings dialog, which floats in the top layer, left the main window counting the mouse as its last input, and the gear the keyboard went back to drew neither its focus ring nor its tooltip. A change of modality redraws the owner and all its overlays.

Drop it with `overlay-window`.

`focus-visible-press` (`gpui-pre`, `src/window.rs`, `src/elements/div.rs`), applied on top of `overlay-input-modality`: `focus_visible` styles, and `Window::last_press_was_keyboard`, follow the last key or pointer press, not the last input of any kind. The browser's `:focus-visible` (develop's rings and keyboard tooltips) stays through pointer moves and goes on a press; upstream dropped it on any move. Hover still hides while the last input was a key, as upstream.

Drop it when upstream gpui keeps focus-visible through pointer moves. Tracking: zed-industries/zed.

`rounded-content` (`gpui-pre`, `src/window.rs`), applied on top of `focus-visible-press`: an overlay's content region leaves out the rounded corners of an opaque fill. Each corner arc becomes a rectangle per device-pixel row, and a pixel is kept when the arc covers its centre. Without a compositor the X11 overlay is cut to that region, and transparent pixels inside it show black. A rounded card counted as its square bounds, so the drag pill, menus and popovers had black square corners on bare X11.

Drop it with `overlay-window`.

`opaque-ink` (`gpui-pre`, `src/window.rs`), applied on top of `rounded-content`: an overlay's content region counts glyphs, images, paths and underlines only when they paint opaque, as it already did for fills. A toast fading out drops its translucent card from the region but kept its text, so bare X11 showed the text's boxes as black blocks.

Drop it with `overlay-window`.

`overlay-opened-with-owner` (`gpui-pre`, `src/window.rs`), applied on top of `opaque-ink`: an overlay no longer looks its owner up when it is created; `Window::share_input_modality` takes the owner's modality from whoever opens it. The shell opens its top layer while the main window is built, before that window's first frame, and the app cannot hand out a window it is building, so the lookup found nothing and the two kept separate modalities. Opened after the first frame instead, the top layer came up a second or more after the window on a slow host, and a cover the first frame showed painted under the native web views meanwhile.

`rounded-images` (`gpui-pre`, `src/window.rs`), applied on top of `overlay-opened-with-owner`: an overlay's content region leaves out the rounded corners of an opaque image, as `rounded-content` does for fills. A fading toast paints the captured window under its rounded card; counted as its square bounds, the capture showed its corners black for a frame on bare X11 while the shape grew past the card.

Drop it with `overlay-window`.

## calloop 0.14.5

`tracing-without-log` (`Cargo.toml`): calloop, reached through `gpui-pre-linux`, asks for tracing's `log` feature, and cargo turns a feature on for every crate that uses tracing. With `log` on, each `tracing` macro also expands to its `log` fallback, which quadruples `clippy::cognitive_complexity` in every function that logs. Nothing in the shell reads `log` records (no `log` logger or bridge is installed), so calloop loses only output nobody received.

Drop it when calloop leaves the feature to its users. Tracking: Smithay/calloop.

`no-bitflags` (`Cargo.toml`): calloop 0.14.5 declares `bitflags` but never uses it, so `cargo machete` reports it. Drop it when calloop removes the dependency.

## gpui-base 0.10.0 and gpui-component 0.10.0

The editor engine (`gpui-base`: `InputBaseState`, the rope, tree-sitter highlighting) and the `Editor` element over it (`gpui-component`), reached through `gpui-kit` 0.10.0 (`editorview`). A fix to either lands here as a series rather than waiting for a release; both pin `gpui-pre =0.3.12`, the version vendored above.

`std-lazy-lock` (`gpui-component`, `Cargo.toml`, `src/theme/color.rs`): the default colour table is a `std::sync::LazyLock` instead of a `once_cell` `Lazy`, and the `once_cell` dependency goes, as `deny.toml` bans `once_cell` (M24) for any new parent. Drop it when upstream moves to std.

`replace-keeping-selections` (`gpui-base`, `src/input/base/state.rs`): `InputBaseState::replace_all_keeping_selections` replaces the whole text as one undo step and puts every selection back at its line and column, clamped to the new text, with the scroll where it was and the new text highlighted again on the next render (as `replace_all` does), as develop's editor does when a clean file changes on disk (Monaco's `pushEditOperations` with the selections set back). Upstream's `replace_all` clears the selection and scrolls to the top, and `set_value` also drops the undo history. Drop it when upstream keeps the selections.

`editor-commands` (`gpui-base`, `src/input/base/state.rs`, `src/input/editor/display_map/display_map.rs`), applied on top of `letter-spacing`: the engine API the shell's editor chords need, as develop's Monaco has them. `selection_ranges` lists every selection (ctrl+d and the line commands act on all of them); `fold_candidates` and `set_folded` fold one range by its start line (ctrl+shift+[ and ], fold all), where upstream folds only from a gutter click; `reveal_centered` scrolls a position to the middle of the viewport as Monaco's `revealPositionInCenter` does (go to line, find), where upstream only scrolls it into view. Drop each method when upstream offers it.

`whitespace-modes` (`gpui-base`, `src/input/base/layout.rs`, `src/input/base/state.rs`, `src/input/base/element.rs`, `src/input/editor/display_map/text_wrapper.rs`), applied on top of `editor-commands`: `WhitespaceMode` (`None`, `Boundary`, `Trailing`, `All`) says which spaces and tabs a code editor marks, as develop's `renderWhitespace` setting does through Monaco: boundary skips a single space between two words, trailing marks only what follows a line's last other character, judged over the whole line when it soft-wraps. `set_whitespace_mode` sets it and `whitespace_mode` reads it; upstream's `show_whitespaces` bool still means all or none. Drop it when upstream marks whitespace by mode.

Drop the vendored copies when the editor needs no patch of its own and upstream keeps pace with gpui-pre.

## Updating

`update.sh <dir>` rebuilds a vendored directory from its pin, trim list and series; `update.sh --check` rebuilds every one in a scratch tree and fails on any difference (CI runs it in `rust-lint`). To change a patch, edit the vendored copy, regenerate the patch with `diff -u` against a fresh `update.sh` build that stops before it, and run `--check`. To move to a new release, change its line in `upstream.txt` (the sha256 is the `cksum` in the crates.io index), fix or drop the patches that no longer apply, run `update.sh <dir>`, bump the version in `app/Cargo.toml` and refresh `app/Cargo.lock` with `cargo update -p <crate>`.
