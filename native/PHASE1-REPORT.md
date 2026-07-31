# Phase 1 report — the STOP gate

**Date:** 2026-07-31 · **Verdict: the gate holds. The spec is not void.**

Spec §16's gate asks one question: does a GPUI driver plus an anchored-geometry
oracle converge on a real row, across the §8.3 state matrix? If not, the spec is
void and Phase 2 does not start.

It converges. Every number below came from my own run — a live WKWebView capture
diffed against a live GPUI snapshot. No worker's claim is banked here.

## What converged

**The resting matrix: 18/18 cells** on `git-status-row` — 3 viewport widths
(600, 800, 1100) × light/dark × 3 content lengths (short, normal, overflow).

**The hover flag: 6/6 cells**, covering all three axes — 600/800/1100 in dark,
light at 800, and all three content lengths at 800.

**`selected`: converges exactly** on `file-tree-row`, `#ffffff0a` on both sides.

Truncation is genuinely exercised: `git-row-name` carries **476.49 px of text in
a 154.73 px box**, `clipped: true`, and converges there. A pass that only covered
short content would not be a pass, and this is not that.

The Σ ceil-excess allowance splits **1.51 px at 600** against **1.73 px at
800/1100**, tracking the badge's `sm:` breakpoint. That is the evidence the width
axis does real work rather than three cells that happen to agree.

## What is vacuous, and why saying so is the honest close

Three of the six §8.3 flags **do not exist** on either gate surface.
`empty`/`loading`/`error` are *container* states: a tree can be empty, a row
cannot, because a row that does not exist is not rendered. Driving them would
mean inventing a state the product does not have and then agreeing with myself
about it — a manufactured green cell, which is worse than an absent one.

`focus` is **provably vacuous on the gate surface**. Measured live:
`git-row-item.closest('.file-tree-container')` is `null` and the row's button is
`outline: none`, so the only focus rule in the stylesheet —
`.file-tree-container … button:focus-visible` — cannot reach it. Native measured
byte-identical to resting, as predicted.

`selected` is vacuous on `git-status-row` (no live consumer passes `active`),
which is why `file-tree-row` was built as a second, genuinely stateful surface
and the flag was closed there.

## The one construction, stated plainly

`hover` was **not observed**. It was reproduced.

CSS `:hover` is user-agent pointer state, and this machine's screen is locked
(`CGSSessionScreenIsLocked = 1`), so macOS delivers pointer input to no
application at all. Six approaches were tried and each measured to fail —
`CGEventPost`, `NSRunningApplication.activate`, AppleScript, **`CGEventPostToPid`**,
Tauri's own window APIs, and a bare `WKWebView` — all recorded in
`oracle/blocked/hover-and-focus-need-an-unlocked-screen.md` so nobody re-derives
them.

So the state was forced with a class carrying the **byte-identical** declaration
from `.file-tree-item:hover::before`. That is equivalent to hovering only if that
rule is *everything* hover does, so it was proved by exhaustion: of **127** hover
rules carrying declarations, with **0** unresolvable selectors, exactly **two**
reach anywhere in the git row subtree — the `::before` background, and a
`.file-tree-row:hover` rule setting the button background transparent, which is a
**no-op** because the button is already `#00000000` at rest.

**This does not establish that WebKit applies `:hover` on real pointer input.**
That is bedrock CSS behaviour rather than a Crowbar parity risk, but it is a
construction, not an observation, and it is labelled as such rather than counted
as one.

## What the gate does NOT prove — carried to Phase 2 verbatim

**The Phase 1 gate proves the mechanism on single-span truncation. It does not
prove two-span.**

`git-row-dir`, one of the nine anchors, **never renders in the live app**: the
only call site of `GitFileItem` passes `showDirectory={false}`. Forcing it would
render a configuration the app never produces — testing a dead code path and
saying nothing about parity with what users see. Two-span truncation is a harder
*layout*, not a different *mechanism*, and it becomes a Phase 2 component the
moment any surface enables `showDirectory`.

