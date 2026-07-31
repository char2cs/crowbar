# The anchor snapshot contract — v1.5

> **v1.5 — 2026-07-31. `content_sized`, and why it is a correction rather than a
> loosened tolerance.** Measured on the live gate pair, not inferred.
>
> **The observation.** GPUI `ceil()`s a text run's max-content width
> (`elements/text.rs`: `size.width = size.width.max(line_size.width).ceil()`)
> where Blink keeps fractional LayoutUnits. On the gate row both content-sized
> boxes came out **exactly** `ceil(reference)` — `74.11 → 75`, `11.16 → 12`.
>
> **Why not just widen the tolerance.** The error is strictly one-directional —
> `ceil` can only make native wider — so a symmetric tolerance spends half its
> slack on an error that cannot occur. And it is not actionable: if the engine
> ceils, it **cannot produce** a fractional content width, so "is native within
> ±0.5 of Blink's fraction" asks a question the engine is incapable of answering
> correctly.
>
> **The new field.** An anchor may carry `"content_sized": true`. For those,
> `bounds.w` compares against **`ceil(reference.w)`**, with the normal ±0.5
> around it — so a genuine sub-pixel width error is *still caught*, which is
> exactly what a looser tolerance would have given away.
>
> **It is DECLARED, never detected.** Detection is a heuristic on both sides
> (`width: auto` and not-a-stretched-flex-item in the DOM; `width: None` plus a
> text child in GPUI — both falsifiable by flex-grow). Two extractors each
> guessing is the silent divergence this file exists to prevent, and a mis-guess
> is invisible: it opens a blind spot or invents a delta and announces neither.
> Content-sizing is a property of the *component*, which already authors its own
> anchors on both sides, so it is an authored argument
> (`anchor_content_sized(…)` / `data-oracle-content-sized`).
>
> **The knock-on allowance, and why it needs no tree.** The ceil excess is
> **absorbed, not propagated**: measured, the flexible sibling shrank by exactly
> the summed excess (−1.73 against +0.89 +0.84) and the trailing group's right
> edge was **identical on both sides**. So the layout conserves the total. Every
> other geometry field in the same snapshot therefore gets an extra allowance of
> **Σ(ceil excess) over the anchors declared `content_sized` in that snapshot** —
> a single global scalar computed from the anchor list, needing **no flow order
> and no tree**, which keeps §1's rejection of tree-diffing intact.

# The anchor snapshot contract — v1.3

> **v1.3 — 2026-07-30.** Three fixes from the **React extractor** (P1.1) meeting
> the live DOM. One of them is a place where the contract as written would have
> made the two sides disagree on *exactly* the cases the field exists to catch.
> `schema` stays **1**.
>
> | # | Problem | Fix |
> |---|---|---|
> | 1 | **§3 told the DOM side to compute `clipped` as `scrollWidth > clientWidth`. That cannot work.** Both are **rounded integers**, so a 100.4px string in a 100px box reports a whole pixel of overflow that never paints an ellipsis. | `clipped` is computed from the **fractional** `text_width` against the content-box width, with a 0.5px epsilon; `scrollWidth`/`clientWidth` only as a fallback where there is no text node. The GPUI side already does the fractional comparison — had this stayed, the two would have disagreed on precisely the sub-pixel cases `clipped` was added to catch. |
> | 2 | `border.color` is junk when `border.w == 0` — computed style returns the *inherited text colour*, so a zero-width border reports a fully saturated colour. | **`border.color` is compared only when `w > 0`.** Extractors may emit whatever their engine gives; the differ ignores it below that threshold. |
> | 3 | v1.1 fixed the `theme` *vocabulary* but not how to **detect** it. This app sets `data-theme="crowbar"` **and** `class="dark"`. Reading the attribute first yields `"crowbar"`, which is out of vocabulary and refuses every comparison. | **Detection order:** the `dark`/`light` class on the root element → `data-theme` *only if* its value is literally `dark` or `light` → `color-scheme` → background luminance. The luminance fallback is what rescues a *named* theme; defaulting to `light` would be silent poison. |

# The anchor snapshot contract — v1.2

