# `select` (P3.15) — the wrap that **cannot be measured**

`web/src/components/ui/select.tsx` →
`crates/crowbar-ui/src/components/select.rs`, built on
`gpui_component::select::Select`.

> A §6.2 row, in the shape `native/MAPPING.md` fixes. Kept in its own file
> because P3 runs several workers in parallel and one appended table is one
> conflict per item.

**Reference:** `/tmp/p3-ref-select.json`, captured live from Settings →
Appearance → Theme at a 1714px viewport, dark. Root `select-trigger`, three
anchors.

**Live count: 5 `SelectTrigger`s on screen** in the settings dialog, from 7
consumer files (`appearance-settings`, `editor-settings`, `terminal-settings`,
`file-tree-settings`, `keybindings-settings`, `developer-settings`,
`font-selector`). The popup is reachable too — `HTMLElement.click()` on a
trigger opens it, in the *next* `execute_js` call.

## 0. ‼️ THE FINDING: there is no surface, and no anchor is possible

**Read this before looking for `surfaces/select.rs`. There is none, and the
omission is the result rather than a gap.**

`AnchorSink`'s methods take a `gpui::Div` — an element **`crowbar-ui` holds** —
and wrap it in a recording element. Every box `select.tsx` styles is constructed
*inside* `gpui-component` and never passes through this crate:

| React anchor | who builds the box | reachable? |
|---|---|---|
| `select-trigger` | `SelectState::render`'s `div().id("input")` | no |
| `select-value` | `SelectState::render`'s `div().id("title")` | no |
| `select-icon` | `select::Caret`, an `Icon` | no |
| `select-popup` / `select-panel` / `select-list` | `SelectState::render`'s `deferred(anchored(v_flex(…)))` | no |
| `select-item` | `SearchableListItemElement::render`'s `h_flex()` | no |
| `select-item-indicator` | the same, two `h_flex`es deeper | no |
| `select-item-text` | the delegate's children, inside that | no |

`Select` exposes exactly **one** styling seam — its `Styled` impl, which lands
in `SelectState.style` and reaches the trigger box through `refine_style` — and
a `StyleRefinement` is not an element. There is no trigger builder, no content
closure, and no item builder that yields the item's *box*.

The only thing this crate could anchor is a `div()` wrapped *around* the whole
widget. That box is **not** `select-trigger`: it is an extra layer whose bounds
happen to coincide, it says nothing about `select-value` or `select-icon`, and a
snapshot built from it would compare one box and read as converged. That is the
fake convergence `ANCHORS.md` exists to refuse, so it was not done.

**So: the appearance below is real and shipping; the parity claim is not made.**

### Contrast with `popover`, which is the useful part

`popover` wraps the same way and *does* converge, because
`gpui_component::Popover` accepts **children** — so the popup box is
`crowbar-ui`'s own `Div` and carries the root anchor, while the vendor keeps the
behaviour. **A widget is wrappable-and-measurable exactly when it lets the
caller supply an element, not merely a style.** That is the test to apply to the
remaining §6.2 list before starting one:

| accepts a caller-built element | measurable |
|---|---|
| `popover` (`ParentElement`), `dialog`, `sheet`, `resizable` (panels), `sidebar`, `form` | likely |
| `select`, `combobox`, `slider`, `switch`, `native_menu` | **no** — style-only seams |

### The two ways out, neither taken here

1. **Widen `AnchorSink`.** `crowbar_driver::anchor` is already generic over
   `E: InteractiveElement + IntoElement`; it is `AnchorSink` that narrows to
   `Div`, to stay object-safe behind `&dyn`. Widening it would let a
   `Stateful<Div>` be anchored — but the vendor's boxes still never reach the
   caller, so **this alone does not help**. It is listed because it is the
   change people will reach for first, and it is not the one.
2. **Fork the widget into `crowbar-ui`.** Reproduces `select.tsx`'s structure
   exactly and anchors every box — at the cost of §6.2's whole point, which is
   confining the upgrade surface. This is the real decision, and it is the
   orchestrator's.

## 1. Values — the trigger (applied, and measured live)

`size="sm"` at a 1714px viewport, which is what every settings row uses.

| React / Tailwind | Compiles to (measured) | gpui / `crowbar-ui` | Applied? |
|---|---|---|---|
| `sm:min-h-7` (size `sm`) | `min-height: 28px` | `Size::Small.min_height(Sm)` | yes |
| `px-[calc(--spacing(2.5)-1px)]` | `padding-inline: 9px` | `Size::Small.padding_x()` | yes |
| `rounded-lg` | `border-radius: 10px` | `theme.radius_lg` | yes |
| `border border-input` | `1px oklch(1 0 0 / 0.08)` | `theme.input` | yes — the vendor's unconditional `.border_1()` is already the right width |
| `bg-background`, `dark:bg-input/32` | `oklab(1 0 0 / 0.0256)` | `theme.background` | yes |
| `text-foreground` | `oklch(0.97 0 0)` | `theme.foreground` | yes |
| `sm:text-sm` | `font: 14px/20px` | `ui_text_base` + `relative(1.25/0.875)` | yes |
| `min-w-36` | `min-width: 144px` | `TRIGGER_MIN_WIDTH` | yes |
| `shadow-xs/5` | one shadow layer | — | **§6: no field either side** |
| `before:shadow-[…]` | a pseudo-element | — | §6, and a pseudo carries no anchor |
| `gap-1.5` | `gap: 6px` | `Size::Small.gap()` — **recorded, not applied** | **no** (see §2) |
| `sm:[&_svg…]:size-4` | `16px` | `icon_size(Sm)` — recorded | **no** (see §2) |
| `flex-1 truncate` on the value | — | — | **no** (see §2) |

