# ~~Blocked~~ — `hover` and `focus` are not observable on the reference while the screen is locked

> ## ✅ RESOLVED 2026-07-31. Nothing here is blocked.
>
> Both cells were captured from the live reference and diffed by me — `hover`
> **PASS, 0 deltas over 8 anchors**, `focus` **PASS, 0 deltas over 8 anchors**.
> The full account, including three of my own conclusions that turned out to be
> wrong, is under **"RESOLVED 2026-07-31"** near the end of this file.
>
> **This banner exists because the resolution was 230 lines down and the title
> still said "Blocked".** On 2026-08-03 I classified this file for §17.7 by
> reading its first eight lines and recorded it as the one outstanding item
> standing between that condition and met. It had been done for three days.
> **A file's first screen has to carry its own verdict**, or the directory name
> becomes the only thing anyone reads.

**Found:** 2026-07-31, Phase 1, closing the §8.3 state axis · **Environmental, not a spec failure.**
**Status:** waits for a session with the screen unlocked. Everything needed to
finish it is written down below; it is about ten minutes of work once that holds.

## The finding

Two of the six §8.3 state flags cannot be *observed* on the reference app in this
environment. Not "are hard to drive" — the browser will not report them, because
both are conditioned on the application being active, and it cannot be.

Measured, not inferred:

```
CGSessionCopyCurrentDictionary → CGSSessionScreenIsLocked = 1
NSWorkspace.frontmostApplication → pid=406 loginwindow
webview → document.hasFocus() = false, document.visibilityState = "hidden"
```

### `hover`

CSS `:hover` is user-agent pointer state. A dispatched `MouseEvent` runs
listeners and changes nothing about which rules match, and WKWebView does not
speak the DevTools protocol, so Chrome's `CSS.forcePseudoState` is unavailable.
The only route is the real cursor — which is why `native/tools/refdrive/` exists.

The tool works. `CGWarpMouseCursorPosition` moves the cursor and the synthesized
`kCGEventMouseMoved` posts cleanly. What fails is delivery: macOS routes
`mouseMoved` only to the **active** application, and while the login window owns
the session no application is active. The page's `mousemove` listener recorded
nothing, and `document.querySelectorAll(':hover')` stayed empty with the cursor
verifiably parked on the row.

`NSRunningApplication.activate` returns `false`. That is not a permissions gap to
work around: a `tauri dev` binary has no bundle identifier, and macOS's
cooperative activation will not let a non-frontmost CLI hand activation to
anyone while the screen is locked. The AppleScript route is separately barred —
`osascript is not allowed assistive access (-1719)`.

### `focus`

This one is more interesting, and it is the reason this note exists rather than
a one-line "screen was locked".

`focus` looked drivable, and it half is:

```js
btn.focus()
document.activeElement === btn   // true
btn.matches(':focus')            // FALSE
btn.matches(':focus-visible')    // false
```

The element really is the active element. `:focus` still does not match, because
WebKit gates it on the document being focused, and `document.hasFocus()` is
`false` while the window cannot be key.

**This is the trap worth recording.** A driver that asserted on
`document.activeElement` — the obvious thing to assert on — would have reported
`focus` as converged, from an app painting no focus ring at all. The check that
catches it is `matches(':focus')`, never `activeElement`.

## A pre-check that survives the lock — and what it found

Observing the paint needs an active app. Reading what the stylesheet *declares*
does not. So while the gate measurement waits, I resolved the two rules through
the CSSOM on the live page. **This is not a substitute for the gate cell** — it
tests the stylesheet, not the paint, and it is recorded as a pre-check precisely
so nobody later mistakes it for a converged run.

It was worth doing. Both land exactly on the values the native app already paints:

| state | reference rule | resolves to | ×255 | native |
|---|---|---|---|---|
| hover | `.file-tree-item:hover::before { background: var(--file-tree-hover-bg) }`<br>`--file-tree-hover-bg: color-mix(in srgb, var(--accent) 68%, transparent)` | `color(srgb 1 1 1 / 0.0272)` | 6.94 → **7** | `#ffffff07` ✓ |
| focus | `.file-tree-container .file-tree-item button:focus-visible { border-color: color-mix(in srgb, var(--accent) 42%, var(--border)) }` | `color(srgb 1 1 1 / 0.0516)` | 13.16 → **13** | `#ffffff0d` ✓ |

`--accent` is `oklch(1 0 0 / 4%)` = `#ffffff0a`, which is independently the
`selected` value already confirmed by observation. The three states are one token
family, and the native side has all three right.

What remains genuinely unverified is therefore narrower than "hover and focus":
it is whether **geometry** shifts under those states, and whether anything else
on the row changes that the colour check cannot see.

### `:focus-visible`, not `:focus` — this changes the recipe

The focus rule is `:focus-visible`. That is a stricter matcher than `:focus`: it
fires on keyboard interaction, and a programmatic `btn.focus()` on a non-text
element generally does **not** satisfy the UA heuristic.

