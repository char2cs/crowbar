# The §6.2 primitive mapping table

Spec §6.2 makes this a Phase 2 deliverable: *"Each of the 46 portable
`components/ui` primitives maps to exactly one `crowbar-ui` module."* What the
spec does not say, and what actually costs the time, is **how each Tailwind
construct becomes a gpui one** — so that is what this file is.

> **Append, do not rewrite.** One section per component, in the order they were
> ported. An entry that was right for `dropdown-menu` and wrong for the next
> component is a *new row saying so*, not an edit to the old one: the value of
> this file is that a later reader can see which translations held.

**Read the "Traps" section of every component before porting a new one.** Every
row there is a translation that compiles, renders something plausible, and is
wrong — which is the only class of mistake this file can save anyone from.

---

## How to read a row

| Column | Means |
|---|---|
| React / Tailwind | the construct as it appears in `web/src/` |
| Compiles to | what the app's **own** Tailwind emits, read out of it rather than assumed |
| gpui / `crowbar-ui` | the translation |
| Oracle | `compared` — the differ sees it; `invisible` — no field for it (`ANCHORS.md` §6); `absent` — not ported |

**Compile the CSS, do not read the class name.** Every "Compiles to" below came
from running the app's own `src/index.css` through its own `tailwindcss` 4.3.0
with the utility as a candidate. Three of the numbers in the `dropdown-menu`
table are *not* Tailwind's stock values, because `theme.css` redefines them.

---

# `dropdown-menu` (P2.1)

`web/src/components/ui/dropdown-menu.tsx` →
`crates/crowbar-ui/src/components/dropdown_menu.rs`.

Unlike the two Phase 1 rows — whose class lists were half-dead under
`file-explorer-tree.css` — nothing overrides these classes from a stylesheet, so
the `cn(…)` calls are what renders.

## 1. Values: spacing, type, radius, colour

| React / Tailwind | Compiles to | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `p-1`, `py-1` | `calc(var(--spacing) * 1)` = **4px** | `px(SPACING)`, `SPACING = 4.0` — one constant, every multiple derived from it | compared |
| `px-1.5`, `gap-1.5` | `calc(--spacing * 1.5)` = **6px** | `px(SPACING * 1.5)` | compared |
| `pl-7` | **28px** | `ROW_INSET_PADDING_LEFT` | invisible (see traps) |
| `pr-8` | **32px** | `TICK_ROW_PADDING_RIGHT` | compared |
| `right-2` | **8px** | `.right(px(8.0))` | compared |
| `size-4` | **16px** | `.w(ICON_SIZE).h(ICON_SIZE)` on an **empty** div | compared |
| `min-w-32` / `min-w-40` | 8rem / 10rem = **128 / 160px** | `.min_w(…)` | compared |
| `h-px` | **1px** | `.h(px(1.0))` | compared |
| `rounded-lg` | `var(--radius)` = **10px**, *not* Tailwind's stock 8 — `theme.css` redefines `--radius-lg` | `theme.radius_lg.value()` | compared |
| `rounded-md` | `calc(var(--radius) * 0.8)` = **8px**, *not* stock 6 | `theme.radius_md.value()` | compared |
| `text-sm` | `--text-sm: 0.875rem`, line-height `calc(1.25 / 0.875)` → **14px on 20px** | `theme.ui_text_base.value()` + `relative(1.25/0.875)` | compared |
| `text-xs` | `--text-xs: 0.75rem`, line-height `calc(1 / 0.75)` → **12px on 16px** | `theme.ui_text_sm.value()` + `relative(1.0/0.75)` | compared |
| `font-medium` | `--font-weight-medium: 500` | `FontWeight::MEDIUM` | compared |
| `bg-popover`, `text-popover-foreground` | `var(--popover)` / `var(--popover-foreground)` | `theme.popover`, `theme.popover_foreground` | compared |
| `bg-border` | `var(--border)` | `theme.border` | compared |
| `focus:bg-accent` / `focus:text-accent-foreground` | — | `theme.accent` / `theme.accent_foreground` | compared |
| `data-[variant=destructive]:focus:bg-destructive/10`, `dark:…/20` | `color-mix(… N%, transparent)` | `theme.destructive.mix(10 or 20, Color::TRANSPARENT)` | compared |

**The `--ui-text-*` trade, stated once for the whole port.** Tailwind's `text-sm`
is 0.875rem and `--ui-text-base` is the same 0.875rem; `text-xs` is 0.75rem and
`--ui-text-sm` is the same 0.75rem. There is no token for Tailwind's own scale,
and inventing one would put a value in the design system the design system does
not have — so the port reads the token that carries the same number and says so
at the call site. `file_tree_row` made the same trade for `text-sm`.

## 2. Layout constructs

