# `tabs` (P3.2)

`web/src/components/ui/tabs.tsx` → `crates/crowbar-ui/src/components/tabs.rs`,
`crates/crowbar-app/src/surfaces/tabs.rs`.

> **This file is a §6.2 section that does not live in `native/MAPPING.md`.**
> Parallel workers writing one file conflict; the "How to read a row" preamble,
> the column meanings and the append-only rule are `MAPPING.md`'s and apply
> here unchanged. **Read the Traps section of every component before porting a
> new one** — including this one.

**Compile the CSS, do not read the class name.** Every "Compiles to" below came
from running the app's own `src/index.css` through its own `tailwindcss` 4.3.0
with the utility as a candidate. Two of the numbers are not Tailwind's stock
values, because `theme.css` redefines them.

The primitive is a set of class lists over `@base-ui/react` 1.6.0's `Tabs`.
Nothing overrides these classes from a stylesheet — unlike the two Phase 1 rows
— but **both live call sites override them through `cn(…)`**, and
`tailwind-merge` drops the primitive's own utilities against them. Section 8 is
about that, and it is where most of this component's surprises are.

The file also exports a **second, unrelated component**: a plain `Tab` button
used by `features/tabs/components/tab-bar-item.tsx` — the editor's tab strip, not
a base-ui tab. It shares a filename and nothing else: no `data-slot`, no
`Tabs.Root` context, its own `isActive` prop. It is **absent** from this port and
carries no `data-oracle-id`; it is its own §6.2 row for whoever ports the editor
tab strip. Worth one line anyway, because its `isActive` branch reproduces the
indicator's paint by hand — `rounded-full border-background bg-background
shadow-xs shadow-black/10 inset-shadow-[0_1px_…]` — so the two will need the same
answer about shadows.

---

## 1. Values: spacing, type, radius, colour

| React / Tailwind | Compiles to | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `gap-2` (root) | `calc(var(--spacing) * 2)` = **8px** | `ROOT_GAP`, every multiple derived from one `SPACING = 4.0` | compared |
| `gap-x-0.5` (list) | `column-gap: calc(--spacing * 0.5)` = **2px** | `Orientation::list_gap()` — see the traps | compared |
| `p-0.5` (default list) | **2px** | `LIST_PADDING` | compared |
| `py-1` / `px-1` (underline list) | **4px** | `UNDERLINE_LIST_PADDING` | compared |
| `h-9` / `sm:h-8` (tab) | **36 / 32px** | `TAB_HEIGHT_BASE` / `TAB_HEIGHT_SM`, selected by `Breakpoint` | compared |
| `px-[calc(--spacing(2.5)-1px)]` (tab) | `calc(calc(--spacing * 2.5) - 1px)` = **9px** | `TAB_PADDING_X`, written as the arithmetic | compared |
| `border` (tab, indicator) | **1px** | `TAB_BORDER` | compared, **exactly** |
| `gap-1.5` (tab) | **6px** | `TAB_GAP` | compared |
| `size-4.5` / `sm:size-4` (icon) | **18 / 16px** | `ICON_SIZE_BASE` / `ICON_SIZE_SM` on an **empty** div | invisible (unanchored) |
| `-mx-0.5` (icon) | **−2px** | `ICON_MARGIN_X` | invisible (unanchored) |
| `h-0.5` / `w-0.5` (underline) | **2px** | `UNDERLINE_THICKNESS` | compared |
| `translate-y-px` / `-translate-x-px` (underline) | **1px** | `UNDERLINE_NUDGE`, resolved by hand — gpui has no `translate` | compared |
| `ring-2` (tab, focus) | box-shadow spread **2px** | `RING_WIDTH` | invisible (§6: no shadow field) |
| `rounded-lg` (list, indicator) | `var(--radius)` = **10px**, *not* Tailwind's stock 8 | `theme.radius_lg.value()` | compared |
| `rounded-md` (tab) | `calc(var(--radius) * 0.8)` = **8px**, *not* stock 6 | `theme.radius_md.value()` | compared |
| `text-base` / `sm:text-sm` (tab) | `1rem` on `calc(1.5/1)` / `0.875rem` on `calc(1.25/0.875)` → **16/24 and 14/20** | `theme.ui_text_lg` / `theme.ui_text_base` | invisible (nothing paints text) |
| `font-medium` | `--font-weight-medium: 500` | `FontWeight::MEDIUM` | invisible (nothing paints text) |
| `bg-muted` (default list) | `var(--muted)` | `theme.muted` | compared |
| `bg-sidebar-element-idle` (call site) | `var(--sidebar-element-idle)` | `theme.color_sidebar_element_idle` | compared |
| `bg-background` + `border-background` (indicator) | `var(--background)` | `theme.background` | compared |
| `bg-primary` (underline) | `var(--primary)` | `theme.primary` | compared |
| `border-transparent` (tab) | `transparent` | `Color::TRANSPARENT` | compared (`w > 0`, so the colour is too) |
| `text-muted-foreground/72` · `text-foreground/70` | `color-mix(in oklab, … N%, transparent)` | `.mix(N, Color::TRANSPARENT)` | invisible (no anchor paints text) |
| `data-disabled:opacity-64` | `opacity: 64%` | **absent** | invisible — no opacity field, and 0.64 ≠ 0 so v1.7's `visible` term does not fire either |

**The `--ui-text-*` trade, again.** Tailwind's `text-base` is 1rem and
`--ui-text-lg` is the same 1rem; `text-sm` is 0.875rem and `--ui-text-base` is
the same 0.875rem. There is no token for Tailwind's own scale and inventing one
would put a value in the design system the design system does not have — the same
trade `dropdown_menu` records one step down the scale.

## 2. Layout constructs

| React / Tailwind | Compiles to | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `flex-1` on a tab | `flex: 1` → `1 1 0%` | `.flex_grow(1).flex_shrink(1).flex_basis(relative(0.0))` | compared |
| `shrink-0 grow` on a tab | `flex-shrink: 0; flex-grow: 1`, basis `auto` | `.flex_grow(1).flex_shrink(0).flex_basis(Length::Auto)` | compared, **no reference** |
| `absolute bottom-0 left-0` + four `--active-tab-*` | a measured box | an abspos child of the **active tab**, inset `-1px` — see §5 and §6 | compared |
| an abspos child's inset | resolved against the containing block's **padding** box | taffy does the same — **measured**, `row_layout::tabs::the_indicator_is_exactly_the_active_tabs_border_box` | compared |
| `data-[orientation=vertical]:flex-col` | column list | `.flex_col()` | compared, no reference |
| `w-fit` on the list | `width: fit-content` | **absent** — see §3 | — |
| an SVG with `width="14"` under `sm:[&_svg…]:size-4` | **16px**: a presentational attribute has no specificity | one empty 16px box | invisible (unanchored) |

## 3. No gpui equivalent

| React / Tailwind | Why | What the port does |
|---|---|---|
| `w-fit` on `TabsList` | taffy has no `fit-content`; the nearest expression is an `align-self` the class list does not carry | **absent**, and **neither live call site renders it** — for two different reasons, both measured. `sidebar-tab-bar.tsx` writes `w-full`, which `tailwind-merge` substitutes outright. `git-panel.tsx` keeps `w-fit` in the class list but adds `min-w-0 flex-1` on a list that is a **flex item of a row**, so `flex-basis: 0%` decides the main size and `width` never gets a say. So the arm has no reference and reproducing it would be inventing a picture |
| `inset-shadow-[0_1px_--theme(--color-white/16%)]` | needs a **white** literal; `check-invariants.sh` rule 4 will not let a component mint one and no token is `#fff` | **absent.** §6 has no shadow field, so the differ sees neither the layer nor its absence |
| `shadow-black/10` on `shadow-xs` | needs a **black** literal, same reason | the port paints gpui's `shadow_xs()` preset, which carries Tailwind's own `0 1px 2px 0` at `rgb(0 0 0 / .05)` — **half the app's alpha**. Painted rather than skipped because it is closer to the truth than nothing, and recorded because it is not the truth |
| `transition-[width,translate] duration-200 ease-in-out` | a curve | **absent.** §6: a snapshot is one instant. See §6 below — this one is not cosmetic |
| `z-0` / `-z-1` / `z-10` | paint order | **absent.** §6 excludes `z` as a field on purpose |
| `cursor-pointer`, `pointer-events-none`, `outline-none` | no comparable representation | absent |
| the second `Tab` export | a different component that shares a file | absent; see the header |

