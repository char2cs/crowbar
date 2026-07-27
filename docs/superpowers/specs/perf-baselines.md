# Performance Baselines — bare-metal program

Scenario protocol: pinned scenarios per spec §3.3, median of 3 runs, measured in dev-Chrome and (where noted) packaged WKWebView via `webview_execute_js` peek of `window.__perfLog`. Baseline column measured at `db777fa6` (fresh develop) unless noted.

| # | Metric | Baseline | Budget | Post-Wave-1 | Final (Task 33, prod-shaped) |
|---|--------|----------|--------|-------------|-------|
| M1 | keypress→paint terminal p95 | _pending Task 3 runtime pass_ | < 16ms FE-side | | **2ms** `terminal.echo` (local echo round-trip; Task-32 live, react-scan-independent) ✅ |
| M2 | INP app-wide | _pending Task 3 runtime pass_ | < 200ms | | **Unmeasurable in this rig** — synthetic events aren't `isTrusted`, so Event Timing records nothing; needs a foregroundable window + real HID |
| M3 | diff-open→interactive (pinned big diff) | **467ms first-open / 330ms remount** (WKWebView dev, 60-file/12k-line scenario) | < 500ms | | **216ms first-open** (60-file/12k-line, **clean prod frontend**, rAF-shim). Under budget. Task-32's 1026ms was dev+react-scan+cold-`getReview` inflation ✅ |
| M4 | workspace-switch→painted (warm) | **empty ws: 90–149ms (median 109)**; **content ws (60-file diff state): 344–506ms** — every switch is cold today | < 300ms | Task-32 dev (react-scan): warm 323ms | **Warm ~160–220ms median / 70ms best** (clean prod, fast-path, hidden-window rAF-shim). Full-reseed path ~176ms median. Cold ~115ms (Task-32 dev). Under budget; the ~160ms floor is the subtree-unhide reflow (see notes) |
| M5 | renders per keystroke / tab-switch | _pending Task 3 runtime pass_ | 1 (owner only) | | Addressed structurally by Wave-1 (Tasks 8/9/13/14) + Doctor; not re-measured numerically this rig |
| M6 | long tasks >50ms during typing burst | _pending Task 3 runtime pass_ | 0 | | Not measured (typing-burst rig needs foreground + real HID) |
| M7 | entry chunk JS (gzip) | **1,666,190 B** (raw 6,461,843 B; `index-BLdLAavC.js` @ 355d003f post-Wave-0; MonacoEnvironment markers ×6) + CSS 148,831 B gzip | ≤ 840,000 B gzip; 0 Monaco markers | **1,457,918 B** (raw 5,376,825 B; MonacoEnvironment markers ×3) — Task 4 | **481,307 B gzip** (raw 1,603,914 B; `index-CoUDXiAF.js`; **0 Monaco markers, 0 modulepreload**) + CSS 33,784 B gzip. **-71% vs baseline** ✅ |
| RD | React Doctor score | **48/100** (670 issues: Bugs 85E/92W, Perf 6E/93W, Sec 1E/6W, A11y 1E/30W, Maint 353W) | 100/100, 0 issues | | **100/100 — No issues found** ✅ |

## Diff-subsystem-at-scale program — Phase 0 baselines (2026-07-27)

Separate program, separate scenario: `scripts/perf/gen-big-diff-fixture.sh`, not the 60-file/12k-line scenario the M-series above uses. Full results and methodology in `perf-phase0-attribution.md`.

