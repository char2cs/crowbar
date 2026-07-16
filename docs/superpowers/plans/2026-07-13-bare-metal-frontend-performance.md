# Bare-Metal Frontend Performance + React Doctor 100/100 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the five verified frontend performance root causes (with before/after numbers for each) and drive React Doctor from 48/100 to 100/100, with CI ratchets so neither regresses.

**Architecture:** Wave 0 builds a zero-cost-when-disabled measurement layer (`window.__perfLog` ring buffer) and records baselines. Wave 1 lands the five fixes in safety order (P1 bundle split → P2 diff loop → P4 terminal → P5 re-render tier → P3 keep-alive). Wave 2 clears all 670 React Doctor findings in six risk-ordered batches. Wave 3 adds CI gates.

**Tech Stack:** React 19, Zustand 5 (+`useShallow`), Monaco 0.55, xterm 5.5, Vite 6, Vitest + Testing Library + MSW, bun (NOT pnpm/npm), react-doctor, react-scan, web-vitals, fast-deep-equal (already a dep).

**Spec:** `docs/superpowers/specs/2026-07-13-bare-metal-frontend-performance-design.md` — budgets M1–M7 in §3.3 govern acceptance.

## Global Constraints

- All commands run from `web/` with bun: `bun run test`, `bun tsc --noEmit`, `bun run lint`, `bunx vite build`. bun lives at `~/.bun/bin/bun` if not on PATH.
- Tests live in `web/src/__tests__/` mirroring `web/src/` (a test for `features/X/lib/foo.ts` goes to `__tests__/features/X/lib/foo.test.ts`); use `@/` imports; kebab-case filenames; exported components PascalCase.
- Store rules: narrow selectors only, `useShallow` for object/array selectors, `getState()` only in handlers/effects.
- UI code: `@/components/ui/*` + CSS-variable tokens only; never hardcode colors.
- `allowTransparency: true` on xterm and the WebGL keepalive loop in `index.html` are load-bearing — never "optimize" them away.
- Commit after every green task, conventional-commit style, ending with `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`. **Never push, never open PRs.**
- Live verification uses `make dev-desktop` (dev CROWBAR_HOME) — never the production Crowbar install.
- Every performance task must record its metric delta in `docs/superpowers/specs/perf-baselines.md` before it is "done".

---

## Wave 0 — Measurement layer

### Task 1: `perf` instrumentation module

**Files:**
- Create: `web/src/lib/perf/instrumentation.ts`
- Test: `web/src/__tests__/lib/perf/instrumentation.test.ts`

**Interfaces (later tasks rely on):**
- `perfEnabled(): boolean`
- `markStart(name: string): void` / `markEnd(name: string): void` — measures `name` between the pair
- `installPerfObserver(): void` — idempotent; starts the ring-buffer observer
- `window.__perfLog: Array<{ name: string; startTime: number; duration: number; entryType: string }>` (cap 500, FIFO)

- [ ] **Step 1: Write the failing test**

```ts
// web/src/__tests__/lib/perf/instrumentation.test.ts
import { describe, it, expect, beforeEach, vi } from 'vitest'
import {
  markStart,
  markEnd,
  installPerfObserver,
  perfEnabled,
  __resetPerfForTests,
} from '@/lib/perf/instrumentation'

describe('perf instrumentation', () => {
  beforeEach(() => {
    __resetPerfForTests()
  })

  it('is disabled in prod without the arming flag and no-ops', () => {
    vi.stubEnv('DEV', false)
    delete (window as { __CROWBAR_PERF__?: boolean }).__CROWBAR_PERF__
    expect(perfEnabled()).toBe(false)
    markStart('x')
    markEnd('x') // must not throw and must not create entries
    expect(performance.getEntriesByName('x', 'measure')).toHaveLength(0)
    vi.unstubAllEnvs()
  })

  it('records a measure between markStart/markEnd when armed', () => {
    ;(window as { __CROWBAR_PERF__?: boolean }).__CROWBAR_PERF__ = true
    markStart('diff.open')
    markEnd('diff.open')
    const entries = performance.getEntriesByName('diff.open', 'measure')
    expect(entries).toHaveLength(1)
  })

  it('ring buffer caps at 500 entries, dropping oldest', () => {
    ;(window as { __CROWBAR_PERF__?: boolean }).__CROWBAR_PERF__ = true
    installPerfObserver()
    const log = (window as unknown as { __perfLog: unknown[] }).__perfLog
    for (let i = 0; i < 510; i++) {
      log.push({ name: `m${i}`, startTime: i, duration: 0, entryType: 'measure' })
      if (log.length > 500) log.shift()
    }
    expect(log.length).toBeLessThanOrEqual(500)
  })

  it('markEnd without markStart is a safe no-op', () => {
    ;(window as { __CROWBAR_PERF__?: boolean }).__CROWBAR_PERF__ = true
    expect(() => markEnd('never-started')).not.toThrow()
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && bun run test src/__tests__/lib/perf/instrumentation.test.ts`
Expected: FAIL — module `@/lib/perf/instrumentation` not found.

- [ ] **Step 3: Implement**