## 4. Painted but invisible to the oracle

| React / Tailwind | gpui | Note |
|---|---|---|
| `focus-visible:ring-2 ring-ring` | not painted | a box-shadow. Unlike `resizable`'s separator the tab is not `tabIndex`-reachable by itself — base-ui's composite roving tabindex is what focuses it — but the state is real and the field does not exist |
| `hover:text-muted-foreground` | not painted | it colours the **call site's** `<span>`, which carries no anchor and is `display: none` in the live cell |
| the tab's font, weight and text colour | set | nothing on this surface paints a character, so no anchor carries the contract's text group at all |

## 5. Anchoring

| Construct | Decision |
|---|---|
| ids in the primitive | a **per-slot default** written *before* `{...props}`, so a call site can override it. `tabs`, `tabs-list`, `tab-indicator` are fixed strings |
| a list with several tabs | **the id is the tab's own `value`**: `tabs-tab-<value>`, `tabs-content-<value>`. Not the `dropdown-menu` answer ("the call site names them") because no live call site may be edited by this item — and `value` is the better key anyway: it is the tab's identity in base-ui, which looks the active tab up by it, and base-ui already requires it to be unique within a root |
| uniqueness | required **within one `tabs` root**, which is what `value` delivers. `web/src/__tests__/components/ui/tabs-anchors.test.tsx` checks it on a rendered tree rather than assuming it |
| the surface's declared set | **none.** v1.8 permits a declaration only when the set is a property of the *surface*; this one is a property of the *cell* — it moves with the tab count, the tab values and whether a panel is mounted |
| the root a capture is taken from | **`sidebar-tab-bar.tsx`'s**, not `git-panel.tsx`'s. See the traps |
| `CONTENT_SIZED` / `LINE_SIZED` | **both empty**, and empty *lists* rather than absent, so the React side's (also empty) declaration set is diffable against them |

