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
- **Reproducing `h-full` on two of four panels is unfalsifiable.** Mutation-
  tested: normalising the *render* to put `h_full()` on all four leaves all 19
  layout assertions green, because the declaration is inert. What the tests do
  catch is normalising the *fact* — `SidebarTab::full_height` returning `true`
  for every tab fails `h_full_is_on_the_first_two_panels_only`. So the port keeps
  the asymmetry as fidelity, not as something the gate defends.

**Mutation results, so nobody has to take the guards on trust.** Each was
applied to the component, run, and reverted: the scroll offset pinned to zero
→ **3 failures**; `min-w-full` dropped → **5**; the scrollport's overflow set to
`Visible` so it stops clipping → **2**, both of them `visible` assertions;
`full_height` true for every tab → **1**; `SidebarTab::Git.index()` off by one
→ **1**. The control run after each revert is green.

## 8. Cross-component notes added by this component

Things learned here that are **not** about `sidebar-carousel`.

| Note | |
|---|---|
| taffy honours **percentage** margins, and negative ones | resolved against the flex container's inline size, exactly as CSS does. P2.1 measured negative *absolute* block margins; this is the inline, fractional case. **Measured** — it is how the scroll offset is expressed |
| gpui reserves no scrollbar gutter | `Style::scrollbar_width` defaults to `0px` and is handed to taffy, so `Overflow::Scroll` and `Overflow::Hidden` are layout-identical in gpui as well as producing the same content mask. P2.1 asserted the second half; this is the first |
| `Styled::style()` is still the seam | second use, and the first where the two overflow axes have to take *different* values. Every `overflow_*` builder gpui exposes without an element id sets `Hidden` |
| a component with no colour takes no `Theme` | `SidebarCarousel::render` takes only the `AnchorSink`. A `Theme` argument that read no token would advertise a palette the component does not have, and the surface's `render` names its `_theme` unread so the omission is visible at the boundary |
| the content mask is pushed in `Interactivity::prepaint` | `window.with_content_mask(style.overflow_mask(bounds, rem_size))` wraps the *children's* prepaint, so an anchor records the mask its **ancestors** pushed and never its own. That is what makes `visible` mean the same thing on both sides without either extractor being told about carousels |

---

# `resizable` (P2.2)

`web/src/components/ui/resizable.tsx` →
`crates/crowbar-ui/src/components/resizable.rs`.

**This component is not its class lists**, and that is the first thing to know
about it. `resizable.tsx` is three thin wrappers over `react-resizable-panels`
4.11.2, and the library writes **inline styles** onto every element it renders.
An inline declaration beats a class, so several of the Tailwind utilities below
are dead on arrival — including one that can *never* match. `web/src/index.css`
then overrides two of the library's inline styles with `!important`, which beats
the inline declaration in turn. Reading the `cn(…)` call is not enough here; the
"Compiles to" column says which of the three layers actually wins.

Read `web/node_modules/react-resizable-panels/dist/react-resizable-panels.js`
(`Group`, `Panel`, `Separator`) alongside this table. The three inline style
objects are the real source.

## 1. Values: spacing, radius, colour

Every "Compiles to" came from running the app's own `src/index.css` through its
own `tailwindcss` 4.3.0 with the utility as a candidate, exactly as P2.1's did.
`--spacing` is Tailwind's stock `0.25rem`; `theme.css` does not redefine it.

| React / Tailwind | Compiles to | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `w-px`, `h-px` | **1px** — Tailwind's own literal, not a `--spacing` multiple | `HANDLE_THICKNESS` | compared |
| `after:w-1.5`, `after:h-1.5` | `calc(--spacing * 1.5)` = **6px** | `HIT_THICKNESS` | **invisible** (see §5) |
| `h-6` | **24px** | `GRIP_LENGTH` | compared, no reference |
| `w-1` | **4px** | `GRIP_THICKNESS` | compared, no reference |
| `rounded-lg` | `var(--radius)` = **10px**, *not* Tailwind's stock 8 | `theme.radius_lg.value()` | compared, no reference |
| `bg-border` | `var(--border)` | `theme.border` | compared, no reference |
| `hover:after:bg-border/60` | `color-mix(in oklab, var(--border) 60%, transparent)` | `theme.border.mix(60.0, TRANSPARENT)` | **invisible** |
| `focus-visible:ring-1` | `--tw-ring-shadow: 0 0 0 calc(1px + 0px)` — a **box-shadow** | a `BoxShadow`, 0 blur, `spread_radius: 1px` | **invisible** (`ANCHORS.md` §6) |
| `focus-visible:ring-ring` | `--tw-ring-color: var(--ring)` | `theme.ring` | invisible |
| `ring-offset-background` | `--tw-ring-offset-color: var(--background)` | **nothing** — `--tw-ring-offset-width` stays at its `0px` initial, so the offset layer keeps its `0 0 #0000` initial and paints nothing | absent |
| `z-10` | `z-index: 10` | **nothing** — `ANCHORS.md` §6 excludes paint order as a field on purpose | absent |
| `left-1/2` on the `::after` | `left: calc(1/2 * 100%)` of the **host** | resolved by hand: `HIT_LEFT` | invisible |
| `-translate-x-1/2` | `translate: calc(-50%)` of the **element** | resolved by hand; **gpui has no `translate`** | invisible |

