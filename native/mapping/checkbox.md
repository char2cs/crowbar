# `checkbox` (P3.9)

`web/src/components/ui/checkbox.tsx` →
`crates/crowbar-ui/src/components/checkbox.rs`.

> A §6.2 row, in the shape `native/MAPPING.md` fixes. Kept in its own file
> because P3 runs several workers in parallel and one appended table is one
> conflict per item.

A base-ui `Checkbox.Root` box with a `Checkbox.Indicator` fill and an `<svg>`
tick inside that. Every "Compiles to" below was **measured on the live element**
with `getComputedStyle`.

**References:** `/tmp/p3-ref-checkbox.json` and
`/tmp/p3-ref-checkbox-selected.json`, captured live from the Git panel's
"Commit changes" popover at a 1714px viewport.

**Live count: 4 call sites** — `commit-popover.tsx` (one per changed file,
`disabled` while committing), `merge-popover.tsx`, `block-list-todo.tsx` (the
only one passing a `className`, and it is `-left-6 absolute top-1` — position
only, no visual property), `repo-import-dialog.tsx`.

## 0. The headline: `selected` moves **one** field, and only in dark

| field | off | on |
|---|---|---|
| `checkbox.bg` | `#ffffff07` | `#1f1f1eff` |

Nothing else. `bounds`, `radius`, `border.w`, `border.color` and `visible` are
identical across the two files.

**And in the light table it moves nothing at all.** The rule carrying the
difference is `dark:not-data-checked:bg-input/32` — a `dark:` variant. Below it
the box is `bg-background` in *both* states, so a `--flags selected` cell driven
at `--theme light` has every recorded field identical and **cannot fail**. The
surface says so in its own caption on that cell, and
`selected_repaints_the_box_in_dark_and_not_in_light` asserts both halves, so a
green light-theme run cannot be mistaken for coverage.

## 1. Values — the box (the only anchor)

| React / Tailwind | Compiles to (measured) | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `size-4.5` | `18 × 18` | `extent(Breakpoint::Base)` | — |
| `sm:size-4` | `16 × 16` **(live)** | `extent(Breakpoint::Sm)` | `bounds.w`/`h` = 16 |
| `relative` | `position: relative` | `.relative()` | containing block for the fill |
| `inline-flex` | computed `display: flex` | `.flex()` | not a field |
| `items-center justify-center` | centres the tick | `.items_center().justify_center()` | not a field |
| `shrink-0` | `flex-shrink: 0` | `.flex_shrink_0()` | not a field |
| `rounded-[.25rem]` | `border-radius: 4px` — an **arbitrary value**, not a `--radius-*` step | `RADIUS` | `radius` = 4 |
| `border` | `border-width: 1px` | `BORDER_WIDTH` | `border.w` = **1**, compared exactly |
| `border-input` | `oklch(1 0 0 / 0.08)` | `theme.input` | `border.color` = `#ffffff14` |
| `bg-background` | `oklch(0.239 0.002 106.5)` | `theme.background` | `bg` = `#1f1f1eff` (checked) |
| `dark:not-data-checked:bg-input/32` | `oklab(1 0 0 / 0.0256)` | `theme.input.mix(32, TRANSPARENT)` | `bg` = `#ffffff07` (unchecked) |
| `aria-invalid:border-destructive/36` | a border colour | `INVALID_BORDER_ALPHA` | `border.color` — **compared**, no reference |
| `focus-visible:aria-invalid:border-destructive/64` | a border colour | `INVALID_FOCUS_BORDER_ALPHA` | `border.color` |
| `focus-visible:ring-2` | a **box-shadow** | `RING_WIDTH` | §6 — *not* a border |
| `focus-visible:ring-offset-1` | a second shadow layer | `RING_OFFSET` | §6 |
| `focus-visible:aria-invalid:ring-destructive/48` | a shadow colour | `INVALID_RING_ALPHA` | §6 |
| `dark:aria-invalid:ring-destructive/24` | a shadow colour | `INVALID_RING_ALPHA_DARK` | §6 |
| `shadow-xs/5` | `0 1px 2px 0 rgb(0 0 0 / .05)` | `.shadow_xs()` | §6 |
| `[[data-disabled],[data-checked],[aria-invalid]]:shadow-none` | drops it | `Checkbox::has_shadow` | §6 |
| `data-disabled:opacity-64` | `opacity: 0.64` | `DISABLED_OPACITY` | no field; not v1.7's zero |
| `not-dark:bg-clip-padding` | `background-clip` | — | no field |
| `transition-shadow`, `outline-none`, `ring-ring`, `data-disabled:cursor-not-allowed` | not visual / §6 | — | not fields |
| `before:absolute before:inset-0 before:rounded-[3px]` + two inset shadows | the `::before` overlay | `Checkbox::overlay` | unanchored — see §4 |