## 6. Traps

Each of these compiles, renders something plausible, and is wrong.

| Trap | What actually happens |
|---|---|
| **Taking the indicator's box as a parameter.** | It reads like `resizable::Panel::grow` — a number the library measured and wrote as an inline style — and it is **not the same shape of quantity**. `grow` is an *input* both engines then divide space with, so the compared widths stay each engine's own answer. The indicator's `{left, bottom, width, height}` **is** the answer. Passing it in makes `tab-indicator`'s bounds a copy of the reference's own numbers: the anchor converges on every cell and proves nothing, on the one part of this component that is interesting. The port derives it from its own layout instead, and pays a structural deviation for it (§7) |
| **`ring-2` is not a border.** | Tailwind 4 compiles it to `--tw-ring-shadow: 0 0 0 2px …`, a **box-shadow**. `border.w` is compared *exactly* and a tab's is `1` in every cell; a port reaching for `.border_2()` on focus reports `3` |
| **Skipping `border border-transparent` because the colour is transparent.** | It is a **real 1px border**. It is compared exactly, it takes room in the box model, and it is why the tab's padding is `px-[calc(--spacing(2.5)-1px)]` rather than `px-2.5`. It is also what the indicator's `-1px` inset exists to cross |
| **`gap-x-0.5` becoming a gap in both axes.** | It is `column-gap` only, and the class list sets **no** `row-gap`. Flipping the list to `data-[orientation=vertical]:flex-col` therefore leaves the tabs **touching**. Reproduced rather than corrected |
| **`data-[orientation=horizontal]:translate-y-px` adding to `-translate-y-(--active-tab-bottom)`.** | It **replaces** it. Both write `--tw-translate-y`, and the variant's selector outranks the bare class — so an `underline` indicator sits at the bottom of the **list**, one pixel below it, not under the tab. Only its `x` and `width` still come from the tab |
| **Declaring a tab `line_sized`.** | The badge trap in a third shape, and the strongest yet: a tab *contains* the word "Workspaces" and its height is an authored `h-9 sm:h-8`. Declaring it compares 32 against a 20px line box and invents a 12px delta on an anchor both engines agree on. It is also not even a text anchor — see the next row |
| **Rendering the tab's label through `AnchorSink::text_half`.** | The label is a `<span>` the *call site* puts inside the tab, so the tab element has **no text node of its own** and the React extractor emits no `text`/`fg`/`font`/`text_width`/`clipped` for it. Routing the run through the sink records it under the tab's id and fabricates the whole text group on the native side — five `FieldPresence` deltas per tab, in the `git-row-badge` shape with the sides swapped. The port renders the label as a plain unanchored child |
| **Capturing from `git-panel.tsx`'s `Tabs`.** | Its root contains `ChangedFilesTree`, whose rows are `git-status-file-item`s carrying `git-row-icon`, `git-row-name`, `git-row-badge`, `git-row-added`, `git-row-deleted`. The extractor walks **every** `data-oracle-id` under the root, so the capture swallows `git-status-row` — the v1.8 defect, and here it is **cell-dependent**: with a clean working tree the same root measures 0 foreign anchors and looks fine. The sidebar's root cannot acquire one at all, so that is the reference |
| **Assuming the sidebar's labels are on screen.** | They are not. `sidebar-tab-bar.tsx` hides them behind `@[280px]:inline` / `@[420px]:inline`, and the `@container`'s content box measures **278px** — so *every* label, including the active tab's, is `display: none`. Measured, not read off the class names |
| **Trusting the SVG's `size={14}`.** | Phosphor renders `width="14" height="14"`, which are presentational attributes with no specificity. `sm:[&_svg:not([class*='size-'])]:size-4` wins and the icon measures **16** |
| **Assuming `hover` is one cell.** | It paints two different things. Under `default` the only rule is `hover:text-muted-foreground` on an unanchored, hidden label — invisible. Under `underline` the *list* adds `*:data-[slot=tabs-tab]:hover:bg-accent`, which really is an anchored box's `bg` — but that variant has no live call site, so the cell that could fail has no reference |
| **Assuming the indicator is always in the DOM.** | base-ui returns `null` when the root has no value, and writes `hidden` — Tailwind's preflight makes that `display: none !important` — until `width > 0 && height > 0`. So a capture taken before layout settles reports `visible: false`, and a root driven with `value={null}` has no `tab-indicator` anchor at all |