## 2. Layout constructs

The two useful rows here are the ones no earlier component had: a **flex
division** and a **flex item that neither grows nor shrinks**.

| React / Tailwind / inline | Compiles to | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `Group`'s inline `display:flex; flexDirection: row\|column; flexWrap: nowrap` | the layout, unconditionally | `.flex().flex_row()`/`.flex_col()`, `.flex_nowrap()` | compared |
| `Group`'s `className="flex h-full w-full"` | the same three things the inline style already says | rendered once — **the classes are dead but harmless** | compared |
| `Panel`'s inline `flexBasis: 0; flexShrink: 1; flexGrow: <layout>` | each panel's width is `grow/Σgrow × free` | `.flex_basis(px(0.)).flex_shrink(1.).flex_grow(g)` | compared |
| `Panel`'s inline `minWidth: 0; minHeight: 0` | defeats CSS's content-based automatic minimum size | `.min_w(px(0.)).min_h(px(0.))` — **load-bearing**; taffy defaults to `auto` exactly as CSS does | compared |
| `Panel`'s inline `maxWidth: 100%; maxHeight: 100%` | never binds here, and is reproduced anyway | `.max_w(relative(1.)).max_h(relative(1.))` | compared |
| `Separator`'s inline `flexGrow: 0; flexShrink: 0; flexBasis: auto` | one pixel at every width | `.flex_grow(0.).flex_shrink(0.).flex_basis(Length::Auto)` | compared |
| no `align-items` on the group | CSS's `stretch` | gpui's `AlignItems` default is `Stretch` too — **measured**, see `the_panels_and_the_separator_tile_the_group_exactly` | compared |
| `height: 100%` on the group | resolves against `SidebarProvider`'s `h-screen` | `ResizablePanelGroup::shell_height`, a **parameter**, rendered as a definite-height box **above** the root anchor and therefore outside the snapshot | absent by construction |
| the `::after` | `position: absolute` out of flow | a `.absolute()` child; takes no room in the flex line either way | invisible |

**The `flex-grow` numbers are a parameter, not a property.** `Group` writes
`style={{ flexGrow: layout[panelId] }}` from its own layout engine, and the
numbers it writes are **percentages summing to 100**. Reproducing that engine
would be reproducing the wrong thing — what a parity run compares is whether two
flex implementations divide the same free space the same way. Same call P2.1
made for `--anchor-width`.

## 3. No gpui equivalent

| React / Tailwind | Why | What the port does |
|---|---|---|
| `translate`, on the `::after` | gpui's `Style` carries no transform at all | **resolved by hand.** Both percentages are of lengths this component authors (the 1px host, the 6px strip), so `left: 50%` + `-translate-x-1/2` is exactly `−2.5px` and there is no engine-dependent value in it |
| `[&[aria-orientation=horizontal]>div]:rotate-90` | same — no transform | **absent.** It only applies to a vertical group *with* `withHandle`, and neither exists in the app, so the cell it would change does not exist on either side |
| `after:transition-colors` | a transition | **absent.** `ANCHORS.md` §6: a snapshot is one instant |
| `focus-visible:outline-hidden` | `outline-style: none` | **absent.** The contract has no outline field, and gpui paints no outline to suppress |
| `touchAction` (inline, all three elements) | pointer routing | **absent.** Not a visual property |
| `pointerEvents: "none"` on panels during a drag | hit testing | **absent.** Not a visual property, and drag state is the library's |
| `cursor` (inline, only when disabled) | `not-allowed` | **absent.** No live call site disables the separator |

## 4. Anchoring

