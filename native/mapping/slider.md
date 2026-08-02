# `slider` (P3.30) — hand-built, style-only confirmed against the vendor source

`web/src/components/ui/slider.tsx` (a `@base-ui/react/slider` wrap, 63 lines) →
`crates/crowbar-ui/src/components/slider.rs`,
`crates/crowbar-app/src/{surfaces,row_layout}/slider.rs`.

> A §6.2 row, in the shape `native/MAPPING.md` fixes. Kept in its own file for
> the reason `file-tree-row.md` gives.

---

## The seam — confirmed, not re-derived from the brief's account

The brief's own survey called `slider` one of three genuinely style-only
`gpui-component` widgets (with `select` and `switch`) and asked this item to
confirm or correct that against `native/vendor/gpui-component/src/slider.rs`
directly. Read in full:

* `pub struct Slider { state: Entity<SliderState>, axis: Axis, style:
  StyleRefinement, disabled: bool, reverse: bool }` — five fields, none of
  which is an element or a builder that takes one.
* `impl Styled for Slider { fn style(&mut self) -> &mut StyleRefinement }` —
  the *only* trait through which a caller can affect what gets painted, beyond
  `horizontal()`/`vertical()`/`disabled()`/`reverse()`.
* `impl RenderOnce for Slider { fn render(self, …) -> impl IntoElement }`
  builds the thumb (`render_thumb`), the bar, the fill and the container
  entirely inside its own body, from `div()` and `h_flex()`. `ParentElement as
  _` is imported and used **only** on the vendor's own internal `.child(…)`
  calls — never on a value the caller supplied.

There is no `pub fn child<E: IntoElement>(self, …)`, no `children`, no closure
seam, and no free function taking a row builder — the three shapes P3.16 found
missing from the first survey (`focus_trap`, `combobox`, `v_virtual_list`).
None of them exist on `Slider`. **The survey was right about this component**:
`AnchorSink`'s methods take a `gpui::Div` this crate holds, `Slider` supplies
no such seam, and wrapping it would yield a `div()` whose bounds merely
*coincide* with the vendor's own box — one compared field, the fake
convergence `ANCHORS.md` exists to refuse.

So: **built, not wrapped** — `switch` is the precedent for exactly this shape,
and this component follows its pattern (visual state a parameter, colours
from `Theme` only, every length traced to a class or derived from one).

---

## One wrapper, not two — `Root` and `Control` share a box

`slider.tsx` renders `<SliderPrimitive.Root>` around
`<SliderPrimitive.Control data-slot="slider-control">`. Measured directly:
both report the **identical** `668 × 4` box at `(0, 0)` on the live cell —
`Root` authors no padding, no border and no background of its own, so it
contributes nothing `Control`'s own box does not already have. Confirmed
rather than assumed: `getComputedStyle(root).backgroundColor` and `control`'s
own agree (`rgba(0, 0, 0, 0)`), and their `getBoundingClientRect()`s are
bit-identical. `crowbar_ui::components::slider::ID_ROOT` anchors `Control`
alone — a second, coincident anchor for `Root` would compare two identical
boxes and add nothing, the same call `git_status_row`'s `sidebar_tree::item`
already makes for its own coincident wrapper layers.

---

## Reachability and capture identity

