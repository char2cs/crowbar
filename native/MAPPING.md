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

---

# `sidebar-carousel` (P2.3)

`web/src/components/layout/sidebar-carousel.tsx` →
`crates/crowbar-ui/src/components/sidebar_carousel.rs`.

> Appended **after** P2.1's cross-component notes rather than before them, to
> keep this file strictly append-only. The notes this component adds are in its
> own §8 at the end.

Nothing overrides these classes from a stylesheet — `data-sidebar-carousel=""`
carries no rule anywhere in the app and is a query hook for the component's own
Vitest file — so the class lists are what renders. Every value below came out of
the app's own `tailwindcss` 4.3.0, same method as `dropdown-menu`.

**This is the first component in the port that paints nothing.** No colour, no
type, no radius, no border, no spacing: twelve classes, all of them layout. What
it has instead is a **clip**, and the clip is the whole contract surface.

## 1. Values

There are none. There is no `--spacing` multiple, no token, no `Theme` argument
on `SidebarCarousel::render` — which is a statement rather than an omission, and
`nothing_on_this_surface_paints` is the assertion that keeps it true.

| React / Tailwind | Compiles to | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `flex` | `display: flex` | `.flex()`. gpui's `Style::default()` already has `flex_direction: Row`, so a row needs no `.flex_row()` | compared (through geometry) |
| `flex-1` | **`flex: 1`** — the *shorthand*, which expands to grow 1 / shrink 1 / **basis 0%**, not `flex-grow: 1` alone | `.flex_1()`, which sets exactly those three (`styled.rs`: `flex_grow 1`, `flex_shrink 1`, `flex_basis relative(0.)`) | compared |
| `flex-col` | `flex-direction: column` | `.flex_col()` | compared |
| `min-w-full` | `min-width: 100%` | `.min_w_full()` — `relative(1.)`, a taffy `Percent`, resolved against the flex container's inline size exactly as CSS does | compared |
| `h-full` | `height: 100%` | `.h_full()` | compared |
| `overflow-hidden` (panels) | `overflow: hidden` | `.overflow_hidden()` | compared via `visible` |
| `overflow-x-scroll` | `overflow-x: scroll` | `style().overflow.x = Some(Overflow::Scroll)` through the `Styled::style()` seam — see the traps | compared via `visible` |
| `overflow-y-hidden` | `overflow-y: hidden` | `style().overflow.y = Some(Overflow::Hidden)` | compared via `visible` |
| `[scrollbar-width:none]` | `scrollbar-width: none` | **nothing to write.** gpui's `Style::scrollbar_width` defaults to `AbsoluteLength::Pixels(0)` and is passed to taffy, so `Overflow::Scroll` reserves no gutter — which is what the class asks `WebKit` for | invisible |
| `[&::-webkit-scrollbar]:hidden` | a nested `&::-webkit-scrollbar { display: none }` — the `hidden` **utility**, i.e. `display: none`, *not* `visibility: hidden` | nothing; gpui paints no scrollbar | invisible |
| `[scroll-snap-type:x_mandatory]` | `scroll-snap-type: x mandatory` | **absent** — §3 | absent |
| `[scroll-snap-align:start]` | `scroll-snap-align: start` | **absent** — §3 | absent |
| `data-sidebar-carousel=""` | no rule; a test hook | nothing | invisible |

## 2. Layout constructs

| React / Tailwind | Compiles to | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `el.scrollLeft = index * el.clientWidth`, written in two `useEffect`s | a **DOM property**, not a declaration — there is no class and no element it lives on | a **negative percentage margin on the first snap child**: `.ml(relative(-index))`. A margin percentage resolves against the flex container's inline size, which is the same box `clientWidth` measures, so the translation carries no width and cannot drift from `--width`. In a flex row a negative `margin-left` on item 0 shifts item 0 **and every sibling after it** by the same amount — which is precisely what a scroll offset does. **Measured**, `every_tab_snaps_the_track_by_one_scrollport_per_index` | compared |
| four `min-w-full` items in a one-item-wide row | Σ min-width is 4 scrollports against 1 of space | flexbox freezes every item at its minimum, so each panel is **exactly** the scrollport's width whatever its flex base size was. This is what licenses rendering the panels empty, and it is driven rather than argued — `--panel-content 4000` produces a byte-identical record set | compared |
| a panel's own `overflow-hidden` | clips the panel's *children* | does **not** clip the panel's own anchor, on either side, for two different mechanical reasons that happen to agree: `oracleIsVisible` starts its walk at `el.parentElement`, and `AnchoredBox::prepaint` reads `window.content_mask()` **before** prepainting its child | compared |
| `flex-1` with no `height` anywhere on the component | the block size comes entirely from the parent | the host column belongs to the **surface**, not to `crowbar-ui`: it is `NavStack`'s `flex h-full flex-col`, and `nav-stack.tsx` is a different component. `--height` drives it, for the same reason `dropdown-menu`'s `--anchor-width` is an option — the reference's carousel is as tall as the sidebar it is in | compared |
| `align-items` unset on the row | `stretch` | gpui's `Style::default()` leaves `align_items: None` and taffy's flex default is `Stretch`, so the two panels *without* `h-full` come out the same height as the two with it. **Measured**, `h_full_and_the_default_stretch_give_the_same_height` | compared |