### The `sm:` trap fires **four times, all in the same direction**

`min-h-9 sm:min-h-8`, `min-h-8 sm:min-h-7`, `size-4.5 sm:size-4`,
`text-base sm:text-sm`. Every one gets **smaller** above 640px, which is the
opposite of the usual mobile-first reading — a port that assumed "wider
viewport, bigger box" would have all four backwards. `Breakpoint` is a
parameter, as on `input`, and a unit test asserts the direction rather than the
values so the trap cannot be re-introduced by editing a number.

`Size::Large` is a further trap: it is **not** one step up from `Default`. Above
the breakpoint it is the *base* `Default`'s 36px, and below it the two are both
32. A port treating the three as one scale is a step out at one of the two
viewports.

## 2. What resisted, precisely

Four differences no amount of styling reaches, each a **structural** choice
inside the vendor's `render`:

1. **The caret is the wrong glyph.** `select.tsx` uses lucide's
   `ChevronsUpDown` — two chevrons pointing apart. `select::Caret` renders
   `IconName::ChevronDown`, and `Select::icon` can only substitute another
   `gpui_component::Icon`; that set has no `ChevronsUpDown`
   (`grep -c ChevronsUpDown vendor/gpui-component/src/icon.rs` → 0). Visible.
2. **The trigger's internal gap is 4px, not 6.** The vendor's inner `h_flex()`
   carries `gap_1()`; `select.tsx` at `size="sm"` carries `gap-1.5`. That flex
   is private; `refine_style` reaches only the outer box.
3. **The value box is `w-full`, not `flex-1`.** With a sibling caret those are
   different lengths, so the truncation point differs — and `text_width` +
   `clipped` are the exact pair `ANCHORS.md` was built to catch that with. The
   live reference has `select-value` at `w: 138` inside a 176px trigger; a
   `w-full` child would be 158.
4. **The item is a flex row, not a two-column grid.**
   `SearchableListItemElement` is an `h_flex` with a **trailing** check;
   `select.tsx`'s item is `grid grid-cols-[1rem_1fr]` with a **leading**
   indicator. The tick is on the other side of the row.

(1) and (2) are cosmetic-but-visible. (3) and (4) would each be a delta on every
cell of the matrix — if a cell could be taken at all.

## 3. Values — the popup (measured, ported nowhere)

Recorded so the numbers exist when the decision in §0 is made. Measured live
with the Theme select open.

| element | measured |
|---|---|
| `select-popup` | `206 × 66`, no paint of its own |
| `select-panel` | `206 × 66`, `rounded-lg` 10, `border` 1px `#ffffff0f`, `bg-popover` `oklch(0.239 0.002 106.5)` |
| `select-list` | `204 × 64`, `p-1`, `overflow-y: auto` |
| `select-item` | `196 × 28`, `grid`, `grid-template-columns: 16px 148px`, `gap: 8px`, `rounded-sm` 6, padding `4 16 4 8`, `14px/20px`, selected `bg oklch(1 0 0 / 0.04)` |

`select-item` appears **twice** in that popup, so a capture of the popup root is
a duplicate-id document — `ANCHORS.md` v1.8 makes that a refusal, and resolving
it needs the same `oracleSurfaceScope` entry `popover` wants. Only the trigger
root was captured.

## 4. What wrapping cost

| | |
|---|---|
| `appearance(false)` | the vendor's `true` paints `input_style(…)`, `cx.theme().input` and `cx.theme().radius` — **`gpui-component`'s theme**. Off, then repainted from `Theme` through `Styled`. |
| `.border_1()` survives it | the vendor sets the border width *unconditionally*, before the appearance block; only the colour is inside it. That is the width `select.tsx` wants, so only the colour is restated. |
| `input_size` / `input_text_size` | the vendor derives height, padding and text size from its own `Size`. All three overridden: `select.tsx`'s three sizes are not `gpui-component`'s. |
| `Window::use_keyed_state` | `SelectState::new` needs a `&mut Window` and a `&mut Context`, and `SurfaceParams::render` has neither. The wrapper is therefore a `RenderOnce` — the one place this crate is handed both. **This is why `select` is constructible at all**, and it is the pattern the rest of the stateful §6.2 widgets will need. |

## 5. The second frame

`select` has `popover`'s two-frame problem in a nastier form. Its popup renders
on frame 1, but sized off `SelectState.bounds`, which is still
`Bounds::default()` until the trigger's `on_prepaint` — so the menu comes out
**2px wide** (`bounds.size.width + px(2.)`) rather than absent. A one-frame
capture would therefore look like a *port defect* rather than a missing frame.
`native/mapping/popover.md` §4 carries the mechanism and the measurement.
