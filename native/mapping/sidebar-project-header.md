# `sidebar-project-header` (P3.55)

`web/src/components/layout/sidebar-project-header.tsx` →
`crates/crowbar-ui/src/components/sidebar_project_header.rs`,
`crates/crowbar-app/src/surfaces/sidebar_project_header.rs`,
`crates/crowbar-app/src/row_layout/sidebar_project_header.rs`.

> Cluster 3, "standalone sidebar chrome" (`native/mapping/layout-denominator.md`
> §8): `sidebar-project-header.tsx` · `sidebar-tab-bar.tsx` ·
> `sidebar-skeleton.tsx` · `fps-overlay.tsx` · `sidebar-toast-overlay.tsx`.

**Reference: a live capture exists**, at a 344px column, dark, macOS:

```text
root sidebar-project-header  344x44
  -toggle    x304 y8 28x28      -back      x8  y8 28x28
  -forward   x38  y8 28x28      -settings  x68 y8 28x28
```

## 0. The gap this item closes

The `crowbar-ui` port (`sidebar_project_header.rs`) predates this item. It
already carried the composition's own arithmetic, its own four authored
anchor ids, and a unit-test suite — but no `--surface` existed to drive it
through a real window, so it could not be captured, diffed, or given a
parity verdict. Closing that is this item's whole job: a
`crowbar-app/src/surfaces/sidebar_project_header.rs` cell, a
`crowbar-app/src/row_layout/sidebar_project_header.rs` geometry suite, and
this doc.