## 3. No gpui equivalent

| React / Tailwind | Why | What the port does |
|---|---|---|
| `scroll-snap-type: x mandatory`, `scroll-snap-align: start` | gpui has **no scroll snapping at all** | **absent, and here that costs nothing.** `sidebar-carousel.tsx` computes its own snapped positions — `el.scrollLeft = index * el.clientWidth` in the `ResizeObserver` effect and `el.scrollTo({left: index * el.clientWidth})` in the `activeTab` effect — so the positions the port reproduces are the *component's* arithmetic, not the engine's. What is unported is the settle after a swipe that ended between panels, and `ANCHORS.md` §6 already puts "a snapshot is one instant" outside the contract |
| the `ResizeObserver` re-align, `onScroll → setActiveTab`, the `isUserGesture` ref | behaviour, not appearance | **absent.** The port renders one scroll position; it does not own one. The React comments record a real defect in that logic (a collapse/expand cycle silently changing tabs) and none of it is visible to a snapshot |
| `WorkspaceTree`, `AgentChatsPanel`, `FileExplorerTree`, `GitPanel` | four whole feature subtrees | **empty boxes** — the same call `git_status_row`, `file_tree_row` and `dropdown_menu` made about icons. Unusually, this one is *proved* rather than asserted: see `--panel-content` and §2 |
| `<Suspense fallback={<SidebarSkeleton />}>`, `<ErrorBoundary>` | the `loading` and `error` states the component really has | **absent**, and the cells are declared unmodelled: both swap content *inside* a panel whose box is floored by `min-w-full`, so no anchored box can move |

## 4. Painted but invisible to the oracle

**Nothing.** There is no shadow, no ring, no letter-spacing, no opacity on this
component. Every anchor reports `bg: #00000000`, `radius: 0.0` and
`border.w: 0.0` — which is a *comparison* the differ makes, not an absence it
cannot see. `border.w` is the field `ANCHORS.md` v1.1 compares **exactly**, and
zero on both sides is the strongest form of agreement it can express.

## 5. Anchoring

| Construct | Decision |
|---|---|
| the root | `carousel-scrollport`, the scroll container. It **has** to be the root: §4 puts the origin on it, and a snapped-out panel's `x` is only meaningful relative to the box it is clipped against |
| the four panels | `carousel-panel-workspaces` / `-chats` / `-files` / `-git`. Named for their tab rather than indexed, although `TABS` would supply an index: an id has to stay attached to the same panel across a reorder, and an indexed id would silently repoint |
| `visible` on a snapped-out panel | **`false`.** Five-point argument in the component's module docs. Short form: §3 defines the field as "not fully clipped by an ancestor"; both extractors already compute exactly that with no change to either; the alternative reading turns the field into a claim about *reachability* and reports `true` on all four panels in every cell forever; and it gives up nothing because `bounds` still carries the geometry at ±0.5px |
| `CONTENT_SIZED` / `LINE_SIZED` | both **empty**, and on this surface not even a judgement call — v1.5 is about a text run's max-content width and v1.6 about a line box, and nothing here paints text |
| ids in the primitive | written directly on the five `<div>`s. There is one carousel in the app and it takes no children from a call site, so unlike `dropdown-menu` there is no per-slot default to override |

## 6. Traps

Each of these compiles, renders something plausible, and is wrong.

