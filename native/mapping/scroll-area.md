# `scroll-area` (P3.20) — the wrap test came back **split**

`web/src/components/ui/scroll-area.tsx` →
`crates/crowbar-ui/src/components/scroll_area.rs`, **built** rather than wrapped.

> A §6.2 row, in the shape `native/MAPPING.md` fixes. Kept in its own file
> because P3 runs several workers in parallel and one appended table is one
> conflict per item.

**Reference:** `/tmp/p3-ref-scroll-area.json`, captured live from
`workspace-tree`'s `<ScrollArea className="flex-1">` in the sidebar's Workspaces
panel, at a 1714px viewport, dark.

**Live count: 3 importers, all 3 reachable, and all 3 measured in one frame** —
`workspace-tree` (`344 × 936`), `git-panel` (`344 × 920`) and `autocomplete`
via the command palette (`574 × 46`). Every field but the extent agrees across
the three, which is what makes the extent a parameter and the rest constants.

> **This contradicts the brief**, which said 4 importers. `grep -rn
> "ui/scroll-area"` over `web/src` outside `__tests__` finds **3**:
> `features/git/components/git-panel.tsx`, `components/ui/autocomplete.tsx` and
> `components/layout/workspace-tree.tsx`. Reported rather than reconciled — the
> scope conclusion is unchanged either way, since none of the three is a Plate
> node.

## 0. The headline: wrap-vs-build, and the seam evidence

The vendor answers the wrappability question **differently for its two halves**,
which is why the verdict needed both and why "there is no seam" would have been
the wrong summary.

### The container seam exists, and a name grep would have missed it

`native/vendor/gpui-component/src/scroll/scrollable.rs:16`:

```rust
pub trait ScrollableElement: InteractiveElement + Styled + ParentElement + Element {
    #[track_caller]
    fn scrollbar<H: ScrollbarHandle + Clone>(self, scroll_handle: &H, axis: impl Into<ScrollbarAxis>) -> Self
```

`self` in, `Self` out — one `.child()` push onto **the caller's own element**,
with its id, its handle and its style untouched. `overflow_y_scrollbar(self) ->
Scrollable<Self>` is the `Something<Self>` wrapper form, and `Scrollable<E>` even
implements `ParentElement` by forwarding `extend()` to the element it was handed.

Both are exactly the shapes the brief warned a member-name grep would miss, and
for exactly the stated reason: the trait is `ScrollableElement`, not
`Scrollable*Ext`, and it is bounded on `Element` rather than on `IntoElement`.
**So the honest answer to "is there a seam" is yes.**

### It is the wrong half — the scrollbar takes no element, ever

What this component needs from a vendor is the **scrollbar**, and that half has
no seam at all. From `scroll/scrollbar.rs`:

| | |
|---|---|
| construction | `Scrollbar::new/vertical/horizontal(&H: ScrollbarHandle + Clone)`. The remaining builders are an `ElementId`, a `ScrollbarShow` enum, a `Size<Pixels>` and a `ScrollbarAxis`. **Style knobs, no element.** |
| the grep | within that whole file, `AnyElement`, `ParentElement`, `impl Fn`, `dyn Fn` and `.child(` **do not appear** — run, not assumed |
| its layout box | `request_layout` builds `Style::default()` with `size.width = relative(1.)`, `size.height = relative(1.)` and calls `window.request_layout(style, **None**, cx)` — that `None` *is* the child `LayoutId` slice |
| the thumb | `cx.paint_quad(fill(state.thumb_fill_bounds, state.thumb_bg)…)` at `scrollbar.rs:804` — painted into the scene during `paint`, with **no layout node** |

The layout box is the decisive one. `AnchorSink::root` takes a `Div` this crate
*holds*, so the only way to anchor the vendor's track would be a `div()` wrapped
around it — and that box would be `relative(1.) × relative(1.)`, i.e. **the whole
viewport**, against React's `m-1 w-1.5` 6px track. Not even the coincidence the
brief describes: the bounds would be flatly wrong. And the thumb has no layout
node for the driver to read at all, so it could never be an anchor on this side.

**Verdict: build.** The two boxes this surface anchors are plain `div()`s either
way; the one box a wrap would have supplied is unanchorable and mis-shaped.
Wrapping would have bought fade timing and drag handling — behaviour a snapshot
cannot see — at the cost of the only two things it can.

