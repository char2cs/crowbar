# `button` (P3.1)

`web/src/components/ui/button.tsx` + `web/src/components/ui/button-variants.ts` →
`crates/crowbar-ui/src/components/button.rs`.

> **Read `native/MAPPING.md` first.** This file is §6.2's row for `button`, kept
> separate so three Phase 3 workers do not conflict in one file. Everything in
> `MAPPING.md`'s "How to read a row" applies unchanged, including the rule that
> matters most: **compile the CSS, do not read the class name.**

Every "Compiles to" below came from running `web/src/index.css` through the app's
own `tailwindcss` 4.3.0 with the utility as a candidate, and every "Measured"
came off the **live** app — pid 64880, bridge 9223, `innerWidth` 1714 at DPR 2,
`html.dark`. `--spacing` is Tailwind's stock `0.25rem`; `theme.css` does not
redefine it. Two radii are not stock and are marked.

**The class lists are the whole story here**, unlike `resizable`: `button.tsx`
has no third-party primitive under it that writes inline styles. What it does
have is `cn(buttonVariants({ className, size, variant }), active && …)`, so a
**call site's `className` is merged over the variant's** by tailwind-merge — and
that turns out to be the single most important fact about the *reference*
(§7, §11).

---

## 1. The headline: `border` is 1px in every cell, and that is the inverse trap

`native/MAPPING.md` records twice that **`ring-1` is a box-shadow, not a
border**, and that a port reaching for `.border_1()` reports `border.w: 1`
against a reference's `0` on every cell — `border.w` being the one field
`ANCHORS.md` v1.1 compares **exactly**.

This component is the mirror image, and a port that has learnt the first trap
will walk straight into it:

| | |
|---|---|
| the class | a bare `border` in the **base** class list |
| compiles to | `border-style: var(--tw-border-style); border-width: 1px` — unconditional |
| what the variants change | the **colour** only. `border-transparent` on `ghost`, `link`, `secondary` |
| measured, live | `ghost`/`icon-sm`: `borderTopWidth: "1px"`, `borderTopColor: "rgba(0, 0, 0, 0)"` |
| the port | `.border_1().border_color(Color::TRANSPARENT)` |

So a ghost button — the most-used arm in the app — reports `border.w: 1` with
`border.color: #00000000`. v1.3 made `border.color` compared *only* when
`w > 0`, so this is a comparison on **both** fields rather than an absence on
either. A port that skipped the pixel because "a ghost button has no visible
border" would be wrong on every cell of the matrix, *and* wrong about the box
model: `box-sizing: border-box` is on, so the pixel eats into the padding box
(the `::before` overlay is 26×26 inside a 28×28 button — measured).

---

## 2. Values: spacing, type, radius, colour

