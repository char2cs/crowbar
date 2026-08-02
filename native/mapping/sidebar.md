# `sidebar` (P3.19) — the wrap that reaches **one box of three**

`web/src/components/ui/sidebar.tsx` →
`crates/crowbar-ui/src/components/sidebar.rs`, built on
`gpui_component::sidebar`.

> A §6.2 row, in the shape `native/MAPPING.md` fixes. Kept in its own file
> because P3 runs several workers in parallel and one appended table is one
> conflict per item.

**References:** `/tmp/p3-ref-sidebar.json` and `/tmp/p3-ref-sidebar-empty.json`,
both captured live from the file-explorer panel at a **1714px** viewport, dark,
with the sidebar carousel snapped to Files.

> **`p3-ref-sidebar.json` carries `"surface": "sidebar-header"`**, not
> `"sidebar"`. The item's brief names the *path*; the file is named for the
> component and the field is named for the surface, and the differ refuses two
> snapshots that disagree on that field — so the native half of that pair is
> `--surface sidebar-header`, and `--surface sidebar` is not a word this binary
> takes. `sidebar.tsx` yields **two** surfaces, one anchor and two anchors
> respectively, and the reason there is no single `sidebar` surface spanning
> them is §0: the component that would have contained both has zero call sites
> and no reachable box.

| file | surface | root | anchors |
|---|---|---|---|
| `/tmp/p3-ref-sidebar.json` | `sidebar-header` | `sidebar-header` | 1 |
| `/tmp/p3-ref-sidebar-empty.json` | `sidebar-empty` | `sidebar-empty` | 2 |

**Live count: 15 files import `@/components/ui/sidebar`** — 8 sources and 7
tests, the most-used component left, and the number the brief predicted. What
that count *hides* is §1.

---

## 0. THE HEADLINE: four of the six visual exports are dead, and two of the three vendor boxes resist

Two separate results. Conflating them would be the mistake.

**Result 1 — most of this file is not on screen.** `sidebar.tsx` exports nine
names. Three are React context (`useSidebar`, `useSidebarOptional`,
`SidebarProvider`) and author no box. Of the **six visual** exports, a JSX grep
over `web/src` outside `__tests__` finds:

| export | live call sites | where |
|---|---|---|
| `SidebarHeader` | **1** | `features/file-explorer/file-explorer/components/file-explorer-tree.tsx:1139` |
| `SidebarEmptyActionState` | **2** | the same file, `:1197` and `:1205` |
| `Sidebar` | **0** | — |
| `SidebarFooter` | **0** | — |
| `SidebarHeaderSearch` | **0** | — |
| `SidebarHeaderIconButton` | **0** | — |

```
$ grep -rnE "<Sidebar([ />]|$)"                src --include='*.tsx' | grep -v __tests__   # nothing
$ grep -rnE "<SidebarFooter([ />]|$)"          src --include='*.tsx' | grep -v __tests__   # nothing
$ grep -rnE "<SidebarHeaderSearch([ />]|$)"    src --include='*.tsx' | grep -v __tests__   # nothing
$ grep -rnE "<SidebarHeaderIconButton([ />]|$)" src --include='*.tsx' | grep -v __tests__  # nothing
```

The 15 importers are mostly reaching for the **context**: `ide-shell.tsx` for
`SidebarProvider`, `tab-bar.tsx` and `sidebar-project-header.tsx` for
`useSidebar`, `pane-container.tsx` for `useSidebarOptional`. So the live surface
of this 402-line file is **two components and three call sites**, both in the
file explorer.

**Result 2 — the wrap reaches one box.** `gpui_component::sidebar` has three
boxes and **exactly one** of them can carry this port's geometry. §2 is the
account, with the vendor quoted. The two that resist are `Sidebar` and
`SidebarFooter` — which are also the two with zero call sites, so nothing on
screen is missing a port, and per the item's instruction they are **reported
rather than rebuilt**.

**Verdict: strict parity is reached on `SidebarHeader` and on
`SidebarEmptyActionState`.** No property resisted *styling* on either. What
resisted is the **seam**, on two components that are not rendered.

---

## 1. The seam test needs a second half

`select.md` states the test this phase has been applying:

> A widget is wrappable-and-measurable exactly when it lets the caller supply an
> element, not merely a style.

It is **necessary and not sufficient**, and `sidebar/` is where that shows. All
three vendor boxes take caller-supplied children — `Sidebar::header`/`::footer`
take `impl IntoElement`, `SidebarHeader` and `SidebarFooter` both
`impl ParentElement`. Two of the three still cannot be measured.

