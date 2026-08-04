# `sidebar-toast-overlay` (P3.62)

`web/src/components/layout/sidebar-toast-overlay.tsx` →
`crates/crowbar-ui/src/components/sidebar_toast_overlay.rs`,
`crates/crowbar-app/src/surfaces/sidebar_toast_overlay.rs` (inline),
`crates/crowbar-app/src/surfaces/sidebar_toast_overlay_fallback.rs`
(`Toast.Portal`),
`crates/crowbar-app/src/row_layout/sidebar_toast_overlay.rs`,
`crates/crowbar-app/src/row_layout/sidebar_toast_overlay_fallback.rs`.

> Cluster 3, "standalone sidebar chrome" (`native/mapping/layout-
> denominator.md` §8): `sidebar-project-header.tsx` · `sidebar-tab-bar.tsx` ·
> `sidebar-skeleton.tsx` · `fps-overlay.tsx` · `sidebar-toast-overlay.tsx`
> (this item — the last of the five to land).

**No captured reference.** The `data-oracle-id`s on the two `Toast.Viewport`s
were added by an earlier worker who analysed this component and
deliberately did not start the Rust port (`sidebar-toast-overlay.tsx`'s own
comment, read rather than rewritten: *"no port exists yet … these ids are
read off this component's own structure, not matched against any Rust
source"*). This item builds the port those ids were waiting for. **Verdicts
are the queue's** — no snapshot JSON was captured or fabricated.

## 0. This is the live surface — `toast.rs` is not, and the two must not be confused

`ide-shell.tsx` is `sidebar-toast-overlay.tsx`'s **one** importer
(`native/mapping/layout-denominator.md` §2's own table: `"sidebar-toast-
overlay.tsx" … "ide-shell.tsx" … "none (uses toastManager directly)"`),
mounted unconditionally inside the running app's sidebar. Every real toast a
user sees — `toast.show`/`.info`/`.success`/`.warning`/`.error` from
`features/window/stores/toast-store.ts`, all `toastManager.add(…)` — is
rendered by **this file's own** hand-rolled `SidebarToastItem`, reached
through `Toast.useToastManager()` bound to that same `toastManager`.

The already-merged `toast` surface (`crates/crowbar-ui/src/components/
toast.rs`) ports a **different** component: `ui/toast.tsx`'s `AnchoredToasts`,
bound to a second, independent manager (`anchoredToastManager`) that
`native/mapping/toast.md` §2 establishes has **zero** `.add(` call sites
anywhere in `web/src` — dead code, provably rather than merely unobserved.
Read side by side (this file's own module docs §5) the two components'
`Toast.Root`s share almost no constant: different width behaviour, different
padding placement, a dismiss control one has and the other does not,
different transition properties. **`toast.rs` does not cover any part of
this port**, and this port does not duplicate any part of `toast.rs` — two
independent, correctly-scoped modules, not one thing ported twice.

## 1. Two layout modes on one prop — confirmed, not merely inherited

`sidebarOpen` switches the DOM parent, the positioning scheme **and** the
anchor set:

| | `sidebarOpen === true` | `sidebarOpen === false` |
|---|---|---|
| Parent | inline, inside the sidebar column | `Toast.Portal`, a fixed corner |
| Position | `absolute inset-x-0 bottom-0` | `fixed bottom-4 left-4`/`right-4` |
| Width | `w-full` | `w-72` (288px, authored) |
| `data-oracle-id` | `sidebar-toast-viewport` | `sidebar-toast-viewport-fallback` |
| Windowed? | **yes** — `select_visible`, capped at 3 | **no** — every toast, uncapped |

`Surface::root` requires one fixed string per registry entry
(`surface.rs`'s own doc comment, and `detach-holder-modal`'s split from
`dialog` is the precedent for what a registry constraint does to a single
React file), so this is **two** registered surfaces —
`sidebar-toast-overlay` and `sidebar-toast-overlay-fallback` — over one
shared `SidebarToastOverlay` struct's two render methods
(`render_inline`/`render_fallback`).

## 2. Pinned vs transient windowing — verified, with the exact mechanism named

`SIDEBAR_TOAST_LIMIT` is 3. A toast with `timeout: 0` (`isPinned`, modelled
here as `ToastFixture::pinned`) is never evicted, however many there are —
`selectVisibleToasts`'s own comment explains why: an outage's pinned
"Backend unavailable" toast (`ConnectionIndicator`'s one live producer) must
survive the wave of failure toasts the same outage produces. `select_visible`
ports the algorithm statement for statement: every pinned toast is kept
unconditionally; a transient toast fills a remaining slot only if one is
left, walked in the list's own newest-first order — so when slots run out it
is the **oldest** transient toast dropped, never a pinned one and never the
newest. Confirmed by three deliberate mutations (see the function's own doc
comment for each one's observed panic output) and by
`row_layout::sidebar_toast_overlay::the_outage_fixture_renders_shorter_
here_than_on_the_uncapped_sibling`, which drives the real outage shape
through an actual render.

**Only the inline viewport windows.** `sidebar-toast-overlay.tsx`'s own
fallback branch maps `toasts` directly with no cap — read closely rather
than assumed, since the windowed branch reads so naturally as "the"
behaviour that it would be easy to believe it applies everywhere.
`render_fallback` therefore renders every fixture toast handed to it, and
`row_layout::sidebar_toast_overlay_fallback::outage_is_not_windowed_on_
this_surface` pins the asymmetry: the same five-toast outage fixture renders
far taller here than on the sibling surface — not merely "not identical,"
more than a 3.5× jump a 3-item cap could never produce.

## 3. Enter/exit is CSS-driven — the `toast.rs` principle extends, its numbers do not

Both files rest on `data-starting-style`/`data-ending-style` opacity
transitions the oracle's static snapshot cannot observe on either side, so
`toast.rs`'s own resolution ("transition out of oracle reach, rest state is
the target") extends here as a **principle**: every toast renders at rest.
It does **not** extend as a set of reusable numbers. Measured, reading the
two class lists side by side rather than assuming a shared vendor primitive
means shared values:

| | `toast.rs`'s `Toast.Root` | this component's `Toast.Root` |
|---|---|---|
| transition property | `scale,opacity` | `opacity` alone |
| duration | unspecified | `duration-200` |
| `before:` pseudo shadow layer | yes, two shadows | none |
| width | none authored, shrinks to content | `w-full`, authored |
| padding | on `Toast.Content` | on `Toast.Root` itself |
| dismiss control | none | `Toast.Close`, `absolute top-3 right-3` |

Every row differs. See §5 of the component's own module docs for the table
in full.

## 4. The singleton `Toast.Provider` — a React runtime fact with no port-side counterpart

`SidebarToastOverlay`'s own React doc comment states the mechanism:
`Toast.createToastManager()` is a stateless emitter, so a second mounted
`<Toast.Provider toastManager={toastManager}>` anywhere in the tree
double-renders every toast — the failure mode the old root-level
`<ToastProvider>`'s `suppressToasts={ideShellMounted}` guard existed to
paper over, fragile against any remount race. This is true and load-bearing
for the running app. It has **no counterpart on this side of the port**: a
`crowbar-app` cell is a pure function of a fixture list, rendered once, with
no manager, no subscription and no second call site to collide with.
Verified rather than assumed to be inapplicable — there is nothing here for
it to break.

## 5. `full_bleed`, and why only the fallback surface takes it

`fps_overlay.rs`'s own finding, applied a second time: taffy resolves
`position: absolute` against the immediate parent **unconditionally** — not
the nearest CSS-positioned ancestor, and not the window the way real CSS
`fixed` falls back when nothing establishes a containing block. `render_row`'s
own harness wrapper (`div().w(cell.width_px())`) carries no explicit height,
and an out-of-flow absolute child contributes nothing to that wrapper's own
auto-height — so without intervention the wrapper collapses toward zero and
an absolutely-positioned child lands off the wrong edge. **Measured, not
assumed**: a first attempt at `sidebar-toast-overlay-fallback` without
`fps_overlay.rs`'s own "stage" wrapper (a `div` sized to
`INSET_Y + Cell::window_extent()`, the room the harness's top inset leaves
below it) measured a 152px left gap where `--side left`'s own `left-4`
should have landed at 16px. Adding the identical stage fixed it — every
`row_layout::sidebar_toast_overlay_fallback` test now passes, including the
window-corner-tracks-the-viewport-width one `fps_overlay`'s own test
established the shape of.

The **inline** surface does not take `full_bleed`: its viewport docks to a
`.relative()` wrapper the surface builds for itself (`--height`-tall,
`sidebar_carousel`'s own "the column the surface is drawn inside" fiction),
not to the window, so the true-corner problem does not arise there.

## 6. Anchors — the two viewports, and why `SidebarToastItem` carries none

`ID_VIEWPORT`/`ID_VIEWPORT_FALLBACK` are the surface roots; neither declares
`content_sized` or `line_sized` (both are flex containers with authored
widths, not text runs).

A toast list is a queue whose length is app state — `select-item`'s own
precedent, the brief's own framing: per-item content of a live queue is a
**cell** property, not a surface's anchor set. Read directly rather than
inferred: `SidebarToastItem` carries no `data-oracle-id` anywhere in the
React source (only `data-slot`s), so an undeclared capture of either
viewport would include whatever a real toast happens to render at capture
time — exactly what a cell, not a fixed surface shape, should determine.
This port holds the identical line: `SidebarToastOverlay::item` opts into no
`AnchorSink` method at all, painted for visual completeness only (`toast::
Toast`'s own icon/action placeholders are the precedent one file over).

## 7. Values

| React / Tailwind | Compiles to | gpui / `crowbar-ui` |
|---|---|---|
| `p-2` (inline viewport) | 8px | `VIEWPORT_PADDING` |
| `gap-2` (both viewports) | 8px | `VIEWPORT_GAP` |
| `w-72` (fallback viewport) | 288px | `FALLBACK_WIDTH` |
| `bottom-4`/`left-4`/`right-4` (fallback) | 16px | `FALLBACK_INSET` |
| `px-3.5`/`py-3` (item, folded onto the root — §3 of the component's own docs) | 14px/12px | `ITEM_PADDING_X`/`ITEM_PADDING_Y` |
| `gap-2`/`gap-0.5` (item) | 8px/2px | `ITEM_GAP`/`ITEM_COLUMN_GAP` |
| `border` (item) | 1px | `ITEM_BORDER_WIDTH` |
| `text-sm` (item) | this crate's `--ui-text-*` trade: `theme.ui_text_base` (14px), **not** `theme.ui_text_sm` (12px) — the mirror image of `placeholder-row-actions.md`'s own `text-xs`/`ui_text_sm` note. A first pass read the wrong token; caught writing this doc, fixed, and regression-checked (`the_item_text_size_is_the_ui_text_base_number_not_ui_text_sm`, which fails at `62.285713 against 68` when reverted) | `ITEM_LINE_HEIGHT`, ratio `1.25/0.875` |

## 8. The state axis

`empty` is real: an empty queue renders the viewport with no children — the
one content state that never changes the anchor set (always just the one
viewport). `--toasts` (`single`/`outage`) is this surface's own option, the
same shape `--held` takes on `placeholder-row-actions`; `--side` on the
fallback surface is `sidebarSide`. `hover`/`focus`/`selected`/`loading`/
`error` are all unmodelled: the viewport itself carries no interactive rule
of its own, and everything a real item has (the close button's
`hover:opacity-100` in particular) belongs to unanchored content this
surface does not measure.

## 9. What is not ported

| Thing | Status |
|---|---|
| the enter/exit opacity transition itself | **absent** — rest state only, §3 |
| `Toast.Close`'s dismiss behaviour, `Toast.Action`'s click handler | **absent** — interaction, not a static cell's |
| `swipeDirection` | **absent, no visual effect at all** — a base-ui gesture-only prop, never a `className`/style; not modelled on either side |
| per-item content precision (icon SVG, exact title/description shaping) | **unanchored, painted for visual completeness only** — §6 |
| `z-50`/`z-[var(--z-overlay,60)]` | **absent** — stacking order is not a field this contract's schema carries |

## 10. Reachability

One importer: `ide-shell.tsx`, mounted unconditionally as part of the
running sidebar (§0).