| React / Tailwind | Compiles to | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `gap-2` / `gap-1.5` / `gap-1` | `calc(--spacing * 2 / 1.5 / 1)` = **8 / 6 / 4px** | `Size::gap()` | compared (through geometry) |
| `h-9 sm:h-8`, `size-8 sm:size-7`, … | `calc(--spacing * n)` — **5 numbers for 10 sizes**, each text size sharing its icon twin's | `Size::extent(breakpoint)` | compared |
| `px-[calc(--spacing(3)-1px)]` | `calc(calc(var(--spacing) * 3) - 1px)` = **11px** | `Size::padding_x()` | compared (through the box) |
| `rounded-lg` | `var(--radius)` = **10px**, *not* Tailwind's stock 8 — `theme.css` sets `--radius: 0.625rem` and `--radius-lg: var(--radius)` | `theme.radius_lg.value()` | compared |
| `rounded-md` (`xs`, `icon-xs`) | `calc(var(--radius) * 0.8)` = **8px**, *not* stock 6 | `theme.radius_md.value()` | compared |
| `rounded-sm` (a **call site's**, over the variant's) | `calc(var(--radius) * 0.6)` = **6px** | `RadiusClass::Sm` → `theme.radius_sm.value()` | compared — see §11 |
| `before:rounded-[calc(var(--radius-lg)-1px)]` | **9px** (and **7px** for the `--radius-md` spelling) — measured live on both | `Size::overlay_radius()` | **invisible** (§5) |
| `border` | `border-width: 1px` | `BORDER_WIDTH` | compared, **exactly** |
| `font-medium` | `--font-weight-medium: 500` | `FontWeight::MEDIUM` | compared |
| `text-base` / `sm:text-sm` | `1rem` on `calc(1.5/1)`; `0.875rem` on `calc(1.25/0.875)` | `theme.ui_text_lg` / `theme.ui_text_base` + `relative(…)` | compared |
| `text-xs` (`xs`, `sm:`) | `0.75rem` on `calc(1/0.75)` | `theme.ui_text_sm` | compared |
| `text-lg` (`xl`, unprefixed) | `1.125rem` on `calc(1.75/1.125)` | **`TEXT_LG`, a literal** — see the trap | compared |
| `[&_svg:not([class*='size-'])]:size-4.5 sm:…size-4` | **18 / 16px**; `xl`+`icon-xl` **20 / 18**; `icon-xs` **16 / 14** | `Size::icon(breakpoint)` | invisible (icons are empty boxes) |
| `[&_svg]:-mx-0.5` | `margin-inline: -2px` | **resolved by hand** — see §6, the largest finding here | compared, indirectly |
| `bg-primary` / `border-primary` / `text-primary-foreground` | `var(--primary)` … | `theme.primary`, `theme.primary_foreground` | compared |
| `bg-secondary`, `text-secondary-foreground` | | `theme.secondary`, `theme.secondary_foreground` | compared |
| `bg-destructive`, `border-destructive` | | `theme.destructive` | compared |
| `text-destructive-foreground` | | `theme.destructive_foreground` | compared |
| `text-white` (`destructive`) | `var(--color-white)` — **Tailwind's own token** | `Theme::LIGHT.card` — see the trap | compared |
| `border-input`, `bg-popover`, `dark:bg-input/32` | | `theme.input`, `theme.popover`, `theme.input.mix(32, T)` | compared |
| `text-foreground`, `hover:bg-accent` | | `theme.foreground`, `theme.accent` | compared |
| `hover:bg-*/90`, `/80`, `/64`, `/50`, `/4` | `color-mix(in oklab, … N%, transparent)` | `Color::mix(N, Color::TRANSPARENT)` | compared |
| `bg-accent/20` (the `active` prop) | | `theme.accent.mix(20, T)` | compared |
| `focus-visible:ring-2` + `ring-offset-1` | `--tw-ring-shadow: 0 0 0 calc(2px + 1px)` and `--tw-ring-offset-shadow: 0 0 0 1px var(--background)` — **two box-shadows** | two `BoxShadow`s through `Styled::style` | **invisible** (§6) |
| `disabled:opacity-64` | `opacity: 64%` | `.opacity(0.64)` | **invisible** — and see the trap |

**The `--ui-text-*` trade, and where it runs out.** `MAPPING.md` states the trade
once: read the crowbar token that carries Tailwind's number and say so at the
call site. It holds for three of the four steps this component uses —
`--ui-text-lg` is 1rem = `text-base`, `--ui-text-base` is 0.875rem = `text-sm`,
`--ui-text-sm` is 0.75rem = `text-xs`. It **fails on the fourth**:
`text-lg` is 1.125rem and `--ui-text-xl` is **1.25rem**. The two scales diverge,
so `TEXT_LG` is a named literal with the reason, and reading `--ui-text-xl` would
paint a 20px label where the reference paints 18. It is reachable only from
`--size xl`, which is one of the four sizes no live call site uses.

---

## 3. Layout constructs