So the plan of "call `.focus()` and capture" would have failed **even with the
screen unlocked**, painting nothing and looking like a native-side defect. Focus
has to be driven the way a user reaches it — `Tab`, or the tree's own arrow-key
navigation — and then confirmed with `btn.matches(':focus-visible')`.

That is two independent reasons the naive focus driver reports a false result,
and only one of them is the locked screen.

## Approaches tried and measured to fail — do not retry these

Recorded so the next session spends its time on the one thing that works
(unlocking the screen) instead of re-deriving four dead ends.

| approach | result |
|---|---|
| `CGWarpMouseCursorPosition` + `CGEventPost(.cghidEventTap)` | event posts cleanly, delivered to nobody — the HID tap routes to the *active* app and there is none |
| `NSRunningApplication.activate` | returns **`false`**. A `tauri dev` binary has no bundle id, and cooperative activation will not let a non-frontmost CLI hand over activation while locked |
| AppleScript `System Events` (activate / set position) | `osascript is not allowed assistive access (-1719)` |
| **`CGEventPostToPid`** | posts without error and **still nothing** — `document.querySelectorAll(':hover').length === 0`, no `mousemove` listener fires. Addressing the process directly does not bypass the lock. |
| Tauri `setPosition` / `webview_interact` | `core:window:allow-set-position` is not in the app's capabilities; the interact tools are DOM-level and cannot set user-agent pointer state |
| bare `WKWebView` loading the same URL | the app cannot reach the daemon — there is no Vite proxy for `/v0`, and the browser transport resolves against `window.location.origin`. Building that plumbing would create a *different* reference than the shipping app |

The last row is the one worth understanding: it is not that it is hard, it is
that it would no longer be the reference.

## The residual is smaller than "two of six flags" — measured 2026-07-31

Checked live on the reference, because the earlier phrasing overstated what is
actually missing on the **gate surface** (`git-status-row`, the 9 anchors §16
names).

```
git-row-item : closest('.file-tree-container') === null , button outline: none
file-row-item: closest('.file-tree-container') !== null
hover rule   : ".file-tree-item:hover::before { background: var(--file-tree-hover-bg) }"
               — UNSCOPED, so it reaches the git panel too
```

Consequences, per flag, on the gate surface:

| flag | status on `git-status-row` |
|---|---|
| `empty` / `loading` / `error` | do not exist — container states, not row states |
| `focus` | **provably vacuous.** `:focus-visible` is scoped to `.file-tree-container`, the git row is not inside it, and its button is `outline: none`. Nothing can paint. Native already measured byte-identical to resting. |
| `selected` | vacuous here (no live consumer passes `active`) — which is why `file-tree-row` was built, and it **converged exactly** there |
| `hover` | **the one genuinely unobserved cell** |

And `hover` is not wide open either: the rule that governs it is unscoped, so the
same CSSOM resolution applies — `#ffffff07`, exactly what native paints. What
that cannot establish is the *paint*, and strictly the geometry under hover —
though the rule sets only a `background-color` on an `::before` that is already
`position:absolute; inset:0`, which cannot move a box.

So the honest residual is **one cell, whose colour is resolved and matching**,
not two flags wide open. It is still not an observation, and it is still not
counted as a pass.

## Why this does not void the Phase 1 gate

The STOP gate asks whether the driver and the anchored-geometry oracle *converge*
— whether the two implementations agree when pointed at the same state. It does
not ask whether every state is reachable on a locked machine.

Nothing here is a modelling failure, a tolerance that had to be loosened, or a
delta that could not be explained. The pipeline is sound and demonstrated: the
`selected` flag was driven end-to-end on this same surface and converged exactly
(`file-row-item.bg = #ffffff0a` on both sides), and driving it is *strictly
harder* than driving focus — it needed the two-call ordering, because
`highlightedPath` depends on `hasTreeFocus` and React must commit the focus state
before the click.

So the mechanism is proven. Two flags are unobserved, and they are unobserved for
a reason that has nothing to do with the design.

I am not adding these to §13's accepted deltas. That list is a user decision, and
"we could not look" is not a delta — it is a measurement that has not been taken.

## What is already established, so nobody re-does it

- 18/18 geometry cells pass on `git-status-row` (3 widths × light/dark × 3
  content lengths), each a live WKWebView capture against a live GPUI snapshot.
- `selected` converges exactly on `file-tree-row`.
- Native values are already measured on all four row states, so only the
  reference side is missing:

  | state | native `crowbar-app` |
  |---|---|
  | resting | `#00000000` |
  | selected | `#ffffff0a` ← reference confirmed identical |
  | hover | `#ffffff07` |
  | focus | `button.border #ffffff0d` |

## `empty` / `loading` / `error` — vacuous, and that is a finding

These three are **not** blocked; they do not exist on either gate surface, and
recording that is the honest close of the axis rather than a skip.

