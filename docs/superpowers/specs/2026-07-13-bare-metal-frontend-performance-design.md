# Bare-Metal Frontend Performance & React Doctor 100/100 — Design

**Date:** 2026-07-13
**Branch:** `enhancement/performance` (at `db777fa6`, fresh `origin/develop`)
**Status:** Approved direction; spec pending user review

## 1. Context and evidence

A five-dimension audit (terminal input, diff pipeline, React practices, global heaviness, measurement tooling; 15 agents, every high-severity finding adversarially verified against the code, then revalidated after fast-forwarding onto the agentic-bridge merge) established:

**Crowbar's frontend is not broadly broken.** The hot loops are already imperative: Monaco typing reaches the store only through a 150ms trailing sink, the file tree is virtualized with a hand-written comparator, terminal bytes and LSP diagnostics bypass React state, and store discipline (narrow selectors, `useShallow`) is clean. The "feels like React, not bare metal" experience comes from five specific, verified defects — plus a codebase-hygiene backlog quantified by React Doctor at **48/100 (Critical), 670 issues** on the current tip.

This spec covers two deliverables:

1. Fix the five verified performance root causes (P1–P5), each proven with before/after numbers.
2. Fix **everything** React Doctor reports, reaching **100/100**, and ratchet CI so it never regresses.

## 2. Goals and non-goals

**Goals**

- Cold launch no longer parses the Monaco engine; entry chunk loses ≥50% gzip weight.
- Big-diff branches no longer degrade the rest of the app (no uncached full-diff refetch loop).
- Workspace switching between recently-used workspaces (configurable keep-alive window, default 10m) is a visibility toggle, not a rebuild.
- Terminal echo path sheds all avoidable frontend latency (the discretionary rAF, store churn, chat-pane remount churn).
- Re-render storms eliminated: one component render per state change on tab/pane/terminal-title paths.
- React Doctor score 100/100 with zero errors and zero warnings; dead code (115 files, 131 exports, 23+1 deps) purged.
- A permanent, low-cost measurement layer so every claim above is a number, not an adjective.

**Non-goals (explicitly deferred)**

