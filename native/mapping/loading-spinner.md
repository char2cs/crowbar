# `loading-spinner` (P3.7)

`web/src/components/ui/loading-spinner.tsx` →
`crates/crowbar-ui/src/components/loading_spinner.rs`.

> A §6.2 row, in the shape `native/MAPPING.md` fixes. Kept in its own file
> because P3 runs several workers in parallel and one appended table is one
> conflict per item.

An `inline-flex` span holding a `<Spinner>` and, when `showLabel`, a muted
caption. Every "Compiles to" below came from the running app's own stylesheet,
read out of its CSSOM, and each is confirmed against the captured reference.

**Reference: `/tmp/p3-ref-loading-spinner.json`** — wrapper `138 × 18`, glyph
`16 × 16` at `y 1`, caption `116 × 18` at `x 22` reading `Loading commit diff`,
`fg #a4a4a4ff`, `CalSansUI` 12/18 at weight 400. Captured at a 1714px viewport,
dark, `content normal`, `flags []`.

**Live count: 1**, transient — the same instance the `spinner` reference came
from. See §4.

## 0. `ANCHORS.md` v1.9 reaches **one of three anchors**, not the surface

The check is per anchor, and on this surface the answer differs inside one
snapshot:

| anchor | animated? | why |
|---|---|---|
| `loading-spinner` | **no** | `transform` does not participate in layout, so the wrapper's border box is unmoved by the glyph spinning inside it |
| `spinner` | **yes** | `bounds` travel 6.63px on a 16px box — see `native/mapping/spinner.md` |
| `loading-spinner-label` | **no** | nothing on the caption animates |

The wrapper's immunity is a property of CSS rather than luck, and it is
measured: the reference's wrapper is 138 wide, and 138 = 16 + 6 + 116 with the
glyph's **layout** box in it. Had the transformed box participated, the sum
would have moved with the rotation.

The reference was nonetheless captured with the animation pinned at
`currentTime = 0`, for the one anchor that needs it.

## 1. Values

| React / Tailwind | Compiles to | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `inline-flex` on the wrapper | live computed `display: **flex**` — CSS blockifies a flex item, and every call site makes this one | `.flex()` | `bounds` |
| `items-center` | `align-items: center` | `.items_center()` | `bounds` — it is what puts the 16px glyph at `y 1` in an 18px line |
| `gap-1.5` | `calc(var(--spacing) * 1.5)` = **6px** | `GAP` | `bounds` — the caption's `x 22` = 16 + 6 |
| `gap-1` (`compact`) | **4px** | `GAP_COMPACT` | `bounds` |
| `size-4` / `size-3` on the `<Spinner>` | 16 / 12px | `spinner::CallSite::LoadingSpinner{,Compact}` | `bounds` |
| `ui-text-sm` on the caption | `font-size: var(--ui-text-sm)` = `calc(0.75rem * var(--app-ui-scale))` = **12px** | `Rems::from(theme.ui_text_sm)` — the **sealed token**, so `--app-ui-scale` still moves it | `font.size` |
| — (no `leading-*`) | preflight `html, :host { line-height: 1.5 }`, inherited unitless → **18px** at 12px | `relative(LINE_HEIGHT)`, `LINE_HEIGHT = 1.5` | `font.line_height`, and `bounds.h` |
| — (no `font-*`) | preflight leaves the document weight → **400** | `WEIGHT = FontWeight::NORMAL` | `font.weight` |
| `text-muted-foreground` | `var(--muted-foreground)` = `oklch(0.72 0 0)` = `#a4a4a4ff` | `theme.color_muted_foreground` | `fg` |
| the family | inherited `CalSansUI` | named explicitly, per v1.2 | `font.family` |
| — (no `border`, no `bg`, no `rounded`) | preflight's `border: 0 solid` | nothing | `border.w` **0**, `bg` `#00000000`, `radius` 0 |

## 2. `ui-text-sm` **wins** here — the mirror of `label`'s trap

`native/MAPPING.md` records that on `label` the primitive's `sm:text-sm/4` beats
a call site's `ui-text-sm`, so the rendered size is 14px and not the 12px the
class names. **The caption here carries the same `ui-text-sm` and resolves to
12px.**

The difference is not the utility, it is the competing declaration: nothing on
this caption carries a `sm:` variant, so there is no later layer to lose to.
Measured live — `font-size: 12px` — and the reference says `font.size 12`.

