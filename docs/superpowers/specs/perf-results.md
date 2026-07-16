# Performance Results — bare-metal-frontend-performance (closing summary)

Program goal: make Crowbar's web frontend feel as close to Zed as this stack
(Tauri + WKWebView + React + Monaco) allows. Branch `enhancement/performance`,
base `55625a7f`. This document is the closing scorecard: baseline → final per
metric, what shipped, and the known costs that remain.

> Measurement honesty note. The verification window was hidden the entire final
> pass (`visibilityState==="hidden"`, un-foregroundable). Absolute paint-bound
> numbers are therefore not obtainable in this rig; the numbers below are the
> **synchronous** cost measured under a `requestAnimationFrame→setTimeout(0)`
> shim in a **production-built** frontend (no react-scan, no HMR). See
> `perf-baselines.md` → "Task-33 methodology & measurement caveats".

## Scorecard

| Metric | Baseline (`db777fa6`, dev) | Budget | Final (clean prod) | Verdict |
|---|---|---|---|---|
| M1 terminal echo (keypress→paint) | pending | < 16ms | **2ms** | ✅ far under |
| M3 diff-open (60-file/12k-line) | 467ms first-open (dev) | < 500ms | **216ms first-open** | ✅ under |
| M4 warm workspace-switch | 344–506ms content ws (every switch cold) | < 300ms | **~160–220ms median, 70ms best** | ✅ under |
| M7 entry chunk (gzip) | 1,666,190 B, 6 Monaco markers | ≤ 840,000 B, 0 markers | **481,307 B, 0 markers** | ✅ -71% |
| RD React Doctor | 48/100 (670 issues) | 100/100, 0 issues | **100/100, 0 issues** | ✅ |
| M2 INP / M6 long-tasks | pending | <200ms / 0 | unmeasurable in rig | ⚠ needs foreground+HID |

Every budgeted metric that is measurable in this rig is **under budget**.

## What shipped (by wave)

- **Wave 0 — instrumentation (Tasks 1–3).** A zero-cost-unless-armed perf module
  (`lib/perf/instrumentation.ts`): `markStart/markEnd` spans + a `PerformanceObserver`
  feeding `window.__perfLog`, armed in dev or via `window.__CROWBAR_PERF__` in prod.
  Spans wired: `terminal.echo`, `workspace.switch`, `diff.open`.
- **Wave 1 — the fixes (Tasks 4–17).**
  - **Entry chunk -71%** (1,666KB→481KB gzip, 6→0 Monaco markers): lazy `EditorPane`/`DiffPane`,
    an effect-gated async Monaco seam in `workspace-store.ts`, 32 per-language grammar chunks.
  - **Git/diff churn killed**: coalesced git-status stream (no 2Hz refetch loop), per-file
    diff invalidation (path+status+staged, 30s TTL), split-diff buffers no longer re-serialize
    per toggle, per-tab render isolation.
  - **Keep-alive retention (Tasks 17/31)**: workspaces stay mounted-but-hidden across switches
    (and across the home transit — session-lifetime `WorkspaceHost`) instead of destroy-on-switch;
    warm return is a display flip, not a re-hydrate + re-mount.
  - **Terminal**: model-driven render already landed; `writeFrame`-on-arrival keeps echo at 2ms.
- **Wave 2 — React Doctor 48→100 (Tasks 19–29).** 11 real effect-dependency/cleanup fixes,
  ~130 dead files/islands removed (auto-update stub, cloud settings-sync, Pro/WhatsNew),
  two false-positive rules disabled with per-site justification, security findings cleared.
- **Production readiness (Tasks 30–33).** Reveal-in-Finder unblocked (async + `spawn_blocking`;
  it was freezing the webview on a main-thread XPC call), terminal M1 A/B, the Task-32 live sweep
  (11 PASS/1 FAIL), and **this pass (Task 33)**.

## Task 33 — final pass, what changed