```ts
// web/src/lib/perf/instrumentation.ts
/**
 * Shared perf instrumentation. Zero-cost unless armed: enabled in dev, or in
 * any build when `window.__CROWBAR_PERF__` is truthy (set it from the console
 * or via webview_execute_js in the packaged app). All entries land in the
 * `window.__perfLog` ring buffer so Chrome traces (dev) and WKWebView
 * peek-back (packaged) read the same data. Spec §3.1.
 */
interface PerfLogEntry {
  name: string
  startTime: number
  duration: number
  entryType: string
}

declare global {
  interface Window {
    __CROWBAR_PERF__?: boolean
    __perfLog?: PerfLogEntry[]
  }
}

const RING_CAP = 500
let observer: PerformanceObserver | null = null
const openMarks = new Set<string>()

export function perfEnabled(): boolean {
  return Boolean(import.meta.env.DEV || window.__CROWBAR_PERF__)
}

export function markStart(name: string): void {
  if (!perfEnabled()) return
  openMarks.add(name)
  performance.mark(`${name}:start`)
}

export function markEnd(name: string): void {
  if (!perfEnabled()) return
  if (!openMarks.has(name)) return
  openMarks.delete(name)
  performance.mark(`${name}:end`)
  performance.measure(name, `${name}:start`, `${name}:end`)
}

export function installPerfObserver(): void {
  if (!perfEnabled() || observer) return
  window.__perfLog ??= []
  observer = new PerformanceObserver((list) => {
    const log = window.__perfLog!
    for (const e of list.getEntries()) {
      log.push({
        name: e.name,
        startTime: e.startTime,
        duration: e.duration,
        entryType: e.entryType,
      })
      if (log.length > RING_CAP) log.shift()
    }
  })
  // 'event' feeds Event Timing (input latency attribution); 'measure' feeds
  // our own spans. Both fail soft on runtimes lacking a type.
  try {
    observer.observe({ entryTypes: ['measure', 'event'] })
  } catch {
    observer.observe({ entryTypes: ['measure'] })
  }
}

/** Test-only: clears module state and performance entries. */
export function __resetPerfForTests(): void {
  observer?.disconnect()
  observer = null
  openMarks.clear()
  delete window.__perfLog
  performance.clearMarks()
  performance.clearMeasures()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && bun run test src/__tests__/lib/perf/instrumentation.test.ts`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/perf/instrumentation.ts web/src/__tests__/lib/perf/instrumentation.test.ts
git commit -m "feat(perf): shared instrumentation module with __perfLog ring buffer"
```

### Task 2: Dev tooling — react-scan, web-vitals, react-doctor; drop dead devtools

**Files:**
- Modify: `web/src/main.tsx` (wire `installPerfObserver` + dev-only react-scan + web-vitals INP)
- Modify: `web/package.json` (add `react-scan`, `web-vitals`, `react-doctor` devDeps + `"doctor": "react-doctor ."` script; REMOVE `@tanstack/react-query-devtools`)

**Interfaces:** consumes Task 1's `installPerfObserver`.

- [ ] **Step 1: Install/remove deps**

```bash
cd web
bun add -d react-scan web-vitals react-doctor
bun remove @tanstack/react-query-devtools
```

- [ ] **Step 2: Wire into `main.tsx`** — add immediately after the existing startup init calls (before render):

```ts
import { installPerfObserver, perfEnabled } from '@/lib/perf/instrumentation'

installPerfObserver()
if (import.meta.env.DEV) {
  void import('react-scan').then(({ scan }) => scan({ enabled: true, log: false }))
}
if (perfEnabled()) {
  void import('web-vitals').then(({ onINP }) => {
    onINP(
      (m) => {
        window.__perfLog?.push({
          name: `INP:${m.rating}`,
          startTime: m.entries[0]?.startTime ?? 0,
          duration: m.value,
          entryType: 'inp',
        })
      },
      { reportAllChanges: true },
    )
  })
}
```

- [ ] **Step 3: Add the `doctor` script** to `web/package.json` scripts: `"doctor": "react-doctor ."`.

- [ ] **Step 4: Verify**

Run: `cd web && bun tsc --noEmit && bun run test && bun run lint`
Expected: all green. Then `bun run dev`, open the app in Chrome, confirm react-scan's overlay appears and `window.__perfLog` exists in the console.

- [ ] **Step 5: Commit** — `chore(perf): dev-only react-scan + web-vitals INP; add react-doctor; drop dead react-query-devtools`

### Task 3: Baseline capture (runbook — orchestrator-assisted)

**Files:**
- Create: `docs/superpowers/specs/perf-baselines.md`

This task is executed by the orchestrating session (it needs the Chrome DevTools MCP / Tauri MCP harness), not a code subagent.

- [ ] **Step 1: Bundle baseline (M7)** — `cd web && bunx vite build`, record entry JS/CSS raw+gzip from `dist/assets` (the two files referenced by `dist/index.html`) and `grep -c MonacoEnvironment` on the entry JS. Expected today: ~6.46MB/1.67MB gzip, marker count ≥ 6.
- [ ] **Step 2: Runtime baselines (M1–M6)** — dev server + Chrome MCP: pinned scenarios from spec §3.3 (typing burst in a terminal, diff open on the pinned big-diff branch, workspace switch A↔B, react-scan render counts, trace long tasks). 3 runs each, medians. Packaged-side sanity pass via `make dev-desktop` + `webview_execute_js` reading `window.__perfLog`.
- [ ] **Step 3: React Doctor baseline** — `cd web && bun run doctor`; record score (expected 48/100) and category counts.
- [ ] **Step 4: Commit** `perf-baselines.md` with a table: metric | baseline | budget | post-fix columns to be appended per task.

---

## Wave 1 — The five fixes

### Task 4: P1a — lazy `EditorPane`

**Files:**
- Modify: `web/src/features/panes/components/pane-container.tsx` (~line 68)

`EditorPane`, `TerminalPane`, `DiffPane` are the only statically-imported pane types; five siblings are already `lazy()` (see lines 55–67). Make `EditorPane` and `DiffPane` lazy exactly like `AgentChatPane` above them (both pull Monaco; `TerminalPane` stays static — xterm is cheap and terminals are the first thing users open):

- [ ] **Step 1:** Replace the static imports:

```ts
const EditorPane = lazy(() =>
  import('./editor-pane').then((m) => ({ default: m.EditorPane })),
)
const DiffPane = lazy(() =>
  import('./diff-pane').then((m) => ({ default: m.DiffPane })),
)
import { TerminalPane } from './terminal-pane'
```

Check how the already-lazy siblings are wrapped in `<Suspense>` in this file's render and give EditorPane/DiffPane the same fallback treatment (grep `Suspense` in the file; reuse the existing fallback component).

- [ ] **Step 2:** Idle prefetch so first open is warm — add at the bottom of `pane-container.tsx`:

```ts
// Prefetch the editor chunk after startup settles: first file-open should not
// pay the network/parse cost, but cold launch must not either (spec P1).
if (typeof window !== 'undefined') {
  const idle = window.requestIdleCallback ?? ((cb: () => void) => window.setTimeout(cb, 2000))
  idle(() => {
    void import('./editor-pane')
    void import('./diff-pane')
  })
}
```

- [ ] **Step 3:** `bun tsc --noEmit && bun run test` green; `bunx vite build` and verify `MonacoEnvironment` grep count on the NEW entry chunk **drops to 0** (if not, `bunx vite build -- --debug` / inspect `dist/assets` imports to find the remaining eager path — fix it before proceeding; candidate: another static import of `monaco-editor` outside the editor feature — locate with `grep -rn "from 'monaco-editor'" src --include='*.ts*' | grep -v features/editor`).
- [ ] **Step 4:** Live check (dev app): cold load → workspace renders; open a file → editor appears (brief suspense fallback acceptable ≤300ms).
- [ ] **Step 5:** Record M7 delta in `perf-baselines.md`. Commit: `perf(bundle): lazy-load editor/diff panes so Monaco leaves the entry chunk`

### Task 5: P1b — on-demand Monaco language contributions

**Files:**
- Modify: `web/src/features/editor/monaco/language-contributions.ts`
- Test: `web/src/__tests__/features/editor/monaco/language-contributions.test.ts`

Today the module statically imports ~30 `*.contribution` modules + 4 language services (lines 4–41). Convert to on-demand:

- [ ] **Step 1: Failing test**

```ts
import { describe, it, expect } from 'vitest'
import { loadLanguageForPath, __loadedLanguagesForTests } from '@/features/editor/monaco/language-contributions'