| React / Tailwind | Compiles to | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `inline-flex` | `display: inline-flex` — but the **computed** value on every live Button is **`flex`**, because CSS blockifies a flex item's display and every live Button is one. Measured, not assumed | `.flex()`. gpui has no inline flow at all, so this is a coincidence the port gets for free — but only while the host is a flex container, which is why the surface's is | compared (through geometry) |
| `shrink-0` + no authored width (the five text sizes) | `flex: 0 0 auto` → used width is the item's **max-content** | nothing to write; taffy agrees. This is what makes a labelled button shrink-to-fit | compared |
| `whitespace-nowrap` | `white-space: nowrap` | `.whitespace_nowrap()`, and it is **load-bearing**: gpui computes a wrap width only when `white_space == Normal` (`elements/text.rs`). With it, a long label shapes on one line on both sides — so `dropdown-menu`'s wrapping trap **cannot fire on this component** | compared |
| `items-center justify-center` | | `.items_center().justify_center()` | compared |
| `relative` + `before:absolute before:inset-0` | an overlay on the host's **padding** box | an `.absolute()` child with all four insets zero; takes no room in the flex line | invisible (§5) |
| the `<Spinner>`'s `absolute` with no inset | static position from the flex container's alignment — CSS Flexbox §4.1 places it as though it were the sole flex item | `.absolute()` with no inset; taffy applies the container's alignment the same way. **Measured**: `x 6, y 6, 16×16` in a 28×28 button, which is the reference's own `<svg>` position | compared |
| `pointer-coarse:after:*` (four classes) | `@media (pointer: coarse)` | **nothing.** Measured live: `matchMedia('(pointer: coarse)')` is **false** on this machine and `::after`'s `content` is `none`. Dead on arrival, like `resizable`'s three dead classes | absent |
| `disabled:pointer-events-none`, `[&_svg]:pointer-events-none`, `data-loading:select-none`, `cursor-pointer` | | **absent.** Not visual properties | absent |
| `transition-shadow` | | **absent.** §6: a snapshot is one instant | absent |
| `outline-none` / `focus-visible:outline-hidden` | `outline-style: none` | **absent.** No outline field, and gpui paints none to suppress | absent |

---

## 4. No gpui equivalent

| React / Tailwind | Why | What the port does |
|---|---|---|
| `[&_svg]:-mx-0.5` **in a content-sized flex container** | taffy gets it wrong — see §6 | **resolved by hand** into the glyph's own box (`Size::glyph_box`) |
| `shadow-xs`, `shadow-xs/5`, `shadow-primary/24`, `shadow-destructive/24` | drop shadows | **not painted.** Two reasons that hold together: §6 has no field for a shadow, and the colour is `rgba(0,0,0,0.05)` — measured live on the `outline` variant — which `check-invariants.sh` rule 4 will not let a component outside `crowbar-ui/src/theme/` mint. Unlike `dropdown-menu`'s `shadow-md`, gpui's preset is not the one this needs |
| `not-disabled:inset-shadow-[0_1px_--theme(--color-white/16%)]` and the three other `--theme(--color-black|white/N%)` inset shadows on `::before` | inset shadows in Tailwind's own black/white | **not painted**, same two reasons. Measured live on the `outline` variant: `::before`'s computed `box-shadow` is `oklab(1 0 0 / 0.06) 0 -1px 0 0` |
| `not-dark:bg-clip-padding` | `background-clip: padding-box` | **absent.** gpui has no background-clip, and with a *transparent* border it changes no pixel |
| `underline-offset-4` | `text-underline-offset: 4px` | **absent.** gpui's `underline()` has no offset. Reachable only from `link`, one live call site, and the contract has no field for a decoration at all |
| `aria-disabled`, `data-slot`, `type` | not visual | absent |
| the `tooltip` / `shortcut` / `tooltipSide` props | a Radix portal, a different component | **absent.** It is `tooltip.tsx` + `keybinding.tsx`, not this primitive, and it renders outside the root anchor |

---

## 5. Painted but invisible to the oracle

`ANCHORS.md` §6 has no field for any of these. **Say so in any report.**

| React / Tailwind | gpui | Note |
|---|---|---|
| `focus-visible:ring-2 ring-ring ring-offset-1 ring-offset-background` | two `BoxShadow`s inserted in front through `Styled::style` | **the whole of the focus state.** Unlike `resizable`'s `ring-1`, the offset layer here really paints: `ring-offset-1` moves `--tw-ring-offset-width` off its `0px` initial. Both colours are genuine tokens (`--ring`, `--background`), so this one is paintable where the drop shadows are not |
| `disabled:opacity-64` | `.opacity(0.64)` | v1.7's `visible` term fires **only at zero**, so a disabled button is `visible: true` on both sides and the 36% is a difference neither extractor can report. Pinned by a live layout test |
| the `::before` overlay | an `.absolute()` child with the 9px inner radius | **§3's pseudo-backed shortcut is legal here and still must not be taken** — see the traps |
| `*:data-[slot=button-loading-indicator]:text-*` | `.text_color(…)` on the spinner box | the spinner is an empty box, and a box with no text emits no `fg` |

---

## 6. The largest finding: gpui cannot lay out a negative margin here