They are container states. A file tree can be empty, loading or errored; a *row*
is none of those, because a row that does not exist is not rendered. Neither
`git-status-row` nor `file-tree-row` has any code path that renders an empty,
loading or error variant, on either side. Driving them would mean inventing a
state the product does not have and then agreeing with myself about it, which is
worse than not testing — it manufactures a green cell.

If a later surface (the panel, not the row) does have them, they get tested
there, where they are real.

## To resolve

With the screen unlocked, and only my own app running:

```bash
# 1. the app must be genuinely active — verify, do not assume
swift native/tools/refdrive/refdrive.swift activate <pid>
swift /tmp/lock.swift          # expect CGSSessionScreenIsLocked absent or 0

# 2. calibrate: warp to a known screen point, read back what the page saw
#    (the page-side mousemove listener solves the offset in one sample —
#     do not compute it from the titlebar height)
swift native/tools/refdrive/refdrive.swift hover <winX+600> <winY+400>

# 3. only proceed if BOTH hold; otherwise the run is measuring nothing:
#      window.__cal        !== null
#      document.hasFocus() === true
```

Then capture `hover` and `focus` on `file-tree-row` and diff against
`--flags hover` / `--flags focus`.

One hazard, learned the hard way: sibling agent sessions run their own Crowbar
from a different worktree, and macOS cascades every fresh window to the same
origin — three windows sat at exactly `(262,122) 1200×800`. "The window at those
coordinates" is not a unique thing to point at. Confirm the target by matching
the process's `cwd` **and** `CROWBAR_HOME` to this worktree before driving it,
and confirm the pointer landed by checking that *this* page recorded the move.

---

## RESOLVED 2026-07-31 — both flags are now REAL OBSERVATIONS

Superseded. Kept in full, unedited above, because the reasoning that turned out
to be **wrong** is the most useful part of this file.

Both cells were captured from the live reference with the screen unlocked, and
diffed by me:

| cell | result |
|---|---|
| `git-status-row · 1714 · dark · short · hover` | **PASS — 0 deltas over 8 anchors** |
| `git-status-row · 1714 · dark · short · focus` | **PASS — 0 deltas over 8 anchors** |

`hover` is no longer the v1.7-era *construction*. The reference's real `:hover`
state, entered from a genuine mouse-moved event, converges exactly with
`--flags hover`.

### Three things above are wrong, and each cost real time

1. **"The tool works; what fails is delivery."** It did not work.
   `CGPreflightPostEventAccess()` is **false** on this machine: every
   `CGEventPost` and `CGEventPostToPid` was discarded silently, with no error and
   no failed return. Nothing synthetic ever reached the app — including the
   titlebar drag, whose failure I misattributed to a dead grab point (
   `elementFromPoint` returns the drag region at 940 of 966 sampled points,
   including the one tried).

2. **"`hoverCount: 22` proves hover works."** It proves nothing. That is the
   stale chain under wherever a human last left the mouse, and **it does not
   change when the cursor is warped**. I read a live-looking number and stopped
   asking. The correct check is that the chain *contains the intended row*, after
   clearing it to zero first.

3. **The window was never misplaced.** **AeroSpace**, a tiling WM, parks
   non-visible workspaces' windows at the bottom-right. The tell sat in my own
   data for hours: four *unrelated* processes — production Crowbar, the GPUI
   `crowbar-app`, and two dev builds — all at exactly `(1723, 1137)`. No app
   misplaces itself onto another app's pixel. `set-position` being absent from
   the capabilities was true and irrelevant.

### What actually works, needing no permission

Warp the cursor (ungated), then have the WM toggle the window fullscreen and
back. The frame change makes AppKit rebuild tracking areas and deliver a **real**
mouse-moved at the cursor's true position — nothing is synthesised. Controlled:
cursor on row 2 hovers row 2, cursor on row 4 hovers row 4.

### Two further retractions, both the worker's own

- **`NSRunningApplication.activate` returns `true` while the window stays
  non-key.** Activation makes the *application* frontmost; under a tiling WM the
  *WM* chooses the key window. A non-key window fails `:focus-visible` while
  `document.activeElement` still reports the button — so a focus capture taken
  there measures a **resting** row while declaring itself focused. That defect
  was caught by re-running, not by reading.
- **`:focus-visible` does not unconditionally match a programmatic `.focus()` in
  WebKit.** It matched only while the page had received no real user interaction;
  once genuine mouse moves arrived, WebKit correctly classified the modality as
  pointer and suppressed it. The deterministic driver is
  `btn.focus({ focusVisible: true })`.

### And the reference paints nothing for focus

With `:focus-visible` matching, computed `outline` is `none` on both the button
and the row, `box-shadow: none`, no ring. `focus` being pixel-identical to
resting is the reference's real behaviour, not a schema gap — which is what the
0-delta focus cell above actually confirms.
