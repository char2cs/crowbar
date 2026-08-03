# `detach-holder-modal` (P3.51)

`web/src/components/layout/detach-holder-modal.tsx` →
`crates/crowbar-ui/src/components/detach_holder_modal.rs`,
`crates/crowbar-app/src/surfaces/detach_holder_modal.rs`.

> Cluster 2, "standalone modals" (`native/mapping/layout-denominator.md` §8).
> Grouped with `repo-import-dialog` only because both are a dialog instance
> with its own text/buttons/a request — neither imports the other.

**Reference: none.** The one live call site opens only when a protected
branch is held by another worktree — state no capture session has had on
hand (`dialog.md` §5 names this exact gap). This doc instead records the
surface and its `row_layout` coverage that already exist, derived from what
the code and its tests actually assert — this item's own brief is
documentation of prior work, not new porting.

## 0. Not a second primitive — a call site of `dialog`'s

`detach-holder-modal.tsx` renders exclusively through `dialog.tsx`'s
`Dialog`/`DialogPopup`/`DialogHeader`/`DialogTitle`/`DialogDescription`/
`DialogFooter` — the identical primitive `dialog.rs` already wraps.
`dialog.tsx`'s own `data-oracle-id`s (`"dialog-popup"`, `"dialog-header"`, …)
are hardcoded literals in that file, not passed through from a call site, so
the *real* DOM this call site paints carries the same `dialog-*` ids
`add-repository-modal` does — confirmed by reading `dialog.tsx` directly, not
assumed.