**A negative inline margin on an in-flow flex item breaks taffy's content-based
main-size resolution.** Measured in a hand-built harness, isolated from this
component — a flex row with `px-11 border-1`, one 16px item and a text run:

| item margin | gpui container width | CSS |
|---|---|---|
| `0` | 77 | 77 |
| `+2` | 81 | 81 |
| `-1` | **24** | 75 |
| `-2` | **24** | 73 |
| `-4` | **24** | 69 |

24 is the padding box plus the two border pixels: the entire content
contribution is gone. It is **not a clamp at zero** — with a wider run the same
tree gives 199 against CSS's 323 at `-2` and 71 at `-4`, so the error scales with
the margin. **Positive margins are exact.** A container with an *authored* width
is unaffected, which is why the five square sizes never meet it and why every
earlier component in this port missed it: `dropdown-menu`'s `-mx-1` separator and
`sidebar-carousel`'s negative percentage margin both sit in containers whose main
size is already definite. `MAPPING.md` says of the first that "taffy honours
negative block margins exactly as CSS does — **measured**"; that measurement
stands, and this is the case it did not cover.

**What the port does.** `Button::glyph` renders a box of `Size::glyph_box` — the
icon less its two margins — and no margin at all. Every measurable quantity is
then exactly CSS's: the button's border box, the label's advance, the gap. What
changes is the glyph's own box, from 16 to 12, and that costs **nothing**: the
glyph is an *empty* box standing in for a call-site `<svg>` this port does not
draw, so 16px of nothing and 12px of nothing are the same picture — and it is
unanchorable on the reference side anyway, because `button.tsx` cannot put an
attribute on a child a call site passes.

`Button::spinner` **keeps** its margin, for two reasons that have to hold
together: it is `position: absolute`, so it contributes nothing to the content
size and cannot trip the defect, and it **is** anchored, so its box has to be the
reference's 16 rather than 12.

The rejected alternative is pinning the button's width in the component, which
`ANCHORS.md` refuses by name twice.

**Shipped with a control.**
`row_layout::button::a_negative_margin_still_collapses_a_content_sized_flex_container`
measures gpui rather than this component, so a gpui bump that *fixes* the defect
fails there rather than being noticed years later.

---

## 7. Anchoring, and the two anchors this surface has

| Construct | Decision |
|---|---|
| ids in the primitive | `button` and `button-loading-indicator`, both **per-slot defaults**. The first is written into `defaultProps`, *before* `mergeProps(defaultProps, props)`, so a call site can override it — P2.1's convention, and it matters more here than anywhere: **nine Buttons live in one document** |
| the `<Spinner>`'s id | a JSX attribute, because the spinner is a real JSX element. The button's is an **object property** (`'data-oracle-id': 'button'`), because `useRender` builds that element from a props object and there is no tag to put an attribute on. Two spellings of one thing, and worth knowing before writing a strip regex |
| the leading glyph | **not anchored, and not anchorable.** It is a child a *call site* passes, so `button.tsx` cannot reach it |
| the `::before` overlay | **not anchored** — see the traps |
| `CONTENT_SIZED` | **empty, and it is a stated hole rather than a measurement.** The five non-icon sizes author no width, so a labelled button *is* a content-sized box and v1.5 would apply. A v1.5 declaration has to be made on both sides and the React spelling is a `data-oracle-content-sized` attribute, which this item's remit on `button.tsx` (`data-oracle-id` and nothing else) does not allow. Undeclared on **both** sides is the safe direction: it can manufacture a delta of at most the `ceil` excess (< 1px) and cannot hide one. ⚠ **This row used to end "and no live call site renders a Button with a label … so it cannot fire today". That was false** — **at least 72** live non-test call sites render a labelled `<Button>` (P3.68 counted them by parsing the JSX with the TypeScript compiler API, after a regex approach lied; floor, not point estimate, exclusions named in `button.rs`). It does not fire for a narrower reason: none of those 72 is in **this surface's own captured reference**, whose nine `[data-slot=button]` elements are all icon-only (§9). That is one differently-scoped capture away from changing |
| `LINE_SIZED` | **empty, and here it is a measurement.** Every one of the ten sizes authors a height, so no button's box is derived from its line box. Pinned by asserting, for all ten sizes at both breakpoints, that the box differs from its own line box by more than 0.5px |
| `--class-radius` | the surface names the **class** a call site merged, never a number — see §11 |
| a surface scope declaration (v1.8) | **none, deliberately.** The anchor set is a function of the *cell* — `button-loading-indicator` exists only under `--loading` — and v1.8 permits a declaration only when the set is a property of the surface. The root's subtree contains no other anchor either way |

