# Slice-based porting — the method change, and Slice 0

**Date:** 2026-08-04
**Status:** approved by the user, ready for an implementation plan
**Amends:** [`2026-07-30-rust-native-desktop-port-design.md`](2026-07-30-rust-native-desktop-port-design.md)
§11 (execution loop) and §16 (phases). Everything else in that spec stands.

---

## 1. Why the method changes

The port's own queue states the finding, on 2026-08-04, from the user holding the
running Rust shell beside the real app:

> **Component parity is NOT app parity.** Ten `components/layout` surfaces hold a
> PASS verdict, and the assembled app is a bare text list.

Those verdicts are not false. They measure much less than the count implies, for
two reasons that are structural rather than accidental:

1. **Every capture is rooted *at* the component.** Window chrome, the spacing
   *between* components, and composition are therefore unmeasured by
   construction — not missed, but unreachable.
2. **The oracle measures whether a component *can* render a cell. Nothing
   measures whether the app ever *does*.** `project-home-row` holds PASS 0/5 in
   the `selected` cell while the shell renders it inactive, because nothing
   models which row is active. Both facts are true at once. The state is an
   *input* the harness supplies (`--flags selected`), so the cell always exists
   and the app that never produces it is out of frame.

There is a third reason, about throughput rather than measurement: **verdicts are
taken one cell at a time by the orchestrator.** Worker count does not move that
number. The bottleneck is the review loop, and adding a lane does not widen it.

The consequence is visible in the shape of the tree: **45,295 lines in
`crowbar-app`, all of it a capture harness**, against **13 lines in
`crowbar-state`, all of it doc comment.**

## 2. Measured baseline (2026-08-04)

| crate | lines | state |
|---|---|---|
| `crowbar-app` | 45,295 | **entirely a capture harness** — `cargo run -p crowbar-app` opens a matrix cell |
| `crowbar-ui` | 44,590 | 69 component files, individually parity-verified |
| `crowbar-core` | 11,633 | `git/` `keymap/` `settings/` `workspace/` `file_tree/` `review/` — **69.3% net** of ~3,169 TS lines |
| `crowbar-driver` | 4,101 | extractor + injector + MCP, feature-gated |
| `crowbar-proto` | 1,751 | ✅ done |
| `crowbar-diff` | 1,373 | diff algebra only, no view |
| `crowbar-client` | 696 | ✅ done — unix socket HTTP + WS, reconnect, backoff |
| `crowbar-platform` | 44 | scaffold, **zero `unsafe`** since P3.40 |
| `crowbar-state` | **13** | **all doc comment, zero code** |
| `crowbar-webview` | 11 | stub |
| `crowbar-terminal` | 12 | stub |
| `crowbar-editor` | 9 | stub |

Tests: ~2,524 passing. Line coverage on the §12 crates: 99.64% over 4,978 lines.
Both numbers are honest about what exists and silent about what is missing.

**So: the pipe to the daemon works, the bricks are cut and measured, and there is
no building.** Everything remaining is composition and state — the half the old
method never touched.

## 3. The new method

### 3.1 The unit of work is a slice, not a surface

A slice is done when **it runs in the real `crowbar-app` binary, against the real
dev daemon, and the user has seen it beside Crowbar-React and accepted it.**

Not "0 deltas over N anchors". The per-component oracle stops being a gate.

### 3.2 The oracle becomes a frozen regression net

- The corpus stays **append-only** (§8.4) and runs in CI. It catches regressions
  in the 69 components already verified.
- **No new per-component verdicts are taken.** That lane is closed.
- The corpus is **not** re-rooted at the app level. Doing so needs a
  composition-level anchor map that does not exist, and the cheap instrument is
  better: see 3.3.

### 3.3 The review instrument is a side-by-side screenshot pair

Both apps, same window size, same `CROWBAR_HOME`, same data, two PNGs. The
user's eye does the diffing. This is what revealed the §17.9 gap in one glance
after 43 verdicts could not.

If that loop proves too slow in practice, re-rooting the extractor above the
components becomes a real item. Not before — that is the same over-investment
that produced a 45k-line harness and no app.

