# GPUI upgrade: gpui 0.2.2 to gpui-pre 0.3.12

Status: DONE. A0 and A1 landed; A2 landed as one commit with A3 folded in (see Outcome).

## Goal
Move the shell from the vendored, patched gpui 0.2.2 to gpui-pre 0.3.12 (crates.io,
publisher huacnlee), a snapshot of zed@d2a4fb6ded7c470be25d1541281da3882495b65b (the
`zed-rev` in every gpui-pre 0.3.12 `Cargo.toml`). gpui-kit 0.10.0 pins `gpui-pre =0.3.12`,
and its `Editor` replaces Monaco once the switch has landed. Later gpui-pre releases are taken
only when their `zed-rev` exists in zed-industries/zed (0.3.13 claims zed@125ca86, which
does not).

## How gpui-pre is cut
gpui 0.2.2 was one crate. gpui-pre follows zed's split, and every crate keeps zed's lib name:

| crates.io package   | lib name        | holds                                                 |
|---------------------|-----------------|-------------------------------------------------------|
| `gpui-pre`          | `gpui`          | app, window, elements, `PlatformWindow` trait, test platform |
| `gpui-pre-platform` | `gpui_platform` | `gpui_platform::application()` picks the backend       |
| `gpui-pre-linux`    | `gpui_linux`    | X11 and Wayland windows, clipboard, dispatcher         |
| `gpui-pre-wgpu`     | `gpui_wgpu`     | the Linux renderer (wgpu, was blade) and the cosmic-text text system |
| `gpui-pre-macos`    | `gpui_macos`    | the macOS platform                                     |

So a patch that touched one file of gpui 0.2.2 now lands in one of four crates, and
`gpui = { package = "gpui-pre", version = "=0.3.12" }` keeps every `use gpui::` path.

## Current patches and their fate
Verified by applying the series to the published 0.2.2 tarball (the result is byte-identical
to `app/desktop/vendor/gpui`), and by reading the gpui-pre 0.3.12 crate sources.

| Patch | What it does | In 0.3.12? | Plan |
|---|---|---|---|
| `gpui-x11-window-handle` | `HasWindowHandle`/`HasDisplayHandle` for `X11Window` (wry parents web views) | Yes: `gpui_linux/src/linux/x11/window.rs` implements both the same way | Drop |
| `gpui-variable-weight` | Linux text: a `LoadedFont` per requested `wght` of a variable face, shaped and rasterized at that instance; synthetic 14 degree italic for a family without an italic face | No. cosmic-text 0.19 now shapes a variable face at `Attrs::weight`, but `gpui_wgpu`'s `CosmicTextSystem` passes the face's default weight and rasterizes with swash at the default instance; no fake italic | Re-port onto `gpui_wgpu/src/cosmic_text_system.rs` (smaller: the shaping half is upstream in cosmic-text) |
| `gpui-svg-image-bgra` | SVG through `Image::to_image_data` converted to BGRA | Yes: `SvgRenderer::render_parsed` swaps to BGRA (`swap_rgba_pa_to_bgra`) | Drop |
| `gpui-svg-inherit-color` | an svg with no colour of its own paints in the inherited text colour | No: `elements/svg.rs` still paints only with `style.text.color` | Re-port (one hunk, now three branches) |
| `gpui-external-paths` | `ExternalPaths::from_paths` for the sidebar drop test | Not as such, but the field is now `pub` (`ExternalPaths(pub SmallVec<..>)`) | Drop; the test builds `ExternalPaths(paths.into_iter().collect())` |
| `gpui-text-truncation` | text measure cache keyed on truncate width | Yes, differently: the cache is skipped whenever the request or the cached layout is truncated | Drop; the A1 test proves it |
| `gpui-dispatch-event-result-pub` | `DispatchEventResult` public | Yes | Drop |
| `gpui-overlay-window` | the top layer: `WindowOptions::overlay_of`, input/content regions, key routing, X11 ARGB overlay window, macOS child `NSWindow`, hover cleared on `MouseExited`, test-window raw handles | No. 0.3.12 adds `WindowKind::AnchoredPopup`, but X11 rejects it (`PopupNotSupportedError`) and it is a per-popup window, not a top layer with input shaping | Re-port across `gpui` (window.rs, platform.rs, context.rs, div.rs, test window), `gpui_linux` (x11 overlay.rs, window.rs, client.rs, Cargo x11rb `shape`/`xfixes`) and `gpui_macos` |
| `gpui-owner-capture` | `Window::capture_owner`, `owner_version`, `is_composited` (Composite + Damage) for the blurred scrim | No | Re-port with the overlay |
| `cosmic-text-variable-weight` | cosmic-text 0.14 shapes with a `wght` feature | Upstream in cosmic-text 0.19 (variable-font matching and shaping on `wght`, PR 486) | Drop with the vendored copy (A3) |
| `xattr-libc` | xattr 0.2.3 builds against current libc | Not needed: it came in through `zed-async-tar` from gpui 0.2.2; no gpui-pre crate depends on it | Drop with the vendored copy (A3) |

The "italic" patch named in the lane brief is the fake-italic half of
`gpui-variable-weight`. No other in-flight change exists: `vendor/gpui` equals 0.2.2 plus
the nine gpui patches.