## 7. What this surface cannot show the differ

- **Shadows, on the one anchor that has them.** The indicator's `shadow-xs
  shadow-black/10 inset-shadow-[…]` is three-quarters of what makes it read as a
  raised pill, and §6 has no field for any of it. The port paints one of the two
  layers at half its alpha and cannot paint the other at all.
- **The 200ms transition.** `width` and `translate`, so `bounds.x`, `bounds.y`
  and `bounds.w` of `tab-indicator` are interpolated for a fifth of a second
  after the active tab changes; `height` is not in the list and does not animate.
  A reference captured inside that window compares an in-flight box against a
  settled one and the delta says nothing about the port. **A capture has to be
  taken at rest**, and there is no field that would say whether it was.
- **The structural deviation itself.** The indicator is a child of the active tab
  here and a child of the list in the reference. §1 makes that irrelevant to the
  differ *by design* — which cuts both ways: the differ also cannot confirm it.
  What confirms it is `row_layout::tabs`, in a real window.
- **Everything a tab contains.** The icon is a call-site SVG and the label a
  call-site `<span>`; neither can carry an anchor, and under `flex-1` neither can
  move one either. `--labels` renders a string for fidelity and the caption says
  the content axis cannot fail without it.
- **Four arms with no reference at all**, rendered anyway and named per cell:
  `--variant underline`, `--orientation vertical`, `--tab-sizing intrinsic` and
  `--panel`. The same shape of finding as Phase 1's `git-row-dir` and
  `resizable`'s `resize-handle-grip`.

## 8. What `tailwind-merge` leaves — the call site is half the component

The two live call sites do not merely add classes; `cn(…)` **drops the
primitive's own**. Reading `tabs.tsx` alone gives the wrong picture on four
properties, and three of them are compared fields.

| Primitive | `sidebar-tab-bar.tsx` | What renders | Compared? |
|---|---|---|---|
| list `w-fit` | `w-full` | `width: 100%` | yes — the list's `bounds.w`. `git-panel.tsx` gets there differently: it keeps `w-fit` and adds `flex-1`, whose `flex-basis: 0%` overrides it on the main axis |
| list `bg-muted` | `bg-sidebar-element-idle` | `#f5f5f51c` in dark | yes — `tabs-list.bg`. Modelled as `ListBackground` |
| list `text-muted-foreground/72` | `text-foreground/70` | a different mix | no — no anchor here paints text |
| tab `shrink-0 grow` | `flex-1` | `flex: 1 1 0%` | yes — every tab's `bounds.w`. Modelled as `TabSizing` |
| tab `gap-1.5` | `gap-1` | 4px | no — under `flex-1` the tabs are equal shares whatever their content |

