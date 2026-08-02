# `dropdown` (P3.31) — a second wrap, and a distinct primitive from `dropdown-menu`

`web/src/components/ui/dropdown.tsx` →
`crates/crowbar-ui/src/components/dropdown.rs`, built on
`gpui_component::popover::Popover` — the same wrap `popover` (P3.15) uses.

> A §6.2 row, in the shape `native/MAPPING.md` fixes. Kept in its own file for
> the reason every Tier B item's is: one appended table is one conflict per
> item.

## 0. Corrections to the brief, stated first because they are load-bearing

Two things the dispatch brief asserted turned out not to hold, checked before
anything else was built on them.

1. **`native/mapping/dropdown-menu.md` does not exist.** `dropdown-menu`'s
   (P2.1) durable §6.2 output lives in `native/MAPPING.md` (the `# dropdown-menu
   (P2.1)` section, plus the later `# native-menu (P2.14)` section on top of
   it), not in `native/mapping/`. `native/mapping/` holds only components
   ported from P3 onward, one file each; `dropdown-menu` predates that
   convention.
2. **`dropdown-menu` is not "already ported and verified"** in the sense of a
   strict-parity result. `native/QUEUE.md`'s Phase 2 close record is explicit:
   *"`dropdown-menu` is deliberately not a parity result"* — the user's ruling
   ("dropdown menus should be native, not React simulated") moved the
   *context-menu* use of that primitive to `native_menu` (an `AppKit` `NSMenu`,
   P2.14, judged against a checklist rather than diffed), while
   `crowbar-ui`'s `dropdown_menu.rs` **stays** for menus that must carry
   Crowbar's own tokens. Neither half of `dropdown-menu` carries a
   convergence verdict; both are honestly recorded as such. This item does not
   touch either — `dropdown.tsx` is a third, independent thing — but the
   brief's premise that it was "verified" needed correcting before citing it
   as precedent.

## 1. `dropdown` is a distinct primitive, not a second reading of `dropdown-menu`

Evidence, not assertion:

| | `dropdown-menu.tsx` (P2.1 / P2.14) | `dropdown.tsx` (this item) |
|---|---|---|
| Underlying primitive | `@base-ui-components/react`'s `Menu` | **none** — hand-rolled from scratch |
| Positioning | base-ui's own CSS anchor positioning / Floating UI | its own `getBoundingClientRect()` arithmetic, computed in `positionMenu` (`dropdown.tsx:427`) |
| Positioning modes | anchor only | **anchor or point** — `AnchorPositioning` / `PointPositioning`, a discriminated union |
| Mount/exit animation | base-ui's `data-starting-style` | `framer-motion` (`m.div`, `AnimatePresence`) |
| Keyboard traversal | base-ui's roving `tabindex` | hand-rolled `ArrowUp`/`ArrowDown`/`Home`/`End`/`Enter` in `handleKeyDown` |
| Resize/scroll tracking | base-ui's `ResizeObserver` internals | its own `ResizeObserver` + `window`/`visualViewport` listener stack |
| Exported symbols | `DropdownMenu`, `DropdownMenuTrigger`, … | `Dropdown`, `dropdownTriggerClassName`, `dropdownItemClassName`, `MenuItem`, `DropdownSection` |
| Shared code | — | — |

`grep -c 'base-ui\|@radix-ui' web/src/components/ui/dropdown.tsx` is **0**. The
two files import nothing from each other, export nothing in common, and share
no class-list constant. At 725 lines against `dropdown-menu.tsx`'s much
smaller footprint, `dropdown.tsx` actually implements the positioning and
interaction logic that `dropdown-menu.tsx` gets for free from base-ui — it is
not "richer" cosmetically, it is a **different kind of component**: a
general-purpose floating panel primitive that `dropdown-menu.tsx` has no need
to be.

