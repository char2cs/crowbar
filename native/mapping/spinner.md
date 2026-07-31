# `spinner` (P3.7)

`web/src/components/ui/spinner.tsx` →
`crates/crowbar-ui/src/components/spinner.rs`.

> A §6.2 row, in the shape `native/MAPPING.md` fixes. Kept in its own file
> because P3 runs several workers in parallel and one appended table is one
> conflict per item.

lucide's `Loader2Icon` with `animate-spin` and whatever `className` a call site
merges. No variants, no library wrapper, one `cn()`. Every "Compiles to" below
came from the running app's own stylesheet, read out of its CSSOM.

**Reference: `/tmp/p3-ref-spinner.json`** — `16 × 16`, `bg #00000000`,
`radius 0`, `border.w 0`, `visible: true`, and **no text group at all**.
Captured at a 1714px viewport, dark, `content normal`, `flags []`.

**Live count: 1**, and it is transient. See §4.

## 0. The headline: `ANCHORS.md` v1.9 **bites this component**, and it is the first

P3.6 established the right check on `skeleton`: *which* property animates
against *which* the contract reads. Run here it gives the opposite answer.

`animate-spin` compiles to `animation: spin 1s linear infinite` over
`@keyframes spin { 100% { transform: rotate(360deg) } }`. The property in flight
is **`transform`** — and `transform` moves `getBoundingClientRect()`, which is
the exact call `oracleRelativeBounds` feeds. Measured on the live element by
stepping the CSS animation's own timeline with `Element.getAnimations()`, against
a layout box (`clientWidth`/`clientHeight`) that never leaves 16 × 16:

| t (ms) | rotation | `bounds.w` / `.h` | `bounds.x` |
|---|---|---|---|
| 0 | 0° | **16.000** | 936.500 |
| 62.5 | 22.5° | 20.905 | 934.047 |
| 125 | 45° | **22.627** | 933.186 |
| 187.5 | 67.5° | 20.905 | 934.047 |
| 250 | 90° | 16.000 | 936.500 |
| 500 | 180° | 16.000 | 936.500 |

So `bounds.w`, `bounds.h`, `bounds.x` **and** `bounds.y` are animated recorded
fields, by up to **6.63px** against §5's ±0.5px. Four instants in each 1000ms
are at rest and the other 996 are not: a capture taken without care is not
merely noisy, it is **wrong nearly always**.

**How the reference was taken.** The animation was pinned at its origin —
`animation.pause(); animation.currentTime = 0` — which is `rotate(0deg)`, so the
bounding box *is* the layout box. The captured `transform` is recorded as
`matrix(1, 0, 0, 1, 0, 0)`. That is stronger than "captured at rest" by luck:
the instant is chosen. **Anyone re-capturing this surface has to do the same**,
and a run that comes back with deltas on `x`/`y`/`w`/`h` should suspect the
instant before the port — which is exactly what v1.9 asks a reader to do.

**The port does not rotate**, deliberately. A rotating native side would put
*both* snapshots' bounds in flight and make two correct captures disagree by up
to 6.63px. §6 already says a snapshot is one instant; the one instant both sides
can name is `t = 0`. It is a real visual gap, in the same family as `skeleton`'s
unswept sheen — and unlike that one it is a gap the oracle *can* see, which is
why the rest instant is a rule here and a note there.

## 1. Values