## 1. Values — the root

`ScrollAreaPrimitive.Root`. Every "Compiles to" was measured with
`getComputedStyle` on the live element.

| React / Tailwind | Compiles to (measured) | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `size-full`, call site `flex-1` | `344 × 936` | `ScrollArea::width` / `::height` | `bounds` = 344 × 936 |
| `min-h-0` | `min-height: 0px` | taffy's default | not a field |
| base-ui's inline `position: relative` | `position: relative` | `.relative()` | not a field |
| *(none)* | `border-width: 0px` | `BORDER_WIDTH` | **`border.w` = 0** |
| *(none)* | `border-radius: 0px` | `RADIUS` | `radius` = 0 |
| *(none)* | `background: rgba(0,0,0,0)` | no `.bg(…)` | `bg` = `#00000000` |
| base-ui's `--scroll-area-corner-*` vars | `0px` both | nothing | no field |

## 2. Values — the viewport

| React / Tailwind | Compiles to (measured) | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `h-full` + block width fill | `344 × 936` | `.w_full().h_full()` | `bounds`, identical to the root's |
| `rounded-[inherit]` | `border-radius: 0px` | `.rounded(RADIUS)` | `radius` = 0 |
| `outline-none` | `outline-style: none` | nothing | no field |
| base-ui's `overflow` | `overflow: scroll` | `.overflow_hidden()` | drives `clipped` — see §4 |
| `transition-shadows` | — | nothing | §6: a snapshot is one instant |
| `focus-visible:ring-2 ring-offset-1` | a box-shadow | nothing | **§6, and unreachable** — see §3 |
| `data-has-overflow-y:overscroll-y-contain` | overscroll behaviour | nothing | not visual |
| `scrollbarGutter` → `data-has-overflow-y:pe-2.5` | `padding-inline-end: 10px` | `GUTTER` | inside the border box: moves no anchor |
| `scrollFade` → four `mask-*-from-…` | a mask | `FADE_SIZE`, carried as data | **§6: no field, either side** |

**The two boxes land on top of each other.** `h-full` inside a box with no
padding, no border and no radius genuinely is the whole box, and the live capture
reports all four numbers identical. `row_layout/scroll_area.rs` pins it, because
a coincidence like that is the shape a port can reproduce for the wrong reason.

### Declarations

* `CONTENT_SIZED = []`. Neither box paints text, so neither has a max-content
  width to size to.
* `LINE_SIZED = []`, for the sharper half of the same reason: v1.6 refuses the
  declaration on an anchor with no `font`.

## 3. The scrollbars are **ported and unanchored** — v1.8 *and* v1.11

Not a shortcut. From
`node_modules/@base-ui/react/scroll-area/scrollbar/ScrollAreaScrollbar.js`:

```js
const shouldRender = keepMounted || !isHidden;
// …
if (!shouldRender) { return null; }
```

`scroll-area.tsx` never passes `keepMounted`, so it defaults to `false`;
`isHidden` is that axis's overflow state. `ScrollAreaCorner` is the same shape
(`if (hiddenState.corner) return null`). **No overflow, no DOM node** — confirmed
on all three live instances, every one reporting `root.children.length === 1`,
and confirmed a second way by forcing 3000px of content into the viewport and
watching the child count stay at 1.

Two rules follow and point the same way:

* **v1.11** — an element that generates no box is not an anchor.
* **v1.8** — a surface may declare its anchor set only when the set is a property
  of the *surface*. Scrollbar presence is a function of content overflow, which
  is a property of the *cell*, so declaring them would turn every honest
  no-overflow capture into a refusal for a missing declared anchor.

So `ScrollBar` and `Corner` exist in the port — the component has them and §17
wants the picture — rendered under base-ui's own condition and carrying no
anchor. `kbd`'s `KbdGroup` arrangement, with a second reason stacked on it.

`web/src/__tests__/lib/oracle/extract.test.ts` proves both halves rather than
asserting them: one cell with the tracks mounted and one without, both reducing
to the same two anchors.

