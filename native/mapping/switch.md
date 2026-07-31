# `switch` (P3.9)

`web/src/components/ui/switch.tsx` →
`crates/crowbar-ui/src/components/switch.rs`.

> A §6.2 row, in the shape `native/MAPPING.md` fixes. Kept in its own file
> because P3 runs several workers in parallel and one appended table is one
> conflict per item.

A base-ui `Switch.Root` track with a `Switch.Thumb` inside it. Every "Compiles
to" below was **measured on the live element** with `getComputedStyle` rather
than compiled from the class name, because this component's two interesting
values (`rounded-full` and the thumb's slide) are both cases where the class name
and the computed value are different things.

**References:** `/tmp/p3-ref-switch.json` and `/tmp/p3-ref-switch-selected.json`,
captured live from the Settings → Git tab at a 1714px viewport. **Both states
were on screen simultaneously** — "Compact Git Status Badges" off, "Show Git
Status In File Tree" on — so neither reference was produced by driving the other,
and neither could have been caught mid-transition as a consequence.

**Live count: 19 instances**, across six settings tabs
(`editor-settings` ×11, `file-tree-settings` ×3, `git-settings` ×2,
`developer-settings`, `terminal-settings`, `sortable-provider-row` ×1 each).

## 0. The headline: this is the first surface where `selected` is real

Every earlier Tier B surface either declared `selected` unmodelled or found it
vacuous. Here it moves **two compared fields on two different anchors**, and
nothing else:

| field | off | on |
|---|---|---|
| `switch.bg` | `#ffffff14` | `#516a36ff` |
| `switch-thumb.bounds.x` | `1` | `13` |

`bounds.w`, `bounds.h`, `radius`, `border` and `visible` are byte-identical on
both anchors across the two files. So the cell can fail, in two independent ways.

## 1. Values — the track

| React / Tailwind | Compiles to (measured) | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `[--thumb-size:--spacing(5)]` | `--thumb-size: 20px` | `thumb_size(Breakpoint::Base)` | drives every length |
| `sm:[--thumb-size:--spacing(4)]` | `--thumb-size: 16px` **(live)** | `thumb_size(Breakpoint::Sm)` | drives every length |
| `h-[calc(var(--thumb-size)+2px)]` | `height: 18px` | `Switch::track_height` | `bounds.h` = 18 |
| `w-[calc(var(--thumb-size)*2-2px)]` | `width: 30px` | `Switch::track_width` | `bounds.w` = 30 |
| `inline-flex` | computed `display: flex` | `.flex()` | not a field |
| `items-center` | `align-items: center` | `.items_center()` | not a field |
| `shrink-0` | `flex-shrink: 0` | `.flex_shrink_0()` | not a field |
| `p-px` | `padding: 1px` | `TRACK_PADDING` | puts the thumb at `(1, 1)` |
| `rounded-full` | **`border-radius: 340282346638528859811704183484516925440px`** | `TRACK_RADIUS` = `px(f32::MAX)` | `radius` = `3.4028234663852886e+38` |
| *(no `border-*`)* | `border-width: 0px` (preflight) | `BORDER_WIDTH` = 0 | `border.w` = **0**, compared exactly |
| `data-unchecked:bg-input` | `oklch(1 0 0 / 0.08)` | `theme.input` | `bg` = `#ffffff14` |
| `data-checked:bg-primary` | `oklch(0.49 0.082 130)` | `theme.primary` | `bg` = `#516a36ff` |
| `transition-[background-color,box-shadow] duration-200` | a transition | — | `ANCHORS.md` §6 |
| `focus-visible:ring-2` | a **box-shadow** | `RING_WIDTH` | §6 — *not* a border |
| `focus-visible:ring-offset-1` | a second shadow layer | `RING_OFFSET` | §6 |
| `data-disabled:opacity-64` | `opacity: 0.64` | `DISABLED_OPACITY` | no field; does not reach v1.7's zero |
| `data-disabled:cursor-not-allowed` | a cursor | — | not a field |
| `outline-none` | `outline: none` | — | not a field |

## 2. Values — the thumb

| React / Tailwind | Compiles to (measured) | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `block aspect-square h-full` | `16 × 16` | `Switch::thumb_extent` twice | `bounds.w`/`h` = 16 |
| `rounded-(--thumb-size)` | `border-radius: 16px` | `Switch::thumb_radius` | `radius` = 16 |
| `bg-background` | `oklch(0.239 0.002 106.5)` | `theme.background` | `bg` = `#1f1f1eff` — **the same on both cells** |
| `shadow-sm/5` | `0 1px 2px 0 rgb(0 0 0 / .05)` | `.shadow_sm()` | §6 |
| `data-checked:translate-x-[calc(var(--thumb-size)-4px)]` | **`translate: 12px`** | `Switch::thumb_offset`, as a **left margin** | `bounds.x` 1 → 13 |
| `[transition:translate_.15s,…]` | a 150ms transition | — | §6; v1.9's stated hole |
| `origin-left`, `data-checked:origin-[…]` | `transform-origin` | — | no field |
| `in-[…:active]:…:scale-x-110` | a `scale` | `ACTIVE_SCALE_X`, recorded only | no field, and `:active` is §6-unreachable |
| `pointer-events-none`, `will-change-transform` | not visual | — | not a field |

## 3. `rounded-full` is `f32::MAX` — second confirmation

P3.3 found this on `avatar`. It reproduces here exactly: WebKit resolves
Tailwind's `calc(infinity * 1px)` to **`f32::MAX`**, and the live track reports
`340282346638528859811704183484516925440px`. gpui's `rounded_full()` preset is
`px(9999.)` — a 3.4e38 delta on a field compared at ±0.5.

The thumb is **not** `rounded-full`; it is `rounded-(--thumb-size)`, which is
16px. A port that gave both elements the same corner would be wrong on one of
them, and the reference shows the two side by side.

## 4. The slide is `translate`, not `transform` — and it refines v1.9

`ANCHORS.md` v1.9 says WebKit's `getBoundingClientRect()` returns the
**transformed** box while gpui rotates at paint, so an animated transform moves
the reference's record and never the native one.

Measured here, the property in flight is **`translate`**, not `transform`:

```
checked thumb   transform: "none"     translate: "12px"
unchecked thumb transform: "none"     translate: "none"
```

Tailwind 4 compiles `translate-x-*` to the standalone `translate` property.
WebKit folds it into `getBoundingClientRect()` exactly as it folds a
`transform` — the checked thumb reports `x: 13` against the resting `1`. So
v1.9's *reading* applies unchanged; only the property name in it is narrower than
the phenomenon. Worth stating, because a worker checking "does this component use
`transform`?" against `switch.tsx` would answer **no** and conclude wrongly that
the field cannot move.

### Why the port expresses it as layout, and why that is not a manufactured agreement

gpui has no CSS transitions, and the driver reads **layout** bounds at prepaint.
Expressing the slide as a left margin is therefore the spelling that makes both
sides report the same box.

It supplies no number the reference produced.
`translate-x-[calc(var(--thumb-size)-4px)]` is an **input** — a class — and the
port resolves it through its own `--spacing`, which is P3.1's line for
`--class-radius` applied unchanged. The two spellings are indistinguishable to
the contract because the thumb is the track's **only** child, so no sibling's
layout can depend on which one is used.

And the arithmetic is an identity rather than a coincidence: a flush-right thumb
sits at

```
(2t − 2) − 2·1 − t  =  t − 4
```

for any thumb size `t`, which **is** the class's own `calc(--thumb-size - 4px)`.
`the_checked_thumb_is_flush_against_the_tracks_inner_edge` asserts it at both
breakpoints, where the two answers are 12 and 16 — so the derivation is doing
work rather than agreeing with a constant.

## 5. `border` is 0 on both anchors — `kbd`'s side of the trap

`switch.tsx` carries no `border-*` on either element, so Tailwind's preflight
`border: 0 solid` stands. Measured live at `borderTopWidth: "0px"` on both. The
reference's `border.color: "#ffffff0f"` is the junk v1.3 ruling 2 tells the
differ to ignore while `w == 0`.

The mirror trap is on the same component: `focus-visible:ring-2` is a
**box-shadow**, so a focused track's `border.w` is still 0, never 2.

## 6. The `sm:` trap cannot fire from a call site here — measured

`badge` lost an unprefixed call-site `h-4` to the variant's `sm:h-4.5`; `label`
lost a call-site `ui-text-sm` to the primitive's `sm:text-sm/4`. Neither can
happen on this component: **not one of the 19 live `<Switch` call sites passes a
`className`**. The primitive authors both sides of the breakpoint itself, so the
merge has one contributor.

That makes this the first Tier B surface with no call-site parameter at all —
`button`, `input`, `badge`, `avatar` and the P3.6/P3.8 leaves each needed a
`CallSite` or a `--class-*`, and `switch` needs none.

## 7. The `size` prop is inert — and all 19 call sites pass it

`switch.tsx` takes `size?: 'sm' | 'default'` and spends it on `data-size={size}`.
There is no `data-size` in the class list, and a sweep of **every loaded
stylesheet** for a selector mentioning `data-size` returned **0 rules**.

**19 of 19 live call sites pass `size="sm"`.** So the most-passed prop on the
most-instantiated primitive of this item changes no compared field, and the
surface deliberately has no `--size` option: adding one would advertise a picture
the component cannot make.

## 8. `--primary` is theme-invariant, so `--theme` is uneven here

Found by a test that asserted the opposite and failed. `Theme::LIGHT.primary` and
`Theme::DARK.primary` are the **identical `Hsla`** — the brand green is one
colour in both tables. Consequences for the axis:

| cell | does `--theme` move `switch.bg`? |
|---|---|
| off (`bg-input`) | **yes** — white/8% in dark against black/10% in light |
| on (`bg-primary`) | **no** |

The checked cell is still theme-sensitive overall, through the thumb's
`bg-background`. Recorded because "the port forgot the theme" and "the token is
theme-invariant" look identical in a passing run.

`switch.tsx` also contains **zero** `dark:` variants, so the theme reaches this
component only through the token table. The port therefore carries **no**
`is_dark` helper, unlike every other ported surface — noted in the source so that
its absence reads as a measurement rather than an omission.

## 9. What the oracle cannot see here

Stated plainly, per §8.2:

- the 200ms background transition and the 150ms thumb transition — §6;
- `shadow-sm/5` on the thumb and the focus ring on the track — §6, no shadow
  field;
- the pressed `scale-x-110` and its `transform-origin` — no field, and `:active`
  is runtime interaction state the GPUI extractor cannot reach at all (§6);
- `cursor-not-allowed`, `outline-none`, `will-change-transform` — not fields;
- **nothing in the text group**, on either anchor: a switch paints no text, so
  `--content` is vacuous on every cell of this surface.
