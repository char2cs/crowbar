# `card` (P3.10)

`web/src/components/ui/card.tsx` → `crates/crowbar-ui/src/components/card.rs`.

**No reference.** See §0.

## 0. ⛔ Reachability — 0 live, and reaching one means introducing a defect

`[data-slot=card]` measured **0** in the running app. The single importer is
`components/error-boundary.tsx`, whose fallback renders only when
`getDerivedStateFromError` has fired — a **render-phase throw** — *and* the
boundary was given no `fallback` prop.

Boundaries in the tree:

| Site | `fallback` prop? | Reaches a Card? |
|---|---|---|
| `components/layout/ide-shell.tsx:152` (SidebarCarousel) | no | yes, on a throw |
| `components/layout/ide-shell.tsx:162` (WorkspaceHost + Outlet) | no | yes, on a throw |
| `components/layout/sidebar-carousel.tsx:130` (FileExplorerTree) | no | yes, on a throw |
| `features/editor/markdown/plate/markdown-editor-pane.tsx:89` | **yes** (`FallbackToSource`) | **no** |

All three fallback-less boundaries wrap ordinary app subtrees, so tripping one
means *causing a bug*. The one documented render throw in the tree —
`mermaid-diagram.tsx`'s, on invalid diagram syntax — is caught by the boundary
that **does** pass a fallback, so it never produces a Card.

An error boundary's fallback is by construction the picture the app shows when it
is broken. It was not driven and **no reference JSON was fabricated**;
`separator` and `skeleton` are the precedent.

## 1. ⚠ v1.8: this surface **must not** declare its anchor set

v1.8 permits a declaration "only when that set is a property of the surface
rather than of the cell". A Card fails that by construction: every slot is
optional and the call site chooses which to fill, so `card-header`, `card-title`
and `card-panel` are each present or absent per cell. A fixed list would be wrong
in most cells and the loud-missing rule would then reject honest captures —
`git-status-row`'s standing, for the same reason.

> **Consequence, flagged rather than worked around.** With no declaration, an
> `ErrorBoundary` capture would walk the whole subtree and also pick up the
> `button` anchor of the **Try again** control, which is another surface's.
> Resolving that needs *either* a call-site rename in `error-boundary.tsx`
> (`git-row-badge`'s route) *or* an `oracleSurfaceScope` entry — and both are the
> orchestrator's to choose. **This port did not edit `extract.ts`.** The native
> side renders the boundary's content unanchored, so the two sides would differ
> by that one anchor if the surface ever became capturable.

## 2. Where the values came from

The utilities, resolved through the app's own compiled `tailwindcss` 4.3.0 and
read back off a probe element inserted into the live document (with the
`data-slot` attributes, which the `in-[…]` variants key on) and removed again.

| React / Tailwind | Compiles to | Probe | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|---|
| `rounded-2xl` | `calc(var(--radius) * 1.8)` | **18px** | `theme.radius_2xl` | compared |
| `border` | 1px | **1px, painted** | `.border_1()` | compared |
| `bg-card` / `text-card-foreground` | `--card` / `--card-foreground` | — | `theme.card`, `theme.card_foreground` | compared |
| `max-w-sm` | `--container-sm` = 24rem | **384px** | `MAX_WIDTH` (a plain constant — `--container-*` is Tailwind's own scale and is **not** one of the 180 names `theme.css` declares) | compared |
| `bg-destructive/10` / `border-destructive/20` | — | α 0.10 / 0.20 | `theme.destructive.mix(10.0 \| 20.0, TRANSPARENT)` | compared |
| `p-6` | `calc(var(--spacing) * 6)` | 24px | `PADDING` | compared |
| `gap-1.5` | `calc(var(--spacing) * 1.5)` | 6px | `HEADER_GAP` | compared |
| `font-semibold` | `--font-weight-semibold` | 600 | `TITLE_WEIGHT` | compared |
| `text-lg` / `leading-none` | `--text-lg`; ratio forced to 1 | 18px / 18px | `TITLE_STEP` | compared |
| `shadow-xs/5`, `before:shadow-[…]` | — | — | **absent** — §6: shadows have no representation, and the `::before` overlay is unanchored on both sides (`button`'s precedent) | invisible |
| `not-dark:bg-clip-padding` | `background-clip` | — | **absent**, no field | invisible |

`border` is **1px on the card** — `badge`'s and `button`'s trap, the opposite of
`kbd`'s. Measured, not inferred.

## 3. ⚠ The `sm:` trap in a **third** guise: `in-[…]` beats the call site

Three slots carry a variant keyed on what the *card contains*:

```
CardHeader  in-[[data-slot=card]:has(>[data-slot=card-panel])]:pb-4
CardPanel   in-[[data-slot=card]:has(>[data-slot=card-header]:not(.border-b))]:pt-0
CardPanel   in-[[data-slot=card]:has(>[data-slot=card-footer]:not(.border-t))]:pb-0
```

`error-boundary.tsx` writes `pb-2` on the header. **The probe measures 16px, not
8.** Different tailwind-merge modifiers keep both classes and Tailwind emits the
variant later, so the call site loses — the same mechanism as `badge`'s `sm:`,
and as `label`'s in reverse, with a third kind of prefix.

**And one slot away, the call site *wins*:** `CardTitle`'s `text-sm` and the
primitive's `text-lg` are the same tailwind-merge group, so `cn(…)` drops the
primitive's and the probe reads **14px**. Neither fact is readable from the class
list; both are asserted in `row_layout/card.rs`.

This is why `Card` is modelled as a **set of slots** rather than as one box: the
header's bottom padding and the panel's two edges are functions of which siblings
exist, and `--slots` is the axis that moves them.

## 4. gpui has no CSS grid

`CardHeader` is `grid auto-rows-min grid-rows-[auto_auto]`, with
`has-data-[slot=card-action]:grid-cols-[1fr_auto]` for a second column.

**No live call site passes a `card-action`**, so every real header is a single
column of auto-height rows separated by `gap-1.5` — which *is* a flex column with
a gap, and that is what the port renders. The two-column arrangement is **not
ported and not approximated**: `Slots::action` is a private field with no setter,
so no cell can ask for the picture gpui cannot lay out.

## 5. Declarations

`CONTENT_SIZED` is **empty** — the card is `w-full` under `max-w-sm` and every
slot is a stretched item. `LINE_SIZED` is `["card-title"]` alone: it authors no
height, and `leading-none` makes its line box exactly its font size, so the box
*is* the line box (probe: 14 / 14 / 14). The React side declares it
**conditionally**, on `Children.count(...) > 0`, for `label.tsx`'s reason — v1.6
refuses the declaration on a box painting no text, and the native half withholds
it in the same case.

## 6. ⚠ An unattributed 2px, recorded rather than explained away

With `--slots footed` the panel's padding sums to zero and its content is empty,
so it should measure **0**. It measures **2**.

Every other slot cell is exact: `panel-only` is 48 (`pt` + `pb`),
`header-and-panel` is 24 (`pb` alone). The residue appears **only** where the
padding sums to nothing, and it is exactly the card's own `border_1()` top and
bottom — which is the shape of a taffy quirk (P3.1 found one with negative
margins) rather than a coincidence.

**The attribution is not proven**, and no control here can vary it: both call
sites carry a bare `border`, so there is no zero-border card to compare against.
`the_panel_padding_follows_the_slots_around_it` therefore asserts a *bound* — the
footed panel is shorter than the unfooted one and holds less than half a `p-6` —
rather than a number nobody can justify. Written down so that if this ever
matters, the cause is one grep away.