| Construct | Decision |
|---|---|
| ids in the primitive | `resize-group`, `resize-panel`, `resize-handle`, `resize-handle-grip`, each written *before* `{...props}` so a call site can override it — P2.1's convention |
| a group with several panels | the call site names them, as a menu with several rows does. `ide-shell.tsx` names its two `resize-panel-sidebar` / `resize-panel-content`, so the primitive's own `resize-panel` **never appears in the live app** |
| which of a panel's two divs | the **outer** one. `data-slot` and any spread `data-*` land there; the library puts a call site's `className` on the *inner* div, which nothing outside the library can put an attribute on |
| the `::after` | **not anchored.** See the traps |
| `CONTENT_SIZED` / `LINE_SIZED` | **both empty**, and empty *lists* rather than absent, for P2.1's reason. Here it follows from one fact: nothing on this surface paints text |

## 5. Traps

Each of these compiles, renders something plausible, and is wrong.

| Trap | What actually happens |
|---|---|
| **The separator's `aria-orientation` is the OPPOSITE of the group's.** | `Separator` computes `orientation === "horizontal" ? "vertical" : "horizontal"`, and **every** `aria-[orientation=…]` variant in `resizable.tsx` is written on the separator. So `aria-[orientation=horizontal]:w-full` is the rule a **vertical group** gets. A port that matched the words up gives a horizontal group a full-width separator and collapses the layout to nothing. This is the single most likely mistake in the file |
| **`aria-[orientation=vertical]:flex-col` on the group can never match.** | `Group` renders **no `aria-orientation` attribute at all**. The class is dead; the inline `flexDirection` is what stacks the panels. A port that keyed the direction off that class would render every group as a row |
| **`flex h-full w-full` on the group is also dead** — but harmlessly. | The library's inline style already says `display:flex; height:100%; width:100%`, and inline beats a class. Same picture, different reason: a reader who edits the class expecting the layout to move will be surprised |
| **The group does not clip.** | The library writes `overflow: hidden` inline, and `index.css` overrides it with `[data-slot='resizable-panel-group'] { overflow: visible !important }` so a pane's shadow can bleed into the sidebar. `!important` beats an inline declaration. Same for the panel's **inner** div, whose `overflow: auto` is overridden by `[data-slot='resizable-panel'] > div`. A port that clipped would report a different `visible` on any overflowing child |
| **A `ResizablePanel` is two divs, not one.** | The outer carries the flex sizing and the `data-*`; the inner carries the call site's `className`, `flexGrow: 1` and the panel's children. `className="min-w-0"` in `ide-shell.tsx` therefore lands on the **inner** div, not on the flex item it looks like it is modifying — and the outer already has `min-width: 0` inline anyway |
| **`ring-1` is not a border** *(P2.1's trap, in a second place)*. | Tailwind 4 compiles it to a box-shadow. The separator's `border.w` is **zero in every cell**, and that is the one field `ANCHORS.md` v1.1 compares exactly |
| **The `::after` is not `inset: 0`, so it cannot be a pseudo-backed anchor.** | `ANCHORS.md` §3 permits the shortcut *only* while the pseudo is `position:absolute; inset:0`, because the extractor then synthesises bounds from the **host's padding box**. This pseudo is `left:50%; width:6px` with a translate; taking the shortcut anyway would have both sides agree on a 1px-wide box that is really 6px wide and 2.5px to the left. So it is painted and **deliberately unanchored** |
| **The hit strip is centred in one orientation and half a pixel out in the other.** | A vertical separator gets `left-1/2` (of the host) *and* `-translate-x-1/2` (of itself) → centred. A horizontal one gets `after:left-0` and `-translate-y-1/2` with **no `top-1/2`** to pair with it, so it is centred on the separator's top edge instead. Reproduced rather than corrected — see `HIT_TOP` |
| **taffy rounds a panel's width to a whole logical pixel; `WebKit` does not.** | Measured: 600px surface, the live factors, CSS gives **146.8748** and taffy lays out **147**. It is a *round*, so it is bounded by 0.5 and §5's tolerance already covers it — unlike v1.5's one-directional `ceil`, which needed a correction. **But it is the one place on this surface where ±0.5 is genuinely tight:** a share landing on a half pixel is Δ 0.5 exactly and reads as a defect. Pick `--width`/`--grow` so the arithmetic lands near a whole pixel |
| **Assuming a `withHandle` grip exists.** | No live call site passes it. `ide-shell.tsx` writes `<ResizableHandle key="handle" data-testid="sidebar-resize-handle" />` and it is the only `ResizableHandle` in the app |

## 6. Painted but invisible to the oracle

`ANCHORS.md` has no field for any of these. They are painted for fidelity and
the differ will never confirm or deny them — **say so in any report**.

| React / Tailwind | gpui | Note |
|---|---|---|
| the `::after` hit strip, at rest and on hover | a `.absolute()` child, `bg` only when hovered | no DOM node, and §3's pseudo shortcut is invalid here. **This is the whole of the component's hover state.** Unanchored means untestable through the oracle, so its layout was measured once by hand — temporary anchor, read back, removed: horizontal `x 75.5, 6×160` against a separator at `x 78, 1×160`; vertical `y 57, 600×6` against `y 60, 600×1`. The numbers are in `hit_strip`'s doc comment, because no gate would notice if they stopped being true |
| `focus-visible:ring-1 ring-ring` | a `BoxShadow` inserted via `Styled::style` | §6: shadows are not representable. **This is the whole of the component's focus state** |
| `z-10` on the grip | nothing | §6 excludes paint order |

## 7. What this surface cannot show the differ

Recorded because §8.2 requires honesty about it, and because this surface has
more of it than any before — **three of §8.3's four axes are vacuous here**.

- **`hover` cannot fail.** Its only rule paints the `::after`. Measured, not
  argued: `neither_interaction_flag_moves_a_field_the_differ_can_see` asserts
  every anchored record is identical to the resting one.
- **`focus` cannot fail.** Its only rule paints a ring, which is a box-shadow.
  Same test.
- **Neither is declared `unmodelled`.** That field means "this surface's React
  original has no such state", and both of these originals exist. What is
  missing is a *field in the contract*, which is a different fact — so it is
  said per cell, in the caption, where `dropdown-menu` says the same kind of
  thing about `selected` without `--tick`.
- **`--content` cannot fail, on every cell.** Nothing here paints a character,
  so the three content lengths render three identical pictures. The caption says
  so unconditionally.
- **`--theme` cannot fail without `--with-handle`.** `bg-border` on the grip is
  the only colour on the component; every other anchor is `bg: #00000000`,
  `radius: 0`, `border.w: 0` in both themes. Measured by
  `without_the_grip_the_surface_has_no_colour_at_all`.
- **The grip has no reference at all** — no call site passes `withHandle` — so
  the one cell that would make `--theme` real is itself unreachable. The same
  shape of finding as Phase 1's `git-row-dir`.
- **A vertical group has no reference.** The app has exactly one
  `ResizablePanelGroup` and it is `orientation="horizontal"`. Rendered, and named
  as unreachable in the caption.
- **The panel's inner div is unanchorable.** It is the library's own element:
  nothing in `resizable.tsx` or at a call site can put an attribute on it.
  Rendered anyway, because it is what a panel's content lays out inside.
- **`empty` is unmodelled.** `ide-shell.tsx` renders its three children
  unconditionally; a group with no panels is not a state the app has.

**What is left is worth having, and it is the width axis.** `flex-basis: 0` plus
fractional `flex-grow` around a fixed 1px sibling is one division that taffy and
`WebKit` each perform and round independently, and it is the arithmetic the whole
IDE shell's layout rests on. The collapsed sidebar (`collapsible`,
`collapsedSize={0}`) is a real reference state and is `--grow 0,100`, on which
both extractors agree a zero-area box is `visible: false`.