> **v1.2 — 2026-07-30.** Five rulings, all raised by the **GPUI extractor**
> (P1.2) once it was reading a real element tree rather than a spec. Two of them
> would have made *every* comparison fail. `schema` stays **1**.
>
> | # | Question | Ruling |
> |---|---|---|
> | 1 | Is "no flags" `[]` or `["empty"]`? | **`[]`.** `empty` is a *content* state in §8.3's matrix — a surface with nothing in it. The resting state is the empty list. Had the two extractors split on this, every comparison would have refused on a mismatched matrix cell. |
> | 2 | What exactly does `clipped` mean? | **A property of the anchored element itself**, horizontal only. An anchor that reports `clipped` **must sit on the element that truncates** — put it on an ancestor and the two sides measure different boxes. Vertical clipping is out of scope. |
> | 3 | Float precision | **Both sides round to 3 decimal places.** Three orders inside the ±0.5 tolerance, and it keeps snapshots diffable and stable. |
> | 4 | Gradient / pattern backgrounds | **Not representable in v1.** An anchored element must have a *solid* `bg`. The extractor emits no `bg` at all rather than a plausible substitute, so the differ rejects the document by name — a loud failure, which is correct. Added to §6. |
> | 5 | `font.family` — declared or resolved? | **Declared, first family only.** Neither engine can do better: `getComputedStyle().fontFamily` returns the specified list, and GPUI has no `FontId` → name reverse lookup. **Consequence: anchored text on the native side must name its family explicitly.** A style inheriting macOS's `.SystemUIFont` reports that literal string, which the DOM will never produce. |

## Known systematic differences — not noise, and not defects

Both are inside tolerance today. They are recorded because "inside tolerance"
and "the same" are different claims, and if either ever drifts out we should
recognise it rather than re-derive it.

- **`font.line_height` is device-pixel-snapped on the GPUI side.** GPUI applies
  `window.pixel_snap(...)`, so at 2× it lands on a multiple of 0.5px where CSS
  is continuous — e.g. `21.0` where CSS computes `21.034`. Within ±0.5, but it
  is a **bias in one direction**, not random.
- **`radius` and `border.w` report the top-left corner and the top edge only.**
  GPUI carries four of each and so does CSS. **Asymmetric corners or per-side
  borders are silently under-reported by both extractors.** If a component needs
  them, the contract must grow — do not assume the oracle is watching.

# The anchor snapshot contract — v1.1

> **v1.1 — 2026-07-30.** Ten ambiguities, all found by the differ implementation
> (P1.3) while building against v1. Every one of them is a place where the two
> extractors could have disagreed silently, which is the exact failure this file
> exists to prevent. `schema` stays **1** — the wire shape is unchanged; these
> are clarifications, not a format break.
>
> | # | Question | Resolution |
> |---|---|---|
> | 1 | `radius` / `border.w` had no tolerance | `radius` **±0.5**. `border.w` **exact** — ±0.5 on a 1.0px border is a 50% error and plainly visible. Loosen only with a measurement, per §5. |
> | 2 | `#rrggbbaa` or `#rrggbb`? | **Always 8 digits.** 6 is rejected. The only alpha to invent is `ff`, at which point "opaque" and "the extractor forgot alpha" become indistinguishable. |
> | 3 | `state` had no schema or vocabulary | Fixed below. **The vocabulary matters most**: one side emitting `"dark"` and the other `"Dark"` makes *every* comparison refuse. |
> | 4 | Must `root` be in `anchors`, at the origin? | **Yes, both — a load error.** An extractor emitting window coordinates would offset every anchor by a constant, which is precisely what §4 exists to prevent. Skipped when `anchors` is empty. |
> | 5 | Is the text group all-or-nothing? | A partial group is a **`FieldPresence` delta**, not a load error — visible and ranked, rather than swallowed or fatal. |
> | 6 | What does a consumer do with an unknown field? | **Hard failure**, document and anchor level. An extractor that ships a field early breaks the differ outright, which is the intended pressure. |
> | 7 | `font.weight` range | Accept **1–1000** (the CSS range), not 100–900. A legitimate variable-font weight must not fail to *load*; our own tokens stay on the 100s. |
> | 8 | `schema` negotiation | Reject anything but `1`. |
> | 9 | Nothing stopped a zero-anchor "pass" | **Refuse an empty comparison** (exit 2) unless explicitly asked for. "0 deltas over 0 anchors" reading as PASS is the cheapest possible fake convergence. |
> | 10 | `--report` argument shape | Exactly two paths: reference, then native. A corpus-directory sweep is a different shape and is deferred to Phase 5. |

## `state` — the fixed schema and vocabulary (v1.1)

Exactly these four keys, no more:

| Key | Type | Permitted values |
|---|---|---|
| `width` | integer | logical px |
| `theme` | string | `light` \| `dark` |
| `content` | string | `short` \| `normal` \| `overflow` |
| `flags` | string[] | subset of `empty`, `loading`, `error`, `hover`, `focus`, `selected` |

`flags` is a **set**: sorted on load, duplicates rejected. `["selected","hover"]`
and `["hover","selected"]` are the same matrix cell, and refusing to compare
them would be a false alarm that trains a reader to ignore refusals.

