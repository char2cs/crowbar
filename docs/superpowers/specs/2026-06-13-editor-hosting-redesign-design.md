# Editor-Hosting Redesign — VSCode-style retained editors

**Date:** 2026-06-13
**Status:** Design approved, pending spec review
**Scope:** The editor area only — editor groups (panes), tab strips, breadcrumb headers, Monaco panes, the splitter between panes. **The sidebar and its functionality are explicitly out of scope and must not change.**

---

## 1. Problem & motivation

Crowbar runs Monaco (the same engine VSCode ships) inside a single Tauri WKWebView. The engine is fine; the *hosting* diverges from VSCode in ways that cost performance and polish. The walk-away bar set by the product owner: **if the cursor lags behind on a single file, we have failed.**

### Measured baseline (P0 spike, 2026-06-13, live WKWebView)

Profiled in the running app via the Tauri MCP bridge (frame-gap recorder + React commit counter hooked through the DevTools global hook; edits driven through Monaco's real `type` pipeline by grabbing the live editor instance off the React fiber). Harness removed after measuring.

| Path | Measurement | Verdict |
|---|---|---|
| **Tab switch** (within a pane) | Bimodal: ~**110–116ms** main-thread stall on ~half of switches (≈7 dropped frames, perceptible hitch), ~13ms on the rest. Only **1 Monaco instance** exists with 2 tabs open. | **Primary, data-backed pain.** Caused by destroy+recreate of the editor. |
| **Caret movement** | 10 ArrowRight moves → **1 React commit total**. | Not a problem. Refuted. |
| **Typing** | Each keystroke → **2 React commits**, frame 14–27ms (around the 16.7ms budget, no major jank). 40-char burst → 21ms Monaco edit cost (~0.5ms/char), batched. Idle → 0 commits. | **Real but secondary.** Content flows through React (store write + subscriber re-render per change). Did not reproduce catastrophic lag on a single pane / medium file / fast machine — but it is the anti-pattern that balloons under load (large files, splits, slow hardware). |
| **Pane resize** | Already fixed in a prior session: GPU layer-promotion during drag (66ms→17ms). | Done. Not part of this work except as a constraint to preserve. |

**Conclusion:** the headline win is eliminating the tab-switch hitch via a retained editor + model-swap. The typing path is a real-but-secondary anti-pattern we remove for correctness and load-insurance, re-billed honestly from "fixes cursor lag" to "removes per-keystroke React churn."

### Current architecture (grounded in code)

- `web/src/features/editor/components/monaco-editor.tsx` — creates a `monacoEditor.createModel(...)` (`:504`) and `monacoEditor.create(...)` (`:507`) per mounted editor. Creation effect deps (`:693-702`) include `activeBufferId, filePath, modelUri` (but intentionally **exclude** `isActiveSurface`, so pane-focus switches don't recreate).
- `web/src/features/panes/components/pane-container.tsx` — `renderActiveBuffer()` (`:443-490`) mounts **only the active buffer's** editor (`:581-584`). Terminals/web-viewers are kept always-mounted via `visibility:hidden` (`:540-580`). **Therefore a tab switch unmounts the old editor (dispose) and mounts a new one (create)** — the 110ms hitch.
- Text source-of-truth is `EditorContent.content` in the per-workspace buffer store (`features/workspace/stores/slices/buffer-slice.ts`), two-way synced: model→store via `onDidChangeModelContent` (`monaco-editor.tsx:585`), store→model via `model.setValue(...)` (`monaco-editor.tsx:815-833`).
- Scroll/cursor are **manually cached** in `EditorViewStateCacheManager` (`features/editor/stores/state-store.ts:19-134`), keyed with a `paneId:bufferId → bufferId` fallback — **not** Monaco's native `saveViewState`/`restoreViewState`.
- Splits use `react-resizable-panels` v4.11.2 as a pure UI layer; the authoritative tree is `LayoutNode`/`LayoutSplit.sizes` (percentages) in `pane-slice.ts`, rendered recursively by `pane-node-renderer.tsx`.
- Cross-pane tab DnD **already exists** (`@dnd-kit` + `useTabDrag` + `SplitDropOverlay`, `pane-container.tsx:304-401`).

---

## 2. Target architecture (VSCode-style, in one webview)

The principle: **adopt VSCode's hosting architecture — retained editor widgets, model swap, a workspace-wide model registry, native view-state — while React is demoted to laying out *slots* and rendering chrome.** We are *not* going to native multi-webview panes (rejected — see §9).

Three new units, each independently testable, plus a React-boundary change.

### 2a. Workspace model registry — `features/editor/lib/model-registry.ts`

A per-workspace singleton keyed by **file URI** (reusing the existing `athas://editor/...` URI scheme, but keyed by *file*, not `bufferId`).

- `acquire(uri, languageId, initialText): ITextModel` — creates the Monaco model once; ref-counts subsequent acquirers.
- `release(uri)` — decrements; disposes the model when the last holder releases.
- Same file shown in two panes → **same model object** → live cross-pane sync + a single shared undo history, for free (this is the agreed "shared model" decision).
- Lifecycle is per-workspace (lives in the workspace store registry world; disposed with the workspace).

### 2b. Editor manager — `features/editor/lib/editor-manager.ts`

Owns the Monaco **widgets**, outside the React tree. **One persistent editor per live pane** (decision locked: *not* a global recycled pool — YAGNI at our pane counts; the model-swap delivers the entire perf win, and a free-list can be added later behind the same API without callers changing).

- `mountPane(paneId, slotEl)` — creates one `monacoEditor.create(...)` into the React-provided slot element when a pane first appears.
- `showBuffer(paneId, uri)` — the hot path: `saveViewState` for the outgoing `(paneId, uri)`, `editor.setModel(registry.acquire(uri))`, `restoreViewState` for the incoming `(paneId, uri)`. **Tab switch becomes a model swap — sub-frame, no construction.**
- `unmountPane(paneId)` — disposes that pane's editor and releases its current model.
- The manager is the single owner of editor lifecycle; React never disposes/recreates an editor on tab switch or caret move.

### 2c. View-state store (native) — replaces the manual cache

Replace `setScrollForBuffer`/`setCursorPosition`/`getCachedViewState` scroll-cursor caching with Monaco's native `editor.saveViewState()` / `editor.restoreViewState()`, keyed by `(paneId, uri)`. This gives **per-pane independent scroll/cursor/folding even on a shared model** — exactly VSCode. The existing `EditorViewStateCacheManager` is retired (no migration code — pre-production, per project policy).

### 2d. React boundary

- React renders: the **pane slot** (a positioned `<div>` with a stable id/ref), the **tab strip**, the **breadcrumb header** — all unchanged visually.
- `editor-pane.tsx` / `monaco-editor.tsx` collapse from "owns a Monaco lifecycle" to "register/unregister a slot with the editor manager + render the missing-file/error/preview states."
- The manager mounts Monaco's DOM into the slot imperatively. React no longer re-renders or remounts the editor on tab switch or caret move.

### 2e. Content source-of-truth (the typing path)

Flip it: **the model is authoritative for text.** `onDidChangeModelContent` writes to `buffer.content` **throttled + fire-and-forget** (for persistence/dirty-tracking/session save), off the critical path, never render-blocking. Remove the `model.setValue` two-way controlled sync; external changes (file reloaded from disk, format-on-save, multi-cursor from elsewhere) are applied as **guarded model edits**, not as a controlling prop. Target: **~0 React commits per keystroke.**

---

## 3. Data flow

**Open a file in a pane:**
1. Buffer opened in store (unchanged: `openContent` → `addBufferToPane` → `activatePaneBuffer`).
2. Pane renderer mounts a slot; `editor-manager.mountPane(paneId, slotEl)` ensures a widget exists.
3. `editor-manager.showBuffer(paneId, fileUri)` → `registry.acquire(fileUri, lang, text)` → `setModel` → `restoreViewState`.

**Switch tab (same pane):** `activatePaneBuffer` → `showBuffer(paneId, newUri)` = saveViewState(old) + setModel(new) + restoreViewState(new). No mount/unmount.

**Same file in two panes:** both panes `acquire` the same URI → same model → edits in one appear live in the other; undo is shared; each pane keeps its own `(paneId,uri)` view-state.

**Type a character:** model edit → Monaco repaints (its own fast path) → throttled fire-and-forget write to `buffer.content` for dirty/persist. No controlled re-render.

**Close a tab:** pane drops the buffer → `registry.release(uri)`; model disposed only when no pane holds it.

**Move a tab between panes (existing DnD):** destination pane `showBuffer(destPane, uri)` (model already shared, cheap), source pane releases its hold / shows its next buffer, view-state carried per `(paneId,uri)`.

---

## 4. Phase 4 — Imperative pixel splitter (VSCode sash)

Replace `react-resizable-panels` (flex-%) with a VSCode-style sash: panes hold **explicit pixel sizes**, the sash sets sizes imperatively during drag (no flexbox reflow cascade). The authoritative `LayoutNode` tree stays; only the rendering/resizing layer changes. Pairs with the imperative editor layer (sash resizes the *slots*; manager flushes one `editor.layout()` on `pane-resize-end`). Preserve the existing GPU layer-promotion fix and the `data-pane-resizing` mechanism.

**Scope guard:** editor-area splitter only. **Sidebar resize is untouched.**

---

## 5. Phase 5 — Tab DnD polish (already exists)

Cross-pane tab DnD already works (`@dnd-kit` + `useTabDrag` + `SplitDropOverlay`). This phase only ensures it still works under model-swap and that a moved tab **carries its `(paneId,uri)` view-state** and reuses the shared model (no reload). Drop zones (center = move, edges = split) stay as-is. Likely small.

---

## 6. What stays untouched

- **Sidebar** — hard constraint.
- Tab strip + breadcrumb chrome — stay React, visually identical.
- Pane resize GPU layer-promotion (`editor-theme.css`) and `data-pane-resizing` flow.
- CSS-only active-tab indicator (`components/ui/tabs.tsx`).
- Terminals/web-viewers always-mounted (`visibility:hidden`) pattern — editor-only work.
- Multi-workspace store isolation; per-workspace registry/manager instances.

---

## 7. Error handling & edge cases

- **Missing file / read error:** `editor-pane.tsx` keeps rendering the missing-file placeholder + ErrorBoundary; the manager skips `showBuffer` for a missing file (no model acquired).
- **Preview mode** (`isPreviewMode`): still a property of the buffer; model swap respects it (read-only model option toggled per show, not a new editor).
- **Dev StrictMode double-mount:** manager `mountPane`/`unmountPane` must be idempotent and ref-counted so a double-invoke doesn't create two widgets or dispose a live one.
- **Model disposal race:** ref-count must be correct — never dispose a model still shown in another pane; never leak after the last close. Covered by tests.
- **Workspace switch:** manager/registry are per-workspace; switching workspaces detaches widgets (or parks them) without disposing models of the inactive workspace prematurely.

---

## 8. Testing

Unit tests in `web/src/__tests__/features/editor/lib/` (mirror structure, `@/` imports per CLAUDE.md):
- `model-registry.test.ts` — acquire reuses the same model for one URI; ref-count; dispose only on last release; language/initial-text handling.
- `editor-manager.test.ts` — mountPane creates one widget; showBuffer swaps model without remount; saveViewState/restoreViewState ordering; unmountPane disposes + releases; StrictMode idempotency.
- `view-state.test.ts` — per-`(paneId,uri)` save/restore; independent state for the same model across two panes.

Functional verification per phase via the Tauri MCP harness, re-running the P0 measurements and comparing against baseline (see §10).

---

## 9. Rejected alternative — native multi-webview panes

Considered and rejected: making each pane its own child WKWebView positioned/resized natively from Rust. Reasons:
- **"Webviews everywhere" is the opposite of native.** Each child WKWebView is *more* WebKit; native feel comes from a native *renderer* (Zed/GPUI), which is no-webview. Multi-webview pays native-integration cost while keeping webview paint cost.
- Resizing a native frame still forces WebKit to relayout/repaint its contents — and **loses** the GPU layer-promotion fix (works within one compositor, not across native frames).
- Each webview is an **isolated JS heap** → the shared-model registry becomes impossible in-process (cross-pane editing = CRDT/OT over IPC); tab DnD across webview boundaries breaks (HTML5 DnD doesn't cross WKWebViews); command palette / hover / peek widgets can't overflow a pane frame.
- Not a mainstream Tauri pattern; multi-webview is for isolated/untrusted surfaces, not tiled editor groups.
- Reference points cut against it: **VSCode** (best Monaco host) is single-webview imperative DOM; **Zed** is fully native, zero webview. There is no productive middle.

One narrow valid use retained for later: a child webview for **isolated/untrusted preview content** (e.g. rendering arbitrary HTML) — unrelated to editor tiling.

---

## 10. Phasing & acceptance

Each phase is behavior-preserving, independently shippable, and gated on re-measuring in WKWebView before advancing.

| Phase | Work | Acceptance |
|---|---|---|
| **P0** | Spike/baseline | ✅ Done (§1). |
| **P1** | Model registry + editor manager (one-per-pane, model swap); flip pane mounting to slots | Tab switch: no remount; all switches ~13ms (no 110ms case). 1 widget per pane regardless of tab count. App behaves identically. |
| **P2** | Native `saveViewState`/`restoreViewState`, retire manual cache | Scroll/cursor/folding preserved per pane across tab switches and across two panes on the same file. |
| **P3** | Content source-of-truth flip + throttled fire-and-forget store write | ~0 React commits per keystroke (down from 2); re-measure typing under a large file + a 2-pane split. |
| **P4** | Imperative pixel splitter (sash) | Resize stays ≤17ms; pixel-deterministic; layer-promotion + `pane-resize-end` preserved; sidebar untouched. |
| **P5** | Tab DnD polish under model-swap | Cross-pane move carries view-state, reuses shared model, no reload; drop zones unchanged. |
| **P6** | Full verification | Side-by-side before/after numbers vs P0; full app behavior parity; tests green. |

**Definition of done:** Crowbar behaves exactly as today, the sidebar is unchanged, the measured tab-switch hitch is gone, keystroke React churn is ~0, resize remains ≤17ms — each claim backed by a re-run trace, not assertion.
