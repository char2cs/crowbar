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

---

## Correction, 2026-08-02 — the pinned `selected` file is at the wrong cell

I re-ran the differ over **every archived matrix pair** at HEAD, which nobody had
done since the pairs were written. **16 of 17 PASS.** The seventeenth,
`native-file-tree-selected.json` against `ref-file-tree-selected.json`, **FAILS**:

```
file-row-name.fg: #f5f5f5ff, expected #fe9a00ff  (Δ b +245, rgb is exact)
oracle: FAIL — 1 delta over 6 anchors compared (1 colour)
```

**The claim above is not affected, and the cause is not a port defect.**
`#fe9a00` is the `modified` git-status colour. I swept all seven statuses: the
native side produces `#fe9a00ff` at `--git-status modified` — exactly the
reference — and the pinned snapshot was simply driven at the default
`--git-status none`. Re-driven at the right cell:

```
oracle: PASS — 0 deltas over 6 anchors compared, 1 forgiven by v1.5
```

archived as `oracle/runs/matrix/native-file-tree-selected-modified.json`. And the
`selected` claim itself stands, measured again today: `file-row-item.bg` is
`#ffffff0a` on **both** sides.

**What this does change is what the byte-identity guard has been proving.** That
file is quoted in every worker brief as Phase 1 evidence that a change moved no
capture, and it is checked by sha256. It is a **stability canary** — "this
capture did not move" — and it is *not* a convergence pair, because its cell and
its reference's cell differ. Every byte-identical check run against it, mine
included, has been confirming that a snapshot of the wrong cell stays put.

Left in place rather than regenerated, deliberately: two workers are in flight
checking that exact sha256, and yanking it mid-run would hand them a spurious
failure. **It should be regenerated at `--git-status modified` once they land**,
after which the canary and the evidence are the same file again.

The general lesson is the one worth keeping: **a pinned artefact proves only that
it has not changed.** Byte-identity says nothing about whether the thing it is
pinned against still agrees with it, and a pair can rot silently for as long as
nobody re-runs the comparison.

---

## §17 status, re-measured 2026-08-02

### Condition 3 — coverage, gate crates separately, NEVER blended

| crate | lines | missed | line % |
|---|---|---|---|
| `crowbar-proto` | 6 | 0 | 100.00% |
| `crowbar-client` | 452 | 8 | **98.23%** |
| `crowbar-core` | 169 | 0 | 100.00% |
| `crowbar-driver` | 2622 | 14 | **99.47%** |
| `crowbar-diff-logic` | — | — | **does not exist** (Phase 4+) |

**Oracle-corpus coverage, reported separately: 99.13%** (3776 lines, 33 missed).

All four existing gate crates clear the ≥98% bar. Two caveats travel with those
numbers and must not be dropped:

- **`crowbar-proto`'s 100% is structurally vacuous.** `rustc` excludes
  `#[automatically_derived]` items from instrumentation, so `llvm-cov` sees six
  lines against 24 files. It would read 100% with every DTO test deleted; the
  **259 round-trip tests** are the real assurance.
- **`crowbar-diff-logic` is absent, which is not a pass.** §17 names it; it is a
  Phase 4+ crate.

**A regression worth naming: `oracle` was 100.00% and is now 99.13%** — 33 lines
uncovered. The corpus suite has lost coverage as the differ grew. Not a §17 gate
failure (it is the separately-reported number), but it is moving the wrong way.

**`cargo llvm-cov` prints a blended `TOTAL` of 95.86%.** That figure is exactly
what §17 forbids reporting — it averages the gate crates with `crowbar-app`
(95.06%), `crowbar-ui` (96.49%) and `crowbar-platform` (73.87%), none of which
§17 names. **Never quote the TOTAL.**

## §17 status, measured 2026-07-31 — 3 of 9 met

Re-measured rather than recalled, because the coverage numbers had not been
taken since the crates grew.

| # | condition | status |
|---|---|---|
| 1 | every §5.1 anchor converges across the §8.3 matrix | **partial** — 9 surfaces verified, ~66 primitives + 36 layout files remain |
| 2 | every §5.2 surface judged against Zed | **not started** — editor, diff, terminal are behind the state model |
| 3 | both coverage numbers met and reported separately | ✅ **MET** |
| 4 | zero `unsafe` outside `crowbar-platform`, every block proved | ✅ **MET** |
| 5 | zero `unwrap`/`expect`/`todo!` outside tests | ✅ **MET** |
| 6 | leak soak shows no RSS growth vs React on one workload | **half** — detection armed; the soak has no workload until Phase 4 |
| 7 | `blocked/` empty, or every item a listed user decision | **5 items**, listed below |
| 8 | terminal conformance suite green | **not started** |
| 9 | a user cannot tell the two apps apart | **not close** |

