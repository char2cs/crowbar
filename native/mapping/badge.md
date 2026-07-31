# `badge` (P3.3)

`web/src/components/ui/badge.tsx` →
`crates/crowbar-ui/src/components/badge.rs`.

> A §6.2 row, in the shape `native/MAPPING.md` fixes. Kept in its own file
> because P3 runs four workers in parallel and one appended table is one
> conflict per item.

`badgeVariants` is a `cva()` with a base string, three sizes and eight variants
over `@base-ui/react`'s `useRender`. Nothing overrides these classes from a
stylesheet, so the `cn(…)` call is what renders — *at the call sites that write
no `className`*. Three of the five do, and that is half of what this row is
about.

Every "Compiles to" below came from running the app's own `src/index.css`
through its own `tailwindcss` 4.3.0 with the utility as a candidate.

## 0. This component was already half-known, and both facts held

Phase 1 measured `git-status-file-item.tsx`'s Badge exhaustively as
`git-row-badge`, and `ANCHORS.md` v1.6 is written about it. Both findings
survive here unchanged:

* **`content_sized`: yes.** A badge authors no width. `min-w-*` is a floor,
  `shrink-0` stops a flex line squeezing it and nothing grows it, so the used
  width is the label's max-content width plus padding plus the two border
  pixels. gpui ceils that run; `WebKit` keeps the fraction. Measured on the live
  `agent` badge at **44.34** against a native **45**.
* **`line_sized`: no**, and this is the component the rule was written about.
  Every size authors a height. Live: an **18px** box around a **16px** line box.
  The Phase 1 gate's is 16 around 13.33. Declaring either compares the two and
  manufactures a delta on an anchor both engines agree on.

## 1. Values: spacing, type, radius, colour

| React / Tailwind | Compiles to | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `border` (base list) | `border-width: **1px**` — unconditional | `BORDER_WIDTH`, `.border_1()` | compared **exactly** |
| `border-transparent` (base list) | `border-color: transparent` | `Color::TRANSPARENT` — still a 1px border | compared |
| `gap-1` | `calc(--spacing * 1)` = **4px** | `GAP` | compared (through geometry) |
| `rounded-sm` | `calc(var(--radius) * 0.6)` = **6px**, *not* Tailwind's stock 2 | `theme.radius_sm.value()` | compared |
| `rounded-[.25rem]` (`size=sm`) | **4px** | `RADIUS_SM_SIZE` — a literal, see the traps | compared |
| `h-5.5` / `sm:h-4.5` (`default`) | 22 / 18 | `Size::height` | compared |
| `h-6.5` / `sm:h-5.5` (`lg`) | 26 / 22 | `Size::height` | compared |
| `h-5` / `sm:h-4` (`sm`) | 20 / 16 | `Size::height` | compared |
| `min-w-*` / `sm:min-w-*` | the same step as the height | `Size::min_width` | compared (as a floor) |
| `px-[calc(--spacing(1)-1px)]` | **3px** — the border paid out of the padding | `Size::padding_x` | compared |
| `px-[calc(--spacing(1.5)-1px)]` (`lg`) | **5px** | `Size::padding_x` | compared |
| `text-sm` | `--text-sm: 0.875rem` on `calc(1.25 / 0.875)` | `theme.ui_text_base.value()` + `relative(…)` | compared |
| `text-xs` | `--text-xs: 0.75rem` on `calc(1 / 0.75)` | `theme.ui_text_sm.value()` + `relative(…)` | compared |
| `text-base` | `--text-base: 1rem` on `calc(1.5 / 1)` | `theme.ui_text_lg.value()` + `relative(…)` | compared |
| `sm:text-[.625rem]` (`size=sm`) | **`font-size` only** — no paired line height | `TEXT_SM_SIZE`, with the base step's ratio kept | compared |
| `font-medium` | `--font-weight-medium: 500` | `FontWeight::MEDIUM` | compared |
| `bg-primary` / `bg-destructive` / `bg-secondary` | the token | `theme.primary` / `.destructive` / `.secondary` | compared |
| `bg-warning/8`, `dark:bg-warning/16` (and `error`/`info`/`success`) | `color-mix(in oklab, … N%, transparent)` | `theme.warning.mix(8 or 16, TRANSPARENT)` | compared |
| `border-input bg-background dark:bg-input/32` (`outline`) | `--input` border; `--background`, or `--input` at 32% in dark | `theme.input`; `theme.background` / `theme.input.mix(32, …)` | compared |
| `text-white` (`destructive`) | `var(--color-white)` | `Theme::LIGHT.card` — the same declaration, see the traps | compared |
| `[&_svg…]:size-3.5` / `sm:…size-3` | 14 / 12px | one empty box | compared |
| `[&_svg…]:opacity-80` | `opacity: 80%` | `.opacity(0.8)` | **invisible** |