## Steps

### A0: patch series
- `app/desktop/vendor/patches/<crate>/NNNN-<name>.patch`, applied in the order of
  `patches/<crate>/series`; `patches/<crate>/trim` lists release files the copy leaves out.
- `app/desktop/vendor/upstream.txt` pins each vendored directory: crates.io name, version
  and the sha256 of the `.crate`.
- `app/desktop/vendor/update.sh <dir>` downloads the pinned release, checks its sha256,
  applies the series and replaces the directory; `update.sh --check` rebuilds every
  directory in a temporary tree and fails on any difference.
- CI: `rust-lint` runs `update.sh --check`.

### A1: tests that pin each patch
Each patch the shell relies on gets a test in the crate that relies on it, through that
crate's public API, so a gpui that drops the behaviour fails a named test, not a screenshot.
Patches already pinned by a test keep it and the plan names it; the others get one. A patch
whose behaviour a test cannot observe headless (X11 window handle, Composite capture) is
pinned by a `#[ignore = "needs display"]` test that CI's Xvfb step runs.

### A2: the atomic switch (one commit on `lane-gpui`)
1. `upstream.txt` pins `gpui-pre`, `gpui-pre-linux`, `gpui-pre-wgpu` and `gpui-pre-macos`
   0.3.12; `[patch.crates-io]` points each at its vendored copy; the workspace dependency
   becomes `gpui = { package = "gpui-pre", version = "=0.3.12" }` plus `gpui_platform` with
   the `font-kit`, `x11` and `wayland` features: without `font-kit` macOS gets
   `NoopTextSystem` and draws no text.
2. Series: drop the five upstreamed patches; re-port svg-inherit-color, variable-weight
   (with italic), overlay-window and owner-capture, one patch per crate per concern.
3. API fallout, crate by crate: `Application::new()` becomes `gpui_platform::application()`;
   `AsyncApp::update` and entity calls no longer return `Result`; `ExternalPaths`
   construction; renamed or moved items found by the compiler.
4. Gates: fmt, workspace clippy, `check-rust-rules.sh`, `check-standards.sh --strict`,
   `cargo test --workspace --all-features`, the display tests under Xvfb, `cargo deny`
   (new transitive crates: wgpu, accesskit), `cargo machete`.
5. Rig: screenshots before and after from a throwaway `crowbar-novnc-shell:latest`
   container with the binary bind-mounted at `/opt/crowbar/crowbar`: main window, sidebar,
   dialog with scrim, peek card, terminal, tooltip, menu. Pixel-identical or explained; the
   renderer moves from blade to wgpu, so glyph anti-aliasing differences are expected and
   each one is listed.

### A3: drop the vendored cosmic-text and xattr
Both are unused after A2 (the lockfile proves it); delete the directories, their
`[patch.crates-io]` entries and `upstream.txt` lines. If A2 already removes the last user,
A3 is part of the A2 commit.

## Landing without blocking the other lanes
- A0 and A1 change no behaviour and land on `migration/rust-shell` like any lane commit.
- A2 lives on `lane-gpui` and is rebased on `origin/migration/rust-shell` as it moves; each
  rebase recompiles and fixes the API fallout in code other lanes added meanwhile, so the
  other lanes never write against two gpui versions.
- When A2 is reviewed, the orchestrator picks a moment, lands it as one commit and tells
  the lanes to rebase. Lane code written against 0.2.2 meets the compiler on rebase, and the
  fallout is mechanical (step 3). The A1 tests guard behaviour across that rebase.
- No compatibility layer between the two APIs: it would be a second copy of the window
  model and would outlive the switch.

## Risks
- wgpu on the rig's software Vulkan (lavapipe), where blade ran before: checked first in A2.
- The overlay patch is 2,500 lines against code zed has since reorganised (`window.rs`,
  X11 client); it is re-ported by hand, not rebased.
- gpui-pre is published by one person; the `upstream.txt` sha256 pins and the vendored
  copies keep the build reproducible if a release is yanked.

## Outcome
- wgpu runs on lavapipe. Rig shots of all seven states render; whole-window pixels differing
  from Tauri fell from 33,966 to 30,127. Glyph baselines now round to nearest instead of
  flooring: more rows land on Tauri's positions, one italic row sits 1 px low (S3 polish).
- `ui::init` sets `TextRenderingMode::Grayscale`: gpui_wgpu defaults to subpixel on Linux,
  WebKit draws grayscale.
- CPU unchanged within noise (idle 0.3%, spinner 94% against 104% before). Binary 41 to 49 MB.
- calloop 0.14.5 is vendored without tracing's `log` feature.

### Notes: checking the macOS target from a Linux host
CI checks on a macOS runner and needs neither of these; they only let a Linux host run
`cargo check -p crowbar --target x86_64-apple-darwin`:
- `objc2-exception-helper` compiles C: point `CC_x86_64_apple_darwin` at a no-op compiler.
- `gpui-pre-macos`'s build.rs compiles Metal shaders only on a macOS host: enable
  `gpui_platform/runtime_shaders` and create the empty shader outputs it expects.