### 3.4 Parallelism moves inside the slice

The application is now a serial spine, so fan-out comes from:

- **The Tier A lane** — one background worker on the remaining ~31% of
  `crowbar-core`. Independent of every slice, gated by ported tests, not by the
  oracle. **Runs continuously.**

  > **Corrected 2026-08-04, before the first dispatch.** An earlier draft of this
  > line named "§2 diff algebra and §7 review threads" as the untouched areas,
  > copied from `QUEUE.md`'s tier table. **Both are wrong.** Review threads
  > merged at P3.78 (`crowbar-core/src/review/`, 663 Rust lines at 100.00%), and
  > `tier-a-denominator.md` §2 records that standalone diff algebra is **0 files,
  > 0 lines** — the daemon does the diffing, `crowbar-proto` already carries the
  > shapes, and what looks like diff algebra in the React app (windowing, search,
  > placeholder sizing) is scoped to `crowbar-diff`, where P3.79 already landed
  > it. Dispatching against that line would have sent a worker to port something
  > already ported and something that does not exist.

  The real remainder, from `tier-a-denominator.md`'s per-area table measured
  against the five merged items:

  | area | Tier A lines | ported | remaining |
  |---|---|---|---|
  | Git model incl. the `review-code-view.tsx` embedded region | ~609 | 241 | **~368** |
  | File-tree model | ~718 | 436 | **~282** |
  | Settings schema | 629 | 554 | **~75** |
  | Theme tokens — `resolve-css-color.ts` colour math | ~130 | 0 | **~130** |
  | Keymap resolution | 516 | 516 | ✅ 0 |
  | Workspace scoping | 261 | 261 | ✅ 0 |
  | Review threads | ~306 | P3.78 | ✅ 0 |
  | Diff algebra (standalone) | **0** | — | — |

  Report completion in **one unit throughout**. The retracted "Tier A ≈80%" was
  a ratio of Rust lines to TypeScript lines.
- **Within a slice** — one worker on state/wiring, one on view composition,
  against an interface agreed before either starts.

The user's review is the serialisation point, so it is batched: **one review
session per slice**, not per commit.

### 3.5 Definition of done, per slice

Replaces §11.6 for slice items. All of:

1. `cargo build --workspace` · `cargo clippy --workspace --all-targets -D warnings`
   · `cargo test --workspace` · `./scripts/check-invariants.sh` — all clean, run
   by the orchestrator, not reported by the worker.
2. The slice is reachable from `cargo run -p crowbar-app` against a live daemon.
3. A side-by-side screenshot pair exists.
4. The user has accepted it.
5. **A commit SHA, named.** The commit is a numbered deliverable of the item, not
   an afterthought.

## 4. Phase order

Amends §16. Phases 4 and 5 of the original spec are dissolved into the slices —
state and interaction are built per slice, on demand, never as a batch.

| # | slice | contains |
|---|---|---|
| **0** | **Foundation** | §5 below. No feature code. |
| 1a | Sidebar | Composed with real daemon data **and real selection state**. Where `crowbar-state` gets built — only the stores this slice needs. |
| 1b | Tabs | Tab bar, tab groups. |
| 1c | Panes | Split, resize, focus. Hardest of the three; where `resizable` is first used for real. |
| 2 | Settings | Menu, wired. |
| 3 | Terminal | Terminals and agent chats. Placed here because the daemon already owns the VT screen model. |
| 4 | Editor | See the open question in §7. |
| 5 | Diff viewer | `crowbar-diff` has the algebra; the view is unbuilt. |
| 6 | Markdown editor | The Obsidian/Notion-like surface. |

## 5. Slice 0 — Foundation

**Slice 0 creates no feature code.** It builds the frame every later slice drops
into. Nothing new goes into `surfaces/`; the sidebar, tabs and panes wait for
slices 1a–1c.

### 5.1 Move the harness behind `--features driver`

`crowbar-app/src/{surface,row_surface,surfaces,row_layout}` become driver-only.

**Acceptance:** `cargo run -p crowbar-app` no longer opens a matrix cell.
`cargo run -p crowbar-app --features driver` still runs every existing capture,
and the corpus still passes.