**The `--ui-text-*` trade, again.** Tailwind's `text-sm`/`text-xs`/`text-base`
are 0.875/0.75/1rem and `--ui-text-base`/`--ui-text-sm`/`--ui-text-lg` are the
same three numbers. The port reads the token that carries the number and says so
at the call site — `native/MAPPING.md` states the trade once. It **runs out at
`.625rem`**, which no token carries (`--ui-text-xs` is 0.6875rem), so that one is
a named literal.

## 2. Layout constructs

| React / Tailwind | Compiles to | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `inline-flex` | `display: inline-flex` | `.flex()` — and the reference's own computed `display` is **`flex`**, because CSS blockifies a flex item and every live Badge is one. **Measured** | compared (through geometry) |
| `relative` | `position: relative` | `.relative()` — inert; nothing here is `absolute` except the dead touch target | compared |
| `shrink-0` | `flex-shrink: 0` | `.flex_shrink_0()` — **load-bearing**: it is why the used width is max-content rather than a share | compared |
| `items-center justify-center` | both axes centred | `.items_center().justify_center()` | compared |
| `whitespace-nowrap` | `white-space: nowrap` | `.whitespace_nowrap()` — **ports to something and matters**: gpui only computes a wrap width when `white_space` is `Normal`, so this is what makes a long label shape on one line here as it does in `WebKit` | compared |
| the label | an anonymous text node | `AnchorSink::boxed_text`, so the box and the run reach the snapshot as one anchor (`ANCHORS.md` §3, clarified v1.4) | compared |
| the host | — | the **surface** wraps the badge in a flex row, so it is a flex item and shrink-to-fit. Above the root anchor, so outside the snapshot (§4) | absent by construction |

## 3. No gpui equivalent / not ported

| React / Tailwind | Why | What the port does |
|---|---|---|
| `transition-shadow` | a transition | **absent.** §6: a snapshot is one instant |
| `outline-none` | `outline-style: none` | **absent.** The contract has no outline field and gpui paints none to suppress |
| `[button&,a&]:cursor-pointer` | pointer routing | **absent.** Not a visual property |
| `[button&,a&]:pointer-coarse:after:{absolute,size-full,min-h-11,min-w-11}` | a 44px touch target behind `@media (pointer: coarse)`, on a pseudo-element | **absent**, twice over: the media query is false on this hardware and §3's pseudo shortcut needs `inset: 0`, which this is not |
| `[&_svg]:pointer-events-none` | hit testing | **absent** |
| `disabled:pointer-events-none` | same | **absent** |

## 4. Painted but invisible to the oracle

`ANCHORS.md` §6 has no field for any of these. **Say so in any report.**

| React / Tailwind | gpui | Note |
|---|---|---|
| `focus-visible:ring-2 ring-offset-1 ring-ring ring-offset-background` | two `BoxShadow`s inserted via `Styled::style` | spread **3** and **1**, in Tailwind's own composite order. **This is the whole of the component's focus state** |
| `disabled:opacity-64` | `.opacity(0.64)` | no opacity field, and 64% does not reach v1.7's zero |
| `[&_svg:not([class*='opacity-'])]:opacity-80` | `.opacity(0.8)` | same |