Which is why the trap is stated in `MAPPING.md` as *measure, do not infer*: the
same class resolves to two different numbers on two components, and reading
either one off the class list would have got the other wrong.

## 3. Layout constructs

| React / Tailwind | gpui / `crowbar-ui` | Note |
|---|---|---|
| `<span>` caption inside a flex row | a **block** `div`, gpui's default | Live computed `display: block` — CSS blockifies a flex item. Adding `.flex()` would be reproducing a resolution rather than a declaration |
| the caption's used width | taffy's flex-basis-auto base size | The span does not grow and does not stretch on the main axis, so its width is its run's max-content width. That is `content_sized`, §5 |
| the wrapper's used width | the sum of the row | `inline-flex` with no authored width inside a `flex-col items-center` parent (`CenteredState`), so it shrinks to fit |
| `gap` with one item | rendered anyway | CSS `gap` needs two items to show, so the icon-only cells carry it inertly — `label`'s `gap-2` has the same standing |
| the `aria-label` the glyph gets from the `label` prop | **absent** | Not a visual property, and no field records it. It is why `--call-site connecting-chip` passes a label that is never painted |

## 4. Reachability, measured

Four live call sites in two files:

| call site | shape |
|---|---|
| `review-diff-tab.tsx` — `<LoadingSpinner label="Loading commit diff" showLabel />` | **the captured cell** |
| `review-diff-tab.tsx` — `<LoadingSpinner label="Loading branch diff" showLabel />` | the branch scope of the same component |
| `review-diff-tab.tsx` — `<Suspense fallback={<CenteredState><LoadingSpinner /></CenteredState>}>` | icon-only, default label |
| `editor-status-actions.tsx` ×2 — `label="Connecting" compact`, and the same `showLabel` | the LSP status chip and its menu row |

**Live count 1, and it is transient.** `useReviewFilesSummary` has no cache and
resets `loaded` to `false` in the render where the scope changes, so *every*
`ReviewDiffTab` mount paints this for the duration of one HTTP round trip.
Clicking a commit in the git panel's **History** tab is the shortest path to it.
A `MutationObserver` captured it in the same task as the insertion.

**`--content` is deliberately not wired to the caption.** The four captions are
four `label` props at four call sites, so the string and the call site are one
quantity; a `--content` that swapped `Loading commit diff` for a longer string of
the surface's own invention would be painting text the product never shows.
`--call-site` moves the run's width instead, between strings that exist.

## 5. Declarations

`CONTENT_SIZED = [loading-spinner, loading-spinner-label]`,
`LINE_SIZED = [loading-spinner-label]`.

* **The caption is content-sized**: a non-growing, non-stretching flex item whose
  used width is its run's max-content width. The reference is the evidence —
  `text_width 115.99` in a box of `116`, which is `ceil` exactly.
* **The wrapper is content-sized**: `inline-flex` with no authored width, so its
  used width is `16 + 6 + ceil(run)`. `ceil(138.0)` is 138 either way on this
  cell, so the declaration is numerically free here and is made because it is the
  true shape — `label`'s standing.
* **The caption is line-sized**: nothing authors a height, so its box height *is*
  its line box. v1.6's test is "derived from", not "paints text" — `kbd` authors
  `h-5` around a 16px line and is therefore not line-sized, and this is the other
  side of that.
* **`spinner` is in neither list**, which is the control: its box is authored.

## 6. Traps

| Trap | What actually happens |
|---|---|
| **Declaring the wrapper `line_sized`.** | It paints no text of its own, so it carries no `font` — and v1.6 makes the declaration a **refusal** on an anchor without one, not a delta. Its 18px height comes from the flex line's cross size, which happens to be the caption's line box; that is a consequence, not a derivation |
| **Reading the caption's size off `label`'s finding.** | `ui-text-sm` is 12px here and dead at 14px there. §2 |
| **Modelling `compact` as a gap.** | It moves the gap **and** the glyph: 12 + 4 against 16 + 6. A port that carried only the gap across would put the caption 4px too far right on both LSP call sites |
| **Asserting the caption's advance width in `row_layout`.** | `add_fonts` is called by `main.rs` and not by the test harness, so a headless `gpui::test` shapes with a system fallback: this cell measures **136.8** where the reference is 115.99. `label` already recorded this; the harness asserts the box *is* `ceil(advance)` and leaves the run to the oracle |
| **Wrapping the caption in a second flex row.** | The DOM is `[svg, span]` under **one** span. An extra container gives the root anchor a box that is not the one the reference measures |