---

## 8. Traps

Each of these compiles, renders something plausible, and is wrong.

| Trap | What actually happens |
|---|---|
| **Skipping the border because the variant is `ghost`.** | The base list's bare `border` is unconditional; the variant only sets `border-transparent`. `border.w` is compared **exactly**, so this is a delta on every cell — the inverse of the `ring-1` trap `MAPPING.md` records twice, and a reader who has learnt that one is *more* likely to make this one |
| **Reading `size-8 sm:size-7` as `size="icon"`.** | That is `icon-sm`. `icon` is `size-9 sm:size-8`, one step larger. Four pixels in both axes, on the arm the reference cell uses |
| **Taking §3's pseudo-backed shortcut for `::before`.** | It is *legal* here — the pseudo really is `position: absolute; inset: 0`, which is the case §3 permits — and it is still wrong, because a pseudo-backed anchor **replaces** the host's record: `extractSnapshot` reads `pseudoMap[id]` off the element carrying the `data-oracle-id`. Anchoring the overlay would throw away the button's own background, border and 10px radius, which are the whole point of the surface. So it is painted and deliberately unanchored — `resizable` reached the same place by a different door |
| **`data-pressed` on `secondary` is 80%, not 90%.** | `data-pressed:bg-secondary/90` and `[:active,[data-pressed]]:bg-secondary/80` compile to `.cls[data-pressed]` and `.cls:is(:active,[data-pressed])` — **the same specificity**, so source order decides, and Tailwind emits the arbitrary-variant rule later (line 885 against 601 in the compiled sheet). Hover alone is still 90%. It is the only variant where hover and press differ |
| **Assuming the `active` prop loses to nothing.** | `bg-accent/20` is appended by `cn(…)` *after* `buttonVariants(…)`, so it beats every variant's own `bg-*` — and it **loses** to `hover:`/`data-pressed:`, because those are different tailwind-merge groups (both survive the merge) and `.hover\:bg-accent:hover` is one selector more specific at runtime. So it replaces the **resting** background only |
| **Reading `--ui-text-xl` for `text-lg`.** | The `--ui-text-*` trade holds for three steps and breaks on the fourth: 1.125rem against 1.25rem. Two pixels, on the `xl` arm |
| **Minting white for `text-white`.** | Rule 4 forbids it, and no `Theme` field is called white. `Theme::LIGHT.card` **is** `--color-white` — not a coincidence of value: `theme.css` writes `--card: var(--color-white)` in `:root`. Reading `theme.card` instead would paint the dark table's card, which is the background |
| **`px-[calc(--spacing(3)-1px)]` is not `px-3`.** | The `-1px` pays for the base list's border out of the padding, because `box-sizing: border-box` is on. A port that wrote `px(12)` would be one pixel wide on each side of every text button |
| **Expecting `opacity-64` to move `visible`.** | v1.7's opacity term fires only at **zero**. 0.64 leaves `visible: true` on both sides, and nothing else about `disabled` is representable — so `--disabled` is the *most* live prop on the component (35 of 142 call sites) and the *least* visible |
| **`pointer-coarse:` on a Mac.** | `matchMedia('(pointer: coarse)')` is **false** and `::after` has `content: none`. Four dead classes |
| **Letting a call site's `rounded-*` reach the `::before` overlay.** | It does not. `before:rounded-[…]` and a bare `rounded-*` are **different tailwind-merge groups**, so both survive the merge and each applies to its own box. Measured on all nine live Buttons: the five with `rounded-sm` are host **6** / overlay **9**, the two with `rounded-lg` over `icon-xs` are **10 / 7**, and the two unmerged are **10 / 9**. A port that wired the override through to the overlay would be wrong on seven of the nine — and **invisibly**, because the overlay is unanchored on both sides (mutation-tested: **0 failures**, §11) |
| **Trusting `-mx-0.5` to lay out.** | §6 |
| **Naming a surface option `--no-icon`.** | `Cell::parse` matches every shared option *before* it asks the selected surface's bag, and `--no-icon` is `git-status-row`'s `showFileIcon`. A surface option spelled the same is swallowed with no error anywhere. This surface's is `--no-glyph`, and `the_shared_parsers_words_are_not_this_surfaces` pins it |