These values map 1:1 onto §8.3's matrix — ≥3 widths × light/dark × 3 content
lengths × the state flags. Lowercase throughout, no synonyms.

---

# The contract

Spec §8.1 (D8). **This file is the contract.** Three independent implementations
must agree on it byte-for-byte in meaning:

1. the React-side extractor (runs in the webview, walks `data-oracle-id`),
2. `crowbar-driver`'s extractor (walks the GPUI element tree post-layout),
3. the `oracle` differ (consumes two snapshots, emits ranked deltas).

If an implementation needs a field this file does not define, **the field is
added here first**, with a version bump, and all three are updated. A field that
exists in one extractor and not the other is invisible to the differ and is
therefore worse than useless — it looks like coverage and is not.

---

## 1. Why anchors and not trees

The draft proposed diffing layout trees. Rejected: the DOM and GPUI element
trees are **not isomorphic**. A sidebar row is six nested `div`s in one and one
element with three children in the other. A tree differ needs a node
correspondence function nobody has designed, and it would converge on one
component by hand-alignment while failing at scale — meaning the Phase 1 gate
could pass while telling us nothing.

Instead both apps **tag semantic anchors** and we compare per anchor only.
Nesting mismatch becomes irrelevant, and a delta is actionable by construction:
`git-row-icon.bounds.x: 12.0, expected 8.0`.

---

## 2. Snapshot shape

```json
{
  "schema": 1,
  "surface": "git-status-row",
  "state": { "width": 320, "theme": "dark", "content": "overflow",
             "flags": ["selected", "hover"] },
  "root": "git-row-item",
  "anchors": [ { /* §3 */ } ]
}
```

`surface` names the component under test. `state` is the §8.3 matrix cell that
produced this snapshot — it is **data, not decoration**: the differ refuses to
compare two snapshots whose `state` differs, because a delta between different
states is meaningless and would be the easiest possible way to fake convergence.

`root` names the anchor all geometry is relative to. See §4.

---

## 3. The anchor record

```json
{
  "id": "git-row-name",
  "bounds":  { "x": 30.0, "y": 4.0, "w": 118.0, "h": 16.0 },
  "fg":      "#c8ccd4ff",
  "bg":      "#00000000",
  "text":    "resolve-terminal-connection.ts",
  "text_width": 186.5,
  "clipped": true,
  "font":    { "size": 13.0, "weight": 500, "family": "Inter", "line_height": 17.55 },
  "visible": true,
  "radius":  2.0,
  "border":  { "w": 1.0, "color": "#00000000" }
}
```

| Field | Meaning | Required |
|---|---|---|
| `id` | Stable semantic name. Identical string on both sides. | yes |
| `bounds` | **Border box**, logical px, relative to `root` (§4). | yes |
| `fg` | Resolved text colour, `#rrggbbaa` sRGB. | if it paints text |
| `bg` | The element's **own** background paint, `#rrggbbaa`. Not composited with ancestors. | yes |
| `text` | Full string content, before any visual truncation. | if it paints text |
| `text_width` | Rendered advance width of `text` in px, **unclipped**. | if it paints text |
| `clipped` | Whether the text is visually truncated in this box. | if it paints text |
| `font` | `size` px, `weight` 100–900 numeric, `family` resolved first family, `line_height` px. | if it paints text |
| `visible` | Actually painted: not `display:none`, not `visibility:hidden`, non-zero area, not fully clipped by an ancestor. | yes |
| `radius` | Corner radius px. Single value; if corners differ, emit the top-left and note it. | no |
| `border` | Width px + colour. | no |

### An anchor may be **both** a painted box and a text run *(clarified v1.4)*

Nothing in this table forbids it, and the first real gate run hit it immediately:
`git-row-badge` is a rounded, tinted, bordered box whose content is the word
`uncommitted`. The React extractor emitted `bg`/`radius`/`border` **and**
`text`/`fg`/`font`/`text_width`/`clipped`; the GPUI extractor emitted only the
box, producing **five `FieldPresence` deltas on every cell of the matrix**.

That was a limitation of the driver's `anchor()` / `anchor_text()` split, not of
this contract. **Stated explicitly so no one re-derives it:** the box fields and
the text group are independent, an anchor may carry both, and an extractor that
can only produce one or the other must say so rather than silently dropping half.

### `text_width` and `clipped` are why the gate target was chosen

The Phase 1 gate is a row whose filename and directory spans **truncate against
each other** through three nested flex containers. `bounds` alone cannot catch a
wrong truncation point — two implementations can agree on the box and disagree
on where the ellipsis lands. `text_width` (the width the string *would* occupy)
plus `clipped` makes that difference visible.

DOM side: a `Range` over the text node → `getBoundingClientRect().width`, with
the clip detected by comparing `scrollWidth` against `clientWidth`.
GPUI side: the shaped line's advance width.