The missing half: **the caller's element has to be able to *be* the box.**
`AnchorSink` takes a `gpui::Div` this crate holds, so the anchor lands on the
child. If the vendor puts geometry between its own border box and that
child — padding, a wrapper, a hard-coded alignment — then the anchored box is
not the box `sidebar.tsx` describes, and no amount of styling closes the gap.

Restated as a rule for the rest of the §6.2 list:

> **Wrappable-and-measurable** = the caller supplies an element **and** the
> vendor's `Styled` impl addresses the box that element ends up inside.

`popover` passes both halves (its `v_flex` wrapper paints nothing and takes no
room). `SidebarHeader` passes both. `SidebarFooter` fails the second on its
`Styled` impl; `Sidebar` fails it on a wrapper with no seam at all.

---

## 2. What resisted, precisely

### 2.1 `SidebarFooter` — `Styled` addresses the wrong box

```rust
impl Styled for SidebarFooter {
    fn style(&mut self) -> &mut StyleRefinement { self.base.style() }   // the INNER box
}
impl ParentElement for SidebarFooter {
    fn extend(&mut self, els: …) { self.base.extend(els) }              // also the inner box
}
impl RenderOnce for SidebarFooter {
    fn render(self, _, cx) -> impl IntoElement {
        h_flex().id("sidebar-footer").gap_2().p_2().w_full()             // ← no seam reaches this
            .justify_between().rounded(cx.theme().radius)
            .hover(…).when(self.selected, …)
            .child(self.base)
    }
}
```

`SidebarFooter` in `sidebar.tsx` is **one** `<div>`: `flex flex-col gap-2 p-2`.
The vendor's outer `h_flex()` is built from literals inside `render`, and
neither `Styled` nor `ParentElement` reaches it — both land on `base`, which is
*inside* the padding. So:

| property | `sidebar.tsx` | vendor's outer box | reachable? |
|---|---|---|---|
| direction | `flex-col` | `h_flex()` — row | **no** |
| `justify-content` | `flex-start` (unset) | `justify_between()` | **no** |
| `border-radius` | `0` | `cx.theme().radius` — **`gpui-component`'s** table | **no** |
| padding | `p-2`, on the anchored box | `p_2()`, outside it | **no** |

An anchor on the only element this crate holds reports **origin (8, 8)** against
(0, 0) and **W − 16 × H − 16** against W × H. Four fields wrong on one box, and
`radius` is compared exactly.

*Could a negative margin cancel it?* `base.m_neg_2()` does make the two border
boxes coincide — the arithmetic works. It is not done: it makes the component
worse to make a measurement pass, which is the trade `ANCHORS.md` refuses for
pinned widths, and it would leave `justify_between` and the vendor radius
un-cancelled anyway.

### 2.2 `Sidebar` — a wrapper React does not have

`gpui_component::sidebar::Sidebar::render`, in order:

```rust
self.style.padding = EdgesRefinement::default();          // a caller's padding is DISCARDED
…
.when_some(self.header.take(), |this, header| this.child(
    h_flex().id("header").pt_3().px_3().gap_2().child(header)))          // 12px, no seam
.child(v_flex().id("content").flex_1().min_h_0().child(
    v_flex().id("inner").size_full().px_3().gap_y_3()
        .child(list(list_state, …).size_full())                          // a VIRTUALISED list
        .vertical_scrollbar(&list_state)))
.when_some(self.footer.take(), |this, footer| this.child(
    h_flex().id("footer").pb_3().px_3().gap_2().child(footer)))          // 12px, no seam
```

`sidebar.tsx`'s `sidebar-inner` is `flex h-full w-full flex-col bg-sidebar` and
puts its children **flush**. So a header rendered through the vendor's `Sidebar`
sits at **(12, 12)** with width **W − 24**, where React's sits at (0, 0) with
width W — a constant offset on every anchor beneath it, which is exactly the
tell the brief warns about for `--width` / `--viewport-width`, arriving from a
different direction.

Three further blocks, each sufficient on its own:

* `self.style.padding = EdgesRefinement::default()` — a caller cannot set
  padding on the sidebar at all, because `render` wipes it first.
* the content children are `E: SidebarItem`, and `SidebarItem: Collapsible +
  Clone` is `gpui-component`'s own trait. **No `gpui::Div` implements it**, so
  the body cannot be this crate's element — the same wall `select` hits, one
  level up.
