# Drop the vendored GPUI: stock crates, own components

Status: IN PROGRESS. The owner closed the open questions on 2026-11-25 (see "Owner decisions").

## Summary
`app/desktop/vendor/` holds seven patched crates (412,719 lines of `.rs`, 13 MB) wired in through
`[patch.crates-io]` (`app/Cargo.toml:140-147`). There are 49 patch files: 41 logical patches,
because `letter-spacing`, `physical-key`, `overlay-window` and `owner-capture` each span
several crates. That adds up to 6,441 patch lines, and 3,870 of them belong to the overlay
family (with `owner-capture` and `xdnd-relay`).

The 41 logical patches by bucket:
- OWN-ELEMENT: 18
- OWN-PLATFORM: 12
- DROP: 7
- BLOCKED: 4, all in the gpui-kit editor and none in GPUI.

None needs a change to GPUI, but the overlay family needs a redesign rather than a port:
- **Linux.** The top layer becomes stock GPUI windows (`WindowKind::Dialog`, `Floating` and
  `PopUp`). Our `platform` crate places, shapes, stacks and focuses them through the X window
  id that `raw-window-handle` gives. Dropdowns and selects become native GTK menus, as context
  menus already are.
- **macOS.** gpui-pre 0.3.14's new `NativeView` paints GPUI above a `WKWebView`, so macOS
  needs no overlay at all.
- **Text and input.** The text-system patches are replaced by static font instances and our
  own glyph painting. The input patches (physical key, inferior focus, XDND, WM_CLASS) move
  to our own x11rb/xkb code in `platform`.
- **Editor.** gpui-kit 0.11 already has the fold API. Most other editor patches become our
  own code in `editorview`.
- **Blocked.** Four have no public path in gpui-kit 0.11: `selection_ranges`, `shared-text`,
  `top-space` and `std-lazy-lock` (a `deny.toml` policy). Each needs either an upstream PR to
  gpui-kit (not a fork) or a reduced behaviour, and that is the owner's call.

## Target versions (checked 2026-11-25)
| Crate | Use | Why |
|---|---|---|
| `gpui-pre` / `-platform` / `-linux` / `-wgpu` / `-macos` **0.3.14** (2026-11-23) | target | snapshot of zed@f126950f, which exists in zed-industries/zed (verified through the GitHub API) |
| `gpui-kit` / `gpui-base` / `gpui-component` **0.11.0** (2026-11-23) | target | pins `gpui-pre =0.3.14` (crates.io dependency API) |
| `gpui` (official) 0.2.2 | no | last release was 2025-10-22, and gpui-kit does not build on it |
| github.com/zed-industries/gpui | not yet | one commit (2026-11-23), README "See you in a few weeks". Revisit when it publishes. |
| `calloop` 0.14.5 | unchanged | still the latest release. Master still asks for `tracing/log` and still declares `bitflags`. |

API differences between 0.3.12 and 0.3.14 that matter here:
- Removed public items: `build_path`, `clipped_bounds`, `push_triangle`, `PathId`, `PathVertex`
  and `TabStopOrderNodeSummary`. The shell uses none of them (grep).
- Added: `NativeView` (`src/window/native_view.rs`). On macOS only, it composites an `NSView`
  under GPUI content: "elements painted after it cover it". On other platforms
  `NativeView::new` fails.
- Still true on 0.3.14:
  - `WindowKind::AnchoredPopup` is rejected on X11 (`gpui-pre-linux x11/window.rs:484`).
  - X11 does not implement `set_input_region`; it is a default no-op (`platform.rs:1472`), and
    only Wayland has it (`wayland/window.rs:2173`).
  - X11 has no letter spacing, no physical key (`Keystroke` at `platform/keystroke.rs:18` has
    only `modifiers`, `key` and `key_char`), and still drops faces without an `m` from the
    loaded list (`cosmic_text_system.rs:428`).

Public API this plan relies on (gpui-pre 0.3.14):
- Windows:
  - `Application::with_platform` (`app.rs:242`).
  - `impl HasWindowHandle for Window` (`window.rs:8112`).
  - `WindowKind::{PopUp, Floating, Dialog}` (`platform.rs:2929`).
  - X11 always uses an ARGB visual (`x11/window.rs:494`).
  - `PopUp` is override-redirect (`:519`).
  - `Floating` and `Dialog` get `WM_TRANSIENT_FOR` the focused window (`:634`).
  - `Dialog` is `_NET_WM_STATE_MODAL` and blocks its parent's input (`is_blocked`, `:1181`).
