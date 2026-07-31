# `crowbar-wordmark` (P3.8)

`web/src/components/ui/crowbar-wordmark.tsx` →
`crates/crowbar-ui/src/components/crowbar_wordmark.rs`.

> A §6.2 row, in the shape `native/MAPPING.md` fixes. Its own file for the same
> reason `crowbar-mark.md` is.

One `<svg>` with a `453 × 115` viewBox, thirty-one `<path>`s,
`fill="currentColor"` — and no class list of its own.

**Reference:** `/tmp/p3-ref-crowbar-wordmark.json`, captured live from the
new-tab pane's isologo at a 1714px viewport in a `1417 × 1073` pane.

## 0. The lettering is **not** a text run

The word "Crowbar" is path fill. `extract.ts` builds the text group from
`oracleOwnText(el)`, which walks the element's own child *text nodes*; an `<svg>`
has none. So `text`, `text_width`, `clipped`, `font` and `fg` are **absent** from
the reference — not empty, absent — and the comparison is:

```json
"bounds": { "x": 0, "y": 0, "w": 148, "h": 37.56 },
"bg": "#00000000",
"visible": true,
"radius": 0,
"border": { "w": 0, "color": "#ffffff0f" }
```

This is worth stating separately from `crowbar-mark`'s identical situation
because the wordmark *looks* like text. It is not, and no amount of convergence
here says anything about the letterforms. The port draws an empty box.

## 1. Values

| React / Tailwind | Compiles to | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `viewBox="0 0 453 115"` | not CSS; supplies the intrinsic **ratio** | `VIEW_BOX_WIDTH` / `VIEW_BOX_HEIGHT` | drives `bounds.h` |
| `h-auto` | `height: auto` → the ratio | `height_for(width)` | `bounds.h` |
| `w-[clamp(96px,14cqmin,148px)]` | `width: clamp(96px, 14cqmin, 148px)` | `CallSite::NewTabView::width(basis)` | `bounds.w` = 148 |
| `pointer-events-none` | not visual | — | not a field |
| `text-muted-foreground` | `color: var(--muted-foreground)` | not drawn | **invisible** |
| `aria-hidden="true"` | not visual | — | not a field |
| OOBE `w-[min(360px,44vw)]` | `width: min(360px, 44vw)` | `CallSite::OobePresentation` | unreachable, see §3 |
| OOBE `w-[min(180px,28vw)]` | `width: min(180px, 28vw)` | `CallSite::OobeCard` | unreachable, see §3 |
| OOBE `text-white` | `color: #fff` | not drawn | **invisible** |
| *(nothing)* | preflight's `border: 0 solid` | — | `border.w` = 0 |

`14cqmin` is measured against the nearest `container-type: size` ancestor —
`.pane-cq`, measured live at `1417 × 1073`, so `cqmin` is **1073** and the middle
term is `150.22`. **The clamp's ceiling binds**, at 148. `--pane-min` supplies
that container side; the port resolves the clamp itself, which is P3.1's line
held (the same *input* both engines resolve, never the reference's *output*).

## 2. ⚠ The height is quantised by **both** engines, and differently

The one derived number here, and the only place this component can drift.

| | captured cell |
|---|---|
| exact `148 × 115 / 453` | `37.5717` |
| **WebKit** floors into `1/64`px `LayoutUnit`s | **`37.5625`** = `2404/64` |
| **gpui** snaps to the device pixel grid (`0.5`px at DPR 2) | **`37.5`** |

Delta **`0.0625`** against `ANCHORS.md` §5's ±0.5. Measured in
`crates/crowbar-app/src/row_layout/crowbar_wordmark.rs`, not derived: the port
hands taffy the exact `f32` and reads `37.5` back.

**The bound is DPR-dependent and at DPR 1 it grazes the tolerance.**
`|snap(L) − L| ≤ 1/(2·dpr)` and `|L − floor₆₄(L)| ≤ 1/64`, so the worst case is
`1/(2·dpr) + 1/64` — `0.266` at DPR 2, `0.516` at DPR 1. The archived runs are
DPR 2 and the live clamp only ever produces 96 … 148, so nothing today is near
it. Recorded because it is a property of the two engines rather than a defect in
the port, and because the honest fixes are all worse: pinning the reference's
number in the component is what `ANCHORS.md` rejects by name, and adding a
declaration would make the differ compare something it recomputed — v1.6's own
objection, in a case where **both** sides transform.

## 3. Reachability — **1 live instance of 3 call sites**

| Call site | Live count | Why |
|---|---|---|
| `features/panes/components/new-tab-view.tsx` | **1** (with a new tab open) | the captured cell |
| `components/oobe/oobe-screen.tsx` presentation lockup | **0** | see below |
| `components/oobe/oobe-screen.tsx` card lockup | **0** | see below |

`oobe-screen.tsx` is the `/oobe` route's component, and the only navigation to
`/oobe` is `routes/_shell/index.tsx`'s `redirect` when the app has **no
projects**. The fixture workspace has projects, so the redirect never fires.

**There is a second wall behind that one and it is the more interesting.** Both
lockups sit inside `framer-motion` layers animating `opacity` from 0.
`ANCHORS.md` v1.7 makes a non-opaque ancestor a capture the driver cannot
reproduce, and `oracleAssertComparableOpacity` refuses such a document outright;
v1.9's hole — a snapshot cannot say *when* it was taken — applies to the rest of
the animation in full. So even a driven `/oobe` would need the transition
settled and the guard satisfied.

Both are **ported and neither is fabricated**: `git-row-dir`'s precedent, and
`crates/crowbar-app/src/row_layout/crowbar_wordmark.rs` lays both out so
"unreachable" means *no reference* rather than *no implementation*.

## 4. Declarations

| | Value | Why |
|---|---|---|
| `content_sized` | **false** | The width is a call site's `clamp()`, not a content measure |
| `line_sized` | **false** | No text, so no line box. The height comes from a replaced element's intrinsic ratio, which is a **third** way for a box to get a size and the first in this port — `ANCHORS.md` has no declaration for it and needs none: it is compared as an ordinary `bounds.h` |

## 5. What is **not** modelled

* **The lettering.** See §0.
* **The bare primitive** — no className at all. WebKit falls back to SVG's own
  `width: auto` and the 300 × 150 default object size constrained by the ratio;
  the port has no such fallback. No live call site renders it, so it is not a
  `CallSite` variant. §8.3's `empty` pins **zero** instead, which a call-site
  `size-0` produces and both engines agree has no area.
* **`--theme`, `--content`, `--width`, `--viewport-width`** — all vacuous, for
  `crowbar-mark`'s reasons. `--pane-min` is the one real axis, and it carries what
  `--viewport-width` would carry on the two OOBE cells.