* the body goes through `list(ListState, …)` with `vertical_scrollbar`, and its
  items take `pt_3()` / `pb_3()`. `sidebar-inner` has no list, no scrollbar
  gutter and no per-item padding.

`Styled for Sidebar` *does* address the sidebar's own box, and `refine_style`
runs after `bg`, `border_r_1()` and `w(DEFAULT_WIDTH)` — so the outermost box is
reachable. It is the **children's placement** that is not, and a sidebar whose
children are in the wrong place is not a sidebar this port can measure.

### 2.3 The `SidebarHeader` wrap costs everything and buys no geometry

Stated plainly, because §6.2's point is to know.

`SidebarHeader` in `sidebar.tsx` is six Tailwind classes and no behaviour.
`gpui_component::sidebar::SidebarHeader` is a **different component that shares
the name**. Every one of its own decisions has to be cancelled:

| vendor writes | `sidebar.tsx` has | cancelled by |
|---|---|---|
| `h_flex()` — row + `items_center` | `flex flex-col` (align `normal`) | `.flex_col().items_stretch()` |
| `justify_between()` | unset — `flex-start` | `.justify_start()` |
| `rounded(cx.theme().radius)` | `border-radius: 0` | `.rounded(px(0.))` |
| `p_2()` | `p-2` **on the anchored box** | `.p_0()` |
| `gap_2()` | `gap-2` **on the anchored box** | `.gap_0()` |
| `hover(bg sidebar_accent)` | *nothing* | **cannot be cancelled** — §2.4 |
| `when(selected, bg sidebar_accent)` | *nothing* | **cannot be cancelled** — §2.4 |

The vendor's `p_2()` and `gap_2()` happen to be `sidebar.tsx`'s own numbers.
That is a **coincidence, not a seam** — `Sidebar` and `SidebarFooter` write the
same literals around boxes that must not have them — so they are cancelled and
re-applied on the box this crate holds, which is the box the anchor is on. This
is `popover`'s `appearance(false)` one step further along: there the vendor's
box stopped *painting*; here it also stops taking room.

**What survives and is worth having:** the vendor's element identity and
`w_full()`, its `InteractiveElement` / `Selectable` / `Collapsible` /
`DropdownMenu` impls, and the reason §6.2 asks for the wrap at all — a
`gpui-component` bump changes `crates/crowbar-ui/src/components/sidebar.rs` and
no other file.

### 2.4 Two things the vendor does that `sidebar.tsx` does not

`hover` and `selected` both paint `sidebar_accent`. They are applied **after**
`refine_style` and live in a separate style map, so no refinement reaches them.
They are invisible to a snapshot — one instant, at rest — and they are a real
divergence. `surfaces/sidebar_header.rs` therefore declares both **unmodelled**,
which is the *opposite* of the usual reason: normally a flag is unmodelled
because neither side has the state; here it is unmodelled because **only the
native side does**, and a cell driven on it would disagree by construction.

---

## 3. Values — `SidebarHeader`

Every "Compiles to" was measured with `getComputedStyle` on the live element.
Root anchor `sidebar-header`, **344 × 44**.

| React / Tailwind | Compiles to (measured) | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `flex` | `display: flex` | `.flex()` | not a field |
| `flex-col` | `flex-direction: column` | `.flex_col()` | drives the layout |
| (unset) | `align-items: normal` | `.items_stretch()` on the wrap | the child is 328 in a 344 box |
| `gap-2` | `row-gap: 8px` | `HEADER_GAP` | `bounds.h` with >1 child |
| `p-2` | `padding: 8px` ×4 | `HEADER_PADDING` | `bounds` |
| (unset) | `background-color: rgba(0,0,0,0)` | nothing | `bg` = `#00000000` |
| (unset) | `border-width: 0px` | nothing | **`border.w` = 0** |
| (unset) | `border-radius: 0px` | nothing | **`radius` = 0** |
| `backdrop-blur-sm` | `backdrop-filter: blur(8px)` | **nothing** | **§6: no field, either side** |
| inherited | `font: 16px/24px CalSansUI` | nothing | the box paints no run, so no `font` group |

**`border` is 0 here and the trap runs the other way.** `popover`'s popup has a
bare `border` and reports `border.w: 1`; `dropdown_menu`'s has `ring-1` and
reports 0. This box has neither, and a port that reached for `.border_1()` out of
`popover`'s habit would be wrong by exactly one pixel on all four edges.
Measured, not inferred — the same instruction, third component running.

