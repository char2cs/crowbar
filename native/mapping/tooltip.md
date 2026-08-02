# `tooltip` (P3.22) — built, not wrapped; a distinct surface from `popover`'s unreached variant

`web/src/components/ui/tooltip.tsx` (`@radix-ui/react-tooltip`) and the
byte-identical duplicate `button.tsx` carries inline for its own
`tooltip`/`shortcut` props →
`crates/crowbar-ui/src/components/tooltip.rs`, built from raw `div()`s.

> A §6.2 row, in the shape `native/MAPPING.md` fixes. Kept in its own file
> because P3 runs several workers in parallel and one appended table is one
> conflict per item.

**Reference:** `/tmp/p3-ref-tooltip.json`, captured live through
`HTMLElement.focus()` on `tab-bar-item.tsx`'s close button (`tooltip="Close"
shortcut="⌘W"`), at a 1714px viewport, dark, resting (`data-state:
instant-open`).

**Live count: 24 total call sites, all reachable** — 21 `<Button tooltip=…>`
(outside the Plate set; `floating-toolbar-buttons.tsx`,
`equation-toolbar-button.tsx`, `link-toolbar-button.tsx`, `table-node.tsx`,
`markdown-view-toggle.tsx` and `comment-plugins.tsx` are Plate's and excluded)
plus 3 standalone `<Tooltip>` (`terminal-settings.tsx`,
`path-breadcrumb.tsx`'s non-interactive arm, one inside the Plate floating
toolbar). Both routes render byte-identical `tooltipContentBase` output — see
§0. Confirmed live, twice: the `Close ⌘W` fixture above, and
`path-breadcrumb.tsx`'s `a.ts` segment (no shortcut), `demo/src/a/a.ts` at
`111.28125 × 32`.

## 0. The seam test, applied — and why the verdict is the opposite of `popover`'s

§10.1 says not to rebuild a primitive `gpui-component` already has, and
`native/vendor/gpui-component/src/tooltip.rs` does have a `Tooltip`. The test
`popover`'s own module docs set — **a widget is wrappable-and-measurable
exactly when it lets the caller supply an element, not merely a style** — was
applied here in full, not assumed from the name:

| seam | what it reaches | wrappable? |
|---|---|---|
| `Styled::style()` → `StyleRefinement` | the private `h_flex()` `Render::render` builds — `bg(popover)`, `border_1()`, `shadow_md()`, `rounded(6px)`, `py_0p5().px_2()` | no — a `StyleRefinement`, not an element, on a `Div` this crate never holds |
| `Tooltip::element(builder)` | `impl IntoElement`, but placed **inside** that same private box as a child | no — the chrome the reference needs is never reached |
| `ManagedTooltipExt::managed_tooltip` | wraps the **trigger**, and the content builder is `'static` | no — same 'static-vs-borrowed-`AnchorSink` problem `popover`'s `Popover::content` had, and there is no `appearance(false)`-style escape hatch to reach past it either way |

`popover` passed the same test because `Popover::appearance(false)` turns the
vendor's own box off entirely and `ParentElement::child()` accepts a `Div`
this crate built and already anchored. `gpui_component::Tooltip` has **no**
`appearance` flag and **no** `ParentElement` impl on the type whose render
paints the chrome — there is no escape hatch at all, not even the one
`popover` needed. **Verdict: built**, the same way `dropdown_menu` and
`checkbox` are.

### `tooltip.tsx` is not `popover --tooltip`

`crowbar_ui::components::popover::Variant::Tooltip` models `PopoverContent`'s
`tooltipStyle` prop (`w-fit rounded-md text-xs shadow-md/5`), reached by
`toast.tsx` and no live `PopoverContent`. Different React primitive
(`@base-ui/react`'s `Popover` against `@radix-ui/react-tooltip`'s `Tooltip`),
different class list, different tokens. Measured, not merely reasoned:
`tooltipStyle`'s popup is `rounded-md` (8px) with `shadow-md/5`;
`tooltip.tsx`'s box is `rounded-lg` (10px) with `shadow-lg`,
`border-border/70`, `bg-card/95` and `backdrop-blur-sm` — none of which
`tooltipStyle` has. **They are two surfaces**, and this file is the
independent port of the second, found real and reachable where
`tooltipStyle` is modelled-and-unreached.

## 1. Reachability — measured live, through `element.focus()`

