# Editor Decouple — production-grade, React-off-the-switch-path

**Date:** 2026-06-13
**Status:** Design — pending approval
**Scope:** The editor area only (pane editor surface, breadcrumb, find bar, LSP overlays, the Monaco host, the legacy diff path). **Sidebar is out of scope and must not change.** Builds on the retained-editor work (`docs/superpowers/specs/2026-06-13-editor-hosting-redesign-design.md`).

## 1. Problem (measured)

After the retained-editor refactor, an editor↔editor tab switch is **~63–105ms** (P0 was ~110ms). A-spike (2026-06-13, clean WKWebView) attributed the residual:

- Switch cost is **file-type independent** — switching to a JSON file costs the same (~63–68ms) as a TS file (~68ms). So it is **NOT** LSP/code-lens/TS-specific work.
- Cost settles over **3 React commits clustered at the end** of the switch; run-to-run variance (63→105ms) is scheduling/GC noise on top of a real reconciliation cost.
- The pure Monaco model swap is **15ms** (measured); the rest is **React reconciling the entire `CodeEditor` subtree** (breadcrumb, find bar, all overlays, all hooks) because they re-render on every `activeBufferId` change.

**Conclusion:** the lever is to stop re-rendering the editor subtree on a buffer switch. A surgical memoization does not touch this; only a full decouple does.

This is also a **code-quality** effort: `code-editor.tsx` (~580 lines) and `monaco-editor.tsx` (~1100 lines, dual managed/legacy paths) each do far too much. Production-grade = decompose into small, single-responsibility units with clear interfaces.

## 2. Target architecture (VSCode-faithful)

The per-pane editor region becomes a **stable React tree mounted once per pane** (keyed by `paneId`), living across buffer switches. A buffer switch must be: `manager.showBuffer(paneId, uri)` (imperative `setModel` + native view-state) **plus only the narrow leaf updates whose data actually changed** — never a subtree remount.

### 2a. Stable per-pane shell
`features/editor/components/editor-surface.tsx` — mounted once per `paneId`. Renders: the Monaco **slot** (manager-owned, already exists), and **stable containers** for the satellites. It does **not** key on `activeBufferId`; switching buffers does not unmount or re-render it.

### 2b. Imperative switch controller
A per-pane controller (hook or small module) subscribes to the pane's `activeBufferId` via a narrow store subscription and, on change, calls `manager.showBuffer(...)` and updates a single per-pane "active editor context" (current model, uri, filePath) held in a ref/event-emitter — **not** React state that re-renders the shell. Satellites read this context through narrow subscriptions.

### 2c. Satellites mount once per pane, update via narrow subscriptions
Breadcrumb, find bar, and the overlays (hover, completion, signature-help, code-lens, rename) each mount **once per pane** and subscribe **only** to the specific slice they render from:
- Breadcrumb → active file path (updates when path changes, i.e. on switch — a tiny leaf update, not a subtree).
- Find bar → find-visible + query state (independent of buffer switch).
- Overlays → cursor/position/LSP-result state for the active model; they idle when their data doesn't change. They attach to the **retained** editor and retarget on model swap rather than remounting.

The point: on a switch, React reconciles a handful of small leaves (e.g. breadcrumb text), not the entire editor subtree. Target: **≤20ms/switch**, ≤1 commit of meaningful work.

### 2d. LSP/rich-services as a per-pane controller
Move LSP wiring (`useLspIntegration`, `useCodeLens`, `useRename`) out of the per-buffer render path into a **per-pane controller** that attaches to the retained editor + current model and reacts to model swaps. `didOpen`/`didClose` and code-lens fetches are scheduled off the critical switch frame (idle callback / microtask), so they never block the switch.