---

## 9. What this surface cannot show the differ

Recorded because §8.2 requires honesty about it.

- **No live Button paints text.** All nine `[data-slot=button]` elements in the
  fixture workspace are icon-only, and none of the 142 `<Button` call sites in
  `web/src/` was captured with a label. So `fg`, `text`, `text_width`, `clipped`
  and the whole `font` group — half the contract — have **no reference on this
  surface today**. The port renders them and the layout gate measures them
  against the compiled CSS; the oracle cannot confirm any of it.
- **The primitive's own radius has no visible reference.** Every live Button
  merges a call-site `className` over the variant, and the radius is where that
  shows: of the nine, five carry `rounded-sm` (6px over the primitive's 10), two
  carry `rounded-lg` over `icon-xs`'s `rounded-md` (10 over 8), and **the only
  two whose radius *is* the primitive's are inside the sidebar carousel's
  snapped-out panels**, reporting `visible: false`. So there is no live Button
  that is both unmerged and visible, `--class-radius none` is a cell that can be
  drawn and not compared, and its caption says so — the same shape of finding as
  `resizable`'s grip and Phase 1's `git-row-dir`.
  `/tmp/p3-ref-button-unmerged.json` is the archived half of it: the unmerged
  button, `radius: 10`, `visible: false`.
- **`hover` is real, visible and unreachable.** `hover:bg-*` moves `bg` on six of
  the seven variants — the first time in this port an interaction flag moves a
  *compared* field, where `dropdown-menu`'s focus and `resizable`'s hover both
  land on something the contract cannot see. The rule compiles to
  `@media (hover: hover)`, which the live app reports as **true**, so a human can
  reach the state. This item cannot: `CGPreflightPostEventAccess()` is false on
  this project's machines.
- **`focus` cannot fail.** Its only rules are two box-shadows and an outline.
- **`loading` and `active` have no live call site.** Zero of 142 `<Button`
  elements pass either. Both are rendered and both are named as unreachable, the
  same shape of finding as `resizable`'s grip and Phase 1's `git-row-dir`.
  `loading` is the one that changes a compared field — `data-loading:text-transparent`
  empties `fg` — so it is the most valuable cell with nothing on the other end.
- **`--theme` is vacuous on `default`.** `theme.css` declares `--primary` and
  `--primary-foreground` to the *same value* in `:root` and in `.dark`, so the
  arm 53 of 142 call sites use is one picture in both tables. Asserted, so that a
  reader does not bank the cell.
- **Four of ten sizes and one of seven variants are dead**: `lg`, `xl`,
  `icon-lg`, `icon-xl`, and `destructive-outline`. Rendered, and named as
  unreachable per cell.
- **`empty` is unmodelled.** Every live call site passes a child.
- **v1.9's timing hole does not reach this surface.** `transition-shadow` is the
  only transition on the component, and a shadow has no field in §6 — no
  geometry property is animated, so a capture taken mid-transition is
  indistinguishable from one taken at rest. `tabs`'s `tab-indicator` is the case
  v1.9 was written for; `button` is the control for it.

---

## 10. Cross-component notes added by this component

Things learned here that are **not** about `button`.

| Note | |
|---|---|
| **taffy mishandles a negative margin on an in-flow flex item** whose container is content-sized | The container collapses. Positive margins and definite-width containers are exact, which is why P2.1 and P2.3 both measured negative margins working. Resolve it into the box; ship a control. See §6 |
| CSS **blockifies** a flex item's `display` | so `inline-flex` computes to `flex` on every live Button, and gpui's lack of inline flow costs nothing *as long as the host is a flex container*. A surface hosting a shrink-to-fit primitive has to be one anyway |
| `whitespace-nowrap` **is** portable, and load-bearing | gpui computes a wrap width only when `white_space == Normal`, so `.whitespace_nowrap()` reproduces it exactly. Any component carrying it is immune to `dropdown-menu`'s wrapping trap |
| the `--ui-text-*` trade has a **fourth step where it breaks** | `--ui-text-xl` is 1.25rem and Tailwind's `text-lg` is 1.125rem. Three of four match; the fourth needs a named literal |
| `Theme::LIGHT.card` **is** `--color-white` | `theme.css` writes `--card: var(--color-white)` in `:root`, so that is the door rule 4 leaves open for Tailwind's own white. There is **no** such door for `--color-black`, which is why every `--theme(--color-black/N%)` shadow in this component is unpainted |
| a **shared** parser option shadows a surface's own | `Cell::parse` matches its own words before delegating, silently. Check `row_surface.rs`'s match arms before naming a surface option |
| `struct_excessive_bools` is a real design question | six booleans in one bag became `Interaction` (three pseudo-classes) and `Props` (three props) — which is the division the surface already had between §8.3 flags and its own options |
| `cn(variants(…), className)` means **the call site is half the component** | a wrapped `cva` primitive is not its variant table: tailwind-merge resolves a call site's class over it, and for `button` that is the *only* thing separating the port from its reference. Model it as a knob naming the **class**, resolved through the sealed token — never as the number. §11 has the line between that and handing the port the reference's output |
| an **anchor id can be an object property**, not just a JSX attribute | `useRender`-based primitives build their element from a props object. Anything mechanical over `data-oracle-id` has to handle both spellings |

---

## 11. `--class-radius`, and the line it sits on

The first gate run came back with **exactly one delta**:

```
button.radius: 10.0, expected 6.0   (Δ +4.0, tol ±0.5)
```

Diagnosed, not tuned away. The reference is the tab bar's sidebar toggle, whose
call site writes `className="shrink-0 rounded-sm …"` over
`buttonVariants({ variant: 'ghost', size: 'icon-sm' })`. The port drew the
primitive's `rounded-lg`; the app draws the call site's `rounded-sm`.

### Why a call-site class is a legitimate parameter, and where it stops being one

| | |
|---|---|
| **Forbidden** | a knob that hands the port the reference's **output**. P3.2 refused exactly that for `tab-indicator`, whose box *is* the answer — passing it in makes the anchor unable to fail |
| **Correct** | a knob that supplies the same **input** both engines then resolve independently. `rounded-sm` is a class; each side still computes a radius from `--radius` through its own scale |

So the option is `--class-radius none|sm|md|lg` — it names the **class**, and
`RadiusClass::value` reads the sealed token that class names
(`theme.radius_sm` / `_md` / `_lg`). **There is deliberately no numeric form**:
`--class-radius 6` is a rejection whose message says "never a pixel value",
because a numeric knob would let a future cell be tuned to whatever the
reference happened to report. Same shape as `--loading`, and as P3.2's
`--list-bg` / `--tab-sizing`.

`theme.css` derives the whole scale from one `--radius: 0.625rem` —
`--radius-sm: calc(var(--radius) * 0.6)`, `--radius-md: calc(… * 0.8)`,
`--radius-lg: var(--radius)` — so the assertion is that arithmetic rather than
`px(6.0)`, and a project that moved the base moves all three together instead of
failing here.

### What the default is, and why

`Button::fixture` carries `Some(RadiusClass::Sm)`: the tab bar's toggle in
`web/src/features/tabs/components/tab-navigation-buttons.tsx`, whose own comment
names the intent — "icon-sm (28px, 6px radius)" — so this is the design's number
rather than a stray override. Four more Buttons write the same class string
verbatim. A bare `--surface button --viewport-width 1714` therefore describes the
same picture the reference does, and the caption names the call site so a reader
can check the claim against the app.

`--class-radius none` is the primitive's own, reachable and **unreferenced** —
see §9.

### Mutation results

Each applied to the component, run with `--no-fail-fast`, reverted; the control
after every revert is **0 failures over 833 tests**.

| Mutation | Failures |
|---|---|
| `Button::radius` ignores `class_radius` | **5** |
| `RadiusClass::Sm` reads `radius_md` (the wrong token) | **6** |
| the fixture defaults to `None` instead of `Some(Sm)` | **5** |
| the `::before` overlay follows the call site's class instead of the variant's | **0** |

**The last row is the honest one.** The overlay is unanchored on both sides, so
no gate can see it and the trap in §8 is *recorded rather than defended* — the
standing `resizable`'s hit strip has, whose geometry was measured once by hand
and written into a doc comment because nothing would notice if it stopped being
true. The numbers for this one are in `Button::overlay`'s doc comment: `6 / 9`,
`10 / 7`, `10 / 9`, read off all nine live Buttons.