The app cannot grow while its `main` is a test rig. This is the whole point of
the slice.

### 5.2 `crowbar-sidecar` — daemon management, shared by both binaries

Lift `desktop/src-tauri/src/sidecar/` (1,042 lines: `mod.rs` 740,
`supervisor.rs` 302) into `native/crates/crowbar-sidecar/`. It already carries
everything needed: socket-path resolution mirroring the Go daemon's order,
daemon-log rotation, startup health probe, a watchdog with wedge detection, a
per-window respawn cap against crash loops, and SIGQUIT→SIGKILL shutdown.

**No external dependency is taken.** The logic is Crowbar-specific — `CROWBAR_HOME`
resolution, the macOS 104-byte `sun_path` cap that forces overridden homes onto a
temp-dir socket, the fd-limit workaround, the daemon's own stale-socket reclaim.
A generic process supervisor would replace none of it.

**One addition:** *adopt-if-healthy*. Probe the socket first; spawn only if
nothing answers. Both apps must be able to run simultaneously against one
`CROWBAR_HOME` — that is what makes side-by-side review possible, and it is the
double-spawn trap QUEUE item 0.4 documents.

**Acceptance:** `crowbar-app` starts a daemon when none is running, adopts one
when Crowbar-React already owns it, and both apps serve the same workspace list
concurrently. `desktop/` consumes the same crate — one implementation, not two.

### 5.3 Split `crowbar-ui/src/components/`

Today: one flat directory of 69 files — `mod.rs` plus **68 components** — mixing
generic primitives (`button.rs`, `checkbox.rs`, `slider.rs`) with Crowbar-specific
layout (`sidebar_tree.rs`, `project_home_row.rs`, `repo_icon_popover.rs`).

```
crowbar-ui/src/
  primitives/   button, checkbox, input, select, tabs, dialog, …
  surfaces/     sidebar/, workspace/, repo/, rows/, …
```

The split is roughly 38/30, but the exact classification is the item's own work
and a few are genuinely arguable — `row_base` (shared base, Crowbar-specific),
`nav_stack`, `search`, and `fps_overlay` (a dev instrument, arguably neither).
Each borderline call is recorded with its reason rather than decided silently.

**This is a `git mv` of existing files and their `use` paths. No logic changes,
no new files.** Doing it now costs an afternoon; doing it after six slices have
piled into the flat directory costs a week.

