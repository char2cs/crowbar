# `search-toggle-icons` (P3.8)

`web/src/components/ui/search-toggle-icons.tsx` →
`crates/crowbar-ui/src/components/search_toggle_icons.rs`.

> A §6.2 row, in the shape `native/MAPPING.md` fixes. Its own file, per the P3
> process note.

Not a component: a module-level `Record` of four `ReactNode`s — three
`@phosphor-icons/react` glyphs and one `<span>` reading `Aa`.

**References:** `/tmp/p3-ref-search-toggle-icons.json` (the `preserve-case` run)
and `/tmp/p3-ref-search-toggle-icons-glyph.json` (a phosphor cell), both captured
live from the editor find bar at a 1714px viewport with the replace row expanded.

## 0. The headline: **two shapes, and the contract sees very different amounts**

| Shape | What the differ can compare |
|---|---|
| the three phosphor `<svg>`s | `bounds`, `bg`, `visible`, `radius`, `border.w` — **and nothing else** |
| `preserveCase`'s `<span>` | all of the above **plus** `text`, `text_width`, `clipped`, `font`, `fg` |

An `<svg>` has element children, so `extract.ts`'s `oracleOwnText` finds no text
node and emits no text group; `fill: currentColor` has no field either. The span
is an ordinary text run and carries the lot.

That asymmetry is why the **run-shaped** cell is the fixture even though three of
the four live icons are glyphs — `kbd`'s reason, where the `Esc` cap was chosen
over the three icon caps.

## 1. Values — the three glyphs

They carry **no props at all**, so phosphor emits `width="1em" height="1em"`.

| React / Tailwind | Compiles to | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `<CaseSensitive />` etc. | `width="1em" height="1em"` presentation attrs — **14px** at the button's `sm:text-sm` | ignored | — |
| host `button`'s `[&_svg:not([class*='size-'])]:size-4.5` | `18px` below `sm` | `glyph_extent(Base)` | `bounds` = 18 |
| host `button`'s `sm:[&_svg:not([class*='size-'])]:size-4` | `16px` at/above `sm` | `glyph_extent(Sm)` | `bounds` = **16**, which the reference reports |
| host `button`'s `[&_svg]:-mx-0.5` | `margin-inline: -2px` | **not applied** — see §2 | outside the border box, so not in `bounds` |
| `fill="currentColor"` | the button's `color` | not drawn | **invisible** |

**This is the P3.2 trap in its purest form**: the presentational `1em` (14px) is
beaten by the class rule (16px), because presentation attributes have no
specificity. Measured live: `class` absent, `width="1em"`, rendered `16 × 16`.

It is also the exact mirror of `sidebar-toggle-icon` next door, which pins
`size-4` *to escape the same rule*. Two icon files in one directory, opposite
answers, and one class between them.

## 2. Why the negative margin is not carried here

`button::ICON_MARGIN_X` documents a real taffy defect — a negative inline margin
on an in-flow flex item collapses a content-sized flex container — and
`button::Size::glyph_box` folds the margin into the glyph's box (16 → 12) to
work around it.

**Neither applies to this anchor.** A margin lies outside the border box, so the
reference's `bounds.w` is 16 and not 12; and the toggle button pins `size-6`, so
its main size is definite and the defect cannot bite. The port therefore uses
`button::Size::Default::icon(breakpoint)` — the icon rule — and no margin.

## 3. Values — `preserveCase`

`<span className="ui-font ui-text-xs font-semibold">Aa</span>`.

| React / Tailwind | Compiles to | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| *(blockified)* | live computed `display: block` — it is a flex item of the `inline-flex` button, and CSS blockifies those | `div()` | not a field |
| `ui-font` | `font-family: var(--app-font-family, var(--font-sans))` → `CalSansUI` | `.font_family(theme.font_sans…)` | `font.family` = `CalSansUI` |
| `ui-text-xs` | `font-size: var(--ui-text-xs)` = `calc(0.6875rem * 1)` = **11px**, and **no line-height** | `theme.ui_text_xs.value()` | `font.size` = 11 |
| *(inherited)* `sm:text-sm`'s `line-height: calc(1.25 / 0.875)` | `1.4285714` × 11 = **15.714286px** | `inherited_line_height(theme, Sm)` | `font.line_height` = 15.71 |
| *(inherited)* `text-base`'s `line-height: calc(1.5 / 1)` below `sm` | `1.5` × 11 = **16.5px** | `inherited_line_height(theme, Base)` | moves at the breakpoint |
| `font-semibold` | `--font-weight-semibold: 600` | `WEIGHT` | `font.weight` = 600 |
| *(inherited)* button `text-muted-foreground` | `oklch(0.72 0 0)` | `theme.color_muted_foreground` | `fg` = `#a4a4a4ff` |
| *(inherited)* button **active** `text-foreground` | `oklch(0.97 0 0)` | `theme.color_foreground` | `fg` = `#f5f5f5ff` |
| `Aa` | — | `LEGEND` | `text`, `text_width` 14.48 |