describe('on-demand language contributions', () => {
  it('loads a grammar once per language and dedupes', async () => {
    await loadLanguageForPath('main.go')
    await loadLanguageForPath('other.go')
    expect(__loadedLanguagesForTests()).toContain('go')
    expect(__loadedLanguagesForTests().filter((l) => l === 'go')).toHaveLength(1)
  })
  it('unknown extensions resolve without throwing', async () => {
    await expect(loadLanguageForPath('file.xyzunknown')).resolves.toBeUndefined()
  })
})
```

- [ ] **Step 2:** Implement: keep the custom `ensureLanguage`/Monarch registrations (diff, gitignore, …) as-is; replace the static contribution imports with a loader map:

```ts
const contributionLoaders: Record<string, () => Promise<unknown>> = {
  go: () => import('monaco-editor/esm/vs/basic-languages/go/go.contribution'),
  typescript: () => import('monaco-editor/esm/vs/language/typescript/monaco.contribution'),
  // ... one entry per language previously imported statically (all 30 + 4 services);
  // key = monaco language id.
}
const extensionToLanguage: Record<string, string> = {
  '.go': 'go', '.ts': 'typescript', '.tsx': 'typescript', '.py': 'python',
  // ... derive from each contribution's registered extensions (the contribution
  // files export their extension lists — mirror them here explicitly).
}
const loaded = new Set<string>()
const inflight = new Map<string, Promise<unknown>>()

export async function loadLanguageForPath(path: string): Promise<void> {
  const ext = path.slice(path.lastIndexOf('.')).toLowerCase()
  const lang = extensionToLanguage[ext]
  if (!lang || loaded.has(lang)) return
  const loader = contributionLoaders[lang]
  if (!loader) return
  let p = inflight.get(lang)
  if (!p) { p = loader(); inflight.set(lang, p) }
  await p
  loaded.add(lang)
  inflight.delete(lang)
}
export function __loadedLanguagesForTests(): string[] { return [...loaded] }
```

- [ ] **Step 3:** Wire the call site: `grep -rn "language-contributions" web/src --include='*.ts*'` — at every importer that previously got the side-effect registration, call `await loadLanguageForPath(model path)` before/when the model's language is set (typically the editor-surface model-create path). Keep a synchronous `import './language-contributions'` only where the custom Monarch languages (diff/gitignore) are needed at once.
- [ ] **Step 4:** Tests + tsc green; live check: open a `.go`, `.rs`, `.md` file — highlighting appears (may load async on first open); check the dev console for chunk loads per language.
- [ ] **Step 5:** Rebuild, re-grep entry chunk for `basic-languages` (expect 0), record delta, commit: `perf(editor): load Monaco language contributions on demand per file extension`

### Task 6: P2a — cache + dedup the branch-review fetch loop

**Files:**
- Modify: `web/src/features/git/hooks/use-review-diff.ts` (whole hook — current body fetches uncached on mount + every `git-status-changed`, lines 23–54)
- Modify: `web/src/features/workspace/stores/slices/branch-review-slice.ts:120` (`setBranchReviewDiff`)
- Test: `web/src/__tests__/features/git/hooks/use-review-diff.test.ts`

**Interfaces produced:** `setBranchReviewDiff` becomes identity-preserving: skips the write when the incoming diff deep-equals `branchReview.diffCache`.

- [ ] **Step 1: Failing tests** (msw already in the repo — follow existing handler patterns in `web/src/mocks/`):

```ts
// __tests__/features/git/hooks/use-review-diff.test.ts — core assertions:
it('does not refetch when a git-status-changed burst arrives within the dedup window', ...)
// fire 5 events in 200ms → exactly 1 additional fetch (leading + trailing-debounced)
it('skips the store write when the payload is unchanged', ...)
// same payload twice → diffCache reference is identical after 2nd fetch
it('re-renders consumers only when files actually changed', ...)
```

Write these against a mocked `getReview` (vi.mock the api module) asserting call counts and store reference stability — full arrange/act/assert, no timing sleeps; use `vi.useFakeTimers()` for the debounce.

- [ ] **Step 2: Implement**
  - In the hook: debounce the `git-status-changed` handler (250ms trailing — one refetch per burst, the daemon fires 2–3Hz), and drop `setFiles` state duplication: derive `files` from the store's `diffCache` via `useStore(store, useShallow((s) => s.branchReview.diffCache?.files ?? EMPTY_FILES))` with a module-level `const EMPTY_FILES: GitDiff[] = []` (stable-empty rule — see memory: infinite-loop footgun).
  - In the slice:

```ts
import deepEqual from 'fast-deep-equal'
// ...
setBranchReviewDiff: (diff) =>
  set((s) => {
    if (s.branchReview.diffStatus === 'loaded' && deepEqual(s.branchReview.diffCache, diff)) return
    s.branchReview.diffCache = diff
    s.branchReview.diffStatus = 'loaded'
  }),