> On naming: Rust has no `features/` convention, and the word is taken — it means
> Cargo features (`crowbar-app`'s `[features] driver`). A `src/features/`
> directory beside a `[features]` table reads as `#[cfg(feature = …)]`. The
> idiomatic feature boundary is a module (`mod git`, with `pub(crate)` privacy the
> compiler enforces) escalating to a crate. `crowbar-core/src/` already follows
> this: `git/`, `keymap/`, `settings/`, `workspace/`, `file_tree/`, `review/` —
> near 1:1 with `web/src/features/`.

**Acceptance:** the tree matches the shape above, all four gates clean, and the
corpus still passes unchanged — surface identity in `crowbar-app` does not derive
from `crowbar-ui` module paths, so a green corpus confirms the move was inert.

### 5.4 Real window chrome

The React app's chrome, in `crowbar-platform` — which the crate's own doc comment
already names as its destination ("macOS vibrancy, `NSWindow` manipulation …
ported from `desktop/src-tauri`").

What it must reproduce, from `desktop/src-tauri/src/lib.rs`:

- `NSVisualEffectMaterial::HudWindow` behind a transparent window, with
  `NSVisualEffectState::FollowsWindowActiveState`.
- The appearance pin — the `NSVisualEffectView` (tag `91376254`) is forced to a
  fixed appearance so the frost renders per-theme, main-thread only.
- Overlay titlebar, rounded window edges, theme applied at app level.

**This is the one part of Slice 0 that is not cheap, and it is scheduled first.**

#### The spike, run before the rest of Slice 0 is scheduled

`window_vibrancy::apply_vibrancy` needs a raw `NSWindow` handle. Whether GPUI
hands one out cleanly is **unverified**. Two paths, and the spike picks one:

- **A.** GPUI exposes a `raw-window-handle` — take `window-vibrancy` as a dep.
- **B.** It does not — insert the `NSVisualEffectView` directly with `objc2`,
  which is `crowbar-platform`'s declared dependency contract anyway (§4.2:
  "`objc2`, macOS only. Nothing from the rest of the workspace").

The spike is timeboxed and reports which path, with evidence. If neither works,
that is a finding that reshapes Slice 0 and is worth knowing on day one rather
than day ten.

#### `unsafe` returns, under the existing rules

`crowbar-platform` has held zero `unsafe` since P3.40, which makes §17.4 pass
**vacuously**. This slice ends that. The user's ruling: `unsafe` is permitted
where macOS leaves no alternative, **in `crowbar-platform` and nowhere else**,
and the rest of the code must use Rust's real features rather than reach for it.

So, unchanged and now non-vacuous:

- `#![forbid(unsafe_code)]` on every other crate root — a hard compile error, not
  a lint (check-invariants rule 2).
- `#![deny(unsafe_op_in_unsafe_fn)]` in `crowbar-platform`, so an `unsafe fn`
  body gets no implicit unsafe block.
- Every `unsafe` construct carries a `# Safety` doc comment on its enclosing item
  that **proves the obligation**: which selector, to what class, on which thread,
  with what lifetime guarantee on every pointer crossing the boundary. "Safe
  because it works" is rejected (check-invariants rule 3).
- **The §17.4 mutation is re-run in this slice**, as the queue requires: *"re-run
  that mutation the moment `unsafe` returns."* It lands with the AppKit bridge,
  not after.

**Acceptance:** a screenshot of the Rust window beside the React window shows the
same frost, the same corners, and the same titlebar treatment; the mutation
evidence is recorded; check-invariants is 6/6.

### 5.5 Side-by-side capture script

Both apps launched at the same size against the same `CROWBAR_HOME`, two PNGs
written to a known path.

Two hazards already recorded and to be honoured:

- **Capture only from a settled app.** After any nav-stack push, carousel scroll
  or modal, reload before capturing. A cluster of `visible: false` on the
  reference side is a contaminated capture, not a finding.
- `screencapture` is blocked for secondary windows; a `CGWindowList` Swift script
  is the working path.

**Acceptance:** one command produces a labelled pair. This is the instrument the
user reviews every slice with, so it ships in Slice 0.

## 6. What carries over unchanged

All of §4.3 and §12 of the original spec:

- `crowbar-core` never mentions `gpui` (check-invariants rule 1).
- Design tokens stay sealed — a colour or spacing literal outside
  `crowbar-ui/src/theme/` is a compile error, plus rule 4's scanner for raw
  colour construction.
- `unwrap`/`expect`/`panic!`/`todo!`/`unimplemented!` denied outside tests via
  `[workspace.lints.clippy]`, no per-site `#[allow]`.
- All of `clippy::pedantic`.
- ≥98% line coverage on the logic crates, reported separately from corpus
  coverage and **never blended**.
- The §4.2 dependency graph.

`QUEUE.md` becomes a findings archive rather than a work queue. Its four recorded
oracle blind spots, the settled-app capture rule, and the contaminated-reference
hazard are all worth keeping and are all still true.

## 7. Open question, not blocking Slice 0

**The editor.** Zed's is out — Phase 0 items 0.9 and 0.13 audited it and the
verdict is NOT extractable at any tier: `editor`, `language` and `terminal` are
welded to `settings`; the diff-hunk model is a `gpui::Entity`-backed type welded
to `language`; `git_ui` alone pulls a 764,595-line closure. That leaves:

- a Rust component — there is no mature GPUI code editor;
- building on `crowbar-editor` from scratch;
- hosting Monaco in `crowbar-webview` as a bridge, shipping slice 4 and replacing
  it later.

This is the largest open question in the port, and the answer may reshape
`crowbar-webview` and slice 6 as well as slice 4. It should be settled by the
time slice 2 starts rather than when slice 4 does — but it does not block
Slice 0.