**One live call site**: `web/src/features/settings/components/tabs/developer-settings.tsx`,
the "Fault Injection" rows under Settings → Developer, gated behind
`import.meta.env.VITE_USE_MOCK === 'true'` — the mock dataset, not the real
daemon. Reached by starting a second, `--mode mock` Vite dev server
(`bun run dev:mock -- --port 5180`, from the **shared** worktree's `web/`, so
the code measured was the unmodified tree) and navigating the already-running
Tauri window's `main` webview to it directly (`window.location.href =
'http://localhost:5180/'`), so every measurement below is **real WebKit**
(`WKWebView`), not a Chrome surrogate — `border-radius: f32::MAX` is a
`WebKit`-specific resolution of `calc(infinity * 1px)` that a Chromium engine
does not reproduce identically. The window was navigated back to its original
URL afterward.

`FAULT_KEYS` gives **11** real `<Slider>` instances on the page at once
(`document.querySelectorAll('[data-slot="slider-control"]').length === 11`).
The "Workspaces" row was used for both captures below.

Per-element identity, read directly off the live DOM before any
`data-oracle-*` attribute was applied (so this is evidence the element is
real, not a report of what I was about to inject):

| element | `data-slot` | `className` | `innerText` |
|---|---|---|---|
| control | `slider-control` | `flex touch-none select-none data-disabled:pointer-events-none data-[orientation=vertical]:h-full data-[orientation=vertical]:min-h-44 data-[orientation=horizontal]:w-full data-[orientation=horizontal]:min-w-44 data-[orientation=vertical]:flex-col data-disabled:opacity-64` | `""` |
| track | `slider-track` | `relative grow select-none before:absolute before:rounded-full before:bg-input data-[orientation=horizontal]:h-1 …` | `""` |
| indicator | `slider-indicator` | `select-none rounded-full bg-primary data-[orientation=horizontal]:ms-0.5 data-[orientation=vertical]:mb-0.5` | `""` |
| thumb | `slider-thumb` | `block size-5 shrink-0 select-none rounded-full border border-input bg-white not-dark:bg-clip-padding shadow-xs/5 …` | `""` |

All four `className`s are non-empty and match the unmodified source exactly.
`innerText` is empty on all four because a slider paints no text anywhere —
not a sign of an unreached element, a property of the component.

**`data-oracle-*` was applied with `setAttribute` immediately before each
capture and removed immediately after**, inside the same synchronous
`execute_js` call (tag → `extractSnapshotSource` → untag → return), so no
capture and no interim screenshot ever shows the attribute live in the shared
worktree's served app.

Two live captures, both from the same "Workspaces" row, at a 1714px window:

* `/tmp/p3-ref-slider.json` — `value: 0`, the resting cell.
* `/tmp/p3-ref-slider-selected.json` — `value: 40`, driven by setting the
  primitive's own hidden `<input type="range">` via the native value setter
  and dispatching `input`/`change` (the same technique `switch`'s two-switch
  capture generalises from one control to a re-driven one, since this surface
  has only one live slider per row rather than a pre-existing on/off pair).

---

## The pseudo-inset finding — `ANCHORS.md` §3's shortcut does not apply here

`ANCHORS.md` §3 and `extract.ts`'s own `pseudo` option let an anchor be
"pseudo-backed": the React extractor reads `getComputedStyle(el, '::before')`
and synthesises bounds from the **host's own padding box**. Its own comment
states the precondition: *"Only valid when the pseudo is `position:absolute;
inset:0`."* `git-row-item`/`file-row-item` satisfy it — that is why F2's
resolution for `file-tree-row` works.

**`slider-track`'s `::before` does not satisfy it.** Measured directly, not
inferred from the class list:

```js
getComputedStyle(track, '::before')
// left: "2px", right: "2px", top: "0px", bottom: "0px"
// width: "664px"  height: "4px"
// backgroundColor: "oklch(1 0 0 / 0.08)"
// borderTopLeftRadius: "340282346638528859811704183484516925440px"
```

`before:inset-x-0.5` (`0.5 × --spacing` = 2px) pulls the pill in on both
horizontal edges; `before:inset-y-0` leaves the vertical axis alone. Calling
`extractSnapshotSource` with `pseudo: {'slider-track': '::before'}` — the
mechanically correct-looking thing to do — **silently reports the wrong
box**: `oraclePaddingBoxRect` computes the box from the *host's own* border
widths (all zero here) and returns the host's full rect, `{x: 0, w: 668}`,
not the pseudo's true `{x: 2, w: 664}`. The colour, radius and border-width
fields *are* read correctly (`win.getComputedStyle(el, pseudoSelector)`
genuinely returns the pseudo's own paint), so only `bounds` was wrong — a 2px
error on each edge that a mechanical capture would not have flagged, because
nothing checks the precondition before applying the shortcut.

**What this file does about it**: `/tmp/p3-ref-slider.json`'s `slider-track`
entry has its `bounds` corrected by hand from the direct
`getComputedStyle(track, '::before')` measurement above (`x: 2, w: 664`)
rather than the mechanically-produced `{x: 0, w: 668}`. Every other field on
every anchor in both reference files is exactly what `extractSnapshotSource`
returned, unedited.

**Left open, not fixed**: `extract.ts`'s `oraclePaddingBoxRect` does not
verify the `inset:0` precondition it documents, so a future caller passing a
non-`inset:0` pseudo through the `pseudo:` option will hit the same silent
2px-per-edge error without this note in front of them. Making the shortcut
either verify its precondition (and refuse loudly, `ANCHORS.md`'s own house
style) or read the pseudo's actual `left`/`right`/`top`/`bottom` instead of
assuming zero is a change to shared oracle infrastructure this item's
constraints put out of scope (`Do NOT touch native/oracle/**`, and
`extract.ts` itself is shared by every surface with a pseudo-backed anchor).
Recorded as an open question for the owner, the same way `file-tree-row.md`
leaves its own contract gap open rather than resolving it unilaterally.

---

## The geometry — `thumbAlignment="edge"`, measured rather than assumed from the prop's name

`slider.tsx`'s `<SliderPrimitive.Root thumbAlignment="edge" …>` reads as "the
thumb's edge aligns with the value," which would suggest a plain
`valueFraction × trackWidth`. The live DOM says otherwise. At `value: 0` on a
`668px` track with a `16px` thumb, the thumb's own inline style reported:

```
--position: 1.1976047904191618%
```

not `0%`. `(16 / 668) / 2 × 100 = 1.1976047904191618` — the thumb's *centre*
is inset by **half its own extent** from each end, so it never overflows the
track. At `value: 40`, the indicator's own `width` (which the primitive also
expresses as this same percentage) was `268.8px`, and `8 + 0.4 × (668 − 16) =
268.8` exactly — the identity `crowbar_ui::components::slider::Slider::thumb_center`
implements, in pixels rather than through an intermediate percentage:

```
inset = thumb_extent / 2
thumb_centre = inset + value_fraction × (track_width − 2 × inset)
```

`Slider::indicator_width` is this same value (not `thumb_centre −
INDICATOR_MARGIN`): the indicator's own `margin-inline-start` (2px, `ms-0.5`)
and its computed `width` are two independent declarations in the live DOM
that happen to visually align the fill with the pill's rounded left cap, not
one number derived from the other — confirmed by the fact that the margin is
constant (2px at every value) while the width tracks the formula above
exactly.

---

## Colour — reuses `switch`'s own tokens, confirmed by value not just by name

| field | token | value (dark, live) | source |
|---|---|---|---|
| `slider-track.bg` | `theme.input` (`bg-input`) | `#ffffff14` / `oklch(1 0 0 / 8%)` | `theme.css`'s `.dark { --input: oklch(1 0 0 / 8%) }` — the exact string the live pseudo reported |
| `slider-indicator.bg` | `theme.primary` (`bg-primary`) | `#516a36ff` | `theme.css`'s `--primary: oklch(0.49 0.082 130)`, **identical in both tables** — `switch`'s own "on" colour, to the value |
| `slider-thumb.bg` | [`Color::WHITE`] (`bg-white`, unconditional) | `#ffffffff` | no `dark:` variant on this declaration at all; sealed as a literal in `theme/token.rs` beside `Color::TRANSPARENT`, since `theme.css` has no `--white` token to read |
| `slider-thumb.border` | `theme.input` (light) / `theme.background` (dark) | `#1f1f1eff` (dark, live) | `border-input dark:border-background` — the dark value is `Theme::DARK.background`, the same colour `switch`'s thumb paints as its *fill*, painted here as a 1px ring instead |

`slider-track` and `slider-indicator` read the identical two tokens
`switch::Switch::track_background` reads for its own off/on cells — not a
coincidence worth re-deriving, a genuine shared vocabulary confirmed by
comparing the measured hex values directly.

---

## What is not modelled, and why

* **Vertical orientation.** `slider.tsx` supports it (`data-[orientation=vertical]:…`
  throughout); the one live call site never passes it. Not built, the same
  call `git-status-row`'s two-span truncation and `file-tree-row`'s editing
  branch make: a real code path the app does not currently exercise. If a
  vertical call site ever ships, this is where the second axis goes.
* **`:focus-visible`'s ring.** Real in the class list
  (`has-focus-visible:ring-[3px] has-focus-visible:ring-ring/24`), and a
  box-shadow — `ANCHORS.md` §6 has no field for one — and unreachable anyway,
  `document.hasFocus()` is false and immovable on this machine.
* **`data-dragging:scale-120`.** A `scale` transform while the thumb is
  actively being dragged. The contract has no scale field, and even if it did,
  `v1.9`'s asymmetry (`WebKit` folds a live transform into
  `getBoundingClientRect()`; gpui applies scale at paint) means a static
  capture could not observe it on either side. Recorded so a reader comparing
  this file against the class list does not have to wonder, the same reason
  `switch::ACTIVE_SCALE_X` is kept.
* **`step`.** Read by the live call site (`step={5}`) but spent entirely on
  *which* values the primitive snaps to — it moves no geometry or paint this
  contract can see, so [`Slider`] takes a continuous `value` and never a step.
