# `repo-import-dialog` (P3.51)

`web/src/components/layout/repo-import-dialog.tsx` →
`crates/crowbar-ui/src/components/repo_import_dialog.rs`,
`crates/crowbar-app/src/surfaces/repo_import_dialog.rs`.

> Cluster 2, "standalone modals" (`native/mapping/layout-denominator.md` §8).
> Grouped with `detach-holder-modal` only by shape — neither imports the
> other.

**Reference: none.** `dialog.md` §5 names why: this call site's own trigger
was not identified live before that item's time budget ran out. This doc
records the surface and its `row_layout` coverage that already exist,
derived from what the code and its tests actually assert.

## 0. A call site of `dialog`'s primitive, with no footer at all

`repo-import-dialog.tsx` renders through the same `Dialog`/`DialogPopup`/
`DialogHeader`/`DialogTitle`/`DialogDescription` primitive `dialog.rs`
already wraps — see `detach-holder-modal.md` §0 for why the *real* DOM this
call site paints therefore carries `dialog-*` ids and why this module's own
ids differ anyway (the identical registry constraint). It never nests a
`DialogFooter`, so there is no footer concept anywhere in this module.

## 1. The one real shape this call site adds: a viewport-relative popup height

`DialogPopup className="flex h-[70vh] max-w-md flex-col p-0"`. `h-[70vh]`
sets the popup's own height to 70% of the *window*, not to the
header/body/footer sum `dialog::Dialog::popup_height` derives it as.
`RepoImportDialog::popup_height` **inverts** `dialog`'s own relationship: the
popup's height is the **input** (this component's own free variable), and
the space below the header is the **derived remainder**
(`RepoImportDialog::body_height`). `flex-col` (already `dialog`'s primitive
default) and `p-0` (a no-op — the primitive's popup carries no padding of its
own to zero) contribute nothing new.

## 2. Why this port does not read a live window's height, unlike its width

`dialog::Dialog::render` reproduces `w-full max-w-*` by reading
`window.viewport_size().width`, because `crowbar-app`'s row-layout harness
carries an **independent** `--viewport-width` axis. There is **no vertical
equivalent**: `Surface::min_window_height`/`SurfaceParams::driven_height`
make the window exactly as tall as the surface's *own* declared content
needs, so asking `window.viewport_size().height` from inside `render` would
close a loop this harness has no independent input to break — the window's
height is an **output** of what this struct declares, never an input to it.

So `h-70vh`'s live responsiveness is not reproduced against a real browser
window; `RepoImportDialog::popup_height` is a plain field, exactly
`dialog::Dialog::body_height` is one — "the port takes its measured extent,"
restated one level up because here it is the whole popup, not just its body,
that this call site's CSS makes exogenous to content. The default,
`popup_height_at(px(900.0))` = 630px, is a **stated assumption**, not a
measurement: no live window-height reference exists for any session that has
worked this component, and `native/mapping/command.md` records this
project's own machine's logical screen height at 982px — 900 is a
conservative, headroom-adjusted round number under that. The `row_layout`
tests sweep several assumed window heights rather than treating 900 as "the"
real one.

## 3. Values

| React / Tailwind | Compiles to | gpui / `crowbar-ui` | Notes |
|---|---|---|---|
| `max-w-md` (no `sm:` prefix) | 448px | `RepoImportDialog::max_width` | same numeric step as `dialog`'s and `detach-holder-modal`'s |
| `h-[70vh]` | 70% of the assumed window height | `popup_height_at` → `RepoImportDialog::popup_height` | §1–2 |
| `p-4 pb-2` (header) | top/left/right 16px, bottom 8px | `HEADER_PADDING` (16) / `HEADER_PADDING_BOTTOM` (8) | **not** `dialog`'s `p-6` (24) — this call site's own override, on the opposite side from `detach-holder-modal`'s `pr-10` |
| `gap-2` (header) | 8px | `HEADER_GAP` | identical to `dialog`'s, and live here — this call site always nests both a title and a description |
| `text-sm` (description) | line-height `1.25/0.875` | `DESCRIPTION_LINE_HEIGHT` | **`dialog`'s own default** — no `leading-*` override on this call site, unlike `detach-holder-modal`'s `leading-relaxed` |
| `p-4` (`DialogViewport`) | 16px | `VIEWPORT_PADDING` | `dialog`'s own |
| `border` | 1px | `BORDER_WIDTH` | `dialog`'s own |

