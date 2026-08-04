# Blocked — `resizable` cannot be parity-run on this display

**Found:** 2026-07-31, Phase 2 · **Environmental, and now well-bounded.**
**Status:** needs a taller display, or a user decision on a measurement window.

Three separate blockers were hit and two were removed. This records the third,
because it is the one I cannot remove myself.

## The numbers

| | value |
|---|---|
| live IDE shell (`resize-group`) | **1119 px** |
| window height the driver must ask for | 1135 px + caption |
| drawable height this display grants | **1098 px** |
| tallest surface the driver can emit here | **1082 px** (bisected against the binary) |

Short by **37 px**.

## What was already fixed, so nobody redoes it

1. **A driver cap** (`--shell-height` capped at `1..=160`). Removed in P2.5; the
   window now follows the surface and the guard moved to `row_snapshot::emit`,
   which refuses any frame with an anchor below the area the platform actually
   granted. Verified: emits at 1082 with every anchor visible, refuses at 1083
   naming the exact number.
2. **The MCP bridge port**, held by a sibling session. Worked around: I built a
   bridge-free capture path — a local HTTP sink plus a temporary script injected
   via `index.html` — and captured the reference with it. That path is reusable
   and does not depend on a contended global port.

## Why I cannot close the last 37 px

- **The reference will not shrink.** `core:window:allow-set-size` is not granted
  to the page, so injected JS cannot resize the window. The MCP `manage_window
  resize` works only because the bridge plugin carries its own privileges, and
  the bridge is the thing that is unavailable. Relaunching does not help either:
  the window came back at 854×1119 rather than the configured 1200×800.
- **The driver will not grow.** 1098 px is what the platform grants a titled
  window on this screen.

## The two ways out — the second is a user decision

1. **A display with ≥1135 px of drawable height** — an external monitor, or a
   more-scaled built-in mode. Nothing in the repo changes; the run just works.
2. **A chrome-free full-screen measurement window in the driver.** P2.5
   considered this and deliberately declined: it is a second window mode whose
   blast radius includes **Phase 1's archived geometry**, which was captured in
   the current window. That is a real risk to the evidence the STOP gate rests
   on, and it is not mine to take unasked.

## What is NOT blocked

The component is built, gated and unit-tested, and the reference is captured and
preserved. Only the comparison is missing. Nothing else in Phase 2 depends on it.


---

## UNBLOCKED 2026-07-31 — `aerospace layout floating`

Kept above unedited; the wrong turn is the useful part.

The 37px shortfall was never a property of the display. **AeroSpace**, a tiling
window manager, was forcing the window to fill its workspace at 1714×1119. One
command released it:

```
aerospace layout floating --window-id <id>
```

The window immediately reverted to the app's own configured **1200×800**, and the
IDE shell with it:

```
resize-group          1200 × 800     ← under the 1082 ceiling
resize-panel-sidebar   294 × 800
resize-handle            1 × 800
resize-panel-content   905 × 800
flexGrow              24.521 / 75.478996
opacity chain above root: []          ← corpus/001 precondition holds
```

### What I got wrong, and it is the same mistake twice

This file says *"The reference will not shrink"* and *"the driver will not
grow"*, and concluded the platform was the binding constraint. The reference
shrinks fine. What would not shrink it was **the set of levers I had tried** —
`core:window:allow-set-position`, `manage_window resize`, AppleScript, a
synthetic drag — every one of which a tiling WM overrides anyway, because it
holds Accessibility and they do not.

That is the identical error as the `hover` note next door: I measured the levers
I knew about, found them all denied, and reported the *environment* as the
blocker. In both cases the answer was to **ask the window manager instead of
fighting it** — and in both cases the WM's presence was visible in my own data
long before I looked at it.

**Escalating to `blocked/` was still right.** Three attempts, no convergence,
move on — that rule exists so a stuck item stops consuming the run, and it did
its job. What was wrong was the *diagnosis* inside the note, not the decision to
file it.

### Standing note

`floating` is not sticky under a tiling WM. Re-assert it before any run that
depends on the window's natural size, and re-read the frame rather than assuming
it held.


---

## Not done yet — TWO STRUCTURAL blockers, and this time they are ours

The window is the right size now, the capture succeeded (81 anchors, opacity
guard clean, state held exactly). The diff still cannot run, for two reasons
that are **design constraints in our own tooling**, not the environment.

### 1. The oracle refuses the reference — correctly

```
/tmp/p2.10-ref-resizable.json is not a v1 snapshot: `anchors[13].id`:
anchor id `git-row-item` appears twice; the differ matches by id and would
have no way to say which of the two it compared
```

`extractSnapshot` walks **every** `data-oracle-id` under the root, not a
per-surface set. `resize-group` **is the IDE shell root**, so it swallows the
whole sidebar — carousel, file rows, ten git rows — giving 81 anchors with
repeated ids against the native surface's 4.

This is not specific to `resizable`. The carousel hit it too; I worked around it
there by hand-filtering to the `carousel-*` ids and saying so. That worked
because the carousel's own ids are unique — here they are not, and the oracle is
right to refuse rather than guess.

