# `sidebar-peek` (P3.59)

`web/src/components/layout/sidebar-peek.tsx` →
`crates/crowbar-ui/src/components/sidebar_peek.rs`,
`crates/crowbar-app/src/surfaces/sidebar_peek.rs`,
`crates/crowbar-app/src/row_layout/sidebar_peek.rs`.

**No live reference.** This item does not run the oracle or capture a
snapshot — see the item brief's hard constraints. Every number below is read
off the app's own compiled Tailwind (`native/MAPPING.md`'s method), not off a
live capture.

## 0. What this file is, and what it is not

`sidebar-peek.tsx` is `SidebarPeek({ hidden, side, width, children })`: the
hover-to-peek host. While the sidebar is collapsed, bringing the pointer to
the window edge slides a floating copy of it in over the editor; moving away
slides it out. In the real app it wraps the *entire* sidebar column
(`SidebarProjectHeader`, `ContextPill`, `SidebarTabBar`, `SidebarCarousel`,
`SidebarToastOverlay` — see `ide-shell.tsx`), which this port does not own
and cannot repaint a second copy of — the same opaque-`children` call
`nav-stack.tsx`'s own base layer and `sidebar-carousel.tsx`'s own four
panels already make.

## 1. Re-deriving `layout-denominator.md` §4's own reasoning — the closer of the two calls, checked rather than assumed

§4 pairs this file with `nav-stack.tsx` as the tier's two judgment calls, and
flags this one as the closer call specifically: the *trigger* is a
`document`-level `pointermove` listener computing a `hovered` boolean from
raw cursor coordinates, continuous interaction rather than a discrete store
action the way `push`/`pop` are. Checked directly against this file rather
than taken on that flag: what the trigger produces is a single `hovered`
boolean, collapsed with `hidden` into one `data-state`, which drives
**three** concrete, static geometries (`docked`/`closed`/`peeking`) — every
one of them drivable by setting the state directly, with no simulated
pointer motion needed, exactly the shape `sidebar-carousel`'s own `selected`
cell already established as in scope. **The reasoning holds for this file
too.** The trigger's own continuity is real but orthogonal to what gets
ported: the *geometry* three static states produce is exactly as portable as
`nav-stack.tsx`'s store-driven states are, even though what *drives* the
state differs.

## 2. No anchor on the outer wrapper — a new shape of an established argument

The item brief pointed at `workspace-switcher.tsx`'s wrapper (unconditional
`display: contents`, no id, no surface) as the precedent to check this
file's own wrapper against. It is the same shape, extended by one step:

`sidebar-peek.tsx`'s outer `<div data-sidebar-peek data-state={...}
className={cn('flex min-h-0 flex-col', hidden ? 'contents' : 'h-full')}>` is
**conditionally** `display: contents` — `workspace-switcher.tsx`'s own
wrapper always is. Two facts, together, are why it still carries no
`data-oracle-id`:

1. **In two of the three states it generates no box at all.** When `hidden`
   (`closed`/`peeking`), its class list resolves to `contents` —
   `ANCHORS.md` v1.11 ("an element that generates no box is not an anchor")
   forbids anchoring it in those two cells outright.
2. **In the third state its box is byte-identical to its own sole child's.**
   When `!hidden` (`docked`), the outer wrapper's class list is
   `flex min-h-0 flex-col h-full` — and the inner `<div>`'s own
   `hidden ? [...] : 'h-full'` resolves, in that same branch, to
   `flex min-h-0 flex-col h-full`: the identical string. Neither carries
   padding, margin or border, so one box exactly contains the other with
   zero offset in every `docked` cell there is.

So there is no cell in which anchoring the outer wrapper would report
anything the inner div's own `sidebar-peek` anchor does not already carry.
The real anchor is placed on the inner div — a real box in **all three**
states — instead.

## 3. `before:`, the muted wash and `bg-clip-padding` — unpainted, the `popover` precedent

`sidebar-peek.tsx`'s own comment says its card recipe *is*
`popover.tsx`/`dialog.tsx`'s: a `before:` inset highlight, a
`bg-clip-padding` border blend and `shadow-lg/5`. `popover.rs`'s own module
docs already settle this for the identical recipe: "the `before:` inset
shadow [is] `ANCHORS.md` §6 material — no field, either side." `shadow_lg()`
is painted anyway — the same call `popover.rs` and `fps_overlay.rs` both
make, for visual fidelity though it moves nothing the differ can see — but
the `before:` pseudo-element and the clip-padding blend are not reproduced
at all: no gpui primitive exists for either, and no contract field would
ever compare them.

## 4. The card's screen position is computed, not transformed

`translate-x-0` (peeking) and `translate-x-[calc(±100%+1rem)]` (closed) both
resolve against the card's own border box — a **literal pixel width** here
(`w-(--peek-width)`), not a percentage — so the two resting offsets are
ordinary arithmetic on `SidebarPeek::peek_width` rather than anything that
needs gpui's absent `transform` support:

| React | gpui |
|---|---|
| `inset-y-2` (top only — see §4a for why not bottom) / `left-2` / `right-2` | `PEEK_MARGIN = px(8.0)` |
| `+1rem` clearance in the closed calc | `OFFSCREEN_GAP = px(16.0)` |
| `max-w-[calc(100vw-1rem)]` | `MAX_WIDTH_GAP = px(16.0)`, `.max_w(viewport_width - MAX_WIDTH_GAP)` |
| peeking edge offset | `PEEK_MARGIN` |
| closed edge offset | `PEEK_MARGIN - peek_width - OFFSCREEN_GAP` |

## 4a. `top`+`bottom` alone do not stretch the height — a real taffy/CSS gap, found by running the row_layout test rather than assumed

`inset-y-2` is `top: 0.5rem; bottom: 0.5rem` with no authored `height`. In a
real browser that is enough: CSS's absolute-positioning algorithm computes
an auto height as the containing block's own height minus `top` minus
`bottom`. **taffy does not do this.** The first draft of this port set
`.absolute().top(PEEK_MARGIN).bottom(PEEK_MARGIN)` with no `.h()` call, and
`row_layout::sidebar_peek::peeking_docks_left_by_the_peek_margin` measured
the card **2px tall** — exactly `BORDER_WIDTH` doubled. Taffy computed an
auto height from the (empty) *content*, the same shrink-to-fit answer it
would give a `position: relative` box, and never consulted `bottom` at all.

The fix: [`SidebarPeek::content_height`] is now an explicit field, and only
`top` is set on the card; `bottom` is dropped rather than kept alongside an
explicit height, since a taffy that silently preferred one input over the
other would be a second thing to go stale against. `content_height` itself
is **not** `cell.window_extent()` bare either — that measured the card 16px
short at the bottom, because `window_extent` is the room *below*
`RowSurface`'s own unconditional `pt(INSET_Y)` (see §5's own account of the
harness), while this card's `top` is measured from that already-inset edge
and its height needs to reach the window's **true**, un-inset bottom. The
value that makes both ends land correctly is `INSET_Y + window_extent` —
confirmed empirically (`eprintln!`-ing the raw bounds mid-fix, not derived
and trusted on paper) before being reduced to the closed-form expression now
in `crowbar_app::surfaces::sidebar_peek::Params::peek`.

**Consequence for the two hidden states' own vertical gap:** `origin.y` is
`INSET_Y + PEEK_MARGIN` (24px in the default cell), not bare `PEEK_MARGIN` —
the harness's own top inset is baked into it and there is no way to remove
that without misrepresenting the card's own `top: 8px` declaration as
something else. `bottom_gap` (window height minus the card's own bottom
edge), by contrast, **is** bare `PEEK_MARGIN` — nothing pads the window's
true bottom, so the explicit-height fix reaches it exactly. Both are
asserted in `row_layout::sidebar_peek`, with the arithmetic spelled out in
each test's own doc comment.

## 5. `full_bleed`, and the `fps_overlay.rs` convention it borrows

`Closed`/`Peeking` are `position: absolute` with no `.relative()` ancestor of
their own — the identical shape `surface.rs`'s own `full_bleed` field
documents for `fps_overlay`: taffy resolves `position: absolute` against the
**immediate parent** unconditionally, so reaching the window's true edges
needs that immediate parent to be given the window's own uninset width,
which is what `full_bleed: true` buys. Following `fps_overlay.rs`'s own
`row_layout` convention directly: a `Closed`/`Peeking` cell drives `--width`
equal to `--viewport-width`, confirmed by this item's own
`row_layout::sidebar_peek` tests reading gaps off `RowSurface::window_size`
rather than off any intermediate box. `Docked` is unaffected either way —
its own box is `w_full()`/`h_full()`, which resolves the same regardless of
the ambient inset — and, for the identical reason, only `Docked` is wrapped
in a height-driving column at the surface layer; wrapping `Closed`/`Peeking`
the same way would make that wrapper the absolute box's immediate parent
instead of the harness's own full-window box, and the card's `top`/`bottom`
insets would land against the wrapper's edges, not the window's.

## 6. Anchoring

`sidebar-peek.tsx` carried a `data-sidebar-peek`/`data-state` pair (not an
oracle attribute) before this item, on the **outer** wrapper. One
`data-oracle-id` is added, on the **inner** div instead — `sidebar-peek` —
per §2. The outer wrapper's existing `data-sidebar-peek`/`data-state` are
left untouched (unrelated to the oracle contract) and no `data-oracle-id` is
added to it.

## 7. Declarations

`CONTENT_SIZED = []`. `LINE_SIZED = []` — this surface paints no text run of
its own; the card wraps the opaque sidebar column and nothing else.

## 8. The state axis

Every one of the six §8.3 flags is unmodelled — neither of `sidebar-
peek.tsx`'s two `<div>`s carries a `hover:`/`focus:`/`data-active` rule at
all; every visual difference is `data-state`, this surface's own axis.
`--state`, `--right`, `--peek-width`, `--content-width` and `--height` are
this surface's own options instead. `Params::no_state_axis()` returns
`true`.

## 9. `row_layout` coverage

* every state — docked, closed or peeking alike — carries exactly the one
  root anchor
* docked fills the column exactly, tracking both `--width` and `--height`,
  at `origin.y = INSET_Y` — the harness's own unconditional top inset, not
  zero (see §4a)
* peeking docks left (or right, mirrored) by exactly `PEEK_MARGIN` on the
  horizontal axis; vertically its `origin.y` carries the same `INSET_Y`
  offset docked's does, and its `bottom_gap` (window height minus the
  card's own bottom edge) is bare `PEEK_MARGIN` — the two are *not* the same
  number, and both are asserted with the arithmetic spelled out, per §4a
* closed parks the card fully past the window edge — its own width plus
  `OFFSCREEN_GAP` beyond where peeking rests, confirmed as an exact value
  rather than a bare "off-screen" inequality. **The test's own mutation was
  run, and its first doc comment was wrong**: dropping the `- OFFSCREEN_GAP`
  term moves the card by `OFFSCREEN_GAP` (16px), not `PEEK_MARGIN` (8px) —
  caught only by executing the mutation and reading the actual failure
  ("expected -308px, got -292px"), not by re-deriving the formula on paper
* `--right` mirrors both hidden states off the window's right edge instead
* the card is capped at `viewport - MAX_WIDTH_GAP` even when `--peek-width`
  asks for more
* the content filler moves neither the card's own width nor its origin, in
  any state
* the card paints a real box — solid background, a real border width and
  colour, a positive radius — unlike `sidebar-carousel`'s own
  paints-nothing surface

## 10. Reachability

`ide-shell.tsx:147` — `<SidebarPeek hidden={!sidebarOpen} side={sidebarSide}
width={preferredWidth}>` wraps the sidebar column unconditionally, itself
mounted by `routes/_shell.tsx`. Confirmed via `layout-denominator.md` §2
(sole importer: `ide-shell.tsx`) — **not** via `liveness-audit.md` naming
`sidebar-peek` directly, which it does not (that audit covers only the 48
already-registered `Surface::names()` entries, a narrower scope than the 22
remaining Tier B layout targets this file is one of). `ide-shell.tsx` itself
is root-reachable by construction (`routes/_shell.tsx`'s own route
component), so `sidebar-peek`'s own liveness needs no further chain beyond
that one hop.