| # | Metric | Baseline | Budget | Notes |
|---|--------|----------|--------|-------|
| D1 | `GetFiles` (the `/review/files` hot path) | **489.8 ms/op** at 200 files / 100k lines | < 50ms, independent of diff size | A *tenth* of target scale. Called ≤4Hz with the review pane CLOSED |
| D2 | `git diff --numstat` vs `--name-status` (1M-line fixture) | **345ms vs 51ms** (~6.7×) | — | numstat compute is O(diff size); the code comment claiming O(file count) describes output size only |
| D3 | `lock.read.hold`, single reader | mean **203.9ms**, max **370.9ms** | < 100ms per acquisition | |
| D4 | `lock.read.hold`, 16 concurrent readers | mean **180.0ms**, max **522.9ms** | < 100ms per acquisition | `lock.read.wait` / `lock.write.wait` both 0.0ms — writer starvation NOT reproduced |
| D5 | `ChangedFilesTree` DOM cost | **500 files → 500 rows, 6,292 nodes** (12.6/file) | O(viewport), not O(files) | Permanently mounted: all 4 carousel panels are `display:flex`/`visible`, confirmed live |
| D6 | Entry chunk (gzip), pre-Shiki | **243,804 B** (raw 692,175 B) | ≤ 840,000 B | 596 KB headroom for Phase 3 |