| React / Tailwind | Compiles to | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| a plain `div` child of a `div` | block layout | gpui's `Style::default()` is `Display::Block` — **do not** add `.flex()` to reproduce a block container | compared |
| `-mx-1` | `margin-inline: -4px` | `.mx(px(-4.0))`. taffy honours negative block margins exactly as CSS does — **measured**, see `the_separator_bleeds_out_through_the_popups_padding` | compared |
| `w-(--anchor-width)` + `min-w-40` | `width: 24px; min-width: 160px` | declare **both** (`.w(…).min_w(…)`) and let taffy clamp — reproduce the two declarations, not their resolution, or a taffy that clamped differently would be pre-empted here | compared |
| `overflow-x-hidden overflow-y-auto` | scroll container in both axes | `.overflow_hidden()`. gpui's `overflow_y_scroll` is on `StatefulInteractiveElement` and needs an element id + scroll handle — runtime state `ANCHORS.md` §6 makes invisible. taffy zeroes the automatic minimum size for `Hidden` and `Scroll` alike, and macOS overlay scrollbars reserve no gutter in either engine, so the two are layout-identical here | compared (via `clipped`) |
| `absolute right-2` inside `flex items-center` | abspos with an auto block-axis inset takes its static position from the container's alignment | `.absolute().right(px(8.0))` — taffy applies the parent's `align_items` to an auto inset, so it centres. **Measured**, not assumed | compared |
| `ml-auto` | `margin-left: auto` | `.ml_auto()` | compared |
| a text node with no element around it | anonymous flex item | `AnchorSink::text_half` placed by the caller, so `[icon, label, chevron]` keeps its DOM order. `boxed_text` appends the run **last** and would reverse the last two | compared |
| `[&_svg:not([class*='size-'])]:size-4` + a call site's own `size-4` | 16px either way | one empty 16px box; no glyph — see the icon note below | compared |

**Icons are empty boxes.** The same call `git_status_row` and `file_tree_row`
made: the React icon is an SVG a call site chooses, there is no native
equivalent, and drawing a substitute would put a shape on screen for the oracle
to converge on. The box is what the contract measures.

## 3. No gpui equivalent

| React / Tailwind | Why | What the port does |
|---|---|---|
| `tracking-widest` (`letter-spacing: 0.1em`) | gpui has **no letter-spacing at all** | nothing. The shortcut is rendered without it, and is **deliberately unanchored**: a shaped advance short by 0.1em per character is several px against `ANCHORS.md` §5's ±1.0px `text_width` tolerance, so an anchor there would report a delta that says nothing about the port |
| `max-h-(--available-height)` | Floating UI writes it at runtime from the viewport | **absent.** Positioner output, not a component property. The port renders the popup, not its placement |
| `w-auto` on `DropdownMenuSubContent` | shrink-to-fit on an *absolutely positioned* popup | **absent.** Needs the positioner; the submenu popup is its own surface and its own item |
| `MenuPrimitive.Group` / `RadioGroup` | unstyled `<div role=group>` | **absent.** No padding, no border, no background — its children lay out identically without it |
| `data-open:animate-in`, `zoom-in-95`, `duration-100` | transitions and keyframes | **absent.** `ANCHORS.md` §6: a snapshot is one instant. §6.3 says transitions are re-implemented by measurement, not translated — and there is nothing to measure until there is a reason to animate |

## 4. Painted but invisible to the oracle

`ANCHORS.md` §6 has no field for any of these. They are painted for fidelity and
the differ will never confirm or deny them — **say so in any report**.

| React / Tailwind | gpui | Note |
|---|---|---|
| `shadow-md` | `.shadow_md()` | gpui's preset is **byte-identical** to Tailwind's: `0 4px 6px -1px rgb(0 0 0/.1), 0 2px 4px -2px rgb(0 0 0/.1)`. Reaching for it also solves a rule-4 problem — Tailwind's default `--tw-shadow-color` is a literal no token carries, and `check-invariants.sh` rule 4 will not let a component mint one. gpui's copy lives inside gpui |
| `ring-1 ring-foreground/10` | a third `BoxShadow`, 0 blur, `spread_radius: 1px`, colour `theme.foreground.mix(10.0, TRANSPARENT)`, **inserted in front** of `shadow_md`'s two | Tailwind's composite is `box-shadow: <ring>, <shadow>`, and `Styled::shadow_md` sets the whole list — so the ring is inserted via `Styled::style`, which is the public seam. See the trap below |
| `opacity-50` on a disabled row | `.opacity(0.5)` | the contract has no opacity field, and the DOM's `getComputedStyle().color` is unaffected by it either, so both extractors agree by reporting nothing |

## 5. Anchoring

| Construct | Decision |
|---|---|
| ids in the primitive | `dropdown-menu.tsx` carries a **per-slot default** id, written *before* `{...props}` so a call site can override it |
| a menu with several rows of one kind | the call site names them. The primitive has no index to build unique ids from, and `React.useContext` would be a refactor. `review-thread-item.tsx` names its three items |
| uniqueness | required **within one popup only**: `extractOracleSnapshot` walks `[data-oracle-id]` under the root anchor, and the root is `menu-popup` |
| `DropdownMenuSubContent` | `menu-sub-popup`, not `menu-popup` — a submenu is a second portal, and two `menu-popup`s in one document make the extractor's root lookup ambiguous |
| `CONTENT_SIZED` / `LINE_SIZED` | **both empty**, and empty *lists* rather than absent, so the React side's (also empty) declaration set is diffable against them |

