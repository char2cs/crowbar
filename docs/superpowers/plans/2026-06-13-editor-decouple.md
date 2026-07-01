# Editor Decouple Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development (fresh subagent + two-stage review per task — "perfect quality" bar). Steps use `- [ ]`.

**Goal:** Make an editor tab switch React-free — stable per-pane shell + imperative switch controller + satellites (breadcrumb/find/overlays) that mount once per pane and update via narrow subscriptions, so a switch is `setModel` + tiny leaf updates (≤20ms), not a ~65–105ms subtree reconciliation. Decompose the oversized editor files into focused units; delete the manual view-state cache.

**Architecture:** React renders a per-pane shell that does NOT key on `activeBufferId`. A per-pane controller subscribes narrowly to `activeBufferId` and drives `EditorManager.showBuffer` imperatively, publishing an "active editor context" (model/uri/filePath) via a small per-pane emitter. Satellites subscribe to only their slice. LSP lifecycle moves to a per-pane off-frame controller. Reference spec: `docs/superpowers/specs/2026-06-13-editor-decouple-design.md`.

**Tech Stack:** TS, React 19, Zustand (narrow selectors + `subscribe`), monaco-editor 0.55, Vitest (`web/src/__tests__/` mirror, `@/` imports), Tauri MCP (clean-window profiling — no reloads mid-measure, instrument via `performance.mark` + manager, never `querySelector` across reloads).

**Baseline:** switch ~63–105ms (generic React reconciliation, file-type independent); model swap 15ms; typing ~0.3 commits/keystroke. Target: switch ≤20ms / ≤1 meaningful commit; typing ~0 commits incl. large-file + split.

---

## File structure (target)
- `features/editor/components/editor-surface.tsx` — NEW stable per-pane host (slot + satellite containers; does not re-render on buffer switch).
- `features/editor/lib/active-editor-context.ts` — NEW per-pane emitter holding {paneId, uri, filePath, model, editor}; `subscribe(paneId, cb)`; published by the switch controller.
- `features/editor/hooks/use-pane-editor-controller.ts` — NEW per-pane controller: narrow `activeBufferId` subscription → `manager.showBuffer` + publish context. Replaces the per-buffer create/show effects in `monaco-editor.tsx`.
- `features/editor/components/satellites/` — breadcrumb, find-bar, and overlay wrappers reworked to mount once/pane + subscribe narrowly.
- `features/editor/components/monaco-diff-editor.tsx` — NEW isolated legacy per-instance editor for the git-diff viewer (extracted from `monaco-editor.tsx`).
- DELETE: `EditorViewStateCacheManager` + dead actions in `features/editor/stores/state-store.ts` once no consumer remains.
- Tests under `web/src/__tests__/features/editor/...` mirror.

---

## D1 — Stable shell + imperative switch controller + breadcrumb/find once-per-pane

### Task 1: active-editor-context emitter (TDD)
**Files:** Create `features/editor/lib/active-editor-context.ts` + test.
- [ ] **Step 1 — failing test** (`__tests__/features/editor/lib/active-editor-context.test.ts`): a `PaneEditorContext` (or `createActiveEditorRegistry()`) where `set(paneId, ctx)` then `subscribe(paneId, cb)` fires cb with latest ctx; multiple subscribers; unsubscribe stops calls; `get(paneId)` returns latest; setting an unchanged ctx (same uri) does not refire. Write assertions with vitest mocks.
- [ ] **Step 2** run → fail.
- [ ] **Step 3** implement a tiny per-pane pub/sub (Map<paneId, {ctx, listeners:Set}>), de-dupe by uri identity.
- [ ] **Step 4** run → pass.
- [ ] **Step 5** commit `feat(editor): per-pane active-editor context emitter`.

### Task 2: use-pane-editor-controller hook (TDD where possible + integration)
**Files:** Create `features/editor/hooks/use-pane-editor-controller.ts` + test; will be consumed by editor-surface in Task 3.
- [ ] **Step 1** Read `monaco-editor.tsx` managed mount+controller effects (`:518`, `:617`) and `code-editor.tsx` (how `activeBufferId`/`filePath`/`value` are derived) to capture exactly what the controller must do.
- [ ] **Step 2** failing test: given a fake manager + fake store, mounting the controller for `paneId` calls `manager.mountPane`; when the store's `activeBufferId` changes, it calls `manager.showBuffer(paneId, fileUri(path))` and publishes context to the emitter (assert via the emitter); unmount calls `manager.unmountPane`. (Use the workspace store test harness pattern already in the repo; if a hook test is awkward, extract the logic into a plain function `runSwitch(deps)` and unit-test that, with the hook a thin wrapper.)
- [ ] **Step 3** run → fail.
- [ ] **Step 4** implement: narrow `store.subscribe` to `state.panes[paneId].activeBufferId` (NOT a React render); on change → showBuffer + publish. mountPane on init, unmountPane on cleanup.
- [ ] **Step 5** run → pass. Commit `feat(editor): imperative per-pane editor switch controller`.