- Painting:
  - `Window::paint_glyph` / `paint_emoji` / `paint_svg` / `paint_image` (`window.rs:5291/5398/5455/5547`).
  - `TextSystem::shape_line`, with public `LineLayout.runs[].glyphs[].position`.
- Input and tests:
  - `Window::on_mouse_event` / `on_key_event` with `DispatchPhase` (`:5948/5971`).
  - `Window::text_style()` (`:2828`).
  - `rendered_scene()` with `pub Scene.shadows` (feature `bench-support`, `:3263`).

## Patch table
Buckets: **EL** = OWN-ELEMENT, **PF** = OWN-PLATFORM, **DROP**, **BLK** = BLOCKED.
"Ours" means the shell's own crates under `app/desktop/crates`.

### gpui-pre / -linux / -macos / -wgpu (26 logical patches)
| Patch (lines) | Bucket | Evidence and replacement |
|---|---|---|
| svg-inherit-color (16) | EL `ui` | `ui::Icon` becomes its own `Element`. At paint it reads `window.text_style().color`, which carries the parent's `.hover()` refinement because `Div` paints its children inside `with_text_style`. It then calls the public `paint_svg` (`window.rs:5455`). Stock `Svg` paints only `style.text.color` (`elements/svg.rs:178`). |
| letter-spacing (188+78+10+162 field fallout) | EL `ui` | Users: `sidebarview column/switcher*`, `ui menu/row.rs`, `ui tokens/font.rs` and the three `ui control/editor` elements, which already paint shaped lines themselves. `ui::TrackedText` shapes with `shape_line`, offsets glyph *i* by *i*·spacing, paints with `paint_glyph`/`paint_emoji`, and measures width plus n·spacing; truncation and the ellipsis are ours. The gpui-base/gpui-component halves are only `TextRun` field fallout and vanish. The terminal's `letter_spacing` is its own cell setting and is untouched. |
| variable-weight (163) | PF `ui` fonts | Ship static instances of `CalSansUI.ttf` (the weights the tokens use) plus a 14° oblique instance, made once with fontTools `instancer` and a skew transform. They are registered by `add_fonts` (`ui/src/foundation/tokens/font.rs:48`), and cosmic-text then picks a face by weight and style. No text-system code changes. Needs a licence check (Q2). |
| whole-pixel-mono (51) | DROP | Geist Mono labels lay out at fractional advances: 6.3 px instead of 6 px per glyph at 10.5 px, so a 20-character label is about 6 px wider than develop's. The terminal uses its own grid and is unaffected. |
| fallback-faces-without-latin (66) | EL `ui` + `terminalview` | The emoji marks and the terminal paint emoji clusters as images: a `ColorEmoji` element rasterizes the cluster from the system `Noto Color Emoji` with swash and paints it with `paint_image`. Everything else keeps stock fallback. |
| derived-faces-keep-their-chain (10) | DROP | Exists only because of `variable-weight`. It goes with it. |
| physical-key (58+53+13) | PF `platform` | One consumer: `app/src/shell_view/shell_commands.rs:180`. `platform::physical_key(&Keystroke)` maps the produced keysym back to its keycode through our own xkbcommon keymap (x11rb XKB on our connection), then the keycode to its US name. The `us_unshifted_key` table moves into `platform`. `chord::PhysicalKey` stays. macOS returns `None`, as today. |
| inferior-focus (14) | PF `platform` | `platform::FocusChange` already reads `NotifyInferior` from our own x11rb connection (`platform/src/linux/focus_change.rs:4-30`). A `WindowActivity` source built on it replaces `window.is_window_active()` at our six readers (`ui toast timer.rs:72`, `line_element.rs:210`, `wrapped_element.rs:201`, `app child_events.rs:36,75,91`). |
| webkit-wheel-step (33) | EL `ui` | `ui::ScrollArea` registers a capture-phase `ScrollWheelEvent` listener (`on_mouse_event`). For `ScrollDelta::Lines` on Linux it scrolls `floor(h^(2/3))` per click through its `ScrollHandle` and stops propagation, so the stock div listener never runs. Pixel deltas pass through. |
| painted-shadows (16) | EL (tests) | Three tests (`select/ring.rs:50`, `button.rs:323`, `row_focus.rs:74`) read `window.rendered_scene().shadows` with gpui's `bench-support` enabled as a dev-dependency feature. Alternatively they assert the ring decision through our own focus-ring API (Q7). |
| focus-visible-press (95) | EL `ui` | `ui::Modality`, an app global, is set by a capture-phase `MouseDownEvent`/`KeyDownEvent` listener that each window's root installs. `ui::focus_ring()` replaces the 7 `.focus_visible(..)` calls and `tooltip.rs:148`. |
| overlay-input-modality (83) | EL `ui` | One app-global `ui::Modality` is shared by every window, so the owner and its float windows agree without GPUI. |
| overlay-opened-with-owner (38) | DROP | Not needed: the modality is an app global, and float windows are opened lazily (below). |
| overlay-window (736+1763+162+test) | PF `platform` + EL `ui` | See the overlay section. |
| owner-capture (78+206) | PF `platform` | Composite `RedirectWindow`, a `Damage` object and `GetImage` on the owner's XID from our own x11rb connection. The `_NET_WM_CM_S<n>` owner tells whether a compositor runs. `ui::Backdrop` keeps its blur and paints with `paint_image`. All of it is X protocol on a window we own; no GPUI hook is involved. |
| unpainted-exposure (73) | PF `platform` | The float window's bounding shape grows only after the frame that paints it, which our code sequences with `on_next_frame`. On bare X11 the background is set to `None` through `ChangeWindowAttributes` on our connection. |
| rounded-content (69) / rounded-images (15) / opaque-ink (71) | EL `ui` | The content region is no longer scraped from the scene. Each floating card reports its rect and radius to its float window (`ui::floating` already knows every card's bounds), and `platform` turns that into per-row XShape rectangles. Text and images sit inside cards, so `opaque-ink` is moot. On bare X11 a fading toast skips the fade (Q3). |
| overlay-focus-owner (27) / -unrendered (35) / -owner-renders (44) | PF `platform` | These exist because one GPUI focus spanned two windows. With stock windows each window keeps its own focus, and the *X* focus decides which window gets keys. The WM gives it to a modal `Dialog` and gives it back on close; `platform` calls `SetInputFocus` for a keyboard popover. |
| drag-over-painted-overlay (51) | PF `platform` | Drag ghosts and the drop preview move to an override-redirect `PopUp` window, so the WM draws no frame, with an empty XShape input region. The pointer never enters it, the owner keeps its implicit grab, and no leave is generated. |
| xdnd-relay (419) | PF `webview` | Instead of relaying inside GPUI's event loop, `webview` sets `XdndProxy` on the GPUI window, pointing to a hidden window of its own (XDND spec §Proxy). That window does the relay we wrote: to the web view child's deepest window, or back to the GPUI window naming itself as source (copying `XdndTypeList`). GPUI's `ConvertSelection` still reads from the real source. |
| gtk-identity (45) | PF `platform` | After `open_window`, `platform` rewrites `WM_CLASS` (`crowbar-desktop`, `Crowbar-desktop`) and `_NET_WM_WINDOW_TYPE_NORMAL` on the XID. GPUI maps before we can (`window.rs:2429`), so the desktop file also lists `StartupWMClass=crowbar-desktop` (Q6). |
| press-drops-preedit (115) | EL `ui`, `terminalview`, `editorview` | Stock already resets the XIM context on a press and then calls `unmark_text` (`x11/client.rs:1243-1246`). Our `EntityInputHandler::unmark_text` (`ui text_input/input_method.rs:11`, `text_area/input_method.rs:16`, `terminalview ime.rs:75`) deletes the marked range on Linux instead of keeping it. For the gpui-kit editor, `editorview` paints after the `Editor` and installs a handler wrapping `InputBaseState`'s public `EntityInputHandler`. Risk: the same-click row offset on a soft-wrapped line (verify on the rig). |

### calloop (2)
| Patch | Bucket | Evidence and replacement |
|---|---|---|
| tracing-without-log | DROP | The user sees nothing. calloop master still asks for `tracing = { features = ["log"] }`. The cost is clippy `cognitive_complexity` (threshold 10, `app/clippy.toml:3`) counting tracing's `log` fallback: functions that log may need splitting. Measure it in stage 1. The lint config is not loosened. |
| no-bitflags | DROP | Nothing visible. machete checks workspace members only, and calloop is a registry crate once unpatched. |

### gpui-base / gpui-component (13 logical, plus the letter-spacing field fallout)
B = `gpui-base-0.11.0/src`, C = `gpui-component-0.11.0/src`, E = `editorview/src`. No public
item the shell calls was removed in 0.11. The calls that break are the patch-only ones:
`TextModel`, `share_text`, `WhitespaceMode`, `selection_ranges`, `reveal_centered`,
`shows_line_with_context`, `set_top_space` and `replace_all_keeping_selections`. `set_folded`
now returns `bool`.

| Patch | Bucket | Evidence and replacement |
|---|---|---|
| replace-keeping-selections | EL `editorview` | Stock `replace_all` resets the selection and scroll (B `input/base/state.rs:1345`). Ours takes the common prefix and suffix, then `set_selected_range` (3799) and `replace` (1392, one undo step), and restores the caret by line and column (`selected_range` 3775, `set_selected_range_with_direction` 3809) and the scroll (`scroll_offset`/`set_scroll_offset` 3717/3726). Secondary carets collapse until there is a selection reader (next row). |
| editor-commands | **BLK** (one part) | The fold half is upstream: `set_folded`, `fold_all`, `fold_candidates`, `unfold_at` (state.rs:13953-14028). `reveal_centered` is ours: `display_map().buffer_pos_to_display_pos`, `line_height` (3764), `set_scroll_y_f64` (3738). **`selection_ranges` has no public path:** `selections` is `pub(super)` (state.rs:484), and `selected_range` reads "only the active selection" (3768). Alternative: a roughly 8-line upstream PR to gpui-kit for a public reader (Q8). Meanwhile ctrl+d and the line commands act on the active selection only (E `editor_view/occurrences.rs`, `line_commands.rs`, `folding.rs`, `command_actions.rs`). |
| whitespace-modes | EL `editorview` | Stock has the bool only (`set_show_whitespaces`, state.rs:1601). None and All map to it. For Boundary and Trailing we turn the stock markers off and a canvas over the editor paints `·`/`→` at `range_to_bounds` (4481, which uses the same frame's layout) for the rows in `visible_row_range` (3712). `WhitespaceMode` moves into `editorview`. |
| monaco-gutter | DROP | `MIN_LINE_NUMBER_DIGITS` 3 and `FOLD_ICON_HITBOX_WIDTH` 18 are private (B `base/element.rs:58,65`). `EditorLineNumberRenderer` is only for gpui-base's `#[doc(hidden)]` `Editor`, not gpui-component's `Input`. The user sees the text column start about 2 digit cells and 8 px left of develop's in files under 1,000 lines. `gutter_style(..with_gap(0))` removes 6 px of that. |
| shared-text | **BLK** | `UndoManager` is `pub(crate)` (B `base/undo_manager.rs:66`) and undo/redo are `pub(super)` (state.rs:3570/3574). Every public edit collapses other cursors (`set_selected_range` 3795), so two states cannot share one text and one history. Alternatives: an upstream "shared buffer" PR (Q8), or a reduced mirror of our own (about 400 lines): each state's `InputEditsEvent` (state.rs:154) is replayed into the others, with a document-owned undo stack bound in the `Input` context. Follower secondary carets collapse. |
| line-shown-with-context | EL `editorview` | The patch's ten lines of arithmetic over the public `line_height` (3764), `input_bounds` (781) and `scroll_y_f64` (3733), in E `find_widget/stepping.rs`. |
| top-space | **BLK** | The vertical clamp's top bound is hard-coded `0.0` in a `pub(super)` fn (B `base/element.rs:457-462`), `reveal_row`/`update_scroll_offset` are private, and `Input` rewrites the paddings every render (C `input/input.rs:650-662`). Alternative: an upstream view-zone or top-inset PR (Q8). Reduced: the open find widget covers lines 1-2 at scroll top, as Monaco with `addExtraSpaceOnTop` off does. |
| std-lazy-lock | **BLK** (policy) | 0.11 still uses `once_cell` (C `theme/color.rs:348`, its `Cargo.toml:329`). Alternatives: a two-line upstream PR, or the `deny.toml` wrapper (Q5). `once_cell` is already in the lock through about 25 other parents, so the wrapper adds no new code to the build. |
| monaco-markdown-classes | EL `editorview` | At init, take `LanguageRegistry::singleton().language("markdown")` (C `highlighter/registry.rs:547,652`), rewrite its public `highlights` string (63-69) with the four hunks as anchored replaces that fail loudly on a bump, and `register` it again (561). No `.scm` is copied into the repo. |
| typescript-builtin-keyword | EL `editorview` | The same rewrite on the TypeScript config: `(predefined_type) @type.builtin` becomes `@keyword`. |
| code-editor-no-left-inset | DROP | The padding is `input_px().min(6px)` (C `input/input.rs:657`). The user sees line numbers start up to 6 px from the pane edge. |
| bracket-pair-colors | EL `editorview` | Brackets outside string, comment and link captures are found from a per-document `SyntaxHighlighter::styles` (C `highlighter/highlighter.rs:1199`) and set through the public `create_decorations_collection` (B `input/editor/decorations.rs:950`). A `TextDecoration` style wins over syntax (B `base/element.rs:3106`). The cost is a second parse per document. |
| unexpected-brackets | EL `editorview` | The same collection: match the brackets once per change and paint strays in `destructive()`. `SyntaxColors` loses the key (E `theme/highlight.rs:34`). |

The editor items cost about 525 lines of ours, plus about 400 lines if the reduced
shared-text mirror is chosen.

## The overlay family
### Why it exists
Web views are X11 child windows of the GPUI window (`webview/src/lib.rs:7`). X always draws a
child above its parent, so nothing GPUI paints in that window can cover a web view. On
0.3.14, macOS solves this upstream with `NativeView`. Linux has no in-window path, so
anything that floats over a web view must be another X window. That window can be a stock
GPUI window placed by our code.

### Building blocks (all stock GPUI plus `platform`, about 1,200 lines of ours)
- **`platform::x11::FloatWindow`** takes the XID of a stock GPUI window (`raw-window-handle`)
  and can:
  - set `_MOTIF_WM_HINTS` and `WM_TRANSIENT_FOR`;
  - move it with `ConfigureWindow`, following the owner's `ConfigureNotify` and translating
    to root;
  - set the XShape input and bounding regions;
  - set `SetInputFocus`.

  GPUI learns each new position from its own `ConfigureNotify`. Stock X11 never touches
  shapes (no `shape` in `gpui-pre-linux src/linux/x11`).
- **Three kinds of float window:**
  - `Modal`: `WindowKind::Dialog`, owner-sized, with a scrim and `Backdrop`. The WM focuses it,
    and GPUI blocks the owner's input while it is open.
  - `Passive`: `WindowKind::PopUp`, with an empty input shape except for clickable cards. It
    never takes keys, and its shape is emptied while the owner is not active.
  - `Drag`: `PopUp`, input shape empty, alive only during a drag.
- **`ui::Portal`** keeps its API: `here`, `here_sized`, `lifted`, `window` and `cover`. It
  routes to the right float window, or paints in place when its bounds miss every web-view
  rect. That is the common case: sidebar and tab-strip tooltips. The registry, `Layers` and
  `Slot` stay.

### Every user of the top layer and its replacement
| User (file) | Today | Replacement |
|---|---|---|
| Context menus: explorer `explorer.rs:196`, file rows `file_tree_row.rs:112`, strip tabs `strip_tab.rs:160`, chat rows `tree_row.rs:159`, terminal links `terminal_links.rs:88-91` | native GTK when `NativeMenu` is installed, which it is on Linux (`app/src/native_menu.rs`) | No change. The drawn fallback stays for macOS and tests. |
| Dropdown menus: host picker `host_picker.rs:133`, daemon picker `daemon_picker.rs:57`, settings page picker `page_picker.rs:26`, pane strip "more" `pane_strip.rs:222` | drawn in the top layer | Native GTK menu through `NativeMenu` with `Placement`, anchored at the trigger rect |
| Selects: settings rows `controls/select.rs:37`, merge strategy `merge_card.rs:204` | drawn | Native GTK menu with check items (`NativeEntry::Item.checked`) |
| Dialogs (17): `name_prompt`, `reload_dialog`, `file_prompt`, `unsaved_dialog`, settings `settings_modal.rs:202` and `prompt_view.rs:32`, `pairing`, `folderimport`, `branchimport`, `folderpicker`, sidebar `question`, `confirm` and `file_dialog_view`, plus the shell/sidebar/pairing `Portal::cover` layers | top layer plus scrim, and `owner-capture` blur | `Modal` float window per open dialog. The same `Dialog` component renders inside it. Simple confirms could be `GtkMessageDialog` instead (Q4). |
| Popovers: space icon `switcher.rs:191`, create-space tile `create_space.rs:262`, editable mark `editable_mark.rs:97` | top layer, takes keys | `WindowKind::Floating` float window plus `SetInputFocus`. It closes on its own window's deactivation, which `observe_window_activation` reports. |
| Tooltips (18 call sites), tab preview `tab_preview.rs:69` | top layer | In place when clear of web views, otherwise `Passive` |
| Toasts: shell `ToastQueue`, sidebar `overlays/toasts.rs:53` | top layer with `beneath` capture | `Passive` (clickable card rects). There is no fade on bare X11 (Q3). |
| Drag ghosts: `DragGhost` (`grip_ghost.rs:29`, tab `tab_bar/ghost.rs:33`), `row_ghost.rs:27`, tab `TabPreview` lift, drop preview `drop_preview.rs:34`, pane float `pane_cell.rs:79` | top layer, `lifted` | `Drag` window. It is positioned from the owner's drag position, because the overlay gets no pointer events under the owner's grab. |
| Breakpoint `foundation/breakpoint.rs:58` (`owner_bounds`) | patch API | the owner's own `viewport_size()` |
| `observe_overlay_keys` (`child_events.rs:43`) | patch API | `platform::FocusChange` on the float windows' XIDs |
| macOS drawn menus and all floating elements | child `NSWindow` (`gpui-pre-macos` 0001) | In-window painting over web views attached as `NativeView` (0.3.14). `webview` macOS hands wry's `WKWebView` to `NativeView::new` after `removeFromSuperview`. Must compile; full parity later. |

### Behaviour changes to accept
- While a dialog or popover has the keys, the owner window is X-inactive. Every
  "active" reader of ours uses `WindowActivity`, which treats a pair as one.
- A WM-decorated owner on bare X11 (openbox on the rig) dims its frame while a dialog is
  focused, as any modal dialog does.
- Without a compositor, float windows get no translucency outside their card shapes. That
  means no shadows beyond the card and no fades. Develop's look holds only with a compositor.

## What deleting `app/desktop/vendor/` takes
- **`app/Cargo.toml`:**
  - drop all seven `[patch.crates-io]` lines (`:140-147`) and `exclude = [..."desktop/vendor"]` (`:4`);
  - set `gpui`, `gpui_wgpu` and `gpui_platform` to `=0.3.14` and `gpui-kit` to `=0.11.0`;
  - add `x11rb` features `shape`, `composite`, `damage` and `xkb` where `platform` uses them;
  - refresh `Cargo.lock`.
- **`app/deny.toml`:** keep the wrappers it already lists. gpui-component 0.11 pulls
  `once_cell` (its `Cargo.toml:329`), which makes it a new wrapper (Q5). The `ctor`
  wrapper `gpui-pre-macos` stays.
- **`.github/workflows/rust.yml:101`:** delete the `update.sh --check` step.
- **Delete** `app/desktop/vendor/` entirely (`README.md`, `update.sh`, `upstream.txt`,
  `patches/`).
- **Scripts:** the `*/vendor/*` exclusions in `tools/scripts/check-rust-rules.sh:22,34,127,139`
  and `inline-rust-tests.sh:32` become dead, so delete them.
- **Docs:** `docs/rust-style.md:3`, `app/crates/README.md:3`, `app/desktop/crates/editorview/README.md:23`,
  plus a closing note in `docs/plans/2026-11-20-gpui-upgrade.md` and `2026-11-17-overlays-above-web-views.md`.
- **Tests:** the A1 tests that pin each patch move to the new owners, so a regression still
  fails a named test. The tests that only exercised the patch API are deleted:
  `top_layer.rs`, `redraw.rs`, `portal.rs`, `veil.rs`, `beneath.rs` and the `takes_keys`
  assertions in `shell_view.rs:606-667`.

## Stages
Only the end state must work. Intermediate commits only need to build where noted. The
shortest path takes three stages.

| # | Stage | Contents | Size | Risk |
|---|---|---|---|---|
| 1 | Switch to stock | Delete the vendor dir, bump the crates, apply the Cargo/deny/CI/docs edits above, and fix the 0.3.14/0.11 fallout. Replace every non-overlay patch: the EL/PF rows above (Icon, TrackedText, ColorEmoji, font instances, ScrollArea wheel, Modality and focus ring, test scenes, physical key, WindowActivity, XdndProxy, WM_CLASS, preedit) and the editor rows. `ui::Portal` paints in place on every platform for now, so the app builds and runs with floating content under web views. | about 2,500 lines added and 413k deleted; 15 crates touched | M: mechanical except XdndProxy and the editor rows |
| 2 | Float windows (Linux) | `platform::x11::FloatWindow`, owner capture, shapes and focus; `ui::floating` routes `Portal` to the Modal, Passive and Drag windows; dropdowns and selects go native GTK; popovers become `Floating`; delete the patch-API tests and write new ones (Xvfb display tests, as `ui/tests/support/on_x11.rs` does) | about 2,000 lines | **H**: WM behaviour (openbox, xfwm4, mutter) for transient, undecorated and shaped windows, X focus hand-off, drag grab |
| 3 | macOS and parity | `webview` macOS via `NativeView`; the rig side-by-side audit against develop for dialogs, menus, toasts, drag, IME, DnD, fonts and wheel; the `rust.yml` macOS job | about 400 lines | M: wry's `WKWebView` ownership under `NativeView` |

## Owner decisions (2026-11-25)
The open questions are closed. No vendored copies, patches, forks or upstream PRs for GPUI,
gpui-kit, gpui-base, gpui-component or calloop: everything missing is our own code.
1. **Target:** gpui-pre 0.3.14 and gpui-kit/base/component 0.11 now.
2. **Fonts:** generate fixed-weight Cal Sans UI instances if the licence allows it. It does:
   `legal/licenses/CalSans-OFL-1.1.txt` is OFL 1.1 with no Reserved Font Name (the
   copyright line names none, and the font's `name` table carries only the copyright of
   Mark Davis DBA Wordmark), so Modified Versions may be made, may keep the name and must
   stay under the OFL. The file is a variable font (`wght` 400-700, `GEOM` 0-100) with no
   italic axis, so the oblique instance is a skew of the upright one.
3. **Bare X11:** no shadows outside cards and no toast fade without a compositor.
4. **Dialogs:** simple confirms (unsaved changes, reload/discard, confirm, question) are
   native GTK message dialogs; the rest are `Modal` float windows.
5. **`deny.toml`:** the `once_cell` ban gains the wrapper `gpui-component` (owner approved).
6. **WM_CLASS:** the class is `Crowbar` and the desktop file says `StartupWMClass=Crowbar`.
7. **Focus-ring tests:** gpui's `bench-support` is enabled in dev-dependencies only.
8. **Editor:** no upstream PRs; the reduced behaviour is accepted:
   - multi-cursor line commands act on the active selection only, unless gpui-kit ships its
     own command for them, which we use;
   - split views of one file share a buffer of our own, without shared undo;
   - top-space: the find bar is laid out above the editor, not over it.

## Open questions for the owner (closed above)
1. **Target:** gpui-pre 0.3.14 now, or wait for zed-industries/gpui's own release ("in a few
   weeks")? gpui-kit only follows gpui-pre today.
2. **Fonts:** commit generated static and oblique instances of Cal Sans UI (check its OFL
   reserved-name terms), or accept regular-weight-only Cal Sans and upright provisional
   titles?
3. **Bare X11 (openbox rig, no compositor):** accept no shadows outside cards and no toast
   fade there?
4. **Dialogs:** keep every dialog in GPUI (`Modal` window), or show simple confirms (unsaved
   changes, reload/discard, confirm/question) as native `GtkMessageDialog`?
5. **`deny.toml`:** gpui-component 0.11 still depends on `once_cell` (its `Cargo.toml:329`).
   May the `once_cell` ban gain the wrapper `gpui-component`? The rule says bans only shrink.
6. **WM_CLASS:** is `StartupWMClass=crowbar-desktop` acceptable if the post-map rewrite to
   `Crowbar-desktop` is not honoured by some WM?
7. **Focus-ring tests:** enable gpui's `bench-support` as a dev feature (it pulls criterion
   into test builds), or rewrite the three tests against our own ring API?
8. **Editor:** may we open PRs upstream to longbridge/gpui-kit for the blocked editor hooks?
   That is not a fork. Or accept the reduced behaviour listed?