**So why does this module mint its own `detach-holder-modal-*` ids at all?**
A registry constraint, not a rendering difference: two surfaces cannot share
a root (`surface.rs`'s own `every_registered_surface_has_its_own_name_and_root`),
and `dialog`'s own surface already claims `dialog-popup`. Anchoring this call
site under the same id would either collide with `dialog`'s registration or
require the two surfaces to compare snapshots that both claim to be "the"
`dialog-popup` — so this module's ids are a namespace assigned to satisfy the
registry, not evidence that the real DOM disagrees with `dialog.tsx`'s own
attributes. `crowbar_ui::components::detach_holder_modal`'s own module docs
carry the same finding; this section is its mapping-doc twin.

## 1. What is genuinely this call site's own, not `dialog`'s

Two real `className` overrides `dialog.tsx`'s own primitive does not carry —
the actual reason this is its own module rather than a second construction
of `dialog::Dialog`:

| | `dialog`'s `add-repository-modal` cell | `detach-holder-modal.tsx` |
|---|---|---|
| `DialogHeader` | `p-6` (24px, all four sides) | `p-6 pr-10` — tailwind-merge overrides only the **right** side, to 40px |
| `DialogDescription` | *(not rendered)* | `leading-relaxed` (1.625), not the primitive's default `text-sm` (1.25/0.875) |

Everything else this module declares — `BORDER_WIDTH`, `HEADER_PADDING`,
`HEADER_GAP`, `FOOTER_PADDING_X`/`_Y`, `FOOTER_GAP`, `VIEWPORT_PADDING`,
`FOOTER_CONTENT_HEIGHT_DEFAULT`, `FOOTER_BG_TINT`, `FOOTER_RADIUS` — is
`dialog`'s own value, restated rather than re-derived
(`the_unoverridden_constants_match_dialogs_own` holds all seven equal at the
unit level).

## 2. Values

| React / Tailwind | Compiles to | gpui / `crowbar-ui` | Notes |
|---|---|---|---|
| `max-w-md` (no `sm:` prefix) | 448px, the same numeric step `dialog`'s `sm:max-w-md` resolves to at every width this port drives | `DetachHolderModal::max_width` | identical to `dialog::Dialog::max_width`'s fixture |
| `p-6 pr-10` | top/left/bottom 24px, right 40px | `HEADER_PADDING` (24) / `HEADER_PADDING_RIGHT` (40) | the one padding delta |
| `gap-2` | 8px | `HEADER_GAP` | **live** here, unlike `dialog`'s own reachable cell (title only) — this call site always nests both a title and a description |
| `leading-relaxed` | line-height 1.625 | `DESCRIPTION_LINE_HEIGHT` | the one line-height delta |
| `px-6 py-4` (footer) | 24 / 16px | `FOOTER_PADDING_X` / `_Y` | `dialog`'s own |
| `bg-muted/72` (footer) | `theme.muted` mixed 72% against transparent | `FOOTER_BG_TINT` | `dialog`'s own |
| `sm:rounded-b-[calc(var(--radius-2xl)-1px)]` | 17px | `FOOTER_RADIUS` | `dialog`'s own |
| two default-sized `Button`s in the footer | 32px content height (`sm:h-8`) | `FOOTER_CONTENT_HEIGHT_DEFAULT` | identical to `dialog`'s own default footer content height, for the identical reason |
| the two `font-mono` spans inside the description | inline runs, no box of their own | flattened into one `SharedString` | see §3 |

## 3. The description is one flattened text run

The real JSX interleaves plain prose with two
`<span className="font-mono text-foreground">` runs (the held-by path, the
branch name) inside one `DialogDescription`. `font-mono`/`text-foreground` on
an inline span move glyph rendering *inside* the description's own box, not
the box's bounds — the identical reasoning `alert-dialog.md` §2 gives for
`text-center`/`sm:text-left`: real, and not a field `ANCHORS.md` §3 tracks.
This module therefore renders the description as one flattened
`SharedString`, losing only the run-level font distinction.

## 4. The footer is unmodelled content, exactly `dialog`'s is

`detach-holder-modal.tsx`'s `DialogFooter` holds two default-sized `Button`s
(`variant="ghost"` "Cancel", default "Detach") — no `size` prop on either.
Neither button is rendered as its own anchor: `dialog::Dialog::footer` never
renders one for a real call site's buttons either, and this module holds the
same line (`FOOTER_CONTENT_HEIGHT_DEFAULT` is a plain height, not two
buttons).

## 5. Declarations

`CONTENT_SIZED = []`. `LINE_SIZED = [detach-holder-modal-title]` — the
identical arithmetic `dialog::LINE_SIZED`/`alert-dialog`'s both hold: the
title alone is `leading-none` over a padding-free box; the description keeps
its own (overridden) line height and is prose that can wrap.

## 6. `empty`, and the five flags with no original

| flag | here |
|---|---|
| `hover`, `focus`, `selected` | unmodelled — `grep -o 'hover:[a-z-]*\|focus:[a-z-]*\|active:[a-z-]*' detach-holder-modal.tsx` is empty |
| `loading`, `error` | unmodelled, as on every surface |
| `empty` | **real**, the identical arithmetic `dialog`'s own `empty` arm takes: removes the header and the footer together. No live call site actually takes this shape — the one reachable call site always renders both — the same "port it and say so" call `dialog`'s and `alert-dialog`'s own `empty` arms make |

## 7. `row_layout` coverage that already exists

`crates/crowbar-app/src/row_layout/detach_holder_modal.rs` drives the surface
in a real window and asserts, among other things:

* every contract anchor is present and no bare `dialog-*`/`alert-dialog-*` id
  ever leaks through (§0's claim, held as a test)
* the popup is `dialog`'s own 448px wide, at the origin
* border/radius/background/text colour are `dialog`'s own tokens
* **`pr-10` genuinely narrows the header's content column** — the
  description's own box is 382px wide (`446 − 24(pl) − 40(pr)`), not the 398px
  `dialog`'s uniform `p-6` would produce. **Mutation:** replacing
  `.pr(HEADER_PADDING_RIGHT)` with `.pr(HEADER_PADDING)` in
  `DetachHolderModal::header` turns this red (398, not 382).
* **`gap-2` genuinely separates the title from the description**, measured as
  the vertical gap between the two boxes' own origins rather than as a
  byproduct of the popup's total height — the test's own doc comment records
  a first draft that summed the header's own observed height and therefore
  passed unchanged even with the gap deleted. **Mutation:** deleting
  `.gap(HEADER_GAP)` turns this red (0px gap, not 8).
* **`leading-relaxed` genuinely changes the description's line height**,
  checked against the *identical* short string rendered on `dialog`'s own
  surface (20px there, taller here) rather than only against itself — the
  test's own doc comment records that a self-only check passed unchanged
  under the same mutation that changes both the numerator and denominator
  together. **Mutation:** replacing `DESCRIPTION_LINE_HEIGHT` (1.625) with
  `dialog`'s own `1.25 / 0.875` turns this red — both surfaces then render
  "Short." at the identical 20px.
* the footer's own height follows its content across three content heights
* the popup's own height is two borders plus the real (wrapped) header plus
  the footer — a self-consistency check, named as one rather than presented
  as independent (its own doc comment: "This is a *consistency* check, not an
  independent one")
* `empty` removes the header and the footer together, and the popup shrinks
  to two borders
* the light table paints a different popup

## 8. Reachability

One live call site, `ide-shell.tsx`'s detach-holder modal, gated on
`useDetachModalStore` — real app state (a protected branch held by another
worktree) this port's own sessions have never had on hand, hence no
reference. `native/mapping/layout-denominator.md` §2 already records
`detach-holder-modal.tsx`'s single importer.