So: **8 of 9 anchors exercised**, the ninth absent from the product.

## Coverage at the gate, both numbers, never blended

| crate | lines | missed | line % |
|---|---|---|---|
| `crowbar-core` | 148 | 0 | 100.00% |
| `crowbar-client` | 277 | 1 | 99.64% |
| `crowbar-driver` | 1279 | 25 | 98.05% |
| `crowbar-proto` | 6 | 0 | 100.00% — **vacuous, see below** |

**Oracle-corpus coverage, reported separately:** `oracle` 2815 lines, 0 missed,
**100.00%**.

`crowbar-proto`'s number is meaningless and is not banked. `rustc` excludes
`#[automatically_derived]` items from instrumentation, so `llvm-cov` sees one
file and six lines against 24 files, 1828 lines and 133 declarations. It would
read 100% with every DTO test deleted. The **259 round-trip tests** are the real
assurance. `crowbar-diff-logic` does not exist yet.

## Standing quality bars, verified rather than assumed

- `check-invariants.sh` — **7 `ok` lines**, including the two added during this
  phase: rule 5 (`cargo fmt --check`) and rule 6 (every `#[gpui::test]` arms
  leak detection). Both were mutation-tested in **both** directions.
- Lint bars bite: an injected `unwrap()` in non-test code is a hard error, exit
  101, alongside pedantic's `must_use` and missing-`# Panics`.
- Leak detection is armed on all 51 gpui tests. The gap it closes was verified
  by mutation: the identical leak **fails** with the guard and **passes green**
  without it.
- 606 tests pass, 0 fail. `clippy --workspace --all-targets -D warnings` clean.

## Verdict

**Phase 1 converges. Proceed to Phase 2**, carrying forward:

1. two-span truncation (`git-row-dir`) as a Phase 2 component;
2. `hover` re-run as a real observation once the screen is unlocked — the recipe
   and its `:focus-visible` trap are in `blocked/`;
3. the §17 RSS soak, which needs a real workload and therefore a Phase 2 surface.

---

## Addendum, 2026-07-31 — `hover` is now an observation, not a construction

The report above says, in its own words: *"`hover` was **not observed**. It was
reproduced."* That caveat is now discharged, and the section is left standing
unedited so the difference is visible.

With the screen unlocked, both remaining flags were captured from the live
reference and diffed by me:

| cell | result |
|---|---|
| `git-status-row · 1714 · dark · short · **hover**` | **PASS — 0 deltas over 8 anchors** |
| `git-status-row · 1714 · dark · short · **focus**` | **PASS — 0 deltas over 8 anchors** |

The reference's real `:hover` state — entered from a genuine mouse-moved event,
confirmed to contain the intended row with the hover chain cleared to zero
first — converges exactly with `--flags hover`. The forgivenesses are the same
ones every resting cell earns (v1.5 content-sizing, v1.6 line-sizing); nothing
was loosened to make this pass.

So **every §8.3 flag on the gate surface is now settled by measurement**:
`hover` and `focus` observed, `selected` observed on `file-tree-row`,
`empty`/`loading`/`error` shown not to exist.

**What was wrong in the meantime is worth more than the result.** Three claims in
the original report's reasoning were false, and are corrected in
`oracle/blocked/hover-and-focus-need-an-unlocked-screen.md`: synthetic events
were never delivered at all (`CGPreflightPostEventAccess()` is `false`), a live
`:hover` count was stale state I mistook for proof, and the "misplaced" window
was a tiling window manager parking it — visible in my own data as four
*unrelated* processes sharing one pixel.

**The exhaustive-construction argument still stands on its own**, and is worth
keeping for the next surface that cannot be driven: of 127 hover rules carrying
declarations, exactly two reach the git row, and one is a no-op. That reasoning
predicted `#ffffff07`, and the live observation measured `#ffffff07`.

**Unchanged by this addendum:** the gate still proves the mechanism on
single-span truncation only, `git-row-dir` still never renders in the product,
and §17's RSS soak is still outstanding.
