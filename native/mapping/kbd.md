# `kbd` (P3.6)

`web/src/components/ui/kbd.tsx` →
`crates/crowbar-ui/src/components/kbd.rs`.

> A §6.2 row, in the shape `native/MAPPING.md` fixes. Kept in its own file
> because P3 runs four workers in parallel and one appended table is one
> conflict per item.

Two plain `<kbd>` elements with one class list each and no library underneath —
the simplest component in the port so far. Every "Compiles to" below came from
running the app's own `src/index.css` through its own `tailwindcss` 4.3.0 with
the utility as a candidate.

**Reference:** `/tmp/p3-ref-kbd.json`, captured live from the workspace
switcher's `CommandFooter` at a 1714px viewport.

## 0. The headline: `border.w` is **0**, which is `badge`'s trap in reverse

`native/MAPPING.md` records that **`border` is 1px on every button and badge
variant**, because those class lists carry a bare `border` that compiles to
`border-width: 1px` unconditionally — five badge variants pay the pixel while
leaving the colour transparent.

**`kbd.tsx` carries no `border` at all.** Preflight sets `border: 0 solid` on
every element and nothing puts it back, so the keycap pays nothing. Measured
live — `borderTopWidth: "0px"` — and the reference agrees:

```json
"border": { "w": 0, "color": "#ffffff0f" }
```

The colour is `--color-border` resolved by the cascade and never painted. A port
that carried `badge`'s pixel across by habit would have been wrong on every cell
**and** wrong about the box, `box-sizing: border-box` making the pixel eat into
the 4px padding. This is why the rule in `MAPPING.md` is "measure rather than
infer" and not "borders are 1px".

## 1. Values

| React / Tailwind | Compiles to | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `inline-flex` | `display: inline-flex` | `.flex()` | live computed `display` is **`flex`** — every cap is a flex item and CSS blockifies those |
| `items-center` | `align-items: center` | `.items_center()` | not a field |
| `justify-center` | `justify-content: center` | `.justify_center()` | not a field |
| `gap-1` | `calc(--spacing * 1)` = **4px** | `GAP` | not a field |
| `h-5` | `calc(--spacing * 5)` = **20px** | `HEIGHT` | `bounds.h` = 20 |
| `min-w-5` | `min-width: calc(--spacing * 5)` = **20px** | `MIN_WIDTH` | floors `bounds.w` |
| `px-1` | `padding-inline: calc(--spacing * 1)` = **4px** | `PADDING_X` | in `bounds.w` |
| `rounded` | `border-radius: 0.25rem` — a **literal**, not a `--radius-*` step | `RADIUS` | `radius` = 4 |
| `bg-muted` | `background-color: var(--muted)` = `oklch(1 0 0 / 4%)` | `theme.color_muted` | `bg` = `#ffffff0a` |
| `text-muted-foreground` | `color: var(--muted-foreground)` = `oklch(0.72 0 0)` | `theme.color_muted_foreground` | `fg` = `#a4a4a4ff` |
| `text-xs` | `font-size: var(--text-xs)` = 12px, `line-height: var(--text-xs--line-height)` = `1/0.75` → **16px** | `TYPE_STEP` | `font.size` 12, `font.line_height` 16 |
| `font-medium` | `font-weight: 500` | `WEIGHT` | `font.weight` = 500 |
| `font-sans` | `--default-font-family` → `CalSansUI` | `.font_family(theme.font_sans…)` | `font.family` = `CalSansUI` |
| `[&_svg:not([class*='size-'])]:size-3` | **12px** | `GLYPH` | an empty box — no native SVG |
| `pointer-events-none`, `select-none` | not visual | — | not a field |

`rounded` is worth a second look: `rounded-sm` next door is
`calc(var(--radius) * 0.6)` = 6px and **moves with the theme**, where a bare
`rounded` is pinned at `0.25rem`. Two names one character apart, one themed and
one not.

## 2. Declarations

| | Value | Why |
|---|---|---|
| `content_sized` | **true** | The used width is the legend's max-content width plus `px-1`, floored by `min-w-5` — and `min-w-*` is a floor, never a stretch. `badge`'s reasoning exactly |
| `line_sized` | **false** | `h-5` **authors** the box at 20px around a 16px line box. `ANCHORS.md` v1.6's test is whether the height is *derived from* the line box, not whether the element paints text |

The reference is the evidence for the second: `bounds.h 20` against
`font.line_height 16`. Declaring it would have compared the two and manufactured
a **4px delta on this surface's only anchor** — the precise failure v1.6 was
written about, now confirmed on a second component.

`crates/crowbar-app/src/row_layout/kbd.rs` carries a control for it: the box's
height stays 20 across all three `--content` lengths while the width moves, so
"authored" is a measurement rather than a reading of the class list.

## 3. `KbdGroup` is ported but **not anchored** — `ANCHORS.md` v1.8

The group is real: the switcher's footer wraps its ↑/↓ caps in one, measured at
`44 × 20` (two 20px caps and one 4px gap).

It cannot be a snapshot root. `data-oracle-id="kbd"` lives on the **primitive**,
so every cap inside a group carries it, and a snapshot rooted at the group would
contain that id twice. v1.8 ranks a duplicate id a **refusal** rather than a
delta — the differ matches by id and "would have no way to say which of the two
it compared". So `KbdGroup` exists in the port as the layout its caps sit in and
carries no id, and the `--group` cell asserts the anchor set is still exactly
one.

**One measurable oddity, recorded because nothing else will notice it.** The
group does **not** set `font-sans`, so it keeps the UA's monospace default for
`<kbd>`. Measured live:

```text
[data-slot="kbd"]        font-family  CalSansUI
[data-slot="kbd-group"]  font-family  JetBrains Mono Variable
```

The group paints no text of its own, so nothing observable follows today — but
two elements in one file resolving different families is the kind of difference
that stays invisible until something inherits through it.

## 4. Reachability

**4 live `[data-slot="kbd"]` and 1 `[data-slot="kbd-group"]`**, all in the
workspace switcher's `CommandFooter`, reached by clicking the
`command-dialog-trigger`. Three caps hold icons and sit exactly on the 20px
floor; the fourth is `Esc` and overruns it at 27.61 — which is why `Esc` is the
captured cell and the fixture. The floor case is reachable from the same footer,
so both branches of `min-w-5` are live.

## 5. What is **not** modelled

* The `<svg>` in an icon cap is an empty 12px box. The same call every component
  since `git_status_row` has made: there is no native equivalent, and drawing a
  substitute would put a shape on screen for the oracle to converge on.
* `--viewport-width` is **vacuous** here: `kbd.tsx` contains no `sm:` variant at
  all — the second component in the port with none, after `avatar`.