## 5. Anchoring

| Construct | Decision |
|---|---|
| ids in the primitive | `badge`, as an **object property** of `defaultProps` rather than a JSX attribute — `useRender` builds the element from a props bag. `mergeProps(defaultProps, props)` puts the call site's last, so an override still wins (P2.1's convention, in the shape `useRender` forces) |
| a document with several badges | the call site names them. `git-status-file-item.tsx` writes `git-row-badge`, which is why the six badges in the live git panel do **not** answer to this surface |
| `CONTENT_SIZED` | **`["badge"]`**, and the React spelling — `data-oracle-content-sized` — is on `badge.tsx`'s own `defaultProps`. v1.5 says content-sizing is a property of the *component*; `git-status-file-item.tsx` had already been asserting the same thing one call site at a time |
| `LINE_SIZED` | **empty**, and an empty *list* rather than an absent one, for P2.1's reason |

## 6. Traps

Each of these compiles, renders something plausible, and is wrong.

| Trap | What actually happens |
|---|---|
| **`border` is 1px on every variant.** | P3.1's finding in a second place, and the inverse of `ring-1`. The base list's bare `border` compiles to `border-width: 1px` unconditionally; the variants only ever change the *colour*, and five of the eight leave it `transparent`. `ANCHORS.md` v1.1 compares `border.w` **exactly**, so a port that skipped it because "a warning badge has no visible border" is wrong on every cell — and wrong about the box, because `box-sizing: border-box` makes the pixel eat into the padding |
| **A call site's unprefixed `h-4` is DEAD above 640px.** | The single most likely mistake in this file, and it cost the fixture's height. `h-4` and the variant's `sm:h-4.5` are the same tailwind-merge group but **different modifiers**, so both survive the merge and both reach the stylesheet — and Tailwind emits `sm:`-prefixed rules *after* unprefixed ones. So at ≥640px the variant wins and the call site's 16px never paints. Measured: `getComputedStyle().height` on the live `agent` badge is **18**. What the call site *does* remove is the variant's unprefixed `h-5.5` |
| **…and its `px-1` is alive in every cell.** | Same group, *same* modifier, written later — so tailwind-merge drops `px-[calc(--spacing(1)-1px)]` outright and 4px wins at every viewport. The two halves of one `className` behave oppositely, which is why the port resolves them separately |
| **Declaring the badge `line_sized`.** | `ANCHORS.md` v1.6's own worked example, restated because this is the component. It paints one line of text in a box, so a detector keyed on "has text and no explicit height" fires — and every size authors a height |
| **Reading a token for `rounded-[.25rem]`.** | `theme.css` builds the whole radius scale off `--radius: 0.625rem`, and its smallest step `--radius-sm` is **6px**. `.25rem` is 4 and is not on that scale, so it is an arbitrary Tailwind value. A port that read `theme.radius_sm` would paint a 6px corner on the Phase 1 gate's own badge, where the reference paints 4 |
| **`sm:text-[.625rem]` also setting a line height.** | It does not. Tailwind's `text-*` utilities carry a paired `--text-*--line-height`; an arbitrary `text-[…]` sets `font-size` alone. So a `size="sm"` badge above the breakpoint is a 10px face on `text-xs`'s `calc(1 / 0.75)` ratio → a **13.33px** line box, which is exactly what the archived `git-row-badge` reference reports |
| **`[button&,a&]:hover:bg-*` firing on a badge.** | The `&` is the badge itself and `badge.tsx`'s `defaultTagName` is `'span'`. **No live call site passes `render`**, so the whole hover group is dead on every rendered Badge. So is `disabled:*` — `&:disabled` matches form elements only |
| **`text-white` as a theme token.** | It is `var(--color-white)`, Tailwind's own, which `Theme` has no field for. The port reads `Theme::LIGHT.card` — not a coincidence of value but the *same declaration*, since `theme.css` writes `--card: var(--color-white)` in `:root`. Deliberately not `theme.card`, which in dark is the background |
| **Assuming `min-w-*` makes the box non-content-sized.** | It is a **floor**. Above it the width is the run; at or below it the width is the floor, and every floor here is a whole number of px so `ceil(reference)` degenerates to the reference. The declaration is safe in both regimes — and on the fixture's own short label the two land on 18 **exactly**, which is `min-w-*` being the same `--spacing` step as the height |
| **Counting live Badges and calling them references.** | Six `[data-slot=badge]` elements render in the live git panel and **not one of them is a reference for this surface**: all six are `git-row-badge`. See §7 |