`git-panel.tsx` overrides only `flex-1` (plus `min-w-0 flex-1` on the list), so it
keeps `bg-muted`, `text-muted-foreground/72` and `gap-1.5` — measured live at
`oklch(1 0 0 / 0.04)`. It is the one live reference for `ListBackground::Muted`,
which is why that arm is *not* declared unreferenced even though the capture in
§9 does not exercise it.

## 9. The reference capture

`/tmp/p3-ref-tabs.json`, taken off the running app at `innerWidth` **1714**
(`theme` omitted, so it derived `dark`), rooted on `tabs` at index 0 — the
sidebar tab bar, at rest, on the home route where it renders three tabs.

```text
tabs                   0,0,278,36     bg #00000000  r 0   border 0
tabs-list              0,0,278,36     bg #f5f5f51c  r 10  border 0
tabs-tab-workspaces    2,2,90,32      bg #00000000  r 8   border 1 #00000000
tabs-tab-chats        94,2,90,32      bg #00000000  r 8   border 1 #00000000
tabs-tab-files       186,2,90,32      bg #00000000  r 8   border 1 #00000000
tab-indicator          2,2,90,32      bg #1f1f1eff  r 10  border 1 #1f1f1eff
```

Six anchors, each exactly once — v1.8 satisfied without a declaration.
**`tab-indicator` and `tabs-tab-workspaces` are the same box**, which is the
measurement the whole port rests on, and `row_layout::tabs` reproduces every
number above from taffy's own layout.

The running app the capture was taken from was **not** the one this item was
briefed to reuse: pid 80769 on bridge 9224 exited mid-item, and the surviving
`crowbar-desktop` on 9223 — same worktree, confirmed through `driver_session` —
is 1714×1119 rather than 1200×800. That is why `state.width` is 1714 and why the
surface is driven `--width 278 --viewport-width 1714`; the sidebar is 278px wide
at either window size, so no anchor above moved.

The bundle that app was serving predates the `data-oracle-id` edit, which lives
in a worktree the dev server does not serve. The attributes were therefore
applied to the live DOM with `setAttribute` immediately before the extract and
removed immediately after, with each tab's `value` read back **off its React
fiber** rather than guessed from its label. `tabs-anchors.test.tsx` is what
proves the same ids come out of the source.