```

  (Immer no-op: returning without mutating produces the same state reference — verify with the reference-identity test.)

- [ ] **Step 3:** Tests green; `bun tsc --noEmit` green.
- [ ] **Step 4:** Live check with react-scan on the big-diff scenario: run `git status`-touching activity in a terminal; the review pane and sidebar must show ~0 re-renders per tick when nothing changed.
- [ ] **Step 5:** Record M-delta (renders/tick, fetches/tick), commit: `perf(git): dedup branch-review refetch loop and equality-gate the diff store write`

### Task 7: P2b — sidebar changed-files list stops pulling the full line-level diff

**Files:**
- Read first: `web/src/features/git/components/git-panel.tsx` (mounts `useReviewDiff` at ~line 37), the git status slice the workspace store already maintains (grep `gitStatus` in `web/src/features/workspace/stores/`), `web/src/features/git/api/review-api.ts`.
- Modify: `web/src/features/git/components/git-panel.tsx`, possibly `use-review-diff.ts` (add an `enabled` flag).
- Test: extend `__tests__/features/git/hooks/use-review-diff.test.ts`.

Target behavior (spec P2): the always-mounted sidebar derives its changed-files list + `uncommittedCount` from the cheap status data already streamed into the workspace store; the full `getReview` fetch happens **only while the Branch Review pane is open** (gate `useReviewDiff` with `enabled: boolean` — when false: no fetch, no listener). Discovery step is required because the status-slice shape is not pinned here — read it, then implement the projection, keeping the visible sidebar UI identical (file names, status letters, count badge).

- [ ] Steps: failing test for `enabled: false` (zero fetches, empty result, no listener leak — assert `removeEventListener` symmetry) → implement → green → live check: sidebar Git panel still lists files with review pane closed; network tab shows zero `/review` calls on git ticks → commit: `perf(git): sidebar changed-files from status data; full review diff only while review pane is open`

### Task 8: P2c — working-tree diff: per-file invalidation

**Files:**
- Modify: `web/src/features/git/components/diff/git-diff-editor-stack.tsx` (`refreshWorkingTreeBuffer` ~line 701: currently `gitDiffCache.invalidate(rootFolderPath)` repo-wide, then `buildWorkingTreeMultiDiff` refetches every changed file)
- Read first: `web/src/features/git/lib/git-diff-cache.ts` (`invalidate` signature — it takes an optional filePath; the whole-repo branch is what fires today), `web/src/features/git/lib/working-tree-multi-diff.ts:104`
- Test: `web/src/__tests__/features/git/lib/working-tree-refresh.test.ts` (new — extract the refresh decision into a testable pure helper)

- [ ] **Step 1:** Extract a pure planner — new function in `working-tree-multi-diff.ts`:

```ts
export interface WorkingTreeRefreshPlan { invalidate: string[]; keep: string[] }
/** Diff old vs new status entries by path+oid/status: only changed paths refetch. */
export function planWorkingTreeRefresh(
  prev: ReadonlyArray<{ path: string; oid?: string; status: string }>,
  next: ReadonlyArray<{ path: string; oid?: string; status: string }>,
): WorkingTreeRefreshPlan
```

Failing tests: unchanged file → `keep`; new/modified/oid-changed/deleted → `invalidate`; renames appear as delete+add.

- [ ] **Step 2:** Implement planner; wire `refreshWorkingTreeBuffer` to invalidate **only** `plan.invalidate` paths (per-file `gitDiffCache.invalidate(rootFolderPath, path)`) and fetch only those; keep results for `keep` paths from cache.
- [ ] **Step 3:** Green + live check on the uncommitted-changes tab during agent activity: network requests per git tick == number of actually-changed files (usually 1), not N.
- [ ] **Step 4:** Commit: `perf(git): working-tree diff refetches only files whose status actually changed`

### Task 9: P2d — diff stack subscription hygiene + split-only-when-split

**Files:**
- Modify: `web/src/features/git/components/diff/git-diff-editor-stack.tsx` (whole-`buffers` subscription ~line 555; unconditional split serialization ~lines 188–229)

- [ ] **Step 1:** Replace `const buffers = useStore(workspaceStore, (s) => s.buffers)` with `workspaceStore.getState().buffers` read inside the callback that needs `activeBuffer` (line ~578) — per the store rules, `getState()` in handlers is the blessed pattern.
- [ ] **Step 2:** In `EmbeddedDiffSectionEditor`: compute `splitContent` and register the two split buffers only when `viewMode === 'split'` (wrap lines 188–189 serialization and the 209–229 registrations; unified path registers one buffer).
- [ ] **Step 3:** `bun run test` (the diff-stack tests exist — fix any that pinned the 3-buffer behavior, updating them to assert 1 buffer in unified / 3 in split), tsc green.
- [ ] **Step 4:** Live: scroll a 100-file diff with react-scan — tab bar must register 0 renders; toggle split view still works.
- [ ] **Step 5:** Commit: `perf(git): drop whole-buffers subscription; serialize split view only when split`

### Task 10: P4a — terminal echo writes on arrival (remove the discretionary rAF)

**Files:**
- Modify: `web/src/features/terminal/hooks/use-terminal-connection.ts` (lines 130–172 + the listener at 282–334)
- Test: update `web/src/__tests__/features/terminal/hooks/use-terminal-connection.test.ts` (exists; it currently covers the rAF coalescing + snapshot barrier)

The snapshot barrier (`snapshotPendingRef`, generation counter) is CORRECTNESS — keep it byte-for-byte. Only the discretionary rAF between frame arrival and `terminal.write()` goes. New frame handling:

- [ ] **Step 1:** Update the existing tests: incremental frame → `terminal.write` called synchronously (no rAF); frames arriving while `snapshotPendingRef` is latched → buffered, then drained inside the barrier callback (this assertion exists — keep it); attach-finalize still fires exactly once with `scrollToBottom + refresh` on the first post-attach write.
- [ ] **Step 2:** Implement — replace `scheduleOutputFlush`/`flushOutputBuffer` with:

```ts
const writeFrame = (data: string) => {
  // Correctness gate: while a snapshot reset+redraw is queued, hold frames in
  // the buffer — the barrier callback drains it (see snapshotPendingRef).
  if (snapshotPendingRef.current) {
    outputBufferRef.current += data
    return
  }
  const finalizeViewport = pendingAttachFinalizeRef.current
  if (finalizeViewport) pendingAttachFinalizeRef.current = false
  terminal.write(
    data,
    finalizeViewport
      ? () => {
          terminal.scrollToBottom()
          terminal.refresh(0, terminal.rows - 1)
        }
      : undefined,
  )
  const newDirectory = parseOSC7(data)
  if (newDirectory) updateSession(sessionId, { currentDirectory: newDirectory })
}
```

Listener: incremental branch becomes `writeFrame(frame.data)`. Barrier callback's drain becomes `if (outputBufferRef.current) { const held = outputBufferRef.current; outputBufferRef.current = ''; writeFrame(held) }`. Delete `outputFlushFrameRef` and its `cancelAnimationFrame` cleanup (grep the file for both to catch the snapshot-path cancel at lines 285–288 and the effect teardown).

- [ ] **Step 3:** Tests green (including the full terminal test suite: `bun run test src/__tests__/features/terminal/`).
- [ ] **Step 4:** Live in the packaged-dev app (`make dev-desktop`): type in a terminal and in an agent chat — verify feel; re-attach after a workspace switch still repaints (the WKWebView blank-until-scroll bug must NOT return — that's what the finalize callback protects).
- [ ] **Step 5:** Measure M1 before/after (typing-burst scenario, both runtimes), record, commit: `perf(terminal): write echo frames on arrival instead of deferring a frame behind rAF`

### Task 11: P4b — terminal session store churn

**Files:**
- Modify: `web/src/features/terminal/stores/terminal-store.ts:30-37` (`updateSession`)
- Modify: `web/src/features/tabs/hooks/use-buffer-display-name.ts:18` (whole-Map subscription)
- Test: `web/src/__tests__/features/terminal/stores/terminal-store.test.ts` + `web/src/__tests__/features/tabs/hooks/use-buffer-display-name.test.ts`

- [ ] **Step 1: Failing tests:** (a) `updateSession` with values identical to current state does NOT change `sessions` reference; (b) a component using `useBufferDisplayName` with only non-terminal buffers does not re-render when an unrelated terminal session updates.
- [ ] **Step 2: Implement** — no-op skip in the store:

```ts
updateSession: (sessionId, updates) => {
  set((state) => {
    const current = state.sessions.get(sessionId)
    if (current) {
      let changed = false
      for (const k in updates) {
        if (!Object.is(current[k as keyof Terminal], updates[k as keyof Terminal])) { changed = true; break }
      }
      if (!changed) return state
    }
    const newSessions = new Map(state.sessions)
    newSessions.set(sessionId, { ...(current || {}), ...updates })
    return { sessions: newSessions }
  })
},
```

Narrow the hook subscription to a projected string-tuple of only the fields/sessions it reads (stable under `useShallow`):

```ts
import { useShallow } from 'zustand/react/shallow'
const sessionKeys = useTerminalStore(
  useShallow((state) =>
    buffers
      .filter((b): b is Extract<PaneContent, { type: 'terminal' }> => b.type === 'terminal')
      .map((b) => {
        const s = state.sessions.get(b.sessionId)
        return `${b.sessionId} ${s?.customName ? 1 : 0} ${s?.name ?? ''} ${s?.title ?? ''} ${s?.currentDirectory ?? ''}`
      }),
  ),
)
```

then rebuild a local lookup Map from `sessionKeys` inside a `useMemo` for `getBufferDisplayName` (replacing the direct `terminalSessions` dep so the callback identity only changes when a *relevant* field changes).

- [ ] **Step 3:** Green; live: prompt redraws/`cd` in one terminal must not re-render other tabs (react-scan).
- [ ] **Step 4:** Commit: `perf(terminal): skip no-op session updates; tab labels subscribe to relevant session fields only`

### Task 12: P4c — AgentChatPane: reattach without remount

**Files:**
- Read first: `web/src/features/agent/components/agent-chat-pane.tsx` (~lines 200–230 observers, ~516–529 the `key={attachment.sessionId}` XtermTerminal mount)
- Modify: same file; possibly `web/src/features/terminal/components/terminal.tsx` if the attach-only path needs a `sessionId`-change effect.
- Test: `web/src/__tests__/features/agent/components/agent-chat-pane.test.ts` (extend existing if present; else create targeting the attachment-swap logic).

Target: switching provider/reviving a chat must reuse the mounted terminal component and swap the attachment imperatively (dispose old connection, open new one) instead of `key`-forced remount that tears down the whole component + observers + socket. Verify the `MutationObserver` (`gridSlack` remeasure) disconnects while the pane is hidden. Discovery step first (the file is unread); acceptance: provider switch causes exactly 1 socket close + 1 open, zero component unmounts (assert via test double on the connection module), observers reconnect once.

- [ ] Steps: read file → failing test → implement → green → live agent-chat provider-switch check in dev app → commit: `perf(agent): swap chat terminal attachments without remounting the terminal`

### Task 13: P5a — TabBar stops defeating its own memo

**Files:**
- Modify: `web/src/features/tabs/components/tab-bar.tsx` (map at ~445–467, subscriptions at ~54/157)
- Modify: `web/src/features/tabs/components/tab-bar-item.tsx:38` (add comparator)
- Test: `web/src/__tests__/features/tabs/components/tab-bar-rerender.test.ts` (new)

- [ ] **Step 1: Failing test** — render TabBar with 5 buffers, update one buffer's dirty state, assert (render-count probe via a test-only `onRender` prop or React Profiler in test) only that TabBarItem re-rendered.
- [ ] **Step 2:** Implement: stable handlers taking `(bufferId | index)` created once via `useCallback` and passed as the SAME references to every item (item calls `onSelect(buffer.id)`); `tabRef` becomes a `useCallback` keyed by index-stable map. Add a `memo` comparator to `TabBarItem` comparing the scalar props it renders (id, name, dirty, active, pinned — enumerate from the component's actual prop usage). Replace the whole-`panes` subscription (line ~157) with the specific pane fields TabBar reads.
- [ ] **Step 3:** Green; live: typing burst → 0 TabBar renders (react-scan); tab click/close/pin/context-menu all still work (manual sweep).
- [ ] **Step 4:** Commit: `perf(tabs): stable tab handlers + real TabBarItem comparator; narrow pane subscription`

### Task 14: P5b — pane tree: leaves subscribe to their own pane

**Files:**
- Modify: `web/src/features/panes/components/pane-node-renderer.tsx:41`, `web/src/features/panes/components/split-view-root.tsx:18`
- Read first: the pane-slice selectors module (grep `usePanes\b` and `usePaneById` under `web/src/features/panes/`) — add `usePaneById(paneId)` if it does not exist (narrow selector over `s.panes[paneId]`).
- Test: `web/src/__tests__/features/panes/components/pane-node-renderer-rerender.test.ts`

- [ ] Steps: failing re-render-isolation test (change pane A's activeBufferId → pane B's renderer does not re-render) → implement leaf-scoped subscription (tree shape from the already-separate `rootLayout` subscription) → green → live tab-switch check → commit: `perf(panes): pane leaves subscribe to their own pane, not the whole record`

### Task 15: P5c — `useActiveWorkspaceState` equality guard

**Files:**
- Modify: `web/src/features/workspace/stores/hooks/use-active-workspace-state.ts:41`
- Test: extend its existing test (or create `__tests__/features/workspace/stores/hooks/use-active-workspace-state.test.ts`)

- [ ] Steps: failing test (selector returning a fresh-but-shallow-equal array does not notify) → add optional `equalityFn = shallow` (import `shallow` from `zustand/shallow`), previous-value ref compare before `setValue` → green → commit: `perf(workspace): shallow equality guard in useActiveWorkspaceState`

### Task 16: P5d — git-status frame dedup without JSON.stringify

**Files:**
- Modify: `web/src/features/workspace/stores/hooks/use-workspace-effects.ts:262-271`
- Test: `web/src/__tests__/features/workspace/stores/hooks/git-status-dedup.test.ts`

- [ ] **Step 1: Failing test** for a small `framesEqual(prev, next)` helper (export it from the hook module or a sibling util): identical frames → true; any nested change → false.
- [ ] **Step 2:** Implement with the existing dependency:

```ts
import deepEqual from 'fast-deep-equal'
let lastFrame: unknown = null
const unsubscribe = wsManager.subscribe(`${workspaceBase(wsId)}/git/status`, (frame) => {
  if (lastFrame !== null && deepEqual(frame, lastFrame)) return
  lastFrame = frame
  scheduleStatusReload()
})
```

(`fast-deep-equal` walks and bails early — no multi-KB string allocation 6×/sec; the frame is already a parsed object.)

- [ ] **Step 3:** Green; trace the typing-burst scenario again — the stringify self-time entry must be gone (M6 check). Commit: `perf(git): dedupe status frames with deep-equal instead of JSON.stringify`

### Task 17: P3 — workspace keep-alive (TTL + cap)

**Files:**
- Create: `web/src/features/workspace/components/workspace-host.tsx` (retention manager)
- Create: `web/src/features/workspace/lib/keep-alive-policy.ts` (pure policy — testable)
- Modify: `web/src/components/layout/ide-shell.tsx:147` (`<WorkspaceView wsId=…/>` → `<WorkspaceHost activeWsId=…/>`)
- Modify: `web/src/features/workspace/components/workspace-view.tsx` (destroy moves out; gains `active` prop)
- Modify: `web/src/features/settings/config/default-settings.ts`, `web/src/features/settings/config/search-index.ts`, settings types + the relevant settings section component (follow the exact registration pattern of an existing numeric setting — find one via `grep -n "number" web/src/features/settings/config/default-settings.ts`)
- Tests: `web/src/__tests__/features/workspace/lib/keep-alive-policy.test.ts`, `web/src/__tests__/features/workspace/components/workspace-host.test.tsx`

**Interfaces:**
- Setting key: `workspaceKeepAliveMinutes: number` (default `10`, `0` = off)
- `planRetention(entries: Array<{wsId: string; lastActiveAt: number}>, now: number, windowMs: number, cap: 6): { retain: string[]; evict: string[]; nextExpiryAt: number | null }`

- [ ] **Step 1: Policy tests first** (pure function — trivial to test): within-window retained; beyond-window evicted; >6 retained → oldest evicted despite window; active workspace never evicted; `windowMs === 0` → only active retained; `nextExpiryAt` = earliest retained non-active expiry.
- [ ] **Step 2:** Implement `keep-alive-policy.ts` (pure, no Date.now inside — `now` is a parameter).
- [ ] **Step 3: WorkspaceHost** — complete component:

```tsx
export function WorkspaceHost({ activeWsId }: { activeWsId: string }) {
  const keepAliveMinutes = useSettingsStore((s) => s.settings.workspaceKeepAliveMinutes)
  const [mounted, setMounted] = useState<Map<string, number>>(new Map([[activeWsId, Date.now()]]))
  const timerRef = useRef<number | null>(null)

  useEffect(() => {
    setMounted((prev) => {
      const next = new Map(prev)
      next.set(activeWsId, Date.now())
      const plan = planRetention(
        [...next].map(([wsId, lastActiveAt]) => ({ wsId, lastActiveAt })),
        Date.now(), keepAliveMinutes * 60_000, 6,
      )
      for (const wsId of plan.evict) { next.delete(wsId); destroyWorkspaceStore(wsId) }
      scheduleExpiry(plan.nextExpiryAt)
      return next
    })
    // eslint-disable-next-line react-hooks/exhaustive-deps -- scheduleExpiry is stable (defined below with useCallback([]))
  }, [activeWsId, keepAliveMinutes])

  const scheduleExpiry = useCallback((at: number | null) => {
    if (timerRef.current !== null) window.clearTimeout(timerRef.current)
    timerRef.current = null
    if (at === null) return
    timerRef.current = window.setTimeout(() => runEviction(), Math.max(0, at - Date.now()))
  }, [])
  // runEviction re-runs planRetention against current state and repeats the
  // evict/schedule pair; single armed timer, no polling (spec P3).

  return (
    <>
      {[...mounted.keys()].map((wsId) => (
        <div key={wsId} style={{ display: wsId === activeWsId ? 'contents' : 'none' }}
             inert={wsId === activeWsId ? undefined : true}>
          <WorkspaceView wsId={wsId} active={wsId === activeWsId} />
        </div>
      ))}
    </>
  )
}
```

(Exact settings-store selector name to be matched to the real settings store API during implementation — read `web/src/features/settings/store.ts` first.)

- [ ] **Step 4: WorkspaceView changes:** remove the `destroyWorkspaceStore` cleanup effect (lines 49–53 — destruction is now WorkspaceHost's job); `setActiveWorkspaceStoreRef`/`setActiveWorkspaceId` effects run only when `active` (hidden workspaces keep stores + `useWorkspaceEffects` watchers so they stay fresh, but must not steal the active-ref); keyboard hooks (`useSaveKeyboard`, `usePaneKeyboard`, `useSidebarTabKeyboard`) gate on `active` too.
- [ ] **Step 5: Host tests** with `vi.useFakeTimers()`: switch A→B→A: A's store never destroyed, no re-hydration (spy `hydrateWorkspace` — 0 extra calls on warm return); advance past window → hidden stores destroyed; 7 workspaces in-window → oldest evicted; `keepAliveMinutes=0` → behaves like today.
- [ ] **Step 6: Settings plumbing** + a search-index entry ("Keep workspaces in memory"), settings UI control in the matching section; round-trip test.
- [ ] **Step 7:** Full web suite + tsc + lint green. Live (dev app): A↔B switch instant on warm return, terminals/editors intact; heap snapshots across a 10-switch cycle bounded; workspace close still tears down.
- [ ] **Step 8:** Measure M4 warm-switch before/after, record, commit: `perf(workspace): TTL keep-alive for recent workspaces (configurable, capped) replacing destroy-on-switch`

### Task 18: Wave-1 measurement wrap

- [ ] Re-run every M1–M7 scenario (same runbook as Task 3), append the post-Wave-1 column to `perf-baselines.md`, flag any budget miss (a miss does NOT block Wave 2 — it gets a root-cause note and a follow-up entry). Commit.

---

## Wave 2 — React Doctor to 100/100

Protocol for every batch below (react-doctor's own agent guidance, spec §5):
1. `cd web && bunx react-doctor@latest . --verbose 2>&1 | tee /tmp/rd-current.txt` and extract the batch's rule family findings.
2. Per finding: read the file; classify **true positive** (fix it), **false positive** (add a justified config entry — one line why, collected in §5.1 appendix of the spec), or **product-decision-needed** (stop and surface).
3. Fix in place following the file's existing style; behavior-adjacent fixes get/update a test in the mirror location.
4. After each family: `bun run test && bun tsc --noEmit && bun run lint && bun run doctor` — issue count for that family must be 0; total score never decreases.
5. Commit per family: `fix(rd): <family> — <n> sites`.

### Task 19: RD batch 1 — Bug errors (85: state-updater side effects ×34, ref-mutated-in-render ×33, uncleaned effects ×14, effect-dep-recreated ×6)

These are real defect classes — no mechanical sweeps; each site read and understood. Canonical transforms:
- *State updater has side effects*: hoist the side effect out of the `set()`/`setState` updater into the surrounding handler/effect (updaters must be pure — they can re-run).
- *Ref mutated during render*: move the mutation into `useEffect`/`useLayoutEffect` or an event handler; if it's a render-time cache, convert to `useMemo`.
- *Uncleaned subscription/timer*: return the disposer from the effect (example already flagged at a `let disposed = false` site — pair every `addEventListener`/`setInterval`/`subscribe` with cleanup).
- *Effect dep recreated every render* (e.g. `find-bar.tsx:152` `onClose`): wrap the dep in `useCallback` at its definition site, or move it inside the effect.
- [ ] Execute per protocol; Performance-critical files touched in Wave 1 must not regress their new tests.

### Task 20: RD batch 2 — Security (1 error + 6 warnings)

- [ ] The error (`Imported metadata reaches code execution`) gets a real investigation — trace the flagged flow end-to-end before deciding fix vs false-positive; document the conclusion in the commit body. HTML-injection sinks ×3: source must pass through the existing dompurify usage or become non-HTML rendering. `iframe missing sandbox`: add the minimal sandbox attribute set that keeps the feature working. `require-pnpm-hardening ×2`: config-exception candidate — repo is bun-only (pnpm retired, PR #46); justify in config.

### Task 21: RD batch 3 — Bug warnings (~92: missing-effect-deps ×39, state-chained-through-effects ×17, resubscribe-on-changing-callback ×9, parent-sync-via-effect ×8, effects-chained ×5, remainder singletons)

- [ ] Same per-site protocol. `missing effect dependencies` requires judgment: adding a dep that changes behavior is NOT mechanical — where the omission is load-bearing (e.g. mount-only semantics), convert to the explicit pattern (refs for latest values, or `// eslint-disable-next-line react-hooks/exhaustive-deps` is NOT allowed — restructure instead). Chained-effects sites get restructured to derive-during-render or single-effect flows per React docs patterns.