### 2e. Split the Monaco host (`monaco-editor.tsx`)
Decompose into focused files:
- `monaco-managed-surface.ts(x)` — the managed (paneId) path: slot mount + controller wiring (no creation; uses `EditorManager`).
- `monaco-diff-editor.tsx` — the legacy per-instance path used by the git-diff viewer, cleanly isolated (addresses #2's dual-path mega-file).
- Shared bits (create options, language id) already live in `lib/monaco-adapters.ts` / utils.

## 3. Issue #2 — two architectures

The diff viewer legitimately needs per-instance editors that share a file path across split left/right (breaks the registry's one-model-per-URI keying). **Decision:** do NOT force the diff viewer onto the file-keyed registry. Instead: (a) cleanly **isolate** the legacy diff editor into its own focused component (`monaco-diff-editor.tsx`) so there is no shared dual-path mega-file, and (b) migrate the diff viewer's manual view-state usage so the **manual `EditorViewStateCacheManager` can be deleted** — the diff editor keeps its own local view-state (it doesn't need cross-pane sharing). End state: managed panes on the manager+native view-state; diff editor a small self-contained unit; the legacy manual cache **gone**. Two *components*, one *bookkeeping model* (native), clean boundary.

## 4. Issue #4 — load testing (acceptance, not a feature)

Verify on a **large file (≥5k lines) in a 2-pane split**: typing stays ≤16.7ms/frame with ~0 commits/keystroke; switch ≤20ms. This is a required acceptance gate, run in a clean window.

## 5. Issue #3 — the mysteries (resolve, don't rationalize)

During implementation, with clean-window profiling (no reloads mid-measure; instrument the manager + `performance.mark`s in the real code, not `querySelector`): (a) confirm the new switch path is ≤1 meaningful commit and explain any residual; (b) resolve the `getDomNode` mismatch — with the subtree rebuilt and a single stable slot, the editor's DOM node must be stable across a switch; assert it in a test/probe.

## 6. Issue #5 — methodology guardrails (process)

Baked into every measurement task: sole control of the webview (no other agent driving it); never `location.reload()` between capturing a reference and comparing it; verify any "process/interference" claim with `ps` before asserting it; attribute cost with in-code `performance.mark`s, not cross-reload DOM diffs.

## 7. What stays untouched
Sidebar; the `EditorManager`/`ModelRegistry`/`ContentSink` units (P1–P3, keep); the GPU layer-promotion resize fix; the pixel sash; terminals/web-viewers; multi-workspace isolation.

## 8. Risks
- Overlays (hover/completion/signature/code-lens/rename) are intricate and positioned relative to the editor — retargeting them imperatively without remount is the riskiest part. Mitigate: migrate one satellite at a time, each behind its own verification.
- LSP `didOpen`/`didClose` ordering when deferred off-frame — must not drop or duplicate document lifecycle. Test explicitly.
- Behavior parity across: completions, hover, go-to-def, rename, code-lens, find, breadcrumb, search highlights, preview-promote, dirty/save, large files, split panes, diff viewer, branch review.

## 9. Phasing (each behavior-preserving, gated, reviewed)
- **D0** — A spike ✅ (done; findings in §1).
- **D1** — Stable per-pane shell + imperative switch controller; move slot mounting into `editor-surface.tsx`; breadcrumb + find bar mount once per pane via narrow subscriptions. Gate: switch no longer re-renders breadcrumb/find subtree; measure.
- **D2** — Overlays mount once per pane, retarget on model swap (one overlay per sub-step: hover → completion → signature → code-lens → rename). Gate: each overlay behaves identically; switch cost drops toward ≤20ms.
- **D3** — LSP controller per pane; `didOpen`/`didClose`/code-lens scheduled off-frame. Gate: LSP features parity; switch frame not blocked by LSP.
- **D4** — Split `monaco-editor.tsx` into managed surface + isolated `monaco-diff-editor.tsx`; delete the manual `EditorViewStateCacheManager` (#2/#3). Gate: diff viewer + branch review parity; suite green.
- **D5** — Load test (#4) + resolve mysteries (#3) + full parity sweep + before/after report.

## 10. Acceptance (definition of done)
- Tab switch **≤20ms** (warm), ≤1 meaningful React commit; verified clean-window.
- Typing **~0 commits/keystroke** including large-file + 2-pane split.
- Every editor feature behaves exactly as today (full parity checklist in D5).
- `monaco-editor.tsx` and `code-editor.tsx` decomposed into focused single-responsibility files; manual view-state cache deleted; one bookkeeping model.
- tsc clean, lint clean, tests green; new units unit-tested.
- Sidebar unchanged.