`document.hasFocus()` is `false` on this machine and synthetic keyboard
events are denied, the same wall `popover` hit. Radix's `TooltipTrigger`
opens on focus **without the hover delay** — `data-state` reads
`instant-open` rather than `delayed-open` — so a direct `HTMLElement.focus()`
call (a DOM method, not a synthetic event) on the trigger opens the content,
read in the next `execute_js` call: `popover`'s "open in one call, read in
the next" shape, transplanted from `.click()` to `.focus()`.

Confirmed on two independent trigger shapes:

| trigger | route | measured |
|---|---|---|
| tab bar's `Close` button | `Button` with `tooltip`/`shortcut` (`button.tsx`'s inline duplicate) | `99.296875 × 32`, `data-side="bottom"` |
| breadcrumb's `a.ts` segment | `Button` with `tooltip`, no `shortcut` | `111.28125 × 32` |

Both routes paint from the identical `tooltipContentBase` constant
`tooltip.tsx` exports and `button.tsx` imports — confirmed by reading the
live `className` off both captured elements, byte for byte.

### The capture needed two corrections, neither a fabrication

A first attempt read `visible: false` on every anchor and a size 0.95 of the
one recorded here. `popover.md` §6's exact finding, on a second component:
`document.hasFocus()` is `false`, so rAF never fires and the `animate-in
fade-in-0 zoom-in-95` mount transition never resolves — the box sits mid-fade
indefinitely. Fixed the same way: pin the transition at rest
(`el.style.setProperty('animation','none','important')`, likewise
`transition`/`opacity`/`transform`) on the two anchored elements before
reading them, rather than trust an unpinned capture.

The second correction is new to this surface: Radix's `TooltipContent`
renders `children` **twice** — once into the visible, portalled box, and
again into a visually-hidden `role="tooltip"` accessibility description
(`position: absolute; width: 1px; height: 1px; clip: rect(0,0,0,0)`) nested
inside the *same* element this surface roots on. Since the shortcut chip is
part of `children`, `data-oracle-id="tooltip-shortcut"` is duplicated into
that hidden node too — a second anchor at the same id, which
`extractSnapshot` does not reject for an undeclared surface (only
`oracleSelectDeclaredAnchors` catches repeats, and this surface declares no
scope — see the next section for why it does not need to). The hidden
duplicate was removed from the DOM before extraction; nothing about the
*visible* box was touched. Recorded here because it is a new trap, not
folded silently into the reference: `clip` (the property this hidden node
uses) is not `overflow`, so none of `oracleIsVisible`'s existing ancestor
checks would have caught it, and a surface that put a `data-oracle-id` on
shared `children` of any Radix primitive using this accessibility pattern
would hit it again.

### Why no `oracleSurfaceScope` entry is needed

Every anchor `extractSnapshot` finds under this root belongs to the
primitive itself — no call site injects unrelated content the way
`popover`'s body does. `popover` and `select` needed a scope declaration
because their root's subtree contains a call site's own markup; `tooltip`'s
does not, so the un-declared, walk-everything default is already correct.

## 2. Values — the root

Every "Compiles to" was measured with `getComputedStyle` on the live,
focused element.