**Target A — warm-switch reactivation (incremental activation reconcile).** New
`features/workspace/lib/activation-freshness.ts` ledger + fast paths in
`use-workspace-effects.ts`: when a workspace was hidden **only briefly**
(`WARM_FRESHNESS_WINDOW_MS = 5s`) **and** the global file-tree / git stores still
hold *its* data (store-identity guard), a warm return **keeps the loaded data** —
skipping the full `fetchFileTree` + the four-request `fetchAllGitData` seed, the
synchronous `isFileTreeLoading:true` "Loading…" flash, and the downstream
`git-status-changed` diff refetch. Correctness-first: any doubt (hidden too long,
store clobbered by another workspace, cold mount) falls back to the full re-seed,
so the **Task-29 stale-tree bug cannot return**. Git status/log **self-heal**
regardless — the re-subscribed stream is seeded with the frame preserved across
the gap, so a frame that *differs* (something changed while hidden) still reloads.

- **Effect on the switch span:** modest (full-reseed ~176ms → fast-path ~162ms median;
  110ms → 70ms best). The bulk of the ~160ms is the **subtree-unhide reflow**, common to
  both paths and *not* the refetch (which is async, off the paint frame).
- **Effect on the async tail (the real win):** eliminates ~5 `crowbar://` daemon requests
  and the post-switch diff-refetch cascade + loading flash on every quick warm return —
  proven deterministically by the fast-path unit tests. This is where "feel" improves.

**Target B — diff.open.** Re-measured honestly: **216ms first-open** for the 60-file
scenario in the clean prod frontend — **under the 500ms budget, no optimization needed**.
Task-32's 1026ms was dev + react-scan + cold-`getReview` inflation. (The virtualizer mounts
only ~overscan sections, so `diff.open` does not scale with file count.)

**Target C — production-shaped run.** `vite build` → `vite preview` served to the dev
Tauri shell (bridge intact); confirmed react-scan and the vite dev client absent. This is
what made the honest numbers above possible — react-scan alone was ~half the apparent cost.

**Tests / gates:** `activation-freshness.test.ts` (7) + 5 fast-path cases in
`use-workspace-effects.test.ts` (fresh skip / expired / clobbered / cold / git self-heal);
the stale-tree regression suite stays green. Full suite **1930 passing**, tsc clean,
lint **0E/2W** (pre-existing), React Doctor **100/100**.

## Known remaining costs & follow-ups

1. **Warm-switch floor ~160ms = subtree-unhide reflow.** To approach Zed's ~100ms the lever
   is `content-visibility`/deferred Monaco+xterm refit on the retained slot (Task-32 caveat-8).
   **Deferred deliberately**: it is a paint-behaviour change and this rig cannot verify paint
   (hidden window) — shipping it blind would violate the project's "verify in Tauri before
   claiming" rule. Needs a foregroundable session.
2. **Home-ws Duplicate 404** (Task-32 FAIL-4): `POST /home/files/copy` route missing on the
   home group — owned by the concurrent api/ agent this pass.
3. **Home explorer shows the previous workspace's tree** momentarily (Task-28 follow-up): global
   FS store staleness on home activation; clear/repopulate on activation.
4. **Locked-workspace menus still show mutation items** (always-409) — needs gating (Task-28).
5. **Perf-module ledger items**: unbounded INP push bypasses the ring cap (main.tsx);
   `diff.open` rAF not cancelled on unmount (can record an open-to-unmount span).
6. **Editor edges**: `fileMissing` buffer arms Monaco unnecessarily; arm-load failure is a
   silent blank pane; `.jsx/.erb` bypass the language-id drift guard (plaintext before+after).
7. **Live gates still open** (all blocked on a foregroundable window): Task-5 multi-language
   highlight check, Task-12 Claude↔Codex live switch, M2 INP / M6 long-tasks under a real
   typing burst, and a live paint-bound re-measure of M3/M4.
8. **Task-7 UX trade-off** (awaiting user verdict): a closed-pane sidebar shows working-tree
   only (loses committed-on-branch files + count badges); the name-status endpoint is the
   restore option.