| Trap | What actually happens |
|---|---|
| **Reaching for gpui's `overflow_x_scroll()`.** | It lives on `StatefulInteractiveElement`, so it drags in an element id **and** a `ScrollHandle` — runtime state gpui re-clamps every frame from measured content size, and which `ANCHORS.md` §6 says a snapshot reads *around*. A surface whose defining quantity lived there could not be pinned by a cell. Everything the layout and the content mask need is the *style*, and `Styled::style()` reaches it — P2.1's seam, second use |
| **Anchoring a panel and expecting its own `overflow-hidden` to clip it.** | It clips the panel's children, not the panel. Both extractors agree, by two different mechanisms (see §2), and a port that pushed its own mask before recording would report `visible: false` for the panel that *is* showing |
| **Treating "the adjacent panel" as a borderline `visible`.** | It is not borderline. At `scrollLeft = k·W` the next panel's left edge **is** the scrollport's right edge and the overlap is exactly zero; the DOM side requires `r - l > 0` and the driver requires a non-empty intersection, so both say `false` without a tolerance being involved. `--active-tab files` is the default here precisely so that the sharp case is in the default cell |
| **Modelling the scroll offset as a transform on the container.** | It would move the **root anchor**, and `ANCHORS.md` v1.1 §4 makes a root anywhere but the origin a load error. The offset has to land *inside* the root, which is why it is a margin on the first child rather than anything on the scrollport |
| **`flex-1` is `flex: 1`, and `flex: 1` sets `flex-basis: 0%`.** | Compiled and read, not assumed. A port that translated it as `flex_grow_1()` would leave `flex-basis: auto`, and a content-sized basis in a column of one item happens to give the same answer here — so the mistake would be invisible on this surface and wrong on the next one |
| **Assuming a `--theme` or `--content` cell can fail here.** | Two of §8.3's four axes cannot move an anchor on this surface: no colour means `--theme` selects a token table nothing reads, no text means the three content lengths are one picture. `the_theme_and_content_axes_move_nothing` pins that as a measurement so that a future background would fail loudly instead of quietly converging |
| **Assuming the `h-full` on two of four panels means something.** | It is inert — `align-items: stretch` already fills the row's height and `height: 100%` resolves against the same box. Reproduced anyway, because a port that "tidied" the four to match would be hiding a difference rather than showing that there is none |
| **Letting the panels' real contents decide the panel width.** | They cannot. All four are `min-w-full`, so the track always overflows and every item freezes at its minimum. Without that, rendering the four subtrees as empty boxes would be unsound — and it is the single largest assumption in this port, which is why `--panel-content` exists to drive it rather than a comment to assert it |

## 7. What this surface cannot show the differ

Recorded because §8.2 requires honesty about it.

- **Two of the four §8.3 axes are vacuous.** `theme` and `content` cannot move
  an anchor here, for the reason in the traps. The `width` axis does real work —
  it scales the scrollport, every panel and the whole track.
- **Five of the six state flags are unmodelled**, and the sixth is a *reading*.
  `selected` is mapped onto "the carousel has scrolled to `--active-tab`",
  because the fixed six-word vocabulary has no word for "which panel is
  showing". Declaring all six unmodelled would be the honest answer and
  `surface.rs`'s `no_surface_declares_its_entire_state_axis_unmodelled` forbids
  it. Reported rather than resolved quietly.
- **A capture precondition, not a port property.** `oracleIsVisible` walks
  ancestors all the way to `<html>`. In the live app the carousel additionally
  sits inside `NavStack`'s `overflow-hidden` root and inside a layer that becomes
  `opacity-0 -translate-x-1/4` whenever a nav screen is pushed — so a reference
  capture taken with the sidebar collapsed, or with a screen pushed, reports
  `visible: false` on **every** anchor while the native side reports the truth.
  The native surface has no such ancestor and cannot reproduce that.
- **The snap gesture is unreachable.** `hover` and `focus` were already
  `blocked/` on a locked screen; a *swipe* is worse, and the port has nothing to
  compare it against anyway (§3).

## 8. Cross-component notes added by this component

Things learned here that are **not** about `sidebar-carousel`.

| Note | |
|---|---|
| taffy honours **percentage** margins, and negative ones | resolved against the flex container's inline size, exactly as CSS does. P2.1 measured negative *absolute* block margins; this is the inline, fractional case. **Measured** — it is how the scroll offset is expressed |
| gpui reserves no scrollbar gutter | `Style::scrollbar_width` defaults to `0px` and is handed to taffy, so `Overflow::Scroll` and `Overflow::Hidden` are layout-identical in gpui as well as producing the same content mask. P2.1 asserted the second half; this is the first |
| `Styled::style()` is still the seam | second use, and the first where the two overflow axes have to take *different* values. Every `overflow_*` builder gpui exposes without an element id sets `Hidden` |
| a component with no colour takes no `Theme` | `SidebarCarousel::render` takes only the `AnchorSink`. A `Theme` argument that read no token would advertise a palette the component does not have, and the surface's `render` names its `_theme` unread so the omission is visible at the boundary |
| the content mask is pushed in `Interactivity::prepaint` | `window.with_content_mask(style.overflow_mask(bounds, rem_size))` wraps the *children's* prepaint, so an anchor records the mask its **ancestors** pushed and never its own. That is what makes `visible` mean the same thing on both sides without either extractor being told about carousels |