- Speculative local echo or transport re-architecture (direct webview↔daemon socket). The IPC round trip is the structural floor vs Ghostty; we first remove avoidable latency (P4), measure the residual gap, and decide separately.
- Daemon-side changes. Everything here is frontend-only. (A backend git-status revision/etag field would help P5's dedup; noted as a follow-up, not required.)
- Surfacing base-branch staleness in the UI (follow-up from the worktree-fork fix; separate work).
- Branch Review UX/renderer redesign (separate, contested project — see memory `git-review-redesign`).

## 3. Measurement layer (prerequisite for every fix)

No fix in this spec may merge without before/after numbers. The audit found the repo's existing instrumentation is **silent**: `use-performance.ts` marks the tokenizer but funnels into `frontendTrace()` (a hardcoded no-op) and a `performance-metric` CustomEvent nobody listens to; `file-open-benchmark.ts`'s `finish()` is never called. We wire this up rather than duplicating it.

### 3.1 `web/src/lib/perf/instrumentation.ts` (new)

- `markStart(name)` / `markEnd(name)` wrapping `performance.mark`/`performance.measure`.
- A `PerformanceObserver({ entryTypes: ['measure', 'event'] })` pushing `{name, startTime, duration}` into a **ring buffer at `window.__perfLog`** (cap 500). The `event` entries give Event Timing (`processingStart/End`) for real input-latency attribution.
- Zero-cost when disabled: everything no-ops unless `import.meta.env.DEV || window.__CROWBAR_PERF__`. Nothing ships armed to users.
- Existing silent instruments are rewired into this module (tokenizer marks, file-open benchmark) or deleted if redundant.
- This single buffer is the contract for **both** runtimes: Chrome DevTools/CDP traces read it in dev; `webview_execute_js` polls it in the packaged WKWebView app (which has no CDP).

### 3.2 Dev-only tooling (two new devDependencies)

- **react-scan** — dynamic-imported in `main.tsx` behind `import.meta.env.DEV`; visual re-render counts. Not the Vite plugin (it patches JSX in the shared build).
- **web-vitals** — `onINP(cb, {reportAllChanges: true})` behind the same gate, logging into `__perfLog`.
- Remove the dead `@tanstack/react-query-devtools` devDependency (React Query was retired; the devtools mislead).
- **react-doctor** — added as devDependency with a `doctor` package script (also serves Workstream RD).

### 3.3 Metrics and budgets

| # | Metric | How measured | Budget |
|---|--------|--------------|--------|
| M1 | keypress→paint, terminal, p95 | mark at `terminal.onData`, end at `terminal.write` parse-complete callback + rAF | < 16ms FE-side |
| M2 | INP, app-wide | web-vitals onINP (dev Chrome + WKWebView event-timing) | < 200ms |
| M3 | diff-open→interactive (pinned big-diff scenario) | mark on tab click → Monaco diff ready | < 500ms |
| M4 | workspace-switch→painted | mark on switch → first painted buffer/tree | < 300ms warm |
| M5 | component renders per keystroke / per tab-switch | react-scan | 1 (owner only) |
| M6 | long tasks >50ms during typing burst | Chrome trace | 0 |
| M7 | entry chunk size (JS gzip) | `vite build` + `dist/assets` | ≤ 50% of 1.67MB baseline; no Monaco markers |

Scenarios are pinned against the already-seeded dev repos (fixed branch/commit for the big-diff case), run 3×, median reported. Baselines for M1–M7 are captured **before any fix lands** and recorded in `docs/superpowers/specs/perf-baselines.md` as the comparison table each subsequent change appends to. WKWebView numbers via `make dev-desktop` + Tauri MCP `webview_execute_js` peek-back; never against the production install.

## 4. Workstream P — the five performance fixes

### P1 — Monaco out of the entry chunk

**Root cause (verified in the production bundle):** `ide-shell.tsx` statically imports `WorkspaceView`; `pane-container.tsx:69` statically imports `EditorPane` (its five sibling pane types are already lazy); the chain reaches `monaco-editor` and `features/editor/monaco/language-contributions.ts`, which eagerly registers ~30 language grammars + 4 language services. Result: 6.46MB/1.67MB-gzip entry JS + 615KB CSS parsed on every launch. Route-level splitting otherwise works (`_shell`, `oobe`, three, mermaid, xterm all split correctly), so this is a broken import chain, not a broken bundler.

**Fix:**
- Make `EditorPane` lazy in `pane-container.tsx`, matching its siblings (Suspense fallback consistent with the existing pane loading states).
- Convert `language-contributions.ts` to on-demand registration: dynamic `import()` per language keyed off the file extension on first open; the 4 language services likewise.
- Investigate why `TanStackRouterVite({ autoCodeSplitting: true })` leaves `routeTree.gen.ts` fully static; regenerate/fix config if cheap, otherwise the two lazy boundaries above suffice.
- After first paint, idle-prefetch (`requestIdleCallback`) the editor chunk so the first file open doesn't pay the full load on slow disks.

**Acceptance:** M7 met (no `MonacoEnvironment`/`monaco-editor` markers in the entry chunk; gzip ≤ 0.84MB); first editor open after cold launch < 300ms on the dev machine (measured, median-of-3); no regression in editor tests.

### P2 — Diff pipeline: stop the refetch-everything loop

**Root cause (verified):** the sidebar carousel renders all panels permanently, so `GitPanel` → `useReviewDiff` (`use-review-diff.ts:23`) is always mounted and refetches the **entire branch diff, uncached** (`getReview`, unlike the cached `getFileDiff`) on every `git-status-changed` (~2–3Hz during agent activity). `setBranchReviewDiff` (`branch-review-slice.ts:120`) has no equality guard, so identical payloads still swap object identity and re-render the review pane's every visible section. The working-tree tab (`git-diff-editor-stack.tsx:701`) blanket-invalidates `gitDiffCache` for the whole repo and refetches every changed file per tick. The stack also subscribes to the whole `buffers` array (line 555) and registers 3 buffers per visible file even in unified mode (lines 188–229).

**Fix:**
- Cache `getReview` like `gitDiffCache`, and dedup: hash/compare the payload; skip `setBranchReviewDiff` + `setFiles` entirely when unchanged. This one guard protects the sidebar and the pane at a single choke point.
- Sidebar changed-files list derives from the cheap git-status data, not the full line-level diff; the full diff is fetched only when the Branch Review pane is actually open.
- Working-tree refresh: invalidate/refetch only files whose status/oid changed since the previous frame; never repo-wide `invalidate()`.
- Drop the whole-`buffers` subscription (read via `getState()` inside the callback); serialize split-view content and register split buffers only when `viewMode === 'split'`.

**Acceptance:** with the review pane closed, a git tick on the pinned big-diff branch performs **zero** full-diff fetches and zero review-store writes; with it open and content unchanged, react-scan shows 0 section re-renders per tick; M3 < 500ms; scrolling a 100-file diff triggers no tab-bar renders.

### P3 — Workspace keep-alive (kill destroy-everything switching)

**Root cause (verified):** `workspace-view.tsx:55` — on `wsId` change the whole subtree returns `null` until async re-hydration, and the `[wsId]` effect cleanup calls `destroyWorkspaceStore` for the workspace being left. Every switch is a cold rebuild: Monaco re-instantiation, file-tree refetch, terminal re-attach with replay.

**Fix:**
- **Time-based retention:** `IDEShell` keeps every `WorkspaceView` whose last activation is within the **keep-alive window** mounted with `display: none` (plus `inert`), stores retained. Default window: **10 minutes**; user-configurable in the Settings menu (with a search-index entry); `0`/off restores today's destroy-on-switch behavior. `destroyWorkspaceStore` runs only on window expiry, cap eviction, or explicit workspace close.
- **Safety cap (non-configurable):** at most 6 workspaces retained simultaneously regardless of TTL — xterm's WebGL addon holds a GPU context per terminal and browsers silently drop contexts beyond ~8–16, so retention must be resource-bounded, not just time-bounded. Beyond the cap, evict oldest-activated first.
- **Eviction scheduling:** no polling interval (ambient-cost hygiene, §4/global-heaviness) — arm a single timer for the earliest upcoming expiry, re-armed whenever activations change.
- Activation of a warm workspace = visibility toggle + a cheap reconcile (refresh git status/tree via the existing watchers — no rebuild, no render gate).
- Cold entries (expired or never mounted) keep today's hydrate-then-render path.
- The history-store leak is already fixed; terminal Phase-1 persistence already survives switches.

**Acceptance:** M4 — warm switch < 300ms to painted, with instrumentation proving no Monaco re-instantiation and no file-tree HTTP refetch on warm activation; memory growth across a 10-switch cycle bounded (heap snapshot before/after within noise); expiry verifiably destroys stores (test on a fake clock signal, no real timing waits); the setting round-trips through the Settings UI; cold-path behavior unchanged (regression tests).

### P4 — Terminal typing latency (frontend-controllable share)

**Root cause (verified):** `use-terminal-connection.ts:171` defers every incoming frame — including single-character echoes — through `requestAnimationFrame` before `terminal.write()`, stacking up to a full frame (~8–16ms) on top of xterm's own internal render rAF. The daemon adds zero fixed latency (its 8ms interval is a burst ceiling, verified); the batching buys nothing because the daemon already coalesces and xterm caps rendering at once per frame. Secondary: `terminal-store.ts:32` reallocates the sessions Map on every title/cwd update and `use-buffer-display-name.ts:18` subscribes to the whole Map → full tab-bar re-render per prompt redraw. New since the chat merge: `AgentChatPane` keys the terminal by `attachment.sessionId`, forcing a full remount (fresh socket, fresh listeners) on every revive/provider switch.

**Fix:**
- Write incremental (non-snapshot) frames to `terminal.write()` **on arrival**; keep the snapshot-barrier correctness gate as a plain boolean; keep the one-shot rAF + `refresh()` only for bulk attach-replay finalize (the WKWebView repaint workaround stays).
- `updateSession` skips the Map reallocation when merged fields are unchanged; `useBufferDisplayName` selects only the per-session fields it reads.
- `AgentChatPane`: reattach to the new session imperatively instead of remounting via `key`; verify its subtree `MutationObserver` stays rAF-coalesced and detaches when hidden.

**Acceptance:** M1 improves ≥8ms at 120Hz vs baseline (median-of-3, measured in both runtimes); tab bar registers 0 renders during a typing burst and during an agent-chat stream (react-scan); terminal conformance suite untouched and green; live-verified typing feel in the Tauri app.

### P5 — Re-render tier

**Verified defects:** TabBar builds fresh closures per tab per render, defeating `TabBarItem`'s comparator-less memo (`tab-bar.tsx:445-467`), and subscribes broadly (`s.panes`, `pendingClose`); every recursion level of the pane tree subscribes to the whole `panes` record (`pane-node-renderer.tsx:41`, `split-view-root.tsx:18`), so any tab/pane-local change re-renders the entire layout tree; `use-active-workspace-state.ts:41` has no equality guard (latent storm for the next object-selector consumer); `use-workspace-effects.ts:265` `JSON.stringify`s the full git-status frame ~6×/s just to dedup.

**Fix:**
- TabBar: stable delegated handlers (id/index-keyed), a real `TabBarItem` comparator, and a projected subscription (id/name/dirty tuples with `useShallow`).
- Pane tree: leaves read their own pane via `usePaneById(node.id)`; tree structure comes from the separately-subscribed `rootLayout`.
- `useActiveWorkspaceState`: accept an `equalityFn` (default shallow), skip `setValue` when equal.
- Status dedup: shallow structural compare against the parsed previous frame (or a cheap field-hash) instead of full stringify.

**Acceptance:** react-scan on the pinned scenarios — tab switch renders only the affected pane's components; M5 = 1 across tab-switch/typing/title-change paths; the stringify hotspot gone from typing-burst traces (M6 = 0).

## 5. Workstream RD — React Doctor 100/100

Baseline (fresh tip `db777fa6`): **48/100** — 670 issues: Bugs 85E/92W, Performance 6E/93W, Security 1E/6W, Accessibility 1E/30W, Maintainability 353W. Score formula is `100 − 1.5×(error rule kinds) − 0.75×(warning rule kinds)`, so 100/100 means **zero remaining violations of every rule kind**.

### 5.1 Policy

- **Fix, don't suppress.** A rule may be config-disabled only when factually inapplicable to this repo, and each exception must carry a one-line justification in the config and be listed in the final report. Known candidates (to be confirmed at fix time, not assumed): `require-pnpm-hardening` (repo is bun-only; pnpm retired in PR #46). Accessibility issues are fixed, not excepted.
- **React Doctor findings are hypotheses** (its own guidance): each fix batch reads the code, classifies true/false positive, and false positives are the only other legitimate config entries — again justified inline.
- Behavior-adjacent fixes (the Bugs classes) require the surrounding tests to pass and, where a fix changes observable behavior, a test pinning the corrected behavior (mirror structure in `web/src/__tests__/`, `@/` imports, per CLAUDE.md).

### 5.2 Batches (by rule family, riskiest first)

1. **Bug errors (85):** state-updater-with-side-effects ×34, ref-mutated-during-render ×33, uncleaned effect subscriptions/timers ×14, effect-dep-recreated ×6. Real defect potential; per-finding code reading; small batches per family.
2. **Security (1E/6W):** imported-metadata-reaches-code-execution (the one error — investigate properly), HTML-injection sinks ×3, iframe sandbox.
3. **Bug warnings (~92):** missing-effect-deps ×39, state-chained-through-effects ×17, effect-resubscribes-on-changing-callback ×9, parent-sync-via-callback-effect ×8, effects-chained ×5, and the singletons.
4. **Performance warnings (~93):** heavy-library-eager ×20 (largely resolved by P1 — rerun after P1 lands), chained-array-iterations ×25, await-in-loop ×13, unstable-context-value ×7, full-framer-motion-import ×6, map-filter-double-loop ×6, Intl-rebuilt ×4, and the rest.
5. **Accessibility (1E/30W):** interaction-on-static-element ×9, label-missing-control ×8, missing-keyboard-handler ×7, role-vs-tag ×6.
6. **Maintainability (353W) & dead code:** unused-file ×115, unused-export ×131, unused-dependency ×23 + 1 dev-dep — **sample-first**: validate a ~15-file sample against dynamic imports, mock-mode entry points (`dev:mock`, MSW), generated files, and test helpers before sweeping the rest; pre-production rules apply (delete freely, no migration shims). Then non-component-exports ×41, large-components ×20 (split only where it doesn't fight P1–P5 work), pure-function/static-value-rebuilt ×20, boolean-prop combos ×2, unversioned localStorage key.

Batch order interleaves with Workstream P where they touch the same files (e.g. batch 4's heavy-lib findings after P1; effect hygiene in terminal/diff files rides along with P2/P4 commits to avoid churn).

### 5.3 Verification per batch

`bunx react-doctor .` scoped rerun + full `bun run test` + `bun tsc --noEmit` + `bun run lint` green; for batches touching UI behavior (effects, a11y, security), a designed manual check in the live Tauri dev app.

### 5.4 Ratchet (CI)

- `doctor` script + react-doctor devDependency.
- CI job on PRs: react-doctor over changed files — no new issues allowed; full-run score must be monotonically non-decreasing, final gate = 100.
- Bundle-size budget check (M7) added to `frontend-checks`.
- Perf smoke: the M1/M4/M5 scenario script run manually per release until it proves stable enough to gate CI (flaky perf gates are worse than none).

## 6. Sequencing and delivery

**Delivery mode: big bang** (user decision, 2026-07-13). All four waves execute as one continuous program on `enhancement/performance` with no per-wave user checkpoints; the budgets and per-batch verification gates below are the only stop conditions. Internal ordering is still safety-driven:

| Wave | Content | Exit criterion |
|------|---------|----------------|
| 0 | Measurement layer (§3) + baselines recorded | `perf-baselines.md` committed with M1–M7 baselines |
| 1 | P1 → P2 → P4 → P5 → P3 (keep-alive last, it's the invasive one), each with before/after numbers | All P budgets met |
| 2 | RD batches 1–6 | react-doctor 100/100, exceptions documented |
| 3 | Ratchets (§5.4) | CI enforcing score + bundle budget |

- All work lands as reviewable commit slices on `enhancement/performance`. **No pushes, no PRs unless explicitly requested.**
- Implementation via subagents on cheaper models (Opus for P-fixes and RD bug/security batches; Sonnet for mechanical RD families), each slice adversarially reviewed before commit.
- House rules bind throughout: tests mirror `web/src/__tests__/` with `@/` imports; kebab-case files; narrow store selectors; no timing in tests; `@/components/ui/*` + token colors only; live Tauri verification via `make dev-desktop` (never the production install); evidence before assertions.

## 7. Risks

- **P3 (keep-alive) is the invasive one:** hidden-but-mounted subtrees can leak listeners/observers or fight the pane-persistence logic, and a TTL window is unbounded within its duration (12 workspaces touched in 10 minutes = 12 live Monaco/xterm trees). Mitigation: land last among P-fixes, behind thorough switch-cycle memory measurements; the hard retention cap (6) bounds GPU contexts and memory regardless of the window.
- **P1 first-open latency** trades startup weight for a lazy-load pause; idle prefetch mitigates; budget enforces it.
- **Dead-code sweep false positives** (mock mode, dynamic refs): sample-first protocol; git history is the undo.
- **100/100 vs honesty:** if a rule turns out to demand a change that harms the product (e.g. an a11y rule fighting the terminal canvas), the exception route (§5.1) with written justification is the escape hatch — score is the target, silent suppression is the failure mode.
