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

### The port turns, and the two questions separate cleanly

An earlier draft of this port painted a **static box** and disclosed it as a
visual gap. Disclosing it was right; shipping it was not — §17's deliverable is
that a user cannot tell the two apps apart, and a spinner that does not spin
fails that at a glance, on the one component whose entire purpose is motion.
Two different things were being conflated:

* **what the app does** — it must turn;
* **what the harness measures** — one instant, and both sides must name the same
  one.

They separate for free, and for two independent reasons, both **measured**:

1. **`CROWBAR_ROW_SNAPSHOT` emits the first frame and quits**, and gpui's
   `AnimationElement` stamps `start = Instant::now()` on its first
   `request_layout`. So the native capture is at `delta ≈ 0` by construction.
   `row_layout::spinner::the_first_frame_is_the_turns_origin` drives the
   component's own `Spinner::turn()` and asserts the first delta is below 1e-3
   of a turn, with a control that later frames really do advance. Six
   consecutive `--surface spinner` emissions produced **one** distinct file.
2. **gpui rotates at paint time and never touches layout.** `Window::paint_path`
   tessellates into the scene; taffy is not involved. The driver records *layout*
   bounds at prepaint, so it reports the same 16 × 16 at **every** delta.
   `row_layout::spinner::the_recorded_box_never_moves_while_it_turns` steps eight
   animation frames and asserts the recorded box is unmoved.

**That is the asymmetry to carry forward.** WebKit's `getBoundingClientRect()`
returns the *transformed* box and travels; gpui's recorded bounds do not. So
pinning the **reference** at `currentTime = 0` is still necessary, and the native
side needs no pinning at all — it cannot get the instant wrong.

Confirmed end to end: the rotating port's emitted snapshot is **byte-identical**
to the static one's for both `--surface spinner` and `--surface loading-spinner`.
The turn changed no recorded field.

### The glyph is lucide's own arc, drawn on an unanchored child

This is the one place the port departs from the "icons are empty boxes" rule
`git_status_row` set, and the departure is narrow. That rule exists because a
call site *chooses* the icon, so a substitute would put a shape on screen for the
oracle to converge on. **`Spinner`'s glyph is not a choice** — `Loader2Icon` is
hardcoded in `spinner.tsx`, it is the whole component, and there is nothing for a
call site to vary.

It is drawn from lucide's path data rather than by eye, and the derivation is in
§1. It goes on an **unanchored** child, so the anchor's `bg`, `radius` and
`border.w` stay the reference's — the same standing `resizable`'s hit strip and
`button`'s `::before` overlay have.

`arc` swallows a tessellation failure, because a `paint` callback can do nothing
about one — and a swallowed failure would look exactly like the static box this
component used to paint. So `arc_path` is split out and unit-tested: it builds at
every call site's size and at eight instants round the turn, its bounds are the
right fraction of the box, and a whole turn returns the arc to where it started
while a quarter turn moves it.

### What is **not** verified: the pixels

Everything above the paint call is tested — `arc_path` builds at every size and
at eight instants, its bounds are the right fraction of the box, a quarter turn
moves its vertices and a whole turn returns them, frames are scheduled, and the
recorded box does not move. The one link left is `Window::paint_path` itself,
and **both routes to checking it are blocked, each tried rather than assumed**:

* `Window::render_to_image` exists under `test-support` but returns *"no
  HeadlessRenderer configured"* — `TestAppContext` passes no renderer factory,
  and the Metal one sits behind `gpui_macos`'s own `cfg(test)`, which this
  workspace cannot reach and `native/vendor/**` may not be edited to expose;
* `screencapture` is denied Screen Recording for this process (*"could not
  create image from rect"*), so the running binary cannot be photographed.

Recorded as a gap rather than glossed. A human running
`crowbar-app --surface spinner` closes it in a second, and that is the one check
worth asking for on this item.

### One stated difference, flagged rather than worked around

Under **reduced motion the two disagree, and it is the app that is right.**
gpui's `AnimationElement` renders a repeating animation at `delta 0.0` and
schedules no frames when `App::reduce_motion` is set — a blanket policy. The web
app's rule is not blanket; measured in the running app's CSSOM it is

```text
:not(.animate-spin, [data-essential-motion], [data-essential-motion] *) { animation-duration: 0.01ms !important; … }
```

— `.animate-spin` is **exempted by name**, a deliberate product decision that a
loading indicator is essential motion. Honouring it natively needs a
hand-written animation element, because `reduce_motion` is consulted inside
gpui's and there is no hook. Recorded rather than silently diverged from.

## 1. Values

| React / Tailwind | Compiles to | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `animate-spin` | `animation: var(--animate-spin)` = `spin 1s linear infinite`, `@keyframes spin { 100% { transform: rotate(360deg) } }` | `Spinner::turn()` — `Animation::new(PERIOD).repeat().with_easing(linear)`, driving `PathBuilder::rotate` once per turn | **`bounds`** — see §0. The only animation in this item that reaches a recorded field, and only on the **reference's** side |
| `size-4` (`loading-spinner.tsx`) | `calc(var(--spacing) * 4)` = **16px** | `SIZE_4` | `bounds` |
| `size-3` (`compact`) | **12px** | `SIZE_3` | `bounds` |
| lucide's `size` default | `width="24" height="24"` on the `<svg>` | `INTRINSIC_SIZE` | `bounds` — **dead**, both importers merge a size |
| `[&_svg:not([class*='size-'])]:size-4.5` + `sm:…:size-4` (the **button's**) | 18px below 640px, **16px** at or above | `CallSite::ButtonLoadingIndicator` | `bounds` — **dead**, see §3 |
| preflight `svg { display: block }` | | gpui's `Div` is already block | invisible |
| the arc `<path>` `M21 12a9 9 0 1 1-6.219-8.56`, `stroke-width="2"` | centre `(12,12)`, r `9`, start at angle 0, end at **287.998°**, `large-arc-flag 1` → the **major** arc, 288° | `GLYPH_RADIUS`/`GLYPH_STROKE`/`GLYPH_SWEEP_DEGREES`, as ratios of `GLYPH_VIEWBOX`; drawn with `PathBuilder::stroke` + `arc_to` on an **unanchored** child | **no field** — an unanchored child, see §0 |
| `stroke="currentColor"` | | `Window::text_style().color`, read at paint time | **no field.** See §2 |
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
| **Leaving the port static because the reference's box moves.** | The mistake this item was sent back for. The two questions are separate: the app must turn, and the *harness* must name one instant. gpui rotates at paint time, so the recorded bounds never move — pinning the reference is the whole fix, and the port pays nothing for turning |
| **Assuming a rotating port needs its own pinning.** | It does not, and it cannot get the instant wrong: the driver reads layout bounds and gpui's rotation never reaches taffy. The first-frame rule is belt and braces, and is asserted anyway |
| **Reading `border` off `badge`.** | `spinner.tsx` carries no `border` class, so preflight's `border: 0 solid` stands and `border.w` is **0** — `kbd`'s side of the trap, not `badge`'s. `border.w` is compared exactly |
| **Expecting a colour to be compared.** | There is no text node, so no `fg` is emitted on either side. `--theme` is vacuous here |
| **Assuming the bare primitive is 24px in a Button.** | It would be, but the button's `[&_svg:not([class*='size-'])]` rule catches exactly the className the loading indicator merges — so the live number would have been 16, not 24, had the path been live at all |
