# `flicker-spinner` (P3.7)

`web/src/components/ui/flicker-spinner.tsx` →
`crates/crowbar-ui/src/components/flicker_spinner.rs`.

> A §6.2 row, in the shape `native/MAPPING.md` fixes. Kept in its own file
> because P3 runs several workers in parallel and one appended table is one
> conflict per item.

A `size-4` span that random-picks one of 31 build-time SVGs and inlines its
markup through `dangerouslySetInnerHTML`. Every "Compiles to" below came from
the running app's own stylesheet, read out of its CSSOM.

**Reference: `/tmp/p3-ref-flicker-spinner.json`** — `24 × 24`, `bg #00000000`,
`radius 0`, `border.w 0`, `visible: true`, and **no text group at all**.
Captured at a 1714px viewport, dark, `content normal`, `flags []`.

**Live count: 1**, and reaching it needed a live agent chat. See §4.

## 0. `ANCHORS.md` v1.9 does **not** reach this component — checked twice

The component is unmistakably animated, so v1.9 is the first thing to rule out.
Two independent facts do it, and both are measurements rather than readings:

1. **Every animation is on `fill-opacity`.** Counted over the whole asset
   directory: **775 `<animate>` elements across 31 files, and all 775 carry
   `attributeName="fill-opacity"`** — there is no second animated property to
   have missed. No field in §3 records it: `fg` and `bg` read `color` and
   `background-color`, and `visible` reads `opacity`, which is a different
   property on a different element.
2. **The animation is not on the anchored element at all.** The dots are
   unanchored `<circle>` descendants. `Element.getAnimations()` on the live
   anchored span returned **`[]`**, with `transform: none`. That is the sharpest
   possible contrast with `spinner`, where the identical call returned the `spin`
   animation and stepping its timeline moved `bounds` by 6.63px.

So a capture here is timing-independent in every recorded field, and would be
whether taken at rest or mid-flicker — the stronger statement P3.6 made about
`skeleton`, reached the same way and confirmed on the element rather than
inferred from the stylesheet.

**There is also no settling wait that could exist**, which is worth stating
because it is the fix a reader would reach for first: the `dur=` values are
twelve distinct numbers from **0.36s to 1.62s**, and the variant is picked at
random per instance. No wait puts every variant at a known phase. The
timing-independence argument has to be about the *property*, and it is.

## 1. Values