### Task 3: editor-surface stable shell
**Files:** Create `editor-surface.tsx`; modify `editor-pane.tsx` to render it; carve the slot + breadcrumb/find mounting out of `code-editor.tsx`.
- [ ] **Step 1** Build `editor-surface.tsx`: mounts once per `paneId` (parent must give it a stable key=paneId and NOT pass `activeBufferId` as a remount trigger). Renders: manager slot div (ref → `use-pane-editor-controller`), a stable Breadcrumb (subscribes to active path via narrow selector/emitter), a stable FindBar (subscribes to find UI state). Uses `use-pane-editor-controller`.
- [ ] **Step 2** Repoint `editor-pane.tsx` to render `<EditorSurface paneId .../>`; keep missing-file/error states. Ensure `pane-container.tsx` mounts the surface per pane (stable) — the editor branch no longer remounts on buffer change.
- [ ] **Step 3** Rework Breadcrumb + FindBar to read from narrow selectors/emitter so a buffer switch updates only their leaves.
- [ ] **Step 4** `bun run vitest run`, `bunx tsc --noEmit`, `bun run lint` green.
- [ ] **Step 5 (clean-window WKWebView)** Measure: tab switch no longer re-renders the breadcrumb/find subtree wholesale; capture commit count + frame. Record numbers.
- [ ] **Step 6** Commit `refactor(editor): stable per-pane editor surface; breadcrumb/find mount once per pane`.

> NOTE: After D1 the overlays may still be rendered the old way; D2 moves them. D1 must keep all behavior identical.

## D2 — Overlays mount once per pane, retarget on model swap (one per sub-step)
For EACH of: hover → completion → signature-help → code-lens → rename:
- [ ] Read the overlay component + its hook; identify what it reads (cursor/position/LSP result) and how it positions relative to the editor.
- [ ] Move it under `editor-surface` mounted once per pane; subscribe narrowly to its own state + the active-editor context; retarget (recompute position / rebind to current model) on model swap WITHOUT remount.
- [ ] Verify the feature behaves identically live (trigger hover/completion/etc.) + tests/tsc/lint green.
- [ ] Commit `refactor(editor): <overlay> mounts once per pane, retargets on model swap`.
Gate after all five: switch cost trends to ≤20ms; each overlay parity confirmed.

## D3 — LSP controller per pane, off-frame lifecycle
- [ ] Move `useLspIntegration`/`useCodeLens`/`useRename` wiring into a per-pane controller attached to the retained editor + current model, reacting to model swaps.
- [ ] Schedule `didOpen`/`didClose` + code-lens fetch off the critical frame (idle callback/microtask); guarantee document lifecycle ordering (no drop/dupe) — explicit test.
- [ ] Verify LSP parity (completions, hover, go-to-def, rename, code-lens) live; switch frame not blocked by LSP. Commit.

## D4 — Split monaco-editor.tsx + delete manual cache
- [ ] Extract the legacy per-instance path into `monaco-diff-editor.tsx`; the git-diff viewer (`git-diff-editor-*`) imports it. Managed surface logic stays in the managed-path module. No shared dual-path mega-file.
- [ ] Migrate the diff editor's view-state to local handling; once no consumer of `EditorViewStateCacheManager`/`setScrollForBuffer`/`getCachedViewState`/`restorePositionForFile` remains, DELETE them (grep to confirm zero importers).
- [ ] Verify diff viewer + branch review parity (unified + split diff). tsc/lint/tests green. Commit.

## D5 — Load test + resolve mysteries + parity sweep + report
- [ ] **Load:** large file (≥5k lines) in a 2-pane split — typing ≤16.7ms/frame, ~0 commits/keystroke; switch ≤20ms. (Generate/open a big file.)
- [ ] **#3 mysteries:** clean-window profile a switch; confirm ≤1 meaningful commit and explain residual; assert the editor DOM node is stable across a switch (no detach) via a probe/test.
- [ ] **Parity sweep:** open/close, split/unsplit, tab reorder + cross-pane drag, undo/redo, save, dirty, preview-promote, missing-file, multi-workspace, terminals/web-viewers, completions/hover/def/rename/code-lens/find/breadcrumb/search-highlights, diff + branch review, sidebar unchanged.
- [ ] **Report:** before/after table vs the ~65–105ms baseline. Commit.

---

## Self-review
- Spec §2a→D1/T3, §2b→D1/T2, §2c→D1/T3+D2, §2d→D3, §2e→D4, §3(#2)→D4, §4(#4)→D5, §3-mystery(#3)→D5, §5(#5)→guardrails in every measure step, §10 acceptance→D5 gates. Covered.
- Risk-ordered: emitter+controller (testable) → shell → overlays one-at-a-time (riskiest, isolated) → LSP → file split/cleanup → verify.
- Types consistent: `fileUri`, `EditorManager.{mountPane,showBuffer,closeBuffer,unmountPane,getRawEditor}`, active-editor-context `{set,get,subscribe}`.
- Later-phase literal code is authored at execution time against post-D1 reality (per the gated, profile-between-phases design) — intentional, not placeholder.