## 2. Values — the fill and the tick (**unanchored** — see §3)

Measured live in the checked state and reproduced by the port, but carried by no
snapshot on either side:

| React / Tailwind | Compiles to (measured) | gpui / `crowbar-ui` |
|---|---|---|
| `absolute -inset-px` | `(0, 0, 16, 16)` relative to the box | `.top(-INDICATOR_INSET).left(…)` |
| `rounded-[.25rem]` | `border-radius: 4px` | `RADIUS` |
| `data-checked:bg-primary` | `oklch(0.49 0.082 130)` | `theme.primary` |
| `data-unchecked:hidden` | `display: none` | not rendered |
| `text-primary-foreground` | `oklch(0.98 0.027 98)` | `theme.primary_foreground` |
| `data-indeterminate:text-foreground` | the foreground token | `theme.foreground` |
| `<svg class="size-3.5 sm:size-3">` | `12 × 12` at `(2, 2)` | `Checkbox::glyph` — a box, no path |
| `stroke="currentColor" strokeWidth="3"` | `stroke-width: 3px` | — (no field) |

The `-inset-px` lands the fill exactly on the box's **border** box, because the
containing block is the box's *padding* box — inset by the 1px border all round —
and a `-1px` on each side puts it back. `INDICATOR_INSET` is written as
`BORDER_WIDTH` rather than as another `px(1.0)` so the relationship is visible.

## 3. Why the indicator carries no `data-oracle-id`

**The interesting decision on this component**, and the one place this item
leaves coverage on the table deliberately.

`data-unchecked:hidden` is `display: none`, and the two extractors do different
things with that:

| | an anchor at `display: none` |
|---|---|
| `web/src/lib/oracle/extract.ts` | **emitted.** base-ui keeps the element mounted, so the walk finds it; `oracleIsVisible` returns `false` and `getBoundingClientRect()` returns all zeros, which §4's root-relative arithmetic turns into `bounds: { x: −330, y: −406, w: 0, h: 0 }` — the *viewport* origin expressed against the root |
| `crowbar-driver` | **absent.** `ANCHORS.md` §6: "`display: none` is caught implicitly — prepaint never arrives and the anchor is simply absent" |

Verified on the live element rather than reasoned about: unchecked,
`checkbox.children.length` is still `1`,
`getComputedStyle(indicator).display === "none"`, and
`indicator.getBoundingClientRect()` is `{x: 0, y: 0, width: 0, height: 0}`.

So anchoring the fill would put a **structural** delta on this surface's
*default* cell — an anchor present on one side only — whose cause is the contract
rather than the port.

Both available repairs are worse than the omission:

- **render a zero-area box at the root's negated viewport position natively.**
  This is writing the reference's *output* into the port in its purest form —
  the number is literally a coordinate only the reference can know. It is the
  knob P3.2 refused for `tab-indicator` and P3.1 refused for `--class-radius`.
- **render the fill unconditionally and let `visible` carry it.** Then the boxes
  disagree on `x`, `y`, `w` **and** `h` as well: four manufactured deltas instead
  of one.

`ANCHORS.md` v1.8 offers no third way. A surface may declare its anchor set "only
when the set is a property of the surface rather than of the cell", and this set
is a function of the cell — exactly `git-status-row`'s `git-row-dir`.

**Consequence, stated rather than left to be inferred:** the green `bg-primary`
fill, its 4px corner, and the tick's `12 × 12` box are painted by both sides and
compared by neither. `selected` therefore has **one** field here where `switch`
has two. `the_anchor_set_is_one_and_does_not_change_with_the_state` pins the
decision so a later change cannot re-introduce the problem quietly.

