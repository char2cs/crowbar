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