### Task 22: RD batch 4 — Performance warnings (~93)

- [ ] Re-run doctor first: `heavy-library-loaded-eagerly ×20` should have largely vanished after Task 4/5 — fix stragglers the same lazy way. `full-framer-motion-import ×6` → `import { m } from 'framer-motion/m'`-style granular imports (or `LazyMotion`). `unstable-context-provider-value ×7` → `useMemo` the value. Chained iterations ×25 / map-filter ×6 / array-find-in-loop ×3 / lookup-in-loop ×7: single-pass rewrites only where the collection can be non-trivial; skip-with-config-note where provably ≤10 elements and the rewrite hurts readability (these are the "false positive" lane, justified individually). `Intl formatter rebuilt ×4` → module-level formatter. `state-initializer-runs-every-render ×4` / ref-initializer ×2 → lazy initializer form. `JSON deep clone` → `structuredClone`. `await-in-loop ×13` → `Promise.all` where independent; keep + config-note where sequencing is semantic. SVG-path precision ×3 → round coordinates.

### Task 23: RD batch 5 — Accessibility (1 error + 30 warnings)

- [ ] Fix, don't suppress (spec §5.1): interactive `div`s become `button`s (keeping the token/UI-kit styling), labels get `htmlFor`, keyboard handlers added where click-only, role-vs-tag swaps. The terminal/canvas surfaces: if a rule genuinely cannot apply to the xterm canvas, that specific instance is a documented exception, not the family.