## 4. The body is unmodelled, and more of it than `dialog`'s ever was

Between the header and the (absent) footer, the real call site renders a
search `Input`, a virtualized, network-fetched branch list, a hint paragraph
and an `Import` button — substantial, but every bit of it is this call
site's *own* content in exactly the sense `dialog.md` §3 and
`primitive-dialog-service.md` §4 both already establish for less content: the
port takes its measured extent rather than reproducing one of them.
Reproducing the list in particular would mean modelling network-fetched,
unbounded-count rows this port cannot pin to any reference regardless. A
future item that goes further would need its own `data-oracle-id`s on the
search input, the list viewport, the hint paragraph and the button — none of
which `repo-import-dialog.tsx` carries today — rather than folding the gap
silently into "body, measured."

## 5. Declarations

`CONTENT_SIZED = []`. `LINE_SIZED = [repo-import-dialog-title]` — the
identical arithmetic every dialog-shaped surface in this tree holds.

## 6. The state axis

| flag | here |
|---|---|
| `hover`, `focus`, `selected` | unmodelled — `grep -o 'hover:[a-z-]*\|focus:[a-z-]*\|active:[a-z-]*' repo-import-dialog.tsx` is empty |
| `loading`, `error` | unmodelled, as on every surface |
| `empty` | **real**: removes the header. No live call site takes this shape — the one reachable call site always renders both a title and a description. There is no footer to remove alongside it — this call site never renders one, unlike `dialog`'s and `detach-holder-modal`'s own `empty` arms |

## 7. `row_layout` coverage that already exists

`crates/crowbar-app/src/row_layout/repo_import_dialog.rs` drives the surface
in a real window and asserts, among other things:

* every contract anchor is present, no bare `dialog-*` id leaks through, and
  no id contains `"footer"` — this call site never renders one
* the popup is 448 × 630px (`max-w-md` × the 900px default's 70%), at the
  origin
* **`--window-height` moves the popup by exactly 70% of its own delta** —
  swept at 600/900/1200px. **Mutation:** replacing `VIEWPORT_HEIGHT_FACTOR`
  (0.7) with `1.0` turns this red (1200px would render an 1198px popup, not
  840)
* border/radius/background/text colour are `dialog`'s own tokens
* **`p-4 pb-2` genuinely differs from `dialog`'s own `p-6`** — the header's
  content column is 414px wide (`448 − 2 − 16(pl) − 16(pr)`), not the 400px
  `dialog`'s `p-6` would produce, and its top inset is 16px, not 24.
  **Mutation:** replacing the header's own `pl`/`pr`/`pb` with `dialog`'s
  `.p(px(24.0))` turns both assertions red (398 instead of 414; 24 instead
  of 16)
* the title is its own line box, painting `"Import branches"`
* **the description keeps `dialog`'s own default line height** — checked the
  font-metric-robust way: the observed height is a whole multiple of the
  *default* 20px line height, not the *overridden* 22.75px one
  `detach-holder-modal`'s own description uses
* **the header's own height is independent of `--window-height`** — the
  inverse of `dialog`'s relationship, where the popup's height is derived
  from the body. The test's own doc comment records a first-draft trap: an
  "expected" body height derived from the same three quantities the
  assertion then re-summed, which cannot fail regardless of what
  `body_height` actually computes; this test instead compares the header's
  own real rendered height directly. **Mutation:** hardcoding
  `header_height_estimate` to return a fixed 200px turns this red
* `empty` removes the header, and the popup's whole height stays the
  `--window-height`-driven 630px (there is no footer to keep it non-zero the
  way `detach-holder-modal`'s `empty` arm needs one)
* the light table paints a different popup

## 8. Reachability

`repo-section.tsx` is the one importer
(`native/mapping/layout-denominator.md` §2). Its own trigger — presumably a
button inside a repo's row — was not identified live in the session that
built the surface, so `dialog.md` §5's "not driven this pass" stands as the
current state; a future item that finds the live trigger should capture a
reference and confirm the 900px window-height assumption against it.