## 7. What this surface cannot show the differ

Recorded because §8.2 requires honesty about it, and because finding the one
reference took most of this item.

- **Four of the five call sites cannot produce a snapshot of this surface.**

  | call site | why not |
  |---|---|
  | `git-status-file-item.tsx` (×6 live) | overrides the id — those are `git-row-badge` |
  | `diff-review-header.tsx` | `DiffReviewHeader` is **dead code**: `git grep` finds its own definition and its unit test and nothing else |
  | `review-thread-item.tsx`, two `Outdated` badges | gated on `isOutdated`, which `use-review-annotations.tsx` — the component's only call site — never passes |
  | `review-thread-item.tsx`, the `agent` badge | **this one**, and only when a review message carries `isAgent` |

  The reference was taken by posting an agent reply to the fixture's review
  thread through the daemon's own API
  (`POST …/threads/:id/replies` with `isAgent: true`), which is the app's own
  path and not a fixture edit. The message is still there; deleting it removes
  the only capturable Badge in the app.
- **Five of the eight variants have no live call site** — `default`,
  `destructive`, `error`, `info`, `success`. `secondary`'s only call site is
  inside the dead `DiffReviewHeader`, so it is live in the source and
  unreachable in the app.
- **`lg` has no live call site**, and neither does a Badge with a glyph: every
  live one's only child is text.
- **`hover` and `focus` cannot fail on the element the app renders.** Hover needs
  `render` to have made the badge a `<button>` or an `<a>`, which nothing does;
  focus paints a ring, which is a box-shadow, which §6 has no field for. Neither
  is declared `unmodelled` — the rules exist — so the caption says it per cell.
- **`selected` is unmodelled.** `badgeVariants` has no selected, active or
  pressed rule of any kind.
- **`empty` is real, visible and unreferenced.** With no label the box falls to
  `min-w-*` and the badge is a circle; no live call site renders one.

## 8. Cross-component notes added by this component

Things learned here that are **not** about `badge`.

| Note | |
|---|---|
| A `useRender` primitive carries its anchor as a **props-object property** | not a JSX attribute — `useRender({ props: mergeProps(defaultProps, props) })` builds the element from a bag, and there is no JSX element to hang an attribute on. `mergeProps` puts later objects last, so a call site still overrides. `button.tsx` has the same shape; every remaining `useRender` primitive will |
| **An unprefixed call-site class loses to a variant's `sm:` one** | and wins below the breakpoint. Both survive tailwind-merge (different modifiers) and Tailwind's emitted order decides at runtime. This is invisible in the class list and shows up only as a computed value — so for any merged call site, *read `getComputedStyle` before believing the merge* |
| A dead component is not a dead class list | `DiffReviewHeader` compiles, has a passing unit test and renders nowhere. `git grep <ComponentName>` over `src/` is the check, and it has to be run before counting a call site as a reference |
| The primitive's default anchor id is not a reference | it is a *default*. Whether any live element still carries it is a separate question, and here the answer was one branch behind a boolean on a stored message |