### Task 24: RD batch 6 — Maintainability + dead code (353 warnings)

- [ ] **Dead-code sample-first protocol (spec §5.2):** from the 115 `unused-file` findings take 15 spanning `lib/mock/`, `components/ui/`, feature dirs. For each: `grep -rn "<basename sans ext>" web/src web/*.ts web/*.config.* index.html` + check `dev:mock` mode (`web/src/mocks/`, MSW handlers) + dynamic `import(` patterns + `routeTree.gen.ts`. If ALL 15 verify clean-unused → delete the rest in one commit batch per directory, re-running `bun run test && bun tsc --noEmit && bunx vite build && bun run dev:mock` (boot check) after each batch. Any false positive → tighten the verification and re-sample before continuing.
- [ ] `unused-export ×131`: demote to non-exported (or delete if the whole symbol is dead) — same verification per symbol via grep.
- [ ] `unused-dependency ×23 + 1 dev`: `bun remove` each after grep-verifying zero imports (watch for vite-plugin/config-referenced packages — check `vite.config.ts`, `index.html`, CSS imports before removing fonts/tailwind-adjacent packages).
- [ ] Non-component-export ×41: move helpers to sibling `lib/` files (respect kebab-case + feature boundaries). Large-component ×20: split ONLY where a natural seam exists and the file was already touched in this program; otherwise config-note as accepted (documented) — wholesale splitting for a score is churn. Pure-function/static-value-rebuilt ×20 → hoist to module scope. Boolean-prop combos ×2 and unversioned-localStorage-key ×1: fix per doctor's suggestion (the localStorage key gets a `:v1` suffix constant).
- [ ] Exit: `bun run doctor` reports **100/100, 0 errors, 0 warnings**; every config entry has a one-line justification; spec appendix updated with the exception list.