**`backdrop-blur-sm` is `blur(8px)`, not 4.** Read off the live element rather
than off Tailwind's scale. It carries no `ANCHORS.md` field either way, so it is
recorded and not painted; noted because "sm ⇒ 4px" is the natural guess and it
is wrong.

### The body is a *height*, and why that is not a fudge

`SidebarHeader`'s children are the call site's. The live one holds a search
`<Input>` and a filter `<Button>` — two primitives this port already measures as
their own surfaces, and neither is *this* component. So `--body-height` is their
**measured extent** (28px), exactly as `popover`'s is.

There is deliberately **no `--body-width`**: the header is a column whose
cross-axis alignment `sidebar.tsx` leaves at `normal`, so the child is
*stretched* — 328 inside 344 without authoring a width. A width parameter would
have decided nothing, and a parameter that decides nothing is one more way for a
cell to be wrong invisibly.

### Declarations

* `content_sized`: **no.** The header is stretched by its parent; its used width
  is 344 because the panel is.
* `line_sized`: **no.** Its height is padding plus a body, and it paints no run
  of its own.

---

## 4. Values — `SidebarEmptyActionState`

The only component in the file with **no `gpui-component` equivalent**, so it is
built from `div()`. Not a §6.2 exception: the instruction is not to rebuild what
the library provides, and it provides no empty state.

Root anchor `sidebar-empty`, **123.94 × 96**, holding `sidebar-empty-message`
**99.94 × 16** at (12, 40).

| React / Tailwind | Compiles to (measured) | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `ui-font` | `font-family: CalSansUI, …` | `theme.font_sans.primary()` | `font.family` on the run |
| `flex flex-col` | column | `.flex().flex_col()` | layout |
| `items-center` | `align-items: center` | `.items_center()` | **makes both boxes shrink-wrap** |
| `justify-center` | `justify-content: center` | `.justify_center()` | puts the run at **y 40** |
| `min-h-24` | `min-height: 96px` | `EMPTY_MIN_HEIGHT` | **`bounds.h` = 96** |
| `gap-1.5` | `row-gap: 6px` | `EMPTY_GAP` | `bounds` |
| `px-3` / `py-6` | `12px` / `24px` | `EMPTY_PADDING_X/Y` | `bounds` |
| `text-muted-foreground` | `oklch(0.72 0 0)` | `theme.muted_foreground` | `fg` = `#a4a4a4ff` |
| `text-center` | `text-align: center` | nothing | **no field** |
| `select-none` | `user-select: none` | nothing | no field |
| (unset) | `background-color: rgba(0,0,0,0)` | nothing | `bg` = `#00000000` |
| message `ui-text-sm` | `font-size: 12px` | `theme.ui_text_sm` | `font.size` = 12 |
| message `leading-[1.35]` | `line-height: 16.200001px` | `relative(1.35)` | **`line_sized`**, `font.line_height` = 16.2 |
| icon `size-7` / `mb-0.5` | `28px` / `2px` | `EMPTY_ICON_EXTENT` / `_MARGIN_BOTTOM` | `bounds` |
| description `ui-text-xs` | `11px` | `theme.ui_text_xs` | `font.size` |
| description `max-w-[24ch]` | a clamp in `ch` | **recorded, not applied** | see below |
| the action `<Button>` | a whole primitive | **not rendered** | §6 |

**`ui-text-sm` is the token, literally.** `@utility ui-text-sm` in `index.css`
compiles to `font-size: var(--ui-text-sm)` and nothing else, and
`--ui-text-sm: calc(0.75rem * var(--app-ui-scale))`. There is no `sm:`
counterpart on this box, so the `label` trap — where a primitive's
`sm:text-sm/4` beats a call site's `ui-text-sm` — does not fire. Measured live at
12px, which is the check.

**`max-w-[24ch]` is recorded and not applied.** `ch` is the advance width of `0`
in the resolved font and gpui has no such unit; reaching it means shaping a glyph
at layout time through an API `Styled` does not expose. The description has **no
live call site**, so nothing measures the clamp, and inventing a pixel number for
it would be asserting a width the reference has no opinion about.
`EMPTY_DESCRIPTION_MAX_WIDTH_CH` exists so the gap is a declaration.

### Declarations, and the arithmetic behind the container's