> **This is a hole in the contract, not only in this port** — the second of its
> kind after v1.7's opacity split. §6 states the GPUI side's behaviour for
> `display: none` and says nothing about the DOM side's, and the two differ.
> Reported rather than patched: this item may not edit `native/oracle/`.

## 4. The `::before` overlay is unanchored too, for `input`'s reason

§3's pseudo-backed shortcut is *legal* here — the pseudo really is
`position:absolute; inset:0` — and taking it would still be wrong, because a
pseudo-backed anchor **replaces** the host's record. It would throw away the
box's own background, its 1px border and its 4px corner, which are the entire
surface.

Its two inset shadows are not painted: they need Tailwind's own black and white
(`--theme(--color-black/4%)`, `--theme(--color-white/6%)`), which
`check-invariants.sh` rule 4 will not let a component mint. The same wall
`input`'s overlay meets, recorded there in the same words.

## 5. `border` is 1px — `button`'s trap, third occurrence

`checkbox.tsx` writes a bare `border`, so `border.w` is `1` in every state and
v1.1 compares it exactly. The **mirror** trap is on the same component:
`focus-visible:ring-2` is a box-shadow, so a focused box's `border.w` is still 1,
never 3. A worker who has just learned one of these is the most likely person to
get the other backwards, which is why both are asserted.

## 6. Two arbitrary radii, neither a token

`--radius-sm` is 6, `--radius-md` is 8, `--radius-lg` is 10. The box's corner is
`rounded-[.25rem]` = **4** and the overlay's is `rounded-[3px]` = **3**. They
differ by a pixel, exactly as `input`'s control and overlay do — but there the
class *is* the arithmetic (`calc(var(--radius-lg)-1px)`) and here the two are
independently authored flat values. Writing the 3 as `RADIUS - 1px` would invent
a relationship the class list does not have, so it is its own constant.

## 7. `error` is real and still declared unmodelled

`aria-invalid:border-destructive/36` moves `border.color`, which the differ
compares. So the honest declaration would be that `error` is modelled.

It cannot be: `surface.rs`'s `no_surface_declares_its_entire_state_axis_unmodelled`
asserts `unmodelled(Error)` for *every* registered surface. The state is driven
by `--invalid`, exactly as `input`'s is — and `input`'s notes predicted this
("`select`, `checkbox`, `radio-group` and `textarea` carry the same four rules and
will hit this again"). They do.

It costs nothing today: **no `<Checkbox` in `web/src/` passes `aria-invalid`**, so
the cell has no reference either way.

## 8. Where this component differs from `input`, which it otherwise mirrors

Three places, each of them the sort of thing a port copies across by habit:

| | `input` | `checkbox` |
|---|---|---|
| bare `focus-visible:border-*` | **yes** (`has-focus-visible:border-ring`) — focus alone moves `border.color` | **no** — focus alone moves nothing |
| what drops the shadow | disabled, **focus**, invalid | disabled, **checked**, invalid |
| `ring-offset-*` width | absent, so the offset layer stays at its `0px` initial | present (`ring-offset-1`), so it paints |

## 9. `indeterminate` is a third state with no live call site

base-ui's `Checkbox` has three values, not two. `indeterminate` takes the *same*
`data-checked:bg-primary` fill and differs only in the glyph (a dash) and its
colour (`data-indeterminate:text-foreground`). Since the glyph is a box with no
path here and sits on the unanchored element, **it moves nothing recorded** —
that cell cannot fail.

All four live call sites pass a plain boolean `checked`, so there is no reference
for it either. It is modelled anyway, behind `--indeterminate`, because leaving
it out would make the port quietly smaller than the component.

## 10. What the oracle cannot see here

- the green fill, its corner, and the tick — see §3;
- the `::before` overlay and its two inset shadows — §4;
- `shadow-xs/5`, the focus ring and all four invalid ring colours — §6;
- `transition-shadow`, `bg-clip-padding`, `cursor-not-allowed` — not fields;
- **nothing in the text group**: the tick is an `<svg>`, which has element
  children rather than text nodes, so no `text`/`fg`/`text_width`/`clipped`/`font`
  is emitted on either side and `--content` is vacuous on every cell.