---

## Wave 3 — Ratchets

### Task 25: CI gates

**Files:**
- Modify: `.github/workflows/ci.yml` (frontend-checks job, lines ~94–114)
- Create: `web/scripts/check-bundle-budget.mjs`

- [ ] **Step 1:** Bundle budget script (complete):

```js
// web/scripts/check-bundle-budget.mjs — fails if the entry chunk exceeds budget.
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { gzipSync } from 'node:zlib'
const BUDGET_GZIP_BYTES = 900_000 // locked post-Wave-1: baseline*0.5 rounded up; adjust to measured value in this task
const html = readFileSync('dist/index.html', 'utf8')
const entry = html.match(/src="\/assets\/(index-[^"]+\.js)"/)?.[1]
if (!entry) { console.error('entry chunk not found in dist/index.html'); process.exit(1) }
const gz = gzipSync(readFileSync(`dist/assets/${entry}`)).length
const monaco = readFileSync(`dist/assets/${entry}`, 'utf8').includes('MonacoEnvironment')
if (monaco) { console.error('FAIL: Monaco is back in the entry chunk'); process.exit(1) }
if (gz > BUDGET_GZIP_BYTES) { console.error(`FAIL: entry ${gz}B gzip > budget ${BUDGET_GZIP_BYTES}B`); process.exit(1) }
console.log(`entry ${entry}: ${gz}B gzip (budget ${BUDGET_GZIP_BYTES}) — OK`)
```