| React / Tailwind | Compiles to (measured) | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `rounded-lg` | `border-radius: 10px` | `tooltip::RADIUS` (`theme.radius_lg`'s own value, 10) | `radius` = 10 |
| `border border-border/70` | `border-width: 1px`, `oklab(1 0 0 / 0.042)` | `.border(BORDER_WIDTH).border_color(theme.border.mix(70.0, TRANSPARENT))` | `border.w` = 1, compared exactly |
| `bg-card/95` | `oklab(0.239 -0.000568 0.001918 / 0.95)` | `theme.card.mix(95.0, TRANSPARENT)` | `bg` |
| `text-foreground` | `oklch(0.97 0 0)` | `theme.foreground` | `fg` |
| `ui-text-sm` | `12px`, line-height `18px` | `rems(0.75)` + `relative(1.5)` | `font` |
| `whitespace-nowrap` | used width = max-content | no authored width | `content_sized` (v1.5) |
| `py-1.5 px-2.5` | `padding: 6px 10px` | `PADDING_Y`/`PADDING_X` | `bounds` |
| `flex items-center gap-2` (conditional on `shortcut`) | `gap: 8px` | `.gap(GAP)`, only when `shortcut.is_some()` | `bounds` of the chip |
| `shadow-lg` | two shadow layers | `.shadow_lg()` | **§6: no field, either side** |
| `backdrop-blur-sm` | `blur(8px)` | not painted | **§6: no field**; not worth reaching for gpui backdrop-filter support for a property the differ cannot read either way |
| `z-[99999]` | stacking | not a field | placement |
| `animate-in fade-in-0 zoom-in-95`, `data-[state=closed]:…` | the mount/exit transition | nothing | `ANCHORS.md` v1.9 — captured at rest (`instant-open`) |
| `data-[side=*]:slide-in-from-*` | placement-dependent enter direction | nothing | placement |

## 3. Values — the shortcut chip (`web/src/components/ui/keybinding.tsx`)

**Not `gpui-component`'s `Kbd`, and not `crowbar-ui`'s own `kbd.rs` either.**
Three separate keycap primitives exist in this codebase; `keybinding.tsx` is
the one `tooltip.tsx` and `button.tsx` both use for their `shortcut` prop, and
porting it as a fourth shared component was scope this item was not handed —
so it is a private helper inside `tooltip.rs`, built the same way `popover`'s
title is.

| React / Tailwind | Compiles to (measured) | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `rounded-md` | `border-radius: 8px` | `SHORTCUT_RADIUS` (`theme.radius_md`'s value) | `radius` = 8 |
| `border border-border` | `1px`, `oklch(1 0 0 / 0.06)` — **the bare token, no `/N` modifier** | `.border(BORDER_WIDTH).border_color(theme.border)` | `border.w` = 1, `border.color` compared (`w > 0`) |
| `bg-card` | `oklch(0.239 0.002 106.5)` — full alpha, unlike the root's `/95` | `theme.card` | `bg` |
| `min-h-4` | `height: 16px` — a **floor**, not the run's own line box | `SHORTCUT_MIN_HEIGHT` | `bounds.h` |
| `leading-none` | `line-height: 1` → a 12px line box, **4px short of the floor** | `relative(1.0)` | not `line_sized` (see below) |
| `px-1.5` | `padding-inline: 6px` | `SHORTCUT_PADDING_X` | `bounds.w` |
| `text-muted-foreground` | `oklch(0.72 0 0)` | `theme.muted_foreground` | `fg` |
| `ui-text-sm`, `ui-font` | `12px`, `CalSansUI` (measured; not the editor's monospace) | `rems(0.75)`, `theme.font_sans` | `font` |
| `shadow-[inset_0_-1px_0_rgba(0,0,0,0.12)]` | an **inset** box-shadow | not painted | **§6: no field** — `border` carries width/colour only, nothing shaped like a shadow |

### Declarations

* `CONTENT_SIZED = [tooltip, tooltip-shortcut]`. Both size to their own
  content: the root via `whitespace-nowrap`, the chip via its padding plus
  run, neither with an authored width.
* `LINE_SIZED = []`. The root's height is border + padding around an 18px
  line, not the line itself — not equal, so not line-sized. The chip's is a
  `min-h-4` **floor** (16px) over a `leading-none` line box of 12px — the
  same "authored height, not the line box" shape `kbd.rs` establishes for its
  own cap, confirmed independently here on a different primitive.

## 4. The frame this surface needs — none, and that is the finding's other half

Unlike `popover` and `select`, this surface's box tree is a plain `div()`
tree with no `deferred(anchored(…))` behind a captured trigger bound —
because the build verdict means there is no `gpui_component::Popover`-style
mechanism in the render path at all. It settles on the **first** frame; the
shared `on_settled_frame` signal (`crowbar-driver/src/frame.rs`) still runs
and still passes, it simply confirms immediately rather than waiting a
second draw. `native/mapping/popover.md` §4's list of widgets needing the
two-frame fix names `tooltip` among the *vendor* shapes that would need it
**if wrapped**; having been built instead, this surface never enters that
path.

## 5. What had to be overridden to reach this design

| | |
|---|---|
| The shortcut chip is a private helper, not a shared primitive | see §3 — porting `keybinding.tsx` as its own component was out of this item's scope |
| `RowSurface` draws every surface into a gpui **block** container | a content-sized box drawn straight into one is a block-level flex box, stretched to the container's width. `kbd.rs`'s exact fix — wrap the anchored element in an unanchored `div().flex().flex_row()` — applied in `surfaces/tooltip.rs`'s own `render` |
| `--content` is a shared, closed-vocabulary option | `git-status-row`'s (`short`/`normal`/`overflow`), claimed before a surface's own `accept()` runs. This surface's text option is `--text` |

**No property resisted styling.** Every field `ANCHORS.md` §3 records has a
token or a measured constant behind it on both anchors (§2, §3); the two
things that were not straightforward were capture (§4 — resolved by the
build verdict itself, unlike `popover`, which needed the settled-frame fix)
and a non-comparable field (`backdrop-blur-sm`, §6, silent on both sides).
This item does not run the oracle or the differ — the convergence verdict
itself belongs to whoever does.