### Condition 3 — both numbers, never blended

| crate | lines | missed | line % |
|---|---|---|---|
| `crowbar-proto` | 6 | 0 | 100.00% |
| `crowbar-client` | 277 | 1 | 99.64% |
| `crowbar-core` | 148 | 0 | 100.00% |
| `crowbar-driver` | 1491 | 26 | 98.26% |

**Oracle-corpus coverage, separately:** `oracle` 2815 lines, 0 missed,
**100.00%**.

Two caveats that must travel with those numbers rather than being dropped:

- **`crowbar-proto`'s 100% is structurally vacuous.** `rustc` excludes
  `#[automatically_derived]` items from instrumentation, so `llvm-cov` sees one
  file and six lines against 24 files, 1828 lines and 133 declarations. It would
  read 100% with every DTO test deleted. The **259 round-trip tests** are the
  real assurance.
- **`crowbar-diff-logic` does not exist.** §17 names it; it is a Phase 4+ crate,
  so there is nothing to measure yet — not a pass, an absence.

### Condition 4 — ⚠ now VACUOUS, and that is a weakening

**As of P3.40, `crowbar-platform` contains no `unsafe` at all.** Retiring
`crowbar-platform::native_menu` removed the last AppKit code; the only remaining
mention in the crate is `lib.rs`'s `#![deny(unsafe_op_in_unsafe_fn)]` attribute.

So §17 condition 4 — *"zero `unsafe` outside `crowbar-platform`, every block
proved"* — is now **trivially true**, and invariant **rule 3** ("every `unsafe`
in `crowbar-platform` carries a `# Safety` proof") **passes vacuously**.

**This must not be read as the guarantee it used to be.** Rule 3 was previously
mutation-tested by me: removing one `# Safety` heading from the AppKit code
failed the build. That evidence is now historical — there is no `unsafe` left
for it to bite on. A green rule 3 today proves only that there is nothing to
check.

**What to do when `unsafe` returns** (it will, the moment a platform surface
needs AppKit again): re-run the mutation before trusting the rule — delete one
`# Safety` heading and confirm the build fails. Do not infer from a green line.

### Condition 4 — verified, not asserted (historical, while AppKit code existed)

Rule 2 (14 crate roots) and rule 3 (`# Safety` on every `unsafe`) both pass, and
rule 3 is **not vacuous**: removing one `# Safety` heading from
`crowbar-platform`'s AppKit code fails the build. Mutation-tested by me.

### Condition 5 — verified adversarially

An injected `unwrap()` in non-test `crowbar-core` is a hard error, exit 101,
alongside pedantic's `must_use` and missing-`# Panics`.

### Condition 6 — the half that is done, and why the other half cannot start

Leak detection is armed on **all 142** `#[gpui::test]` tests and gated by rule 6,
with the gap it closes proved by mutation: the identical leak **fails** with the
guard and **passes green** without it.

The RSS soak has no workload. `crowbar-app`'s `main` parses one matrix cell,
renders one surface and exits — no workspace, no daemon connection, no
navigation. §17 asks for no growth *"on the same workload"*, and there is no
workload on the native side to hold against React. Measuring the harness's own
RSS and calling it a soak would be a manufactured green. It unblocks at Phase 4.

### Condition 7 — the blocked list

| item | needs |
|---|---|
| `s13-native-menus-accepted-delta.md` | **decided by the user**; the spec edit to §13/§5.1 is still owed |
| `resizable-needs-a-taller-display.md` | superseded — the surface is now verified; kept for the two wrong diagnoses it records |
| `hover-and-focus-need-an-unlocked-screen.md` | resolved — both are real observations now; kept for the same reason |
| `cla-policy.md` | a user decision: CLA, DCO, or nothing |
| `route-audit-red-at-head.md` | a user decision: two undeclared routes in `api/`, out of scope for this port |
| `vendored-crates-without-a-licence.md` | a user decision: confirm two Zed crates' licence, or accept both candidates are compatible |