The one measured constant they carry that is worth keeping: **`rounded-full` on
the thumb is `px(f32::MAX)`**, the value `switch`'s track records, not gpui's
`rounded_full()` preset of `px(9999.)`. Nothing compares it today because the
thumb has no anchor; a constant that is right only because nobody looks is one
that will be wrong the day somebody does.

## 4. `clipped`, and why v1.12 matters here

The viewport's computed `overflow` is **`scroll`**, so it genuinely hides what
overruns it — an anchored run inside one would report `clipped` honestly on both
sides under v1.12's rule. This surface anchors no text, so nothing turns on it
today. It is recorded because the viewport is the one box in this port's tree
whose `overflow` is not `visible`, and v1.12's whole point is that the DOM side
must consult it.

## 5. The axes, and the two that are **not** what a reader would guess

| Axis | Here |
|---|---|
| `--width` | **real**, which almost no leaf surface can say. The root is `size-full` and authors no width at all, so the surface column *is* the containing block and both anchors move together. |
| `--area-height` | the other half, as a parameter — the surface column has no height to inherit. The same call `popover` makes about `--body-height`. |
| `--theme` | **vacuous, and asserted rather than assumed.** Both boxes are transparent, unbordered, unrounded and paint no text, so there is no field for the two tables to differ on. The one colour that does move — the thumb's `bg-foreground/20` — is on an element with no anchor. `the_two_themes_paint_the_same_two_boxes` pins the vacuity with a control, because a theme cell that quietly compares equal is indistinguishable from one that converged. |
| `--viewport-width` | vacuous. `scroll-area.tsx` contains no `sm:` variant. |
| `--content` | vacuous. Neither box is sized by its content. |

## 6. The state axis

Five flags are declared unmodelled. `empty` is **modelled**, and both halves of
why are worth stating because either alone would mislead:

* It is **real**: `{children}` is a prop, rendering none is an expressible
  picture, and base-ui *reacts* to it — overflow is computed from content, so a
  childless viewport can never mount a track. The cell forces `Overflow::NONE`
  however `--overflow` was spelled, which is the coupling `popover`'s `empty`
  makes when it drops the title with the body.
* And **neither anchored box moves**, because neither is sized by its content.

That is not the fact `unmodelled` announces. `unmodelled` means the original has
**no such state to disagree with** — `popover.tsx` has no `hover:` rule anywhere,
`kbd.tsx` carries `pointer-events-none`. This one has the state; what it lacks is
a box whose size depends on it, and the caption says so per cell rather than
leaving a reader to infer coverage from a flag that was accepted.

`focus` is unmodelled for a second reason on top of §6: `focus-visible:ring-2` is
a ring, which is a box-shadow with no field on either side, **and**
`document.hasFocus()` is false and immovable in the harness, so the reference can
never produce it.

## 7. `oracleSurfaceScope`, and how the reference was taken

`scroll-area` needed an entry, and needed it more plainly than `popover` did: a
`ScrollArea`'s root contains **whatever the call site scrolls**, which for
`workspace-tree` and `git-panel` is a tree of `file-row-item`s and
`git-row-item`s — repeated ids, which v1.8 refuses outright. The entry is
`{ root: 'scroll-area-root', anchors: ['scroll-area-root', 'scroll-area-viewport'] }`,
derived from the component that builds the boxes rather than from a capture.

**How the attributes reached the live DOM, stated because it is a deviation.**
The intended route was to apply the two `data-oracle-*` edits to the running
worktree, let vite reload, capture and restore. Writing into that worktree is
**denied by this environment**, and it is the worktree several sessions share —
so the attributes were instead set on the live nodes with `setAttribute`, on
exactly the elements `ScrollAreaPrimitive.Root` and `.Viewport` render
(`[data-slot="scroll-area-viewport"]` and its `parentElement`), and removed
afterwards.

The measurement is unaffected and that is checkable rather than asserted: a
`data-*` attribute generates no box, no style and no layout, and every number in
the reference — `344 × 936`, `#00000000`, `radius 0`, `border.w 0` — was read
independently off the untagged DOM before the attributes existed and agrees to
the last decimal.

**Verdict: strict parity is reached on every field the contract carries.** No
property resisted. What the contract cannot see here is the mask, the scrollbar
fade and the focus ring, all §6, all recorded above.