| React / Tailwind | Compiles to | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `animate-spin` | `animation: var(--animate-spin)` = `spin 1s linear infinite`, `@keyframes spin { 100% { transform: rotate(360deg) } }` | `spinner::PERIOD` (the duration only); **not drawn** | **`bounds`** — see §0. The only animation in this item that reaches a recorded field |
| `size-4` (`loading-spinner.tsx`) | `calc(var(--spacing) * 4)` = **16px** | `SIZE_4` | `bounds` |
| `size-3` (`compact`) | **12px** | `SIZE_3` | `bounds` |
| lucide's `size` default | `width="24" height="24"` on the `<svg>` | `INTRINSIC_SIZE` | `bounds` — **dead**, both importers merge a size |
| `[&_svg:not([class*='size-'])]:size-4.5` + `sm:…:size-4` (the **button's**) | 18px below 640px, **16px** at or above | `CallSite::ButtonLoadingIndicator` | `bounds` — **dead**, see §3 |
| preflight `svg { display: block }` | | gpui's `Div` is already block | invisible |
| `stroke="currentColor"`, `stroke-width="2"`, the arc `<path>` | | **an empty box** — the `dropdown_menu` icon rule | **no field.** See §2 |
| — (no `border` class) | preflight's `border: 0 solid` stands | nothing | `border.w` = **0**, and `border.color` is ignored below that threshold (v1.3 ruling 2) |

## 2. The glyph's colour is **uncomparable**, and that is sharper than usual

The `<svg>` has **no text nodes**. `extract.ts` builds `fg`/`text`/`text_width`/
`clipped`/`font` from `oracleOwnText(el)`, which walks `el.childNodes` for text
nodes and finds `<path>` — so the whole text group is skipped and no `fg` is
emitted. `stroke="currentColor"` is the only colour the glyph has, and **no
field in the contract records it**.

The reference is the evidence: the anchor carries `bounds`, `bg`, `visible`,
`radius` and `border` and nothing else. Consequence: `--theme` is **vacuous** on
this surface, and a port that got the spinner's colour completely wrong would
converge.

This is the same class of blind spot `input` recorded for the `<input>` void
element, reached from the other direction: there the extractor could not see the
text because there was no text node to walk; here there is none because the
content is vector geometry.

## 3. Reachability, measured

`spinner.tsx` has exactly **two importers**, and only one of them is live.

| importer | live? |
|---|---|
| `loading-spinner.tsx` | **yes.** `size-4`, or `size-3` under `compact` |
| `button.tsx` (the `loading` indicator) | **no.** `loading` is never passed to a `<Button>` anywhere in `web/src` — the two greps that look like it are `disabled={… || loading}` |

The one live instance is transient: it is what a freshly-mounted `ReviewDiffTab`
paints while `useReviewFilesSummary`'s fetch is in flight. Reached by clicking a
commit in the git panel's **History** tab, which mounts a new tab with
`loaded === false`. A `MutationObserver` captured it in the same task as the
insertion, which is also why `animation.currentTime` read **0** before it was
pinned.

**The dead button path is modelled anyway**, because it is the one place in this
item where the `sm:` breakpoint moves a compared number: that call site's
className carries no `size-`, so the *button's* own descendant rule sizes it —
18px below 640px, 16px at or above. Its `absolute` is not modelled and needs no
excuse: on this surface the spinner **is** the root anchor, so §4 puts it at the
origin by construction and its position is not a compared field.

## 4. Declarations

Both lists are **empty**.

* `content_sized` — the box is authored by a `size-*` utility at every call site
  and by lucide's `width`/`height` attributes on the bare primitive. Neither is a
  text run's max-content width.
* `line_sized` — no text at all, so no line box. v1.6 makes the declaration valid
  only on an anchor carrying a `font`.

## 5. Traps

| Trap | What actually happens |
|---|---|
| **Capturing without pinning the timeline.** | The reference's `bounds` are the layout box at four instants per second and wrong at every other. A capture at 45° reports 22.627 against the port's 16 — four deltas on the surface's only anchor, none of them a port bug |
| **Rotating the port to "match".** | Both sides' bounds then depend on their own unrelated instants, and two *correct* snapshots disagree by up to 6.63px. The fix is to pin the reference, not to animate the port |
| **Reading `border` off `badge`.** | `spinner.tsx` carries no `border` class, so preflight's `border: 0 solid` stands and `border.w` is **0** — `kbd`'s side of the trap, not `badge`'s. `border.w` is compared exactly |
| **Expecting a colour to be compared.** | There is no text node, so no `fg` is emitted on either side. `--theme` is vacuous here |
| **Assuming the bare primitive is 24px in a Button.** | It would be, but the button's `[&_svg:not([class*='size-'])]` rule catches exactly the className the loading indicator merges — so the live number would have been 16, not 24, had the path been live at all |