**`ui-text-xs` sets a font-size and no line-height**, which is the whole reason
the box is 15 rather than 16: the ratio is inherited from the *button's* type
step and applied to the span's own 11px. Measured live at `15.714286px`, and
WebKit floors the line box to a whole logical pixel → `bounds.h` **15**.

## 4. Declarations — both, and only on the run

| | Value | Why |
|---|---|---|
| `content_sized` | **true** on `preserve-case` | No authored width; the box is the run's max-content width. Reference `bounds.w 14.48` against `text_width 14.48` |
| `line_sized` | **true** on `preserve-case` | No authored height either, so the box **is** the line box |
| both | **false** on the three glyphs | Their box is authored by the button's icon rule, and they paint no text |

**`line_sized` is not free here**, unlike `label` where both comparisons asked
the same question. The reference is `bounds.h 15` against
`font.line_height 15.71`. Without the declaration the port's snapped `15.5` would
be compared against `15` — **exactly on the ±0.5 boundary**. With it, `15.5`
against `15.714` is `0.214`. This is the first component in the port where the
declaration is load-bearing rather than merely true.

§8.3's `empty` drops both: it authors a zero box on each axis, and a box that
did not size itself to anything must not claim it did.

## 5. Reachability — **4 live instances, 3 without the replace row**

| Toggle | `find-bar.tsx` | `terminal-search.tsx` |
|---|---|---|
| `caseSensitive` | yes | yes |
| `wholeWord` | yes | yes |
| `regex` | yes | yes |
| `preserveCase` | **only while the replace row is expanded** | **never** |

Measured: opening the editor find bar (the breadcrumb's *Find in file* button)
gives 3 live `[data-oracle-id="search-toggle-icon"]`, and expanding replace gives
**4**. Driving the run-shaped reference therefore needs the editor's find bar
*with replace open*; the terminal's search can never produce it.

All four are `<Button variant="ghost" compact>` children carrying
`searchToggleButtonVariants` — `flex size-6 items-center justify-center
rounded-lg border border-transparent`.

> **An aside worth recording, on the button rather than on this anchor.** Those
> toggle buttons measure **24 × 32** live, not 24 × 24: the className's unprefixed
> `size-6` and the variant's `sm:h-8` are different tailwind-merge modifier
> groups, Tailwind emits the `sm:` later, and it wins on the height only. That is
> `native/MAPPING.md`'s "a call site's unprefixed class can be dead above 640px"
> firing a third time, and it belongs to `button`'s surface, not this one.

## 6. `selected` is real — **and only on one of the two shapes**

`searchToggleButtonVariants({ active: true })` swaps the button's
`text-muted-foreground` for `text-foreground`. Measured by toggling the control
live: the icon's computed `color` moves `oklch(0.72 0 0)` → `oklch(0.97 0 0)`.

* On `preserve-case` the run inherits it and **`fg` is a compared field**.
* On the three glyphs it reaches `fill: currentColor`, which has **no field** —
  so driving `selected` there compares resting against resting and proves
  nothing.

The flag is left **modelled** rather than declared unmodelled because it is real
on the surface's own default cell, and `Surface::unmodelled` is a per-surface
claim rather than a per-cell one. The caption says which of the two a given cell
is, so nobody reads a glyph `selected` run as evidence.

`hover` and `focus` are unmodelled: `search.tsx` puts both on the button
(`hover:border-border/70 hover:bg-muted`), which is a different anchor on a
different surface.

## 7. One id, four renderings — `ANCHORS.md` v1.8

All four entries carry `data-oracle-id="search-toggle-icon"`, exactly as the four
live `<Kbd>`s all carry `kbd`. They are siblings in the same options row, so four
*distinct* ids would name a set no single root contains and no snapshot could
hold. One id plus `extractSnapshot`'s `index` gives one anchor rooted at the
glyph itself, which is the only arrangement that compares — and it is what both
references were taken with (`index: 3` for the run, `index: 0` for a glyph).

## 8. What is **not** modelled

* **The three glyphs' art.** Empty boxes, per §0.
* **`--content`** is vacuous: the legend is hard-coded `Aa` in the primitive and
  no call site supplies it.
* **`--width`** is vacuous: neither shape stretches.
* **`--theme`** is real on the run and vacuous on the glyphs, for §6's reason.