## 6. Traps

Each of these compiles, renders something plausible, and is wrong.

| Trap | What actually happens |
|---|---|
| **`ring-1` is not a border.** | Tailwind 4 compiles it to `--tw-ring-shadow: 0 0 0 1px …`, a **box-shadow**. A port reaching for `.border_1()` reports `border.w: 1` against the reference's `0` — the one field `ANCHORS.md` v1.1 compares *exactly* — on every cell of the matrix, **and** is wrong about the paint, because a border takes space in the box model and a ring does not. The popup's visible edge is a shadow, and neither extractor can see it |
| **Declaring a row `line_sized`.** | The badge trap in a new shape. A row paints text and has a box, so it reads like the case v1.6 was written for. `py-1` puts 4px above and below a 20px line box, so the border box is 28 against a 20px line — declaring it compares 28 to 20 and invents an 8px delta on an anchor both engines agree on. Same for the label: 24 against 16 |
| **Declaring anything `content_sized`.** | Every row is a **block-level** child of the popup, so its used width is the popup's content width whatever its text says. v1.5 exists for boxes whose used width *is* a text run's max-content width; nothing here is one |
| **`data-inset:pl-7` adds to `px-1.5`.** | It **replaces** the left half. Different tailwind-merge groups, and the `data-inset:` variant is written later |
| **An `overflow` label with spaces.** | It **wraps** — a menu row has no `truncate` and no `whitespace-nowrap` — and a wrapped run is outside what the contract can compare: the DOM extractor sums a `Range`'s client rects while the gpui side shapes the whole string on one line, so every wrap costs about a space's advance against a ±1.0px tolerance. The fixture's long string is one unbreakable token (no space, no hyphen, no slash) so it clips instead, which both extractors *do* agree on |
| **A destructive row turning accent-coloured on focus.** | `data-[variant=destructive]:text-destructive` is unconditional and beats `focus:text-accent-foreground`. The class list restates the same red under `:focus` precisely so that it does |
| **Assuming `hover` and `focus` are different cells.** | There is **no `hover:` rule anywhere** in `dropdown-menu.tsx`. The only interaction style a row carries is `focus:`, and `base-ui` reaches it by moving DOM focus to the row under the pointer. Both flags select the same paint — a *reading* of `base-ui`, recorded as one |
| **Keeping the row's item id when `--tick` makes it a checkbox.** | A different primitive is a different element with a different default `data-oracle-id`. Leaving `menu-item-copy` on a `CheckboxItem` names an anchor the reference cannot produce, which is a `FieldPresence` delta that forgives nothing |

## 7. What this surface cannot show the differ

Recorded because §8.2 requires honesty about it, and because a reader will
otherwise assume the oracle covers more than it does.

- **`inset` is invisible.** The leading padding is inside the row box, and the
  two things it moves — the label (the row's own *text node*, with no element
  around it) and the leading icon (a call-site SVG) — are both unanchorable. Every
  anchored box is identical with and without it. Asserted as such.
- **The `overflow` content cell has no reference.** No menu in the app carries a
  label long enough to exceed a 160px popup, and the labels come from call sites
  this item may not modify. The native side renders the cell; the reference
  cannot. The same shape of finding as Phase 1's `git-row-dir`.
- **`selected` needs `--tick`.** The comment menu has no checkbox or radio row,
  so a bare `--flags selected` renders resting on both sides. The caption says so
  per cell, because `Surface::unmodelled` is per surface and this is per cell.
- **`empty` is unmodelled.** A menu with no rows is not a state the app has:
  every menu in it renders at least one unconditional row.

---

## Cross-component notes

Things learned here that are **not** about `dropdown-menu` and that the next
component should not re-derive.

| Note | |
|---|---|
| Compile the CSS | `tailwindcss` 4.3.0's JS API: `import { compile } from 'tailwindcss/dist/lib.mjs'`, base `web/src`, then `compiler.build([…candidates])`. Three of this component's numbers are not Tailwind's stock values |
| gpui's `Style::default()` is `Display::Block` | a `div()` is a block container, `flex_shrink: 1.0`, `min_size: auto`. So CSS block layout ports one-for-one and only flex containers need `.flex()` |
| gpui's shadow presets are Tailwind's | `shadow_2xs` … `shadow_2xl` carry Tailwind's exact offsets, blurs, spreads and `rgb(0 0 0 / .1)`. Using them is also the only way to paint a Tailwind shadow without minting a colour outside `crowbar-ui/src/theme/` |
| gpui has no letter-spacing | any `tracking-*` utility is unportable today |
| `Styled::style()` is the seam | for anything the builder methods cannot express — here, inserting a shadow layer in front of a preset |