**The component's own module doc header was stale, and is now fixed.** At
the time `sidebar_project_header.rs` was first written,
`sidebar-project-header.tsx`'s own `<div>` wrote no oracle attribute, and its
four `<Button>`s all inherited `button.tsx`'s own `'data-oracle-id':
'button'` default with no override — so the module's own header read "why
there is no reference." P3.54 has since landed exactly the fix that
component's own `toggle_id`/`back_id`/`forward_id`/`settings_id` fields were
authored in anticipation of: `sidebar-project-header.tsx` now carries
`data-oracle-id="sidebar-project-header"` on the wrapper and a distinct id on
each button, matching this port's own four ids exactly. The component's
module docs have been corrected in place as part of this item rather than
left to say something no longer true.

## 1. Why the wrapper does not compose `Button::render`

`button.tsx` writes `'data-oracle-id': 'button'` as a **default**, before
`{...props}`, and none of the four `<Button>` call sites in
`sidebar-project-header.tsx` passes an override even now — so a naive
composition through `Button::render` (which calls `anchors.root`) would
still carry the same anchor id four times inside one root, which
`ANCHORS.md` v1.8 refuses outright. `dialog.rs`'s and `alert_dialog.rs`'s own
footers already settle the general shape of this problem one door over
(collapsing a button row to an opaque content height); this file goes one
step further because the traffic-light spacer's arithmetic and the
row-reverse mirror are worth testing on their own terms. Each button is
therefore a plain `anchors.boxed` box, sized and radiused off
`button::Size::IconSm`/`button::RadiusClass::Sm`'s own public,
independently-verified values — reusing what `button.rs` measured, without
reusing the anchor machinery that would collide.

The toggle's real glyph (`<SidebarToggleIcon />`) is left unpainted for the
identical reason: it is a separately-ported primitive with its own anchor
and its own surface, and this composition does not reach into another
component's anchoring.

## 2. `sidebar-toggle-icon` must never appear on this surface

`web/src/lib/oracle/extract.ts`'s own `oracleSurfaceScope` entry for
`sidebar-project-header` (landed by P3.54) declares exactly five anchors —
the wrapper and the four buttons — and excludes `sidebar-toggle-icon`
explicitly, with its own comment naming the reason: the toggle button always
nests that separately-registered surface's own anchor, and an undeclared
capture rooted here would otherwise carry an anchor with no native
counterpart in this composition's own render. This port's own
`row_layout::sidebar_project_header::the_default_cell_carries_every_contract_anchor_and_no_toggle_icon`
test holds the same line from the native side.

## 3. Values

| React / Tailwind | Compiles to | gpui / `crowbar-ui` |
|---|---|---|
| `gap-1` | 4px | `GAP` |
| `gap-0.5` (cluster) | 2px | `CLUSTER_GAP` |
| `pl-3`/`pr-3` (outer, screen-edge side) | 12px | `PADDING_OUTER` |
| `pr-2`/`pl-2` (inner side) | 8px | `PADDING_INNER` |
| `h-[44px]` (`IS_MAC`) | 44px, a literal | `HEIGHT_MAC` |
| `h-[34px]` (non-mac) | 34px, a literal — modelled because the source branches on it, unreachable in a running webview | `HEIGHT_OTHER` |
| `w-[72px]` (traffic-light spacer) | 72px, a literal | `TRAFFIC_LIGHTS_WIDTH` |
| `size="icon-sm"` on all four buttons | `28×28` at the `sm` breakpoint | `button::Size::IconSm.extent(Breakpoint::Sm)` |
| `rounded-sm` | `button::RadiusClass::Sm.value(theme)` | reused, not re-derived |

## 4. The live reference confirms this port's arithmetic, to the pixel

The captured cell (§ above) is **not** this surface's own default: the
toggle sitting at the trailing edge with the cluster at the leading one only
happens with `--right`. Worked out from the numbers alone, `is_right: true`
is the only configuration that produces them: `pl(PADDING_INNER=8)` on the
cluster's own leading edge (`back` at `x8`), `pr(PADDING_OUTER=12)` on the
toggle's trailing edge (`344 − 28 − 12 = 304`), and `flex_row_reverse`
mirroring the DOM order `[toggle, spacer, cluster]` to the visual order
`[cluster, spacer, toggle]`. `forward`'s `x38` and `settings`'s `x68` are
each 30px (28 + `CLUSTER_GAP`) past the previous button, and the root's own
44px height is `HEIGHT_MAC` — the only arm a running webview ever produces.
`row_layout::sidebar_project_header::the_right_docked_cell_reproduces_the_live_reference`
drives `--width 344 --right` through a real taffy layout and asserts every
one of these seven numbers directly, independent of the surface's own
constants.

Neither `--platform` nor the two `can_go_*` booleans are recoverable from the
capture — opacity is not a field `ANCHORS.md` carries at all (§5), so a
dimmed vs. enabled back/forward button is indistinguishable in a snapshot —
so this surface's own default cell stays `SidebarProjectHeader::fixture()`'s
left-docked, both-enabled resting state rather than guessing at those two
axes from a capture that cannot settle them.

## 5. Declarations

`CONTENT_SIZED = []`. `LINE_SIZED = []` — the header paints no text of its
own; its four children are icon-only, unpainted buttons.

## 6. The state axis

Every one of the six §8.3 flags is unmodelled — `sidebar-project-header.tsx`'s
own `<div>` carries no `hover:`/`focus:`/`data-active` rule at all; every
interactive class list on the four buttons is `button`'s own surface's
business (the identical call `sidebar-toggle-icon.md` §5 makes about the
button around *that* glyph). This is not a row, so `Empty` does not apply
either. `--right`, `--platform`, `--no-back` and `--no-forward` are this
surface's own axis instead — the same shape `fps-overlay`'s
`--fps`/`--max-dt`/`--drops` are. `Params::no_state_axis()` returns `true`,
held by `surface.rs`'s own
`no_surface_declares_its_entire_state_axis_unmodelled`.

A dimmed back/forward button (`DISABLED_OPACITY` 0.64) is still `visible` —
v1.7's opacity term only fires at exactly zero — the identical limitation
`row_layout::button::a_disabled_button_is_still_visible_to_both_extractors`
already documents for `button`'s own surface, and
`row_layout::sidebar_project_header::disabled_back_and_forward_stay_visible_and_keep_their_slot`
holds the same line here, proving geometry is unaffected rather than
assuming it.

## 7. `row_layout` coverage

* every contract anchor present exactly once, `sidebar-toggle-icon` never
  present
* the default (left-docked, mac) cell shows its traffic-light spacer, at
  `PADDING_OUTER` + 72 + `GAP` off the left edge
* `--platform other` drops the spacer even left-docked, and shortens the bar
  to `HEIGHT_OTHER` (34px) — the conjunction `is_mac() && !is_right`, checked
  on the axis the left-docked default cannot reach on its own
* `--right` reproduces the live reference to the pixel (§4)
* a dimmed back/forward button keeps its exact resting bounds
* the bar's own width tracks `--width` exactly (`w(relative(1.0))`)

## 8. Reachability

`ide-shell.tsx` is the one importer
(`native/mapping/layout-denominator.md` §2) — mounted unconditionally as the
sidebar's own top bar.

---

## ✅ VERDICT — PASS, 0 deltas over 5 anchors (2026-08-03, taken by me)

```
oracle: sidebar-project-header · width=1714 theme=dark content=normal flags=[]
oracle: PASS — 0 deltas over 5 anchors compared
```

Phase 1 canary `native-short.json` re-captured byte-identical immediately before.

### The drive that produced the reference — required by ANCHORS **v1.14**

`state` records the §8.3 cell and **cannot express which side the sidebar is
docked on**. This reference is the **right-docked** cell, and diffing it against
the surface's own left-docked default fails on every horizontal number.

```
reference:  live Tauri app, sidebar docked RIGHT, dark, viewport 1714
            (window.innerWidth 1714 — read, not assumed)
            captured via import('/src/lib/oracle/extract.ts') from the page
native:     crowbar-app --surface sidebar-project-header \
                        --width 344 --viewport-width 1714 --theme dark --right
```

**`--right` is the whole point of this note.** I supplied this capture to P3.55 as
if it were the default cell; the worker hand-derived the arithmetic, proved it was
right-docked, and kept the surface's default left-docked per convention. Without
`--right` the native side is a mirror image and every button x is wrong.

The left-docked cell — the surface's actual default — **has no reference yet**
and is not covered by this verdict.