Caveats: D1–D4 measured on a *clean* worktree; the reported symptom occurs while agents churn the tree, which makes `Status()` and the working-tree diff strictly more expensive — these are a floor. D6 measured via `vite build` directly (`bun run build` runs `tsc` first, currently red on a sibling session's untracked file).

### Phase 1 results (2026-07-27)

| # | Metric | Baseline | After Phase 1 | Verdict |
|---|--------|----------|---------------|---------|
| D1 | `GetFiles` bench, 200 files / 100k lines | 489.8 ms/op | **~94 ms/op** | 5.2× — budget replaced, see below |
| D3 | `lock.read.hold`, bench | mean 203.9ms / max 370.9ms | **mean 33ms / max 45.9ms** | ✅ under 100ms |
| D5 | `ChangedFilesTree`, 500 files | 500 rows / 6,292 nodes | **39 rows / 502 nodes** | ✅ bounded |
| — | `git.diff` invocations per `GetFiles` | 2 | **1** | whole-branch numstat gone |

**Live, end-to-end through the real daemon, on the 1M-line / 407-file fixture workspace** (`/v0/.../review/files`, warm cache, instrumentation armed):

| sample | value |
|---|---|
| `http.GET …/review/files` | **73.6 ms mean** (71–77ms wall) |
| `lock.read.hold` | **22.6 ms mean, 30.5 ms max** |
| `git.diff` | 1 call, 16.1 ms |
| `git.merge-base` | 2 calls, 13.7 ms each |
| `git.status` | 16.3 ms |
| `git.rev-parse` | 12.8 ms |
| `lock.read.wait` | 0.0 ms |

Phase 0 measured the same endpoint's git work at ~440ms on this fixture (name-status 51 + numstat 345 + status 44), so this is **~6× faster at target scale**.

**The result that matters:** 73.6ms at **1M lines** is *faster* than the 94ms benchmark at **100k lines**. Cost no longer tracks diff size — invariant 3 demonstrated at target scale, not merely argued. At a 4Hz tick that is a ~29% duty cycle instead of over 100%.

The `<50ms` figure in the original spec was replaced rather than met; it was picked before measurement and the residue is process-spawn floor (~13ms per `git` invocation on this machine, established by `rev-parse HEAD` which does no real work). See the spec's budget-correction note.

**Frontend live check:** with 407 changed files the Git panel holds **614 DOM nodes / 36 rows**; scrolling to 6000px moves the window (48 rows) while the scroll extent stays 10,704px = 446 rows × 24px. Rendering verified by screenshot: folder nesting, indentation, icons, ±badges, binary marker, 108fps.

### Phase 2 results — windowed diff API (2026-07-27)

Live through the real daemon on the 1M-line / 407-file fixture workspace.

| endpoint | result |
|---|---|
| `review/outline` | 2,283,960 B raw → **26,927 B gzipped (84.8×)**, 580 ms, 407 files / 38,604 hunks / 1 partial |
| `review/patch`, uncapped | **13,282,412 B streamed in 127 ms** |
| `review/patch`, `maxLines=2000` | 142 B + readable `X-Crowbar-Diff-Truncated: true` |
| `review/patch`, ordinary file | 75,201 B in 38 ms |
| `review/search`, literal, limit 200 | 200 hits, truncated, **44 ms** |
| `review/search`, regex, limit 100 | 100 hits, 41 ms |
| `review/search`, no match (full 46 MB scan) | 0 hits, 700 ms |
| missing `path` / invalid regex | 400 |

Daemon-side memory measured with `getrusage(RUSAGE_SELF)` — whole-process
`time(1)` is meaningless here because `git diff -M -U3` *itself* peaks at
71.3 MB on this fixture. Outline 17.7 MB RSS / 6.4 MB total alloc across 1.44M
lines; patch 16.98 MB vs 57.25 MB buffering; search 17.7 MB, flat against both
diff size and hit count.

**Gzip beat its own pessimistic bound by 13×** — 26.9 KB against an estimated
359 KB, because real hunk numbers are far more regular than the randomised
entropy model used to bound it. The outline payload is a non-issue on the wire;
if anything bites in Phase 3 it will be `JSON.parse` and the ~38.6k objects the
webview allocates, which compression does not help.

**Known shape for Phase 3:** the fixture's monster file is a *single*
420k-line hunk, so no hunk fits under the default cap and the response rounds
down to a 142-byte header-only patch with `truncated: true`. Correct by design
(rounding up would return all 420k lines and defeat the cap), but the client
must render that as "show all", not as an empty file.

**Task-33 methodology & measurement caveats (read before trusting the Final column):**
- **Prod-shaped rig** = `vite build` output (no react-scan, no HMR — react-scan is `import.meta.env.DEV`-gated so it is absent from a build) served by `vite preview` on :5173 to the running **dev Tauri shell** (so the MCP bridge on :9223 stays available for measurement). Confirmed live: `window.__REACT_SCAN__` absent, `@vite/client` absent. `window.__CROWBAR_PERF__=true` set post-boot so `markStart/markEnd` record (they gate at call-time).
- **Hidden-window rAF shim.** The window was on an inactive Space the whole run (`visibilityState==="hidden"`, un-foregroundable — two Crowbar apps run; osascript name-targeting was avoided to protect the user's PRODUCTION app). `workspace.switch` and `diff.open` close their spans in a `requestAnimationFrame`, which never fires while hidden, so a `requestAnimationFrame = cb=>setTimeout(cb,0)` shim was installed. Verified `setTimeout(0)` is **not** clamped here (~0ms; backgroundThrottling:disabled), so shimmed spans are honest synchronous cost — real hardware adds paint/reflow on top, so these run slightly **low** for the paint tail.
- **react-scan was ~half the apparent cost.** Task-32's dev warm-switch 323ms and diff-open 1026ms were measured with react-scan active; the clean-prod re-measure (M4 ~160–220ms, M3 216ms) is the honest floor.
- **M4 measured via A→home→A** driven by `location.hash` (TanStack hash router). Fast path = hidden <5s (`WARM_FRESHNESS_WINDOW_MS`); the >5s dwell reproduces the pre-fix full-reseed path in the same build, giving a clean single-build A/B.

**Pinned big-diff scenario:** 60 new files × 201 lines (12,060 insertions) at `<devhome>/projects/d92815d4…/github.com/zen-browser/desktop/dev/perf-baseline-scenario/`, surfaced via `git add -N perf-baseline-scenario`. Reset: `git reset && rm -rf perf-baseline-scenario` in that checkout. Additive-only by design (no pre-existing files touched). KEEP until Wave-1 A/B re-measures are done.

**WKWebView session notes (2026-07-13, dev build @c9cb78e4 + react-scan active — overhead present equally in baseline and post-fix runs):**
- M4 measured via `workspace.switch` span (hydrate→painted): scales ~3-5× with workspace content under the destroy-everything model; blows budget on any real workspace.
- M3 via `diff.open` span; scenario diff is add-only (cheap daemon side) — content-modification diffs will be heavier; revisit in Task 18.
- M1 not measurable in WKWebView (xterm key injection blocked — known bridge limitation); Chrome leg pending, must land before Task 10 merges.
- Steady-state observation: an `Update` measure ticks ~2Hz continuously while the git panel is mounted — live corroboration of the P2 refetch-loop churn.
- Harness lesson: `__perfLog`'s 500-cap ring floods with Event Timing entries within seconds, evicting measures — external readers must keep their own measure collector (session used an in-page `PerformanceObserver` → `window.__measures`). Also: the INP push in main.tsx bypasses the ring cap (unbounded); fold a fix into the next perf-module-touching task.

Measurement notes:
- M7 baseline from a real `vite build` of `db777fa6` (verified by the revalidation pass, 2026-07-13): entry referenced by `dist/index.html`, zero modulepreload links; `ts.worker` (7.02MB) is a separate worker chunk and NOT counted against the entry budget.
- M7 Task-4 pass (2026-07-13): lazy'd `EditorPane`/`DiffPane` in `pane-container.tsx` (same `lazy()` + shared `<Suspense fallback={null}>` pattern as the five already-lazy sibling panes) plus an idle-callback prefetch of both specifiers so the first real file-open doesn't double-fetch (`ls dist/assets` shows exactly one `editor-pane-*.js` and one `diff-pane-*.js` — Vite deduped the `lazy()` import and the prefetch `import()` onto the same chunk). Entry dropped 6,462,310 B → 5,376,825 B raw (16.8%) and 1,666,277 B → 1,457,918 B gzip (12.5%); `editor-pane` chunk itself is only 22,983 B raw / 8,108 B gzip (the bulk of Monaco/tree-sitter grammar weight lands in further-split async chunks, e.g. `define-theme-*.js` 209KB, `git-diff-editor-stack-*.js` 865KB, per-language grammar chunks).
  **Blocker — 0-marker target NOT met (3 of 6 MonacoEnvironment markers remain in entry):** root-caused to `web/src/features/workspace/stores/workspace-store.ts:21` statically importing `EDITOR_CREATE_OPTIONS`/`langForUri`/`realEditorApi`/`realModelApi` from `@/features/editor/lib/monaco-adapters.ts`, which does `import { editor as monacoEditor, Uri } from 'monaco-editor'` at module scope. `createWorkspaceStore()` calls `new ModelRegistry(realModelApi())` and `new EditorManager(realEditorApi(EDITOR_CREATE_OPTIONS), registry, meta)` **synchronously, unconditionally**, at workspace-store creation (i.e. on cold launch, before any editor pane is ever opened) — confirmed by stubbing out the import (throwaway experiment, reverted) and rebuilding: entry fell to 1,595,210 B raw / 478,610 B gzip with 0 markers, proving this is the only remaining eager path. `workspace-store.ts` is reached from `pane-container.tsx`'s (non-lazy) `useWorkspaceStore()` import, so it's on the static chain regardless of pane laziness. Fixing it cleanly requires the real `MonacoEditorApi`/`MonacoModelApi` adapter to load `monaco-editor` asynchronously — but `.editorManager`/`.modelRegistry` are consumed synchronously not just by editor components but by generic, buffer-type-agnostic store logic (`workspace/stores/slices/buffer-slice.ts`, `workspace/stores/slices/pane-slice.ts`), so making this lazy means giving `EditorManager`/`ModelRegistry`'s real adapter (and every synchronous call site) an async-aware contract — real editor-internals restructuring, not an import-graph edge, and out of Task 4's scope per its own guardrail. Left as an explicit follow-up (candidate Task 6): defer `ModelRegistry`/`EditorManager` construction in `workspace-store.ts` behind a lazy/async seam without changing `MonacoEditorApi`/`MonacoModelApi`'s public shape for callers that don't need the real backing yet.
- Lint baseline (recorded during Task 1): 10 pre-existing errors in `web/scripts/provision-tree-sitter.mjs` + 49 `react-hooks/exhaustive-deps` warnings. Per-task lint gate = no NEW findings; RD batches 3/6 own clearing these.