---

## Cross-component notes (P2.2)

Things learned here that are **not** about `resizable`.

| Note | |
|---|---|
| A wrapped third-party primitive's inline styles outrank its class list | and `!important` in `index.css` outranks both. For anything over a library — `react-resizable-panels`, `base-ui`, Plate — read the library's rendered `style` object before the `cn(…)` call. `dropdown-menu` happened not to need this; most of the remaining 44 primitives will |
| gpui's `AlignItems` default is `Stretch`, like CSS's | so a flex container that sets no `align-items` ports as nothing at all. **Measured** |
| gpui has no transform | `rotate-*`, `translate-*`, `scale-*` are unportable. A translate by a percentage of lengths the component authors can be resolved by hand exactly; one against a runtime-measured length cannot |
| taffy rounds layout to whole logical pixels | which is DPR-independent and bounded by 0.5, so §5's tolerance covers it — but it is *tight* on any box whose used size is a fraction. Worth choosing matrix widths that avoid half-pixel shares |
| `ANCHORS.md` §3's pseudo-backed shortcut is narrower than it reads | it is valid only for `inset: 0`, and there is no mechanism for anything else. A pseudo positioned any other way is simply not anchorable today |

---

# `native-menu` (P2.14)

`web/src/components/ui/dropdown-menu.tsx` → **`AppKit`**, via
`crates/crowbar-platform/src/native_menu.rs`.