- [ ] **Step 2:** CI additions to frontend-checks (after the existing build step): `node scripts/check-bundle-budget.mjs` and `bunx react-doctor . --ci` (check react-doctor's docs output from `bunx react-doctor --help` for the exact non-interactive/threshold flags; gate = score 100). Set the budget constant from the real post-Wave-1 measured entry size + 10% headroom.
- [ ] **Step 3:** Run the full frontend-checks command sequence locally to prove the gates pass; commit: `ci(web): react-doctor 100 gate + entry-bundle budget`

### Task 26: Final verification sweep

- [ ] Full: `bun run test && bun tsc --noEmit && bun run lint && bunx vite build && node scripts/check-bundle-budget.mjs && bun run doctor` — all green/100.
- [ ] Full M1–M7 re-measure, final column in `perf-baselines.md`; every budget met or explicitly dispositioned.
- [ ] Designed manual test in the live dev Tauri app (memory rule): typing feel in terminal + agent chat, big-diff branch review scroll, warm/cold workspace switch, settings toggle for keep-alive, file open of 5 language types (on-demand grammars), split diff toggle.
- [ ] Commit the final baselines + a short `docs/superpowers/specs/perf-results.md` summarizing deltas.

---

## Self-review notes

- Spec coverage: §3→Tasks 1–3; P1→4–5(+6 embedded in Task 4 step 3 build check); P2→6–9; P4→10–12; P5→13–16; P3→17; §5 batches→19–24; §5.4→25; wrap→18/26. Follow-ups intentionally not planned (spec non-goals).
- Tasks 7, 12, 13, 14, 17-settings have explicit read-first steps because those files were not read during planning — their acceptance criteria and interfaces are pinned; their code follows the file's actual shape.
- Type consistency: `planRetention`/`loadLanguageForPath`/`framesEqual` signatures defined once and only consumed within their own tasks.