**Does the native-menu ruling reach it?** No, on the evidence: the ruling
(`native/MAPPING.md`'s `# native-menu (P2.14)` section) is scoped to base-ui's
`Menu` — the review-thread comment menu and context menus — and `dropdown.tsx`
is never mentioned by it. `dropdown.tsx`'s seven live render sites are a
provider switcher, a file-tree filter, a directory breadcrumb and three editor
toolbar menus — utility panels, not context menus, and only one of the seven
(the file-tree filter) is a `role="menu"` in the base-ui sense at all.
Treating this as an ordinary Tier B port target is this item's call, made
explicit rather than assumed.

## 2. What the two exported `*ClassName` helpers are for

Traced against every live call site (`grep -rn '<Dropdown' web/src/features`,
7 renders across 4 files):

| call site | positioning | content |
|---|---|---|
| `provider-switch-dropdown.tsx` (populated) | anchor | `items` |
| `provider-switch-dropdown.tsx` (empty state) | anchor | `children` — a `<p>` |
| `file-explorer-tree.tsx` (filter menu) | anchor | `items` |
| `file-path-breadcrumb.tsx` | **point** | `children` — `<Button>`s styled with `dropdownItemClassName` |
| `editor-status-actions.tsx` (language) | anchor | `children` — a search input + `<Button>`s |
| `editor-status-actions.tsx` (LSP status) | anchor | `children` — bespoke status rows |
| `editor-status-actions.tsx` (view menu) | anchor | `children` — `<Button>`s styled with a locally-defined `menuItemClass` |

**5 of 7 pass `children`, not `items`/`sections`.** `dropdownTriggerClassName`
and `dropdownItemClassName` exist because most call sites build their **own**
trigger/item elements and want this family's look merged onto them — the same
relationship `buttonVariants` has to anything button-shaped that isn't
literally `<Button>`. `MenuItemsList` (the primitive's own row renderer) is
the **minority** shape in practice.

This is the finding that shapes §3's scope decision: since the majority of
real usage is call-site-built content, and the two structured-`items` call
sites are themselves narrow (a filter menu, a provider list), following
`popover`'s own precedent — *"the body is the call site's, a different sub-UI
at every render; the port takes its measured extent rather than reproducing
one"* — is not a shortcut here, it is the same reasoning applied to a primitive
with *more* content diversity than `popover.tsx` has, not less.

## 3. Wrap or build — the test, applied

§10.1 names `popover` (among others) as a `gpui-component` primitive to wrap.
`dropdown.tsx` is not itself one of `gpui-component`'s named primitives, but it
*renders through* the same underlying shape a wrap already covers: a
deferred, anchored floating panel. The seam test from this item's brief:

> A widget is wrappable-and-measurable exactly when it lets the caller supply
> an *element*, not merely a style.

`gpui_component::popover::Popover` passes this test — `popover.rs`
(P3.15) already proved it converges at 0 deltas, using
`ParentElement`/`.child(...)` to hand the vendor an already-anchored box built
by this crate. Nothing about `dropdown.tsx`'s different **positioning
algorithm** changes whether that seam exists; it changes only what has to be
proven before reusing it, which is §4 below.

**The sidebar lesson, applied.** `sidebar`'s finding was that an element seam
is necessary but not sufficient — the caller's element must be able to *BE*
the box, not merely sit beside it. Here: with `GpuiPopover::appearance(false)`,
the vendor's own content box (`render_popover_content`, a private `v_flex()`)
paints nothing, and the box passed through `.child(root)` — this crate's own
`div()`, carrying every one of `dropdownRootVariants`' Tailwind values — *is*
the box the differ measures under `ID_ROOT`. Verified the same way `popover`'s
was: by reading `render_popover_content`'s source, not by assuming
`appearance(false)` behaves as documented.

**Built, not wrapped: the two style-only exports.** `trigger()`/`item()` in
`dropdown.rs` are free functions over a caller's `Styled` element, mirroring
`dropdownTriggerClassName`/`dropdownItemClassName` exactly — no vendor
involvement, because there is no vendor concept of "a row styled like a
dropdown item" to wrap. They carry no anchor and no `row_layout` capture: no
current Rust call site needs them (the 4 React call sites that use them are
not yet ported), so building the seam now and proving it against a capture
later — rather than fabricating a capture for code nothing renders yet — is
the same discipline `select`'s and `sheet`'s "no surface"/"unreached" findings
already established for this codebase.

## 4. Why the positioning algorithm does not need to be reproduced

The one finding this item adds beyond restating `popover`'s: **`dropdown.tsx`
computes its own anchor-vs-point, side-flip, viewport-clamp geometry, and none
of it needs a native equivalent**, because `ANCHORS.md` §4 fixes the
comparison space:

> All `bounds` are logical pixels **relative to the `root` anchor's top-left**,
> with the root itself at `{x: 0, y: 0}`.

The root's own *absolute* page position is never part of the contract — only
its **size** (`bounds.w`/`bounds.h`) and its children's bounds *relative to
it* are compared. Whatever corner `dropdown.tsx`'s JS resolved to, whatever
side it flipped to, whether the trigger was a real element (`anchorRef`) or a
synthesised point (`point`) — all of it is **placement**, the same §6-invisible
category `popover`'s own module docs put Floating UI's output in and
`dropdown-menu`'s put CSS anchor positioning in.

This is what makes wrapping `gpui_component::Popover` **unconditionally at its
default `Anchor::TopLeft`** — exactly as `popover.rs` already does, with no
override — sufficient for both of `dropdown.tsx`'s positioning modes without
modelling either. `crowbar-ui`'s `Dropdown::render` never asks which mode a
cell is in; it only has to get the root's **size** right, which is a declared
parameter (§5), not a derived one.

## 5. One structural difference from `popover`: one box, not two

`PopoverPrimitive` splits its popup and its viewport into two nested boxes
with two different paddings. `dropdown.tsx`'s root (`m.div`,
`dropdownRootVariants`) carries its own `p-1` **and** is the scrolling
viewport (`overflow-y-auto`) in the same box; its child, `<div role="menu"
className={menuClassName}>`, carries no default padding of its own (`menuClassName`
is `undefined` at every live call site inspected). So this module has exactly
**one** anchor, `ID_ROOT` — not an omission, the shape the markup actually
has, confirmed live (§7's DOM chain).

## 6. Values, measured against the app's own Tailwind

| React / Tailwind (`dropdownRootVariants`) | Compiles to | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `rounded-xl` | `border-radius: 14px` | `theme.radius_xl` | `radius` = 14 — **distinct from `popover-popup`'s `radius_lg` (10)** |
| **`border border-border`** | `border-width: 1px`, `oklch(1 0 0 / 0.06)` | `.border(BORDER_WIDTH).border_color(theme.border)` | **`border.w` = 1, compared exactly** — a real border like `popover`'s, the inverse of `dropdown_menu`'s `ring-1` trap |
| `bg-card/95` | `color-mix(in oklab, var(--card) 95%, transparent)` | `theme.card.mix(95.0, Color::TRANSPARENT)` | `bg` = `#1f1f1ef2` (dark) |
| `p-1` | `padding: 4px` | `PADDING = px(4.0)` | drives `bounds` on every child |
| `min-w-[240px]` | `min-width: 240px` | `UNNARROWED_FLOOR = 240` — **no live call site renders it unnarrowed** | absent from every reachable cell |
| `max-w-[min(480px,calc(100vw-16px))]` | a viewport-relative cap | not modelled — no reachable cell approaches it | placement/sizing, not compared once resolved |
| `overflow-y-auto` | `overflow-y: auto` | `.overflow_hidden()` | the same trade `popover`'s viewport and `dropdown_menu`'s popup both make — gpui's *scrolling* overflow lives on `StatefulInteractiveElement` |
| `select-none`, `pointer-events-auto`, `z-[10040]`, `[overscroll-behavior:contain]` | — | not painted | §6: no field, either side |
| `backdrop-blur-sm` | `backdrop-filter: blur(4px)` | not painted — gpui has no backdrop filter | §6, the same absence `popover`'s `not-dark:bg-clip-padding` is |
| `shadow-[0_14px_30px_-24px_rgba(0,0,0,0.45)]` | a custom box-shadow | `.shadow_lg()` (fidelity, not a match) | §6: no field, either side, the same trade `popover`'s `shadow_lg()` is |

**The border is the same trap `popover`'s module docs already name, hit again
in the same direction.** `dropdown_menu`'s `ring-1` compiles to a box-shadow
and reports `border.w: 0`; here the class is a bare `border`, 1px real, and
`border.w: 1` — the field `ANCHORS.md` v1.1 compares *exactly*. Third
component in the tree to make this point in `popover`'s direction, after
`dropdown_menu` made it in the other.

**`rounded-xl`/`bg-card` are this surface's own tokens, not `popover`'s** —
worth stating because `theme.card` and `theme.popover` turned out to be the
**same colour** on measurement (§8 below), which could read as evidence the
two primitives converge. They do not: the radius differs (14 vs 10) and the
`/95` mix leaves this shell's composited alpha at `0.95` where `popover-popup`
paints fully opaque — checked as the actual `Paint` the differ reads, not as
the pre-mix token.

## 7. Reachability, and the live element identity

**Reachable: `file-explorer-tree.tsx`'s filter menu**, driven live
2026-08-02 against the running dev-desktop app (`oracle-fixture/home`
workspace) via `Show sidebar` → `Files` tab → the funnel icon beside the
search box. `document.hasFocus()` false throughout; `.click()` on the trigger
reached React and opened it.

**Identity evidence**, read off the live DOM before any attribute was
injected:

```
className: "pointer-events-auto fixed z-[10040] max-w-[min(480px,calc(100vw-16px))]
            select-none overflow-y-auto rounded-xl border border-border bg-card/95 p-1
            shadow-[0_14px_30px_-24px_rgba(0,0,0,0.45)] backdrop-blur-sm
            [overscroll-behavior:contain] w-fit min-w-fit"
data-slot: null
innerText (of the 3 real menuitem primitives): "Hidden Files", "Gitignored Files", "Git Status"
role="menuitem" count: 3
inline style at capture: transform-origin: right top; visibility: visible; opacity: 1;
                          transform: none; max-height: 934px; width: 197.378128px;
                          left: 1508px; top: 183px;
```

`transform: none; opacity: 1` — settled, not mid-transition. The DOM chain
from a menuitem button up: `button[role=menuitem]` → unstyled `div`
(`MenuItemsList`'s wrapper) → unstyled `div[role=menu]` (`menuClassName`
undefined at this call site) → the `fixed …` root above → `body`. Matches §1's
architectural reading exactly: one styled box, containing an unstyled
`role="menu"`, containing the rows.

**The reachable menu has a separator the live measurement exposed.** Root
height 105px; three `dropdownItemVariants` rows measured at 30px each
(188–218, 218–248, 253–283) — a 5px gap between rows 2 and 3, which is
`my-0.5 border-t` (`4px` margin + `1px` border): `fileTreeFilterMenuItems`
groups "Hidden Files"/"Gitignored Files" apart from "Git Status" with a
`separator: true` entry. `30 + 30 + 5 + 30 = 95`, and `105 − 2×1 − 2×4 = 95` —
the two derivations agree, which is what makes `body_height = 95` the
component's real content rather than a guess.

## 8. `content_sized` — measured, and it overturned this item's own first draft

The first version of this module reasoned that `dropdown.tsx`'s width is
**not** content-sized: `applyLockedWidth` (`dropdown.tsx:440`) reads the
menu's natural width once and writes it back as an explicit
`style.width`, so by the time any capture runs, the used width should be a
definite pixel value — the same trade `popover`'s width is.

**Live measurement refutes it, for the reachable cell.** The captured inline
style read `width: 197.378128px` — the locked value — but
`getComputedStyle(root).width` (and `offsetWidth`, and what
`getBoundingClientRect()` reports, and what the extractor actually reads) was
`201.40625px`. The reason is CSS, not a bug: `file-explorer-tree.tsx` passes
`className="w-fit min-w-fit"`, and **`min-width` wins over a conflicting
`width`** per the CSS sizing spec. `min-w-fit` is `min-width: fit-content`,
which re-measures the row content on every layout regardless of what the lock
wrote — so the rendered width tracks content, not the lock.

**`data-oracle-content-sized` is therefore declared on `dropdown.tsx`'s root**
(the only React edit beyond `data-oracle-id`), and `Dropdown`'s
`ID_ROOT` anchor declares `content_sized()` to match. Per `ANCHORS.md` v1.5,
that makes the compared width `ceil(reference.w)` — `ceil(201.40625) = 202` —
which is exactly what `crowbar-ui`'s `DEFAULT_WIDTH` is set to; the binary's
own emitted `w: 202.0` and the reference's `w: 201.41` (already rounded to
2dp by the extractor) satisfy the rule by construction, not by tolerance.

**This does not generalise to every call site**, and that is recorded rather
than smoothed over: `provider-switch-dropdown.tsx` passes `className="min-w-0"`
*with* an explicit `style.width`, which clears the conflicting floor — its
rendered width there *is* the locked pixel value, un-content-sized. The
declaration on `dropdown.tsx`'s root is therefore true of **this reachable
cell**, stated as such, the same epistemic shape `popover::Variant::Tooltip`'s
"no live call site" note and `dropdown_menu`'s reachability caveats already
have in this codebase. A future item porting `provider-switch-dropdown.tsx`'s
own call site will need its own cell, not a re-use of this one's declaration.

## 9. The reference

`/tmp/p3-ref-dropdown.json`, captured via `extractSnapshotSource` against the
live element (§7), **not fabricated**: `data-oracle-id="dropdown-root"` and
`data-oracle-content-sized="true"` were injected with `setAttribute`
immediately before the capture (the dev server serves the **shared**
worktree, so this branch's `dropdown.tsx` edit is not live there) and removed
immediately after, then the sidebar was closed again to leave the shared
session as it was found:

```json
{
  "schema": 1,
  "surface": "dropdown",
  "state": { "width": 1714, "theme": "dark", "content": "normal", "flags": [] },
  "root": "dropdown-root",
  "anchors": [{
    "id": "dropdown-root",
    "bounds": { "x": 0, "y": 0, "w": 201.41, "h": 105 },
    "bg": "#1f1f1ef2", "visible": true, "radius": 14,
    "border": { "w": 1, "color": "#ffffff0f" },
    "content_sized": true
  }]
}
```

`theme` omitted from the *capture request* (derived from the live document,
which is what `oracleNormalizeState` reports back as `"dark"`); `width: 1714`
is the live `window.innerWidth` at capture time, per this item's brief.

**The binary's own emission at the matching cell**, self-checked (this item
does not run the oracle/differ — no verdict is claimed):

```
$ crowbar-app --surface dropdown --viewport-width 1714 --theme dark
crowbar-app: dropdown · 320px in a 1714px viewport · dark · normal · flags -
             · depth 2 · shell-width 202px · body 95px
{
  "id": "dropdown-root",
  "bounds": { "x": 0.0, "y": 0.0, "w": 202.0, "h": 105.0 },
  "bg": "#1f1f1ef2", "visible": true, "radius": 14.0,
  "border": { "w": 1.0, "color": "#ffffff0f" },
  "content_sized": true
}
```

Every field but `w` agrees to the character; `w` agrees under the v1.5 rule
`CONTENT_SIZED` declares. `bg`, `radius` and `border` are exact matches
measured independently on each side — not carried over from one to the other.

## 10. Declarations

* `CONTENT_SIZED = [dropdown-root]`. §8 carries the measurement and the
  caveat.
* `LINE_SIZED = []`. The root paints no text of its own — its one anchor is a
  box, not a run — the same reasoning `popover`'s popup (not its title) rests
  on.

## 11. State axis

| flag | here |
|---|---|
| `hover` | unmodelled — the one live `hover:bg-muted` rule is on `MenuItemsList`'s rows, which this surface does not render (§3) |
| `focus` | unmodelled, same reason — `focused` is a row axis |
| `selected` | unmodelled — `dropdown.tsx` has no selected/checked concept at all, unlike `dropdown-menu.tsx`'s tick rows |
| `loading`, `error` | unmodelled, as on every surface so far |
| `empty` | **real** — a zero-height body is `provider-switch-dropdown.tsx`'s own no-other-provider branch, a genuine reachable shape, not merely a shape the port can draw |

## 12. What resisted, and what did not

**Strict parity is reached on every field the contract carries**, on the
reachable cell, by the binary's own self-check (§9) — this item's own
verification, not the oracle's; no diff was run. What resisted was not a
styling property but an **assumption**: this module's first draft declared
`CONTENT_SIZED = []` on the theory that the lock mechanism made the width
definite everywhere, and only a live capture caught that it does not on the
one call site a parity run can actually reach. §8 carries the correction in
full, left in rather than quietly fixed, because it is the more useful record.
