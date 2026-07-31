# `crowbar-mark` (P3.8)

`web/src/components/ui/crowbar-mark.tsx` →
`crates/crowbar-ui/src/components/crowbar_mark.rs`.

> A §6.2 row, in the shape `native/MAPPING.md` fixes. Kept in its own file
> because P3 runs several workers in parallel and one appended table is one
> conflict per item.

One `<svg>` with a `146 × 145` viewBox, two `<path>`s, `fill="currentColor"` —
and **no class list at all**. There is nothing here to compile off a class name;
every number below is the call site's, measured on the running app.

**Reference:** `/tmp/p3-ref-crowbar-mark.json`, captured live from the tab bar's
`newTab` icon at a 1714px viewport.

## 0. The headline: the anchor pins a box, and the box is all there is

`native/oracle/ANCHORS.md` §3 records `bounds`, `bg`, `visible`, `radius` and
`border` for every anchor, plus a text group for an element with its own text
nodes. **An `<svg>`'s paint is not in that list**, and `<path>` is an element
rather than a text node, so `extract.ts`'s `oracleOwnText` returns `""` and no
`fg`, `text`, `text_width`, `clipped` or `font` is emitted.

The whole reference is therefore:

```json
"bounds": { "x": 0, "y": 0, "w": 18, "h": 18 },
"bg": "#00000000",
"visible": true,
"radius": 0,
"border": { "w": 0, "color": "#ffffff0f" }
```

Five fields. The ring, the crowbar glyph and their colour — the entire visible
content — are outside the contract, exactly as `resizable`'s hit strip and
`button`'s `::before` overlay are. The port draws an **empty box** of the right
extent, which is the call every component since `git_status_row` has made about
icons; a substitute shape would be something for the oracle to converge on that
neither engine is actually measuring.

**Say this in any report on this surface: a 0-delta run here proves the box is
in the right place at the right size, and says nothing about the picture.**

## 1. Values

| React / Tailwind | Compiles to | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `viewBox="0 0 146 145"` | not CSS | **not a constant** — the module docs, and §4 below | **invisible** — no field |
| `fill="currentColor"` | resolves the call site's `color` | not drawn | **invisible** — `fg` needs a text node |
| call site `size-[18px]` | `width: 18px; height: 18px` (arbitrary value, no `--spacing`) | `TAB_BAR_EXTENT` | `bounds.w`, `bounds.h` = 18 |
| call site `shrink-0` | `flex-shrink: 0` | `.flex_shrink_0()` | keeps 18 in a 14px slot |
| call site `text-muted-foreground` | `color: var(--muted-foreground)` = `oklch(0.72 0 0)` | not drawn | **invisible** |
| slot `grid size-3.5 place-content-center` | `14px`, centred | `CrowbarMark::slot()`, **unanchored** | not a field |
| *(nothing)* | preflight's `border: 0 solid` stands | — | `border.w` = 0, colour `#ffffff0f` never painted |
| *(nothing)* | no `background`, no `border-radius` | — | `bg` `#00000000`, `radius` 0 |

`border.w` is **0**, `kbd`'s finding a second time: `crowbar-mark.tsx` carries no
`border` class, so preflight's `border: 0 solid` is what renders. The colour is
`--color-border` resolved by the cascade and never painted;
`ANCHORS.md` v1.3 ruling 2 compares that field only above zero width.

## 2. Declarations

| | Value | Why |
|---|---|---|
| `content_sized` | **false** | The box is a pinned extent the call site names. Nothing here measures content — the mark *has* no content in the layout sense |
| `line_sized` | **false** | No text at all, so no line box. `ANCHORS.md` v1.6 makes the declaration valid only on an anchor carrying a `font` |

## 3. Reachability — **1 live instance**

`grep` finds exactly one importer of `<CrowbarMark>`:
`features/tabs/components/tab-bar-item.tsx`, behind `buffer.type === 'newTab'`.
A live count of `[data-oracle-id="crowbar-mark"]` was **0** in the resting IDE
and **1** after clicking the tab bar's `New tab` button.

One call site is also why the port has no `CallSite` vocabulary: a one-word enum
is a knob with nothing to choose. The 18px lives in the component, where it is
reviewable, rather than on a command line where it would hand the port the
reference's own output — P3.1's line for `--class-radius`.

## 4. The viewBox is `146 × 145` — **not** square, and not a constant

The one call site's box *is* square, so the art is letterboxed inside it by
`preserveAspectRatio`'s default. Nothing in the contract can see that and the
port draws no art, so the two numbers live here and in the module docs rather
than as `pub const`s: a constant nothing draws from can drift without anything
noticing, and a test that asserted it equalled its own literal would be the
vacuous guard the Wave 3 record warns about.

A reader who assumed a brand mark is square would derive a wrong height from a
width, which is why the numbers are written down at all.

## 5. The mark **overflows its slot**, on purpose

The tab bar's icon slot is `grid size-3.5 place-content-center` — 14px — and the
mark is 18. The call site's own comment says so: *"Deliberately LARGER than the
14px icon slot (it overflows the place-content-center box, which has no clip).
… Don't 'normalise' this back to size-3.5 — that regresses it."*

Measured live at `18 × 18` inside a `14 × 14` parent, `visible: true`, unclipped.
`--in-slot` renders the slot so a taffy layout has to reproduce it;
`crates/crowbar-app/src/row_layout/crowbar_mark.rs` asserts the extent survives.
The slot carries **no anchor**: `data-oracle-id` lives on the primitive, and an
id on a wrapper the primitive does not own is an anchor the React side has no
way to place.

## 6. What is **not** modelled

* **The art.** See §0.
* **The bare primitive** — an `<svg>` with a viewBox and no size. WebKit resolves
  that through SVG's own `width: auto`, answering with the 300 × 150 default
  object size constrained by the viewBox ratio; the port has no such fallback and
  a zero box would be a different picture. No live call site renders it, so the
  only sizeless cell modelled is §8.3's `empty`, which pins **zero** explicitly —
  a rendering a call-site `size-0` produces and both engines agree has no area.
* **Every §8.3 axis except `empty`.** `--theme`, `--content`, `--width` and
  `--viewport-width` are all vacuous here, and that is the finding rather than an
  omission: every field the contract records on this anchor is invariant under
  all four. This surface's whole matrix is one picture.