| React / Tailwind | Compiles to | gpui / `crowbar-ui` | Oracle |
|---|---|---|---|
| `inline-flex` | live computed `display: **flex**` — CSS blockifies a flex item, and the pane's container makes this one | `.flex()` | `bounds` |
| `size-4` (the primitive's) | `calc(var(--spacing) * 4)` = **16px** | `SIZE_4` | `bounds` |
| `size-6` (`agent-chat-pane.tsx`) | **24px** | `SIZE_6` — **the captured cell** | `bounds` |
| `size-3.5` (`workspace-branch-icon.tsx`) | **14px** | `SIZE_3_5` | `bounds` |
| `items-center justify-center` | | `.items_center().justify_center()` | `bounds`, of the SVG the port does not draw |
| `[&>svg]:size-full` | `width: 100%; height: 100%` on the child `<svg>` | **absent** — there is no child | invisible |
| `<animate attributeName="fill-opacity" calcMode="discrete" dur="0.36s…1.62s">` ×775 | SMIL, inside the inlined markup | **absent** | **no field** — see §0 |
| `fill="currentColor"` on 25 `<circle>` per asset | | **absent** | **no field** — see §2 |
| `text-foreground` (the pane's call site) | `var(--foreground)` = `oklch(97% 0 0)` | **not modelled** | **no field** — see §2 |
| — (no `border`, no `bg`, no `rounded`) | preflight's `border: 0 solid` | nothing | `border.w` **0**, `bg` `#00000000`, `radius` 0 |

## 2. The colour is **uncomparable**, which is what the primitive's own comment protects

The span has **no text nodes** — its only child is an `<svg>` written through
`dangerouslySetInnerHTML`. `extract.ts` builds `fg`/`text`/`text_width`/
`clipped`/`font` from `oracleOwnText(el)`, so the whole text group is skipped and
no `fg` is emitted. The dots' `fill="currentColor"` inherits a `text-*` token
from an ancestor, and **no field in the contract records either end of that
chain**.

The reference is the evidence: the anchor carries `bounds`, `bg`, `visible`,
`radius` and `border` and nothing else. Consequence: `--theme` is **vacuous**
here, and a port that got the dots' colour completely wrong would converge.

## 3. What is not modelled, and one of them is a real visual gap

**The dots are not drawn.** The port paints the box; it does not draw 25
flip-dots or flicker them. Like `skeleton`'s unswept sheen this is where the port
is visibly less than the original, and like it the contract records neither the
dots nor their opacity, so closing it would be unverifiable by the oracle either
way and it needs a frame loop this leaf has no other use for.

**Which spinner is drawn is random per instance.**
`SPINNERS[Math.floor(Math.random() * SPINNERS.length)]` over
`import.meta.glob('./spinners/*.svg')`. A component whose output is not a
function of its props would be a problem for any oracle; it is not a problem for
this one, because the choice only ever changes unanchored markup — every asset is
`viewBox="0 0 30 30"` and every one of them is scaled to the span by
`[&>svg]:size-full`.

## 4. Reachability, measured — and it needed a live agent

Three live call sites, all behind an agent:

| call site | shape | gate |
|---|---|---|
| `agent-chat-pane.tsx` | `size-6 text-foreground` | `attachment.state === 'reviving'` — a spawn **this pane asked for** is in flight |
| `agent-chat-glyph.tsx` | `size-4` | `working` — the chat's agent is mid-turn |
| `workspace-branch-icon.tsx` (`WorkspaceAgentSpinner`) | `size-3.5` inside its own `size-4` box | `workspace.working` — the daemon's agent overlay |

The last two are gated on a **turn**, which needs a message sent to a CLI, and
the bridge cannot type into an xterm. The first is gated on a **spawn**, which is
a click. So the reference was taken from `agent-chat-pane.tsx`:

1. create a chat from the chats panel's `New chat` button (data-only, through the
   app's own UI — `badge`'s precedent);
2. open its provider dropdown in the pane and pick the other provider.
   `handleSwitch` sets `{ state: 'reviving', message: 'Starting Codex…' }`
   **before** the request goes out, so the spinner is painted for the whole
   round trip.

**The chat was left in the fixture.** Deleting it removes the only route to a
`FlickerSpinner`, exactly as deleting P3.3's agent reply removes the only
capturable `Badge`.

## 5. Declarations

Both lists are **empty**.

* `content_sized` — the box is `size-4` on the primitive and a `size-*` at every
  call site; no text run decides it.
* `line_sized` — no text at all, so no line box.

## 6. Traps

| Trap | What actually happens |
|---|---|
| **Assuming v1.9 bites because it obviously animates.** | It does not, and the sibling `spinner` in the same item does — the answer is per component and comes from *which property*. Checking the animated property against the recorded fields is the whole method |
| **Waiting for the animation to settle.** | There is nothing to wait for: twelve loop lengths from 0.36s to 1.62s, picked at random per instance. §0 |
| **Reading `border` off `badge`.** | No `border` class, so preflight's `border: 0 solid` stands and `border.w` is **0** — `kbd`'s side of the trap. Compared exactly |
| **Expecting `text-foreground` to be compared.** | The span paints no text node, so no `fg` is emitted on either side. §2 |
| **Treating the empty-glob branch as §8.3's `empty`.** | `SPINNERS[…] ?? ''` really does render the span with no SVG in it, but the SVG is unanchored — so that cell compares **identical** to the resting one on every recorded field. A state flag whose cell cannot fail is what `Surface::unmodelled` exists to name, so `empty` is modelled as a `size-0` call site instead: zero area, `visible: false`, `skeleton`'s picture |
| **Expecting the `sm:` trap.** | Neither the primitive nor any of the three call sites carries a `sm:` variant, so tailwind-merge's last-wins is the whole story and the call site's `size-*` stands. **Checked**, not assumed — a `row_layout` test asserts every call site's box is unmoved across 639/1714 |