**This entry is the shape of an exception, and that is why it is here.** Every
other section above is a Tailwind construct becoming a gpui one. This one is a
component leaving the table: the user's ruling is that Crowbar's dropdown menus
are native rather than simulated, so a *context* menu is now an `NSMenu` and
there is no class list to compile, no token to read and nothing for the differ to
compare.

`crowbar-ui`'s `dropdown_menu` (P2.1) **stays**. It is the right answer for a
menu that must carry Crowbar's tokens or live inside a pane — an `NSMenu` takes
the system's appearance, not `theme.css`'s. What moved is the *context* menu.

## 1. What maps

| `dropdown-menu.tsx` | `AppKit` | Oracle |
|---|---|---|
| `menu-item` | `NSMenuItem` with a target and an action | **absent.** No anchor can reach an OS-drawn window |
| `menu-separator` | `+[NSMenuItem separatorItem]` | absent |
| `menu-checkbox-item` | `-setState:NSControlStateValueOn` | absent |
| `data-disabled` | `-setEnabled:NO`, **and no action at all** | absent |
| `menu-sub-trigger` + `menu-sub-popup` | `-setSubmenu:` | absent |
| `menu-popup`'s `w-(--anchor-width)` / `min-w-*` | **gone.** `AppKit` sizes a menu to its rows | absent |

## 2. What does not map, and what happens to it

| `dropdown-menu.tsx` | Why | What it becomes |
|---|---|---|
| `menu-radio-item` | `AppKit` has no radio primitive; a radio group is a set of ticks the *application* keeps exclusive | an item with `checked`, exclusivity owned by the caller |
| `menu-label` | a section header is not a `NSMenu` concept | a disabled item |
| `menu-shortcut` | a key equivalent belongs to the responder chain, not to the row's text | not ported |
| `inset` | `AppKit` lays out its own tick gutter | not ported |
| `focus:bg-accent` | which row is highlighted is decided inside the tracking loop | **`AppKit`'s**, and deliberately unreachable |

## 3. Traps

| Trap | |
|---|---|
| **A queued `dispatch_async` cannot close a menu that a queued block opened.** | `GPUI`'s foreground executor schedules onto the **main dispatch queue**, so a menu shown from a `GPUI` task is shown from inside a main-queue block — and `libdispatch` will not begin another main-queue block while one is on the stack, however long the nested run loop spins. Verified by sampling: the main thread sits in `_dispatch_main_queue_drain → … → popUpMenuPositioningItem:` and the queued cancel never arrives. Use a run-loop timer in `NSRunLoopCommonModes` (`cancel_tracking_after`), which is not on that queue |
| **`-[NSMenuItem setTarget:]` stores the target weakly.** | So the target must be kept alive across the whole `popUpMenuPositioningItem:` call. It is: `show` holds the `Retained<MenuTarget>` on its own stack frame for the duration |
| **A misspelled action selector compiles and does nothing.** | `AppKit` never fires an action its target does not answer, so every row is silently inert. Ask the runtime instead — `AnyClass::responds_to` — which is what `the_target_answers_the_selector_the_rows_are_pointed_at` does |
| **A misspelled *timer* selector is worse.** | It raises `NSInvalidArgumentException` inside the run loop, in a frame no Rust code appears in |
| **`Default` on a selection type is a trap.** | Zero is a valid `NSMenuItem` tag, so a derived `Default` reports the menu's first row as chosen by a user who never opened it. The sentinel has to be negative and written out |
| **`popUpMenuPositioningItem:atLocation:inView:` blocks.** | It runs its own event-tracking loop and returns when tracking ends. Calling it from inside a `GPUI` event handler holds the app borrowed for as long as a user holds the menu open |
| **`AppKit` screen coordinates are y-**up**, from the bottom of the primary display.** | GPUI's global space is y-down from the top. The flip is `ScreenPoint::from_top_left`, and it lives in `crowbar-platform` because the convention is a fact about `AppKit` |

## 4. What this surface cannot show the differ

**Everything.** That is the decision, not a gap in it. `--surface native-menu`
has no anchored-geometry gate: its root anchor is the *plate* the menu is opened
from, and the popup itself is an OS window no extractor can see. The surface's
own module docs carry the 16-line judgement checklist that replaces the differ,
and one of those lines — `--open launch --dismiss-after` — is the only one that
runs without a human, because synthetic pointer and keyboard events are denied
on this project's machines.