**A surface whose root contains other anchored subtrees cannot currently be
captured in isolation.** The fix belongs in the extractor: capture the surface's
own anchor set, not everything beneath its root.

### 2. The driver cannot render a full-bleed surface

```
--viewport-width 1200 is narrower than the 1200px surface plus its 24px insets;
the row would be cut at the window edge … Give it at least 1248
```

The refusal is sound — it is the same guard that stops `visible` becoming an
artefact of window size. But the real shell **fills its window**: surface width
*is* viewport width. Every gate surface so far has been a row narrower than its
viewport, so the 24px inset was free. It is not free for a surface that goes
edge to edge.

Widening to `--viewport-width 1248` would make `state.width` 1248 against the
reference's 1200, and the oracle would refuse — **as it should**; those are
different cells. Loosening that check to force this through is exactly the trick
this project does not do, and the breakpoint being the same at both widths is an
argument for fixing the driver, not for weakening the differ.

### Escalation status

Well past three attempts. It stays filed. But the diagnosis is now **ours to
fix** rather than the platform's to grant, which is a materially better place
than the last two versions of this note — both of which blamed the environment
and were wrong.

---

## 2026-08-04 — two dialog surfaces join this item, and one good result came out of it

### ✅ First: P3.74 is CONFIRMED WORKING, measured, not assumed

`repo-import-dialog`'s reference **captured cleanly for the first time ever** —
8 anchors, no refusal from the differ:

```
repo-import-dialog-popup        x=0    y=0     w=448  h=783.3
repo-import-dialog-header       x=1    y=1     w=446  h=72
repo-import-dialog-title        x=17   y=17    w=414  h=20
repo-import-dialog-description  x=17   y=45    w=414  h=20
input-control                   x=17   y=73    w=414  h=28
input                           x=18   y=74    w=412  h=30
button                          x=17   y=738.3 w=414  h=28
dialog-close                    x=407  y=9     w=32   h=32
```

Exactly **one `button` and one `dialog-close`** — the two anchors that used to
collide on the id `button` and made the differ refuse the reference outright.
That refusal is gone. The fix did what it claimed, and this is the measurement
that says so rather than the reasoning that predicted it.

### ⛔ But the native side of that cell cannot be driven here

```
crowbar-app: no snapshot: `repo-import-dialog-popup` reaches 867.5px down the
window but its drawable area is only 845px tall … This window holds a surface
up to 829px tall. Nothing was written.
```

`h-[70vh]` against the reference's 1119px window is 783.3px, and 783.3 + 84.2
of insets = 867.5 against 829 granted. **Short by 38px** — the same order as
`resizable`'s 37px, which is why this belongs here and not in its own file.

The arithmetic fix is a reference window ~55px shorter (`0.7H + 84 ≤ 829` needs
`H ≤ 1064`; the window is 1119). **I cannot make one.**

### ⛔ `detach-holder-modal`: opens fine, but its cell cannot be composed either

Reached by opening the store directly through Vite's dev module graph, which
works and is worth reusing:

```js
import('/src/features/window/stores/detach-modal-store.ts')
  .then(m => m.useDetachModalStore.getState().open({
    wsId: '…', branch: 'main', heldByPath: '…' }))
```

All five anchors render (`-popup`, `-header`, `-title`, `-description`,
`-footer`). It is `max-w-md` with **no `vh` sizing**, so it is not
height-blocked. It is blocked on **width**: the popup measured 458px in a 458px
viewport — viewport-clamped, so `max-w-md` (448) was never the binding
constraint and the cell is degenerate.

### ‼️ The tool that made this expensive: `manage_window resize` reports success while doing nothing

This cost more attempts than the blocker itself, so record it as an
instrument fault rather than an app fault:

| requested | actual | `success` |
|---|---|---|
| 1684 × 976 | 1684 × 976 | true — **worked** (first call after launch) |
| 1684 × 800 | 744 × 1119 | true — no-op |
| 1200 × 700 | 744 × 1119 | true — no-op |
| 1684 × 500 | 458 × 1119 | true — width moved, wrong value; height ignored |
| 1400 × 1000 | 316 × 1119 | true — **smaller than before**, after a fresh restart |

**`success: true` is not evidence the window resized.** Always read
`window.innerWidth`/`innerHeight` back from the webview afterwards. Height never
moved off 1119 at all (`screen.availHeight` is 1130, so the window sits at the
display's limit); width moved erratically and monotonically *downwards* across
restarts.

Direct `setSize` from inside the webview is refused outright, which is at least
honest:

```
window.set_size not allowed. Permissions associated with this command:
core:window:allow-set-size
```

So the capability is genuinely absent from the app's allow-list, and the bridge
appears to swallow the same denial and report success anyway.

### What would unblock all three

Any one of: a taller display; `core:window:allow-set-size` added to the dev
capability set (a one-line `desktop/src-tauri/capabilities/*.json` change,
**dev-only** — it must not ship in the packaged app); or a user decision to
accept a measurement window smaller than the live shell for `vh`-scaled cells,
with the cell recorded as such.

**This is a user decision, and it now gates three surfaces, not one.**