* `LINE_SIZED = [sidebar-empty-message]`. `leading-[1.35]` on a block `<div>`
  with no padding and no authored height puts a 12px run in a 16.2px line box
  and makes that box the border box — v1.6's shape exactly. The reference
  carries both numbers: `bounds.h` **16** (WebKit's floor) and
  `font.line_height` **16.2**.
* `CONTENT_SIZED = [sidebar-empty, sidebar-empty-message]`. `items-center` on a
  column flex container means its items are **not stretched**, so each width is
  its own max-content. The container's is that plus 24px of `px-3`, and integral
  padding carries `ceil` through unchanged:

  ```
  ceil(99.94) + 24  ==  ceil(123.94)      # 100 + 24 == 124
  ```

  which is the arithmetic v1.5 needs, confirmed by the reference's own two
  numbers. Declaring a *container* alongside the run it is sized by is therefore
  correct rather than convenient — and it is the same call `loading_spinner`
  makes for its wrapper.
* The **root is neither line-sized nor `min-h`-free**: `min-h-24` authors 96px
  around a 64.2px column, so declaring `line_sized` on it would compare 96
  against a 24px line box — the mistake v1.6's badge warning is about.
* The **description declares neither**: `max-w-[24ch]` makes it a line box only
  while the string fits on one line, and the component cannot know that.

---

## 5. THE FINDING: a root anchor could not be `content_sized`

`sidebar-empty` is the first surface in this port whose **root** shrink-wraps,
and it uncovered a hole in the driver.

```rust
// crowbar-app/src/driver_anchors.rs, before P3.19
fn root(&self, id: AnchorId, element: Div) -> AnyElement {
    crowbar_driver::anchor_root(id.id, element).into_any_element()   // declarations DROPPED
}
```

`anchor_root` hard-coded `Declared::nothing()` and there was no other spelling.
The DOM extractor has no such hole — it reads `data-oracle-content-sized` off
whichever element carries the root id — so a surface like this one would have
emitted `content_sized: true` on the reference and `false` on the native side,
and the differ reports that as a **`ContentSizedMismatch` on every cell**: a
disagreement, not a missing feature, which is the hardest kind of finding to
read backwards.

Fixed by `crowbar_driver::anchor_root_declared`, with the declarations now
travelling through `DriverAnchors::root`. Two tests in
`crowbar-driver/src/element.rs` pin it — one that a declared root carries its
flags and a **control** that `anchor_root` still declares nothing, so the pair
cannot pass by both spellings having become the same thing.

No existing surface changes: none of them declares anything on its root, and the
1176-test baseline is byte-identical either side of the fix.

---

## 6. What is deferred, and why it is named rather than approximated

**The empty state's action `<Button>`.** `SidebarEmptyActionState` renders
`<Button variant="ghost" compact className="ui-text-xs h-6 px-2
text-muted-foreground hover:text-foreground">` when a call site passes
`actionLabel` + `onAction`. It is not modelled. Two reasons, each sufficient:

* `crowbar_ui::components::Button::render` reaches `AnchorSink::root`, and the
  root anchor **clears the registry** — a button nested inside this surface takes
  this surface's anchors with it. The way round is to render the nested
  primitive through `Unanchored`, which is a real option and a change to make
  deliberately.
* `h-6 px-2` is not one of `button`'s ten `Size` arms, so modelling it means
  inventing a size that surface does not have — on an anchor with **no live
  reference**, since "No folder open" is the one call site a parity run cannot
  reach.

`SidebarHeaderIconButton` and `SidebarHeaderSearch` are deferred the same way and
for the first of those reasons. Both are **presets over primitives this port
already has** — `size-6 rounded-md p-0` over `button`, and `h-6 px-2 ps-7`
plus a `size-3.5` glyph over `input` — and both have **zero call sites**. They
are carried as named constants (`sidebar::header_icon_button`,
`sidebar::header_search`) so the values are in the system, and as no element, so
the port does not gain an unreachable, untestable tree.

---

## 7. Reachability, and how the references were taken

Both surfaces live in the **file explorer**, inside the sidebar carousel.

**The carousel has to be on Files.** The first look found the header at
`x 2058` against an `innerWidth` of 1714 — geometrically perfect and entirely
off-screen, which is `sidebar-carousel`'s §7 "capture precondition" finding
meeting a third surface. `HTMLElement.click()` on the Files tab moved it to
`x 1370`, in view. (The sidebar is on the **right** in this workspace.)

**The empty state was driven, not waited for.** `No matching files` needs only a
tree filter that matches nothing:

```js
const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set
setter.call(input, 'zzzznomatchzzzz')
input.dispatchEvent(new Event('input', { bubbles: true }))
```

which reaches React because it is a DOM dispatch rather than a `CGEvent`. The
filter was cleared afterwards and the carousel returned to Workspaces, as found.
`No folder open` — the one with the action button — needs the fixture workspace
to have no folder open at all and was **not** driven.

**`document.visibilityState` is `hidden` and could not be changed.** The window
is at physical (3446, 2274), off the main display; `setPosition` is refused by
the Tauri capability set (`core:window:allow-set-position` is not granted) and
`osascript` has no assistive access on this machine. Unlike `popover`, **nothing
here needs a frame**: neither box has a transition, an animation or a mount
state — `backdrop-blur-sm` is a static filter — so the capture is of a settled
layout either way, and the two `getBoundingClientRect` readings taken minutes
apart agree to the pixel. Recorded because the brief is right that a hidden
window is where a 0.98-scale error hides; it is not where this one could.

**The bundle predates the `data-oracle-id` edit** — the dev server serves the
other worktree — so the attributes were applied to the live DOM with
`setAttribute` immediately before the extract and removed immediately after
(P3.2's arrangement). The snapshots were written **byte-exact through a local
HTTP sink** (`Bun.serve` on an ephemeral port, body straight to `writeFileSync`),
so nothing round-tripped through the bridge's JSON. `theme` was omitted from the
options and derived as `dark`; `state.width` is the live `innerWidth`, **1714**.

### Both surfaces needed an `oracleSurfaceScope` entry

Measured, not assumed. An undeclared capture rooted on `sidebar-header` comes
back with **`input-control`, `input` and `button`** under it — the call site's
`<Input>` and `<Button>`, each carrying its own primitive's id. Those are not
this surface's anchors (v1.8), so `extract.ts` gained:

```ts
'sidebar-header': { root: 'sidebar-header', anchors: ['sidebar-header'] },
'sidebar-empty':  { root: 'sidebar-empty',  anchors: ['sidebar-empty', 'sidebar-empty-message'] },
```

Both sets are derived from the **component** — the boxes it renders
unconditionally — and not from the capture. `sidebar-empty`'s icon, description
and action are each behind a prop, so none is declared, which is the same call
`popover` makes about `PopoverTitle` and for the same reason: a declared anchor
that is not in the document throws.

---

## 8. The state axis

### `sidebar-header`

| flag | modelled? |
|---|---|
| `empty` | **yes** — a childless header collapses to its own `p-2`: 344 × 44 → 344 × 16. No live reference; the caption says so. |
| `hover`, `selected` | **no**, and §2.4 is why: React has no rule and the *vendor* does. |
| `focus` | no — neither side has one. |
| `loading`, `error` | no. |

### `sidebar-empty`

| flag | modelled? |
|---|---|
| `empty` | **yes**, in `label`'s reading of the word: the message is the empty string, the run vanishes, and the anchor drops its font group and its `line_sized` declaration with it. `min-h-24` then decides the box alone. |
| `error` | **no** — and this is a ruling. `SidebarEmptyActionState` really does have an error *appearance* (`tone="error"` → `text-destructive`), which makes it tempting; but `StateFlag::Error` is §8.3's "the component is reporting a failure", which this port holds unmodelled on **every** surface. The appearance is a **prop**, so it is `--tone`. `surface.rs`'s `no_surface_declares_its_entire_state_axis_unmodelled` is what caught the two being conflated — the first attempt wired `--flags error` to the tone and failed that invariant, correctly. |
| `loading`, `hover`, `focus`, `selected` | no. `grep -o 'hover:[a-z-]*\|focus:[a-z-]*\|active:[a-z-]*'` over the component finds **one** hit, `hover:text-foreground`, and it is on the action `<Button>` this surface does not render. |

`--tone` walks all three tones; **none is live**, since neither call site passes
one, so `neutral` is reached through the prop's default and the other two have no
reference. The caption says which cells those are.

---

## 9. What the oracle cannot see here

`ANCHORS.md` §6 material, both sides, listed so nobody looks for it:

* `backdrop-blur-sm` on the header — a filter, no field.
* `select-none` and `text-center` on the empty state — no field.
* the vendor's `hover` / `selected` fills — a snapshot is one instant at rest.
* `max-w-[24ch]` — unreachable in gpui and unreferenced in React (§4).
