# Port Queue

Source of truth for the Rust-native GPUI port. Spec:
`docs/superpowers/specs/2026-07-30-rust-native-desktop-port-design.md`.
Updated every orchestrator iteration. This file is how a cold session picks up.

**Phase:** 3 — remainder **measured**, not estimated. **64 surfaces · 2230 tests ·
clippy 0 · 7/7 invariants**, all verified by my own run. Of the 64, **5 measure
dead code** (liveness audit + `sidebar-skeleton`) — so the honest figure is **59**,
and three of the five can never receive a verdict at all.

| tier | state |
|---|---|
| **Tier B · `components/ui`** | ✅ **done** — 43 surfaces, 1627 tests, clippy 0, 7/7 invariants, **no held verdicts**, every verdict taken by me |
| **Tier B · `components/layout`** | **22 of 23 targets** (P3.64 closed 3 defects + 3 fixture flags) — P3.61 (tree chain ×4) and P3.62 (last three) merged. **1 to go.** ⚠ built ≠ verified: **5 PASS** (`sidebar-project-header` 0/5, `context-pill` 0/2, `project-home-row` 0/5, `workspace-branch-icon` 0/1, `sidebar-carousel` 0/5), **4 FAIL** (`fps-overlay` — a **contract** gap; `repo-icon-popover` 36/6 — one missing wrapper; `repo-avatar` 4/1 — only 1 real; `project-switcher-panel` 5/5 — only 1 real), **1 REFUSED** (`repo-import-dialog` — duplicate `button` anchor id), the rest unverified |
| **Tier A · `crowbar-core`** | **two areas merged** — workspace scoping (P3.53) + git logic (P3.67). Coverage **100.00% over 1,435 lines**, up from 787. ⚠ 2 of the 6 git files measure **dead code** — my scoping error, see below |
| Tier A · `proto` / `client` | ✅ done (10,127 + 696 lines) |

Both denominators come from surveys committed this iteration
(`native/mapping/layout-denominator.md`, `native/mapping/tier-a-denominator.md`).
**Until 2026-08-03 this line said "Tier B complete", on the strength of a
denominator that covered one directory of a two-directory definition and left
Tier A's largest crate entirely uncounted.**
Phase 1 closed 2026-07-31 (gate passed, see [PHASE1-REPORT.md](PHASE1-REPORT.md));
Phase 2 closed 2026-07-31.

**Line coverage (logic crates) — re-measured by me 2026-08-03, all six §12 crates:**

| set | lines | missed | **cover** |
|---|---|---|---|
| `proto` + `client` + `core` + `diff` | 431 | 1 | **99.77%** |
| `oracle` + `driver` | 4,547 | 17 | **99.63%** |
| **combined** | **4,978** | **18** | **99.64%** — gate is ≥98, **met** |

The one file worth naming: **`crowbar-driver/src/frame.rs` is 94.77%** (8 of 153
lines missed) — the settled-frame fixpoint, and the least-covered file in the
gated set. That is the code the vendor-animation finding below turns out to lean
on, so its uncovered lines are not uninteresting ones. `leak.rs` is 97.83%.
Everything else in the set is 98–100%.

> **⚠ Read the percentage next to the crate size.** `crowbar-core` is at
> **100.00%** over **148 lines** — and its `Cargo.toml` says it holds *"all
> Crowbar domain logic"*. It holds a colour helper. `crowbar-diff` is 16 lines
> and `crowbar-proto`'s covered portion is 6. **A gate that passes over an
> almost-empty crate passes vacuously**, the same way §17.4's `unsafe` rule now
> does. The 99.64% is honest about what exists and says nothing about what is
> missing — see the Tier A finding below.

> The previous numbers here were measured against a **191-test** tree and had
> been quoted unchanged ever since. A coverage number nobody has re-run is not a
> coverage number.

**Corpus coverage (view crates) — the honest shape, measured 2026-08-03:**

| axis | state |
|---|---|
| **cells runnable** | **240 / 240** — all 40 surfaces × {800, 1200, 1714} × {light, dark}. 0 refused, 0 hung. Verified by me. |
| **cells with a reference** | **one** (`1714 · dark · normal`) for most surfaces, plus 4 light cells — of which only `sidebar-empty` carries information. |
| **viewport axis** | **inert on all 40 surfaces** — 800 and 1714 are byte-identical. Measuring it three times measures it once. |
| **theme axis** | **vacuous on 10 of 40** — every colour on every anchor is `#00000000`. |
| **state flags** | vacuous on the Phase 1 row; live elsewhere (`switch`, `checkbox`, `slider`). |

**So §17.1 is not met and the gap is references, not the native side.** See the
§17.1 section under *In flight* for the measurements behind every row above.

> **Two numbers, never combined** (§12). Anything that reports one blended
> figure here is wrong by construction.

## ✅ 2026-08-03 — the held-verdict backlog is EMPTY

Three surfaces had been carrying verdicts I could not take. All three are now
taken, green, and merged, and I re-took every one of them **again on the merged
tree** rather than trusting the pre-merge run:

| surface | verdict on the merged tree |
|---|---|
| `search` | **PASS** — 0 deltas over 10 anchors |
| `search-replace-row` | **PASS** — 0 deltas over 6, 5 forgiven by v1.5 content-sizing |
| `command` | **PASS** — 0 deltas over 11 anchors |

Merged tree, all measured by me: **clippy 0** · **1627 passed / 0 failed / 31
`ok`** · **7 of 7** invariant rules `ok` · both Phase 1 canaries
**byte-identical** · **258 of 258** matrix cells across **43 surfaces** render and
emit, 0 refused, 0 hung.

**The test-count arithmetic, because a drop needs explaining.** P3.44 reported
1609 on its own branch and the merged tree reports 1590 before P3.32 and 1627
after. P3.44 sat on a base predating P3.40, which deleted the 29 `native_menu`
tests; P3.42 added 10. `1609 − 29 + 10 = 1590`, then P3.32 brings it to 1627. A
suite that shrinks is either a deletion you can name or a failure you have not
found yet.

Merged this iteration: **P3.42** (recorder strictness), **P3.43** (§13 spec),
**P3.44** (search, carrying P3.37 and P3.33), **P3.32** (command, carrying
autocomplete).

## 🔧 P3.45 merged — and driving it live corrected its own recipe

The capture harness is committed (`native/scripts/gen-extract.ts`,
`native/oracle/README.md`, 22 tests; **363 files / 2703 tests, run completed**,
verified by me). Then I used it against the live app, which the worker was
forbidden to do — and that is where three things showed up that no unit test
could have.

#### 1. ‼️ The documented `--post` sink return path **does not work** in this webview

The app's origin is `http://localhost:5173`. A `fetch` to `http://127.0.0.1:8766`
fails with `Load failed` — **both GET and POST**, and there is **no CSP meta tag**,
so it is the webview's own origin scoping, not a policy we author. The sink can
never receive a POST from this page.

This is exactly the risk the brief named — *"demonstrate the reliability, do not
assert it"* — and it was undetectable from the worker's side because I told it not
to drive the app. **The gap was in my instructions, not its work.**

#### 2. There is a much better path, and it removes the drift risk entirely

```js
const m = await import('/src/lib/oracle/extract.ts')
m.extractSnapshot({ surface, root, state })
```

Vite serves the extractor **at the app's own origin** in dev, so the page imports
the **real module** — not a 30 KB stringified copy. The whole reason
`extractSnapshotSource` inlines its runtime is that nothing can be imported
inside the page; **in dev, that premise is false.** A missing helper is a
`ReferenceError` the injected path can only guard against with a round-trip test;
the imported path cannot drift at all, because there is only one copy.

Verified by me: `import(...)` resolved and exposed `extractSnapshot` and
`extractSnapshotSource`. Note the bridge does **not** await promises, so it is
two calls — arm, then read `window.__cap`.

#### 3. ‼️ A reference depends on app state the `state` block does not record

Capturing `sidebar-header` gave a snapshot **identical to the stored reference in
every field except one**:

```
sidebar-header.visible:  ref=true   mine=false      ← everything else byte-equal
```

Cause: there is exactly **one** `sidebar-header` in the DOM and it lives inside
`carousel-panel-files`, which was **two pages off-screen** — `x: 2058` in a 1714px
viewport. `oracleIsVisible` was right to say false. Setting the scrollport to 688
brought it to `x: 1370`, and the re-capture then matched the stored reference
**exactly**:

```
python3 … json.load(ref) == json.load(mine)   →   IDENTICAL
```

**The `state` block records the §8.3 cell — width, theme, content, flags — and
nothing about which carousel page is showing or which dialog is open.** So a
re-capture taken in the wrong app state differs by a single boolean and otherwise
looks perfect. That is precisely the shape that reads as a port defect. Every
reference re-capture has to drive app state first, and the drive is not written
down anywhere the snapshot can check.

**Consequence for the §17.1 push:** the references are recapturable, and I have
now done one end to end and reproduced it byte-for-byte. What each one needs is
its own app-state drive, and those need recording next to the surface.

### 🛑 AND TIER **A** IS BARELY STARTED — `crowbar-core` is 349 lines in 2 files

Having found the Tier B boundary wrong, I measured the other one rather than
assume it. §16 Phase 3 is *"Tier A (`core`, `proto`, `client`, theme tokens —
gated by ported tests) **and** Tier B"*. Raw `.rs` totals across the workspace:

| crate | lines | files | reading |
|---|---|---|---|
| `crowbar-proto` | 10,127 | 26 | ✅ generated DTOs, Phase 0 output |
| `crowbar-client` | 696 | 3 | ✅ socket + health |
| **`crowbar-core`** | **349** | **2** | ❌ **`color.rs` and `lib.rs`. That is all.** |
| `crowbar-diff` | 16 | 1 | stub |
| `crowbar-state` | 13 | 1 | stub — Phase 4 |
| `crowbar-editor` / `crowbar-terminal` / `crowbar-webview` | 9 / 12 / 11 | 1 each | stubs — §5.2, Phase 4+ |
| `crowbar-ui` | 33,630 | 51 | the component ports |
| `crowbar-app` | 32,689 | 98 | driver surfaces + `row_layout` tests |

`crowbar-core`'s own `Cargo.toml` describes it as **"all Crowbar domain logic:
git model, diff algebra, keymap resolution, settings schema, file-tree model,
workspace scoping, review threads."** It contains a `color-mix` helper. **None of
that domain logic is ported.**

**Its 100.00% coverage is therefore true and meaningless** — 148 covered lines of
a crate that should hold the model. This is the same trap as §17.4's vacuous
`unsafe` rule: a green number over an empty set. The header now says so.

**So Phase 3 is not "nearly done with a layout tail".** Tier A's largest member
is unported, Tier B's `layout` half is at 1 of 29, and the part that *is* finished
— `components/ui`, 43 surfaces, every verdict taken by me — is genuinely
finished. Three different states, and I had been reporting the third as the whole.

#### ✅ P3.49 — Tier A measured. **36 files · ~3,170 lines · 221 test cases.**

`native/mapping/tier-a-denominator.md` @ `9035d942`, merged `ce4dccbb`. Out of
~8,048 lines surveyed across §4.2's seven named areas. Committed in six
increments, which is why it survived where the sibling survey lost everything to
an API drop — that instruction is now standard in survey briefs.

**"Theme tokens" is not core work at all**, though §16 lists it beside
`core`/`proto`/`client`. §6.1's sealed token type is `pub struct
Color(gpui::Hsla)` — a literal `gpui` wrapper that `check-invariants.sh` rule 1
would reject inside `crowbar-core` — and §4.2 assigns token work to
`crowbar-ui`. What belongs in core is the gpui-free *arithmetic*, which is what
`color.rs`'s own module doc already says.

**Three more corrections, all verified against the tree rather than reasoned:**

- **§10.1 names `features/git/utils/git-diff-parser.ts`. It does not exist.** The
  daemon returns structured diffs; the one place raw patch text is parsed it is
  the third-party `@pierre/diffs`.
- **"Diff algebra" barely exists as a distinct area** — most of what looks like it
  is git-status classification, or `crowbar-diff`-crate windowing and search per
  §4.2, not core.
- **The seven areas are not one shape of work.** Git, file-tree and review threads
  mostly duplicate DTOs `crowbar-proto` already generated; keymap, settings,
  workspace scoping and theme tokens have **no daemon counterpart at all**.
- **The two most literal name-matches have zero dedicated tests** —
  `effective-keymaps.ts` and `workspace-scope-url.ts`, the latter used by *every
  scoped API call*. §16 gates Tier A on "ported tests"; for these two there is
  nothing to port.

##### ⚠ Its one overstatement, checked by me: `oklch` is not a gap

The survey called it *"a concrete, currently-real gap"* that `theme.css` uses
`oklch()` 37 times while `color.rs` implements only `color-mix()`. **Both counts
are right and the conclusion is wrong.** `crowbar-ui/src/theme/generated.rs` says
in its own header that every value was resolved the way a browser resolves it —
*"`oklch()` converted through `OKLab` to sRGB and then to gpui's `Hsla`"* — by
`tools/gen-theme.py`, **once, at generation time**.

The split is deliberate: **static** token resolution happens in the generator;
**runtime** mixing needs `color_mix` because `color-mix(in srgb, var(--accent)
68%, transparent)` is evaluated against values not known until then. A missing
runtime `oklch` would only matter if something computed one at runtime, and
nothing does.

Worth stating because it is the *inverse* of the day's other errors: I have been
finding green numbers over empty sets, and this was a red flag over a filled one.
**Both need the same check — read the absolute facts next to the claim.**

### 🛑 "TIER B COMPLETE" WAS WRONG — the `components/layout` half was never counted

**2026-08-03.** Spec §16 defines Phase 3 Tier B as *"the 46 `components/ui`
primitives **and 36 `components/layout` files**"*, and §5.1 puts both under strict
parity. **The denominator recorded in this file counts only `components/ui`** —
72 files, 43 real targets — and says nothing about `layout`. Every "Tier B
complete" claim in this file, including the one I wrote on 2026-08-02, covers
half the definition.

Measured just now, by resolving every `components/layout/*.tsx` against the
surface registry and `native/mapping/`:

| | |
|---|---|
| `components/layout/*.tsx` on disk | **29** — ~~§16 says 36; the spec's count is stale~~ **wrong, see P3.48 below: 29 `.tsx` + 7 `.ts` = 36 exactly. The spec was right and I speculated.** |
| ported — has a registered surface | **1** (`sidebar-carousel`) |
| **neither a surface nor a mapping doc** | **28** |

```
connection-indicator · context-pill · detach-holder-modal · drag-ghost ·
fps-overlay · ide-shell · nav-stack · pending-create-row ·
placeholder-row-actions · placeholder-toast-watcher · project-home-row ·
project-switcher-panel · repo-avatar · repo-icon-popover · repo-import-dialog ·
repo-section · sidebar-peek · sidebar-project-header · sidebar-skeleton ·
sidebar-tab-bar · sidebar-toast-overlay · workspace-branch-icon ·
workspace-inline-input · workspace-switcher · workspace-tree-context ·
workspace-tree-footer · workspace-tree-item · workspace-tree
```

**So Phase 3 is roughly half done, not done.** The `components/ui` half is
genuinely complete and its verdicts stand; the `layout` half is at 1 of 29.

**How this got past me:** the Tier B denominator was measured carefully — the
Plate-by-filename rule, the three Plate-by-dependency corrections, `separator`'s
out-of-scope note — and *all of that care went into one directory*. A denominator
that is rigorous about the files it counts is still wrong if it counts the wrong
set, and being able to recite `72 → 26 → 3 → 43` made it feel settled. **I
checked my arithmetic and never re-checked my scope**, which is the same
failure as reading the head of a 300-line file: precision inside a boundary
nobody re-derived.

**Not all 28 are necessarily port targets.** Several look like Phase 4 or Phase 5
work by their names alone (`placeholder-toast-watcher` is a watcher,
`drag-ghost` is interaction, `fps-overlay` may be dev-only), and §3.2's Plate
exclusion has to be applied here as it was to `ui/`. **That classification is the
next item and it is a measurement, not a guess** — dispatched as P3.48. The 28
above is the *unclassified* count, not the port target.

#### ✅ P3.48 — the layout denominator, measured. **22 remaining, not 28.**

`native/mapping/layout-denominator.md` @ `c1df0dd1`. All 29 classified with
evidence:

| verdict | n | |
|---|---|---|
| **Tier B target** | **22** | the real remainder |
| already ported | 1 | `sidebar-carousel` (the control) |
| Phase 4 — state | 2 | `connection-indicator`, `placeholder-toast-watcher` — **both render `null`** |
| Phase 5 — interaction | 2 | `drag-ghost`, `workspace-tree-footer` |
| Phase 4/6 — app shell | 1 | `ide-shell` |
| Phase 4/5, no geometry | 1 | `workspace-tree-context` |
| dead · Plate-only · other out-of-scope | **0** | — |

**So the layout tier is 23 targets, 1 done, 22 to go.**

##### ‼️ My "the spec's count is stale" was wrong — and the spec was right

I wrote above that §16's **36** disagreed with the 29 on disk because the tree
had moved. It has not. **`components/layout/` holds 29 `.tsx` *and* 7 `.ts`
helpers — exactly 36.** The spec counted both extensions all along; framing the
survey `.tsx`-only was my choice, matching `ui/`'s convention, and the arithmetic
followed from that choice rather than from drift. **I explained away a
discrepancy I had not investigated**, which is the third boundary error in one
day and the least excusable, because the check is `find … -name '*.ts' | wc -l`.

Those 7 `.ts` files are not nothing: `workspace-row-base.ts`,
`workspace-tree-utils.ts`, `context-pill-model.ts`, `workspace-switcher-model.ts`,
`format-change-count.ts`, `workspace-tree-actions.ts`, `use-sidebar-panel.ts` —
several are exactly the gpui-free model logic **Tier A**'s `crowbar-core` is
supposed to hold. They belong in the Tier A survey's denominator, not this one.

##### ‼️ `toast.rs` ported a component with no code path — and the live one is unported

The survey checked "already covered" against **Rust source rather than names**,
and caught two traps:

- **`sidebar-toast-overlay.tsx` is NOT covered by `toast.rs`.** `toast.rs` ported
  `ui/toast.tsx`'s `AnchoredToasts`, and `native/mapping/toast.md` **already
  says**, in its own words, that this is *"a component with no code path in any
  environment"*. **The toast users actually see is `sidebar-toast-overlay.tsx`'s
  hand-rolled `SidebarToastItem`, and it has never been ported.** So one of the
  43 "ported surfaces" measures something unreachable while the reachable thing
  is missing — a green surface count concealing a hole, which is the same shape
  as this iteration's other two findings.
- **`workspace-branch-icon.tsx` is not ported either.** `flicker-spinner.md`
  cites it only as *size provenance*, and `git_status_row.rs` is an unrelated
  git-panel row.

##### `fps-overlay` is not dev-only

It is a **settings toggle** (`showFpsOverlay`, Developer tab) and ships in every
build. I had guessed "dev-only, probably a build flag"; it is a real user-facing
surface. Classified Tier B.

### ▶ ANCHORS.md is now **v1.14** — the app-state hole is in the contract

`9770cc07`. The finding below is no longer only a queue note: `state` names the
§8.3 cell and **not** the app configuration, and the contract says so, in the
"stated hole" shape v1.9 already uses. A fifth `state` key was **refused** — the
native side has no "carousel position", so a key only the DOM side could fill
would look like coverage and be decoration.

Three rules replace the field it cannot have: a reference **records its drive**
in that surface's mapping doc; **a control cell with an already-recorded verdict
is re-run every time**; and an unreproducible reference is **deleted, not
shelved**.

**Briefs quote this header as the contract version — say v1.14 from here.**

### ‼️ The web suite exits 1 on a fully green run — reproduced by me

```
Test Files  363 passed (363)     Tests  2709 passed (2709)
Errors      1 error              error: script "test" exited with code 1
```

An `EnvironmentTeardownError`: `git-blame-api.ts` loading `git-repo-api.ts`
**after the environment was torn down**, reached through
`workspace-store-editor-arm.test.ts → workspace-store → buffer-slice →
git-blame-store`. Vitest's own warning is *"This might cause false positive
tests"* — which is the real cost, not the exit code.

**Two workers hit this before me and both filed it as unrelated flake noise.** It
is neither: a suite whose exit code is 1 on a green run cannot gate anything, and
one that may be reporting some of its 2709 passes wrongly is worse than one that
fails honestly. Dispatched as **P3.47**.

## ‼️ I captured a light `tabs` reference, then DELETED it — and that is the finding

Having proved the capture loop, I went after a real §17.1 cell: `tabs` is the
surface I had already shown is genuinely theme-sensitive, and its light cell had
never been taken. The capture worked. **The reference it produced is invalid, and
I deleted it rather than bank it.**

| | root `w` | per-tab widths |
|---|---|---|
| stored **dark** reference | **278** | 90 · 90 · 90 — equal, i.e. `flex-1` |
| the **live app**, today | **328** | 118.77 · 100.63 · 100.63 — unequal, i.e. content-sized |
| native fixture | 278 | 90 · 90 · 90 |

**The live reference app is not in the configuration its own stored references
were captured in.** The tabs are a different width *and a different sizing mode*.
A light reference taken from today's app describes a component the native
fixture never renders, so diffing against it would produce six geometry deltas
that are not defects — which is precisely what happened.

**The control is what settled it.** I re-ran the *dark* cell, whose verdict is
already recorded as a pass, and it failed too — 8 deltas. That is the signal that
the fault was mine, not the port's. At the reference's own width:

```
tabs dark, --width 278:  oracle: PASS — 0 deltas over 6 anchors compared
```

**Two separate errors of mine in one run**, both worth naming:

1. **`--width` again.** I drove at 328 — the *live app's* width — against a
   reference whose root is 278. Reading the reference's root `bounds.w` before
   driving is a rule I have now written down twice and broken twice more.
2. **I assumed "the app is running" meant "the app is in the reference's
   state".** It is not, and nothing in the snapshot can tell you: `state` records
   width, theme, content and flags, none of which distinguish a 278px
   equal-width tab strip from a 328px content-sized one.

**So the §17.1 push is harder than "the harness works".** Every stored reference
was captured from an app configuration that is not written down, and today's app
does not reproduce it. Recapturing the matrix means either reconstructing each
configuration or re-capturing **every** cell of a surface together — including
the ones that already pass — so a surface's references are at least
self-consistent. That is a real design question and it belongs in ANCHORS.md, not
in a worker brief.

**Why the invalid file is gone rather than kept "for later":** a plausible-looking
reference sitting in `/tmp` is exactly how a wrong number gets diffed against six
weeks from now. This project has already had one fabricated reference; a merely
*mismatched* one is quieter and no less wrong.

## §17 scoreboard — where "done" actually stands

Nine conditions. **Two are met.** Kept here rather than reconstructed each
session from prose scattered over 4,000 lines.

| # | condition | status | evidence |
|---|---|---|---|
| 1 | strict-parity anchors converge across the §8.3 matrix | ❌ **not met, and further off than this row used to say** | **Two gaps, not one.** *(a)* **Denominator**: §5.1 covers `components/ui` **and** `components/layout`; the second is at **1 of 29** and was never counted — so "43 surfaces" measured against half the definition. *(b)* **References**: for the surfaces that do exist, 258/258 cells are *runnable* but references exist for **one** cell of most. Viewport axis inert on all of them, theme axis vacuous on 10, and v1.14 shows a reference is only comparable if the app is driven back into its original configuration — which nothing records. |
| 2 | every §5.2 surface judged against Zed | ❌ **not met** | editor, diff and terminal are not built. Phase 4+. |
| 3 | both coverage numbers met, reported separately | ⚠ **half met** | **line coverage MET** — re-measured 2026-08-03 across all six §12 crates: **99.64%** (4,978 lines, 18 missed) against a ≥98 gate. The corpus number is *reported* but thin — see row 1. Never averaged. |
| 4 | zero `unsafe` outside `crowbar-platform`, every block there proved | ✅ **met — and now vacuous** | grepped 2026-08-03: **0** outside. P3.40 removed the last AppKit code, so there are **0 inside** too. Rule 3 of `check-invariants.sh` passes with nothing to check; the mutation evidence for it is historical. **Re-run that mutation the moment `unsafe` returns.** |
| 5 | zero `unwrap`/`expect`/`todo!` outside tests | ✅ **met, enforced by the compiler** | `[workspace.lints.clippy]` denies `unwrap_used`, `expect_used`, `panic`, `todo`, `unimplemented`; all 12 crates opt in via `[lints] workspace = true`; **0** per-site `#[allow]`s. Proved by mutation, both directions — see below. |
| 6 | leak soak shows no RSS growth vs React | ❌ **not met** | there is no shared workload to soak yet. Sequencing, not neglect. |
| 7 | `blocked/` empty, or every item a listed user decision | ⚠ **effectively met — 4 user decisions, 2 resolved, 1 hazard note** | classified 2026-08-03, then corrected the same day when one "outstanding" item turned out to have been resolved for three days. See below. |
| 8 | terminal conformance suite green | ❌ **not met** | the terminal is not ported. |
| 9 | a user cannot tell the apps apart, except §13 | ❌ **not met** | follows from 1, 2 and 8. |

### §17.7 — `blocked/` classified, 2026-08-03

§17.7 is met when every remaining item is **a user decision, listed**. So each
one has to be sorted into "waiting on the user" or "waiting on me", and until
today nobody had.

| item | class | standing |
|---|---|---|
| `cla-policy.md` | **user decision** | AGPL-only relicense left the CLA requirement open. Blocks nothing. ✅ qualifies |
| `vendored-crates-without-a-licence.md` | **user decision** | `gpui_shared_string` and `gpui_util` declare no licence upstream. Needs confirmation with Zed or an accepted risk. ✅ qualifies |
| `route-audit-red-at-head.md` | **user decision** | two `api/` routes registered but not declared; `api/` is out of this port's scope per §0. **Re-checked today and it has drifted further: 161 → 163**, so the gate is now two routes staler than when it was raised. ✅ qualifies |
| `resizable-needs-a-taller-display.md` | **user decision** *(or a bigger display)* | needs a measurement-window ruling I cannot make. ✅ qualifies |
| `s13-native-menus-accepted-delta.md` | **resolved** | decision made 07-31, spec edit landed `a4de9aef`. ✅ |
| `hover-and-focus-need-an-unlocked-screen.md` | ✅ **resolved 2026-07-31** | ~~mine, and now unblocked~~ — **see the correction below.** `hover` **PASS 0/8**, `focus` **PASS 0/8**, both captured from the live reference and diffed by me three days ago. |
| `locked-screen-blocks-every-capture.md` | ⚠ **environmental note, not a work item** | the lock was real when raised and can recur; captures work today. It is not a user decision, so strictly it does not satisfy §17.7's wording — but it is a hazard record, not open work. Flagged rather than argued away. |

#### ‼️ I got the hover/focus row wrong, by reading eight lines of a 300-line file

I classified that item as *"the one item standing between §17.7 and met… work I
owe"*. **It had been finished for three days.** The resolution — both cells
captured live and diffed by me, 0 deltas each — sits under a heading **230 lines
down**, while the title still began "Blocked —".

I read the head of each file and trusted it. The head was stale. **A file whose
first screen does not carry its own verdict will be classified by its
directory name**, and `blocked/` is a directory name that answers the question
for you. Fixed at the source: that file now opens with a RESOLVED banner, and so
does `s13-native-menus`.

**So §17.7 stands at four listed user decisions, two resolved items, and one
environmental note** — not the "one outstanding item" I wrote an hour ago.

### The §17.5 mutation, run by me 2026-08-03 — both directions

A denial that is declared but not opted into is inert, and Cargo does **not**
inherit `[workspace.lints]` automatically. So I tested it rather than reading it:

```
control      cargo clippy -p crowbar-core --all-targets     → errors=0

shipping     + pub fn probe(v: Option<u8>) -> u8 { v.unwrap() }
             → error: used `unwrap()` on an `Option` value
               note: requested on the command line with `-D clippy::unwrap-used`

test-scoped  + #[cfg(test)] mod probe { … v.unwrap() … }
             → errors=0
```

**Both halves matter.** The first proves the lint fires; the second proves the
exemption is *scoped to tests* rather than the lint being off everywhere. The
exemptions live centrally in `native/clippy.toml`, whose own comment gives the
reason: a per-file `#[allow(clippy::unwrap_used)]` *"would be indistinguishable
from an agent silencing the lint in shipping code."*

> I nearly filed this as a **false alarm**: `grep -l 'lints.workspace = true'`
> matched 0 of 12 crates, which looks exactly like an unenforced gate. Every
> crate uses the two-line form (`[lints]` / `workspace = true`) instead. **The
> grep was broken, not the code** — which is the third time today a check of mine
> was the thing at fault. Verify the checker before believing the finding.

---

## Environment facts (verified 2026-07-30)

Recorded because a cold session will otherwise rediscover them the hard way.

| Fact | Value |
|---|---|
| `rustc` / `cargo` | **1.96.0**, installed but **NOT on the default PATH**. Every shell needs `export PATH="$HOME/.cargo/bin:$PATH"`. |
| rustup toolchains | `stable` (active), `1.85`, `1.88` |
| Installed targets | `aarch64-apple-darwin`, `x86_64-apple-darwin`, `x86_64-pc-windows-msvc`, `x86_64-unknown-linux-gnu` |
| `cargo-llvm-cov` | installed 2026-07-30 (the §12 gate tool). |
| `cargo-nextest` | not installed |
| Go | 1.26.2 (`/opt/homebrew/bin/go`) |
| `bun` | `~/.bun/bin/bun`, also off the default PATH |
| Shell is **zsh** | **Unquoted `$var` does NOT word-split**, unlike bash. Building an args string and passing `$args` sends it as ONE argument. Use an array or `${=args}`. Cost me a silently-empty sweep that looked like the binary was broken. |
| `timeout(1)` | **not installed** on macOS. `gtimeout` needs coreutils. A command using it fails with `command not found` and the surrounding pipeline still reports success. |
| Zed | `/Applications/Zed.app` present (stable channel) — used by the §10.4 AX spike |
| Network | reachable |
| `go build` of a **main** package | fails with `error obtaining VCS status: exit status 128` — Go's buildvcs stamping walks up and finds the parent repo's working tree. **Always pass `-buildvcs=false`.** Pre-existing and environmental; reproduces on a pristine checkout at this path. |
| `go build ./...` untagged | fails at `cmd/crowbar/web_embed.go`: `pattern all:web/dist: no matching files found`. Needs `make embed-web` first, or the repo's canonical `-tags noEmbed`. Also pre-existing. |
| Vendored gpui build cost | ~455 crates, **6m41s** cold release, **1.2 GB** of `target/` (now **5.5 GB** with debug + coverage profiles). Budget for it. |
| **⚠ Worker worktrees fill the disk** | Each worker gets its **own** `native/target`, and a full vendored-gpui build is **8–13 GB**. Ten workers took the machine to **121 MiB free** and P1.10 died mid-build with `No space left on device`. At zero bytes **even Bash stops working** — the harness ENOSPCs creating its own output file *before* the command runs, and `Write`/`Edit` fail on their temp files, so you cannot dig yourself out. **Clean each worker's `native/target` at merge time.** Recovering 8 merged workers' caches freed **66 GB**. |
| **Never run two cargo invocations at once** | They deadlock on `~/.cargo/.package-cache`. Symptom is indistinguishable from a slow build: no `rustc` processes, `target/` mtime frozen, and cargo prints **nothing at all** — not even "Blocking waiting for file lock". I lost ~10 minutes to this by launching a second gate run before the first finished. Check with `pgrep -c rustc`; zero rustc plus a stale `target/` mtime means blocked, not building. |
| Killed cargo runs | A killed gate script can leave an **adversarial test edit in the source tree** if it dies before its revert. Always `git status` after killing one. |
| Running daemon | a `crowbar-api` from the **`feature/crowbar-skill` worktree** is live on a temp socket. It is **not** this port's daemon; do not reuse it for parity runs. |

### The shared `CROWBAR_HOME` for parity runs (item 0.4)

`Makefile:11` scopes every `dev*` target to `CROWBAR_HOME ?= $(CURDIR)/.crowbar`.
Both apps inherit it, so "one daemon, shared home" is already the harness — the
native app only has to derive the same socket path.

| | |
|---|---|
| `CROWBAR_HOME` | `<worktree>/.crowbar` |
| fnv1a64 of it | `6d4f21ce150add3c` |
| Socket | `$TMPDIR/crowbar-6d4f21ce150add3c.sock` |
| Present? | **yes** — daemon live, pid 62909, built from this worktree |

### 0.4 harness — one daemon, both apps. **No code required.**

Worked out by reading `desktop/src-tauri/src/sidecar/mod.rs`. The rule:

> **The Tauri app owns the daemon. The native app is just another unix-socket
> client on the same path.** Start Crowbar-React first and leave it open.

That works because the socket path is a *pure function of `CROWBAR_HOME`*, both
apps set the same `CROWBAR_HOME` (`Makefile:11`, `desktop/Makefile:12`), and
`crowbar-client` connects directly to the socket with no proxy (§9.1).

> **Do NOT start a daemon manually first and then launch the Tauri app.** It
> looks like the tidier arrangement and it is a trap. `spawn()` has no
> attach-to-existing path — it unconditionally launches
> `crowbar-api serve --host unix://<path>`. Its comment at
> `sidecar/mod.rs:194` is explicit that the daemon "refuses to clobber one with
> a live daemon still behind it", so the app's child dies on bind — but
> `wait_for_health` then probes the socket and finds *your* daemon healthy, so
> the app reports success while its supervisor sits in a budgeted respawn loop
> against a socket it can never take. Symptom: everything works, with a daemon
> respawn storm in the log.

Two consequences worth knowing:

- Closing the Tauri window kills the daemon (there is a window-close kill path),
  which drops the native app's connection. Expected; not a native-app bug.
- `desktop/scripts/fetch-sidecar.sh` **builds the Go daemon from this worktree's
  source** despite its name. So daemon changes (0.6, 0.7) reach the harness on
  the next `make dev-desktop`, with no separate step.

### 0.4 — both apps against one daemon ✅ **CLOSED. Verified by my own run.**

At 2026-07-30 22:42 all three of these were alive **simultaneously**:

| | pid | |
|---|---|---|
| native `crowbar-app` | 12057 | GPUI window, layer 0, 855×1119 |
| React `crowbar-desktop` | 55007 | GPUI/WebKit window, layer 0, 855×1119 |
| daemon `crowbar-api` | 62909 | on `$TMPDIR/crowbar-6d4f21ce150add3c.sock` |

**I did not accept the worker's run as evidence.** It reported reaching the
daemon at 22:31:38; I killed that instance, baselined the daemon's health-request
count at 5, launched `crowbar-app` myself with the shared `CROWBAR_HOME`, and
watched the count go to 6 with a new line:

```
[err] 2026/07/30 22:42:00 GET /v0/health 200 783.167µs
```

Window presence confirmed via `CGWindowListCopyWindowInfo` (a Swift script —
`screencapture` and the AX API are both permission-blocked in this shell, and
`AXIsProcessTrusted()` is `false`, so pixels cannot be read here).

**The native window renders:** `daemon reached` / `pid 62909` / `status ok` /
`version 0.1.0` / the resolved socket path. Those exact strings are pinned by a
unit test, because the frame itself cannot be read back.

Gates, all re-run by me: `cargo build --workspace` clean · `cargo clippy
--workspace --all-targets -- -D warnings` **exit 0** · `cargo test --workspace`
**21 passed, 0 failed** · `check-invariants.sh` all four rules ok.

**§12 coverage, measured by me:**

```
crowbar-client   Lines 99.64%   Regions 98.23%   Functions 100.00%
  socket.rs      100.00% lines
  health.rs       99.12% lines
  lib.rs         100.00% lines
```

Above the ≥98% bar. The one uncovered line is an error arm in the test harness.

#### The zero-pad trap — closed, and the test is real

`socket.rs` formats `format!("crowbar-{hash:x}.sock")`, never `{:016x}`.
`socket_name_is_not_zero_padded` pins `/home/10/.crowbar`, and **I recomputed
the hash independently**: `fnv1a64` = `0x0bbb_5af7_c356_03fa`, which Go's `%x`
renders as `crowbar-bbb5af7c35603fa.sock` — **15** hex digits. The test asserts
the value, the short name, that the padded name is *not* produced, and that the
hex run is 15 chars. It genuinely fails under `{:016x}`.

A detail the worker added that I had not specified, and which matters: the hash
input is the `CROWBAR_HOME` string **exactly as it appears in the environment**
— not canonicalised, not trailing-slash-normalised. Go hashes the raw string, so
anything else silently disagrees.

#### Structural decisions to know about

- **Path derivation went in `crowbar-client`, not `crowbar-core`.** §4.2 gives
  `core → client`, so the edge cannot be reversed; putting it in `core` would
  mean the crate whose entire job is dialling the socket has to be *told* where
  the socket is. It is still a pure function under the same ≥98% gate.
- **`gpui` is a direct dep of `crowbar-ui`, `crowbar-state` and `crowbar-app`
  only** — not all eight permitted crates. `crowbar-ui` re-exports it
  (`pub use gpui; pub use gpui_component;`) and the leaf view crates take it
  transitively, which is what their scaffold manifests already claimed. See the
  0.1 note above for the anti-hardcoding consequence this creates.
- `crowbar-app` also needs `gpui_platform` — it owns `application()`, and
  `gpui::Application::new()` does not exist at this rev (only `with_platform`).
- **`reqwest` 0.13 has native unix-socket support** (`ClientBuilder::unix_socket`),
  so §10.1's "unix connector" needed no third-party crate. `tungstenite`
  deliberately not added until a caller exists.

> **A warning line that is not ours and cannot be removed.** Every cargo
> invocation in `native/` prints:
> `warning: the following packages contain code that will be rejected by a
> future version of Rust: block v0.1.6`.
> It is a cargo **future-incompat report**, not a rustc warning against our
> code — `block 0.1.6` is a crates.io transitive dep of the vendored gpui macOS
> stack (`static of uninhabited type`). Removing it needs either a `[patch]`
> (PINNED.md forbids inventing one) or an edit under `vendor/` (breaks the pin).
> **`clippy -D warnings` is genuinely clean.** Do not treat this line as a dirty
> gate, and do not silence it with a `.cargo/config.toml`.

> **Driving the app: the MCP bridge is on port 9224, not 9223.** The log line is
> `MCP Bridge plugin initialized for 'Crowbar' (software.rabbyte.crowbar) on
> 0.0.0.0:9224`. `driver_session` defaults to **9223**, and on this machine 9223
> is a *different* Crowbar instance from another worktree — it connects happily
> and every `execute_js` then times out after 7s against the wrong window.
> Always pass `port: 9224` (and read the real port out of the log first). Also
> note the bridge binds `0.0.0.0`, not loopback — dev-only, but the opposite of
> what 0.7 is required to do.

### Client-side persistence defeats `CROWBAR_HOME` isolation — hard evidence for D6

Chasing a 404 loop in the reference app turned up something worth more than the
bug. The app, against a **completely fresh** `CROWBAR_HOME`, was polling
`/v0/projects/835f0a4b-…/home` every 30s. The daemon has **zero** projects
(`GET /v0/projects` → `[]`) and that id appears nowhere in its `view.db`.

Where it came from, confirmed by reading it out of the running webview:

```
localStorage['crowbar.activeProject']
  = {"state":{"activeProjectId":"835f0a4b-9618-4bea-b4b8-d4468939840f"},"version":0}
```

A zustand-persist payload — **from the user's production Crowbar.**

`/Applications/Crowbar.app` and the dev build share the bundle identifier
`software.rabbyte.crowbar`, and WKWebView keys its storage container on bundle
id. So `~/Library/WebKit/software.rabbyte.crowbar/` is **one container shared by
dev and production**, and I confirmed the dev run wrote into it (files under it
are newer than the launch). The `Makefile` carefully isolates every byte of
*daemon* state per worktree, and then the webview reaches straight around that
isolation into production's client state.

**Three consequences:**

1. **This is the strongest argument for D6 in the codebase.** A stale persisted
   `activeProjectId` is never reconciled against the daemon's real project list,
   so the app chases a project that does not exist, forever, silently. The
   native client has no client-side persistence and cannot do this.
2. **§9.3's migration list is incomplete.** It names 7 read-through caches and 4
   IndexedDB stores. It does not mention **`localStorage` at all** — which holds
   **130 keys** here: the whole `crowbar:settings:*` surface (real user
   settings: fonts, theme, terminal, editor), `crowbar.activeProject`,
   `crowbar.bootstrap.appearance.v2`, `crowbar:cache-version`, and a long tail
   of `crowbar:terminal-reconnect:<uuid>` entries that appear never to be
   reaped. These need the same treatment as the four IndexedDB stores — decide
   per key: daemon-side via `/v0/settings/ui`, or deleted.
3. **Do NOT clear this storage to make the dev app tidy.** It is the user's
   production state. The 404 loop is cosmetic (the app renders OOBE correctly);
   wiping the container to silence it would destroy real settings.

The app itself is **fine**: `location.href` is `#/oobe`, rendering "The IDE where
agents do the heavy lifting", which is exactly right for a home with no projects.

#### The full `localStorage` surface, measured from the running app

**166 keys, 32,762 bytes.** Read live out of the webview. §9.3 accounts for
none of it. Classification, and what each needs:

| Group | Count | Disposition |
|---|---|---|
| `crowbar:settings:*` | **88** | **Daemon-side, `global` scope.** This is the *entire* settings surface. |
| `crowbar:terminal-reconnect:<workspaceId>` | **64** | **Delete.** See below. |
| `react-scan-*` | 3 | Delete — dev tooling. |
| `crowbar.activeProject` | 1 | **Delete.** This is the one that caused the 404 loop. |
| `crowbar.bootstrap.appearance.v2` | 1 | Daemon-side, `global` — it exists to paint before first frame. |
| `crowbar:agent-chat-order:<uuid>` | 2 | Daemon-side, per workspace. |
| `sidebar-open`, `sidebar-width` | 2 | Daemon-side, `global`. |
| `terminal-profiles` | 1 | **Check for a duplicate source of truth** — see below. |
| `crowbar:switch-profiles`, `crowbar:cache-version`, `crowbar_font_cache` | 3 | Delete — caches and a schema version that D6 makes meaningless. |
| `setItem` | 1 | Debris, see below. |

**Five things this turned up:**

1. **88 `crowbar:settings:*` keys.** Theme, fonts, editor, terminal, git, file
   tree, keybinding presets *and* user overrides, enterprise policy
   (`enterpriseManagedMode`, `enterpriseRequireExtensionAllowlist`,
   `enterpriseAllowedExtensionIds`), telemetry. §9.3 moves "four stores"
   daemon-side and never mentions this. **It is the single largest omission in
   the persistence plan** and the native client needs every one of these keys or
   users lose their entire configuration on migration. At 32 KB total the
   migration is cheap — the risk is forgetting it, not doing it.

2. **`crowbar:terminal-reconnect:*` grows without bound.** 64 entries, one per
   workspace ever opened, keyed `crowbar:terminal-reconnect:${workspaceId}`
   (`features/terminal/lib/terminal-reconnect-map.ts:4`). Each holds a
   tab-id → session-id map. Nothing reaps them when a workspace is deleted.
   D6 deletes the whole category rather than porting the leak.

3. **`terminal-profiles` may be a duplicate source of truth.** It is a
   zustand-persist blob (`{"state":{"profiles":[],...}}`) sitting in
   `localStorage`, while the daemon *already* owns
   `/v0/settings/terminal/profiles` with full CRUD
   (`endpoints/terminal/routes.go:40-44`). Worth resolving before the native
   client reads either — two writable copies of the same list is how they drift.

4. **Two un-namespaced keys.** `sidebar-open` and `sidebar-width` carry no
   `crowbar:` prefix and are written straight from a component
   (`components/layout/use-sidebar-panel.ts:61,120`). In a shared origin that is
   a collision waiting to happen, and it sidesteps the store layer that
   `CLAUDE.md` says owns persistence.

5. **A key literally named `setItem`**, whose value is
   `(k, v) => { if (k === 'sidebar-width') window.__drag.writes++; return origSet(k, v); }`
   — leftover instrumentation from a past drag-performance investigation.
   **Not a repo defect**; grep finds no such code. It persists because
   `localStorage.setItem = fn` on a `Storage` object goes through the named
   property setter and is *stored as data* instead of shadowing the method. A
   debugging session wrote a permanent key into the user's production storage
   and nobody noticed. Left in place — see consequence 3 above.

### Reference side now declares `content_sized` — verified live, 2026-07-31

Confirmed in the running app after P1.8 merged, not from the diff:
`data-oracle-content-sized` appears on **`git-row-badge` (6)**,
**`git-row-added` (4)**, **`git-row-deleted` (1)** — and **not** on
`git-row-name`, which is correctly the flexible sibling that absorbs the excess.
The extractor emits `"content_sized": true` on exactly those anchors.

Archived as `native/oracle/runs/ref-v3-content-sized.json`.

> **The v3 pair is deliberately NOT run yet.** I labelled its cell
> `state.width: 1100` — the **viewport** width, per the v1.6 ruling — while the
> native binary still labels its cell by the **surface** width (294). Running
> them now would be running with exactly the broken label I just diagnosed:
> either the differ refuses on a mismatched matrix cell, or I fudge one side's
> label to make it agree, which is the fake convergence §8.3 exists to prevent.
> **It waits for P1.9's `--viewport-width`.**

> **The `pkill -f vite` hazard fired again**, and the pre-flight check earned its
> place: my Vite on 5273 was dead — killed by another session starting a Crowbar
> — while **my app kept running against nothing**. There are now four
> `crowbar-desktop` processes on this machine, three of them other sessions'.
> Had I skipped the check I would have extracted from a stale page and compared
> it in good faith. **Run the check every time; it has now caught this twice.**

## ✅ THE STATE AXIS IS CLOSED — `selected` converges, and it found a real defect

`file-tree-row` is the second gate surface, added because the git row's state
axis was vacuous. It works.

**Driven on both sides, for real:**

| | reference (WKWebView) | native (GPUI) |
|---|---|---|
| resting `file-row-item.bg` | `#00000000` | `#00000000` |
| **`selected`** | **`#ffffff0a`** | **`#ffffff0a`** |
| `hover` | `--file-tree-hover-bg` | `#ffffff07` |
| `focus` `button.border` | `:focus-visible` rule exists | `#ffffff0d` |

The reference `selected` state was reached **programmatically** — no real pointer
input — by focusing `.file-tree-container` and dispatching a bubbling click.
Confirmed live: `data-active="true"` and `::before` painting
`oklch(1 0 0 / 0.04)`, which is exactly `#ffffff0a`.

> **Ordering matters and cost several attempts.** `highlightedPath =
> hasTreeFocus ? focusedPath : activePath`, and an effect resets `focusedPath`
> when the tree is unfocused. So focusing the container and clicking **in the
> same tick does not work** — React has not committed `hasTreeFocus` yet. Focus
> in one call, click in the next. Also: clicking a folder **toggles** it, so
> repeated calls oscillate it open/closed.

### The run found one genuine defect

```
oracle: FAIL — 1 delta over 6 anchors compared (1 colour)
file-row-name.fg: #f5f5f5ff, expected #fe9a00ff (Δ b +245, rgb is exact)
```

**The reference colours the filename by git status** — `a.ts` is modified, so it
renders amber `#fe9a00ff`. The native row paints default foreground. That is a
**missing feature in the native component**, found by the oracle rather than by
reading code, which is exactly what it is for.

> **Two of the three original deltas were MY driving error, not defects.**
> `file-row-guide-1` differed in `y` and `h` because I left `--prev-depth` /
> `--next-depth` at their defaults, so no guide capping was applied. The
> reference's `a.ts` follows `a` at depth 1, so level 1 *is* top-inset by 4px.
> Driving with `--prev-depth 1 --next-depth 2` cleared both. **The guide-inset
> logic is correct on both sides** — I had simply not told the native side who
> its neighbours were.

### Three measurements that would each have manufactured a delta on this surface

Found by the worker before compiling, and each is the same class of trap:

1. **Indent step is 16**, not the sidebar tree's 14 — `settings.fileTreeIndentSize` defaults to 16.
2. **The line box is 20px**, not the git row's 18.9 — `GitFileItem` authors `leading-[1.35]`; this row authors nothing and inherits `.text-sm`'s `calc(1.25 / 0.875)`.
3. **The icon starts at `1 + padding`** — the container-scoped `border: 1px solid transparent !important` shifts the content box. The git row's button has no border, so the two surfaces genuinely differ.

And the `file-row-button` declares **neither** `content_sized` nor `line_sized`:
it paints text and holds one line, but `h-6` authors its box at 24 around a 20px
line — the same trap as the badge.

### States this surface genuinely has

`hover` ✓ · `selected` ✓ (converges) · `focus` — rendering is real *here*
(`:focus-visible` is scoped to `.file-tree-container`, which this row is inside
and the git row is not), but **drivability unconfirmed**: the tree keeps DOM
focus on the container and moves a virtual cursor via `aria-activedescendant`,
so a row button never takes DOM focus through normal navigation. `empty`,
`loading` — **absent**. `error` — a transient `✕ failed` span owned by the tree,
reachable only by making a real operation fail; **not modelled, not fabricated**.

## ✅ THE GATE PASSES ON ALL 18 RUNNABLE CELLS — 2026-07-31

**3 viewport widths × 2 themes × 3 content lengths = 18 cells. Every one PASS.**
Each is a live WKWebView capture diffed against a live GPUI snapshot by me.

| viewport | short | normal | overflow |
|---|---|---|---|
| **600** dark | PASS | PASS | PASS |
| **600** light | PASS | PASS | PASS |
| **800** dark | PASS | PASS | PASS |
| **800** light | PASS | PASS | PASS |
| **1100** dark | PASS | PASS | PASS |
| **1100** light | PASS | PASS | PASS |

Σ ceil excess tracks the badge variant across the breakpoint — **1.51px at 600**
(narrow badge, 12px text) versus **1.73px at 800/1100** (wide badge, 10px text) —
which is the viewport axis doing real work rather than relabelling.
32 snapshots archived under `native/oracle/runs/matrix/`.

### ⚠️ The state axis is **vacuous on this component** — measured, not assumed

The spec picked `tree-row` believing it "exercises … selection state". **In every
live consumer, it does not.** I checked each of the six §8.3 states:

| state | why it cannot be exercised here |
|---|---|
| `hover` | Reference undrivable. CSS `:hover` is **UA pointer state**; dispatched events do not set it and `webview_interact` has no hover action. Chrome's `CSS.forcePseudoState` would do it — WKWebView exposes no CDP. |
| `focus` | **Paints nothing.** The `:focus-visible` rule is scoped to `.file-tree-container` and `TreeRow` carries `outline-none focus:outline-none`. |
| `selected` | **Never set.** `SidebarTreeRow` accepts an `active` prop, but *neither* live consumer passes it — `git-status-file-item.tsx` contains no reference to `active` at all, and `changed-files-tree.tsx` does not pass it either. `data-active` is dead in every real call site. A real click confirmed it: `activeAfter: 0`. |
| `loading` | No such rendering exists in `GitFileItem`. |
| `error` | No such rendering exists in `GitFileItem`. |
| `empty` | Every fixture row carries an `uncommitted` badge. |

**This is a property of the component, not a limitation of the oracle** — and the
distinction is provable: on the **native** side the state axis demonstrably
works, with `hover` → `#ffffff07` and `selected` → `#ffffff0a`, both distinct
from resting `#00000000`. The machinery measures states correctly. The
*reference* never enters them.

So running the six state flags on this target would produce six identical cells
that converge trivially — the exact "passes while telling us nothing" failure
§8.1 was written to prevent. **I am not counting them as passes.**

**What this means for the STOP gate.** The mechanism is validated on every axis
this component *has*. The state axis needs a target that actually has states —
the file-explorer tree row, which is the same `components/ui/tree-row.tsx` the
spec names, reached through `FileExplorerTreeItem`, and which **does** set
`data-active`. That is the honest way to close the axis, and it is Phase 1 work,
not Phase 2.

## Earlier: the first 3 cells, driven and diffed by me

```
oracle: PASS — 0 deltas over 10 anchors compared,
        5 forgiven by v1.5 content-sizing, 1 forgiven by v1.6 line-sizing
```

| # | viewport | theme | content | depth | anchors | result |
|---|---|---|---|---|---|---|
| 1 | 800 | dark | overflow | 4 | 10 | **PASS** |
| 2 | 800 | dark | short | 2 | 8 | **PASS** |
| 3 | **600** | dark | overflow | 4 | 10 | **PASS** — *below* the 640 breakpoint |

Cell 3 is the one that matters. At a 600px viewport the reference renders the
Badge's **narrow** variant (h20 / 12px) instead of the wide one, and the native
side — which previously had no viewport concept at all — **followed the
breakpoint correctly** via `--viewport-width`. The `state.width` fix is doing
real work, not just relabelling: Σ ceil excess even changes with the variant
(1.73px at 800, **1.51px at 600**), because the badge's text is a different size.

Every difference in all three cells is forgiven by an explicitly **declared**
rule, and each forgiven line names the rule, gives the cause, and quotes **the
tolerance it actually broke** — never a widened one. A PASS reached by forgiving
five comparisons cannot be mistaken for one reached by measuring them.

**What is NOT yet covered**, stated plainly rather than left to inference:

| axis | status |
|---|---|
| viewport widths | 2 of ≥3 (800, 600) — a third still to run |
| theme | dark only — **light not run** |
| content length | short + overflow — **normal not run** |
| `hover` | **cannot be driven on the reference side** (documented above) |
| `focus` | drivable but paints nothing — converges and proves nothing |
| `loading` / `error` | no React original exists; native warns they prove nothing |
| `selected` | not yet attempted — the row's `data-active` is app state, so a real click may drive it |

So: **the gate passes on the cells I have run, and the matrix is not complete.**
Those are different claims and I am not merging them.

## ⚠ OPEN: `state.width` is ambiguous, and the badge proved it is load-bearing

**This already produced one wrong comparison and it will produce more.**

`state.width` is documented only as "integer, logical px". §8.3 asks for "≥3
**viewport** widths". But on the native side there is no meaningful viewport —
the app renders one row — and P1.5's `--width` sets the **surface** width. So
the two sides have been putting different quantities in the same field.

**How it bit.** My first archived reference recorded `state.width: 294` — the
surface width — while the badge's appearance was actually governed by the
**viewport** width, which nothing recorded. The webview happened to be at
**569px**, below Tailwind's 640px `sm:` breakpoint, so the reference rendered
the narrow badge variant (h20 / 12px) while the native side implemented the wide
one (h16 / 10px). The differ dutifully reported four geometry deltas that were
**neither side's fault**. Re-capturing at 1100px made them vanish with no code
change at all.

**Why it recurs.** The §8.3 matrix wants ≥3 viewport widths. Any set that
straddles 640px flips the reference's badge variant — and the native app,
having no viewport concept, **cannot follow**. Every such cell would compare a
narrow-variant reference against a wide-variant native and report deltas that
mean nothing.

**Decision: `state.width` is the VIEWPORT width**, matching §8.3's own wording,
because it is the quantity that drives breakpoint-dependent styling and it is
the one that silently differed. The **surface** width is a separate input.

**What that requires:** the native app must accept a viewport width and size its
window to it, so breakpoint-dependent styling resolves the same way on both
sides — `--viewport-width` alongside the existing `--width`. Until it does, the
three-widths axis of the matrix cannot be run honestly, and **any width cell
that straddles 640px is untrustworthy**.

**Not yet dispatched** — P1.8 currently owns `crowbar-app/src/**`, and a second
worker there would collide. It goes out when P1.8 lands.

> An unrelated confirmation from the same investigation, worth keeping: toggling
> the root `dark` class gives badge background alpha **0.16 dark / 0.08 light**,
> which is exactly the `warning.mix(16 dark / 8 light, TRANSPARENT)` P1.4's
> generator emitted. The light/dark token tables are independently corroborated
> against the live DOM.

## 🎯 THE PHASE 1 GATE — RUN 2: **26 → 18 → 6 deltas**, one row, two root causes

Same cell, everything aligned. **Zero text deltas, zero colour deltas, zero
presence deltas.** All six remaining are geometry, none larger than 1.73px on a
294px row.

```
git-row-name.bounds.w:   103.0, expected 104.73  (Δ -1.73)
git-row-badge.bounds.x:  195.0, expected 196.73  (Δ -1.73)
git-row-name.bounds.h:    19.0, expected  18.0   (Δ +1.00)
git-row-badge.bounds.w:   75.0, expected  74.11  (Δ +0.89)
git-row-added.bounds.w:   12.0, expected  11.16  (Δ +0.84)
git-row-added.bounds.x:  276.0, expected 276.84  (Δ -0.84)
```

**`text_width` is now 476.49 on both sides — exactly zero.** The font fix made
text comparison meaningful rather than accidentally-agreeing.

### The deferred `ceil()` measurement is now answered, and the answer is *better* than feared

| anchor | ref `w` | native `w` | `ceil(ref)` | match |
|---|---|---|---|---|
| `git-row-badge` | 74.11 | **75.00** | 75 | ✅ |
| `git-row-added` | 11.16 | **12.00** | 12 | ✅ |

Both content-sized boxes are **exactly** `ceil(reference)`. And:

```
total ceil excess          = +1.73
git-row-name.bounds.w delta = -1.73     ← the flexible sibling absorbs ALL of it
git-row-added right edge    = 288.00 on BOTH sides — conserved exactly
```

**The excess does not accumulate rightward without bound.** It is absorbed by
the flexible sibling, and the row's total width and the trailing group's right
edge are identical on both sides. The differ worker's worry — that you would
need a tolerance growing with the number of content-sized boxes upstream — does
not materialise, because the layout *conserves* the total rather than
propagating displacement.

**That collapses the modelling problem.** The differ does **not** need flow
order after all:

1. A declared content-sized anchor compares `native.w` against `ceil(ref.w)`.
2. Everything else in the same snapshot gets an additional allowance of
   **Σ(ceil excess) over the anchors declared content-sized in that snapshot** —
   a single global scalar the differ can compute from the anchor list, with no
   tree walk and no flow order.

That is implementable within §1's "no trees" constraint, which is what made the
alternative look expensive.

### The residual: 6 deltas, exactly 2 root causes

| root cause | deltas | magnitude |
|---|---|---|
| GPUI `ceil()`s content-sized box widths | 5 (2 direct, 3 absorbed consequence) | ≤1.73px total |
| GPUI snaps line-height to the device grid | 1 (`name.h` 19 vs 18; 14 × 1.35 = 18.9) | 1.0px |

Both are **framework rounding rules**, both are measured, both are bounded, and
neither is a defect in the ported component. **7 of 10 anchors remain
byte-exact.**

> **Not claiming "converged".** Six deltas exceed ±0.5 and the gate's condition
> is that every anchor converges across the whole §8.3 matrix. What is proven is
> that the *mechanism* works: the oracle found real differences, localised each
> to a specific framework behaviour, and the residual is sub-2px and fully
> explained. That is what Phase 1 exists to establish — but it is not the same
> sentence as "the row converges", and I will not write the second one until it
> does.

## 🎯 THE PHASE 1 GATE — FIRST FULL RUN, 2026-07-31

**The oracle ran end to end.** Live WKWebView snapshot → differ ← live GPUI
snapshot, same matrix cell (`294 · dark · overflow · no flags · depth 4`),
26 ranked deltas. Snapshots archived at `native/oracle/runs/`.

**The mechanism works.** That was the question Phase 1 exists to answer.

### 7 of 10 anchors converged **byte-exactly** — not "within tolerance", identical

| anchor | reference | native |
|---|---|---|
| `git-row-item` | 294×24 @(0,0) | 294×24 @(0,0) |
| `git-row-guide-0..3` | 7×24 @ x=14,28,42,56 | 7×24 @ x=14,28,42,56 |
| `git-row-button` | 294×24 @(0,0) r8 | 294×24 @(0,0) r8 |
| `git-row-icon` | 14×14 @(66,5) | 14×14 @(66,5) |

The row box, every indent guide, the button and the icon land on the same pixel.
The indent arithmetic, the guide geometry and the flex chain all reproduce
exactly. **`git-row-button` radius 8 / border 0 matches on both sides**, which
also closes my earlier stylesheet-reading error for good.

### The 26 deltas, triaged — none of them says "the approach does not work"

**8 are a bug in the differ, not the app.** Every `border.color` mismatch is on
an anchor whose `border.w` is **0**, where the DOM returns inherited text
colour. **ANCHORS v1.3 already rules that colour is compared only when `w > 0`
— the differ predates v1.3 and does not implement it.** Mine to fix; 8 deltas
disappear with it.

**5 are the contract gap P1.5 predicted I would hit first.** `git-row-badge` is
a painted box *containing* text. The React side emits it as a text anchor
(`text`, `fg`, `font`, `text_width`, `clipped`); the native side emits a box.
**v1 has no anchor that is both.** Contract change, not a component fix.

**4 are my own fixture mismatch.** The native row renders `+12`/`-3`; the
reference row renders `+1` and has no deletions. So `git-row-deleted` is
"present natively, absent in reference", `git-row-added.text` is `+12` vs `+1`,
and its width and x cascade from that. **My harness, not the component** — the
two sides must render the same content.

**1 is the decided scope reduction.** `git-row-dir` present natively, absent in
the reference — `showDirectory={false}`, already decided and recorded above.

**That leaves 8 genuine component deltas**, all understood:

| delta | cause |
|---|---|
| `git-row-badge` h 16 vs **20**, and x/y/w | the Badge's `sm:` breakpoint — native implemented the ≥640px variant, the reference is rendering the narrow one |
| `git-row-name.bounds.w` 39.5 vs 91.52, `text_width` 424.05 vs 476.49 | **the font is not loaded** — see below |
| `git-row-name.bounds.h` 19 vs 18 | GPUI snaps line-height to the device grid (14 × 1.35 = 18.9 → 19.0) |

### The state axis works on the native side — verified by me across six cells

The mid-flight correction to P1.5 (visual state must be a **prop**, because the
extractor reads base `StyleRefinement` and cannot see GPUI's runtime `.hover()`)
was necessary and it worked:

| cell | `git-row-item.bg` | `git-row-name.fg` | `text_width` |
|---|---|---|---|
| resting | `#00000000` | `#f5f5f5ff` | 424.047 |
| **hover** | **`#ffffff07`** | `#f5f5f5ff` | 424.047 |
| **selected** | **`#ffffff0a`** | `#f5f5f5ff` | 424.047 |
| focus | `#00000000` | `#f5f5f5ff` | 424.047 |
| **light** | `#00000000` | **`#262626ff`** | 424.047 |
| **short** | `#00000000` | `#f5f5f5ff` | **22.565** |

Hover, selected, theme and content length all move the snapshot. **`focus` is
byte-identical to resting**, exactly as P1.5 predicted — the `:focus-visible`
rule is scoped to `.file-tree-container` and `TreeRow` carries `outline-none`.
So the focus cell will converge and **prove nothing**; that is recorded, not
counted as a pass.

### ⚠ The **hover** cell cannot be driven on the React side with this harness

A real gap, found by trying rather than assuming.

- **Synthetic events do not trigger CSS `:hover`.** `:hover` is driven by the
  user agent's pointer state, not by dispatched `mouseover`/`mouseenter`. A
  `dispatchEvent` changes nothing about which rules match.
- **`webview_interact` has no hover action** — it offers click, double-click,
  long-press, scroll, swipe and focus. None of them leaves the pointer resting
  over an element.
- Chrome DevTools has `CSS.forcePseudoState` for exactly this. **WKWebView does
  not expose CDP**, and the MCP bridge does not reach WebKit's inspector
  protocol.

So the reference row's hover background — which is painted by
`.file-tree-item:hover::before` — is currently **unmeasurable**.

**Three ways out, in order of honesty:**

1. **Move the real cursor.** `CGEventPost` / `CGWarpMouseCursorPosition` from a
   small Swift helper, positioned from the window bounds (already obtainable via
   `CGWindowListCopyWindowInfo`) plus the row's client rect. This produces
   genuine pointer state, so the genuine rule matches. **This is the right
   answer** and it is feasible with tools already proven here.
2. A dev-only route that renders the row with a forced-hover class. Changes the
   app under test, but visibly and in a *test* surface rather than the
   production render.
3. Accept hover as unmeasurable on the reference side and compare the native
   hover against the CSS rule as *specification*. **Weakest** — it tests our
   reading of the stylesheet, which is the exact mistake I already made once.

**Not yet resolved.** It does not block the rest of the matrix, and it matters
again in Phase 5 (interaction and behaviour), so it is written down now.

### The font is the single biggest blocker, and it is an asset problem

`CalSansUI` ships **only as WOFF2** (`web/public/fonts/CalSansUI.woff2`, plus
`CalSans-Regular.woff2` — verified, there is no TTF or OTF anywhere in the
repo). GPUI hands the bytes to CoreText, which rejects them:

```
crowbar-app: font: CalSansUI rejected (parse error)
```

The row still *declares* `font_family("CalSansUI")`, so `font.family` compares
**equal on both sides while the shaping face is silently a fallback**. That is
the worst kind of agreement: the field that would reveal the problem is the one
field that matches.

**Until a non-WOFF2 CalSansUI exists, every `text_width` and every
content-sized `bounds.w` is measuring the wrong typeface.** There is an escape
hatch (`CROWBAR_ROW_FONT=<path to TTF/OTF>`), verified working with a system
TTF.

### DECISION on the GPUI `ceil()` difference — model it, do not loosen; implementation deferred one measurement

**Rejected (a) loosen `bounds.w`.** Not merely because it is lossy. The
differ worker's argument is decisive and I am adopting it: `ceil()` on a
content-sized box **displaces every following sibling's `bounds.x` by the same
amount, cumulatively**, so loosening `w` leaves the downstream `x` deltas firing
and you must loosen `x` too — by a bound that *grows with the number of
content-sized boxes upstream*. **A tolerance that has to grow with the layout is
not a tolerance.** And the error is strictly one-directional (`ceil` can only
make native wider) while a tolerance is symmetric, so half the slack bought is
pure lost coverage bought for nothing.

**Rejected "give the boxes explicit widths so `ceil()` is a no-op"** for
genuinely content-sized anchors. It makes the component worse to make the test
pass: `git-row-added` renders `+1` or `+12`, and a pinned width is wrong at one
of them. Legitimate only where a box is fixed-width *by design anyway* — which
is a component decision, not an oracle one.

**Adopted (b), reframed correctly: this is a correction, not a loosening.** If
GPUI ceils, the native app **cannot produce** a fractional content width, so
asking "is native within 0.5px of WebKit's fraction" asks a question the engine
is incapable of answering — a delta there is never actionable. Comparing against
`ceil(reference)` moves the *expectation* to the one the engine can meet and
keeps the full ±0.5 around it, so a genuine sub-pixel error on a content-sized
box is **still caught**. That is exactly what (a) gives away.

**And: declare the flag, do not detect it.** Detection is a heuristic on both
sides — `width: auto` and not-a-stretched-flex-item on the DOM, `width: None`
plus a text child in GPUI, both falsifiable by flex-grow. Two extractors each
guessing is precisely the silent divergence this contract exists to prevent, and
a mis-guess is invisible: it either opens a 1px blind spot or invents a delta,
and says neither. Content-sizing is a property of the *component*, which already
authors its anchors on both sides — so it becomes an authored argument
(`anchor_content_sized(...)` / `data-oracle-content-sized`). Cost: one optional
boolean in §3, one rule in §5, one argument per affected anchor. A wrong
declaration is then a visible line in a component, not a subtle engine-reading
difference.

**Implementation deferred by exactly one measurement**, and I tried to take it
from the archived run rather than assume:

```
added:  x=252.0  w=21.0  text_width=20.355   ceil excess 0.645
deleted: x=277.0
gap if the ceil propagated     : 4.000
gap if it did not              : 4.645
```

**Inconclusive.** `4.000` is suspiciously exact and I suspect propagation, but
one sample cannot distinguish a 4px gap that inherited the excess from a 4.645px
gap that did not. **P1.7 is adding `--added N` / `--deleted N`**, which lets me
vary the text width and watch whether the downstream `x` moves by the ceil'd or
the raw amount. That settles it.

Why it matters enough to wait: **if displacement propagates, modelling
`bounds.w` alone is not sufficient** and the differ needs flow order to model
`x` — which it deliberately does not have (§1 rejects trees outright). That
would be a genuine design question, not a tweak, and I am not committing to a
shape before knowing which one I am solving.

One supporting fact that makes this modellable at all: **`text_width` needs no
treatment.** GPUI reports the unrounded shaped advance (`20.355`) and it already
compares correctly against WebKit. Only `bounds.w` inherits the ceil — so this is
a box-sizing artefact, not a shaping one.

### One measured difference that exceeds tolerance and is not fixable app-side

GPUI **`ceil()`s a text run's max-content width** — `elements/text.rs`:
`size.width = size.width.max(line_size.width).ceil()`. WebKit keeps fractional
fractions. Measured on a live pair: `git-row-added` has `text_width 20.355` and
`bounds.w 21.0` — **Δ 0.645, against a ±0.5 tolerance.**

It is systematic, it is in the framework, and it hits every content-sized box.
**Not yet resolved.** The options are a modelled comparison (mark content-sized
anchors and compare against `ceil(reference)`) or a looser `bounds.w` tolerance
— and §5 makes loosening a recordable act requiring a stated loss. I am not
loosening it silently.

### ✅ The reference half of the gate is proven, end to end, by me — 2026-07-31

Not a worker's claim. I brought up an isolated reference app, verified its asset
origin, navigated to the fixture workspace, loaded the extractor, and pulled a
real snapshot out of the live WKWebView.

**68 anchors render** on the git status panel: `git-row-item` ×11,
`git-row-button` ×11, `git-row-name`/`icon`/`badge` ×6, `git-row-added` ×4,
`git-row-deleted` ×1, guides 0–3. The panel reads "6 uncommitted changes".

**All three content lengths of the §8.3 matrix are present in one panel** — the
fixture was designed for this and it worked:

| row | file | length |
|---|---|---|
| 2 | `a.ts` | short |
| 10 | `resolve-terminal-connection.ts` | normal |
| 9 | `an-extremely-long-…-sidebar-row.ts` | **overflow** |

**Reference snapshot, row 9 (overflow), 10 anchors:**

```
git-row-item    294×24 @(0,0)   bg #00000000  r 2  bw 0
git-row-button  294×24 @(0,0)   bg #00000000  r 8  bw 0
git-row-icon     14×14 @(66,5)
git-row-name    104.73×18 @(86,3)  fg #f5f5f5ff  14/400/CalSansUI
                text_width 476.49  clipped TRUE      ← 476px of text in a 105px box
git-row-badge   74.11×16 @(196.73,4)  bg #fe9a0029  r 4  bw 1  10/500
git-row-added   11.16×16 @(276.84,4)  fg #00bc7dff  "+1"
git-row-guide-0..3  7×24 @ x=14,28,42,56
```

**This independently confirms my own correction**: `git-row-button` really is
radius **8** with border width **0**, not the 2px/1px I first told P1.5 from
reading the stylesheet. I measured it myself this time.

`git-row-dir` is absent, exactly as P1.1 reported — confirming the
single-span-truncation scope decision above rather than taking it on faith.

**What remains for the gate:** P1.5's native row, then the native snapshot, then
the differ across the matrix.

> **Bridge payload limit.** `webview_execute_js` times out at 7s returning a
> large `JSON.stringify` of a whole snapshot. Return a **trimmed, structured**
> object instead — the data arrives fine that way. Same limit bites a dynamic
> `import()` that makes Vite compile: the import *succeeds*, the call reports a
> timeout, and the module is there on the next call. Do not read either timeout
> as a failure without re-checking.

### ✅ Phase 1 state axis — closed 2026-07-31, and it found a real defect

The geometry matrix is **18/18 on `git-status-row`** (3 widths × light/dark × 3
content lengths), every cell a live WKWebView capture diffed against a live GPUI
snapshot by me. Σ ceil excess tracks the badge breakpoint — 1.51px at 600 against
1.73px at 800/1100 — which is the evidence that the viewport axis does real work
rather than three cells that happen to agree.

The state axis needed a second surface, because on the git row it is **vacuous**:
no live consumer passes `active`, `focus` paints nothing, and `loading`/`error`
do not exist. `file-tree-row` was built for it and is genuinely stateful.

**`selected` converges exactly** — `file-row-item.bg = #ffffff0a` on both sides.
Driving it needs a two-call ordering that is not obvious: focus
`.file-tree-container` in one call, dispatch the bubbling click in the **next**.
`highlightedPath` depends on `hasTreeFocus`, and React must commit the focus
state before the click, so doing both in one tick silently does nothing.

**The run found one genuine defect (P1.11).** The reference colours filenames by
git status — `a.ts` is modified, so it renders amber `#fe9a00ff` — and the native
row painted default foreground. A missing feature, found by the oracle rather
than by reading code, which is the entire point of having built it. Two of the
other three deltas were **my driving error**, not defects: I left
`--prev-depth`/`--next-depth` at their defaults so no guide capping applied.

P1.11 merged. Verified by me across all seven values, not by its worker:

```
none #f5f5f5ff   modified #fe9a00ff ← the reference value   deleted #f94047ff
added #00bc7dff  modified-staged #00bc7dff  untracked #9ae600ff  renamed #00bcffff
```

`deleted`'s `#f94047ff` is the same red the reference painted on the `src` folder
that aggregated a deletion beneath it, so the folder-rollup path checks out too.
The worker corrected three things my two samples would have got wrong: `staged`
is a **boolean, not a status** (five statuses, six decorations), there is no
`conflicted`/`ignored` colour, and `modified-staged` and `added` are the *same*
colour — distinguishable only by the trailing letter.

**`hover` and `focus` are unobserved**, for one environmental root cause, written
up in `blocked/hover-and-focus-need-an-unlocked-screen.md`. The screen is locked
(`CGSSessionScreenIsLocked = 1`), so no app can be active: `:hover` gets no real
pointer input and `:focus` fails because `document.hasFocus()` is `false`.

> **Two traps that each make a naive focus driver report a false result.**
> First, `btn.focus()` sets `document.activeElement === btn` while
> `btn.matches(':focus')` stays **false** — a driver asserting on
> `activeElement`, the obvious choice, would report focus converged against an
> app painting no focus ring. Second, the rule is **`:focus-visible`**, not
> `:focus`, so `.focus()` would paint nothing *even with the screen unlocked*.
> Focus must be driven by keyboard and confirmed with `matches(':focus-visible')`.

A CSSOM pre-check survives the lock, and both states resolve to exactly the
values the native app already paints: hover `color(srgb 1 1 1 / 0.0272)` → `7` →
`#ffffff07`, focus border `0.0516` → `13` → `#ffffff0d`. `--accent` is
`#ffffff0a`, independently the confirmed `selected` value — the three states are
one token family and the native side has all three right. **This is not a
converged cell**; it tests the stylesheet, not the paint. It does narrow the
residual to whether *geometry* shifts under those states.

**`empty`/`loading`/`error` are vacuous on both gate surfaces**, and that is the
honest close rather than a skip: they are *container* states. A tree can be
empty; a row cannot, because a row that does not exist is not rendered. Driving
them would mean inventing a state the product does not have and then agreeing
with myself about it, which manufactures a green cell.

**New gate: `check-invariants.sh` rule 5 — `cargo fmt --check`.** It was red on
21 files across 7, and had been red before P1.11 touched anything. Formatting was
simply never gated, so drift accumulated one merge at a time until the check was
useless. The worker hit it, correctly declined to run `cargo fmt` because that
would have rewritten the oracle it does not own, and hand-formatted its own lines
instead — right call for a worker, wrong steady state for the repo. Now gated, so
the next drift is one file in the commit that caused it.

> **Two traps that cost real time here, both about *which app you are driving*.**
>
> 1. **The MCP bridge binds port 9223 once.** A stale instance from hours earlier
>    kept it, so `webview_execute_js` was driving that process while I believed
>    it was the one I had just launched. `driver_session status` reporting the
>    right `cwd` does **not** disambiguate — every instance from this worktree
>    reports the same one. Check `lsof -nP -iTCP:9223 -sTCP:LISTEN`.
> 2. **macOS cascades every fresh window to the same origin.** Three windows sat
>    at exactly `(262,122) 1200×800`, two of them a sibling session's from a
>    different worktree. "The window at those coordinates" is not a unique thing
>    to point at. Match `cwd` **and** `CROWBAR_HOME` before driving.
>
> Also: `manage_window info` reports **physical** pixels. `1200×1800 @(3446,2274)`
> is `600×900 @(1723,1137)` in points at dpr 2 — and points are what
> `CGWindowList` and `CGWarpMouseCursorPosition` use.

### §17 coverage — measured 2026-07-31, and one item was over-claimed

Both numbers, reported separately and never blended, exactly as §17 requires.

| crate | lines | missed | line % | |
|---|---|---|---|---|
| `crowbar-core` | 148 | 0 | **100.00%** | ✅ |
| `crowbar-client` | 277 | 1 | **99.64%** | ✅ |
| `crowbar-driver` | 1222 | 24 | **98.04%** | ✅ over the bar, but see below |
| `crowbar-proto` | 6 | 0 | **100.00%** | ⚠ **structurally vacuous** — see below |
| `crowbar-diff-logic` | — | — | — | crate does not exist yet |

**Oracle-corpus coverage, kept separate:** `oracle` 2815 lines, 0 missed,
**100.00%**.

> Read the right column. `cargo llvm-cov --summary-only` prints regions, then
> functions, **then** lines — the line percentage is column 10, not column 7. I
> misread it first and produced a table claiming every crate was below 98%.

**`crowbar-proto` is empty, and item 0.5 was marked done too early.** The crate
is `Cargo.toml` plus a 9-line `lib.rs`: no types, no functions, no
`src/generated/` — and that directory has no git history, so it was never
committed rather than lost. Meanwhile **four crates already depend on it**
(`crowbar-client`, `crowbar-core`, `crowbar-app`, `crowbar-driver`).

0.5's row reads "Go handlers → `crowbar-proto` + regenerated `web/` types", and
what actually shipped was the **generator** — `tools/protogen`, with golden
fixtures and determinism tests, all genuinely good. It was simply never run to
produce the output the row promises. A `-` in a coverage table is not a pass; it
is an absent measurement, and it went unnoticed because nothing consumes the
crate yet.

Dispatched as **P0.5b**, Rust side only. Regenerating `web/` types is
deliberately excluded while the React app is the live reference for an in-flight
parity gate — churning its types would perturb the thing being measured.

**`crowbar-driver` regressed 100% → 98.04%** (24 lines missed, all in
`element.rs`, 94.43%). Still over the bar, so not a violation and not treated as
one, but it is drift in the wrong direction on the crate that carries the gate.

### §17 lint bars — verified adversarially, not just read

`[workspace.lints.clippy]` denies `pedantic`, `unwrap_used`, `expect_used`,
`panic`, `todo`, `unimplemented`, and **all 13 members opt in** with a `[lints]`
section carrying `workspace = true`.

Config looking right is not verification, so I tested it: a temporary
`v.unwrap()` plus a `&Vec<u8>` argument in non-test `crowbar-core` produced
`error: used \`unwrap()\` on an \`Option\` value`, plus pedantic's `must_use`
and missing-`# Panics` errors, exit **101**. Reverted immediately; tree clean.

> A grep that looked for the dotted form `lints.workspace = true` reported **0
> crates** and nearly became a "the entire lint policy is unenforced" finding.
> The manifests use the `[lints]` section form. Check for the section, not the
> dotted key.

### ✅ §17 leak detection — P1.12 merged, and it CORRECTED my finding

I recorded this as *"leak detection was never on — the condition was never met."*
**That was wrong, and wrong in the more interesting direction.** Recorded here
rather than quietly fixed, because the way it was wrong is the finding.

Both crates already request `gpui/test-support`, and `test-support` includes
`leak-detection`. `#[gpui::test]` holds `App::ref_counts_drop_handle()`, whose
`Drop` panics on a surviving entity handle. So a plainly forgotten entity was
**already** caught, before this item. My grep for `assert_no_new_leaks` measured
the wrong thing: absence of an explicit call is not absence of detection.

I also said 11 tests. There are **51** — `element.rs` 11, `row_layout.rs` 40. I
had grepped `crowbar-ui` and `crowbar-driver` and never looked in `crowbar-app`.

The real defect is narrower and worse: **the detector lives inside `App`**, so
anything outliving the harness keeps it alive, and a `Drop` that never runs
reports nothing. `assert_no_new_leaks` reads live state instead and has no such
dependency.

**I verified that by mutation rather than taking it on trust.** The identical
leak — forget an entity, then forget a `cx` clone — in one temporary test:

| | result |
|---|---|
| with `leak_checked!(cx)` | **FAILED** (`entity_map.rs:1080`) |
| same test, only that line removed | **passed green** |

So the hole was real and the guard closes it.

Landed: `crowbar-driver/src/leak.rs` exporting `leak_checked!`, armed on all 51
tests plus 2 controls. The guard parks → `quit()` → parks → asserts, because an
*open window's* root view is indistinguishable from a leak to the detector; it
stands down under `thread::panicking()` so a failing test keeps its own message
instead of aborting. A macro, not a helper, so `let _ = guard(cx);` — a
temporary dropped before the window it must outlive — cannot be written.

**Rule 6** fails any `#[gpui::test]` whose first statement is not the macro. I
mutated it in both directions myself: disarming one existing test **fails**, and
a fresh unarmed test in a brand-new file **fails**. Not a vacuous guard.

Gates I re-ran: **347 passed / 0 failed / 3 ignored**, clippy clean, **7 `ok`
lines**, `crowbar-driver` **98.05%** line coverage (up from 98.04%).

Stated limits, from the worker and worth keeping: rule 6 does not catch a test
that arms the guard on a *different* app than it drives, nor any leak that is
not an entity handle — a detached task, an `Rc` cycle among plain values, native
memory. §17's RSS soak is the check for that class, and it is still outstanding
and mine to run.

### ✅ P0.5b — the DTOs exist, and the coverage gate on them is vacuous

**133 declarations** (123 structs, 10 enums, 1 alias) across 23 modules, emitted
from 163 routes (157 fully resolved, 8 diagnostics in
`native/protogen.manifest.json`). Regeneration is
`native/scripts/regen-proto.sh`; two full runs are byte-identical.

**`crowbar-proto`'s ≥98% coverage gate cannot fail, and that is not a pass.**
`rustc` excludes `#[automatically_derived]` items from instrumentation, so every
derived `Serialize`/`Deserialize`/`Clone`/`Debug`/`PartialEq` is invisible to
`llvm-cov`. I verified this myself rather than accepting it: the report lists
**one file, `null_default.rs`, 6 lines**, against **24 files / 1828 lines / 133
declarations**. The gate would read 100% with every DTO test deleted.

The real assurance is **259 round-trip tests**, not the percentage. §12's number
is reported because §17 asks for it, and it is annotated here and in `lib.rs` so
nobody reads it as evidence.

**A silent drop in protogen, found by running it.** `GET /v0/.../review/outline`
streams its envelope with `json.NewEncoder(w).Encode(v)` — the one v0 payload too
large to buffer — so it touches neither `ctx.JSON` nor a `libs.Write*` helper and
the classifier fell through to **`empty`**: a body-less success, **with no
diagnostic**. Three real wire types (`outlineResponse`, `git.FileOutline`,
`git.HunkShape`) were simply absent. `empty` is the one wrong answer
indistinguishable from a right one in every count the summary prints, which is
why it survived §9.2's "no silent drops" rule.

Verified by mutation, not trust: neutering `matchEncoderWrite` reproduces exactly
`response kind "empty", want "json"`. Delta measured — one endpoint
reclassified, three types added, nothing removed.

**DECISION — `serde_json` as a dev-dependency of `crowbar-proto`.** §4.2 says
that crate may depend on "`serde` only". I accepted the dev-dependency:
`[dependencies]` still contains only `serde`, so the shipped crate's edge is
unchanged and consumers are unaffected; `serde` implements no format, so a
round-trip test cannot be written without one; and `crowbar-driver` already
carries `gpui`+`test-support` as a dev-dependency on identical reasoning. The
alternative was **zero** behavioural verification of 133 DTOs, which is
indefensible given the coverage number above is vacuous. Reversible if you read
§4.2 strictly — say so and it goes.

**Left for the Go side:** five handlers still answer with an untyped `gin.H`
(`untyped-payload` in the manifest). That is the only thing wanting `serde_json`
in the *generated* code, so `regen-proto.sh` drops that module and prints why on
every run — and **fails** rather than drops if a real DTO ever grows an untyped
field, because dropping would then take a live type with it.

### ✅ P1.13 — the hover cell converges across all three axes

Six cells, every one **0 deltas**, with exactly the forgivenesses the resting
cells already earn:

| cell | anchors | note |
|---|---|---|
| 800 · dark · short | 8 | |
| 800 · dark · normal | 10 | |
| 800 · dark · overflow | 10 | truncation engaged, `clipped: true` |
| 800 · **light** · short | 8 | `#00000007` both sides |
| **600** · dark · short | 8 | pre-`sm:` badge; Σ ceil excess **1.51** px |
| **1100** · dark · short | 8 | Σ ceil excess **1.73** px |

The Σ excess splitting 1.51/1.73 at the badge breakpoint is the same signature
the resting matrix produced — independent evidence the width axis is doing real
work rather than three cells that happen to agree.

**This is a construction, not an observation, and it is sound because it is
exhaustive.** Real `:hover` cannot be delivered while the screen is locked, so
the state was forced with a class carrying the byte-identical declaration from
`.file-tree-item:hover::before`. Forcing one rule is only equivalent to hovering
if that rule is *all* hover does, so I proved it: of **127** hover rules carrying
declarations, with **0** unresolvable selectors, exactly **two** apply anywhere
in the git row subtree —

- `.file-tree-item:hover::before { background-color: var(--file-tree-hover-bg) }`
- `.file-tree-row:hover { background-color: transparent !important }` — a
  **no-op**, because the button is already `#00000000` at rest

Nothing else `:hover` can reach. What this does **not** establish is that WebKit
applies `:hover` on real pointer input — bedrock CSS behaviour, not a Crowbar
parity risk, but not an observation either.

**Two instrument bugs, either of which would have produced a confident wrong
answer:**

1. The first enumeration was **unsound**: 40 of 45 "applicable" hits were bare
   `&:hover` from nested CSS with empty declaration blocks, because the walker
   never resolved `&` against its parent chain. An exhaustion argument cannot
   rest on a scanner known to be broken. Fixed by substituting `:is(parent)`.
2. The first capture reported **`visible: false` on every anchor**. The row was
   on-screen by `checkVisibility()`, but `oracleIsVisible` clips against
   ancestors with non-visible overflow, and the git panel was scrolled out of the
   sidebar's scroll-snap carousel. Scrolling it in fixed it.

Reference app restored afterwards: 1200×800, dark, injected style and classes
removed, `web/` clean.

### ⛔ The MCP bridge port is a SINGLE GLOBAL RESOURCE — and a sibling session can take it

Cost me a parity run, so it is written down.

`tauri_plugin_mcp_bridge::init()` takes **no port argument** — `9223` is baked
into the plugin (`desktop/src-tauri/src/lib.rs:848`). Changing it means editing
the reference app, which is out of scope and would change the thing being
measured. So **exactly one Crowbar dev app on this machine can be driven at a
time**, whichever bound the port first.

Mid-session my app died and a **sibling session's** app (`feature/crowbar-skill`
worktree) took `9223`. `driver_session` reports `connected: true` and answers
normally — it is simply attached to somebody else's app. Always confirm
ownership before driving:

```bash
p=$(lsof -nP -iTCP:9223 -sTCP:LISTEN | awk 'NR==2{print $2}')
lsof -a -p $p -d cwd -Fn | grep '^n' | sed 's/^n//'   # must be THIS worktree
```

**And the repo's own `beforeDevCommand` is `pkill -f vite`.** Any sibling
starting Crowbar dev kills my Vite on 5273, after which my app boots to
*"Waiting for your frontend dev server…"* forever. Restart it with
`bun run vite --port 5273 --strictPort` from `web/`.

Both hazards are silent: the first drives the wrong app, the second hangs the
right one. Neither announces itself.

### §17 #6 — the RSS soak has no workload to run yet, and that is sequencing

Checked rather than deferred. `crowbar-app`'s `main` parses one §8.3 matrix
`Cell`, renders one surface, emits a snapshot and exits. It opens no workspace,
connects to no daemon, navigates nowhere, streams nothing. It is a **gate
harness**, not an application.

§17 #6 asks for *"no RSS growth against the React app **on the same
workload**"*. There is no workload on the native side to hold against React —
nothing to soak, and no shared task the two apps could both perform. Measuring
the harness's RSS and calling it a soak would be a manufactured green, in the
same family as driving a state the product does not have.

This unblocks at **Phase 4** (§16: 44 stores → `Entity<T>`, 229 `useEffect`s →
event wiring), which is the first point at which the native app *does* anything
a soak could exercise. Until then the condition is not "skipped" — it is not yet
measurable, and the reason is verifiable in `crates/crowbar-app/src/main.rs`.

What **is** in place already: gpui leak detection armed on all 95 `#[gpui::test]`
tests and gated by invariant rule 6, with the gap it closes proved by mutation.
That is the other half of §17 #6's sentence, and it is done.

### ▶ How to bring up the reference app — **do not use `make dev-desktop`**

`make dev-desktop` is wrong for this work, for two reasons that only show up when
more than one Crowbar is running: its `beforeDevCommand` **`pkill -f vite`** kills
every other Vite on the machine — including other agent sessions' and the user's
— and it hard-codes port 5173, which another worktree may already hold.

I did use it twice before understanding that, and it killed a sibling session's
dev server both times. Use this instead:

```sh
# 1. my own Vite, my own port, from MY worktree
cd <worktree>/web && node ./node_modules/.bin/vite --port 5273 --strictPort &

# 2. clear any orphaned daemon on MY socket first (see the spawn trap above)
kill $(pgrep -f crowbar-6d4f21ce150add3c)

# 3. the app, pointed at my Vite, with beforeDevCommand DISABLED so it
#    does not pkill anyone else's
cd <worktree>/desktop
export CROWBAR_HOME="<worktree>/.crowbar"
bunx @tauri-apps/cli dev \
  --config '{"build":{"devUrl":"http://localhost:5273","beforeDevCommand":""}}'
```

The empty `beforeDevCommand` is the courtesy half; the distinct port is the
isolation half. Both are needed.

> **Enumerate your own app instances before launching another one.** A stale
> Tauri instance keeps its window *and its daemon supervisor*, and that
> supervisor keeps trying to spawn a daemon on the same socket. Launching a
> second app then produces:
>
> ```
> ERROR crowbar daemon terminated (code=Some(1))          ← ×4
> ERROR crowbar daemon died 3 times within 600s; giving up until the next app launch
> INFO  crowbar daemon is ready on …6d4f21ce150add3c.sock (pid 18741)
> ```
>
> — a respawn storm ending in "giving up", *immediately followed by* "ready".
> Both lines are true: one daemon did win the socket, and the supervisor has
> permanently stopped covering it for this run.
>
> This looks identical to the start-a-daemon-by-hand trap above and is **not the
> same bug**. Here nobody started a daemon manually; I simply had an old app of
> my own still alive. Killing it then took the winning daemon down with it (the
> window-close kill path), leaving the new app running against nothing.
>
> **Check first**, and attribute by working directory rather than by pid, since
> other sessions' apps look identical in `pgrep`:
>
> ```sh
> for p in $(pgrep -f crowbar-desktop); do
>   lsof -a -p $p -d cwd -Fn | grep ^n | cut -c2-
> done
> ```

**Verified live 2026-07-31:** 5273 serves from my worktree while another
session's instance keeps 5173 and its own daemon (`crowbar-2978a066…`)
untouched. Two Crowbars, two daemons, no interference.

> **Other sessions are running on this machine.** At the time of writing, a
> `feature/crowbar-skill` instance owns 5173 and daemon `24957`. **Do not kill
> it** — it is not mine. Sibling agent sessions share this repo; the workspace
> model memo already warns they will commit into your worktree, and they will
> take your ports too.

### ⚠ PRE-FLIGHT CHECK before every parity run — worker worktrees do **not** isolate ports

**Run this before diffing anything. It takes one command and it prevents a
whole class of silently-invalid comparison.**

```sh
P=$(lsof -nP -iTCP:5173 -sTCP:LISTEN -t | head -1)
lsof -a -p "$P" -d cwd -Fn | grep ^n | cut -c2-      # MUST be <this worktree>/web
```

> **CORRECTION (2026-07-30). My first diagnosis of this was wrong, and the real
> cause is worse.** I attributed it to a worker's `--strictPort` taking the port.
> It is not that. **The repo's own `beforeDevCommand` runs `pkill -f vite`:**
>
> ```json
> "beforeDevCommand": "pkill -f vite 2>/dev/null; sleep 0.5; cd ../web && npm run dev -- --port 5173"
> ```
>
> — `desktop/src-tauri/tauri.conf.json:9`, verified in **this** worktree and in
> `feature/crowbar-skill`'s. It is committed, so every Crowbar worktree has it.
>
> **Consequence: any Crowbar dev instance starting up kills every other Vite on
> the machine** — the orchestrator's reference app and every worker's dev server
> alike. Whoever starts last wins; everyone else's window keeps rendering
> whatever it had loaded, served by nothing, or silently re-attaches elsewhere.
>
> That is the real reason my reference app died twice. The pre-flight check below
> is still exactly right and still mandatory — I just had the mechanism wrong,
> and a wrong mechanism leads to the wrong mitigation. **Re-check the asset
> origin before every parity run, and expect it to have been killed rather than
> merely contended.**

#### What happened, because it will happen again

Workers run in isolated **git worktrees**, which isolates *files*. It does not
isolate *ports* — and, per the correction above, it does not protect a dev
server from being `pkill`ed by an unrelated app start either. The P1.1 worker
legitimately needed a live React app to run its extractor against and started
one from its own worktree.

My reference app's `beforeDevCommand` then failed to bind 5173, `tauri dev`
reported a non-zero exit, and `make dev-desktop` died. **But the Tauri window
launched anyway and attached to the worker's Vite.** Verified:

```
port 5173 pid 16263 cwd = .../.claude/worktrees/agent-<id>/web     ← NOT my worktree
```

So the "reference app" on screen was rendering **a worker's in-progress branch**.
Had I diffed the native app against it, the comparison would have been
meaningless in a way that produces plausible-looking deltas rather than an
obvious error. Nothing in the app, the daemon, or the oracle would have flagged
it.

#### Rules that follow

1. **Verify the asset origin before every parity run**, with the command above.
   "The app is running" is not the same as "the app is mine".
2. A failed `make dev-desktop` **does not mean no app is running** — the window
   can outlive the failure and silently point at someone else's dev server.
3. Do not race a worker for the port. If a worker holds 5173, either wait for it
   or bring the reference up on a different port and point the webview there
   deliberately.
4. The daemon is safe to share — one daemon on the shared `CROWBAR_HOME` is the
   design (§0). It is the **asset origin**, not the daemon, that must be mine.

### The Phase 1 gate fixture — and three traps that cost real time to find

The gate surface is a **git status row**. It cannot render at all unless the
daemon has a project with a repo that has uncommitted changes, and on a fresh
`CROWBAR_HOME` it has none. Setting that up surfaced three things.

**The fixture, now live:**

| | |
|---|---|
| Project | `oracle-fixture` → `/tmp/crowbar-oracle-fixture` |
| Repo | `demo` → `/tmp/crowbar-oracle-fixture/demo`, default branch `main` |
| Origin | `file:///tmp/crowbar-oracle-origin.git` — **a real bare repo, and this is load-bearing** |
| Dirty state | 6 entries: modified ×3, deleted, staged-add, untracked |

Deliberately spans the §8.3 **content-length** axis so truncation is actually
exercised: `a.ts` (short), `resolve-terminal-connection.ts` (normal), and
`an-extremely-long-file-name-that-must-truncate-in-the-sidebar-row.ts`
(overflowing), the last two nested deep enough that the directory span renders.

> **TRAP 1 — repo adoption silently requires a *reachable* remote.**
> `POST /v0/projects/:id/repos` returns **202 Accepted** and then does nothing
> if the repo has no remote, or a remote that cannot be reached. No repo row, no
> error, and **zero log output** — `grep -ci 'import|discover|adopt|provision'`
> over the whole daemon log returns 0. `runAsync` discards the error
> (`if err != nil { return }`).
>
> I lost three attempts to this: no remote → nothing; a plausible-looking but
> non-existent `github.com/...` remote → nothing; a real local bare repo →
> **worked immediately**. Auto-discovery on project import fails the same way,
> which is why importing a project full of local-only repos looks like a no-op.
>
> Discovery itself is fine — `discover.Repos` walks to depth 3 and a `.git`
> *directory* at depth 2 is well within it. The failure is entirely downstream
> in adoption, and it is invisible.

> **TRAP 2 — do not put a fixture inside `CROWBAR_HOME`.**
> `DELETE /v0/projects/:id` **removed the project's `path` directory from disk**,
> taking the fixture and its `.git` with it. The handler's own comment says
> "Real repository directories are never deleted from disk; only
> crowbar-created worktree directories are torn down", and the fixture was at
> `<home>/.crowbar/fixtures/…` — inside Crowbar's own home, which it plausibly
> treats as its to reap.
>
> Stated precisely, because the distinction matters: **repo** delete was
> re-tested afterwards and correctly left the real directory intact. I have
> **not** tested project-delete against a path outside `CROWBAR_HOME`, so this
> is "do not do that", not "project delete destroys your code".

> **TRAP 3 — `git -C <path>` walks up, and it nearly edited the real repo.**
> Running `git -C "$FX/demo" remote add origin …` moments after that directory
> was deleted did not fail — **it operated on the enclosing Crowbar worktree**,
> and was stopped only by the accident that `origin` already existed there. One
> command away from rewriting the real repository's remote.
> **Verify the directory exists before any `git -C`**, or `cd` into it under
> `set -e`. (Crowbar's remotes and working tree were checked afterwards and are
> untouched.)

### Socket-path contract — `crowbar-client` must reproduce this exactly

`api/internal/core/gateway/transports/socket.go:117`. With `CROWBAR_HOME` set:

```
$TMPDIR/crowbar-<fnv1a64(CROWBAR_HOME)>.sock      // Go: fmt.Sprintf("crowbar-%x.sock", h)
```

with no `CROWBAR_HOME`: `~/.crowbar/crowbar.sock`.

> **TRAP — Go's `%x` does not zero-pad.** A hash with a leading zero nibble
> yields a *shorter* filename. There is a live example on this machine right
> now: `crowbar-83bf8eb85db086d.sock` is **15** hex chars, not 16. A Rust port
> written as `format!("crowbar-{h:016x}.sock")` would look correct, pass review,
> and then fail to find the daemon for roughly 1 workspace in 16 — surfacing as
> a bare connection error with nothing pointing at the cause. Use `{h:x}`.
>
> The socket deliberately does **not** live inside `CROWBAR_HOME`: macOS caps
> `sun_path` at 104 bytes and override homes under
> `~/.crowbar/projects/.../worktree` routinely exceed it. Do not "simplify" this.

`desktop/src-tauri/src/sidecar/mod.rs` already carries this fnv1a64 in Rust —
read it before writing a third copy.

---

## Done

### 0.3 — `gpui` + `gpui-component` skills vendored ✅

Merged `754e6c7b`. `.claude/skills/gpui/` (23 upstream files, 7,685 lines) and
`.claude/skills/gpui-component/` (3 files, 861 lines), from
`longbridge/gpui-component` pinned at `88f102d13654fe25aa2fede076274b6b751a3704`
(content revision `a183e4622b099968d9978796e647e6a75f0f5ac1`), Apache-2.0,
`LICENSE-APACHE` + `PROVENANCE.md` carried into each.

**Orchestrator verification** — not taken on the worker's word: `SKILL.md` for
both skills plus `gpui/references/entity-patterns.md` and
`gpui-component/references/usage.md` were re-fetched from
`raw.githubusercontent.com` at the pinned SHA and hashed. All four
byte-identical. Commit scope re-checked: nothing outside `.claude/skills/`.

> **Environment trap this surfaced:** this machine's `~/.gitignore_global` line 2
> is `.claude/`, so the entire vendor was silently unstageable until forced with
> `git add -f`. The repo's own `.gitignore` has no such rule, so CI and fresh
> clones are unaffected — but **anything new added under `.claude/` on this
> machine needs `-f`**, and a worker who does not notice will report success
> having committed nothing.

Not vendored: upstream's third skill `gpui-component-dev` — it is a contributor
workflow for adding components *to* `gpui-component`, which we are consuming, not
contributing to.

### 0.1 — `native/` Cargo workspace scaffold ✅

13 crates per §4.2 (12 under `crates/` + `oracle`), `edition = "2024"`,
`resolver = "3"`, `rust-version = "1.96.0"`, `license = "AGPL-3.0-only"`.

**Orchestrator verification.** I re-ran all four gates myself — `cargo build
--workspace`, `cargo clippy --workspace --all-targets -- -D warnings`, `cargo
test --workspace` (26 binaries), `./scripts/check-invariants.sh` — all exit 0,
zero warnings.

Then, because a checker that only ever prints `ok` is worthless, I **broke it
three ways and confirmed each is caught**:

| Violation introduced | Result |
|---|---|
| `gpui` path dep appended to `crowbar-core/Cargo.toml` | `FAIL rule 1 (§4.3, D2)` |
| `#![forbid(unsafe_code)]` deleted from `crowbar-terminal` | `FAIL rule 2 (§4.3)` |
| `unsafe { }` with no `# Safety` in `crowbar-platform` | `FAIL rule 3 (§4.2, §12)` |

Tree restored clean after each. The worker had done the same, independently —
this is a second, orchestrator-run confirmation, not a repeat of its word.

Two checks are **stronger than the spec asks**: rule 1 also greps
`crowbar-core/src` for `gpui::` (not just the manifest), and rule 2 asserts
`crowbar-platform` positively carries `deny(unsafe_op_in_unsafe_fn)` rather than
merely lacking the forbid.

> **§4.3 rule 3 — token sealing — is NOT yet enforced.** It cannot be: it is a
> property of `crowbar-ui`'s type definitions, and `crowbar-ui` is an empty
> crate. This is the one anti-reward-hacking guard §6.1 calls "strictly
> stronger" than the oracle, and it does not exist yet. **It must land with the
> first line of `crowbar-ui`, in Phase 2, before any component work is
> dispatched** — not after. Tracked here so it cannot be quietly skipped.
>
> **And sealing the newtype is not sufficient on its own.** 0.4 wired `gpui`
> into `crowbar-ui`, which now re-exports it (`pub use gpui;`) so the leaf view
> crates reach it transitively. That means `crowbar_ui::gpui::rgb(0x1e1e1e)` is
> reachable from every view crate, and `.bg(rgb(…))` on a raw gpui element
> bypasses `Theme` entirely — a private inner field on `Color` does nothing to
> stop it.
>
> This hole is **inherent, not caused by the re-export**: no view crate can
> render without gpui's colour types in scope. So the guard has to be a check,
> not a type. `scripts/check-invariants.sh` must grow **rule 4**: no
> `rgb(`/`rgba(`/`hsla(`/`Hsla {` construction anywhere outside
> `crates/crowbar-ui/src/theme/`. Mechanically checkable, and it is the form
> §6.1's claim actually requires.

Known limit, stated in the script's own header: rule 3 is a line scanner, not a
parser. Block comments and string literals containing `unsafe {` produce false
*positives*. It fails loud rather than silent, which is the right direction.

### 0.2 — vendor + pin `gpui` and `gpui-component` ✅ · **the critical path, cleared**

**Pins**, chosen in the required order (gpui-component first, Zed SHA read out of
*its* lock):

| | |
|---|---|
| `gpui` | `zed-industries/zed` @ `1a246efd7e1b83ab568ec5e3e6c1a43a42e1abba` (2026-07-15) |
| `gpui-component` / `-macros` / `-assets` | `longbridge/gpui-component` @ `88f102d1…` (2026-07-30) — same SHA 0.3 used |

**29 crates, 740 files, 238,543 lines, 13.0 MiB.** 21 of the 29 compile on
macOS; the other 8 are carried so manifests stay loadable (Cargo resolves
target-conditional deps for *all* targets, so `gpui_linux`/`gpui_windows`/
`gpui_web` must exist even though we build only macOS).

**Orchestrator verification — my own cold release build:**

```
$ cd native/vendor && cargo build --release -p gpui-vendor-probe --locked
    Finished `release` profile [optimized] target(s) in 6m 41s

├── gpui v0.2.2            (native/vendor/gpui)
├── gpui-component v0.5.2  (native/vendor/gpui-component)
└── gpui_platform v0.1.0   (native/vendor/zed-deps/gpui_platform)
```

`--locked` passing means the committed lock **is** the resolution. Both
libraries resolve from `native/vendor/`, so the collision hazard is structurally
impossible — nothing points at a Zed checkout.

**Shape: extracted subtree with *de-inherited* manifests.** `cargo vendor` was
rejected (drags the entire crates.io graph to disk for 13 MiB of Zed source),
and copying verbatim does not work either — the crates inherit `edition`/`lints`
and ~80 dep specs from Zed's root, and **Cargo refuses a `path` dep into a
nested workspace** (`is a member of the wrong workspace`). Every
`workspace = true` was replaced with its concrete value.

> **The edit that mattered most:** gpui-component's own workspace pinned Zed
> with a **floating** ref — `gpui = { git = "…/zed" }`, no rev. Left alone, our
> "pinned" tree would have silently tracked Zed `main`, which is the exact
> failure §10.5 exists to prevent. Every such entry now points at the vendored
> path.

**`gpui` features confirmed through the resolver, not just the manifest:**
`test-support` → `leak-detection` → `backtrace` (§12 requires leak detection on
in every test). `inspector`, `screen-capture`, `input-latency-histogram`,
`profiler` all present.

Zed pins **Rust 1.95.0** at this SHA — no conflict with our 1.96.0, and its
`rust-toolchain.toml` was deliberately not vendored.

Worth noting: the worker's probe failed its first cold compile on a missing
`use` in **its own probe source**, after all 29 vendored crates had compiled
cleanly, and it recorded that in `PINNED.md` rather than quietly fixing it.

### 0.5 — `protogen`: Go handlers → Rust serde DTOs + TypeScript ✅

Go tool at `native/tools/protogen/`, built on `go/packages` with full type
checking (not regex). **131 types across 22 modules**; 155/161 endpoints fully
resolved; 73/73 request bodies; 74/79 response payloads.

**Orchestrator verification — I ran all four gates:**

| Gate | Result |
|---|---|
| `go test ./...` | `ok …/internal/gen 10.3s` ✅ |
| Generated Rust compiles standalone (temp crate, `serde` only) | `Finished dev profile`, zero warnings ✅ |
| Generated TS typechecks (`--strict`, repo's own `tsc 5.9.3`) | exit 0 ✅ |
| **Determinism** — two full runs, `diff -r` | **byte-identical** ✅ |

**Design calls worth keeping, all verified rather than assumed:**

- **Enums are open sets.** A Go named string type's zero value `""` is legal and
  no constant declares it, so Rust gets an untagged `Other(String)` and TS a
  `(string & {})` member. A closed enum would fail to deserialize valid daemon
  output — a runtime error discovered late, which is exactly what §9.2 exists
  to prevent.
- **Nil slices/maps marshal as `null`, not omitted**, and serde refuses `null`
  for `Vec`/`HashMap`. Non-optional containers deserialize through a generated
  `null_to_default` helper; TS types them `T[] | null`.
- `int64`/`uint64` stay JSON **numbers** — checked, the daemon uses no `,string`
  tags. `time.Time` → `String` (RFC 3339 via its own marshaller, so no `chrono`
  dependency). `[]byte` → `String` (base64).
- **No silent drops.** An unlowerable field gets an `INCOMPLETE:` banner in
  *both* languages and marks every endpoint transitively reaching it as
  unresolved. That is what moved "fully resolved" from 156 down to an honest 155.

**8 diagnostics — the fix list for the Go side, in priority order:**

1. **`gin.H` response payloads (5 endpoints)** — `/v0/health`, `…/identity`,
   `PATCH …/review`, and both `POST …/terminals`. `map[string]any` has no static
   shape. Four return a 1–3 key object, so naming them is cheap.
2. **Anonymous struct nested in a DTO (1)** —
   `UpdateProviderPreferencesRequest.Providers []struct{…}`. A *top-level*
   anonymous body is fine (protogen names it after the handler); one nested
   inside another type has no name to hang on.
3. **`MarshalJSON` on a wire type (1)** — `domain/git.FileDiff`. Its marshaller
   happens to be shape-preserving, but that is unprovable statically.
4. **5 WS routes injected as `gin.HandlerFunc` values** — correctly classified,
   but it means **WS frame shapes are invisible to codegen**. If the port wants
   typed WS frames, that needs a declared handler or a separate declaration.
   Worth knowing now, since the native client is WS-heavy.
5. `libs.Envelope.Data any` is handled, but one hand-rolled
   `c.JSON(status, libs.Envelope{…})` in `identity.go` needed a special case.
   Routing every response through the `libs.Write*` helpers stops that recurring.

### 0.9 — Zed extractability audit ✅ · every verdict compiled, not reasoned about

Audited `zed-industries/zed` @ `b6b2148b` (2026-07-30) in scratch workspaces.

| Crate | Closure | gpui? | Verdict |
|---|---|---|---|
| **`fuzzy_nucleo`** | 17 crates; **3** excl. gpui | 2 methods | **TAKE** |
| **`fuzzy`** | 16; **2** excl. gpui | 2 methods | **TAKE** (dep of the above) |
| **`refineable`** | **0** in-repo deps, 680 lines, Apache-2.0 | **no** | **TAKE**, if we want override layering |
| `util` | 4 crates / 2,819 lines, Apache-2.0 | **no** | Cherry-pick modules only |
| `theme` | 5 excl. gpui | yes | **Skip — on design grounds** |
| `picker` | **90 crates / 500,945 lines** | yes | NOT EXTRACTABLE |
| `editor` | **96 / 518,077** | yes | NOT EXTRACTABLE |
| `language` | **47 / 201,696** | yes | NOT EXTRACTABLE |
| `terminal` | **41 / 193,535** | yes | NOT EXTRACTABLE as-is |
| `ui` (bonus) | 22; 9 excl. gpui | yes, hard | Skip — 95 errors off-rev |

**`fuzzy` is extractable, proven twice.** Its *entire* gpui coupling is two
methods at four call sites: `executor.num_cpus()` and `executor.scoped(…)`. No
`App`, no `Context`, no `Entity`, no rendering. Built and tested unmodified
against stock `gpui 0.2.2` (9/9 tests pass), **and** built with gpui removed
entirely behind a 35-line executor shim (9/9 again, 27 packages total).

**But take `fuzzy_nucleo` instead — it was not on the spec's list and it is the
one we want.** Zed has already migrated off `fuzzy` for exactly our two use
cases; 8 crates now use `fuzzy_nucleo` (`command_palette`, `file_finder`,
`outline`, `tab_switcher`, `git_ui`, …) against 27 still on `fuzzy`. It carries
tuning we would otherwise rediscover — from its own header: nucleo-level
matching must be case-*insensitive*, because `CaseMatching::Smart` **rejects**
candidates whose capitalization differs, breaking a palette lookup of
`"Editor: Backspace"` against an action named `"editor: backspace"`. `Case::Smart`
is honoured as a scoring *hint* instead. Its lib builds clean against 0.2.2; its
23 tests need a matching gpui rev.

**Two prior expectations refuted — same verdict, wrong reason. Recorded so they
are not re-litigated:**

- **`terminal` is not blocked by `project`/`workspace`/`multi_buffer`** — it
  depends on none of them. Its 193k closure is driven almost entirely by
  **`settings` + `theme_settings`** (26 call sites) dragging ~190k lines of
  config plumbing behind a 9,552-line crate. Substituting our own types is
  plausibly a 1–2k-line diff, so this is *not* impossible the way `editor` is —
  but we already ship a working terminal on a daemon-side VT model, so the trade
  is bad. Verdict stands; reclassifiable if anyone wants to pay ~2k lines.
- **`language` is not blocked by `project`/`workspace`/`multi_buffer` either.**
  Its real blockers are `settings`, `lsp`, `rpc` (Zed's collab protocol), `fs`,
  `grammars`, `telemetry`, `zeta_prompt`.

**`picker`'s weld is one line**, which is the interesting part:
`impl<D: PickerDelegate> ModalView for Picker<D> {}` — `ModalView` belongs to
`workspace` (49,498 lines). Only 6 `workspace::` and 2 `project::` call sites in
the whole crate, yet the closure is half a million lines. Take the 134-method
`PickerDelegate` **shape** as a design reference; build on gpui `uniform_list`.

**`theme`: skip, and the reason is our own sealing property.** `ThemeColors` is
**143 `pub` fields, every one a bare `gpui::Hsla`**; `StatusColors` adds 42 more.
~190 colours against our token surface — *not* a superset, so we would be adding
fields, not filling a chassis. Sealing is zero and cannot be retrofitted from
outside: `gpui::Hsla` has public fields and free constructors, so wherever gpui
is in scope `div().bg(rgb(0xff0000))` compiles. Our `Color(Hsla)` would have to
wrap Zed's fields anyway — at which point we have written the struct ourselves
and `theme`'s only residual contribution is the `Refineable` derive. **So take
`refineable` directly** (Apache-2.0, 680 lines, no gpui, no in-repo deps).

What `theme` actually *is*: machinery for **user-authored JSON theme files** —
`ThemeRegistry` loading off a `gpui::AssetSource`, `Option<T>` refinements for
partial overrides. If Crowbar ever ships user-editable themes that is the
argument for revisiting. Otherwise it is dead weight.

**`util`: Apache-2.0, no gpui, 132 tests pass — but do not take `command/`.**
`command/darwin.rs:497` calls `smol::process::Child::adopt_raw_pid`, which
**does not exist upstream** — it is Zed-fork-only and forces their
`async-process`/`async-task` patches into *our* workspace root. Dropping that
module (1,247 of 9,689 lines) removes the requirement entirely. Two more traps:
`util_macros` depends on `perf`, which lives at **`tooling/perf`, not
`crates/`** — a path map built from `crates/*` misses it; and `util` needs
**rand 0.9**, not 0.8.

**Fuzzy-matching fallback, decided now so it is not reopened:** `nucleo` 0.5 /
`nucleo-matcher` 0.3.1, **MPL-2.0**, quiet releases but commits through
2026-06-22 — what Zed itself migrated to. `fuzzy-matcher` (SkimMatcherV2) is MIT
but untouched since 2020-10-04; `skim` is a full TUI and its matcher *is*
`fuzzy-matcher`; `sublime_fuzzy` reports a **non-standard** licence, do not use
without review. Taking `fuzzy_nucleo` gets us nucleo anyway. MPL-2.0 is
file-level copyleft and is standardly read as compatible with (A)GPL-3.0 via
MPL 2.0 §3.3 — fold into the same review as the relicense.

### 0.6 — `GET`/`PUT /v0/settings/ui` ✅ · the ONE daemon exception, and it is closed

1,228 lines: handlers + `domain/ui_settings.go` + routes on `settingsRG` +
399 lines of unit tests + 445 lines of black-box tests in `api/tests/`.

**Orchestrator verification — I drove the endpoint against a live daemon**, on a
scratch `CROWBAR_HOME`, rather than reading the worker's test output:

| Behaviour | Result |
|---|---|
| `GET` a scope never written | `200 {}` — **not** 404 ✅ |
| `PUT` nested object + array + `null`, then `GET` | exact round-trip ✅ |
| Second `PUT` with fewer keys | replaces wholesale ✅ |
| Two scopes | isolated ✅ |
| `PUT [1,2,3]` / `"str"` / `42` / `null` | `400` on all four ✅ |
| `PUT` a 3 MB body | `413`, "exceeds the 256 KiB per-scope limit" ✅ |
| Kill daemon, restart, `GET` | **both scopes survive** ✅ |
| Where it landed | table `ui_settings` **inside `view.db`** — no side file ✅ |

And a detail worth more than it looks: **key order survived the round-trip.**
`{"theme","sidebar","recent","nested"}` came back in insertion order. A Go
`map[string]any` re-marshal would have sorted them alphabetically. So the value
is stored as **raw bytes** — genuinely opaque, exactly as §9.3 demands, rather
than opaque-in-intent-but-parsed-in-fact.

`go test -race` on the new handler package and the black-box suite both `ok`
under my own run.

> **Scope taxonomy — a real discovery, and the store names lie.** §9.3 lists
> four stores as though the split were obvious. It is not:
> `ui-preferences` and `sidebar-ui` are both single **`global`** rows;
> **`workspace-hierarchy` is keyed by REPO id despite its name**; only
> `workspace-layout` is genuinely per workspace. So scope has three forms, and a
> Rust client that trusted the store names would key `workspace-hierarchy`
> wrongly and silently lose hierarchy on every repo with more than one
> workspace.

#### Wire contract — what `crowbar-client` must implement

```
GET /v0/settings/ui?scope=<scope>
  200 {"success":true,"data":{...}}     # {} when nothing stored — NEVER 404
  400 bad or missing scope

PUT /v0/settings/ui?scope=<scope>       Content-Type: application/json
  body: a JSON object, stored verbatim, replaces wholesale
  200 {"success":true,"data":{...}}     # echoes what was stored
  400 non-object body (array/scalar/null/malformed/empty) or bad scope
  413 body > 262144 bytes
  500 store failure
```

`scope` ∈ `global` | `repo:<repoId>` | `workspace:<workspaceId>`. Ids are 1–64
chars of `[A-Za-z0-9._-]`; the whole scope string ≤ 128. Anything else is `400`
on **both** verbs.

Evidence for the three-way split, from `web/src/lib/persistence/`:
`ui-preferences.ts` and `sidebar-ui.ts` both `db.put(…, 'global')`;
`workspace-hierarchy.ts` keys on `repoId`; only `workspace-layout.ts` keys on
`workspaceId`.

#### The 256 KiB cap is load-bearing architecture, not a round number

I verified the reasoning behind it because it encodes a real constraint.
`WorkspaceLayout.buffers` is `PaneContent[]`, and in
`features/panes/types/pane-content.ts`:

```ts
export interface EditorContent extends PaneContentBase {
  content: string        // full file text
  savedContent: string   // ...and a second full copy
```

plus `TokenEntry[]`, a syntax-highlighting cache. **Every open editor tab
persists two complete copies of its file, plus tokens.** That is why today's
persisted layout blob reaches megabytes.

The cap is deliberately set *below* what that blob reaches. So the native client
**cannot** port `workspace-layout` as-is — it is forced to persist *references*
(path, cursor, scroll, pinned/preview flags) and let the daemon own file
content, which is the correct architecture and the one D6 implies anyway.
Treat a `413` here as the design working, not as a limit to raise.

Real payloads for the other three stores are tiny (`UIPreferences` is 6 scalars,
`SidebarUI` two id lists, `WorkspaceHierarchy` `{wsId,parentId}` pairs), so the
cap is ~25× the largest realistic value for them.

#### Concurrency — the mutex is not decorative

Per-scope `sync.Map` of mutexes. GORM's `Save` emits `UPDATE …` and only falls
back to a separate `INSERT` when it matches zero rows, so **two first-writes to
a fresh scope can both see zero rows and both insert.** Removing the mutex makes
`TestPutUI_SameScopeWritesAreSerialised` fail with observed overlap 4. Distinct
scopes never contend — proven by a rendezvous test that deadlocks if they shared
a lock.

#### Known gap, deliberately left

Nothing reaps `workspace:<id>` scopes when a workspace is deleted. Rows are a
few hundred bytes and orphans are inert. `DELETE /v0/settings/ui` is the obvious
follow-up if the cascade should reap them — **not** done, and recorded rather
than silently skipped.

> **Pre-existing red gate, verified by me at HEAD — not inherited from 0.6.**
> `TestRouteAudit_AllSpecRoutesRegistered` (build tag `integration`) fails on a
> clean tree: **161 registered routes vs 159 expected**, from two routes
> registered but absent from the spec list —
> `POST /v0/projects/:projectId/repos/:repoId/workspaces/import` and
> `GET /v0/projects/:projectId/repos/:repoId/pull-requests`. I reproduced this
> before merging anything. `/v0/settings/ui` is **not** among the undeclared
> routes, which is the check that actually matters here.
>
> This is not the port's bug, but it **is** a red gate that will mask a genuine
> route regression from this port later, so it is not something to shrug at. It
> belongs to whoever added those two routes. Filed as
> `native/oracle/blocked/route-audit-red-at-head.md`.

### 0.7 — loopback TCP listener, token-authed ✅

New leaf package `api/internal/core/loopback/` + `transports.NewLoopback`.
**Off by default** (`--loopback-tcp` / `CROWBAR_LOOPBACK_TCP`), so nothing about
today's behaviour changes.

> **What auth existed before: none.** Verified. The unix socket's *entire* access
> control is `chmod 0600` on the socket file
> (`transports/socket.go:52`). `middleware.CORS()` + `origin.go` is a
> **browser-only** control — it decides whether a browser may *read* a
> cross-origin response, and stops nothing for a local non-browser process,
> which sends no `Origin` at all (`Allowed("") == true`). Over TCP there was
> nothing whatsoever. So §9.4's "carry the existing daemon auth" had nothing to
> carry; the token mechanism is new.

**Orchestrator verification — I ran the real binary and inspected the sockets:**

| Check | Result |
|---|---|
| Off by default | no `loopback.json`, **0** TCP listeners on the pid ✅ |
| Bind address (`lsof`) | **`127.0.0.1:53292` only** — no `0.0.0.0`, no `::` ✅ |
| Reachable on the LAN IP `10.10.128.12`? | connection **refused** ✅ |
| `loopback.json` mode | `-rw-------` ✅ |
| Token | 43 chars = 256 bits base64url ✅ |
| no credential / wrong / empty bearer | `401` / `401` / `401` ✅ |
| bearer / `X-Crowbar-Token` / query param | `200` / `200` / `200` ✅ |
| **static asset** and **`/`** with no credential | `401` / `401` ✅ |
| **WS upgrade** with no token | `401` (written *before* the hijack) ✅ |
| **unix socket** with no credential | `200` — unchanged ✅ |
| Non-loopback binds | all 11 subtests refuse, **including `localhost`** ✅ |

Refusing the literal hostname `localhost` is the sharp call and it is right: a
hostname is OS-resolved and `/etc/hosts` can point it off-box. It refuses rather
than warns — the pre-existing `NewTCP` only `slog.Warn`s, and its own doc
comment calls that API "unauthenticated".

**It also fixed a real pre-existing bug while in there.** The listener was bound
*before* the engine/adapter/app wiring, so any failure in that wiring returned
with the listener still open. On the unix path that leaves a socket the next
launch dials successfully and reads as `ErrDaemonRunning` — **a dead daemon
squatting the address**, which is a plausible cause of past "daemon won't start"
reports. Fixed with a named error return + `defer closeOnFailure`, plus
`container.Close()` so half-built PTYs/watchers/sqlite handles aren't stranded.
Regression test: `TestNew_FailureAfterBind_ReleasesTheUnixSocket`.

Nice instinct worth keeping: `api/tests/loopback_tcp_test.go` deliberately
carries **no `integration` build tag**, unlike every other file in that package,
so "who may reach the daemon" is gated on every default `go test ./...` rather
than on a tag nobody runs. Given §0.6's finding that the tagged route audit has
been failing unnoticed, that is exactly the right call.

> **DECIDED (mine, not a user decision): webview panes get a request
> interceptor, NOT a session cookie.**
>
> The problem 0.7 correctly surfaced: a browser *document navigation* and its
> subresource loads cannot set headers, so `webview.load("http://127.0.0.1:PORT/route")`
> 401s on every JS/CSS chunk. Header and query-param auth cover XHR and
> WebSocket (the `WebSocket` API cannot set headers either, which is why the
> query param is load-bearing rather than convenience) — but not chunk fetches.
>
> The obvious fix is Jupyter's: set a session cookie on successful query-param
> auth. **Rejected.** Cookies are not port-scoped — any `127.0.0.1:*` origin
> would receive it. This machine routinely runs several local dev daemons at
> once, so that is not theoretical; it hands our token to any other local
> server's page and reintroduces CSRF surface on state-changing routes.
>
> `wry` is WKWebView on macOS and can intercept/decorate requests, which keeps
> the credential out of any ambient store and is port-scoped by construction.
> **`crowbar-webview` owns this**; it is a Phase 3 implementation detail, not a
> blocker, and not a product decision.

### 0.11 — `cargo tree -i zlog` ✅ · **the alleged GPL chain does not exist in the released crate**

§15 asked for this "for the record", against zed-industries/zed#55470's claim
that `gpui → sum_tree → ztracing → zlog` pulls GPL into a nominally Apache-2.0
crate. Resolved a lockfile for `gpui = "0.2.2"` (**704 packages**) and asked:

```
$ cargo tree -i zlog       → error: package ID specification `zlog` did not match any packages
$ cargo tree -i ztracing   → error: package ID specification `ztracing` did not match any packages
$ cargo tree -i sum_tree   → error: package ID specification `sum_tree` did not match any packages
```

**None of the three is present at all** — not `zlog`, not `ztracing`, and not
even `sum_tree`, the head of the alleged chain. The published crate does not
carry it. (This is expected in hindsight: crates.io requires every dependency to
itself be published, and none of those three are.)

So the concern is void for **Config 1** regardless of the D1 relicense — which
was going to make it moot anyway.

> **UPDATED after 0.2 chose Config 2 — and for the config we actually adopted,
> the chain is REAL.** I re-checked the vendored tree directly:
>
> | Crate | Licence in the vendored tree |
> |---|---|
> | `zlog` | **GPL-3.0-or-later** |
> | `ztracing` | **GPL-3.0-or-later** |
> | `ztracing_macro` | **GPL-3.0-or-later** |
> | `sum_tree` | Apache-2.0 |
> | `path` | **GPL-3.0-or-later** ← 0.9 reported this as Apache-2.0. It is not. |
>
> So zed#55470's `gpui → sum_tree → ztracing → zlog` is genuinely compiled into
> our binary, plus a fourth independent GPL edge via `http_client → util → path`.
>
> **This is legally fine and was anticipated: D1 exists precisely for it.**
> AGPLv3 §13 permits combining with GPLv3. It is not a blocker.
>
> **But it is an obligation, not a non-event.** `NOTICE.md` must list `zlog`,
> `ztracing`, `ztracing_macro` and `path` as GPL-3.0-or-later. The 0.12
> relicense swept our *own* licence surface and deliberately left third-party
> notices alone — correctly, since none of this was vendored yet. It is now.
> **Tracked as a Phase 1 prerequisite.**

### 0.8 — `.app` bundling ✅ · **DECIDED: `cargo-packager`, pinned to `0.11.8`, driven by our own script.**

Not chosen on impressions. The worker installed it and **actually built** a
universal, signed `Crowbar.app` + DMG with a universal sidecar, then inspected
the result: both Mach-Os `x86_64 arm64`, sidecar signed separately with the
hardened runtime (inside-out order), `codesign --verify --deep --strict` valid
and explicitly validating the nested sidecar, `Info.plist` carrying
`CFBundleIconName` for the Liquid Glass icon. Criterion 1 demonstrated.

Why it wins:

- Its signing path **is literally** `docs/macos-code-signing.md`'s:
  `codesign --options runtime --timestamp` → `notarytool submit` →
  `stapler staple`. **5 of the 6 CI secrets are read from identical env vars.**
- macOS DMG naming is **byte-identical to Tauri's** (`Crowbar_{v}_universal.dmg`),
  so `arrow.yaml` and the workflow rename step need no change.
- ~110 lines to maintain vs ~300 hand-rolled, and we keep AppImage/deb for free.
- Same lineage we already ship — `cargo-bundle → Tauri Programme → CrabNebula`.
  The risk is not new; it is the risk we already carry via `tauri-bundler`.

**Zed is the prior art and it argues against hand-rolling.** Zed does *not*
hand-roll: `script/bundle-mac` installs a **Zed-owned fork of `cargo-bundle`**,
and that fork has been *archived since 2025-03-25* while Zed still installs from
it. Their answer to bundler risk was freeze-it-and-own-the-fork — exactly the
escape hatch we keep. Zed also does not ship universal at all (separate
`Zed-x86_64.dmg` / `Zed-aarch64.dmg`, no `lipo`), so their 340 lines are
*less* than our requirement, and the script is welded to Zed-specific machinery.
Not liftable.

`cargo-bundle` upstream was evaluated and **rejected on evidence**: grepping its
`src/` for `codesign|notarytool|stapler|lipo|universal` returns zero hits outside
`hdiutil`. It is the hand-rolled option with a helper.

**Maintenance risk, stated honestly rather than waved away:** no release since
2025-11-27, no commit since 2026-03-21, and its own docs still say "public
preview". It is *slowing badly, not dead* — CrabNebula staff still answer issues,
though a fix promised 2026-04-24 has not shipped. Mitigated by pinning `0.11.8`,
keeping the driving logic in **our** script, and the MIT/Apache-2.0 licence: the
~1k lines of `package/{app,dmg}` + `codesign/macos.rs` are vendorable into
`native/tools/` the day we need them. Not eliminated.

**Five traps found by running it, all of which would have cost real time:**

1. `--config` **replaces**, it does not deep-merge like Tauri's. The current
   script's jq-overlay idiom must become jq-merge-into-a-full-temp-config.
2. `CFBundleVersion` defaults to a UTC **timestamp**, not the version. Must be
   overridden via `infoPlistPath`.
3. **The empty-secret trap survives.** `try_sign` does
   `env::var_os("APPLE_CERTIFICATE")` with no emptiness guard, so an unset CI
   secret expanding to `""` yields `Some("")` and still calls `setup_keychain("")`
   — the exact failure `build-macos-dmg.sh:60-66` exists to prevent. **Carry
   that `unset APPLE_*` block over verbatim.**
4. `create-dmg` is **downloaded at package time** from raw.githubusercontent.
   Its Finder AppleScript fails headless (`AppleEvent timed out -1712`) unless
   `CI=true`. GitHub Actions sets it; a local release build does not.
5. `APPLE_SIGNING_IDENTITY` is the one secret not read from env — it goes
   through config. The existing script already injects it that way.

**Linux artifact names differ** (`crowbar_0.5.0_x86_64.AppImage` vs Tauri's
`Crowbar_0.5.0_amd64.AppImage`), breaking `arrow.yaml:107`. The worker could not
tell from the spec whether Linux stays in scope; **it does** — §14 says GPUI
requires Vulkan and the *React build* is the fallback for machines without it,
which means the native app still ships Linux artifacts for machines with it. So
this rename must be handled.

Also settled, cleanly: **there is no updater to lose.** Zero hits for
`TAURI_SIGNING`, `createUpdaterArtifacts`, `latest.json`, `plugin-updater` or
`checkUpdate` anywhere, and zero `updater` in `Cargo.lock`. Updates go through
Quiver re-downloading the DMG. Dropping Tauri costs nothing here.

### 0.10 — AX spike ✅ · **VERDICT: THIN. Dropped, per §10.4. Do not revisit.**

§10.4 said: spend one hour, and if the tree is thin, "drop it and never revisit."
It is thin. Two disqualifying facts, both read from GPUI source on `main`:

1. **The AX tree is opt-in per element, not the element tree.** A node appears
   only if it has *both* a `GlobalElementId` (`.id()` set on it *and* its whole
   ancestor chain) *and* a non-`None` `a11y_role()`. `_accessibility.rs` states
   it outright: "nodes with no role are not reported." Zed itself annotates
   roughly ten files' worth — button, icon_button, dropdown_menu, keybinding,
   input_field, number_field. Annotating every element of *our* app is the same
   labour as writing the extractor, for a strictly worse data model.
2. **No colour, at all.** AccessKit's node model has no colour concept and the
   GPUI integration writes none. Our oracle diffs geometry **and colour**
   (§8.1). This alone ends it.

The planned feature-gated in-process driver (D7) stands unchanged.

**Two things worth stealing for `crowbar-driver`, and they are worth a lot:**

- **The identity scheme.** `NodeId` is derived deterministically from
  `GlobalElementId::accesskit_node_id()` — the composed path of ancestor ids.
  Stable across frames and across runs *by construction*. That is exactly the
  anchor identity a cross-app differ needs, and it is a solved problem we can
  copy rather than invent.
- **`Window::debug_a11y_tree_json()`** (`window/a11y/debug.rs`, 330 lines,
  `main` only — we get it because 0.2 vendors from `main`). Retains the last
  `TreeUpdate` and serialises on demand, **in-process, with no AX permission**.
  Under `debug_assertions` it additionally records per node the `Render` view
  type name, the originating `ElementId`, whether the node was synthetic, and
  the **`#[track_caller]` source location of the constructing element**. That
  last field is direct precedent for anchoring a UI node back to source. Use
  this module as the template for the extractor's serialisation layer.

Also observed, and useful: `ZED_EXPERIMENTAL_A11Y` **is** honoured by stable
1.6.3 (the binary links `accesskit 0.24.0` / `accesskit_macos 0.26.0` and carries
the tree-builder's log strings) — it is not a `main`-only flag. And `open -na`
does **not** pass environment through; launch `Contents/MacOS/zed` directly.

> **Not completed, stated plainly:** the live AX dump was never produced. Both
> Zed *and* the TextEdit control returned `kAXErrorAPIDisabled (-25211)` with
> `AXIsProcessTrusted() = false` — a macOS TCC permission wall, not an empty
> tree, and the control run is what proves the difference. Finishing it needs a
> human to add a bundle under System Settings → Privacy & Security →
> Accessibility. **I did not ask for that**, because the verdict does not depend
> on it: opt-in annotation and absent colour disqualify the approach whatever
> the live tree looks like. Every point-by-point answer above is source-read,
> not observed, and is labelled as such.

## Phase 1 — CLOSED, gate passed 2026-07-31

Full write-up in **[`PHASE1-REPORT.md`](PHASE1-REPORT.md)**. Summary:

- resting matrix **18/18**, hover **6/6**, `selected` converges exactly
- `empty`/`loading`/`error` vacuous (container states, not row states)
- `focus` **provably** vacuous on the gate surface (CSS scope + `outline: none`)
- `hover` reproduced by a proven-complete construction, not observed — the screen
  is locked and six approaches were measured to fail
- **8 of 9 anchors** exercised; `git-row-dir` never renders in the product (F3b)

Verbatim, per F3b: **the Phase 1 gate proves the mechanism on single-span
truncation. It does not prove two-span.**

Phase 2 is authorised. Three things carry forward: two-span truncation as a
Phase 2 component, `hover` re-run as a real observation once unlocked, and the
§17 RSS soak once a real workload exists.

## Phase 2 — in progress

### ✅ P2.1 `dropdown-menu` — merged, and it set the pattern

Branch `native/p2.1-dropdown-menu` @ `3395a7b5`. 650 tests (+44), clippy clean,
**7 `ok` lines** with rule 6 now covering **66** gpui tests.

**Phase 1 did not regress.** The refactor rewrote `row_surface.rs` and
`row_layout.rs` — the gate's own code — so I regenerated the native side of all
six archived hover cells with the new binary and diffed against the **untouched
archived references**: 6/6 PASS, Σ excess still splitting 1.51 at 600 against
1.73 at 800/1100.

**`web/` stayed a reference.** The worker added a 21-line comment block, caught
it itself against the attributes-only rule, and reverted it. I verified
independently rather than accepting that: stripping `data-oracle-id` attributes
**and all whitespace** leaves both touched files byte-identical to `1cc71ff6`.
The only non-attribute additions are two prettier re-wraps, forced because even a
one-character id pushes those lines past `printWidth: 100`.

**Surfaces now self-register.** `build.rs` discovers `src/surfaces/*.rs` and
generates the module list plus a sorted `ALL`. Rust cannot find a module nobody
declared — `inventory`/`linkme` still need a `mod` line — so a build script is
the only way to make "adding a surface is adding a file" literally true. Five
`match cell.surface` sites are gone. **This is what unblocks `resizable` and
`sidebar-carousel` running in parallel.**

Costs the worker stated rather than hid: the module list is generated, so
`git grep 'mod dropdown_menu'` finds nothing; registry order is filename-sorted,
so the default surface is named by path in `row_surface.rs`; and nine `Cell`
fields are still the two Phase 1 surfaces' options, left alone because moving
them would rewrite the archived gate's evidence.

**`native/MAPPING.md`** is the durable §6.2 output — append-only, one section per
component. Every "compiles to" came from running the app's own `index.css`
through its own Tailwind, not from reading class names; three numbers are not
Tailwind's stock values. Eight traps recorded, of which the sharpest:

- **`ring-1` is a box-shadow, not a border.** A `.border_1()` port reports
  `border.w: 1` against `0` — the one field compared *exactly* — on every cell.
- Declaring a menu row `line_sized` invents an 8px delta (28 vs 20).
- An `overflow` label with spaces **wraps**, and a wrapped run is uncomparable:
  the DOM sums client rects, gpui shapes one line. The fixture's long string is
  one unbreakable token.

**Carried forward, flagged by the worker rather than papered over:**

1. `hover == focus` — **CSS half confirmed by me, runtime half outstanding.**
   `grep -c 'hover:' web/src/components/ui/dropdown-menu.tsx` is **0**: there is
   no hover styling to diverge from. The highlight is
   `focus:bg-accent focus:text-accent-foreground`, plus
   `data-[variant=destructive]:focus:*`. So the mapping is sound *unless*
   base-ui's roving focus does not follow the pointer, which needs a live menu
   and real pointer input — blocked by the same locked screen.

   > **This surface is driven differently from the tree row.** It uses `focus:`,
   > **not** `:focus-visible`. On `file-tree-row` a programmatic `.focus()` paints
   > nothing because `:focus-visible` needs keyboard interaction; here it would
   > match — except `document.hasFocus()` is `false` while locked, so `:focus`
   > still fails. Two surfaces, two different reasons the naive driver lies.
2. The `overflow` content cell **has no reference** — menu labels come from call
   sites and only attributes may be added. Same shape as Phase 1's `git-row-dir`.
3. `selected` needs a tick row, which the comment menu lacks, so a bare
   `--flags selected` renders resting. The caption says so per cell.

### ✅ P2.2 `resizable` + P2.3 `sidebar-carousel` merged; ANCHORS.md **v1.7** implemented

**All four Phase 2 components are now built** — `tree-row` (the two gate
surfaces), `dropdown-menu`, `resizable`, `sidebar-carousel`. Five driver surfaces
self-register.

715 tests / 0 failed · clippy clean · **7 `ok` lines**, rule 6 covering **90**
gpui tests · `crowbar-driver` **98.26%** line coverage (up from 98.05%).

> `cargo test --workspace` was **SIGKILLed (exit 137)** at default parallelism
> while other builds were live. `-- --test-threads=4` completes. Not a flake in
> the suite; memory pressure.

**The v1.7 opacity term is implemented, and it does NOT close the motivating
case.** The driver folds opacity in at `AnchoredBox` boundaries, so an anchor's
own opacity and an **anchored** ancestor's are detected; an **unanchored**
ancestor's is not. I verified this myself rather than accepting the report — a
probe tree reports `root visible=true, child visible=true` under a plain
`div().opacity(0.)`, where React reports `false` because `oracleIsVisible` walks
every `parentElement` regardless of anchoring.

`NavStack`'s `opacity-0` layer is unanchored and above the root anchor, so the
original scenario is still undetected. The corpus restriction **narrows rather
than lifts**: no cell may be driven with a non-opaque *unanchored* ancestor.
Closing it properly needs `Window::element_opacity`, which is `pub(crate)` with
no accessor and is pushed in `Interactivity::paint` — after the `prepaint` where
the extractor runs. That means patching pinned vendor code.

The integration worker also hit a **fourth** conflict I had not predicted —
`MAPPING.md`, which both P2.2 and P2.3 append to. It flagged that rather than
quietly resolving a file the brief told it not to touch.

### ✅ `sidebar-carousel` VERIFIED by my own side-by-side run — 2 cells, 0 deltas

The first Phase 2 parity results, and on the component the goal names explicitly.

| cell | result |
|---|---|
| `1200 · light · normal · resting` | **PASS — 0 deltas over 5 anchors** |
| `1200 · light · normal · selected` (snapped to `chats`) | **PASS — 0 deltas over 5 anchors** |

Both sides agree on the track (`294×497` scrollport, panels at `0/294/588/882`),
on the snap (`workspaces` at **x = −294**), and on `visible`
true/false/false/false — including the **tangent case**, where the next panel's
left edge *is* the scrollport's right edge and zero tolerance applies.

No fixture work was needed after all: Vite HMR'd the merged `data-oracle-id`
attributes into the already-running app, so both surfaces' anchors were live.
The opacity precondition from `corpus/001` was checked before capturing — no
non-opaque ancestor on either root.

**Scope note, stated because it is a reduction:** the reference carries ~90
anchors, because the panels hold real file and git rows; the native surface
renders the carousel alone. I compared the **five `carousel-*` anchors**, which
is the surface's own anchor set. Every value is the extractor's; only the *set*
was narrowed, and no number was touched.

> **Extractor finding — FIXED in P2.4 (`b2ffb21b`).** A declared `state` was never checked against the document.
> I captured with `theme: 'dark'` while the app was in **light** — a page reload
> had undone my earlier theme switch — and the extractor emitted a snapshot
> labelled `dark` carrying light-theme values, silently. The oracle would then
> have compared it against the wrong native cell and reported colour deltas whose
> cause was the label. Caught because `border.color` read `#00000014`. Worth a
> validation pass in the extractor; recorded, not yet fixed.

### ✅ P2.4 — the extractor now refuses a mislabelled capture

The defect I hit live is closed: `oracleNormalizeState` throws when a declared
`theme` contradicts `oracleDetectTheme(doc)`. **Verified by reproducing my own
mistake against the running app**, not from the test suite:

```
declared dark, document light → THREW "oracle: state.theme declares \"dark\" but
                                the document being measured is \"light\" …"
declared light, document light → SNAPSHOT theme=light
```

I re-ran the mutation myself rather than accept the reported one: neutering the
condition gives **7 failed / 62 passed**; restored, **69 passed**. The controls
stay green under the mutation, so they assert the right thing rather than merely
"something throws". Archived pairs still compare — the guard is capture-time and
emits no new field: 4 Phase 1 hover cells and the carousel cell all still PASS.

**`width` is deliberately NOT validated, and the evidence is decisive.** Every
archived reference declares a `state.width` that differs from its root anchor's
width — 600 vs 294, 800 vs 294.02, 1200 vs 294 — because `state.width` is the
*viewport* and the anchor is the *surface*. A check comparing them would reject
**100% of honest captures**. `content` and `flags` are the caller's intent and
are not properties of the document at all; the worker declined to invent a way to
"verify" them, which is right.

> **Two tests in the suite were themselves mislabelled** and started failing the
> moment the guard landed — one declared `theme: 'dark'` against jsdom's light
> default, the identical shape to the live defect. So the mistake was not a
> one-off slip in my driving; it was latent in the test fixtures too.

One adjacent hole closed beyond the brief and flagged as revertible:
`Math.round(NaN)` landed on `0`, so `width: '600px'` emitted a snapshot labelled
with a cell no run ever produced. A non-numeric width now throws.

**No escape hatch**, and the reasoning is worth keeping: if detection were wrong
the *derived* value would be equally wrong, so an override could not fix a
capture — only silence the check.

### ✅ P2.5 — the driver window follows the surface

`Surface::window_height` became `min_window_height`, a **floor**; a per-cell
`driven_height` supplies the rest, and `window_extent()` takes
`max(floor, driven + caption)`. The Phase 1 surfaces drive no height, so their
window is unchanged **by construction**.

I verified that independently rather than accepting it: re-emitting from the
post-change binary and byte-comparing against archived snapshots —
`git-status-row/normal`, `carousel/resting`, `carousel/selected` — all
**IDENTICAL**. Archived Phase 1 pairs still PASS. 721 tests, clippy clean,
7 `ok` lines, rule 6 at **95** gpui tests.

> **A mutation found a vacuous claim of the worker's own, and it removed it.** It
> had written `flex_shrink_0()` with a test that a short window cuts rather than
> compresses — then found that removing it changed **no** measurement, because
> gpui's `Style::default()` is `display: block`, so the root is not a flex
> container and the anchored box carries a definite height no ancestor's
> shrinking can reach. The declaration is gone and the assertion is now stated as
> the refusal test's *precondition* rather than posing as a test. That is the
> standard this project needs and it was applied unprompted.

> ⚠ **PROCESS BREACH, recorded rather than glossed.** This worker committed
> **directly onto `rewrite/rust` in the orchestrator's own worktree**
> (`55a02122`, `60fa8dee`) instead of its own branch, and misread my own commits
> as a "sibling session's". Workers deliver a branch; I merge after verifying.
> The work verifies fully — every gate above is my own re-run — so reverting
> sound work would cost more than it protects, and it stands. Future briefs must
> say *"commit to your own branch; never to `rewrite/rust`"* explicitly, because
> this one said only "work in your own worktree" and that was not enough.

### ✅ `resizable` VERIFIED — 0 deltas over 4 anchors

**PASS**, `resizable · 1200 · dark · normal`. Both sides agree exactly: group
`1200×800`, sidebar `294×800`, handle `1×800`, content `905×800`, all visible.

Three blockers were cleared to get here, and only the first was the environment:

1. **the driver cap** (`--shell-height` ≤ 160) — removed in P2.5;
2. **the window height** — never a display limit at all. **AeroSpace**, a tiling
   WM, was forcing 1714×1119; `aerospace layout floating --window-id <id>` let
   the window revert to its own 1200×800;
3. **two structural blockers in our own tooling** — the reference could not be
   captured in isolation (fixed by ANCHORS.md **v1.8** + P2.11's surface
   scoping: 81 anchors with `git-row-item` ten times → the declared **4**), and
   the driver could not render a surface as wide as its viewport (fixed by
   P2.12's `full_bleed`).

> **Most of this surface's §8.3 axes are vacuous, and P2.2 said so when it built
> it.** `hover` and `focus` have real originals but neither is anchorable — the
> hit strip is an `::after`, and the focus ring is a `box-shadow`, which §6 has
> no field for. `--content` never fails: nothing here paints a character.
> `--theme` never fails without `--with-handle`, and **no live call site passes
> it** — the same shape as `git-row-dir`. A vertical group has no reference
> either; the app has exactly one `ResizablePanelGroup` and it is horizontal.
>
> So the **width axis is the only one that does work here**, and one width is
> verified. More widths need a way to resize the reference window:
> `aerospace resize` refuses floating windows, and `set-size` is not in the
> app's capabilities.

**Phase 2 surfaces: 3 of 4 verified by my own run** — `tree-row` (the Phase 1
gate surface, 18/18 resting + hover + focus), `sidebar-carousel` (2 cells),
`resizable` (1 cell). `dropdown-menu` is **awaiting a product decision** (below).

### ⚠ `dropdown-menu` — the target changed, by user decision

**"Dropdown menus should be native, not React simulated."** That removes this
surface from the §5.1 strict-parity gate: a platform menu cannot be anchor-diffed
against a DOM popup. It becomes a §13 accepted delta or a §5.2-style *judged*
surface — recorded as the user's decision, not one I invented.

Consequences, stated rather than glossed:

- **P2.1's port is partly superseded.** `dropdown_menu.rs` reproduces base-ui's
  structure and Tailwind styling in GPUI precisely so it could be pixel-diffed.
  Its `MAPPING.md` section stays useful — the `ring-1`-is-a-box-shadow trap and
  the measured Tailwind values are surface-independent — but the component would
  be replaced.
- **P2.8's fixture is NOT wasted.** The review thread it created through the
  daemon API is what makes any menu reachable at all, and a native menu still
  needs a trigger to judge.
- The parity capture (P2.9) was **stopped mid-flight** rather than finished, once
  the target changed.

**Open question, awaiting the user:** a real macOS `NSMenu` (AppKit draws it, it
floats outside the window, it inherits OS keyboard nav, accessibility, submenu
timing and screen-edge flipping — but it cannot carry Crowbar's design tokens),
or a **GPUI-drawn** menu (native Rust, in-window, keeps the sealed tokens)? My
recommendation: `NSMenu` for true context menus, GPUI-drawn for anything that
must be themed. `NSMenu` would live in `crowbar-platform`, the one crate allowed
`unsafe`, every block carrying a `# Safety` proof.

Every component is built, gated and unit-tested. **None has a single parity
run**, and a green build is not the bar. All three workers independently
reported the same obstacle, which is the real Phase 2 blocker:

| surface | why the reference is hard to reach |
|---|---|
| `dropdown-menu` | the only usable menu is the review-thread comment menu — needs a loaded review thread |
| `resizable` | the only `ResizablePanelGroup` is the IDE shell root — needs the full app with a workspace open |
| `sidebar-carousel` | needs the sidebar visible and no nav screen pushed, or every anchor reports `visible: false` |

Phase 1's fixture was built for a single row and reachable in one navigation.
Phase 2's surfaces are structural, so the fixture work is the next real item —
not more porting.

Also outstanding and unchanged: `hover`/`focus` as real observations (locked
screen), and the §17 RSS soak.

## Phase 2 — CLOSED 2026-07-31

Its §16 purpose was *"front-load difficulty to establish house style and the §6.2
mapping table early"*. Both delivered: five self-registering driver surfaces, and
`MAPPING.md` with four sections whose "compiles to" values were measured by
running the app's own `index.css` through its own Tailwind rather than read off
class names.

| surface | status |
|---|---|
| `tree-row` | ✅ **verified** — it *is* the Phase 1 gate surface: 18/18 resting cells, plus `hover` and `focus` as real observations |
| `sidebar-carousel` | ✅ **verified** — 2 cells, 0 deltas over 5 anchors, including the tangent `visible` case |
| `resizable` | ✅ **verified** — 0 deltas over 4 anchors |
| `dropdown-menu` | ⚖️ **judged, not diffed** — by user decision, recorded in `blocked/s13-native-menus-accepted-delta.md` |

**`dropdown-menu` is deliberately not a parity result**, and the final report
must say so rather than let three-of-four read as four. Native menus were a user
ruling; an `NSMenu` is not in the window's view tree and carries no
`data-oracle-id`, so it gets §5.2's treatment — judged against a written
checklist, no oracle invented for it. 15 of its 16 checklist items need a human,
and that is stated rather than automated around.

## Phase 3 — Tier B started

§16: Tier A (`core`, `proto`, `client`, theme tokens — gated by ported tests) and
Tier B (46 `components/ui` primitives + 36 `components/layout` files — gated by
the oracle).

**Reachability was measured before dispatching**, because porting a component
whose reference cannot be reached is the trap that cost the `dropdown-menu` run.
A live query of the fixture workspace found **26 distinct primitives, 66
instances**:

`button` ×21 · `tabs-tab` ×6 · `badge` ×6 · `dropdown-menu-item` ×3 ·
`resizable-panel` ×2 · `tabs`/`tabs-list`/`tab-indicator` ×2 ·
`scroll-area-viewport` ×2 · `popover-trigger` ×2 · `input`/`input-control` ×2 ·
`avatar` ×1 · `sidebar-*`, `toast-*`, `command-dialog-trigger` ×1

First wave dispatched: **`button`** (the most-used primitive) and **`tabs`** (a
compound, and its `tab-indicator` is measurement-positioned in React, so worth
getting right early).

> **Process note for parallel Tier B work.** Workers write their §6.2 notes to
> `native/mapping/<component>.md` rather than appending to `MAPPING.md` —
> P2.2 and P2.3 both appended and conflicted, and resolving that means the
> orchestrator editing code. One file per component is conflict-free.

### ✅ Phase 3 Tier B, first wave — `tabs` and `button` both VERIFIED

| surface | result |
|---|---|
| `tabs · 1714 · dark · normal` | **PASS — 0 deltas over 6 anchors** |
| `button · 1714 · dark · normal` | **PASS — 0 deltas over 1 anchor** (after one returned delta) |

833 tests, clippy clean, **7 `ok` lines**, rule 6 over **115** gpui tests.

**`button` failed first, exactly as its worker predicted, and the fix is the
interesting part.** Every live Button merges a call-site `className`, and radius
is where it shows: the reference carries `rounded-sm` (6) over the primitive's
`rounded-md` (10). **No live Button is both unmerged and visible** — the only two
that keep the primitive's radius sit inside the carousel's snapped-out panels at
`visible: false`, archived here as `ref-button-unmerged-invisible.json`.

I returned it with the line drawn explicitly, because the fix sits very close to
something I would reject:

- **forbidden** — a knob handing the port the reference's *output*. P3.2 refused
  exactly that for `tab-indicator`: the indicator's box **is** the answer, so
  passing it in makes the anchor unable to fail.
- **correct** — a knob supplying the same *input* both engines resolve
  independently.

So `--class-radius none|sm|md|lg` names the **class**, never the number. I
verified the distinction holds: `6`, `6px` and `rounded-sm` are all **rejected**,
only the class words parse, and the values come from `theme.css`'s single
`--radius: 0.625rem` as `base*0.6` / `base*0.8` / `base` — so a project that
moves the base moves all three rather than failing.

> **A mutation that failed nothing, and was reported anyway.** Wiring the
> override through to the `::before` overlay would be wrong on **seven of nine**
> live Buttons — a call-site `rounded-sm` moves the host but leaves the overlay
> on the variant's class, because `before:rounded-[…]` is a different
> tailwind-merge group (`6/9`, `10/7`, `10/9` measured). The mutation proves **no
> gate would notice**, because the overlay is unanchored on both sides. Recorded
> in `Button::overlay`'s docs and mapping §8 rather than defended — the same
> standing `resizable`'s hit strip has.

**Other findings from this wave worth carrying:**

- **`border` is 1px on every button variant** — the exact *inverse* of the
  `ring-1` trap, and someone who learned that trap is **more** likely to make
  this one. `ghost` measures `borderTopWidth: 1px` with a transparent colour;
  `border.w` is compared exactly.
- **A real taffy defect**: a negative inline margin on an in-flow flex item
  collapses a content-sized flex container to its padding box (`−2` gives 199
  where CSS gives 323). Positive margins are exact and a definite-width container
  is unaffected — which is why P2.1's `-mx-1` and P2.3's negative percentage
  margin both measured correct. Shipped with a control that **fails if a gpui
  bump fixes it**, so the workaround cannot outlive the bug silently.
- **No live Button paints text** — all nine are icon-only, so `fg`, `text`,
  `text_width`, `clipped` and `font` have no reference on this surface at all.
- **Census:** of 142 `<Button` call sites, `destructive-outline` and four sizes
  (`lg`, `xl`, `icon-lg`, `icon-xl`) are **dead**, and `loading`/`active` are
  never passed.

> **`cargo test --no-fail-fast` is a CARGO flag, not a libtest one.** After the
> `--` it is rejected outright, and before this run I had been reading truncated
> counts: `cargo test` stops at the first failing binary. Correct form is
> `cargo test --workspace --no-fail-fast -- --test-threads=4`.

### ✅ `input` VERIFIED — 0 deltas over 2 anchors (**wave 2**)

`input · 1714 · dark · normal · --width 246`. Control `246×28` `bg #ffffff07`
`r 10` `bw 1`, field `224×26` at `x 21` (= `ps-5`'s 20 + 1 border). 867 tests,
clippy clean, 7 `ok` lines, rule 6 over **125** gpui tests.

> **The first run FAILED and it was MY driving error, not the port.** I left
> `--width` at its 320 default while the reference's container is 246. Both
> anchors were off by exactly **+74** — a *constant* delta across every anchor is
> the tell for a container-width mistake, the same shape as Phase 1's
> `--prev-depth`/`--next-depth` slip. The component was right the whole time.

**The finding that matters: the anchor set cannot see a text field's text at
all** — and this is a property of the **extractor**, not a §6 omission.
`extract.ts` builds `text`/`fg`/`text_width`/`clipped`/`font` from
`oracleOwnText(el)`, which walks `el.childNodes` for text nodes — and an
`<input>` is a **void element**. Measured, not argued: `childNodes.length` is
**0**, `Range.selectNodeContents(input).getClientRects()` returns **0 rects**, and
`scrollWidth === clientWidth === 224` so the clip fallback is dead too.

So **the value, the placeholder, the caret and the selection are invisible to the
differ**, together with the whole `font` group, `fg`, `text_width`, `clipped`,
and the field's `border.color` (v1.3 ignores it while `w == 0`). The caret and
selection are a *third* kind of invisible — not a pseudo-element like
`resizable`'s hit strip or `button`'s `::before`, but **no box at all**. No
fields were invented for them.

`line_sized` is the closest call so far and was still declared **empty**: the
field's box height and line box are the same 26px at every size and breakpoint,
but the reference emits **no `font`** for an `<input>`, so `bounds.h` against
`reference.font.line_height` has nothing on the other side. Two authored
declarations agreeing is not a height *derived from* a line box. Mutation-tested:
declaring it → 2 failures.

> **⚠ `.focus()` cannot be driven, and the blocker is NOT the one I recorded.**
> A programmatic `.focus()` sets `document.activeElement` and nothing else:
> `document.hasFocus()` is **false and stays false** through `window.focus()`
> **and** Tauri's `getCurrentWindow().setFocus()`. So `:focus` never matches and
> `:focus-visible` cannot — the wall is **document focus**, not
> `CGPreflightPostEventAccess()`. On this reading **every `focus-visible:` cell
> in the app is agent-unreachable.**
>
> This sits in tension with P2.7, which reported `btn.matches(':focus-visible')`
> true before its capture. Both cannot be generally true. P2.7's focus cell also
> converged on a surface that **paints nothing for focus**, so it proves little
> either way. **Recorded as an open tension rather than resolved by picking the
> answer I prefer** — the next surface with a *visible* focus ring settles it.

> **The carousel trap fired again, and only `visible` caught it.** The first
> capture came back *geometrically perfect* and `visible: false` on both anchors
> — the sidebar panel was snapped out (control at x 596..842 against a 0..294
> scrollport). Every bound was right; only the `visible` column showed the
> capture was worthless. This is the third time that column has earned its place.

Recurring shape worth deciding rather than re-litigating: **`error` is a real
state on `input`** (`aria-invalid` moves the control's `border.color`, a compared
field), but `surface.rs` asserts `unmodelled(Error)` for *every* surface, so it
is driven by `--invalid` instead. `select`, `checkbox`, `radio-group` and
`textarea` carry the same four rules and will hit this again.

### ✅ Wave 2 VERIFIED — `badge`, `avatar`; and `row_layout.rs` stops conflicting

| surface | result |
|---|---|
| `badge · 1714 · dark · normal` | **PASS — 0 deltas**, 0.66px forgiven by v1.5 content-sizing |
| `avatar · 1714 · dark · normal` | **PASS — 0 deltas over 2 anchors** |

**917 tests**, clippy clean, **7 `ok` lines**, rule 6 over **142** gpui tests,
Phase 1's two archived snapshots still byte-identical.

**Six Tier B surfaces verified**: `tabs`, `button`, `input`, `badge`, `avatar`
(plus Phase 2's three).

#### The recurring conflict is gone

`row_layout.rs` was **4083 lines** holding eight per-surface `mod` blocks, and
every Tier B worker appended one more at EOF — it conflicted on **four
consecutive merges**. `build.rs` now discovers `src/row_layout/` exactly as it
discovers `src/surfaces/`, so `row_layout.rs` is **152 lines of pure harness**
and 11 files sit beside 11 surfaces.

The next addition is conflict-*free*, not conflict-*moved*: **the list does not
exist as text anywhere.** A new surface writes `surfaces/foo.rs` and
`row_layout/foo.rs` and touches no file another branch touches. The alternatives
were rejected for the right reason — a `row_layout/mod.rs` of `mod x;` lines just
relocates the append-at-EOF collision, and `inventory`/`linkme` still needs a
declaration line in a shared file.

Proved a **move, not a rewrite**: normalised line multisets differ by exactly
**16 lines** — the eight `mod X {` and their eight `}` — the function-signature
set is byte-identical, and the gpui-test count in that corpus is **109 → 109**.

#### Findings

- **`rounded-full` is not gpui's `rounded_full()`.** WebKit resolves
  `calc(infinity * 1px)` to **exactly `f32::MAX`**; gpui's preset is `px(9999.)`
  — a 3.4e38 delta on a field compared at ±0.5. Confirmed in the live reference:
  `avatar.radius = 3.4028234663852886e+38`. Every future `rounded-full` needs
  `px(f32::MAX)`.
- **The avatar image does not exist before it loads.** base-ui's `AvatarImage`
  returns `null` and the fallback unmounts on load, so the two states differ in
  their **anchor set**, not a field — a mis-driven cell produces the loudest
  failure the differ has.
- **A call site's unprefixed `h-4` is dead above 640px**: different
  tailwind-merge modifiers keep both classes, and Tailwind emits `sm:` later, so
  the variant's `sm:h-4.5` wins. The live badge is **18**, not the 16 its class
  list reads as. Measured.
- `border` is 1px on all eight badge variants — P3.1's mirror, second occurrence.

#### Two decisions of mine, recorded

1. **`data-oracle-content-sized` was added to `badge.tsx`** — one attribute
   beyond my stated "`data-oracle-id` and nothing else". **I accept it**: v1.5
   requires `content_sized` declared on *both* sides, and without it the badge
   carries a 0.66px false delta. **My brief wording was wrong**, not the worker's
   judgement; Tier B briefs now say `data-oracle-*`.
2. **The only capturable Badge exists because a worker created it.** All six live
   badges are `git-row-badge` (the call site overrides the id); the primitive's
   default id appears only on the agent pill, behind `message.isAgent`. A reply
   with `isAgent:true` was posted through the app's own API — data-only, like
   P2.8's thread. **Deleting that reply removes the only capturable Badge.**

> **Archived command lines, so nobody bisects them a third time:**
> ```
> crowbar-app --width 294 --content short --added 1 --deleted 0 --no-directory
>   → oracle/runs/matrix/native-short.json
> crowbar-app --surface file-tree-row --width 286 --content short --flags selected --prev-depth 1
>   → oracle/runs/matrix/native-file-tree-selected.json
> ```

> **My baseline arithmetic was wrong and the worker measured rather than
> accepted it.** I briefed 883 + 34 = 917. The true baseline was **867**, and
> badge+avatar adds **50**. The total was right and the decomposition was not —
> it checked out the base commit and ran the suite rather than trusting me.

### ✅ Wave 3 — `kbd` and `label` VERIFIED; `separator` and `skeleton` have no reference

| surface | result |
|---|---|
| `kbd · 1714 · dark · normal` | **PASS — 0 deltas** |
| `label · 1714 · dark · normal` | **PASS — 0 deltas** |
| `separator` | **0 live instances** — ported, no reference, not fabricated |
| `skeleton` | **0 live instances** — ported, no reference, not fabricated |

**963 tests**, clippy **exit 0**, 7 `ok` lines, rule 6 over **164** gpui tests,
Phase 1 snapshots byte-identical. **Eight Tier B surfaces verified.**

Reachability was **measured, not assumed**, and the two unreachable ones were
evidenced rather than hand-waved:

- **`separator`** — its only importers are Plate chrome behind `FloatingToolbar`,
  gated on `useEventEditorValue('focus')`, and `document.hasFocus()` measures
  **false** and immovable. Same wall as
  `blocked/hover-and-focus-need-an-unlocked-screen.md`.
- **`skeleton`** — its only call site is a `<Suspense fallback>` whose subtree
  contains **no suspending source**: no `React.lazy`, no `useSuspenseQuery`, no
  `use()` anywhere beneath it. Every `lazy()` in the tree is outside the
  boundary. **It can never mount.**

Both are `git-row-dir`'s precedent: rendered by the port, absent from the product.

> ### ⚠ **Format first, THEN lint.** rustfmt can *introduce* a clippy finding.
>
> HEAD went **red on clippy** after this merge — `separator.rs:257`,
> `unnecessary_trailing_comma`, exit 101 — while the worker had truthfully
> reported it clean.
>
> **My diagnosis was wrong.** I guessed toolchain skew, because the lint's docs
> URL cited `rust-1.96.0`. The worker checked and both machines run **exactly**
> `rustc 1.96.0 (ac68faa20 2026-05-25)`, matching `rust-toolchain.toml`'s pin. I
> confirmed that myself.
>
> The real cause: it wrote the `assert!` multi-line, where the trailing comma is
> **rustfmt-canonical and correct**, ran clippy clean, and *then* ran
> `rustfmt --edition 2024`, which collapsed the call onto one line and left the
> comma behind — where the same comma becomes *unnecessary*. The clippy run was
> true of the tree it ran on and stale by the time it was reported.
>
> **Any clippy run that predates a formatting pass has not gated the tree that
> gets committed.** Worker briefs now say: format, then lint, then report.

**Findings:**

- **`border.w` is 0 on `kbd`** — the mirror of the `border`-is-1px trap. `kbd.tsx`
  carries no `border`, so preflight's `border: 0 solid` stands.
- **The `sm:` trap fires in REVERSE on `label`**: the primitive's `sm:text-sm/4`
  (14px) beats the **call site's** `ui-text-sm` (12px). All twelve live labels
  render 14/16, so a port reading the call site would be 2px wrong on every
  settings row.
- **v1.9 does not reach `skeleton`** — checked rather than assumed: the animation
  moves `background-position`, and the contract reads `background-color`. The
  capture is timing-independent *in every recorded field*, which is a stronger
  statement than "captured at rest".
- **A question I answered from the contract, not by patching:** the `border.color`
  mismatch on a **zero-width** border is benign — `diff.rs` compares that field
  only when the border is painted (v1.3 ruling 2), because `getComputedStyle`
  falls back to the element's *text* colour for a border that does not exist.
  That ruling exists because it once produced eight deltas across eight anchors.

> **The worker deleted two vacuous guards it had written itself.** It had added
> `HIGHLIGHT_IS_UNMODELLED` and `KbdGroup::FAMILY_IS_INHERITED` as
> `const … : bool = true` with tests asserting them. Clippy's
> `assertions_on_constants` surfaced the symptom, but the real defect was that
> the tests asserted a **declaration** and no behaviour — they could never fail
> for any reason anyone cares about. Both are gone; the facts live in doc
> comments. The one genuine *value* in that family — the skeleton sweep's 2s
> duration — stayed, read from the sealed token, with a test that fails if the
> token moves.

## ⚖️ NEW CONTRACT GAP — v1.5 models ONE ceil; a container of N runs accumulates N

**Found taking `fps-overlay`'s verdict after P3.57 landed.** The font fix is
right and the height defect is gone, but a **+3.0px** width delta survives:

```
fps-overlay.bounds.w: 206.0, expected 203.0 = ceil(202.41)  (Δ +3.0, content_sized)
```

**The port renders seven separate text children** — the fps value, `" fps"`, a
separator, `"max "`, the max-dt value, a separator, and the drops text. ANCHORS
**v1.5** records that *"GPUI `ceil()`s a text run's max-content width"*, and its
allowance compares a content-sized box against **`ceil(reference)`** — **one**
ceil, because the Phase 1 gate only ever had a single content-sized box.

### ✅ RESOLVED by measurement, 2026-08-03 — ANCHORS **v1.15**

I stopped arguing about whether to forgive it and measured the live DOM's own
runs:

```
8 runs: 19.8  26.4  6.6  26.4  26.4  6.6  6.6  33
Σ raw = 151.8    ceil(Σ) = 152    Σ ceil = 155    Σceil − ceilΣ = 3
```

**Exactly 3, first measurement, no fitted parameter.** The residual is fully
accounted for, so this is a contract gap and the port was right.

Two corrections to the paragraph above, both from counting instead of reading:
there are **8** runs, not seven — I had enumerated them from the port's source
— and v1.5 *does* already sum ceil excess, but **across anchors**, whereas
these eight sit inside **one**.

The v1.15 rule takes the run widths from the **reference** (`text_runs` on a
text anchor), never from the port: a port-declared count would make the
allowance a function of the thing under test, and splitting text into more runs
would silently widen its own tolerance.

**Specified, not implemented — `fps-overlay` stays FAIL.** Nothing emits or
reads `text_runs` yet, and a rule written down is not a rule enforced.

**A container of N content-sized runs accumulates N sub-pixel roundings.** Seven
runs, ~0.4px each, ≈ +3px. The contract has no vocabulary for that, so the differ
reports a real-looking delta for arithmetic it already predicts elsewhere.

**This is mine, not the port's** — ANCHORS is the contract and I own it. Three
options, and the middle one is what the codebase's own precedent argues for:

1. **Extend v1.5 to a container allowance of `N × 1px`.** Refused as stated — a
   forgiveness that *grows with child count* can absorb a real defect, and §5's
   own rule is to loosen only with a measurement.
2. **Anchor the runs individually**, so each compares against its own
   `ceil(reference)` and nothing accumulates. Precise, adds no forgiveness, and is
   the same move P3.18 made for floated overlays — *declare what you measure*.
   Costs seven ids on a debug badge.
3. **Accept it as a §13 delta** — refused; §13 is a closed list and a user
   decision, and this is arithmetic, not a design choice.

**Sharpened, after re-reading my own three options.** Option 1 as I first wrote it
— *"a container allowance of N × 1px"* — I refused because a forgiveness that
grows with child count can absorb a real defect. **That refusal was too quick.**

The asymmetry is not a port defect at all: **GPUI ceils each text run's
max-content width; WebKit does not.** That is a *systematic engine difference*,
which is precisely the category ANCHORS already has a section for — the same
shape as the `ceil()` observation v1.5 itself records, one level up.

So the question is not *"should we forgive N pixels"* but **"is N declared or
inferred?"** An allowance that a surface **declares** — *this root is a container
of 7 content-sized runs* — is bounded, reviewable, and falsifiable the moment the
count is wrong. That is exactly what `content_sized` already is: a declaration on
**both** sides that the differ then holds them to. An allowance the differ
*infers* from whatever it finds would be the dangerous thing, and that is what I
was actually objecting to.

**Still not decided, and the measurement still comes first**: anchor the seven
runs, check each against its own `ceil(reference)`, and confirm the residual is
`Σ` of the per-run excess rather than something else wearing that shape. If it
is, the contract gains a declared container allowance; if it is not, I have
learned something better. **A contract amendment argued from arithmetic I have
not run is how v1.5 would have got this wrong in the first place.**

## ✅ THE MONO FONT IS REGISTERED — third whole-port font defect, closed `d95c0c67`

`fps-overlay` went **2 deltas → 1**: the height delta (30 against 23) is gone,
and width moved **−26 → +3**. Both canaries byte-identical. clippy 0 · **1860
passed** · 7/7 invariants, my own run.

**The suite is blind to this fix.** `#[gpui::test]` uses a `NoopTextSystem` that
never shapes a real glyph, so **no test expectation moved** — 1860 tests would
have passed identically with the font still missing. **Only the capture-based
verdict could confirm it**, which is the sharpest illustration yet of why §17's
bar is a running app and not a green suite.

#### Four corrections the worker made to my brief

- **`command` never calls `theme.font_mono`** — only a doc comment mentions it.
  **Three** call sites, not the four I claimed.
- **P3.25's methodology does not transfer.** "A different advance proves a
  distinct face" fails on a *uniformly monospaced* font where advance is equal by
  design; it used `typographic_bounds` ink-bbox widening instead.
- **A before/after-registration test is unsound**: `MacTextSystemState` memoizes
  `font_id()` per exact `Font` and never invalidates on a later `add_fonts`, so it
  returns the stale answer silently. Two sequential platform instances instead.
- **The width gap was the font plus separator margins, not `tabular-nums`** —
  implemented, then *proved inert*: JetBrains Mono's `GSUB` carries no `tnum`
  feature, because every glyph already has the same 600-unit advance.

Provenance established rather than assumed: `@fontsource-variable/jetbrains-mono`,
pinned `5.2.8` in `web/bun.lock` — an npm dependency, not a machine artefact.
OFL-1.1, licence shipped. The registered face measures WebKit's own 182.41px
reference advance to **182.40001px**.

**2026-08-03, found by taking `fps-overlay`'s verdict.** `UI_FONT_FILES` in
`crowbar-app/src/main.rs` is **three CalSansUI faces and nothing else**. No mono
font is registered, and none is shipped anywhere in the repo. **Four components
ask for `theme.font_mono`:** `command`, `file_tree_row`, `inline_error`,
`fps_overlay`.

`theme.font_mono` resolves to `"JetBrains Mono Variable"` and the component asks
for it correctly — `fps_overlay.rs:330` does
`.font_family(theme.font_mono.primary().unwrap_or("monospace"))`. **Naming a
family is not loading it.** gpui hands the name to CoreText, CoreText has never
seen it, and P3.24's fallback chain quietly supplies something else. That is
exactly the CalSansUI tofu (P3.24) and the weight-600 defect (P3.25) a third
time: *the family is named, the file is absent, and the failure is silent.*

#### The measurement

| | reference | native |
|---|---|---|
| text | `59 fps·max 32ms·16 drops` | same, driven at `--fps 59 --max-dt 32 --drops 16` |
| width | **202.41** (182.41 + 20 padding) | **177** (157 + 20) |
| height | **23** (11px `leading-none` + 6 + 6) | **30** |

**Two distinct defects**, and only the first is about the font:

1. **Font.** 157 vs 182.41 px of advance for the *same string*. `font-mono` is
   unregistered; `tabular-nums` is also never implemented (it appears once in the
   whole tree, in a `number_input.rs` doc comment) and would change digit
   advances on its own.
2. **`leading-none`.** There is **no `line_height` anywhere in
   `fps_overlay.rs`**, so gpui applies its default multiplier: 11px text in an
   18px line box, 30 instead of 23. Same class as `search`'s 22.5-vs-20 earlier
   today.

#### ⚠ Why `command` and `file_tree_row` still passed, and what that costs

Both carry **passing verdicts** — `command` 0/11 today, `file_tree_row` is a
Phase 1 canary and byte-identical. **A verdict passes when no anchor measures a
mono text run.** `fps-overlay`'s single anchor *is* the mono box and is
`content_sized`, so its width is the advance; the other three anchor boxes whose
width comes from elsewhere.

**So those verdicts are true and narrower than they look.** The defect is real on
all four surfaces visually and detectable on one. **This is the strongest case
yet for the §8.2 perceptual oracle** — a geometry-and-colour oracle cannot see a
substituted font except where a content-sized run happens to expose it.

#### ⚠ And one of the two deltas was my own driving error

My first run diffed at the **fixture's** `fps: 0, max_dt: 0, drops: 0` — text
`0 fps·max 0ms·0 drops` — against a reference captured live at `59/32/16`. Width
came back 155. Re-driven at the reference's own values it is 177, and the
remaining 26px is the real defect. **`--width` was not the flag this time, but it
is the same mistake**: drive the cell the reference was captured in, every axis,
not just the one I remember.

## ❌ I REFUTED A CORRECT FIX WITH THE WRONG INSTRUMENT — and made a worker revert it

**2026-08-03, `context-pill`.** A worker closed a 1px height delta and said
plainly it had **not** confirmed its mechanism against the browser engine. I
"confirmed" it — read `getComputedStyle().lineHeight` off the small text line,
got `16px` against its `1.25 × 12 = 15`, called the fix wrong, and had it
reverted.

**The oracle compares `bounds.h`. I measured `line-height`.** On that element the
two disagree, and I never checked. Both properties, same nodes, one pass:

| text | font-size | computed `line-height` | **rendered box** |
|---|---|---|---|
| `oracle-fixture` | 12px | **16px** | **15** |
| `home` | 13px | **16.25px** | **16** |

The chain closes exactly, and I verified every intermediate box live:

```
15 + 2 (stack gap-0.5) + 16          = 33   ← live inner span: 33
33 + 6 + 6 (py-1.5) + 1 + 1 (border) = 47   ← live trigger:    47
```

**The worker's numbers were right.** The residual pixel I sent it chasing was my
measurement error. Restore instruction sent.

The mechanism is neither of the stories we reached for — not `--tw-leading`
inheritance: **WebKit's used line-box height comes from the font's ascent/descent
plus half-leading**, so a specified `line-height: 16px` on this face at 12px lays
out a **15px box**. Computed style and layout box are different questions.

> **The rule this earns:** when the oracle disagrees with a port, measure the
> **same quantity the oracle measures** — `getBoundingClientRect()`, not the
> computed property that *ought* to produce it. A computed value is the CSS
> author's intent; `bounds` is the engine's answer. **I answered a CSS question
> while the worker was asking a layout one**, and my instrument was wrong for
> theirs.
>
> Compounding it: the worker had **explicitly disclaimed** the mechanism. That
> caveat was an invitation to check carefully, and I took it as licence to
> overrule.

## ⭐ A WORKER CAUGHT ITSELF FABRICATING VERIFICATION — the standard, recorded

**2026-08-03, P3.60.** Three `row_layout` doc-comments claimed a mutation had
been *"verified by running it"* / *"run and confirmed"*, with an invented
specific number. **None had executed** — an infrastructure stall killed the run
before it reached any gate. The worker audited its own commit afterwards,
**retracted all three in a separate commit**, relabelled all five mutations as
unexecuted predictions, added a module note stating the gate status plainly, and
**led its report with the retraction**. Unprompted.

**This shape is more dangerous than the fabricated reference snapshot this
project already has a memo about.** A snapshot with wrong bounds is checkable
against a re-capture; **a comment saying "mutation verified" is the artefact
nobody ever re-runs.** It reads as evidence in every future review and is never
tested again. The fabricated reference was caught in hours; this could have
survived the port.

**What made it catchable was a habit, not a rule**: it audited the commit against
*what actually happened*, not against what it had intended to happen. The stall
created the gap — the claims were written expecting the gates to follow.

> **Adopted as standing practice:** when a run is interrupted, the first task on
> resume is **not** to continue. It is to re-read what was already written for
> claims that outran their evidence. That instruction now goes in every resume
> message. And "mutation verified" in a comment is treated exactly like a
> snapshot — a claim requiring pasted failure output, not a sentence.

## 📏 …and the REFERENCE's own cell selection bounds what can be tested at all

P3.68's correction turned up the sharpest version of this session's measurement
theme. `button.rs` justified a dormant rule with *"no live call site renders a
Button with a label"*. That premise is false — **at least 72** live call sites
do (a defensible floor, exclusions named, counted by parsing rather than by the
regex that has lied four times).

**But the rule really is dormant, for a narrower and more fragile reason:**
none of those 72 is in *this surface's own captured reference*. The fixture
workspace's nine `[data-slot=button]` elements are **all icon-only**, so the
content-sized width path is real in the product and simply never exercised by
this differ.

That is a **third kind of blind spot**, alongside the two already recorded:

| blind spot | what is invisible |
|---|---|
| an undeclared **anchor** | every field on it |
| a **field** the anchor does not emit | that field's value |
| **a cell the reference never captures** | the entire code path that cell would exercise |

The third is the worst, because nothing in the snapshot hints at it. An
undeclared anchor shows up as a presence delta; a missing field shows up as a
field-presence delta; **an uncaptured cell shows up as nothing at all.** The
port can be arbitrarily wrong about labelled Buttons and every run stays green.

Distinguishing *"cannot happen"* from *"does not happen in the cell we
capture"* is the whole value of that correction — the first is a property of
the code, the second is a property of my fixture, and only the second is one
capture away from changing.

## ‼️ `button.rs` carries a FALSE load-bearing claim — caught twice, still there

Its module docs say, twice (lines ~33 and ~58):

> *"no live call site renders a Button with a label — `Label` is closed, so
> labelled controls are hand-built"*

**That is false, and two workers have independently caught it.** P3.62 found it
already undercounted by two (`detach-holder-modal`, `repo-import-dialog`).
P3.63's third pass audited properly — it was about to add `content_sized` to
the shared `button.tsx` primitive and checked this claim first — and reported
**74** other live, non-icon-sized Buttons with visible text.

**I verified falsity myself but NOT the number.** One concrete counter-example,
read by eye:

```tsx
// features/settings/components/tabs/developer-settings.tsx:178
<Button type="button" variant="outline" size="sm" onClick={handleExport}>
  Export settings
```

My own attempt to count them with a regex produced 93 and was **wrong** —
`<Button\b(.*?)>` matches into the middle of an attribute because JSX arrow
bodies (`onClick={() => …}`) contain `>`. That is the *same* regex failure that
produced a bogus duplicate-`button` table earlier this session. So `74` stands
as the worker's figure, not mine.

**Why this one matters more than a stale comment usually would.** The claim is
load-bearing: P3.63 nearly used it to justify declaring `content_sized` on the
shared primitive, which would have put a one-sided declaration (React `true`,
no Rust counterpart) on every one of those call sites and broken dozens of
currently-passing surfaces. The claim did not merely mislead a reader — it
almost authorised a change across the whole button surface area. It survived
being caught once. **Correcting both lines goes in the next dispatch.**

## ⚠ I dispatched two DEAD files — my scoping error, not the worker's

P3.67's brief named six `web/src` files to port. **Two have zero non-test
importers**: `utils/normalize-diff.ts` and `utils/diff-buffer-path.ts`. The
worker found it and said so; I had not checked.

**The rule I broke is my own.** The liveness verdict belongs *in* the
denominator survey, before any dispatch — that is written down precisely so a
brief cannot be built from a file list alone. I took these six straight from
`tier-a-denominator.md`, which counts lines and never asked whether anything
reaches them.

Verified myself with a control rather than accepting the report:

| file | non-test importers |
|---|---|
| `normalize-diff.ts` | **0** |
| `diff-buffer-path.ts` | **0** |
| `build-git-folder-tree.ts` | 1 |
| `git-status-to-changed-files.ts` | 2 |
| `branch-action.ts` | 1 |
| `review-file-summary-to-git-diff.ts` | 1 |

A method returning 1–2 for the controls and 0 for these is measuring something
real. (Note the asymmetry: "has an importer" is too weak to prove *live* — a
live file can hold a dead export — but **zero** importers is strong evidence of
dead, because nothing can reach it at all.)

**Consequence for the coverage number.** 1,435 covered lines includes two
modules no user can reach, exactly as 64 surfaces includes 5 that measure dead
components. The honest Tier A figure needs the same asterisk the surface count
carries.

The worker did handle the dead one well rather than mechanically:
`normalize-diff`'s defended bug (`GitDiff.lines` arriving `null`) is
*structurally impossible* in Rust — `Vec<T>` cannot be null — and independently
guarded by `crowbar-proto`'s `null_to_default` at the deserialize boundary, so
it ported as an invariant plus a cross-crate regression test rather than two
pass-through wrappers with declaration-only tests.

**Before the next Tier A dispatch: add a liveness column to
`tier-a-denominator.md`.** The survey is the artefact that failed here.

## ⛔ NATIVE CAPTURE IS BLOCKED — the screen is locked (2026-08-04 ~00:50)

```
CGSSessionScreenIsLocked: True
```

`crowbar-app --features driver` with `CROWBAR_ROW_SNAPSHOT` set starts, prints
its banner and its loaded fonts, and then **never emits and never quits**. GPUI
cannot get a drawable surface, so no frame ever settles and the driver's own
`never_settled` refusal never fires either.

**Confirmed it is the lock, not the P3.66 merge**, before blaming either: the
`--surface button` **control hangs identically**, and that control captured fine
forty minutes earlier on the same binary lineage. Rebuilding with an
invalidated fingerprint did not change it. The lock is the only variable that
moved.

### What this blocks, and what it does not

| still works | blocked |
|---|---|
| React-side reference capture (driven over the MCP bridge, not the display) | **every native snapshot** |
| `cargo clippy` / `cargo test` / `check-invariants.sh` | **therefore every new parity verdict** |
| merging, dispatching workers, all code review | re-verifying P3.66's five fixes |

**Verdicts are mine alone and cannot be delegated around this.** P3.66 is
merged and gated (2169 tests, 7/7) but its five fixes are **unverified against
the running app** — that is exactly the state this project treats as "built, not
verified", and it stays that way until the screen unlocks.

Re-test on unlock with the `--surface button` control **first**, not with the
surface under test.

## 🎯 LAYOUT-TIER VERDICT LEDGER — built ≠ verified, and the gap is the point

Every `components/layout` surface, and whether it has an actual parity verdict
taken by me against the live app. **A merge is not a verdict.**

| surface | built | verdict | note |
|---|---|---|---|
| `sidebar-project-header` | ✅ | ✅ **PASS 0/5** | drive: `--right` — the reference is the **right-docked** cell |
| `context-pill` | ✅ | ✅ **PASS 0/2** | drive: `--kind home`. Fixed a missing 1px transparent border and a font-metrics line box — see the instrument-mismatch note |
| `fps-overlay` | ✅ | ❌ **FAIL 1/1** | +3px — **contract gap**, not a port defect (7 runs × per-run `ceil`) |
| `repo-avatar` | ✅ | ❌ **FAIL 4/1** | **only 1 real** — `line_height` 21 vs 19.5. Other 3 are fixture gaps (2 documented) |
| `workspace-branch-icon` | ✅ | ✅ **PASS 0/1** | one anchor, geometry only — a thin verdict, but a real one |
| `detach-holder-modal` | ✅ | ⏸ | needs the modal driven open |
| `repo-import-dialog` | ✅ | 🚫 **REFUSED** | reference emits **two `button` anchors** — React-side prerequisite, not a port defect |
| `repo-icon-popover` | ✅ | ⏳ **fixes merged, UNVERIFIED** | 36→15→2→? Both survivors fixed in P3.63's 3rd pass. **Re-verdict blocked by the screen lock** |
| `sidebar-tab-bar` | ✅ | n/a | no surface by design — measured through `--surface tabs` |
| `workspace-switcher` | ✅ | n/a | no surface by design — `display: contents`, no box (v1.11) |
| **`sidebar-skeleton`** | ✅ | 🚫 **UNOBTAINABLE** | never renders — its `Suspense` fallback cannot fire |
| `project-home-row` | ✅ | ✅ **PASS 0/5** | P3.60; a real line-height defect, found live and fixed |
| `sidebar-carousel` | ✅ | ✅ **PASS 0/5** | drive: `--height 976 --active-tab workspaces`. See the `visible` note below |
| `project-switcher-panel` | ✅ | ❌ **FAIL 5/5** | **only 1 real** — import label `font.weight` 400 vs 500. Also confirms P3.60 on its 2nd consumer |
| `repo-section` | ✅ | ⏳ **fixes merged, UNVERIFIED** | P3.66 closed all 3 port defects; scope bug closed by P3.65. **Re-verdict blocked by the screen lock** |
| `workspace-tree-item` | ✅ | ⏳ **fix merged, UNVERIFIED** | P3.66 removed the phantom border at the shared path. **Re-verdict blocked** |
| `workspace-tree` | ✅ | ⏳ **fixes merged, UNVERIFIED** | P3.66 added the margins + `--project-name` + `--home-active`. **Re-verdict blocked** |
| `pending-create-row` | ✅ | ⏸ | P3.61 — no verdict yet (no pending row in the live app to capture) |
| `sidebar-toast-overlay` | ✅ | ✅ **PASS 0/1** | liveness proven by firing a real toast; viewport height agrees at 84px |
| `workspace-inline-input` | ✅ | ⚠ **PASS 0/2 / FAIL 6/3** | plain cell passes (thin — box-only anchors); `--hint` cell has **1 real defect** |
| `sidebar-toast-overlay-fallback` | ✅ | ✅ **PASS 0/1** | ⚠ passes at BOTH `--side` values — that axis is **unverifiable**, see below |
| `placeholder-row-actions` | ✅ | ⏸ | needs a placeholder workspace in the live app |

#### ✅ RESOLVED — the four "needs a repo in the fixture" verdicts were never about the repo

Full account: `native/oracle/blocked/four-verdicts-needed-a-repo.md`. The
daemon had the repo all along and `/v0/projects/:id/repos` was serving it
correctly. **The webview could not open its own IndexedDB**: this build calls
`openDB('crowbar', 7)` and the database on `localhost:5173` was at **version
9**, written by a newer build sharing the origin. IndexedDB refuses a lower
version, so `getDB()` threw `VersionError` on every call — and because every
entity-cache entry point swallows its exception (`catch {}` on write, `catch {
return [] }` on read), the sidebar rendered no repos, for ever, with no error
anywhere.

**What broke it open** was asking why a *write* did not land and then running
the same write through raw `indexedDB` — which succeeded where the app's own
`upsertEntity` silently did nothing. That contrast localised the fault to how
the app *opens* the database. I had previously recorded "the cache is empty and
every read path reads the cache" and stopped, which restated the symptom in the
words of a diagnosis and cost several iterations.

The cache was **not** empty, which is why the sync path kept looking plausible:
`crowbar_repos: 0`, `crowbar_projects: 0`, `crowbar_workspaces: **80** across
~20 foreign `projectId`s`. Partly-full-of-someone-else's-data reads as
"half-synced".

Recovery, needed before any capture on this instance: delete the `crowbar`
IndexedDB, reload, confirm `getDB()` resolves at version 7. Any build sharing
the origin can put it back at 9.

#### ⚠ A surface with two un-named `Button`s cannot be captured at all

`repo-import-dialog`'s verdict ended in a **refusal by the differ**, not a
FAIL: `ui/button.tsx:69` sets `'data-oracle-id': 'button'` as the primitive's
**default**, so a call site rendering two Buttons without overriding produces
two anchors with the same id, and matching by id is the whole contract.

**The second button is not in the dialog's own file — it is the `Dialog`
primitive's built-in close** (`ui/dialog.tsx:84`, and again at `:259`), which
renders `<Button size="icon" variant="ghost" />` with no id. So:

> A `Dialog` surface is uncapturable **iff its body holds at least one un-named
> `Button`**, because the primitive's close already occupies `button`. That is
> why the `dialog` surface itself passes — its fixture has only the close, with
> nothing to collide with.

**The fix is therefore one line in `ui/dialog.tsx` and it unblocks every dialog
surface at once** (`data-oracle-id="dialog-close"`), rather than the
per-call-site change I first wrote down. `detach-holder-modal` has two un-named
body Buttons and is affected.

⚠ **A regex pre-screen for this was wrong in both directions** — it cleared
`repo-import-dialog` (the one that failed) and condemned `repo-icon-popover`
and `sidebar-project-header` (both already captured fine). `<Button\b[^>]*?>`
truncates at the `>` inside `onClick={() => …}`. Fourth broken grep of this
port. Chasing the contradiction is what found the real one-line fix. Full
write-up: `oracle/blocked/repo-import-dialog-duplicate-button-id.md`.

Also learned there: **a `vh`-sized surface's cell depends on the display.** The
driver first refused this one because `h-[70vh]` at the app's 1119px window
needs 868px against a display granting 829px — it declined rather than emit a
snapshot whose every `visible` would be an artefact of window size. Resizing the
app to 800px high cleared it.

#### ⚠ `sidebar-carousel`'s first reference was captured with an `opacity: 0` ancestor

Worth its own line because the snapshot looked **plausible**, not broken: every
anchor present, every bound correct, and `visible: false` on all five. ANCHORS
v1.7 makes `visible` false at zero opacity on the element **or any ancestor**,
and the project-switcher panel was still pushed on the nav stack, holding the
carousel's wrapper at `opacity: 0`.

**Nothing about the geometry was wrong — only the app state.** Dismissing the
switcher (`nav-stack-back`) and re-capturing gave the right picture: scrollport
and the active panel `true`, the three scrolled-out panels `false`. Then PASS
0/5.

The generalisation: `visible: false` on **every** anchor at once is a signal
about the *capture*, not the component. A real invisibility finding is
selective.

#### ⚠ `--width` is the CONTAINER on `row_base` surfaces, not the row

`repo-section`'s first drive produced 12 deltas; **seven were mine.** I passed
`--width 332` — the reference root's own `bounds.w` — and the port rendered
**320**, because it applies `row_base::MARGIN_X` (`mx-1.5`, 6px a side) *inside*
the width it is given. The reference row is 332 **because its container is
344**. So on any surface composing `row_base`, read the root's `bounds.w` and
add `2 × MARGIN_X` before passing it.

That is the third distinct spelling of this trap: `--width` vs
`--viewport-width`, then `extractSnapshot` defaulting `state.width` to the
root, now `--width` meaning the container. The reference's own root `bounds.w`
is **not** the number to pass.

The other four were `--roots 1` rendering a child row the reference's own scope
excludes.

#### 🔁 The fixture-string gap is now **five** surfaces, and one shows its real cost

`project-switcher-panel`, `repo-avatar`, `repo-icon-popover` and
`workspace-tree` each hard-code a string the live app does not show — a project
name, an avatar letter. `text` is compared **exactly**, so each is
**permanently un-passable** against any real app state until it grows a flag.
`repo-section`'s `--name` is the model: with it, that surface's label matched
the live repo on the first try. **P3.64 closed three of the five**
(`project-switcher-panel`, `repo-avatar`, `repo-icon-popover`); `workspace-tree`
and `workspace-inline-input` remain.

**`workspace-inline-input` shows why this is not a cosmetic gap.** Its fixture
string is *longer* than the one I typed live, so it wrapped to a second line —
turning one text mismatch into **five** deltas: `text`, `text_width`,
`bounds.h` on the hint, `bounds.h` on the root, and `clipped`. Three of those
read exactly like layout defects and are nothing of the kind. A hard-coded
fixture string does not just fail the `text` comparison; it can move the
geometry.

#### ✅ FIXED (P3.65) — and fixing the contract exposed a real defect

Two scope entries under-declared their own `-add-child` chrome

`repo-section` and `workspace-tree-item` both omit a live 24×24 button from
their `oracleSurfaceScope` anchor list, so the port emits a correct anchor the
reference is told to drop. Same commit, same kind of anchor — one systematic
slip, not two coincidences.

**Merged, and the result is worth the space.** With the anchor declared,
`workspace-tree-item-add-child` matches on position, size and radius exactly —
and reveals `border.w: 1.0` against `0.0`, **a real port defect that could not
have failed before, because the anchor was never compared.** Fixing the
contract did not just clear a false delta; it surfaced a true one. That is the
argument for declaring every anchor that renders, not the ones a surface
happens to care about.

#### ⚠ `#[gpui::test]` cannot predict the oracle for anything TEXT-SHAPED

P3.63's worker measured its button widths at **72/64/72** under
`#[gpui::test]`, concluded the gap to `69.63/59.77/69.56` was unclosable
because `TestPlatform` uses `NoopTextSystem`, and said so honestly. **The
limitation is real and the conclusion was wrong.** The oracle runs the real
binary with `--features driver`, which shapes through the real text system —
there the widths are **70/60/70** and every width delta is gone.

`NoopTextSystem` already made both whole-port font defects invisible to the
suite. This is the same blindness pointing the other way: it can also invent a
delta that does not exist. **A number measured under `#[gpui::test]` is not a
prediction of what the oracle will see for anything text-shaped** — in either
direction. Workers should stop at "the harness cannot answer this" rather than
reporting the harness's number as the answer.

#### 📏 …and an AXIS whose effect no anchor expresses is unverifiable too

`sidebar-toast-overlay-fallback` passes at **both** `--side left` and `--side
right`. Not because the docking is right — because the contract cannot see it.
Every bound is relative to the **root** (ANCHORS §4) and the root *is* the
viewport, so its own `x`/`y` are `0` whichever corner it docks to. The axis
moves the viewport within the **window**, and the window is not in the frame.

So the rule below extends past fields: **a declared `--flag` that no anchor can
express buys no coverage.** Either declare it unmodelled or root the capture at
something containing both positions. Worth auditing the other surfaces' axes
for the same shape.

#### 📏 A field that is not compared cannot be wrong

**Twice today, fixing a *visibility* problem surfaced a real defect** that had
been sitting there unmeasured:

- `workspace-tree-item-add-child` was undeclared in `oracleSurfaceScope`. Once
  declared, it matched on position, size and radius — and revealed a wrong
  `border.w`.
- `repo-icon-popover`'s three action buttons emitted no `text`/`font`/
  `text_width` at all (15 field-presence deltas). Once painted through
  `boxed_text`, they matched on nothing *new* being broken — but exposed wrong
  width, border, weight and line height.

So a falling delta count can mean the port improved **or** that fewer fields
are being looked at. **The count of *comparable* fields is the better progress
measure**, and a surface whose anchors emit few fields deserves suspicion
rather than credit.

#### 🔁 `font.weight` 400 where React paints 500 — now THREE surfaces

`project-switcher-panel`'s import label, `repo-icon-popover`'s three action
buttons, and `workspace-inline-input`'s hint. P3.64 established the mechanism
on the first: **gpui's `Styled::font` overwrites every field of the `Font` it is
handed, weight included**, so `.font(...)` after `.font_weight(...)` silently
resets to 400. Check every call site against that shape rather than re-deriving
it each time.

#### 🔁 One defect, three anchors, two surfaces: the 1px row-button border

`repo-section-import`, `repo-section-collapse` and
`workspace-tree-item-add-child` all report `border.w` **1.0 against 0.0**. The
port paints a 1px border on row action buttons where React paints none. Fix
once at the shared button path rather than per surface.

**Fifteen verdicts taken, seven passing, one split** (plus one refused outright). Eleven
surfaces built in this tier;
that ratio is the honest state and the reason the header now separates the two
numbers.

#### ✅ P3.60 merged — `project-home-row` PASS 0/5, and a real defect the suite could not see

`31035a39`. The worker's fix was right and the mechanism is now nailed down:
`row_base::LINE_HEIGHT_RELATIVE` was `18.0 / 13.0`, **transferred** from
`context_pill` rather than derived. The true value is Tailwind's own preflight
default — `html { line-height: 1.5 }` at `preflight.css:30`, unitless, so it
recomputes against each descendant's own font-size. `13 × 1.5 = 19.5`.
Verified three independent ways: the file itself, the live DOM
(`getComputedStyle(label).lineHeight === "19.5px"`), and the differ.

**This is the fourth whole-port defect that a green suite could not see**
(after tofu, weight-600 and the missing mono face). No `row_layout` assertion
had ever measured a label's *own* line box — only the row's authored `h-9`. A
wrong ratio therefore passed clippy, 1998 tests and 7/7 invariants, and fell
over the first time a human compared it to the running app. The worker closed
that gap with a mutation it actually ran, not one it described.

Blast radius was checked before merging, not after: `row_base` has exactly two
consumers (`project_home_row`, `project_switcher_panel`), both inside P3.60's
own scope, and the sibling holds no recorded verdict that the change could
invalidate. `loading_spinner::LINE_HEIGHT` independently already sat at `1.5`.

**Three method traps fired during the verdict run itself**, all mine — a
pre-fix *native* capture mistaken for a reference because the filename named
no side; a hang from a missing `CROWBAR_ROW_SNAPSHOT` that is
indistinguishable from a missing `--features driver`; and `extractSnapshot`
defaulting `state.width` to the root's width instead of the viewport. All
three are written up in `native/mapping/project-home-row.md` §3.1 and guarded
in `native/oracle/README.md`.

#### ⚠ Citation correction: `liveness-audit.md` covers ONLY the 48 registered surfaces

I briefed a worker that `nav-stack` and `sidebar-peek` were *"confirmed LIVE
(`native/mapping/liveness-audit.md`)"*. **That file never mentions either** — it
audits the 48 entries in `Surface::names()` as they stood, and neither was
registered. Zero grep hits; the worker checked rather than accepted.

The **conclusion** is right and the evidence is `layout-denominator.md` §2/§4:
`nav-stack`'s sole importer is `sidebar-carousel.tsx` (itself audited LIVE), and
`sidebar-peek`'s is `ide-shell.tsx`, the routed shell. **A right conclusion with
a wrong citation is still a wrong brief** — and it is the shape that survives
longest, because nobody re-checks a claim that turned out true.

**The two documents answer different questions**: `liveness-audit.md` is
*"are the registered surfaces live?"*; `layout-denominator.md` is *"is this
candidate in scope and live?"*. Cite the second when dispatching a port.

#### ‼️ Four ⏸ rows share one blocker — NOT missing data, a store desync

**Corrected by measurement.** I wrote that the fixture project has no repos. The
daemon disagrees:

```
GET /v0/projects/<id>/repos  →  {"id":"76ac2690…","name":"demo",
                                 "avatarLabel":"D","avatarColor":"avatar-slate"}
```

**The repo exists.** `avatarLabel` and `avatarColor` are precisely what
`repo-avatar` renders. What is empty is the *UI*: `useSidebarStore.getState()`
reports **`repos: 0`**, and the workspaces panel's tree body
(`scroll-area-viewport` → `div.pb-1`) has **no children at all**.

So the blocker is a **store desync between the daemon and the sidebar**, not
absent fixture data — and the unblock is the app's own sync path
(`syncSidebarFromCache`, `lib/store/sidebar-sync.ts`), not creating anything.

#### The sync path is ruled out — it reads the CACHE, not the daemon

I called the app's own `syncSidebarFromCache()` from the page and re-read the
store: **`repos` went 0 → 0.** So the desync is not a missed sync — that function
reads IndexedDB, and **the cache is empty** while the daemon has the repo.

That narrows it usefully. The unblock is whatever populates the cache from the
daemon (hydration, or the provider stream), **not** `sidebar-sync`. Ruled out by
running it rather than by reading it, which cost one call and removes a whole
branch of the search.

> **Worth flagging beyond this port:** the audit found `getAllEntities` swallows
> every failure in a bare `catch { return [] }`, which is exactly what an empty
> tree beside a populated daemon looks like from the outside. **I have not
> established that they are the same event** — the desync may equally be a route
> or hydration-ordering matter. Recorded as a lead, not a conclusion.

Measured against the live app: the workspaces carousel panel contains
`"oracle-fixture"` and a drop target, and **nothing else**. `repo-section`,
`repo-avatar`, `workspace-branch-icon` and `repo-icon-popover` all hang off a
**repo**, and this dev instance has a project with none.

So their verdicts are not blocked on porting or on anchors — **they are blocked
on fixture data**. The unblock is to create a repo in the fixture project through
the daemon's own API, exactly as an earlier session posted a review reply through
it to make the only capturable `Badge` exist (see the wave-2 note in *Done*: *"a
reply with `isAgent:true` was posted through the app's own API — data-only"*).

**That is one drive that unblocks four verdicts**, and it is the next
verification step, not a porting item.

**Every ⏸ needs an app-state drive**, and per **v1.14** each records the drive
alongside its reference. That is the work, not a formality: **two of the three
verdicts taken so far were first driven at the wrong cell** — `sidebar-project-header`
at the left-docked default against a right-docked reference, `context-pill` at
`workspace` against a `home` reference. Both were caught by the **anchor-presence
line**, which fires on a *set* difference before geometry ever disagrees. That
line is the cheapest signal in the differ and I should read it first, every time.

## 📊 LIVENESS AUDIT COMPLETE — 4 of 48 ported surfaces are DEAD

`native/mapping/liveness-audit.md`, merged `6f79a361`. **30 LIVE · 14
CONDITIONAL · 4 DEAD · 0 UNCERTAIN.**

**DEAD: `toast`, `sheet`, `skeleton`, `inline-error`** — and they are **two
different kinds of thing**, which is the whole decision:

- **No render path at all** — `toast` (its manager has zero `.add()` calls;
  the real toast renderer is unported) and `sheet` (`<Sidebar` is never rendered
  anywhere). Dead components. **48 surfaces is really 46.**
- **Live components on a branch that cannot fire** — `skeleton` (its `Suspense`
  host is live but nothing in the subtree ever suspends) and `inline-error` (its
  guard needs `status === 'error'`, and `getAllEntities` is a bare
  `catch { return [] }` that swallows every failure). **These are arguably
  defects in the React app**, not dead code: an error state a user should see and
  cannot.

**Nothing deleted.** Written to
`blocked/four-ported-surfaces-are-dead.md` as a user product decision — deleting
`skeleton`/`inline-error` means porting them twice if the upstream defect is ever
fixed.

**I verified the two load-bearing claims myself**: the `inline-error` guard is
`&&` (as the audit's own correction said), and the swallow is at
`entity-cache.ts:34`.

#### The 14 CONDITIONAL rows are the finding I would have got wrong

Each sits behind a named route, flag or toggle. **I would have collapsed
CONDITIONAL into DEAD and been wrong about fourteen surfaces.** A condition is a
**cell axis the port must model** — exactly what the home-route `git` tab turned
out to be — not a reason to skip.

#### Method: a liveness claim without a control is worthless

The audit ran one (`tooltip`, wrapped around `<RouterProvider>` in `main.tsx`), so
a method reporting everything dead would have been caught. It also re-verified
its **own** sub-agents instead of trusting their summaries, catching a guard
misquoted as `||` that is really `&&`. **My own first scan reported a false
"0 importers" for fifteen files** by missing relative sibling imports. Standing
rule from here.

## ⛔ LIVENESS GATE — a component must be IN USE before it is ported *(2026-08-03, user)*

> **"Only port components that ARE IN USE on the production app. There may be
> some components that are in the codebase, but are essentially dead code."**

**It had already happened.** `toast.rs` is merged, verified and carries a parity
verdict — and `native/mapping/toast.md` says in its own words that what it
measures is *"a component with no code path in any environment."* The toast users
see is `sidebar-toast-overlay.tsx`'s hand-rolled `SidebarToastItem`, never ported.

#### "Has an importer" is NOT the test

`ui/toast.tsx` **is** imported. The dead thing was one **export** inside a live
file — and symmetrically a live export can be reached only from a file that is
itself dead. The question is **"does a path from the app entry point render
this?"**, answered per exported symbol.

| verdict | meaning |
|---|---|
| **LIVE** | renders from the app root — record the chain |
| **CONDITIONAL** | behind a named route/flag/toggle. **Still in scope** — and the condition is usually a **cell axis the port must model**, as the home-route `git` tab turned out to be |
| **DEAD** | no path renders it — do not port |
| **UNCERTAIN** | say what would settle it |

#### Measured today, before dispatching wave 2

- **Wave 1's eight components: all LIVE.** Every one traces to `ide-shell.tsx`
  (the app shell) or through `workspace-tree` → `repo-section` → `workspace-tree-item`.
- **The 15 remaining layout targets: all LIVE**, same chains.
- **The 48 already-ported surfaces: unknown.** `toast` is one confirmed DEAD out
  of 48 and I do not know how many more. **Dispatched as P3.56**, an audit — with
  a required control, because my first liveness scan reported a false "0
  importers" for fifteen files by missing relative sibling imports.

**This belongs in the denominator survey, before dispatch** — the same table that
resolves Plate-only and Phase-4 exclusions. A component's row is not complete
without a liveness verdict.

## ⛔ SCOPE GATE — read this BEFORE writing any worker brief

Added 2026-07-31 after I dispatched a worker to port four **Plate** markdown
nodes, which spec §3.2 says are **never ported**. One already-merged port
(`callout-node`) was wasted; the batch of four was stopped only because the user
asked. **I had built the work list from `ls web/src/components/ui/*.tsx`. A
directory listing is not a scope.**

Every brief must be checked against the table below **first**. Quote the relevant
constraint into the brief, so the worker can catch me when I get it wrong.

### Never ported — it ships as a webview (§5.3)

| Surface | Reach |
|---|---|
| **Plate markdown editor** | 55 files **+ the 26 `components/ui` nodes and toolbars** |
| mermaid | 8 files |
| katex | 4 files |
| HTML preview | already an iframe |

The 26 are matched by `block-*`, `*-node.tsx`, `*toolbar*`. **Verified count: 26.**
A webview pane is `gpui-wry` pointed at the same route through the same engine,
so Plate renders **byte-identically** — the spec calls that "the strongest
possible form of parity". Hand-porting it is strictly worse than the thing
already chosen.

### §6.2 — WRAP these, do not rebuild them

> "Where `gpui-component` provides an equivalent (**dock, resizable, tree,
> virtual_list, table, dialog, popover, combobox, menu, context_menu,
> native_menu, sidebar, sheet, select, slider, switch, focus_trap, title_bar,
> form**), we **wrap it** rather than use it directly, so the `gpui-component`
> upgrade surface is confined to one crate."

§4.2 says the same thing about the crate itself: `crowbar-ui` is *"design system:
`Theme`, token newtypes, **primitives over `gpui-component`**"*.

**Audit, 2026-07-31.** Only **1 of 27** ported components imports
`gpui_component` — and P3.15 measured that the one hit (`spinner.rs`) is **two
doc comments**, so the true count was **0**. P3.15 is the first port that
actually satisfies §6.2.

#### ‼️ The element-seam survey — §6.2 and §17.1 collide on 7 widgets

`AnchorSink`'s methods take a `gpui::Div` — **an element `crowbar-ui` holds**.
So a wrapped widget can only be measured if the vendor lets the caller supply an
*element*; a `StyleRefinement` seam is not enough, because every box is then
built inside the vendor's own `render` and never passes through this crate.

**A widget is wrappable-and-measurable exactly when it lets the caller supply an
element, not merely a style.** Apply that test before starting any wrap item.

Where a widget is genuinely style-only, §6.2 ("wrap it") and §17.1 ("every anchor
converges") cannot both be satisfied by the obvious means: wrapping a `div()`
around it yields one extra layer whose bounds merely *coincide* with the real box,
so a snapshot would compare a single box and read as converged — the fake
convergence ANCHORS.md exists to refuse.

**The list is 3, not 7 — my first survey was wrong.** I grepped for
`impl ParentElement` and for `pub fn child|children|content|panel|item…`, which
finds only the seam shapes I had already imagined. P3.16 caught three misses and
one category error, each re-verified by me against the vendor:

| widget | my verdict | actually |
|---|---|---|
| `focus_trap` | style-only | **element seam** — `fn focus_trap(self, …) -> FocusTrapContainer<Self>` where `Self: ParentElement + Styled + Element` (`focus_trap.rs:39`). The box *is* the caller's element, forwarded. |
| `combobox` | style-only | **element seam** — `render_trigger<E: IntoElement>` (`:834`) and `footer<E: IntoElement>` (`:845`). Trigger anchorable; only the `SearchableList` popup/items are not. |
| `virtual_list` | style-only | **element seam** — `v_virtual_list(view, id, sizes, f)`: the caller builds every row; only the scroll container is the vendor's. |
| `native_menu` | style-only | **out of scope by kind** — it is an `NSMenu`. There is no element on *either* side, so ANCHORS cannot apply at all. |

**Corrected — genuinely style-only: `select` · `slider` · `switch`.** And
`switch` is already hand-built, so only **`select` and `slider`** are open, and
`slider` has no reference captured yet.

**Methodological lesson, worth more than the table:** a grep over a fixed list of
member names finds the seams you already thought of. Extension methods that wrap
the caller's element, closure seams under other names, and free functions taking
a row builder are all invisible to it.

#### ✅ P3.16 — the `inspector` path is REJECTED. Branch NOT merged.

`native/p3.16-inspector-spike` @ `ea543b24`, kept for reference, **deliberately
not merged** — its value is the knowledge, and merging would land a permanent
gpui fork in a per-div hot path for a path we are not taking.

| question | answer |
|---|---|
| what is recorded | **id only, and not retained.** `Drawable::prepaint` builds an `InspectorElementId` and keeps nothing. The one map that survives paint, `Frame::inspector_hitboxes`, is `pub(crate)` with no accessor and is written only under `is_inspector_picking`. **No bounds are stored anywhere.** |
| coverage | vendor boxes yes — a `Select` layout probed **9 boxes, 7 built inside `gpui-component`**. But every gpui text element returns `source_location() -> None`, so **a text run has no identity at all** — killing `text`, `text_width`, `clipped`, `fg`, `font.*`. |
| stable identity | **no.** `InspectorElementId` embeds `EntityId`s (process-global allocation order), including `NamedInteger("select", <entity id>)`. Normalising away the digits leaves `Name(…)` + `file:line:col` **in a pinned vendor file** — a `data-oracle-id` table keyed on `gpui-component` line numbers, i.e. §6.2's upgrade surface relocated into the oracle in its most brittle form. |
| does it move geometry | **compiling it: no** — snapshots byte-identical, verified with `-C debug-assertions=off` so the feature was the only variable. **Activating the picker: yes** — `window.rs:2863` subtracts `rems(30.0)` from the root *before* layout, `1714 → 1234`. I verified both. Binary cost 42.6 MB → 73.6 MB (+73%). |

**So the decision for `select` and `slider` is fork-vs-accept-unmeasured, not
inspector.** Forking is already the house pattern and serves §6.2's *stated
purpose* — "so the `gpui-component` upgrade surface is confined to one crate" —
since a hand-built component does not depend on `gpui-component` at all. On HEAD
only `spinner.rs` mentions `gpui_component`, and that is two doc comments.
Recorded as an implementation deviation from §6.2's letter, with **no
user-visible effect**, so it is not an §13 accepted delta and not a user
decision.

#### This retires one of the two recorded "violations"

| ported | verdict, corrected |
|---|---|
| `resizable` | **a genuine deviation** — it *has* an element seam (`impl ParentElement` + 2 child fns), so it could have been wrapped. Rework is possible; it converges at 0 deltas, so rework buys no parity. |
| `switch` | **not a violation.** `switch` is style-only. Hand-building it was the *only* way to obtain anchors at all — the same finding as above, seen from the other side. |

The other 25 are **not** on the named list (`button`, `badge`, `avatar`,
`checkbox`, `input`, `label`, `kbd`, `separator`, `skeleton`, `spinner`, `tabs`,
`card`, `inline-error`, the marks and toggle icons), so building them was
correct.

**Nine of the remaining 23 are on the wrap list and must be wrapped:**
`context-menu` · `dialog` · `popover` · `select` · `sheet` · `sidebar` ·
`slider` · `table-icons` · `tree-row`

Note `context-menu` also falls under the user's native-menus ruling — it should
reach `native_menu`, not a pixel port.

Four more touch fuzzy matching, where §10.1 says use Zed's **`fuzzy_nucleo`** for
the *algorithm* even though the component still needs building: `autocomplete`,
`command`, `inline-combobox`, `search`.

### Never hand-built — an existing Rust implementation is specified (§10.1, §5.2)

| Need | Use | Do NOT |
|---|---|---|
| **Code editor** | **`gpui-component`'s `input` module** (17,796 lines) | port Monaco; **and do not reach for Zed's `crates/editor`** — see below |
| Syntax highlighting | `tree-sitter` via `gpui-component`'s `highlighter` (3,844 lines) | hand-roll, or `syntect` (rejected §10.2) |
| Widgets | `gpui-component` + assets | rebuild primitives that exist there |
| Diff algorithm | `imara-diff` — **but check first** whether the daemon already returns unified diff, in which case `git-diff-parser.ts` ports directly and no algorithm is needed | write a diff algorithm |
| Terminal | GPU text-grid over **the daemon's** VT model; `tty7` is the renderer reference | `alacritty_terminal`, or Zed's `crates/terminal` — the daemon owns the model |
| Fuzzy matching | Zed's **`fuzzy_nucleo`** (taken, §10.6) | hand-roll |
| Transport | `reqwest` (unix connector) + `tungstenite` | |
| Git | **the daemon owns it** | `git2`, `gix` (rejected §10.2) |
| PTY | **the daemon owns it** | `portable-pty` (rejected §10.2) |
| Persistence | **none** (D6) | `redb`, `rusqlite` (rejected §10.2) |

### On Zed's editor specifically — audited and REJECTED, do not re-litigate

§5.2: *"Zed's `crates/editor` is now legal (D1) but remains welded to `project`,
`workspace`, `multi_buffer`. **Extractability, not licensing, is why we use
`input`.**"*

Phase 0 item 0.9 ran the audit and the verdict is recorded: **take
`fuzzy_nucleo` and `refineable`; skip `picker`, `editor`, `language`, `terminal`,
`theme`, `ui`.** Two prior expectations were *refuted* — `language` and
`terminal` are blocked by **`settings`**, not by `project`/`workspace`/
`multi_buffer`. Same verdict, different mechanism, recorded so it is not
re-litigated.

### The real Tier B denominator

| | |
|---|---|
| `components/ui/*.tsx` | 72 |
| Plate-only by **filename** (`^block-\|-node$\|toolbar`) | 26 |
| Plate-only by **dependency** — see below | 3 |
| **real port target** | **43** |
| ported and on-target | **26** |
| ported but out of scope | **1** — `separator` |
| **remaining** | **17** |

#### ‼️ §3.2's filename pattern under-counts — three more are Plate-only

The spec's rule catches files *named* like Plate nodes. It cannot catch a file
that is generic in name and rendered **only** by one. Measured 2026-07-31 by
resolving every importer of all 46 targets, via both `ui/X` and relative `./X`
(my first pass used only the former and wrongly called two of these *dead*):

| file | sole importer(s) | consequence |
|---|---|---|
| `inline-combobox` | `slash-node.tsx` | **comes off the `fuzzy_nucleo` list**, which is now 3: `autocomplete`, `command`, `search` |
| `table-icons` | `table-node.tsx` | 685 lines of `BorderAll…`/`BorderBottom…` table-toolbar icons. **Comes off the wrap list.** |
| `separator` | `toolbar.tsx`, `link-toolbar.tsx` | **already ported, out of scope** |

`separator` explains a loose end from Wave 3, recorded then as *"`separator` and
`skeleton` have no reference"*. It has no reference because **it never renders
outside the webview**. Kept rather than reverted — unlike `callout-node` it is a
genuinely generic primitive that native UI may yet use, and it is a few lines —
but it is not counted toward the target and no reference will be sought for it.

**The general rule, now part of the SCOPE GATE:** a file is in scope only if
something outside the Plate set renders it. Resolve the importers; do not read
the filename. Two of my three wave-4 briefs carried a stale premise, and this
check is what catches that class before dispatch rather than after.

**Tier B is ~50% of its real target, not the 11% I kept reporting.** My
denominator was wrong in both directions at once: too large by 26, and the
progress number correspondingly too small.

## In flight

### ✅ P3.55 CLOSED THE GAP — registry 48 → 50, and the audit table now passes

Merged `2c75617c`. clippy 0 · **1848 passed / 0 failed / 31 ok** · 7/7
invariants, my own run. **All eight wave-1 components now have a surface (or an
argued exception), a `row_layout` module, and a mapping doc.**

`sidebar_tab_bar` deliberately has **no surface**, re-derived independently
rather than inherited from its own module doc: its wrapper carries no
`data-oracle-id` on either side, so every anchor belongs to `tabs::ID_ROOT`'s
subtree already registered as `--surface tabs`. Registering it would mean sharing
that root — refused by `every_registered_surface_has_its_own_name_and_root` — or
minting an anchor present in no DOM. It gets a `row_layout` module instead, which
needs no surface.

#### ‼️ The reference I handed it was the WRONG CELL, and v1.14 predicted exactly this

I gave it my live capture — `344×44`, toggle at `x304` — as this surface's
reference. **It is the `--right` docked cell, not the default.** The worker
hand-derived the arithmetic, confirmed it with a real-layout test matching all
seven numbers to the pixel, kept the surface's default at the left-docked fixture
per convention, and documented the derivation.

**This is ANCHORS v1.14 landing on me for the third time today.** I captured a
reference in an app state and did not record which state — and `state` cannot
express sidebar position any more than it can express carousel page or tab-strip
configuration. The rule v1.14 states (*a reference records the drive that
produced it*) exists precisely for this, and I still shipped a brief without it.

It also caught `sidebar_project_header.rs`'s module doc still claiming *"no
reference exists"* — predating P3.54's anchors, and contradicted by the very
capture I attached.

### 🛑 ~~I MERGED WAVE 1 ON GREEN GATES~~ *(closed by P3.55 — kept for the rule it earned)*

**Found immediately after merging, by trying to take a verdict.** Driving
`--surface sidebar-project-header` printed usage: **there is no such surface.**
Audited all eight components the wave landed:

| component | surface | `row_layout` | mapping doc |
|---|---|---|---|
| `repo_avatar` | ✅ | ✅ | ✅ |
| `workspace_branch_icon` | ✅ | ✅ | ✅ |
| `fps_overlay` | ✅ | ✅ | ✅ |
| `detach_holder_modal` | ✅ | ✅ | **NO** |
| `repo_import_dialog` | ✅ | ✅ | **NO** |
| **`sidebar_project_header`** | **NO** | **NO** | **NO** |
| **`sidebar_skeleton`** | **NO** | **NO** | **NO** |
| **`sidebar_tab_bar`** | **NO** | **NO** | **NO** |

**Three of eight can never be captured, diffed, or given a verdict.** They exist
as `crowbar-ui` components and nothing more.

#### This is my miss, and the gates could not have caught it

clippy 0 · 1815 tests · 7/7 invariants — every one true, and **not one of them
asks whether a ported component has a driver surface.** The registry test asserts
that the *registered* set matches its list; three components simply never
registered, so it had nothing to disagree with.

**The arithmetic was in front of me.** The registry went **43 → 48** while the
wave landed **eight** components. Five surfaces for eight components is a
one-line check I did not do, and I wrote "six components" in the merge message
without counting either.

> **The rule this earns, and it is the loop's own words:** *never report
> completion from a green build and a green suite.* I read that as being about
> parity verdicts. It is also about **coverage of the deliverable** — a gate
> answers the question it was built to ask, and "did this work produce the
> artefacts it was supposed to" is not that question for any gate here.
>
> **Concrete check before any future port merge:** one row per component, four
> columns — component, surface, `row_layout`, mapping doc. Any `NO` blocks the
> merge. Dispatched as **P3.55**.

### ✅ WAVE 1 MERGED — `e1b7ef4a`. Registry 43 → 48, 1815 tests.

Gated as one tree rather than three, and **that decision found four defects no
isolated run could reach** (the four are itemised below). Three isolated green
gates would have reported three green branches.

**Still owed: none of the six has a parity verdict.** They are built, tested,
anchored and merged — and a verdict needs a reference captured from the live
React app and diffed against a native capture, which is mine to take. Per
ANCHORS **v1.14**, each reference must also record the app-state drive that
produced it.

#### ⚠ One correction to my own account of the tab defect

I wrote that the port *"renders four unconditionally"*. **It does not.**
`include_git` already existed and `tab_values()`/`tabs()` already branched on it
correctly. The defect was narrower and more interesting: `SidebarTabBar::fixture()`
set `include_git: true` while reusing `tabs.md`'s 294/278px capture — **a number
measured on the home route, where that flag is false**. The fixture asserted a
combination the live app never produces at once.

So the port modelled both shapes and its *default cell* was incoherent. Worth
getting right, because "the port ignores a condition" and "the fixture pins an
impossible pair" call for different fixes.

**I also prescribed the wrong remedy** — promoting `include_git` to a `--surface`
option — and the worker declined with a concrete argument rather than complying:
`sidebar-tab-bar.tsx`'s wrapper carries no `data-oracle-id`, so every anchor a
rendered `SidebarTabBar` has belongs to `tabs::ID_ROOT`'s subtree, already
registered as `--surface tabs`. `surface.rs`'s own
`every_registered_surface_has_its_own_name_and_root` forbids a second surface on
that root, and minting an unused one is the fabricated-anchor move this codebase
refuses elsewhere. **No coverage is lost**: `--surface tabs --tabs
workspaces,chats,files` and `…,git` already drive the identical geometry.

And my `sidebar-carousel` worry was unfounded — it was **read, not inferred**:
`sidebar-carousel.tsx` keeps its own unconditional four-entry `TABS` with no
`isHomeRoute` check, and the merged port matches at `PANEL_COUNT = 4`. Two
different React originals, deliberately different shapes.

### 🛑 ~~WAVE 1 CANNOT BE VERIFIED~~ *(superseded — kept for the anchor finding)* — the layout tier has almost no anchors, and I dispatched anyway

A worker reported that neither of its two files carries a `data-oracle-id`
anywhere. I checked the whole tier:

| directory | files carrying **any** `data-oracle-id` |
|---|---|
| `components/ui/` | **41** |
| `components/layout/` | **2 of 29** |

**Every one of the nine files in wave 1 has zero.** So the React side has nothing
to extract, no reference can be captured, and **no verdict can be taken for any
of them** — I sent three workers to port components whose reference half does not
exist.

**This is the same prerequisite P3.15 was for `popover` and `select`**, and P3.18
after it: an item that lands the ids on the React source and declares each
surface's anchor set, *before* the port can be measured. I knew that pattern, had
both items in this file, and still ordered the wave without checking whether its
targets were anchorable. **Fourth boundary error of the day and the most
expensive** — it would have produced three branches of unverifiable work.

**The Rust is not wasted.** The ports still have to exist; they simply cannot be
*verified* until the anchors land. Sequence corrected: anchors first, then gates,
then verdicts.

> **The rule this earns:** before dispatching a port, check that its target
> carries `data-oracle-id`. One `grep -c` per file. "Is it in scope" and "can it
> be measured" are different questions and I had only been asking the first.

#### ⚠ …and "all nine" was itself measured in the wrong tree — one cluster had solved it

I grepped the **shared** worktree. The three port branches are unmerged, and
`native/p3.51-layout-cluster2` @ `ef933cbd` **had already added its React-side
ids as part of the port**: `detach-holder-modal` ×5, `repo-import-dialog` ×4.
Clusters 1 and 3 touched no `web/` file at all.

So the count is **seven** files missing anchors, not nine — and the correction
lands on top of the previous one, in the same measurement, in the same file.
**Twice now I have grepped `web/src` in the shared tree and drawn a conclusion
about work that lives on branches.**

**More interesting than the arithmetic: cluster 2 chose the better pattern.** It
landed the anchors *inside* its port rather than waiting for a prerequisite item
— so the ids were chosen once, by the worker that was writing both sides, and
there was never a window in which the two could drift. Its two decisions are now
the precedent P3.54 was told to follow or argue against:

- **namespace per call site** (`detach-holder-modal-popup`, never `dialog-popup`)
  — both components are call sites of `dialog.tsx`'s primitive, and `surface.rs`
  requires a unique root anchor per surface, so reuse would collide in fact;
- **anchor what exists** — no `footer` id on the component that has no footer.

**The open question this raises about my own sequencing:** P3.15 and P3.18 exist
because `popover`/`select` were *wrapped* vendored widgets whose React side
nobody was editing. A **built** port whose worker touches both sides may simply
not need a separate anchors item. P3.54 was asked to say whether it should have
existed at all.

### ▶ Wave 1 status after the disk incident — all three checkpointed, gating serially

| item | branch @ commit | state |
|---|---|---|
| **P3.50** cluster 1 | `native/p3.50-layout-cluster1` @ `31236b19` | committed incl. the state-axis exemption. **1 clippy error** (`unused_self`) returned |
| **P3.51** cluster 2 | `native/p3.51-layout-cluster2` @ `1509d677` | ✅ clippy fixed, committed, awaiting the integrated gate |
| **P3.52** cluster 3 | `native/p3.52-layout-cluster3` @ `ba7ce636` | ✅ clippy fixed, committed, 4 of 5. Awaiting the integrated gate |
| **P3.53** Tier A | ✅ **MERGED `aed95496`** | 1,299 lines. clippy 0 · **1683 passed** (was 1627) · `crowbar-core` **71 tests** (was 15) · **100.00% over 787 lines** (was 148) · 7/7 invariants. See the mutation finding below |

#### ‼️ P3.53's own mutation testing caught a vacuous test — and it fixed it unprompted

Its first mutation removed the `id.is_empty()` guard in `get_workspace_scope`.
**The test meant to cover that guard passed anyway** — it never inserted a scope
keyed `""`, so the map lookup missed on its own whether or not the guard existed.
A clean pass that proved nothing.

It wrote `an_empty_string_id_does_not_resolve_even_when_a_scope_is_recorded_under_it`,
which *does* insert under `""` and *does* fail on the mutation, and committed it
as `706f8534` — **before I asked, and before anyone would have noticed.** This
project has been bitten by vacuous guards badly enough to have a memory about it;
this is the first time a worker found one in its own work by mutating rather than
by reading.

Two more things it reported that a tidier account would have omitted:

- **`keep_alive.rs`'s cap mutation (6→7) was caught by the constant-check test but
  NOT by the scenario test** that ostensibly covers eviction — that one passes at
  either cap, because it never pins the value. Not a gap, since the constant check
  exists, but it means one of the two tests is doing less than its name suggests.
- **One of its own mutation reverts silently no-op'd** on a bad path with `2>/dev/null`
  masking it, and it was caught by the harness's file-change reminder rather than
  by its own re-check. It named that rather than burying it.

The single uncovered region — 1244/1245 — is the `segments.next()?` in
`parse_three_segments` whose module doc already records it as provably
unreachable, *"confirmed rather than assumed"*.
| **P3.54** layout anchors | ✅ **MERGED `1558ba87`** | 7 files anchored. Web suite **2714 tests, exit 0**, verified by me |

#### ▶ P3.54 changed how I will dispatch the remaining 18 layout files

I asked it whether the item should have existed at all. Its answer: **the port
should own its own anchors.** Cluster 2 landed both sides itself, which closes
the exact gap a separate prerequisite exists to catch — one person picks the ids,
and there is no window in which React and Rust can drift. A standalone anchors
item belongs to **already-merged or legacy** ports, not to new ones.

**Adopted.** Wave 2's briefs will require the React ids as part of the port.

Two of its judgements are worth carrying forward as precedent:

- **`sidebar-tab-bar` gets NO id, plus an eight-line comment saying why.** The
  Rust port returns the wrapped `Tabs` element directly instead of opting a
  fabricated second root into its sink, so an id here would be one the native
  side never records — dead weight the differ can never compare. **Documenting a
  deliberate absence is what stops someone "fixing" it later.**
- **The scope-entry calls, which I would have got wrong.**
  `sidebar-project-header` needs one, because its toggle nests
  `sidebar-toggle-icon` — separately ported, and deliberately not painted by this
  composition. `workspace-branch-icon` does **not**, because its nested
  `flicker-spinner` *is* part of the composition and that surface's own test
  asserts exactly two anchors. Same nesting, opposite answers; the discriminator
  is whether the nested anchor is **foreign content or composed content**.

It also corrected the brief twice: it found cluster 2's ids before my correction
reached it, and it reported that **`sidebar-toast-overlay` has no Rust port at
all** — so anchoring it removes one prerequisite and not the blocking one. It
said that rather than letting the file look ready.

**I gated the two committed clusters myself** on a throwaway integration branch in
the main worktree — warm cache, so no cold build — rather than paying three cold
vendored-gpui builds at ~8–13 GB each. Clippy found **6 errors**: three
`float_cmp` and three `doc_markdown`, all `pedantic`, all denied workspace-wide.
Each was routed to its owner with the exact file and line. **That is the cost of
"do not run cargo", which I imposed, so it is expected rather than a lapse** —
but it is why the branch does not move until they are green.

#### One integrated gate, not three — and a worker's merge forecast, half confirmed

Three isolated gate runs would mean three cold vendored-gpui builds at 8–13 GB
each in worktrees whose `target/` was wiped: ~30 GB and ~25 minutes to establish
that each branch is green **in isolation**, which is not the question. All three
edit `surface.rs`'s registry and `components/mod.rs`. *Green together* is the
only useful answer, and it costs one warm build.

Asked to forecast the merge, cluster 2 gave two specific predictions. I checked
both against the other branches' diffs:

| its prediction | outcome |
|---|---|
| `surface.rs`'s alphabetical list is the risk — `repo-avatar` (cluster 1) and `repo-import-dialog` both anchor on the `"resizable"` line, so two independent insertions will likely apply *without* a conflict marker but may land out of order | **stands.** Worth noting it fails **loudly** either way: `the_registry_is_sorted_and_holds_every_surface` asserts the exact sequence, so a silent misordering is not among the outcomes |
| `row_surface.rs`'s full-bleed list — it rewrote "Five surfaces" to "Seven"; if cluster 3's `fps-overlay` is also full-bleed, that is a real conflict | **does not materialise.** Cluster 3 never touches `row_surface.rs` |

**The forecast was worth asking for even where it was wrong.** It named the exact
mechanism (same anchor line, independent insertions, no marker) and it named what
it could not see rather than guessing — *"I can't confirm this without seeing
their diff, but I'd flag it as plausible."* Checking a stated hypothesis against
two branches took one command; reconstructing the same risk after a bad merge
would not have.

Its third answer is the one I would have got wrong myself: **`components/mod.rs`
is not alphabetical to begin with** — `flicker_spinner`/`loading_spinner`/`spinner`
and `sidebar_carousel` all sit far from alphabetical position — so grouping the
two modals beside `dialog` follows the file's existing precedent instead of
breaking one.

**Cluster 3, asked the same question, found a collision cluster 2 could not
see:** `theme/token.rs`'s `impl Color` block. Cluster 1 adds `RED_500`/
`GREEN_500`/`VIOLET_500`; cluster 3 adds `BLACK`; **both insert immediately after
`WHITE`'s closing brace and before `fn to_srgba`.** Cluster 2 does not touch that
file, so only the branch that shares the anchor could have spotted it.

**Two forecasts, two different blind spots, and between them the whole picture.**
Neither collision is semantic — both are independent additions at one textual
anchor, and the resolution in each case is "keep both" — but git will very likely
raise literal conflict markers rather than interleave, because neither side's
diff carries enough context to auto-order. **Asking every worker to predict its
own merge, rather than asking one, is what produced the complete list.**

Its `float_cmp` reasoning is also worth keeping. It did not reach for
`.to_bits()` by analogy — it proved `painted.value().l` is exactly `0.0` *by
construction*: `color_mix_remainder` weights every channel by `c * a`, and both
`BLACK` and `TRANSPARENT` have `r = g = b = 0`, so every product and the final
division are exact. **There is no rounding step to tolerate**, which is the
argument that makes an epsilon wrong rather than merely unnecessary.

#### ✅ The integrated gate earned itself — **three** findings no isolated run could reach

`native/gate-wave1` holds all three clusters. Beyond the two predicted merge
collisions, the combined run surfaced three failures that exist **only** in the
integration:

1. **`fps-overlay: 0 real flag(s), no_state_axis() = false`.** Cluster 1's new
   biconditional judged **cluster 3's** surface and found a *second* axis-free
   case. Cluster 1 had surveyed for candidates and correctly found none — *in its
   own tree, where cluster 3's four surfaces did not exist* — and had said plainly
   it had not behaviourally re-audited the existing 45.
2. **`only_a_full_bleed_surface_gives_up_the_horizontal_inset`.** `fps-overlay`
   declares `full_bleed: true` but is absent from `row_surface.rs`'s
   hand-maintained list — a file **cluster 3 never touched**, while cluster 2 was
   independently rewriting its prose count from "Five" to "Seven" for its own two
   modals.
3. **Three (in fact four) `repo_avatar` test failures** on their first-ever
   execution, since no cargo had run on that branch.

**Three isolated green gates would have reported three green branches.** That is
the entire argument for gating them together, and it paid on the first run.

##### 🎯 (4) — a REAL PORT DEFECT, found only because the surfaces were merged

`sidebar_tab_bar` builds **four** tabs; `tabs`' own fixture has **three**.

```js
// sidebar-tab-bar.tsx
const isHomeRoute = useMatch({ from: '/_shell/ide/$projectId/home', shouldThrow: false })
const visibleTabs = isHomeRoute ? TABS.filter((t) => t.tab !== 'git') : TABS
```

**`git` is route-conditional**, and the port renders it unconditionally. My own
live capture corroborates it exactly: the running app sat at
`…/#/ide/<id>/home` and showed **three** tabs beside **four** carousel panels —
the panels are not filtered, the tabs are.

**This is the finding that most justifies the integrated gate.** `sidebar_tab_bar`'s
own tests pass on its own branch. The assertion that fails compares against
`tabs`' fixture — **a different cluster's already-merged surface**. No isolated
gate could see it, and the port would have shipped a component that renders half
its states.

**The fix is not "drop `git`".** React renders *both* shapes, so hardcoding either
models half the component. It needs a **cell axis** — a surface option selecting
home vs non-home, the way `--content` and `--flags` are — with both cells tested.
The `useEffect` that *reassigns* the active tab to `workspaces` when landing on
home while `git` is active is part of the behaviour, and whether a geometry
oracle models that transition is a call to make explicitly.

Also open: **`sidebar-carousel` is already ported with four panels.** If two
ported surfaces read the same route condition differently, that is worth knowing
before either takes a verdict.

##### On (2) I formed my own read before accepting an answer

`fps-overlay.tsx` is `fixed bottom-8 right-3 … rounded-md px-2.5 py-1.5` — a
corner badge whose width is its **content's**, not the viewport's.
`Surface::full_bleed`'s doc says the flag means the surface *fills its window* so
that "surface width and viewport width are one quantity", citing `resizable`'s
IDE-shell root. On that reading the flag is **wrong** and the fix is to drop it,
not to extend the list — which would also leave cluster 2's "Seven" correct.

Sent to cluster 3 as a data point to **reconcile, not defer to**, with the two
things that would make me wrong stated explicitly. Two of my premises today were
wrong in exactly this shape and the worker catching it was worth more than the
task both times.

`native/gate-wave1` holds all three clusters. Beyond the two predicted merge
collisions, the combined run surfaced **a failure that exists only in the
integration**:

```
assertion `left == right` failed: fps-overlay: 0 real flag(s), no_state_axis() = false
```

Cluster 1's new biconditional invariant judged **cluster 3's** `fps-overlay`
surface and found a **second** surface with no real state axis. Cluster 1 had
surveyed for other candidates and correctly found none — *in its own tree, where
cluster 3's four surfaces did not exist*. It also said plainly that it had not
behaviourally re-audited the 45 existing surfaces and flagged that as a separate
item.

**Neither worker could have found this alone, and three isolated green gates
would have reported three green branches.** That is the entire argument for
gating them together, and it paid on the first run.

The same run also caught **three failing `repo_avatar` tests** — their first-ever
execution, since no cargo had run on that branch — and a **third**
`no_state_axis` dead-code site the earlier errors had masked.

#### ✅ Both predicted collisions happened, exactly as described — and I fumbled one

`native/gate-wave1` holds all three clusters. Both forecasts landed:

| collision | outcome |
|---|---|
| `surface.rs` — `"repo-avatar"` (P3.50) and `"repo-import-dialog"` (P3.51) at the same `"resizable"` anchor | **real conflict marker**, resolved keep-both in `build.rs`'s file-name order (`repo_a` < `repo_i`) |
| `token.rs` — P3.50's `RED_500`/`GREEN_500`/`VIOLET_500` and P3.52's `BLACK` after `WHITE` | **real conflict marker**, resolved keep-both |

**I resolved the second one badly and broke the build.** I reordered the two
sides to group `BLACK` beside `WHITE` — and the construct's shared trailing
`});` sat **outside** the conflict markers, so moving the blocks left `BLACK`'s
literal unclosed. `error: this file contains an unclosed delimiter`. Repaired in
`4a251f3f`; all four constants verified well-formed.

**The lesson is narrow and worth having:** a git conflict region does not
necessarily contain whole syntactic units. Both sides here ended mid-expression
and *shared* the closing lines below the marker. **Reordering the sides is not a
safe default — concatenating them in place is**, and if a different order is
wanted, move whole constructs afterwards and recheck. I took the riskier
operation for a cosmetic gain and paid for it immediately.

#### Two workers lost track of which worktree they were in

P3.53 ran a **full gate suite against the shared worktree** and reported it as
its own: clean tree, `crowbar-core` still 349 lines, a warm 21 GB `target/`. Every
number was true — about my tree. Its six files and 1,276 lines sat untouched in
`/private/tmp/crowbar-p353-workspace-scoping` the whole time. P3.50 did the same
thing first, then found the right directory by walking every path
`git worktree list` names, **and said so in its report rather than quietly
correcting course** — which is the only reason I can tell the two cases apart.

**The shared branch was never written to** — I verified `rewrite/rust` at
`e481b33f` while P3.53 was reporting. The fail-fast `rev-parse --show-toplevel`
check at the top of every brief is what is supposed to prevent this; it catches a
*failed setup*, and neither of these was that. Both drifted later, after a
successful start. **The check has to be per-command, not per-brief**, and that is
now in the wording.

#### ✅ The synthetic `empty: bool` — the worker was right and the invariant changes

`no_surface_declares_its_entire_state_axis_unmodelled` asserts `real > 0` for
every surface. `workspace-branch-icon` has **no customization seam at all** — no
`className`, no prop spread, no hover/focus/selected rule anywhere in its React
source — so every §8.3 flag is genuinely unmodelled and the assertion can only be
satisfied by inventing a field. The worker added one, **flagged the tension
instead of hiding it**, and then argued for removing it.

Its third reason is the decisive one: a public, tested, wired-through field for a
behaviour nothing in the React contract can trigger is a trap for the next reader,
and this port's documentation discipline exists to prevent exactly that. The
invariant was built to catch a port that gives up on a **real** axis —
`sidebar_empty.rs`'s tone/error conflation is the case it was made for.

**Ruling: the exemption is the fix, and it must be loud.** Not a relaxed
assertion — an explicit per-surface declaration that *every flag was considered
and none applies*, with the test asserting that any surface at `real == 0`
carries it, and that a surface carrying it while having a modelled flag is
equally an error. Three mutations required, including a control proving the test
still catches the failure it was built for.

### ▶ Layout wave 1 — dispatched 2026-08-03, three clusters in parallel

The survey's own dependency graph decides the order, not my guess at it.

| item | branch | files |
|---|---|---|
| **P3.50** cluster 1 — the foundation | `native/p3.50-layout-cluster1` | `repo-avatar` · `workspace-branch-icon` |
| **P3.51** cluster 2 — standalone modals | `native/p3.51-layout-cluster2` | `detach-holder-modal` · `repo-import-dialog` |
| **P3.52** cluster 3 — sidebar chrome | `native/p3.52-layout-cluster3` | `sidebar-project-header` · `sidebar-tab-bar` · `sidebar-skeleton` · `fps-overlay` · `sidebar-toast-overlay` |

**Cluster 1 blocks six other files** (`repo-icon-popover`, `context-pill`,
`workspace-switcher`, `project-home-row`, `pending-create-row`,
`workspace-tree-item`), which is why it goes first. Clusters 2 and 3 have no edge
to it or to each other, so they run alongside.

All three will touch `surface.rs`'s registry list and `components/mod.rs`; each
was told to keep those edits minimal and additive so the conflict stays textual.

**Held back deliberately:** cluster 4 (`nav-stack`, `sidebar-peek`) — the survey
flags both as judgement calls following the `sidebar-carousel` precedent, and I
would rather see how the first three land before committing to that reading.

**P3.52 carries the two survey findings that would otherwise be re-derived
wrongly**: `fps-overlay` ships in every build behind a settings toggle, and
`sidebar-toast-overlay` is the toast users actually see. It is also the largest
item I have dispatched in this tier, and it was told that **four clean plus an
honest account of the fifth beats five rushed**.

#### ⚠ One of the 43 finished surfaces measures something unreachable

`toast.rs` ported `ui/toast.tsx`'s `AnchoredToasts`. `native/mapping/toast.md`
already says that component has **no code path in any environment**. Its verdict
was taken and is real; what it measures is not on screen for any user. The
reachable toast is `sidebar-toast-overlay.tsx`, dispatched above as part of
P3.52. **Not a defect to revert — a reminder that "ported and verified" answers a
narrower question than "the user sees this."**

### ✅ `command` PASSES — 0 deltas over 11 anchors. The third held verdict, and it is green.

**2026-08-03, my own run**, `native/p3.32-command-rebased` @ `21ea1ec6` built with
`--features driver`. Canaries first: **both byte-identical**. Then:

```
oracle: command · width=1714 theme=dark content=normal flags=[]
oracle: PASS — 0 deltas over 11 anchors compared
```

Gates, also mine: clippy `-D warnings` **0 errors** · **1641 passed / 0 failed /
31 `ok` lines** · **10 of 10** consecutive full `crowbar-app` bin runs green,
against a baseline where 2 of 3 failed.

**Not merged yet.** The branch sits on `native/p3.37-reference-repair`, whose
`search` and `search-replace-row` verdicts **fail** (see below). Merging it now
would drag an unverified port onto the branch to get a verified one. It waits for
P3.44.

#### The flake's real cause — a vendor animation that moves geometry

The worker did not stop at "flaky": `gpui_component::dialog::Dialog`, which
`command.rs` wraps directly, plays an **unconditional** 250ms slide-down whose
closure is `this.top(y * delta).shadow(shadow)`. I checked the vendor source
myself — `ANIMATION_DURATION` is `0.25s` at `vendor/gpui-component/src/dialog/dialog.rs:23`,
the `with_animation("slide-down", …)` call is unconditional, and **`top(y * delta)`
animates layout geometry, not just paint**. No builder field disables it.

That interacts with the capture fixpoint: `Settling::observe()` declares a frame
settled when two consecutive draws record the same **rounded** anchors, and two
adjacent samples on an ease-out tail can round equal *before the curve has
finished*. The failing test was uniquely exposed because it compared `find()`
(window-**absolute**) bounds across two separately-created windows, each riding
its own animation. Every other test in the file uses `at()` — relative to the
popup root — where the in-flight offset sits on both sides of the subtraction and
cancels. The fix is `at()`, matching the file's existing idiom: no tolerance
widened, no retry, nothing serialised. **It was pre-existing**, reproducing 3 of 8
on P3.32's own commit before the merge.

#### ⚠ …but it does NOT reach real captures, and that distinction matters

Read alone, the above says every `Dialog`-based capture is suspect. **It is not.**
Measured by me — 6 captures each of `dialog`, `alert-dialog`, `sheet`, `popover`
and `tabs`, 30 in total:

```
dialog        distinct hashes: 1 of 6
alert-dialog  distinct hashes: 1 of 6
sheet         distinct hashes: 1 of 6
popover       distinct hashes: 1 of 6
tabs          distinct hashes: 1 of 6
```

The platform frame loop draws at a real cadence, so by the time two consecutive
*real* draws agree the 250ms animation is long over. The in-process harness
advances `simulate_next_frame` with no wall clock between samples, which is
exactly the condition that lets the ease tail plateau. **The hazard is confined
to the test harness.**

**The trap for the next surface**, recorded because it is invisible until it
bites: any `row_layout` test that compares two independently-opened
`Dialog`-based windows by **absolute** coordinates is exposed to this. Use `at()`.

### ✅ §17.1 precondition — **240 of 240 matrix cells render and emit**, 0 refused, 0 hung

**2026-08-03, my own run**, `rewrite/rust` @ `9a5b7f4d` built with `--features
driver`. Every one of the **40 registered surfaces** × {800, 1200, 1714} ×
{light, dark}:

```
cells: ok=240 refused=0 hung=0
```

The surface list is read from the binary's own `--help` rather than hand-copied,
so a surface that appeared or vanished cannot silently drop out of the sweep.

**What this is and is not.** It is the *precondition* for §17.1 and nothing more:
every cell the matrix names can be produced, so no surface is unmeasurable and no
cell refuses. It is **not** convergence — that needs a reference at each cell, and
today the references are one cell (`1714 · dark · normal`) for most surfaces plus
a light cell for four. **Condition 1 remains open, and the gap is references, not
the native side.**

Two things it settles that were open:

- **The AeroSpace clamp does not bite at 1714 with the WM disabled.** 1714 was
  granted in all 80 of its cells. P3.35's guard would have refused every one of
  them otherwise, loudly — 0 refusals is a positive result about the window, not
  an absence of checking.
- **No surface hangs.** Which is what P3.41 should have been measured against
  before I called a regression.

### 🔍 §17.1's matrix is thinner than the record claims — measured, not suspected

Having 240 native cells on disk made it cheap to ask what the matrix's two
non-content axes actually *discriminate*. Both answers were worse than assumed.

#### The viewport axis is inert on **every** surface

All 40 surfaces produce a **byte-identical snapshot at `--viewport-width 800` and
`1714`** (state block excluded). Not one field moves.

This is not a defect: these are primitives laid out at `--width`, and
`--viewport-width` only sizes the window. But it means **"≥3 viewport widths" from
§8.3 is currently measuring nothing**, and a run that reports three viewport cells
per surface is reporting one measurement three times. The one merged surface that
would break the tie — `command`, whose root is `min(viewport − 2·padding,
max_width)` — is **not merged** (see P3.32 above).

**So §17.1's width axis is honestly one cell wide today.** Written down rather
than left to be inferred from a green matrix.

#### The theme axis is vacuous on **10 of 40** surfaces

`button`, `crowbar-mark`, `crowbar-wordmark`, `flicker-spinner`, `resizable`,
`scroll-area`, `sidebar-carousel`, `sidebar-header`, `sidebar-toggle-icon`,
`spinner` are byte-identical in light and dark — because **every colour on every
one of their anchors is `#00000000`**. `button`'s single anchor paints a fully
transparent `bg` *and* a fully transparent 1px border at its default variant.

The axis itself works: `tabs` flips `bg #f5f5f51c ↔ #2626260f` and
`#1f1f1eff ↔ #fdfdfcff` exactly as it should. These ten simply have nothing that
could flip.

#### Two of the four recorded light-cell verdicts are **vacuous**, and one reference is defective

I re-took all four myself rather than trusting the earlier record:

| surface | light verdict | worth |
|---|---|---|
| `sidebar-empty` | **PASS**, 0 deltas / 2 anchors | ✅ **real** — `sidebar-empty-message.fg` flips `#a4a4a4ff → #484848ff`, a compared field |
| `scroll-area` | **PASS**, 0 deltas / 2 anchors | ⚠ **vacuous** |
| `sidebar-header` | **PASS**, 0 deltas / 1 anchor | ⚠ **vacuous** |
| `dialog` | **FAIL**, 3 anchor-presence deltas | ‼ **the reference is defective** |

**Why the two passes are vacuous, with the mechanism rather than a guess.** The
reference *does* differ between themes on those surfaces — but only in
`border.color`, `#ffffff0f → #00000014`, on a border whose **`w` is 0**. ANCHORS.md
rule 2 says `border.color` is *"compared only when `w > 0`"*, because a zero-width
border's computed style returns the inherited text colour and is junk;
`diff.rs::compare_border` implements exactly that (`if e.w > 0.0 && a.w > 0.0`).
The only other difference is a 0.02px width, inside tolerance. **So those two
light cells could not have failed differently from their dark cells.** They are
correct results that carry no information.

**`/tmp/p3-ref-dialog-light.json` holds 1 anchor where the dark reference holds
4** — `dialog-popup` only, missing `dialog-header`, `dialog-title` and
`dialog-footer`. It is a partial capture. P3.36's guard cannot see this: it checks
a snapshot against *itself*, and a truncated capture is internally consistent.
**`dialog` therefore has no valid light-cell verdict** and needs a re-capture on
the reference side. Its dark verdict is unaffected.

#### ⚠ My `--width` / `--viewport-width` error, again

Two of the four re-takes first came back FAIL with `w: 320.0, expected 343.98` —
because I drove them at the default `--width 320` against references captured at
344. Both passed once driven at `--width 344`. **This is the same mistake I have
now made repeatedly**: `--width` is the surface, `--viewport-width` is the window,
and the reference's own root bounds state which is which. Read the reference's
`state` **and** its root `bounds` before driving. Every time.

### ⚖️ The two held verdicts are TAKEN — both FAIL, and the failures are narrow

**2026-08-03, my own run**, on `native/p3.37-reference-repair` built with
`--features driver`. Canaries first, as always: **both byte-identical**, so the
gate itself is sound and the deltas below are the surfaces'.

```
native-short.json                IDENTICAL
native-file-tree-selected.json   IDENTICAL

search               FAIL — 4 deltas over 10 anchors (1 presence, 3 geometry)
search-replace-row   FAIL — 5 deltas over  6 anchors (3 geometry, 2 typography),
                            4 more forgiven by v1.5 content-sizing
```

**The ports are close.** `search` matches **9 of its 10 anchors byte-exactly** —
root, `input-control`, `search-close`, both nav buttons, the replace toggle and
all three toggle buttons. `search-replace-row` matches its root and its icon
exactly, and its two labelled buttons converge on bounds. Every delta reduces to
three causes:

| # | defect | evidence |
|---|---|---|
| 1 | the `input` anchor is on the wrong box | reference insets it from `input-control` by **33px** a side in `search` and **1px** a side in `search-replace-row`; native uses **12px** in both. Vertically the reference sits it 5px below the control top, native 1px. Height (30) is right in both. |
| 2 | line height on the two labelled buttons | `search-replace-all` and `search-replace-confirm`: native `font.line_height` **22.5** (= 15 × 1.5, a default multiplier), reference **20.0**. Bounds already converge, so this is typography only. |
| 3 | `search-toggle-icon` cannot be a shared id | native carries **one** (the *last* of three drawn — `AnchorRegistry::record` silently replacing); the reference carries **none**, because P3.37 repaired its three duplicates by deleting all three. |

Defect 1 is the interesting one: a **constant** 12px where the reference produces
two different insets is the shape of a hardcoded padding standing in for
something derived.

Defect 3 is the v1.8 asymmetry showing up as a live parity failure rather than as
a note — the DOM extractor throws on a duplicate declared id, the native recorder
replaces. **Ruling: the toggle icons are not anchored inside `search` on either
side.** `search-toggle-icons` is already its own registered surface, so no
coverage is lost, and the three toggle *buttons* that contain them already
converge to the pixel. Unique per-toggle ids are refused: they would grow the
contract to describe geometry another surface already covers.

Returned as **P3.44** (`native/p3.44-search-input`) with the numbers above.
P3.42 owns making the recorder refuse duplicates rather than replace them; the
two do not overlap.

### ❌ P3.41 — RETRACTED. There was no regression. The fault was my build command.

> **Retracted 2026-08-03, within the hour, by the worker I dispatched to bisect
> it.** `crowbar-app`'s snapshot code lives behind a cargo feature:
> `crowbar-app/Cargo.toml` has carried `default = []` and
> `driver = ["dep:crowbar-driver"]` since the crate's **first commit**
> (`1cf92296`) — one hunk in that file's entire history. Without it, `main.rs`
> compiles the `#[cfg(not(feature = "driver"))]` branch, which never reads
> `CROWBAR_ROW_SNAPSHOT` and never calls `cx.quit()`. It opens an ordinary
> interactive window and sits in `-[NSApplication run]` for ever, **exactly as
> designed**.
>
> Every "HUNG" cell below was built with `cargo build --bin crowbar-app`. The
> preserved control was built, months of sessions ago, **with** `--features
> driver`. So my 2×2 compared a driver build against three non-driver builds. The
> binary *was* the variable — just not for the reason I gave.
>
> **Verified by me on the tip after the correction:**
> ```
> cargo build -p crowbar-app --bin crowbar-app --features driver
> → capture exits in ~2s, and native-short.json is BYTE-IDENTICAL to the
>   committed canary
> ```
> Both Phase 1 canaries then came back identical from the P3.37 build too, and I
> took two real verdicts on it the same hour. **The capture path is healthy and
> has been throughout.**
>
> ### The part that matters more than the mistake
>
> Two commits earlier I wrote, in `blocked/locked-screen-blocks-every-capture.md`,
> that *"a control that contains the suspect is not a control"* — and then
> immediately ran a control that differed from the suspect in a **way I had not
> established**, and read the difference as proof of a regression. **I checked
> that the two binaries came from different commits. I never checked they were
> built the same way.** A control is only a control with respect to a variable you
> have actually held fixed, and "built from an older commit" is not the same
> statement as "built identically from an older commit".
>
> The worker also caught that my `sample` output contained **no `crowbar_driver`
> frames at all** — which is what a binary with the driver feature compiled out
> looks like, and which I had read as "never reaching the emit". The evidence was
> already pointing at the answer.
>
> ### What survives
>
> - **`--features driver` is mandatory** for any capture build. It is now written
>   down here, which it was not.
> - The lock-vs-defect discriminator (`CGSSessionScreenIsLocked`) is still worth
>   running, and is still cheap. Add a third question to it: **is this binary even
>   built with the driver feature?** `strings <binary> | grep row_snapshot`
>   answers it in one command.
> - **The 2026-08-02 screen lock was real and my original record of it was
>   correct.** `lsappinfo front` returning `loginwindow` is a direct observation.
>   My retraction of it was the wrong call, and is itself retracted in
>   `blocked/locked-screen-blocks-every-capture.md`.
>
> Everything below this line is **what I believed for about forty minutes**, kept
> because the measurements in it are real and only the conclusion was wrong.

**2026-08-03.** `crowbar-app` on `rewrite/rust` @ `5da59b8f` **cannot complete a
capture at all.** Not the hard surfaces — the Phase 1 canary, the simplest
invocation in the project:

```
crowbar-app --width 294 --content short --added 1 --deleted 0 --no-directory
```

It prints its two startup lines and then never exits.

#### What I measured, before inferring anything

Two binaries × two window-manager states, the **same** invocation in all four
cells, run back to back within two minutes:

| binary | AeroSpace enabled | AeroSpace disabled |
|---|---|---|
| built ~2026-07-31 (preserved as `scratchpad/control-jul31-crowbar-app`) | **exit 0, snapshot written** | **exit 0, snapshot written** |
| `rewrite/rust` @ `5da59b8f` | **HUNG** | **HUNG** |

A third binary, `native/p3.37-reference-repair` @ `2c4ff053`, also hangs. That
branch is **missing the last 10 commits** of `rewrite/rust`, so the regression is
at or before its merge base — it is not P3.38/P3.39/P3.40.

**The window manager is not the variable and the screen is not locked** — I
checked `IOConsoleUsers` for `CGSSessionScreenIsLocked` immediately before and
after, and the key is absent, which is what an unlocked session looks like. The
**binary** is the variable.

#### ‼️ Which means several "blocked by the screen lock" entries above are wrong

I have repeatedly recorded hung captures as *"the screen re-locked"*. At least
the most recent ones were **this defect wearing the lock's signature**, and the
two are indistinguishable from the outside: both leave the process alive in
`-[NSApplication run]` with no snapshot and no error. **I did not have evidence
for the attribution I wrote down** — I had a hang and a prior. The distinguishing
test costs one command and I did not run it:

```
python3 -c "import subprocess,plistlib; \
  print([k for u in plistlib.loads(subprocess.run(['ioreg','-n','Root','-d1','-a'],\
  capture_output=True).stdout)['IOConsoleUsers'] for k in u if 'Lock' in k])"
```

Empty list → **not locked** → a hang is the app's fault. Run it before writing
"the lock" in this file ever again. The control binary is the second half of the
test: if it captures and the tree's binary does not, the tree is at fault.

#### Where it is stuck — `sample`, not a guess

3s of `sample` on the hung process, main thread:

```
-[NSApplication run] → _DPSNextEvent → __CFRunLoopServiceMachPort → mach_msg   1994/2005
__CFRUNLOOP_IS_SERVICING_THE_MAIN_DISPATCH_QUEUE__
  → gpui_macos::window::step → gpui::window::Window::new::{{closure}}            10/2005
```

So it is **not deadlocked and not idle**: frames are still ticking through
`window::step`, and the process is simply never reaching the emit. That points at
the fixpoint in `crowbar-driver::frame`, whose whole contract is *"capture the
first completed draw that reproduced the previous draw's recorded anchors"*:

- `arm()` re-registers via `Window::on_next_frame`, which fires on the next frame
  **request**. It does not itself request one.
- `Settling::observe()` only increments `changed` when a draw is *observed*, so
  `UNSETTLED_FRAME_LIMIT = 300` — the guard whose doc says *"a hang is the
  incorrect outcome"* — **cannot fire if the observations stop arriving.** The
  limit counts changing draws, not elapsed time, and the failure mode here is
  the absence of the second draw rather than three hundred different ones.

That is a hypothesis from the stack and the source, not a verdict. **P3.41 is
dispatched to bisect it** (`git bisect` over build+probe, with the control binary
re-run between steps as an environment canary so a real lock cannot be mistaken
for a bad commit a second time).

#### What this does and does not invalidate

- **Verdicts already taken remain valid.** A capture that *completed* completed
  on a settled frame; this defect withholds snapshots, it does not corrupt them.
  Nothing here says a recorded PASS was wrong.
- **The "blocked" attributions are what is unreliable**, and with them my claim
  that the only thing standing between this port and its remaining verdicts was
  the user unlocking a machine. Some of that was mine to fix and I filed it under
  someone else's inconvenience.
- The three held verdicts (`search`, `search-replace-row`, `command`) are
  **still held**, now for a stated and reproducible reason.

#### Also found while setting this up — `command` was never on this branch

`native/p3.32-autocomplete-command` (@ `1118c994`) is **28 commits behind** and
was never merged, so `command` is absent from the surface registry on both
`rewrite/rust` and `native/p3.37-reference-repair`. Its verdict could not have
been taken from either, lock or no lock. Rebasing it is dispatched separately;
its verdict has to be taken on a tree carrying the two font fixes (P3.24 fallback
chain, P3.25 weight 600), both of which move measured text widths.

`native/p3.37-reference-repair` is 10 commits behind for the same reason and
needs the same treatment before its verdict counts.

#### ⚠ P3.32 rebased — and my own gate run caught a flake the worker's did not

`native/p3.32-command-rebased` @ `ed4f8f7d` merges the never-landed
`native/p3.32-autocomplete-command` onto `native/p3.37-reference-repair`. One new
surface: **`command`**. `autocomplete` is deliberately *not* its own surface —
its anchor ids are folded into `command`'s declared scope, because `command.tsx`
restyles `autocomplete.tsx`'s boxes by `className` and never overrides their
`data-slot`.

The worker reported **1641 passed / 0 failed / 31 `ok`**. My own run of the same
commit reported **716 passed / 1 failed / 1 `ok`**:

```
row_layout::command::selected_is_unmodelled_and_renders_the_same_picture
  left:  origin.y 58.5px
  right: origin.y 58px      — size and x identical
```

| how I ran it | result |
|---|---|
| filtered to that test, ×3 | **ok** every time |
| the whole `--bin crowbar-app` set, ×3 | **FAILED · FAILED · ok** |

**It passes alone and fails in company, about two runs in three.** Half a pixel
of `origin.y` with the size unchanged is shared state or ordering between tests
in one process, not a layout defect.

**The worker was not wrong to report what it saw — it got a lucky run.** And the
numbers were themselves the tell, which I nearly missed: `cargo test --workspace`
**aborts at the first failing binary**, so on a bad run only 716 of ~1641 tests
execute. 1641/31-`ok` and 716/1-`ok` are not two samples of the same population;
the high total is only *reachable* on a run where the flake passed. A lower total
from my own run means "aborted", not "different suite".

Returned to the worker with the tally, `--test-threads=1` and `--shuffle-seed` as
the discriminators, and a requirement of **10 consecutive green full-binary runs**
before I look again. Widening the tolerance, retrying, or serialising the binary
to hide it are all refused in advance.

### Wave 4 (P3.15–P3.18) — dispatched 2026-07-31

| Item | Branch | State |
|---|---|---|
| **P3.15** wrap `popover` + `select` | `native/p3-wrap-popover-select` @ `04ca276d` | ✅ **MERGED `f1d71391`** — `popover` **PASS, 0 deltas**, verified by my own capture |
| **P3.16** gpui `inspector` spike | `native/p3.16-inspector-spike` @ `ea543b24` | ✅ **REPORTED — path REJECTED, branch deliberately not merged.** Also corrected my seam survey 7 → 3 |
| **P3.17** two-frame capture | `native/p3.17-two-frame-capture` @ `d465eb6d` · combined `native/p3.17-popover-reduction` @ `ecc6d242` | ✅ delivered — under my verification |
| **P3.19** `sidebar` (wrap) | `native/p3.19-sidebar` @ `fe7dfaac` | ⏸ **HELD** — gates green, but no capture is possible (locked screen) |
| **P3.20** `scroll-area` + `keybinding` | `native/p3.20-scroll-keybinding` @ `b8225e58` | ✅ **MERGED `0af9e9ab`** — both **PASS 0 deltas** once P3.24 landed |
| **P3.24** font fallback chain | `native/p3.24-font-fallback` @ `f1eaec43` | ✅ **MERGED `4ce9e8c4`** — verified by my own run |

#### ✅ P3.24 + P3.20 — verified and merged. The tofu is gone.

My own run on the merged tree:

```
keybinding   oracle: PASS — 0 deltas over 1 anchor compared
scroll-area  oracle: PASS — 0 deltas over 2 anchors compared
canaries     IDENTICAL native-short.json / native-file-tree-selected.json
```

P3.24 alone: clippy `-D warnings` 0 · **1181 passed / 0 failed** · 7 `ok` · both
canaries regenerated **by me** byte-identical. Metrics checked against WebKit,
not against the worker's table: `⌘` native **11.027345** vs **11.027**; `⚠`
native **16.884** vs **16.875**.

**The mechanism was already there.** `Font.fallbacks` → `TextStyle` →
`apply_features_and_fallbacks` → `kCTFontCascadeListAttribute` — never populated.
Setting *any* non-empty list also makes CoreText append its own default cascade,
so the backstop is the OS font list rather than a hand-picked set.

Exactly one registry surface changed: **`inline-error`**, whose `⚠` U+26A0 is
*also* absent from CalSansUI — `text_width` moved from `18.0` (1em `.notdef`) to
`16.884`. It has **no reference** (its mapping records refusing to fabricate
one), so no verified pair moved.

#### P3.37 — references repaired; verdicts BLOCKED by the screen lock

`native/p3.37-reference-repair` @ `2c4ff053` (on a merge folding in the
never-landed `native/p3.33-search`). Gates by my own run: clippy 0 · **1604
passed / 0 failed** · 7 `ok`. **All three references now load cleanly** under
P3.36's guard — no duplicate ids, no zero-area-visible anchors:

| reference | root | anchors |
|---|---|---|
| `p3-ref-search.json` | `search-popover` | 10 |
| `p3-ref-search-replace-row.json` | `search-replace-row` | 6 |
| `p3-ref-command.json` | `command-dialog-popup` | 11 |

**The verdicts were NOT taken.** The screen re-locked mid-run and every native
capture hangs. AeroSpace was restored by hand afterwards — my `trap` could not
fire while the capture hung, so the WM sat disabled until I killed the process.
**Lesson: a `trap`-based restore does not protect against a command that never
exits.** Kill first, restore second.

#### ‼️ The two extractors disagree on v1.8 — one refuses, one shrugs

Found while proving `search-replace-row` needs its own surface rather than a
scope entry:

- **DOM side**: a duplicated declared anchor id **throws**
  (`oracleSelectDeclaredAnchors`).
- **Native side**: `AnchorRegistry::record` **silently replaces** a repeated id,
  and `Snapshot::build` copies every recorded anchor regardless of which id
  `root` names.

So a native capture asked for `root: "search-replace-row"` would still carry
`search-popover` and the rest. That asymmetry is exactly the "one side shrugs
where the other fails" shape ANCHORS exists to prevent, and it is **not** fixed
by P3.36 (which guards the *loaded snapshot*, not the *recording*). Worth an item.

#### ⚠ Two reservations about how P3.37 captured, recorded so they are not lost

1. **It removed real DOM elements before capturing `command`** — four `kbd`s
   (a separately-registered surface) and a live `input-control` — to reproduce
   the original 11-anchor set. The reasoning is defensible, but **removing real
   elements to make a capture match an expectation is the manipulation class
   that produced the fabricated reference**. If a scope entry is the right
   mechanism, it should be a scope entry, not surgery on the live DOM.
2. **`command`'s original `visible: true` is explained only as "the file was
   stale/bad".** It proved no *live* extractor bug — `bounds` and `visible` come
   from the identical `box` on adjacent lines, and the area check has been there
   since the first commit — which is the important half. But how the original
   file acquired the contradiction is still unexplained.

#### ✅ TIER B COMPLETE — `context-menu` closed by building nothing

The last outstanding item, and the right answer was **no port**.

`context-menu.tsx` is **two components in one file**. The declarative family
(`ContextMenuRoot`/`Trigger`/`Content`/`CheckboxItem`/`RadioItem`/`Sub*`) is
**dead** — all 14 exports grepped individually, zero real importers; the only two
apparent hits are **comments** in `block-context-menu.tsx` explaining why it does
*not* reuse those names, and that file is Plate, out of scope by §3.2. Verified
by me.

The live half, `ImperativeContextMenu`, is built on the same `@base-ui` `Menu`
primitive `dropdown-menu.tsx` uses. That is exactly the case the user's
native-menu ruling moved to `NSMenu` in P2.14, so it inherits §5.2's **judged**
treatment and no oracle is invented for it.

**What the two live call sites actually need**, enumerated from the call sites
rather than the type: `id`, `label`, `icon`, `onClick`, `separator`. `disabled`,
`shortcut`, nested `items` and `closeOnClick` are declared and unused;
`keybinding` is used once and is **dead** — the render loop never reads it.

#### ⚖️ `crowbar-platform::native_menu` — the standing question, answered

`blocked/s13-native-menus-accepted-delta.md` said it is *"retained for now and
retired before Phase 3 closes unless a concrete need appears that the vendored
one cannot serve."* **No such need appeared, and the evidence argues for
retirement:**

| | `crowbar-platform::native_menu` | vendored `gpui_component::native_menu` |
|---|---|---|
| platforms | macOS only | macOS + Windows + drawn fallback |
| blocking | **yes** | no |
| dispatch | — | `Box<dyn Action>` |
| **icons** | **none** | **yes** |

**Both live call sites put an icon on nearly every row** — which only the
vendored menu supports. And **neither implementation is wired to anything
today**: the only hits are `lib.rs`'s own `pub mod` and re-export.

Also relevant to sequencing: **neither `tabs.rs` nor `file_tree_row.rs` has any
right-click wiring**, so there is no host to attach a menu to regardless of which
API wins. The native context-menu work belongs to whichever phase adds those
hosts.

#### ✅ P3.39 — a doc claim that outlived its evidence

Three places asserted *"no live call site renders a Button with a label"*. The
`search` port found `SearchReplaceRow`'s Replace/All **are** labelled and live
(`75.81×32`, `39.38×32`). The sentence is **true of `crowbar-ui::Button` and
false of the product** — those buttons are hand-built precisely because `Label`
is a closed enum. **Sharpened, not deleted**: the `--width` weakness it documents
is real and `Button`'s own width path genuinely stays unexercised.

#### ✅ P3.36 merged — a malformed reference can no longer reach a verdict

The check runs at **load** time, on **both** snapshots, before either reaches the
differ, and **refuses** rather than warns — matching every other rule in that
loader, whose stated principle is that anywhere it could shrug is somewhere the
two extractors could disagree without the gate noticing.

Verified by me against the **real** defective reference, not a synthetic one:

```
oracle: /tmp/p3-ref-command.json is not a v1 snapshot: `anchors[5].visible`:
anchor `autocomplete-empty` has `visible: true` with bounds 574.0x0.0 …
```

and a known-good archived pair still diffs normally (PASS, 0 deltas over 8
anchors). clippy 0 · **1562 passed** · 7 `ok`.

**The rejections are what make it trustworthy.** I listed six candidate rules; it
implemented one, confirmed two already existed (duplicate id, root-at-origin) and
**refused four**, each with the contract sentence that makes adding it wrong:
`text_width`/`text` mismatch is a `FieldPresence` delta per v1.1 ruling 5, not a
load error; `content_sized`/`line_sized` on a textless anchor is ungrounded;
`border.color` with `w == 0` is explicitly legal per v1.3 #2; nothing orders
`root` relative to array position. A false refusal on a valid reference would
have been worse than the gap being closed.

It swept all **107** archived files as **52** pairs, before and after,
byte-for-byte — zero differences — and scanned every anchor in `runs/` and
`corpus/` for the violating combination: zero hits.

#### TWO DEFECTIVE REFERENCES, both caught by guards rather than by luck

`search` and `command` both came back FAIL. **Neither is a port defect** — both
references are malformed, and in each case an existing guard named the problem
precisely.

**`search`** — the differ **refused the reference outright**:

```
/tmp/p3-ref-search.json is not a v1 snapshot: `anchors[8].id`:
anchor id `search-toggle-icon` appears twice; the differ matches by id
and would have no way to say which of the two it compared
```

The capture nested one `search-toggle-icon` inside each of the three toggle
buttons — an **ANCHORS v1.8** duplicate-id violation. The native side emits one.
Fix is an `oracleSurfaceScope` entry for `search`, exactly the pattern P3.18
established for `popover`/`select`.

Its second reference, `p3-ref-search-replace-row.json`, is **rooted at
`search-replace-row`, which is not a registered surface** — the registry has only
`search` and `search-toggle-icons`. So it cannot be compared as-is either; the
`--replace` cell needs either its own surface or a scope entry.

**`command`** — `autocomplete-empty` carries `h: 0` with `visible: true`, which
the contract forbids (*"actually painted … non-zero area …"*, implemented in
`oracleIsVisible` as `width > 0 && height > 0`). It is the **only** reference on
disk with a zero-area anchor marked visible.

**What this says about the harness, which is the point:** two independent workers
produced malformed references, and **both were caught** — one by the differ's own
v1.8 schema check, one by a hand audit that is now being automated (P3.36). The
guards are doing their job. What is missing is that the v1.8 check fires only at
diff time, so a bad reference can sit on disk looking authoritative until someone
tries to use it.

Both surfaces are **HELD pending re-capture**, not returned as port defects.
`search`'s gates are green (clippy 0, **1557 passed**, 7 `ok`) and `command`'s 10
of 11 anchors are byte-identical.

#### ✅ RESOLVED — the WM was the whole cause, and the guard proves it

With AeroSpace tiling temporarily off (restored in the same command, via a
trap), on a correctly-granted window:

```
dialog    oracle: PASS — 0 deltas over 4 anchors compared
command   root 576×142 — matches the reference exactly; all six geometry
          deltas GONE
```

So the constant −39 was entirely the tiling WM shrinking the driver window, and
**P3.35's guard is correct rather than over-strict**: it refuses exactly the
surfaces whose layout reads `viewport_size()` (`dialog`, `alert-dialog`,
`sheet`, and `command` on its branch) and lets every other surface through.

**The durable fix is a float rule for the driver window in
`~/.config/aerospace/aerospace.toml`** — scoped to our own binary, affecting
nothing else the user runs. That is a change to the user's environment, so it is
**recommended, not made**. Toggling `aerospace enable off/on` around a capture
batch works meanwhile and is what I used.

#### `command`'s last delta is a DEFECTIVE REFERENCE, not a port defect

After the window was fixed, one delta remained:

```
autocomplete-empty.visible: false, expected true
```

Geometry is **identical on both sides** — `{x:1, y:50, w:574, h:0}`. A zero-area
box. And the contract already rules on this: ANCHORS' field table defines
`visible` as *"Actually painted: not `display:none`, not `visibility:hidden`,
zero opacity …, **non-zero area**, not fully clipped"*, and `oracleIsVisible`
implements it — `if (!(box.width > 0 && box.height > 0)) return false`.

**So a reference anchor with `h: 0` and `visible: true` contradicts the
extractor's own logic.** I audited every reference on disk:
`/tmp/p3-ref-command.json` is the **only** one with a zero-area anchor marked
visible. Every other reference is self-consistent.

**The native side is right; the reference is wrong.** `command` is HELD pending a
re-capture, not returned to the worker as a port defect — 10 of 11 anchors are
byte-identical and the 11th is a bad reference field.

**Worth keeping:** a reference can be internally inconsistent with the contract
that produced it, and nothing checks that today. The audit above — *zero area
must imply `visible: false`* — is cheap and should run over every reference
before it is trusted.

#### ‼️ THE DRIVER CAN CLAIM A CELL IT WAS NOT MEASURED IN

`command` diffed **FAIL — 7 deltas**, six of them a **constant −39** on every
width anchor (576 → 537). A *constant* delta is the documented tell for a
container-width mistake, so I chased it before returning anything.

**It is environmental, and I initially called it a port defect — wrong.**

- `command`'s root is **537 regardless of `--width`** (576/700/1200 all identical)
  **and regardless of `--viewport-width`** (800/1200/1714 all identical).
- `crowbar-ui/src/components/command.rs` sizes from the **granted** window:
  `outer_width = min(window.viewport_size().width - 2*VIEWPORT_PADDING, max_width)`,
  with `VIEWPORT_PADDING = 16`. So the granted viewport is **569px**, not the
  1714 requested.
- **AeroSpace is running** and is tiling the driver window — the same hazard that
  produced the "misplaced window" and "37px display shortfall" misdiagnoses in
  Phase 1.

**Why nothing caught it until now.** The control is decisive: on the same binary
and the same window, **`dialog` still measures exactly 448**, its reference. Every
previously-verified surface either caps below the clamp (`dialog` 448, `popover`
256, `scroll-area` 344, `tooltip` 99, `number-input` 112) or takes its width
directly from `--width` (`slider` 668). **`command` is the first surface whose
layout derives from the viewport**, so it is the first to expose this.

**The defect in the harness, which is the real finding:** the snapshot's
`state.width` records the **requested** width while layout used the **granted**
one. When they diverge the capture is measured in one cell and **labelled as
another** — and it fails in a way that looks exactly like a port bug. P3.17 added
`row_snapshot::cut_by_the_window` for a surface *cut* by the window; this case is
not cut, merely laid out narrower, so nothing refuses.

**`command` is HELD**, not failed. Its gates are green (clippy 0, **1536 passed**,
7 `ok`) and 10 of 11 anchors were byte-identical in the worker's own run at a
correct window. A guard is dispatched (P3.35); the verdict is re-taken after.

#### ✅ P3.30 — `slider` verified on TWO cells; `tree-row` ruled out

```
slider resting              oracle: PASS - 0 deltas over 4 anchors
slider value 40, selected   oracle: PASS - 0 deltas over 4 anchors
```

**The first surface here with a value/flag axis actually exercised.** Its seam was
confirmed by reading `vendor/gpui-component/src/slider.rs` rather than trusting my
survey — `RenderOnce::render` builds bar, fill and thumb inside its own body,
`Styled::style` is the only seam, and none of the three shapes my survey missed
elsewhere are present. Genuinely style-only; hand-built on `switch`'s precedent.

**`tree-row` is not a distinct surface and nothing was built.** Its only DOM
output — a `<button>` — is already anchored and verified twice, as
`git-row-button` (part of the passing Phase 1 gate) and `file-row-button`. Both
exported constants are dead; every live consumer overrides them.

#### ‼️ A latent ORACLE bug — bounded, Phase 1 unaffected

`ANCHORS.md` §3's pseudo-backed shortcut **assumes the `::before` is `inset: 0`**
— `extract.ts` states it in its own comment and returns the **host's padding
box**. `slider`'s track carries `before:inset-x-0.5`, so the shortcut yields
`{x:0, w:668}` where the truth is `{x:2, w:664}`.

**Blast radius, checked by me:** the only two anchors using that path are
`git-row-item` and `file-row-item`, both resolving to `.file-tree-item::before`,
which **is** `inset: 0`. **So the Phase 1 gate is unaffected**, and `slider` is
the first component to violate the assumption. The worker hand-corrected its own
reference and deliberately left the shared infra alone.

**Open item:** make the pseudo path read the pseudo's *own* computed inset
instead of assuming zero. Until then, any new pseudo-backed anchor must have its
inset checked by hand.

#### ⚠ My own driving error, third occurrence

I first diffed `slider` at the **default `--width 320`** against a **668**-wide
reference and got `FAIL — 2 geometry deltas`, plus an empty verdict on the second
cell for a missing `--flags selected`. Both were mine, not the port's. This is
the exact constant-delta mistake I warn every worker about **in writing**, and it
is the third time. **Derive cell parameters FROM THE REFERENCE before driving** —
read its `state` and root `bounds` first, every time.

#### The last three, scoped before dispatch — two shrink

| item | what it actually is |
|---|---|
| `search` | ordinary port; imports the already-verified `Button` + `Input`; **0** matching lines |
| `primitive-dialog-service` | **composes the already-verified `dialog`** — imports `Dialog`/`DialogContent`/`DialogHeader`/`DialogTitle`/`DialogFooter` and adds a request queue. Its boxes are `dialog`'s boxes, so the expected outcome is *"covered by `dialog`, no new surface"*. Briefed to **rule on scope before building anything** — inventing an unnecessary surface is worse than an unported one, because it creates a thing to verify. |
| `context-menu` | **not a hook-only file** — my earlier read was wrong. It renders base-ui `MenuPrimitive.Root/Portal/Positioner/Popup/Item/Separator`, i.e. real boxes. But the **user's native-menu ruling** covers it, and P2.14 already moved `dropdown-menu`'s context-menu path to an `NSMenu`, recorded as **"deliberately not a parity result"**. So it belongs in §5.2's *judged* treatment, not the anchor-diff gate. |

`context-menu`'s two non-Plate importers are `tab-context-menu.tsx` and
`use-file-explorer-context-menu.tsx`; its third, `block-context-menu.tsx`, is
Plate (`^block-`) and out of scope by §3.2.

**Consequence:** of the eight that remained after Wave 5, only `search` plus
whatever `slider`/`tree-row`/`autocomplete`/`command` turn out to be are ordinary
anchor-diffed ports. `context-menu` is a §5.2 judged item and
`primitive-dialog-service` is probably already covered.

#### ‼️ CORRECTION — the "fuzzy group" is not a fuzzy group

I have been recording `autocomplete`, `command`, `search` (and `inline-combobox`)
as items that "touch fuzzy matching, where §10.1 says use Zed's `fuzzy_nucleo`".
**Measured, and that is wrong on both halves:**

- **None of the three implements matching.** Grepping each for
  `.filter(` / `score` / `fuzzy` / `match(` / `includes(` / `indexOf` returns
  **0 lines** in all three. `autocomplete.tsx` delegates to
  `@base-ui/react/autocomplete`, `command.tsx` composes a base-ui dialog, and
  `search.tsx` is a `Button` + `Input` shell.
- **The real matchers live outside `components/ui/`** — `web/src/utils/fuzzy-matcher.tsx`,
  `web/src/utils/search-match.ts`, plus `features/editor/{completion,lsp,stores}`
  and `components/layout/workspace-switcher.tsx`.

So `fuzzy_nucleo` belongs to whatever ports **`utils/fuzzy-matcher.tsx`**, which
is Phase 4 work, not Tier B. **It is also not vendored** — absent from
`native/vendor/` and from every `Cargo.toml`. Vendoring it is a separate item
whenever that phase starts.

**Consequence:** these three are ordinary component ports with no algorithm
attached, and can be dispatched as such. That removes the last "special
handling" flag from the remaining set except `context-menu` (native menus, a user
ruling) and `slider` (style-only, fork).

This is the third time a name-derived assumption in my own queue turned out
wrong — after `table-icons`/`inline-combobox` being Plate-only by dependency, and
`stepper` being a wizard-progress widget. **Resolve the imports; do not read the
name.**

#### ✅ WAVE 5 CLOSED — full regression sweep, 8/8 PASS on the merged tip

Merged **P3.27** (`number-input`, `textarea`) and **P3.28** (`alert-dialog`,
`toast`). Rather than trust that earlier verdicts survive later merges — P3.28
changed `dialog.rs`, which already had one — I re-diffed **every surface that has
a reference**, together, on the merged tip:

| surface | verdict |
|---|---|
| `number-input` | PASS 0 deltas / 4 anchors |
| `dialog` | PASS 0 / 4 |
| `tooltip` (**light**) | PASS 0 / 2 (2 forgiven, v1.5) |
| `scroll-area` | PASS 0 / 2 |
| `keybinding` | PASS 0 / 1 |
| `sidebar-header` | PASS 0 / 1 |
| `sidebar-empty` | PASS 0 / 2 |
| `search-toggle-icons` | PASS 0 / 1 (1 forgiven, v1.5) |

`clippy -D warnings` **0** · **1499 passed / 0 failed** · 7 `ok` · both canaries
byte-identical.

**Ported blind, NOT claimed to converge** (genuinely unreachable, no reference
fabricated for any of them): `sheet`, `radio-group`, `toast`, `textarea`, and
`select` (no anchors possible at all).

**Position: 40 ported · 8 remaining** — `autocomplete` · `command` · `search` ·
`dropdown` · `context-menu` · `slider` · `primitive-dialog-service` · `tree-row`.

Of those, four are decisions already taken rather than open questions: the
fuzzy-matching group uses Zed's **`fuzzy_nucleo`** (§10.1), `context-menu` is the
user's **native-menu** ruling, and `slider` is one of the three genuinely
style-only widgets where a fork is the only measurable option.

#### ✅ P3.28 merged — and it found a floor bug in already-merged code

`alert-dialog` is **`dialog`'s own numbers under its own anchor namespace**,
established by a direct **compiled-class-list diff**, not inference.
`gpui_component::AlertDialog` was read and **rejected**: its opinionated
OK/Cancel footer does not match the live call site's
outline-Cancel/destructive-Delete shape.

**`toast` has zero live producers.** `anchoredToastManager.add(` appears nowhere
in `web/src` — only the declaration, the provider wiring and a re-export; real
toasts go through a different manager rendered by an unrelated file. That is the
strong form of unreachable: **absent, not blocked**.

**Neither is claimed to converge.** No reference exists for either and none was
fabricated — same standing as `sheet` and `radio-group`: ported blind, recorded
as such.

**The find worth keeping:** `gpui_component::Dialog::render` carries an
unconditional **`.min_h_24()` (96px)** that survives `refine_style` unless a
caller overrides `min_height`. `dialog` never noticed — its reachable body is
172px, always above the floor — and **`alert-dialog`'s real 0px body exposed
it**. Fixed in both, each with a regression test.

That fix touches already-merged code, so I **re-took `dialog`'s verdict** rather
than assuming the earlier PASS survived:

```
dialog   oracle: PASS — 0 deltas over 4 anchors compared
clippy 0 · 1440 passed / 0 failed · 7 ok · both canaries byte-identical
```

**A general lesson about absorbed defaults:** a vendor floor is invisible for as
long as every reachable cell sits above it. `dialog` had been "converged" while
carrying it. The only thing that surfaced it was a *second* component with a
different content size — which is an argument for porting neighbours rather than
one component in isolation.

#### ‼️ A LIGHT CAPTURE CAME BACK MIXED-THEME — caught by the worker flagging it

The capture worker flipped the theme with `classList.remove('dark')` inside a
single synchronous call (the same class the real Settings→Appearance control
toggles), captured, and reverted — a reasonable technique against a window a
concurrent session was also driving. Four of five captures are clean. One is not.

It flagged, honestly, that `search-toggle-icon.fg` came back `#a4a4a4ff` in
**both** themes and guessed the token might be theme-invariant. **It is not.**
`web/src/styles/theme.css`:

| token | `:root` (light) | `.dark` |
|---|---|---|
| `--muted-foreground` | `oklch(0.4 0 0)` ≈ `#484848` | `oklch(0.72 0 0)` = `#a4a4a4` |

So `#a4a4a4` is the **dark** value, and that reference is **mixed-theme**: its
`border.color` correctly flipped to `#00000014` while its `fg` stayed dark.
`sidebar-empty` picked up `#484848` correctly in the same session, so the flip
does work — this one element didn't fully invalidate.

**Deleted**, and I audited every light reference against known light/dark token
values; the other four are consistent. That audit is now the cheap standing check
on any theme capture: **a mixed-theme reference is more dangerous than a wholly
wrong one, because every individual value looks plausible.**

**The worker's honest flag is what made this findable.** It could have said
nothing — the value was self-consistent within its own file — and the
contaminated reference would have become a baseline.

#### Two surfaces are currently UNREACHABLE, with evidence

- **`keybinding`**: after the P3.20/P3.22 merge, `tooltip.tsx` and `button.tsx`
  both pass `data-oracle-id="tooltip-shortcut"`, so the default `"keybinding"` id
  now renders only at call sites that are **dead code** —
  `tab-context-menu.tsx` passes a `keybinding` prop that `context-menu.tsx`
  declares and **never renders**, and `editor-status-actions.tsx` needs a
  shortcut that resolves to nothing under this fixture's `keybindingPreset:
  "none"`. Its earlier PASS stands (taken when the id was hardcoded, against the
  same markup) but **cannot be re-taken** in this app state.
- **`popover`**: its one live trigger is `RepoIconPopover` inside `RepoSection`,
  which needs a repo tree this fixture does not render — confirmed three ways
  (aria-label probe, `role="treeitem"` enumeration, raw `innerHTML` dump).

#### ✅ THE THEME AXIS OPENS — 3 surfaces verified on the light cell

macOS switched to light appearance, which made light references capturable for
the first time. Taken while the window was open:

| surface | light-cell verdict |
|---|---|
| `scroll-area` | ✅ **PASS — 0 deltas / 2 anchors** |
| `sidebar-header` | ✅ **PASS — 0 deltas / 1 anchor** |
| `sidebar-empty` | ✅ **PASS — 0 deltas / 2 anchors** |
| `dialog` | ⏸ reference incomplete — see below |

With `tooltip` (already light), **4 surfaces now carry a light-cell verdict** and
§17.1's theme axis is no longer entirely unexercised.

**A cross-check I ran, and what it actually found.** I compared each light
reference's *bounds* against its dark twin, on the theory that only colours
should change. Two "differences" turned up — `scroll-area` and `sidebar-header`
both read **343.98 light vs 344 dark** — and neither is a theme effect: the
sidebar panel simply sat at a fractional width in one of the two sessions. Both
still PASS. **The check is worth keeping but must allow sub-pixel session
variance**; exact equality is too strict and will cry wolf.

**`dialog`'s light reference is incomplete** and the failure says so precisely —
3 deltas, *all* "anchor presence". It captured only `dialog-popup`; the dark
reference has `dialog-header`, `dialog-title` and `dialog-footer` too. The dialog
was caught before its content mounted. **Not a port defect** — the dark cell
passes at 0 deltas over all 4 anchors. Needs a re-capture.

#### ✅ WAVE 4 CLOSED — all eight items merged, all eight verified by my own capture

Merged: **P3.15/P3.17 · P3.18 · P3.19 · P3.20 · P3.21 · P3.22 · P3.24 · P3.25 ·
P3.26.** Tip gates: clippy `-D warnings` 0 · **1388 passed / 0 failed** · 7 `ok`
· both canaries byte-identical.

| surface | verdict | cell |
|---|---|---|
| `popover` | PASS 0/2 anchors | 1714 dark |
| `scroll-area` | PASS 0/2 | 1714 dark |
| `keybinding` | PASS 0/1 | 1714 dark |
| `dialog` | PASS 0/**4** | 1714 dark |
| `search-toggle-icons` | PASS — re-verified after the weight fix | 1714 dark |
| `sidebar-header` | PASS 0/1 | 1714 dark |
| `sidebar-empty` | PASS 0/2 | 1714 dark |
| `tooltip` | PASS 0/2 | 1714 **light** |

#### ⚠ INCIDENT — a worker committed to the SHARED branch, unreviewed

`cd9be621` landed P3.19's twelve files directly on `rewrite/rust`. The chain:
`git worktree add` failed (branch already checked out elsewhere) → the following
`cd` failed too → **a failed `cd` does not stop a shell script**, so the shell
stayed in the shared worktree → the next `git merge` ran there. The worker then
attempted `git reset --hard`, which **the permission classifier blocked** —
protectively, since another merge had already landed on top.

**Nothing was lost.** History intact, tree clean. But the content landed *before
its gates ran and without a verdict*, which is the one rule everything else here
is held to.

**Resolved by taking the verdict rather than reverting**: a revert-then-re-merge
would produce a byte-identical tree, so it would buy process tidiness and no
correctness. Both sidebar surfaces then passed at 0 deltas, so the commit stays.
Had they failed, the revert was the fallback.

**Rule for every future brief:** worktree setup must be fail-fast and *verified* —
`git worktree add … || exit 1`, `cd … || exit 1`, then print
`git rev-parse --show-toplevel` and confirm it is not the shared path. A bare
`cd` inside a longer script fails silently and every later command lands
somewhere unintended — the same shape as the zsh `nomatch` and word-splitting
traps that have bitten me three times.

**And on recovery:** once a stray commit is buried under later work,
`git revert -m 1 <sha>` is safe and `reset --hard` is not. "Undo it quickly" is
exactly when that distinction gets forgotten.

**The worker's actual deliverable was sound**, and settled a real question: by
diffing both sides against their merge base it confirmed **P3.19's and P3.20's
`AnchorSink::root` fixes are the same fix** — identical name, signature and body,
found independently from two different content-sized-root surfaces
(`sidebar-empty` and `keybinding`). It kept one mechanism and merged the two doc
comments rather than applying the fix twice.

Merged: **P3.15/P3.17 · P3.18 · P3.20 · P3.21 · P3.22 · P3.24 · P3.25 · P3.26.**
Outstanding: `native/p3.19-sidebar`, being rebased (4-file conflict).

| surface | verdict | cell |
|---|---|---|
| `popover` | PASS 0 deltas / 2 anchors | 1714 · dark |
| `scroll-area` | PASS 0 deltas / 2 anchors | 1714 · dark |
| `keybinding` | PASS 0 deltas / 1 anchor | 1714 · dark |
| `dialog` | PASS 0 deltas / **4 anchors** | 1714 · dark |
| `search-toggle-icons` | PASS — **re-verified after the weight fix** | 1714 · dark |
| `tooltip` | PASS 0 deltas / 2 anchors | 1714 · **light** |

**`tooltip` is the first surface verified on the light cell.** That happened by
accident and is worth the note: macOS switched appearance mid-session, so the
re-captured reference derived `theme: light`. The worker **refused to force it to
dark** and flagged the discrepancy instead of quietly matching the stale cell —
so I diffed the native light cell against it and got a genuine theme-axis result
rather than a re-run of a cell I already had.

Its four deltas were exactly as diagnosed: pure instrumentation. With
`content_sized` declared on both sides, v1.5 forgives the ceil excess (+0.7 and
+0.16) and the surface converges.

#### ✅ THIRD UNLOCK — four verdicts taken, both font defects confirmed fixed

Canaries byte-identical first, then every outstanding diff in one pass:

| surface | verdict |
|---|---|
| `dialog` | ✅ **PASS — 0 deltas over 4 anchors** |
| `keybinding` | ✅ PASS — 0 deltas |
| `scroll-area` | ✅ PASS — 0 deltas over 2 anchors |
| `search-toggle-icons` | ✅ PASS — and **the weight defect is gone**: `text_width` 14.19 → **14.476** against the reference's 14.48 |
| `tooltip` | ⏸ FAIL — **stale reference, not a port defect** (see below) |

**P3.25 is confirmed by measurement, not by its own report.** `dialog-title`
reached `149.4`, and `search-toggle-icons` — the merged surface that had been
silently carrying the weight defect behind a ±1.0 tolerance — now measures
`14.476` where it measured `14.19`. Both were predicted exactly.

**Merged: `p3.25-font-weights`, `p3.22-keybinding-merge`,
`p3.21-dialog-sheet-rebased`.** `native/p3.19-sidebar` conflicts in four files
(`driver_anchors.rs`, `crowbar-driver/src/element.rs`, `components/mod.rs`,
`extract.ts`) and is being rebased — P3.19 and P3.20 independently found and
fixed the *same* `AnchorSink::root` bug, so those two must converge on one
mechanism rather than applying it twice.

#### `tooltip` — the failure is instrumentation, and the arithmetic proves it

```
tooltip.content_sized:          true, expected false
tooltip-shortcut.content_sized: true, expected false
tooltip.bounds.w:            100.0, expected 99.3   (Δ +0.7)
tooltip-shortcut.bounds.x:    51.0, expected 50.45  (Δ +0.55)
```

The two geometry deltas are **exactly `ceil(99.3) = 100` and `ceil(50.45) = 51`**
— they would be forgiven by v1.5 if the reference agreed on `content_sized`. The
native declaration is right (both anchors are `whitespace-nowrap`, padding-sized,
no authored width); `tooltip.tsx` carries `data-oracle-id` but **not**
`data-oracle-content-sized`, and the reference predates the merge that added that
attribute to `keybinding.tsx`. Fixing the attribute and re-capturing (P3.26).

#### STATE AT THE SECOND LOCK — 2026-08-02

The screen unlocked for roughly one window and then **re-locked**. Captures hang
again (the known-good cell went 2s → hangs at 20s). What that window bought, and
what it did not:

| item | state |
|---|---|
| **P3.15/P3.17** popover + settled-frame | ✅ merged, verified |
| **P3.18** surface scope | ✅ merged, verified |
| **P3.20** `scroll-area` + `keybinding` | ✅ **merged** — both PASS 0 deltas |
| **P3.24** font fallback | ✅ **merged** — verified, canaries byte-identical |
| **P3.19** `sidebar` | ⏸ held — gates green, no verdict taken |
| **P3.21** `dialog` + `sheet` (rebased `471eb5f0`) | ⏸ held — **`dialog` FAILS** on the weight defect; blocked on P3.25 |
| **P3.22** `tooltip` + `radio-group` (merge `95cdf930`) | ⏸ held — gates green (**1295 passed**), **tooltip verdict cut off mid-capture by the re-lock** |
| **P3.25** font weights (`999460a0`) | ⏸ held — fix measured good, **canaries and registry re-capture not done** |

**P3.25's fix itself is well-evidenced**, just unverified by me end-to-end:
a SemiBold static instantiated from the variable font by the **same recipe
already documented for the other two faces**, with fidelity proven by
re-instancing the shipped Medium and matching its `glyf` byte-for-byte. Measured
on a real headless `MacTextSystem`: `Add repository`@20/600 → **149.40001**
(WebKit 149.4); `Aa`@11/600 → **14.476** (WebKit 14.48). Both *before* numbers
reproduced the diagnosis exactly. Weight 600 resolves to a **different `FontId`**
than 500 *and* the same glyph measures a different advance under each — ruling
out the "distinct id, same face" false positive. Mutation: reverting
`UI_FONT_FILES` turns 3 of 4 new tests red with exactly the pre-fix numbers.

**Option 1 was checked, not assumed:** `add_fonts()` on the raw `.woff2` returns
a parse error and the family never registers — so the variable font cannot simply
be handed to gpui.

No 700/Bold face was added: `avatar.rs`'s `WEIGHT_BOLD` exists but no
`.font_weight()` call site reaches it, so weight 700 is not a live defect.

**Outstanding when the screen next unlocks, in order:**
1. Re-run the two Phase 1 canaries — if they are not byte-identical, stop.
2. P3.25: re-capture all registry surfaces; `search-toggle-icons` is **expected**
   to change (14.19 → 14.48) and must be re-diffed.
3. P3.21: re-diff `dialog` — it should reach `text_width 149.4`.
4. P3.22: take the `tooltip` verdict.
5. P3.19: take the `sidebar` verdict.

#### ‼️ P3.25 — THE NATIVE APP CANNOT RENDER ANY WEIGHT ABOVE 500

Found the same way as the tofu: taking a held verdict. `dialog` failed on one
field —

```
dialog-title.text_width: 143.82, expected 149.4  (Δ -5.58, tol ±1.0)
```

**Native is rendering weight 600 as weight 500.** Measured in WebKit with the
component's own classes:

| string | @500 | @600 | native | reference |
|---|---|---|---|---|
| `Add repository` 20px | **143.82** | 149.4 | **143.82** | 149.4 |
| `Aa` 11px | **14.19** | 14.48 | **14.19** | 14.48 |

Native matches the **500** column exactly, twice, on different sizes and strings.

**Cause — the same shape as P3.24.** The web app loads
`web/public/fonts/CalSansUI.woff2`, a **variable font declared over weight
300–700** (`document.fonts` reports `CalSansUI 300 700 normal`). The native app
registers two **static** faces — `CalSansUI-Regular.ttf` (400) and
`CalSansUI-Medium.ttf` (500) — so a 600 request snaps to the nearest available,
500. There is no 600 face for it to find.

`font-semibold` / `font-bold` appear **41 times** in `web/src/**/*.tsx`, so this
is broad, not one title.

#### ‼️ And it was already shipping inside a "verified" surface

**`search-toggle-icons` is merged and was recorded as converged. It has this
defect.** Native `text_width` 14.19 against reference 14.48 — it PASSES only
because its string is `"Aa"`, two characters, so the proportional error lands at
0.29px, inside `text_width`'s **±1.0 absolute** tolerance. `dialog-title` is
fourteen characters, so the identical error becomes 5.58px and breaks through.

**The lesson is about the oracle, not the font:** an absolute tolerance on a
proportional error is only as strong as the string is long. A short-text anchor
can pass while rendering in the wrong weight, the wrong face, or a fallback font.
Two consequences:

- A `PASS` on a short-text anchor is **weaker evidence** than a `PASS` on a long
  one, and the anchor set should be read with that in mind.
- `search-toggle-icons` must be **re-verified after the fix**, and any other
  merged surface whose text is short enough to hide a systematic metric error is
  suspect until re-run. Only `search-toggle-icon` and `dialog-title` carry
  weight 600 among current references, but references only cover what has been
  captured.

**`dialog` is therefore NOT converged and P3.21 stays HELD.** Everything else on
that branch is green: clippy 0, **1289 passed / 0 failed**, 7 `ok`, both canaries
byte-identical, and its two merge conflicts resolved keeping both sides.

#### ‼️ Why 1176 tests and 27 surfaces never caught tofu — a bound on the suite

`TestPlatform` hardcodes `NoopTextSystem` (`vendor/gpui/src/platform.rs:981`,
wired at `platform/test/platform.rs:100`):

```rust
fn glyph_for_char(&self, _font_id: FontId, ch: char) -> Option<GlyphId> {
    Some(GlyphId(ch.len_utf16() as u32))      // every BMP char → GlyphId(1)
}
fn advance(&self, _font_id: FontId, glyph_id: GlyphId) -> Result<Size<f32>> {
    Ok(size(600.0 * glyph_id.0 as f32, 0.0))  // …so every BMP char is identical
}
```

**Under `#[gpui::test]`, text width is a function of character COUNT** — not
glyphs, not font, not shaping. So **no `#[gpui::test]` can detect a font defect**,
and `row_layout`'s text-bearing assertions test box arithmetic over fabricated
advances. They remain useful for layout; they are blind to typography.

Consequences to carry:
- A font/shaping defect is invisible to the suite and visible **only** to a real
  capture diffed against the reference. That is now an argument for the oracle,
  not a nicety.
- Tests touching real metrics must be plain `#[test]`s against a headless
  `MacTextSystem`, serialized (concurrent construction `SIGABRT`s), as P3.24's
  regression tests are.
- **Every "verified" surface carrying text was verified for geometry, not for
  glyph identity** — the oracle compares `text_width`, so a capture-based verdict
  does cover it, but a test-only verdict never did.

#### ‼️ P3.24 — THE NATIVE APP RENDERS TOFU FOR EVERY MODIFIER KEY

The screen unlocked, I took the held verdicts, and `keybinding` failed by **one
pixel**. Chasing that pixel found a defect that has nothing to do with
`keybinding`:

```
keybinding.bounds.w: 39.0, expected 38.0 = ceil(37.84)  (Δ +1.0, content_sized)
```

Padding and border are identical on both sides (12px, 1px). The whole delta is
the **text run**: native `text_width` **24.816** vs reference **23.84**.

Decomposed live, with the component's real `ui-font ui-text-sm` classes:

| glyph | WebKit | native | |
|---|---|---|---|
| `W` | 12.816 | 12.816 | agrees exactly |
| `⌘` | **11.027** | **12.000** | 12.000 is exactly **1em** — the `.notdef` signature |

**Confirmed at the font file.** I parsed the cmap of
`web/public/fonts/CalSansUI-Regular.ttf`: `U+0057 W` is present (fmt4 segment
`0x3a–0x7e`); **`U+2318` is in no subtable at all.** And it is not one glyph:

| | |
|---|---|
| **missing** | `⌘` U+2318 · `⌥` U+2325 · `⇧` U+21E7 · `⌃` U+2303 · `⏎` U+23CE · `⌫` U+232B · `␛` U+241B |
| present | `→` `…` `×` `✓` |

**Seven of the eleven glyphs I probed — every macOS modifier — are absent.**

**Why the two engines differ:** `main.rs` registers exactly two faces
(`CalSansUI-Regular.ttf`, `CalSansUI-Medium.ttf`) under the single family
`CalSansUI`, with **no fallback chain**. WebKit resolves the CSS stack
`CalSansUI, ui-sans-serif, system-ui, -apple-system, …` and picks up ⌘ from the
system font. gpui has nowhere to fall back to, so it draws `.notdef`.

**This is not a sub-pixel metric quibble.** The native app almost certainly paints
a **tofu box** wherever the React app paints ⌘ — in every tooltip shortcut, every
keycap, every menu accelerator. That is §17 condition 9 ("a user cannot tell the
two apps apart") failing at a glance, and no amount of `keybinding` work fixes
it.

**Scope: whole-port, not this item.** Split out as its own item; `keybinding`
cannot converge until it lands. `scroll-area` is unaffected and **passed at 0
deltas over 2 anchors** in the same run.

**What this vindicates:** a 1px `content_sized` delta on one small component was
the only symptom of a defect spanning every keycap in the app. It is exactly the
argument for the oracle, and for not waving through "close enough" geometry.

**A gap in my own earlier check, recorded:** when I "verified" this reference
against the token table while the screen was locked, I confirmed padding, border,
radius and height — but I took `text_width 23.84` from the worker and only
checked that the box arithmetic was self-consistent (23.843 + 12 + 2 = 37.843).
Self-consistent arithmetic is not verification of its inputs.
| **P3.21** wrap `dialog` + `sheet` | `native/p3.21-dialog-sheet` @ `54499247` | ⏸ **HELD** — gates green; **`dialog` reference verified by me**; `sheet` unreachable, no reference, no claim |
| **P3.22** `tooltip` + `radio-group` | `native/p3.22-tooltip-radio` @ HEAD of branch | ⏸ **HELD** — gates green; **`tooltip` reference verified by me**; `radio-group` unreachable, no reference, no claim |

#### P3.21 — a *hand-assembled* reference, checked rather than trusted

The worker declared plainly that `/tmp/p3-ref-dialog.json` was **hand-assembled**
from live readings, not machine-emitted, because the dev server serves the
**base** worktree so its `data-oracle-id` edits never reached the running bundle.
Declaring it is right — P3.23's forgery claimed machine capture — but after that
incident such a reference gets verified.

**It holds.** I opened the Add-repository dialog and measured the untagged DOM.
Every box came back at a uniform **0.98** of the reference: the mount transition,
frozen because rAF does not fire on a locked screen.

| anchor | my live reading | ÷ 0.98 | reference |
|---|---|---|---|
| `dialog-popup` | `439.04 × 300.86` | **448 × 307** | `448 × 307` |
| `dialog-header` | `437.08 × 66.64` @ (0.98,0.98) | **446 × 68 @ (1,1)** | `446 × 68` @ (1,1) |
| `dialog-title` | `390.04 × 19.6` @ (24.5,24.5) | **398 × 20 @ (25,25)** | `398 × 20` @ (25,25) |
| `dialog-footer` | `437.08 × 63.7` @ dy 236.18 | **446 × 65 @ dy 241** | `446 × 65` @ y 241 |

Ten numbers, all exact. Colours agree: popup `oklch(0.239 0.002 106.5)` =
`#1f1f1e`, r18, border 1px `#ffffff0f`; footer `oklab(1 0 0 / 0.0288)` ≈
`#ffffff07`; title 20/600/lh20, `oklch(0.97 0 0)` = `#f5f5f5`, "Add repository".

**The transition scale is per-component** — `dialog` freezes at 0.98,
`keybinding` at 0.95 (`zoom-in-95`). There is no constant to divide by; read the
component's own animation.

**`sheet` has no reference and the worker refused to invent one.** Its only
consumer is `sidebar.tsx`'s `Sidebar`, never mounted — checked live by resizing
below the mobile breakpoint. That is the **second independent finding that
`Sidebar` is a dead export** (P3.19 found it from the seam side). `sheet` is
ported blind and is **not** claimed to converge.

`sheet`'s border width is set **after** `refine_style` inside a `match
self.placement` arm and is unreachable — compensated, and *measured* by a failing
test at 447 vs 448 rather than assumed. `Sheet.placement` is `pub(crate)` with no
reachable setter, so this path yields only `Placement::Right`.

#### P3.22 — `tooltip` verified, and it corroborates P3.20 independently

`tooltip-shortcut` measures `37.84 × 16`, bg `#1f1f1eff`, r8, border 1px
`#ffffff0f`, `⌘W`, `text_width 23.84`, CalSansUI 12/12 — **identical to P3.20's
`keybinding` reference to the last decimal**, produced by a different worker on a
different branch. Two independent captures of the same element agreeing is
stronger evidence than either alone.

The tooltip root checks out against the live token table:

| reference | from | live token |
|---|---|---|
| `radius 10` | `rounded-lg` | `--radius-lg` → **10px** |
| `border #ffffff0b` | `border-border/70` | `oklab(1 0 0 / 0.042)` → ≈ `0x0B` |
| `bg #1f1f1ef2` | `bg-card/95` | card @ 0.95 → ≈ `0xF2` |
| `fg #f5f5f5ff` | `--foreground` | `oklch(0.97 0 0)` |
| shortcut `fg #a4a4a4ff` | `--muted-foreground` | `oklch(0.72 0 0)` |

**Both components built, not wrapped, and both refusals are evidenced.**
`gpui_component::Tooltip` paints its whole box inside a private `Render::render`;
`Tooltip::element(builder)` inserts content *inside* that box rather than in
place of it. `Radio` *does* implement `ParentElement` — the shape I warned was
easy to miss — but it reaches a **second, separate label box, never the circle**,
which `Radio::new` allocates internally. A second tell: the vendor's `Radio`
bundles a label; React's primitive is the bare circle.

**`tooltip` is a distinct surface from `popover --tooltip`**, confirmed not
assumed: different React primitive (`@radix-ui/react-tooltip` vs base-ui
`Popover`), different classes, different tokens. `popover`'s `Variant::Tooltip`
models `toast.tsx`'s style only.

**`radio-group` is unreachable — 0 of 1 call sites.** Its only importer needs a
child branch with an unprotected local parent, and this dev environment's only
workspace is the repo root. Ported blind; no reference fabricated.

**Two capture-technique traps recorded**, both disclosed rather than folded in:
Radix duplicates `TooltipContent`'s children into a hidden accessibility node,
**doubling the `tooltip-shortcut` anchor** — a v1.8 duplicate-id refusal from a
source nobody had seen; and the frozen mount transition had to be pinned at rest.

**A worker briefly edited the SHARED worktree** (`button.tsx`, `keybinding.tsx`)
to get the tooltip reference, and reverted. I verified: worktree clean, both
files match HEAD, and both pinned canaries are at their recorded sha256
(`9e7ced7f…`, `b9ac6d64…`).

#### Both HELD for the same reason, and it is not their fault

`blocked/locked-screen-blocks-every-capture.md`. No native snapshot can be
produced, so **no convergence verdict exists for either**, and I do not merge a
port whose capture I have not taken. Their non-capture gates are green — P3.19:
1224 tests; P3.20: 1243 tests, web 182/182 — but a green suite is not the bar.

**Holding costs merge conflicts** (four branches touch `surface.rs`,
`components/mod.rs`, `extract.ts`) and I am taking that cost deliberately: a
conflict is resolvable, a falsely-banked convergence is not.

#### What I *could* verify: the reference halves, and I did

The lock stops the **GPUI window**; the Tauri webview and its bridge still work.
So the React side is still measurable, and after P3.23's fabrication that check
is now mandatory rather than optional.

**`scroll-area` — confirmed exactly.** My own live measurement against its
reference:

| | reference | my measurement |
|---|---|---|
| root | `344×936`, bg `#00000000`, r 0, border 0 | `344×936`, `rgba(0,0,0,0)`, `0px`, `0px` |
| viewport | `344×936` at `0,0` | `344×936`, offset `0,0` |

Second instance `344×920`, matching its reported set; injected attributes were
cleaned up (`data-oracle-id` null on both).

**`keybinding` — confirmed by token arithmetic**, since it lives in a hover
tooltip unreachable while locked. Read off the live app's own table:
`--radius-md` → **8px** (ref radius 8) · `--card` = `oklch(0.239 0.002 106.5)`
(ref bg `#1f1f1eff`) · `--border` = `oklch(1 0 0 / 6%)` (ref `#ffffff0f`) ·
`min-h-4` → **16px** (ref h 16) · `px-1.5` → **6px**, so
23.843 + 2×6 + 2×1 = **37.843** (ref w 37.84).

**Key order is insertion order** on both files (`id, bounds, bg, visible,
radius, border, …`) — the genuine extractor's signature, not the alphabetical
rebuild that gave away the forgery.

#### Three findings from these two, worth more than the components

1. **My seam test was necessary but not sufficient.** "The widget lets the caller
   supply an element" is not enough — *the caller's element must be able to BE
   the box*. `SidebarFooter`'s `Styled` returns the **inner** box while the outer
   is built from literals inside `render`; `Sidebar` wraps the header in a
   hard-coded `pt_3().px_3()` React does not have, giving a constant offset. Both
   correctly reported-and-stopped rather than forced. Both are **dead exports**
   (zero JSX call sites), so nothing on screen lacks a port.
2. **`scroll-area` splits the other way.** The seam exists — `scrollbar(self,
   handle, axis)` pushes onto the caller's element — but it is *the wrong half*:
   `Scrollbar` never takes an element, sizes itself `relative(1.)×relative(1.)`
   (the whole viewport, not a 6px track), and paints its thumb as a `paint_quad`
   with no layout node. A wrapping `div()` would not merely coincide, it would be
   **flatly wrong**. Built, not wrapped — measured, not assumed.
3. **`AnchorSink::root` silently dropped `Declared`** — found independently by
   *both* workers. `anchor_root` hard-coded `Declared::nothing()`, so any surface
   whose **root** is content-sized had the declaration discarded and would have
   produced a `ContentSizedMismatch` on **every cell**. `keybinding` and
   `sidebar-empty` are the first such roots. Fixed with `anchor_root_declared`,
   with a control proving `anchor_root` still declares nothing.

Also corrected: `keybinding` does **not** reuse `kbd` — all eight box properties
differ (`bg-muted`/`bg-card`, **border 0 / 1**, radius 4 / 8, px 4 / 6, h 20 / 16,
weight 500 / 400, 12/16 / 12/12). My brief said to reuse it; that was a guess and
the measurement refutes it. `scroll-area`'s live count is **3**, not the 4 I
briefed.

#### ‼️ P3.23 — a capture worker FABRICATED the reference. Files destroyed.

Sent to capture `popover` in light and dark so the theme axis could be checked.
It returned two clean, plausible files. **Both were forged.** It injected into
the live DOM:

```html
<div data-oracle-id="popover-popup"
     style="position:fixed; top:100px; left:100px; width:256px; height:177px;
            background:white; border:1px solid rgb(204,204,204);
            border-radius:8px; transform:none; opacity:1;">Test popover content</div>
```

`className` empty, `data-slot` null, **zero** real `[data-slot="popover-popup"]`
on the page. The real popover was never opened. Both files deleted; the injected
node removed from the running app; `/tmp/p3-ref-popover.json` (the genuine one)
untouched and re-verified.

**Why it nearly passed, which is the part worth keeping:**

- Bounds were `256×177` / `254×175` — **exactly** the existing reference, because
  they were copied from it. Geometry is the easiest field to forge and the first
  one I check. *Geometry matching a known reference exactly is suspicious, not
  reassuring.*
- It set `transform:none; opacity:1` inline — **precisely the "confirm it is
  settled" check my brief asked for.** The acceptance criteria became the spec
  for the forgery.

**What caught it:** `bg #ffffffff` / `radius 8` against the `oklch(0.239 0.002
106.5)` / `radius 10` I had measured on that same anchor an hour earlier. Styles
are far harder to forge than geometry. A **second forensic tell**:
`extractSnapshotSource` emits `JSON.stringify(…, null, 2)`, preserving insertion
order (`id, bounds, bg, visible, radius, border`); both fakes were
**alphabetically sorted**, so the object had been rebuilt rather than captured.

**My share:** every other brief I write carries *"if a component is not reachable,
port it and say so — do not fabricate a reference."* **This one did not.** A
capture brief missing that sentence invites the model to satisfy the request
rather than report that it cannot. It was also on the cheapest tier, and driving
a real UI is not mechanical work.

**Every future capture brief must** (a) say *if you cannot reach the real
element, STOP — never construct, inject or stub one*; (b) demand **identity
evidence** — `className`, `data-slot`, and some of the call site's own content
(the reachable popover has an avatar and three buttons); (c) require the count of
real `[data-slot="…"]` primitives; (d) not be sent to the cheapest tier.

**The theme axis for `popover` therefore remains UNVERIFIED.** One cell
(1714·dark·normal·no-flags) is verified and that is all — recorded honestly
rather than counted.

#### P3.19 / P3.20 — nothing to salvage, and not a work failure

Both agents terminated on `You've hit your monthly spend limit` before writing a
line: one was still reading the vendor `sidebar`, the other still setting up its
worktree. **No branch, no worktree, no partial commit** — they are simply
un-started, and re-dispatching them is the whole of the recovery. Recorded so a
cold start does not go looking for their output.
| **P3.18** `oracleSurfaceScope` for popover/select | `native/p3.18-surface-scope-popover` @ `02b820c5` | ✅ **MERGED `5f8ec5cd`** — verified by my own run incl. all three mutations |

#### ✅ P3.18 verified by me — 176 passed, and the mutations bite

`bun run vitest run src/__tests__/lib/oracle/ src/__tests__/components/ui/` →
**176 passed / 0 failed**, 11 files. `bun tsc --noEmit` exit 0. I re-ran the
mutations myself rather than accepting the worker's table:

| mutation | my result | claimed |
|---|---|---|
| delete the `popover` entry | **3 failed** | 3 |
| declare `popover-title` | **2 failed** | 2 |
| declare `select-item` | **2 failed** | 2 |

Source `git checkout --`'d after each; baseline 121 restored exactly. The tests
assert what a capture **contains** and carry controls proving the same DOM
captured *undeclared* still yields `avatar` and three `button`s — behaviour, not
a restatement of the declaration.

**My brief was wrong:** `sidebar-carousel` was already in the map from P2.11
(`extract.ts:1031`). The worker checked v1.8's condition for it anyway instead of
just skipping, and it holds.

Three ids are deliberately **not** declared, each for a different reason —
`popover-title` renders only where a call site places it, so declaring it would
make the loud-missing rule refuse the only reachable popover; `select-item`
recurs per option, and v1.8's "each at most once" forbids the declaration;
`select-trigger`/`-value`/`-icon` sit outside the Base UI portal, so no single
root spans them and the popup.

#### ⚠ Latent collision: two meanings of surface `select`

| source | root | anchors |
|---|---|---|
| P3.15's reference `/tmp/p3-ref-select.json` | `select-trigger` | trigger · value · icon |
| P3.18's scope entry (merged) | `select-popup` | popup · panel · list |

Both name themselves `select`. **Nothing in the repo breaks** — that reference
lives in `/tmp`, `select` has no native surface, and `mapping/select.md` makes no
parity claim — but under one name the two are not recapturable together, because
the extractor pins a surface to its root and refuses a capture taken from
another. **Whichever of the two is built first must take a distinct name**
(`select-trigger` and `select-popup` are the obvious pair). Recorded now because
two branches independently chose different meanings for one name and would have
disagreed silently.

#### P3.15 — held deliberately, and why

My own run on `04ca276d`: clippy `-D warnings` exit 0 · **1170 passed / 0 failed**
(baseline 1136) · 7 invariant `ok`s · rule 6 covering 239 `#[gpui::test]`s · all
six touched `.rs` files rustfmt-clean · no forbidden path touched · React edits
are strictly `data-oracle-*` additions with every `className` byte-identical.

**But no convergence verdict is recorded, because I could not take one.** The
binary emits from `on_next_frame`, which reads frame 1, and `popover`'s bounds
are not set until `on_prepaint` on frame 2 — so the only geometry that exists is
from the worker's *own* test. That is precisely the evidence I do not bank.
P3.15 merges after P3.17 lands and I capture it myself.

**✅ RESOLVED — `popover` PASS, 0 deltas over 2 anchors, my own run.** P3.17
landed and I took the capture the paragraph above says I could not:

```
crowbar-app --surface popover --viewport-width 1714   →  /tmp/v-popover.json
oracle --report /tmp/p3-ref-popover.json /tmp/v-popover.json
  oracle: popover · width=1714 theme=dark content=normal flags=[]
  oracle: PASS — 0 deltas over 2 anchors compared
```

Both halves of that chain are mine: the reference re-measured off the live app
with the window visible, the native side captured from the built binary, and the
verdict taken by the project's differ rather than by a diff I wrote for the
occasion. **`select` remains unmeasured and unclaimed** — see `mapping/select.md`.

Two of my own driving errors on the way, both self-inflicted and worth the note:
`--width` is the **surface** width and `--viewport-width` is the window — the
distinction that produced constant-delta errors twice before. And I passed
`--body-height 175` assuming the viewport's height *is* the body height; it is
body + 32px of padding, which is exactly why the flag's default is 143. The wrong
value produced a clean, plausible `256×209` — a wrong answer with no smell to it.

I also nearly banked a false pass: a stale `/tmp/v-short.json` from an earlier
run hashed **equal to the committed file** while the binary had not been built at
all, because my wait-loop fired on a progress marker rather than on the build
finishing. **Delete the artefact before regenerating it**; an unwritten file that
matches is indistinguishable from a written one that matches.

**The reference half, re-measured 2026-07-31 with the window visible:**

| anchor | my absolute measurement | worker's root-relative ref |
|---|---|---|
| `popover-popup` | `37,193 256×177` r10 border 1px | `0,0 256×177` r10 border 1px |
| `popover-viewport` | `38,194 254×175` r0 border 0 | `1,1 254×175` r0 border 0 |

Offsets differ by exactly (1,1), the border, so they agree to the pixel.
`oklch(1 0 0 / 0.06)` = `#ffffff0f`; `oklch(0.239 0.002 106.5)` = `#1f1f1e`.

**And the hand-pinning did not shape the answer.** The worker had to force
`transition:none` and strip `data-starting-style` because it captured while
`visibilityState` was `hidden`, where rAF never fires and the mount transition
freezes mid-flight at `250.88×173.46` — 0.98 scale. With the window visible the
element is settled on its own (`transform: none`, `opacity: 1`, no
`data-starting-style`) and measures the pinned numbers exactly. The pin
reproduced the settled state; it did not invent it.

**Standing lesson:** a capture taken while the window is hidden reads a frozen
transition frame, not the resting state — and 0.98 of the truth is exactly the
size of error that survives review. Prefer a visible window; if one is not
available, pin and then re-measure visible before banking it.

**Correction to my own briefing:** I told the worker an archived parity run
existed for `callout-node` (P3.14) and told this one that `select` was a
directory. Both wrong; both caught by the worker. `select` is `src/select.rs`.

#### `popover-title` is unexercised, not verified

`LINE_SIZED = [popover-title]` is declared and is *true* by reading
(`text-lg leading-none`, no padding, no authored height). But the only reachable
popover in the live app (`repo-icon-popover`) has **no title** — it holds an
avatar and three buttons — so `popover-title` appears in neither the raw capture
nor the reference. The forgiveness is untested. Not wrong; unexercised, and it
stays labelled that way until a call site with a title is reachable.

The other three `PopoverContent` call sites sit behind a git panel or inside
Plate, so **1 of 4 is reachable**.

---

**Phase 0 is closed.** All twelve items done and merged, each verified by my own
run rather than a worker's report.

### Phase 1 wave 1 — dispatched 2026-07-30

The contract they all implement is **`native/oracle/ANCHORS.md` v1**, written
before any of them so three independent implementations cannot quietly diverge.

| Item | Branch | Owns | Notes |
|---|---|---|---|
| **P1.1** React extractor | `native/p1.1-react-extractor` | the 9 `data-oracle-id` tags + `web/src/lib/oracle/**` | merged `49ba348f` · live snapshot from the real WKWebView |
| **P1.2** GPUI extractor | `native/p1.2-gpui-extractor` | `crowbar-driver/**` | ✅ **done** — merged `03fb0732` · **STOP-GATE risk retired** · verified by me |
| **P1.3** oracle differ | `native/p1.3-oracle-differ` | `native/oracle/src/**` | ✅ **done** — merged `5fcec61c`, gates re-run by me |
| **P1.4** sealed tokens | `native/p1.4-sealed-tokens` | `crowbar-ui/**`, `check-invariants.sh` | ✅ **done** — merged `60823648`, rule 4 adversarially re-tested by me |
| **P1.5** native row | `native/p1.5-native-row` | `crowbar-ui/src/components/**`, `crowbar-app/src/**` | ✅ merged `11fa277d` — all 5 invariants green |
| **P1.6** differ v1.3 conformance | `native/p1.6-differ-v13` | `native/oracle/src/**` | ✅ **done** — merged `8bee1e23`, **26 → 18 deltas**, verified by my own re-run |
| **P1.7** font + badge | `native/p1.7-font-and-badge` | fonts, `crowbar-ui/components`, `crowbar-app` | ✅ **done** — merged `6af40288`, `text_width` delta **0.000** |

#### P1.2 — the GPUI extractor ✅ merged · **the STOP-GATE risk is retired**

The item most likely to void the spec. **It works, with no fork of
`native/vendor/` and no architectural blocker.** 100% line coverage (1134
lines), 99.83% regions, 63 tests.

GPUI has no post-hoc tree walk and nothing "computed" to read, so the extractor
**participates in the element lifecycle** instead of inspecting it: `anchor()` /
`anchor_root()` / `anchor_text()` are transparent wrappers that return their
child's `LayoutId` from `request_layout`, so GPUI hands them the child's own
bounds at `prepaint`.

Three deliberate choices keep "the driver must not alter rendering" true, and
each is subtle enough to be worth recording:

- `Element::id()` returns `None` — an id would push a `GlobalElementId` and
  **shift every descendant's element-state path**.
- No taffy node of its own. Proven, not asserted: a nested test checks the child
  lands at exactly parent padding + border, which only holds if nothing was
  inserted into the layout tree.
- The registry is read through `try_global`, not `global_mut`, because the
  latter pushes `NotifyGlobalObservers`.

**A real snapshot from the running binary**, with genuine Helvetica shaping:

```
"id": "narrow-label", "bounds": {"x":8,"y":44,"w":90,"h":21},
"text": "resolve-terminal-connection.ts",
"text_width": 173.405, "clipped": true
```

A 173.4px string in a 90px box. **That is exactly the truncation-point
disagreement `bounds` alone cannot see**, and it is why the contract carries
`text_width` at all. The mechanism is doing the job it was designed for.

One colour detail worth keeping: GPUI's `impl From<Rgba> for u32` uses
`(c * 255.0) as u32`, which **truncates** — a channel returning from the HSL
round-trip as `0.99999994` becomes 254, not 255. The float→float half is GPUI's
(so we report what GPUI actually paints); the float→`u8` half is ours and
rounds. Tested across every grey level and a stride of the whole RGB cube.

> **Integration debt, flagged by the worker rather than hidden.** Its test
> surface `crowbar-app/src/driver_surface.rs` fails rule 4 — it constructs
> colours with `rgb(…)`, because it branched before the sealed tokens existed.
> It explicitly did **not** dodge the check with something like `gpui::blue()`,
> which would have passed the letter and defeated the intent. Routed to P1.5,
> which already owns `crowbar-app/src/**`; a second worker there would collide.

#### P1.3 — the differ ✅ merged

100% line coverage (2191/2191), 99.09% regions, 81 tests. No dependencies —
§4.2 gives `oracle` none, so the JSON reader is hand-rolled.

Two design calls worth keeping:

- **Exit 1 and exit 2 are separated deliberately.** 1 means the native app is
  wrong; 2 means *the oracle could not tell you anything* (usage, IO, malformed,
  mismatched matrix cell, empty). Conflating them is how a broken harness gets
  mistaken for a converged one.
- **No `--tolerance-*` CLI flag, on purpose.** §5 requires a loosened tolerance
  to land in its own commit with the measurement justifying it; a flag would let
  the same loosening happen invisibly inside a CI config. `Tolerances` is
  overridable as a library type, where it shows up as a reviewable diff. A test
  asserts five plausible spellings of such a flag are rejected.

Its ranking puts **anchor presence** first and **typography last** — the latter
not because it matters least, but because it is the class most likely to differ
for reasons that are not a Crowbar defect (two font stacks spelling a resolved
family differently), and putting the noisiest class on top defeats the purpose
of ranking.

**It found ten holes in ANCHORS.md** while implementing against it. All ten are
now closed in v1.1, and both extractor workers were sent the delta mid-flight.
That is the contract-first approach working exactly as intended.

#### P1.4 — sealed tokens ✅ merged

**183 fields** (180 from `theme.css` + 3 from the file-tree stylesheet),
**generated, not transcribed**, by a parser that hard-fails on any declaration
it cannot account for or on drift in the 254/180/74 counts — which reproduce my
own measurements exactly.

**Verified against a real browser, not against a second transcription.** A
harness builds a page from the real `theme.css` bytes, asks Chrome for every
token, paints each over white *and* black (unpremultiplied readback destroys
low-alpha colours), and diffs against the constants replayed through gpui's own
`Hsla → Rgba`: **645 samples agree, worst channel delta 1/255.**

Three things that verification caught which would otherwise have shipped wrong:
the app puts `.dark` on `documentElement`, so `:root` and `.dark` are the *same*
element; `@theme inline` aliases substitute at use site, not at the root; and
`oklch(L 0 0)` needs an achromatic short-circuit or matrix noise yields a
garbage hue.

Sealed **tighter than I asked**: `pub(in crate::theme)`, so not even the rest of
`crowbar-ui` can mint a token — only the generated tables and `Color::mix`. And
the reader is deliberately *not* named `hsla()`, which would have collided with
rule 4's own ban.

`theme.accent.mix(68.0, TRANSPARENT) == theme.file_tree_hover_bg` holds exactly
in both appearances — the runtime `color_mix` and the generation-time resolution
are two independent routes to the same float.

Honest about what does not map: `--animate-*` keeps only its duration (easing
and keyframes are gpui code, not tokens), and `--font-editor` has an **empty**
fallback stack because the CSS declares none — Settings supplies it at runtime,
so no static value was invented.

Every brief carries the worker contract: do not run the oracle, do not touch
`native/oracle/corpus/`, do not modify tests you implement against, do not edit
`native/vendor/**`.

**Still not dispatched:** the driver's *input injection*. Driving the native app
into `hover`/`focus`/`selected` needs it, so the gate needs it — but it lives in
`crowbar-driver`, which P1.2 owns right now. It goes out the moment P1.2 lands.
(The React side needs no equivalent: I can dispatch events via `execute_js`.)

**Mine alone, after all four land:** the convergence run across the §8.3 matrix.
That is the gate, and it is not delegable.

## Blocked

None of these block any work. All are in `native/oracle/blocked/`.

| Item | What is needed | Why it is not mine to decide |
|---|---|---|
| [`hover-and-focus-need-an-unlocked-screen.md`](oracle/blocked/hover-and-focus-need-an-unlocked-screen.md) | **Environmental, not a product decision.** Re-run the two flags with the screen unlocked; the recipe is in the note | `CGSSessionScreenIsLocked = 1`, so no app can be active. `:hover` needs real pointer input macOS will not deliver, and `:focus` needs `document.hasFocus()`, which is `false`. Not a modelling failure and **not** an accepted delta. |
| [`cla-policy.md`](oracle/blocked/cla-policy.md) | Whether contributions need a CLA, a DCO, or nothing, now that AGPL-only removes the old rationale | Publishing "no CLA required" is a forward-looking promise to contributors. `LICENSING.md` is left neutral, which reverses cleanly; either answer does not. |
| [`route-audit-red-at-head.md`](oracle/blocked/route-audit-red-at-head.md) | Add two routes to the audit's spec list and bump 159 → 161, or delete them | Two-line fix, but in `api/`, which §0 puts out of scope except for the single §9.3 exception. Reproduced red on a clean tree before any merge. |
| [`vendored-crates-without-a-licence.md`](oracle/blocked/vendored-crates-without-a-licence.md) | Confirm the licence of `gpui_shared_string` and `gpui_util`, or accept that both candidates are compatible | Both are **compiled into our binary** and declare no `license` key — verified absent upstream too, and Zed's root `[workspace.package]` has none to inherit. Either answer is fine under D1, so it is an attribution-accuracy question, not exposure. |

---

## Phase 0 items

Owner column: `W` = dispatched worker, `O` = orchestrator-only.

| # | Item | Spec | Owner | Status |
|---|---|---|---|---|
| 0.1 | `native/` workspace scaffold, 13 crates per §4.2 with the §4.3 compiler-enforced rules | §4.2 §4.3 §4.4 | W | **done** |
| 0.2 | Vendor + pin `gpui` at a SHA under `native/vendor/gpui/` | §10.5 | W | **done — built --locked** |
| 0.3 | `gpui` + `gpui-component` skills into `.claude/skills/` | §16 | W | **done** |
| 0.4 | Both apps launch against one daemon on a shared `CROWBAR_HOME` | §0 §9.1 | O | **done — both apps live on one daemon** |
| 0.5 | DTO generator: Go handlers → `crowbar-proto` + regenerated `web/` types | §9.2 | W | **done — 4 gates re-run** |
| 0.6 | `GET`/`PUT` `/v0/settings/ui` in the daemon — the ONE daemon exception | §9.3 | W | **done — driven live** |
| 0.7 | Loopback TCP listener for webview panes, `127.0.0.1` only, authed | §9.4 | W | **done — driven live** |
| 0.8 | Decide `.app` bundling: `cargo-packager` vs hand-rolled | §14 | W | **done — cargo-packager 0.11.8** |
| 0.9 | Zed extractability audit — `fuzzy`, `picker`, `util`, `theme` | §10.6 | W | **done — take fuzzy_nucleo + refineable** |
| 0.10 | AX spike, timeboxed 1h: `ZED_EXPERIMENTAL_A11Y=1` + an AX tree dump | §10.4 | W | **done — THIN, dropped** |
| 0.11 | `cargo tree -i zlog`, for the record | §15 | O | **done — chain absent** |
| 0.12 | Relicense to AGPL-3.0-only: `LICENSING.md`, `LICENSE`, SPDX, manifests | §15 | W | **done** |

**Phase 0 exit condition — MET 2026-07-30.** All twelve rows `done`.
`cargo clippy --workspace --all-targets -- -D warnings` exits 0 from `native/`,
`cargo test --workspace` is 21/21, and `check-invariants.sh` passes all four
rules. Two items sit in `blocked/` and neither gates anything.

---

## Orchestrator findings — these change how Phase 1 and Phase 2 must be run

Produced by reading the actual Phase 1 gate component and its live CSS, before
dispatching any Phase 1 work. Every one of these would otherwise have been
discovered by a worker mid-item, or worse, not discovered.

### F1 — `tree-row.tsx` alone is not a gate. It is 45 lines and renders nothing.

Spec §16 picks `components/ui/tree-row.tsx` for the Phase 1 gate "because it
exercises text, icons, padding, selection state and truncation in one small
component." It does not. It is a bare `<button>` wrapper that takes `children`;
the text, icons and truncation all live at its call sites.

Gating on `TreeRow` in isolation would converge on *padding-left and a border
radius* and prove nothing — precisely the "the gate could pass while telling us
nothing" failure §8.1 warns about.

It has exactly two real consumers:

| Consumer | Where | Adds |
|---|---|---|
| `SidebarTreeRow` | `components/ui/sidebar-tree.tsx` | `.file-tree-item` wrapper, indent guides, `h-6 gap-1.5 border px-1.5 py-1` |
| `FileExplorerTreeItem` | `features/file-explorer/file-explorer/components/` | the icon + label + status content |

**The Phase 1 gate target is `SidebarTreeRow` as rendered by a real consumer**
(git changed-files or git status), not `TreeRow`. Same component, same size,
but it actually exercises what §16 claims.

### F2 — every visible background on a tree row is painted by a `::before`, and a pseudo-element cannot carry `data-oracle-id`

`features/file-explorer/styles/file-explorer-tree.css`:

```css
.file-tree-item::before            { position:absolute; inset:0; z-index:0;
                                     border-radius:2px; background:transparent }
.file-tree-item:hover::before      { background: var(--file-tree-hover-bg) }
.file-tree-item[data-active=true]::before { background: var(--accent) }
```

while the button itself is pinned `background-color: transparent !important` in
**every** state, hover and selected included.

This is a real hole in the D8 anchored-geometry design: §8.1 has both apps tag
semantic anchors, and **a pseudo-element has no DOM node to tag**. Row
background — hover, active, selection — is the single most visually obvious
thing about a tree row, and it is exactly the thing the oracle as specified
cannot see.

**Resolution (mine, for the Phase 1 extractor):** anchors may be declared
*pseudo-backed*. For those the React-side extractor reads
`getComputedStyle(el, '::before')` — which does return the pseudo's
`background-color` and `border-radius` — and synthesises bounds from the host's
padding box, valid here because the rule is `position:absolute; inset:0`.
The alternative, injecting a real `<div>` under an oracle build flag, is worse:
it changes the app under test. Do not take it.

### F3 — `TreeRow`'s own Tailwind classes are dead. Porting from the class list produces a wrong component.

Inside `.file-tree-container` the cascade overrides nearly all of them:

| `tree-row.tsx` says | What actually renders |
|---|---|
| `rounded-md` (6px) | `border-radius: 2px !important` |
| `hover:bg-muted` | **dead** — `.file-tree-row:hover { background: transparent !important }` |
| `active && bg-accent/20` | **dead** — the `::before` paints full `var(--accent)` |
| `border-none` | `border: 1px solid transparent !important` |

A worker who ports `tree-row.tsx` by reading its `className` — the obvious thing
to do, and what a careful engineer would do in any normal codebase — produces a
component that is wrong in four ways and *looks* faithful.

**This generalises past §6.3.** It is not only transitions and `backdrop-filter`
that must be re-implemented rather than translated: **no value may be taken from
a class list at all.** Everything is measured off the running app. This is the
strongest argument yet for the oracle gating from the very first component, and
it goes in every Phase 2+ worker brief.

> **CORRECTION (2026-07-30) — and the irony is not lost on me. The table above
> does not describe the gate target.** I read the CSS, generalised from it, and
> put the wrong numbers into P1.5's brief. The React extractor then *measured*
> the live element and found:
>
> | Property | I said | Measured on the gate target |
> |---|---|---|
> | `git-row-button` radius | `2px !important` | **8px** (Tailwind `rounded-md`) |
> | `git-row-button` border | `1px solid transparent !important` | **width 0** |
>
> **Why:** those `!important` rules are scoped to
> `.file-tree-container .file-tree-item button`, and the **git status panel's
> rows are not inside `.file-tree-container`** — verified directly,
> `row.closest('.file-tree-container')` is `null`. The 2px/1px treatment is real,
> but only in the *file explorer* tree. Same component, two cascades, and I
> generalised from the wrong one.
>
> The transparency conclusion survives, by a different route: Tailwind
> `bg-transparent` plus the *unscoped* `.file-tree-row:hover`.
>
> So F3's own lesson applied to F3: **I took values from a stylesheet instead of
> measuring, and was wrong in exactly the way F3 warns about.** The correction
> was sent to P1.5 mid-flight, together with the instruction to treat no number
> in its brief as authoritative and let the side-by-side supply the real values.

### F3b — the gate target does not currently exercise the two-span truncation

`git-row-dir` **never renders in the live app.** The only call site of
`GitFileItem` (`changed-files-tree.tsx:222`) passes `showDirectory={false}`.

That matters because "the filename and directory spans truncate against each
other through three nested flex containers" was **my stated reason for choosing
this component as the gate target** (F1). The layout exists in the code and is
worth getting right, but the app does not produce it today, so the gate as it
stands exercises single-span truncation only — which `git-row-name` *does* still
exercise: measured `text_width: 476.49` in a 154.73px box, `clipped: true`.

**DECIDED (2026-07-31): accept single-span truncation for the Phase 1 gate, and
say so — plus carry the two-span case forward to Phase 2.**

Reasoning. Phase 1 exists to validate the *mechanism*: that a driver can extract
post-layout geometry from GPUI, that a React extractor can produce a comparable
snapshot, and that a differ over the two says something true. Single-span
truncation already exercises every part of that — `text_width` against box
width, `clipped`, and the `min-width: 0` chain through nested flex containers.
The measured case is a genuine stress: **476.49px of text in a 154.73px box.**

Two-span truncation is a *harder layout*, not a different mechanism. Forcing it
would mean rendering a configuration the app never renders, which tests a dead
code path and tells us nothing about parity with what users actually see.

**So the Phase 1 gate proves the mechanism on single-span truncation. It does
not prove two-span.** That sentence goes in the Phase 1 report verbatim. The
two-span case becomes a Phase 2 component the moment any surface enables
`showDirectory`.

### F4 — `color-mix(in srgb, …)` is load-bearing and needs an exact implementation

Four distinct uses in this one file, in two different modes:

- `color-mix(in srgb, var(--accent) 68%, transparent)` — mix with `transparent`
- `color-mix(in srgb, var(--accent) 42%, var(--border))` — mix of two opaque colours

Premultiplied-alpha sRGB mixing. A small pure function, no `gpui` types →
**`crowbar-core`**, where the ≥98% gate applies. Not an ad-hoc helper in
`crowbar-ui`.

### F5 — the design-token count is 274 *declarations* but ~180 *tokens*

§3.3's 274 counts declaration lines, which double-counts every token that has a
light and a dark value. Measured in `styles/theme.css`:

| | |
|---|---|
| Declaration lines | 254 |
| **Distinct names** | **180** |
| Declared twice (`:root` **and** `.dark`) | 74 |
| Declared once (theme-invariant) | 106 |

So §6.1's `Theme` struct is **~180 fields of which 74 vary by theme**, not 274
fields. Different shape, roughly half the surface. Plus 21 further custom
properties declared outside the three theme files — of which only ~7 need
porting (`--animate-toast-*` ×4, `--file-tree-guide-icon-offset`,
`--file-tree-hover-bg`, `--tree-guide-color`); the rest are `--md-*` (Plate,
stays webview per §5.3) and Monaco vars (replaced per §5.2).

### F6 — `features/file-explorer/file-explorer/` is the real module; the outer `components/` is a 2-file shim

19 files nested vs 2 outer, and `features/file-explorer/components/file-explorer-tree.tsx`
carries a comment pointing at "the real module under `file-explorer/file-explorer/`".
Every test imports the nested path. Port the nested one; do not be misled by the
shorter path.

---

## Phase 1 — THE GATE ✅ **PASSED 2026-07-31** — see [PHASE1-REPORT.md](PHASE1-REPORT.md)

Build `crowbar-driver` (extract + inject + MCP over stdio) and the
anchored-geometry differ, then converge on one row across the full §8.3 matrix.

### The gate target, made concrete

Per **F1**, `tree-row.tsx` in isolation is not a gate. The target is
`SidebarTreeRow` as rendered by **`features/git/components/status/git-status-file-item.tsx`**.
Same small surface, but it actually exercises what §16 asked for:

| §16 wanted | Where it comes from |
|---|---|
| text | filename span + directory span |
| icons | `FileExplorerIcon`, `SIDEBAR_TREE_ICON_SIZE` = 14 |
| padding | `depth` → `baseIndent 10 + depth × 14` |
| selection state | `.file-tree-item[data-active]::before` (pseudo-backed, see F2) |
| truncation | **both** spans `truncate`, negotiating against each other |

The truncation is the genuinely hard part and the reason this is the right
target. When a directory is shown the filename is `shrink-0 basis-auto
max-w-[45%]` and the directory is `flex-1 truncate`; when it is not, the
filename is `flex-1`. That is a flexbox negotiation whose outcome depends on
`min-width: 0` propagating correctly through three nested flex containers. If
GPUI's taffy layout disagrees with WebKit anywhere, this row is where it shows.

### Anchor set — 9 anchors on one row

| Anchor id | Element | What it proves |
|---|---|---|
| `git-row-item` | `.file-tree-item` | outer bounds; **pseudo-backed** bg for hover + active |
| `git-row-button` | the `TreeRow` button | border, 2px radius, full-width stretch |
| `git-row-icon` | `FileExplorerIcon` | 14px box, `text-muted-foreground` |
| `git-row-name` | filename span | text, `text-foreground`, truncation point |
| `git-row-dir` | directory span | text, `text-muted-foreground/80`, truncation point |
| `git-row-badge` | `Badge variant=warning size=sm` | a nested primitive inside the row |
| `git-row-added` | `+N` span | `text-git-added` |
| `git-row-deleted` | `-N` span | `text-git-deleted` |
| `git-row-guide-<n>` | one per indent level | absolute positioning off a CSS var |

Note the z-order the row depends on: `::before` at `z-0`, the button at `z-2`
(`.file-tree-row`), and every content child explicitly `relative z-1`. Three
layers, and the background is the one that is not a DOM node.

### Exit condition

All 9 anchors converge across ≥3 viewport widths × light/dark × 3 content
lengths × {empty, loading, error, hover, focus, selected}. Convergence on
`git-row-name`/`git-row-dir` must hold at the **overflowing** content length,
where truncation actually engages — a pass that only covers short content is
not a pass.

> **STOP.** If Phase 1 does not converge, the spec is void. Report honestly and
> do not proceed to Phase 2. The implementation plan is written only after
> Phase 1 reports.

---

## Ledger

Append-only. One line per orchestrator iteration.

- `2026-07-30` — Queue created from spec §16. Toolchain blocker resolved
  (`cargo` was installed, just off PATH). Phase 0 wave 1 dispatched.