### Pseudo-element-backed anchors

**A CSS pseudo-element has no DOM node and cannot carry `data-oracle-id`.** This
is not hypothetical: on the gate target, *every* visible row background — hover,
active, selection — is painted by `.file-tree-item::before`, while the button
itself is pinned `background-color: transparent !important` in every state.

An anchor may therefore declare itself **pseudo-backed**. The React extractor
then reads `getComputedStyle(el, '::before')` — which does return the pseudo's
`background-color`, `border-radius` and box offsets — and synthesises `bounds`
from the host's padding box. This is valid for the gate target because the rule
is `position: absolute; inset: 0`; **an anchor whose pseudo is not `inset: 0`
must not use this shortcut** and needs its geometry derived properly.

The rejected alternative was injecting a real `<div>` into the React app under
an oracle build flag. It changes the app under test. Do not take it.

---

## 4. Coordinate space — relative to `root`, never absolute

All `bounds` are **logical pixels relative to the `root` anchor's top-left**,
with the root itself at `{x: 0, y: 0}`.

This is deliberate. The two apps do not share a window origin — the React app
draws inside a webview with its own chrome, the GPUI app draws its own — so
absolute window coordinates would differ by a constant on every anchor and drown
the real deltas. Anchoring to a designated root cancels that offset exactly.

Logical pixels, not device: both sides run at the same DPR, and comparing device
pixels would make the diff DPR-dependent for no gain.

---

## 5. Tolerances

| Field | Tolerance |
|---|---|
| `bounds` x/y/w/h | **±0.5 px** — sub-pixel layout legitimately differs between engines |
| `text_width` | **±1.0 px** — shaping and hinting differ |
| `fg` / `bg` / `border.color` | RGB **exact**; alpha **±1/255** for rounding |
| `font.size`, `line_height` | ±0.5 px |
| `font.weight` | exact |
| `text`, `clipped`, `visible` | exact |
| `radius` | ±0.5 px *(v1.1)* |
| `border.w` | **exact** *(v1.1)* — a ±0.5 tolerance on a 1.0px border is a 50% error |

> **Loosening a tolerance is a recordable event.** These are the *starting*
> values. Phase 1 may change them, but a loosened tolerance is the single
> cheapest way to make a gate pass while it tells you nothing — so any change
> lands in its own commit, states the measurement that justified it, and says
> what class of defect the looser value can no longer catch. Tightening needs no
> ceremony.

---

## 6. What this schema deliberately cannot express

Recorded plainly, because §8.2 requires honesty about it and because a reader
will otherwise assume the oracle covers more than it does.

- **Shadows, blur, `backdrop-filter`, vibrancy.** GPUI has no "computed style" —
  it has an already-resolved `Style`. These have no comparable representation.
- **Gradient and pattern fills** *(v1.2)*. `bg` is a single solid colour. An
  anchored element with a gradient gets **no `bg` emitted at all** — not `null`,
  not `#00000000` — so the differ rejects the document and names it. That is
  deliberate: a plausible substitute would compare as a real delta and send a
  reader hunting the wrong thing.
- **Runtime interaction state on the GPUI side** *(v1.2)*. The extractor reads
  each element's **base `StyleRefinement`** at prepaint. GPUI resolves
  `.hover(…)` / `.active(…)` from interaction state that exists only once a
  hitbox is hit, so a snapshot of a `.hover`-styled element reports its
  **resting** appearance. **Components under test must take their visual state
  as a prop** and fold it into the base style, or every hover/selected cell of
  the matrix silently compares resting-against-resting and converges while
  proving nothing. This mirrors the React original anyway, where the row
  background is painted by the *container's* `:hover` / `data-active`, not by
  the button's own interaction.
- **`visibility: hidden` on an *unanchored* ancestor.** Prepaint still runs, so
  a descendant reports `visible: true`. (`display: none` is caught implicitly —
  prepaint never arrives and the anchor is simply absent.)
- **Transitions and animation curves.** A snapshot is one instant.
- **Compositing.** `bg` is each element's own paint, not what the user sees
  after blending. Two different stacks can produce the same final pixel.
- **Paint order / `z`.** DOM stacking contexts and GPUI paint order are not
  isomorphic. Diffing an index would generate noise, not signal, so `z` is
  **not** a field. Occlusion bugs are not caught here.

These fall to the secondary perceptual pixel oracle (§8.2), whose output is a
**human-triaged score, not an agent gradient**.

**Say this plainly in any report: the primary oracle is a geometry-and-colour
oracle.** That is enough for the surfaces under strict parity and it is not
enough for shadows. Do not claim otherwise.
