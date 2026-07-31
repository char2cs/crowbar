# corpus 001 — a non-opaque ancestor, and what `visible` must say about it

**Appended 2026-07-31.** Every entry here is a git-visible admission that a
defect escaped my side-by-side comparison. This one did.

## What escaped, and why

`visible` was compared on **every anchor of every cell** across the whole Phase 1
matrix — 18 resting cells plus 6 hover cells — and it agreed every time. It
agreed because no cell I drove ever had a translucent ancestor, so the field's
one genuine disagreement was never reachable from any sequence I ran.

The two extractors implement different fields:

- `oracleIsVisible` returns `false` for `opacity: 0` on the element **or any
  ancestor**;
- `crowbar-driver`'s `is_visible` has **no opacity term at all** —
  `grep -c opacity crates/crowbar-driver/src/` is `0`.

ANCHORS.md did not decide between them until **v1.7**, so neither side was wrong;
the contract was silent and the silence was invisible to a differ that only ever
saw opaque ancestors.

It was found by a worker **reading both implementations** while porting
`sidebar-carousel`, not by a run. My comparison could not have found it, and that
is precisely what this file is for.

## Why it is not hypothetical

`NavStack` puts `opacity-0 -translate-x-1/4 pointer-events-none` on exactly the
layer holding the sidebar carousel whenever a nav screen is pushed. A reference
captured in that state reports `visible: false` on **every** anchor while the
native side reports `true` — a delta on every anchor, caused by the contract
rather than by the port.

## The sequence that catches it

    surface:  sidebar-carousel
    state:    width 600, theme dark, content short, flags []
    setup:    push a nav screen, so the carousel's ancestor layer carries
              `opacity-0`  (equivalently: set opacity 0 on any ancestor of the
              root anchor)
    expect:   every anchor reports `visible: false` on BOTH sides

Before the v1.7 driver work lands, the native side reports `true` and the cell
fails on every anchor. That failure is the point: it is the regression test for
the fix.

## Standing restriction until the driver implements v1.7

**No cell may be driven with a non-opaque ancestor.** Any such run is measuring
the contract gap, not the port. This is recorded here rather than trusted to
memory, because the whole reason the defect survived is that nobody was watching
this field.

---

## Status update, 2026-07-31 — v1.7 landed, and the restriction NARROWS rather than lifts

`crowbar-driver` now implements the v1.7 opacity term. It is **not** a full fix,
and the difference matters for exactly the case that motivated this entry.

**What the driver now sees**

| | detected |
|---|---|
| the anchor's own `opacity` | **yes** |
| an **anchored** ancestor's `opacity` | **yes** — folded in on `enter`, restored on `leave` |
| an **unanchored** ancestor's `opacity` | **NO** |

The chain is accumulated at `AnchoredBox` boundaries only (`registry.enter(self.style.opacity)`),
so a plain `div().opacity(0.)` between two anchors is never entered.

**Verified by me, not taken from the worker's report.** A probe tree —
`anchor_root("root", div().child(div().opacity(0.).child(anchor("child", …))))` —
reports:

```
PROBE root  visible=true
PROBE child visible=true
```

The React side reports `false` for the same tree, because `oracleIsVisible` walks
**every** `parentElement` regardless of whether it is an anchor.

**Why this is the case that matters.** `NavStack`'s `opacity-0` layer is *not* an
anchor and sits *above* the root anchor. So the original motivating scenario is
still undetected. The gap has moved, not closed.

**The restriction therefore stands, narrowed:**

> No cell may be driven with a non-opaque **unanchored** ancestor. Such a run
> measures the contract gap, not the port.

**If you need to test opacity**, put it on an anchor — the root anchor will do,
since opacity moves no geometry and every bound is root-relative. Note this is
*not* the sequence written at the top of this file, which says "any ancestor of
the root anchor"; that sequence still fails today, and it is left standing rather
than edited to match what currently passes.

**Closing it properly** needs the driver to observe opacity it does not own.
gpui accumulates exactly this in `Window::element_opacity`, but it is
`pub(crate)` with no accessor, and `with_element_opacity` runs from
`Interactivity::paint` — after `prepaint`, where the extractor runs. Reaching it
means patching `native/vendor/`, which is pinned. Recorded as the known cost of
the current design rather than as a bug to be quietly lived with.
