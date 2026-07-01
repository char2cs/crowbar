# Editor-Hosting Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Adopt VSCode-style editor hosting in Crowbar's single WKWebView — a workspace model registry, one retained Monaco widget per pane with model-swap on tab switch, Monaco-native view-state, and model-authoritative content — eliminating the measured ~110ms tab-switch hitch and per-keystroke React churn while keeping the app behavior-identical.

**Architecture:** Two new framework-agnostic `lib` units (`model-registry`, `editor-manager`) own Monaco models and widgets outside React. React is demoted to rendering pane *slots* + chrome. The existing per-workspace Zustand stores remain the orchestration layer (open/activate/close buffers); the manager reacts to store changes. Built incrementally P1→P6, each phase behavior-preserving and gated on re-measuring in the real WKWebView against the P0 baseline.

**Tech Stack:** TypeScript, React 19, Zustand (per-workspace store registry), `monaco-editor` 0.55.1, `react-resizable-panels` 4.11.2 (replaced in P4), `@dnd-kit` (P5), Vitest (`web/src/__tests__/` mirror, `@/` imports), Tauri MCP for WKWebView verification.

**Reference spec:** `docs/superpowers/specs/2026-06-13-editor-hosting-redesign-design.md`

**Baseline to beat (P0, measured 2026-06-13):** tab switch ~110–116ms (≈half of switches) → target ~13ms always; typing 2 React commits/keystroke → target ~0; resize already 17ms (preserve).

---

## How to verify in WKWebView (used by every phase's acceptance)

The app runs under Tauri at `localhost:9223` (driver session) / Vite at `localhost:5173`. Re-use the P0 harness technique:

1. `mcp__tauri__driver_session` action `status` to confirm connection.
2. Install on `window.__spike`: a `PerformanceObserver({entryTypes:['longtask']})`, a `requestAnimationFrame` frame-gap recorder (`S.frames`), and a React commit counter by wrapping `window.__REACT_DEVTOOLS_GLOBAL_HOOK__.onCommitFiberRoot` (`S.commits`).
3. Grab the live editor instance off the React fiber: walk up from `.monaco-editor` to the nearest ancestor with a `__reactFiber$*` key, DFS the fiber subtree for an object with `getModel`/`executeEdits`/`onDidChangeModelContent`.
4. Tab switch: clear `S.frames`, click the editor tab button, read `Math.max(...S.frames)` after settle.
5. Typing: `editor.trigger('spike','type',{text:'X'})` per char (real type path); read `S.commits` after settle.
6. Restore any edits (`editor.trigger('spike','undo')`) and run `S.cleanup()` + restore the commit hook when done — never leave observers in the app.

Each phase: capture before/after numbers, paste into the phase's checklist.

---

## File Structure

**New files:**
- `web/src/features/editor/lib/model-registry.ts` — per-workspace Monaco model store keyed by file URI; ref-counted acquire/release.
- `web/src/features/editor/lib/editor-manager.ts` — owns one Monaco widget per pane; `mountPane`/`showBuffer`/`unmountPane`; saveViewState/restoreViewState per `(paneId,uri)`.
- `web/src/features/editor/lib/editor-uri.ts` — single source of the `athas://editor/<file>` URI helper (extracted from `monaco-editor.tsx:165-169`, re-keyed by file path not bufferId).
- `web/src/__tests__/features/editor/lib/model-registry.test.ts`
- `web/src/__tests__/features/editor/lib/editor-manager.test.ts`
- `web/src/__tests__/features/editor/lib/editor-uri.test.ts`

**Modified files:**
- `web/src/features/editor/components/monaco-editor.tsx` — collapse from lifecycle-owner to slot-registrar (P1), drop manual view-state (P2), flip content sync (P3).
- `web/src/features/panes/components/editor-pane.tsx` — render a slot div + register/unregister with manager.
- `web/src/features/panes/components/pane-container.tsx` — mount slot for active editor buffer via manager instead of remounting `<CodeEditor>`.
- `web/src/features/editor/stores/state-store.ts` — retire `EditorViewStateCacheManager` (P2).
- `web/src/features/workspace/stores/slices/buffer-slice.ts` — content write becomes throttled fire-and-forget sink (P3).
- `web/src/features/panes/components/pane-node-renderer.tsx` — swap `react-resizable-panels` for sash (P4).

**Registry/manager instance ownership:** one `ModelRegistry` + one `EditorManager` per workspace, created in `createWorkspaceStore` (`workspace-store.ts`) and disposed when the workspace store is disposed. Accessed by components via the existing `useWorkspaceStoreContext` / `getActiveWorkspaceStoreRef()`.

---

## PHASE 1 — Model registry + retained per-pane editor (model swap)

Highest-value, data-backed phase. After P1, a tab switch must perform zero Monaco construction.

### Task 1: Editor URI helper

**Files:**
- Create: `web/src/features/editor/lib/editor-uri.ts`
- Test: `web/src/__tests__/features/editor/lib/editor-uri.test.ts`

- [ ] **Step 1: Write the failing test**

```ts
import { describe, expect, it } from 'vitest'
import { fileUri, uriToFsPath } from '@/features/editor/lib/editor-uri'

describe('editor-uri', () => {
  it('builds a stable athas uri from a file path (keyed by file, not buffer)', () => {
    const a = fileUri('/repo/src/index.ts')
    const b = fileUri('/repo/src/index.ts')
    expect(a).toBe(b) // same file => same uri (enables shared model)
    expect(a).toMatch(/^athas:\/\/editor\//)
  })

  it('round-trips back to the fs path', () => {
    expect(uriToFsPath(fileUri('/repo/a b/c.ts'))).toBe('/repo/a b/c.ts')
  })

  it('distinguishes different files', () => {
    expect(fileUri('/a.ts')).not.toBe(fileUri('/b.ts'))
  })
})
```

- [ ] **Step 2: Run test, verify it fails**

Run: `cd web && bun run vitest run src/__tests__/features/editor/lib/editor-uri.test.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement**

```ts
// web/src/features/editor/lib/editor-uri.ts
const PREFIX = 'athas://editor/'

/** Stable Monaco model URI for a file, keyed by file path so the same file
 *  shares one model across panes. Encodes the path to survive spaces/unicode. */
export function fileUri(fsPath: string): string {
  return PREFIX + encodeURIComponent(fsPath)
}

export function uriToFsPath(uri: string): string {
  return decodeURIComponent(uri.slice(PREFIX.length))
}
```

- [ ] **Step 4: Run test, verify it passes**

Run: `cd web && bun run vitest run src/__tests__/features/editor/lib/editor-uri.test.ts`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/features/editor/lib/editor-uri.ts web/src/__tests__/features/editor/lib/editor-uri.test.ts
git commit -m "feat(editor): file-keyed monaco uri helper"
```

### Task 2: Model registry (ref-counted, file-keyed)

**Files:**
- Create: `web/src/features/editor/lib/model-registry.ts`
- Test: `web/src/__tests__/features/editor/lib/model-registry.test.ts`

The registry wraps `monaco.editor` model APIs behind an injected dependency so it is unit-testable without a DOM. Interface:

```ts
export interface MonacoModelApi {
  createModel(value: string, languageId: string, uri: string): IModelLike
  getModel(uri: string): IModelLike | null
}
export interface IModelLike { uri: string; dispose(): void; isDisposed?(): boolean }
```

- [ ] **Step 1: Write the failing test**

```ts
import { describe, expect, it, vi } from 'vitest'
import { ModelRegistry } from '@/features/editor/lib/model-registry'

function fakeApi() {
  const models = new Map<string, any>()
  return {
    models,
    createModel: vi.fn((value: string, _lang: string, uri: string) => {
      const m = { uri, value, disposed: false, dispose() { this.disposed = true; models.delete(uri) } }
      models.set(uri, m); return m
    }),
    getModel: vi.fn((uri: string) => models.get(uri) ?? null),
  }
}

describe('ModelRegistry', () => {
  it('creates one model per uri and reuses it on re-acquire', () => {
    const api = fakeApi(); const r = new ModelRegistry(api)
    const m1 = r.acquire('athas://editor/x', 'ts', 'a')
    const m2 = r.acquire('athas://editor/x', 'ts', 'a')
    expect(m1).toBe(m2)
    expect(api.createModel).toHaveBeenCalledTimes(1)
  })

  it('disposes the model only when the last holder releases', () => {
    const api = fakeApi(); const r = new ModelRegistry(api)
    const m = r.acquire('athas://editor/x', 'ts', 'a')
    r.acquire('athas://editor/x', 'ts', 'a') // refcount 2
    r.release('athas://editor/x')            // -> 1, still alive
    expect(m.disposed).toBe(false)
    r.release('athas://editor/x')            // -> 0, disposed
    expect(m.disposed).toBe(true)
  })

  it('release of unknown uri is a no-op (no throw)', () => {
    const api = fakeApi(); const r = new ModelRegistry(api)
    expect(() => r.release('athas://editor/none')).not.toThrow()
  })

  it('re-acquire after disposal creates a fresh model', () => {
    const api = fakeApi(); const r = new ModelRegistry(api)
    r.acquire('athas://editor/x', 'ts', 'a'); r.release('athas://editor/x')
    r.acquire('athas://editor/x', 'ts', 'b')
    expect(api.createModel).toHaveBeenCalledTimes(2)
  })
})
```

- [ ] **Step 2: Run test, verify it fails**

Run: `cd web && bun run vitest run src/__tests__/features/editor/lib/model-registry.test.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement**

```ts
// web/src/features/editor/lib/model-registry.ts
export interface IModelLike { uri: string; dispose(): void }
export interface MonacoModelApi {
  createModel(value: string, languageId: string, uri: string): IModelLike
  getModel(uri: string): IModelLike | null
}

interface Entry { model: IModelLike; refs: number }

/** Per-workspace registry of Monaco text models keyed by file URI.
 *  Same file in two panes => same model => live sync + shared undo. */
export class ModelRegistry {
  private entries = new Map<string, Entry>()
  constructor(private api: MonacoModelApi) {}

  acquire(uri: string, languageId: string, initialText: string): IModelLike {
    const existing = this.entries.get(uri)
    if (existing) { existing.refs++; return existing.model }
    const model = this.api.getModel(uri) ?? this.api.createModel(initialText, languageId, uri)
    this.entries.set(uri, { model, refs: 1 })
    return model
  }

  release(uri: string): void {
    const e = this.entries.get(uri)
    if (!e) return
    e.refs--
    if (e.refs <= 0) { this.entries.delete(uri); e.model.dispose() }
  }

  get(uri: string): IModelLike | null { return this.entries.get(uri)?.model ?? null }
  disposeAll(): void { for (const e of this.entries.values()) e.model.dispose(); this.entries.clear() }
}
```

- [ ] **Step 4: Run test, verify it passes**

Run: `cd web && bun run vitest run src/__tests__/features/editor/lib/model-registry.test.ts`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/features/editor/lib/model-registry.ts web/src/__tests__/features/editor/lib/model-registry.test.ts
git commit -m "feat(editor): ref-counted file-keyed model registry"
```

### Task 3: Editor manager (one widget per pane, model swap + view-state)

**Files:**
- Create: `web/src/features/editor/lib/editor-manager.ts`
- Test: `web/src/__tests__/features/editor/lib/editor-manager.test.ts`

Manager depends on injected `MonacoEditorApi` (create widget) + a `ModelRegistry`. The widget is a thin interface so tests need no DOM:

```ts
export interface IEditorLike {
  setModel(model: IModelLike | null): void
  getModel(): IModelLike | null
  saveViewState(): unknown
  restoreViewState(state: unknown): void
  layout(): void
  dispose(): void
}
export interface MonacoEditorApi { create(container: HTMLElement): IEditorLike }
```

- [ ] **Step 1: Write the failing test**

```ts
import { describe, expect, it, vi } from 'vitest'
import { EditorManager } from '@/features/editor/lib/editor-manager'
import { ModelRegistry } from '@/features/editor/lib/model-registry'

function fakeModelApi() {
  const models = new Map<string, any>()
  return {
    createModel: vi.fn((v: string, _l: string, uri: string) => { const m = { uri, dispose: vi.fn(() => models.delete(uri)) }; models.set(uri, m); return m }),
    getModel: (uri: string) => models.get(uri) ?? null,
  }
}
function fakeEditorApi() {
  const created: any[] = []
  return {
    created,
    create: vi.fn(() => {
      let model: any = null; let vs: any = null
      const ed = {
        setModel: vi.fn((m: any) => { model = m }),
        getModel: () => model,
        saveViewState: vi.fn(() => vs ?? { for: model?.uri }),
        restoreViewState: vi.fn((s: any) => { vs = s }),
        layout: vi.fn(), dispose: vi.fn(),
      }
      created.push(ed); return ed
    }),
  }
}
const lang = () => 'ts'
const text = () => 'code'

describe('EditorManager', () => {
  it('creates exactly one widget per pane', () => {
    const ea = fakeEditorApi(); const reg = new ModelRegistry(fakeModelApi())
    const m = new EditorManager(ea, reg, { lang, text })
    const el = {} as HTMLElement
    m.mountPane('p1', el); m.mountPane('p1', el) // idempotent (StrictMode)
    expect(ea.create).toHaveBeenCalledTimes(1)
  })

  it('showBuffer swaps the model without creating a new widget', () => {
    const ea = fakeEditorApi(); const reg = new ModelRegistry(fakeModelApi())
    const m = new EditorManager(ea, reg, { lang, text })
    m.mountPane('p1', {} as HTMLElement)
    m.showBuffer('p1', 'athas://editor/a')
    m.showBuffer('p1', 'athas://editor/b')
    expect(ea.create).toHaveBeenCalledTimes(1)
    const ed = ea.created[0]
    expect(ed.getModel().uri).toBe('athas://editor/b')
  })

  it('saves outgoing view-state and restores incoming on swap', () => {
    const ea = fakeEditorApi(); const reg = new ModelRegistry(fakeModelApi())
    const m = new EditorManager(ea, reg, { lang, text })
    m.mountPane('p1', {} as HTMLElement)
    m.showBuffer('p1', 'athas://editor/a')
    m.showBuffer('p1', 'athas://editor/b')
    m.showBuffer('p1', 'athas://editor/a') // back to a
    const ed = ea.created[0]
    // restoreViewState called for 'a' the second time with the state saved on leaving 'a'
    expect(ed.restoreViewState).toHaveBeenCalled()
  })

  it('two panes on the same uri share the model but keep independent view-state keys', () => {
    const modelApi = fakeModelApi(); const ea = fakeEditorApi()
    const reg = new ModelRegistry(modelApi)
    const m = new EditorManager(ea, reg, { lang, text })
    m.mountPane('p1', {} as HTMLElement); m.mountPane('p2', {} as HTMLElement)
    m.showBuffer('p1', 'athas://editor/a')
    m.showBuffer('p2', 'athas://editor/a')
    expect(modelApi.createModel).toHaveBeenCalledTimes(1) // shared model
    expect(ea.created[0].getModel()).toBe(ea.created[1].getModel())
  })

  it('unmountPane disposes the widget and releases its model', () => {
    const modelApi = fakeModelApi(); const ea = fakeEditorApi()
    const reg = new ModelRegistry(modelApi)
    const m = new EditorManager(ea, reg, { lang, text })
    m.mountPane('p1', {} as HTMLElement)
    m.showBuffer('p1', 'athas://editor/a')
    const ed = ea.created[0]; const model = ed.getModel()
    m.unmountPane('p1')
    expect(ed.dispose).toHaveBeenCalled()
    expect(model.dispose).toHaveBeenCalled() // last holder released
  })
})
```

- [ ] **Step 2: Run test, verify it fails**

Run: `cd web && bun run vitest run src/__tests__/features/editor/lib/editor-manager.test.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement**

```ts
// web/src/features/editor/lib/editor-manager.ts
import type { IModelLike, ModelRegistry } from '@/features/editor/lib/model-registry'

export interface IEditorLike {
  setModel(model: IModelLike | null): void
  getModel(): IModelLike | null
  saveViewState(): unknown
  restoreViewState(state: unknown): void
  layout(): void
  dispose(): void
}
export interface MonacoEditorApi { create(container: HTMLElement): IEditorLike }
export interface BufferMeta { lang(uri: string): string; text(uri: string): string }

interface PaneState { editor: IEditorLike; currentUri: string | null }

/** Owns one retained Monaco widget per pane. Tab switch = model swap, not remount. */
export class EditorManager {
  private panes = new Map<string, PaneState>()
  private viewState = new Map<string, unknown>() // key: `${paneId} ${uri}`
  constructor(private editorApi: MonacoEditorApi, private registry: ModelRegistry, private meta: BufferMeta) {}

  private vsKey(paneId: string, uri: string) { return `${paneId} ${uri}` }

  mountPane(paneId: string, container: HTMLElement): void {
    if (this.panes.has(paneId)) return // idempotent (StrictMode-safe)
    this.panes.set(paneId, { editor: this.editorApi.create(container), currentUri: null })
  }

  showBuffer(paneId: string, uri: string): void {
    const pane = this.panes.get(paneId)
    if (!pane) return
    if (pane.currentUri === uri) return
    // save outgoing
    if (pane.currentUri) {
      this.viewState.set(this.vsKey(paneId, pane.currentUri), pane.editor.saveViewState())
      this.registry.release(pane.currentUri)
    }
    // swap in
    const model = this.registry.acquire(uri, this.meta.lang(uri), this.meta.text(uri))
    pane.editor.setModel(model)
    const saved = this.viewState.get(this.vsKey(paneId, uri))
    if (saved) pane.editor.restoreViewState(saved)
    pane.currentUri = uri
  }

  unmountPane(paneId: string): void {
    const pane = this.panes.get(paneId)
    if (!pane) return
    if (pane.currentUri) this.registry.release(pane.currentUri)
    pane.editor.dispose()
    this.panes.delete(paneId)
  }

  getEditor(paneId: string): IEditorLike | undefined { return this.panes.get(paneId)?.editor }
  layoutPane(paneId: string): void { this.panes.get(paneId)?.editor.layout() }
  disposeAll(): void { for (const id of [...this.panes.keys()]) this.unmountPane(id); this.registry.disposeAll() }
}
```

- [ ] **Step 4: Run test, verify it passes**

Run: `cd web && bun run vitest run src/__tests__/features/editor/lib/editor-manager.test.ts`
Expected: PASS (5 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/features/editor/lib/editor-manager.ts web/src/__tests__/features/editor/lib/editor-manager.test.ts
git commit -m "feat(editor): retained per-pane editor manager with model swap"
```

### Task 4: Wire the manager to real Monaco + per-workspace instance

**Files:**
- Modify: `web/src/features/workspace/stores/workspace-store.ts` (create registry+manager per workspace, dispose on teardown)
- Create: `web/src/features/editor/lib/monaco-adapters.ts` (real `MonacoModelApi`/`MonacoEditorApi` over `monaco-editor`, carrying the full create options from `monaco-editor.tsx:507-552`)
- Test: extend `editor-manager.test.ts` is not needed; add a thin smoke test that adapters expose the required methods (no DOM).

- [ ] **Step 1:** Write `monaco-adapters.ts` exporting `realModelApi()` and `realEditorApi(options)` that delegate to `monacoEditor.createModel`/`monacoEditor.create`. Move the 45 create options out of `monaco-editor.tsx` into a shared `EDITOR_CREATE_OPTIONS` const here so the manager and the (soon-thin) component agree.
- [ ] **Step 2:** In `workspace-store.ts createWorkspaceStore`, instantiate `new ModelRegistry(realModelApi())` and `new EditorManager(realEditorApi(EDITOR_CREATE_OPTIONS), registry, meta)` where `meta.lang/text` read from the buffer slice; store them on the workspace store (non-reactive field) and expose getters. Dispose both in the store teardown alongside the existing buffer-subscription cleanup (`workspace-store.ts:48-51`).
- [ ] **Step 3:** `bun run vitest run src/__tests__/features/editor/lib` — all green.
- [ ] **Step 4:** Commit `feat(editor): real monaco adapters + per-workspace registry/manager`.

### Task 5: Convert the editor pane to a slot (the behavior-preserving swap)

**Files:**
- Modify: `web/src/features/panes/components/editor-pane.tsx`
- Modify: `web/src/features/editor/components/monaco-editor.tsx` (remove create/dispose/model-create; keep settings-effects targeting the manager's editor; keep missing-file/preview/error rendering)
- Modify: `web/src/features/panes/components/pane-container.tsx` (`renderActiveBuffer` editor branch renders the slot)

- [ ] **Step 1:** `editor-pane.tsx` renders a single `<div ref={slotRef} className="absolute inset-0" />`. On mount: `manager.mountPane(paneId, slotRef.current)`; on unmount: `manager.unmountPane(paneId)`. On `activeBufferId` change: `manager.showBuffer(paneId, fileUri(filePath))`. Use `useWorkspaceStoreContext` to get the manager. Keep the existing missing-file placeholder + ErrorBoundary (`editor-pane.tsx:41-58`).
- [ ] **Step 2:** In `monaco-editor.tsx`, delete the create effect (`:693-702`), the model creation (`:504`), and the dispose (`:668-685`). Re-point the settings effects (font/theme/wordWrap/etc.) and the line-number formatter to operate on `manager.getEditor(paneId)`. Keep `onDidChangeModelContent`/cursor listeners but attach them in the manager on `mountPane` (move listener wiring into the manager via a callback prop set by the component). **Do not change content sync yet** (still two-way via store) — that is P3.
- [ ] **Step 3 (manual WKWebView verification):** rebuild; open two files in one pane; switch tabs 8×. Run the harness §"How to verify". Acceptance: **0 new Monaco widgets created on switch (instance count stays 1), max frame ≤ ~20ms on every switch (the 110ms case is gone).** Paste numbers here: `__________`.
- [ ] **Step 4:** Confirm visually identical: scroll position, cursor, syntax colors, breadcrumb, dirty dot all behave as before. Note any diffs: `__________`.
- [ ] **Step 5:** Commit `refactor(editor): retained per-pane widget via editor-manager (no remount on tab switch)`.

### Task 6: Same-file-in-two-panes shared model (manual verification)

- [ ] **Step 1:** Open the same file in a split (two panes). Type in pane A. Acceptance: text appears live in pane B; a single undo (Cmd-Z) in either pane undoes the shared edit; each pane keeps its own scroll/cursor.
- [ ] **Step 2:** If sync is not live, the manager is acquiring distinct models — debug `fileUri` keying. Fix, re-verify.
- [ ] **Step 3:** Commit any fix `fix(editor): ensure shared model across panes for same file`.

---

## PHASE 2 — Monaco-native view-state

Replace the manual `EditorViewStateCacheManager` with `saveViewState`/`restoreViewState` (already used by the manager in P1). This phase removes the now-redundant manual cache and routes any remaining consumers to the manager.

### Task 7: Retire manual view-state cache

**Files:**
- Modify: `web/src/features/editor/stores/state-store.ts` (remove `EditorViewStateCacheManager` `:19-134`, `setScrollForBuffer` `:491`, `getCachedViewState` `:337`, `restorePositionForFile` `:342`, and the cursor/selection cache writes)
- Modify: `web/src/features/editor/components/monaco-editor.tsx` (remove `setScrollForBuffer` call `:602` and manual restore `:1018-1037`)
- Test: `web/src/__tests__/features/editor/lib/editor-manager.test.ts` already covers save/restore ordering; add one test that view-state survives a swap-away-and-back round trip.

- [ ] **Step 1:** Add the round-trip test to `editor-manager.test.ts` (show a→b→a, assert the `a` view-state restored equals the one saved on leaving `a`). Run, watch it pass (manager already implements it).
- [ ] **Step 2:** Delete the manual cache + its call sites. Keep any cursor-position-for-status-bar reads by sourcing them from `editor.getPosition()` via the manager (find consumers with a repo grep for `getCachedViewState`, `setScrollForBuffer`, `restorePositionForFile`, `cursorPosition` selectors and re-point them).
- [ ] **Step 3:** `bun run vitest run` (full editor suite) green; `bun run tsc --noEmit` clean.
- [ ] **Step 4 (WKWebView):** scroll file A halfway, switch to B, back to A — scroll/cursor/folding restored. Two panes same file — independent scroll. Paste result: `__________`.
- [ ] **Step 5:** Commit `refactor(editor): use monaco-native view-state, drop manual cache`.

---

## PHASE 3 — Model-authoritative content (kill per-keystroke churn)

### Task 8: Throttled fire-and-forget content sink

**Files:**
- Modify: `web/src/features/editor/components/monaco-editor.tsx` (remove the `model.setValue` controlled effect `:815-833`; keep `onDidChangeModelContent` but make the store write throttled)
- Modify: `web/src/features/workspace/stores/slices/buffer-slice.ts` (content setter no longer feeds back to the model; updates `isDirty` from `content !== savedContent`)
- Create: `web/src/features/editor/lib/content-sink.ts` (a small throttle utility: coalesce model-change events, flush to store on a trailing 150ms timer + on blur/save/close)
- Test: `web/src/__tests__/features/editor/lib/content-sink.test.ts`

- [ ] **Step 1:** Write `content-sink.test.ts`: rapid changes within the window produce **one** store write (trailing); an explicit `flush()` writes immediately; the latest value wins. Use fake timers (`vi.useFakeTimers`).
- [ ] **Step 2:** Run, verify fail.
- [ ] **Step 3:** Implement `content-sink.ts` (a class `ContentSink({ write, delayMs }) { push(value); flush(); dispose() }`).
- [ ] **Step 4:** Run, verify pass.
- [ ] **Step 5:** Wire it: `onDidChangeModelContent` → `sink.push(model.getValue())`; `flush()` on editor blur, before save, and on `unmountPane`. Remove the controlled `setValue` effect. **External edits** (disk reload / format) go through a new `manager.applyExternalEdit(uri, text)` that does `model.pushEditOperations` (guarded) — NOT `setValue` on the React path.
- [ ] **Step 6 (WKWebView):** type 20 chars on a single pane, then on a large file + 2-pane split. Acceptance: **~0 React commits per keystroke** (down from 2), frame ≤ 16.7ms; dirty dot + save still work; external file reload still updates the editor. Paste before/after: `__________`.
- [ ] **Step 7:** Commit `refactor(editor): model-authoritative content with throttled store sink`.

---

## PHASE 4 — Imperative pixel splitter (VSCode sash)

### Task 9: Sash-based split layout replacing react-resizable-panels

**Files:**
- Modify: `web/src/features/panes/components/pane-node-renderer.tsx` (replace `ResizablePanelGroup`/`Panel`/`Handle` with a sash component driving pixel sizes)
- Create: `web/src/features/panes/components/pane-sash.tsx` (the draggable divider; sets sibling pixel widths imperatively during pointer-move; commits to `LayoutSplit.sizes` on pointer-up)
- Create: `web/src/__tests__/features/panes/lib/split-sizing.test.ts` (pure sizing math: convert px↔% given container size, clamp to min sizes, two-pane redistribution)
- Modify: keep `data-pane-resizing` set on pointer-down / removed on pointer-up and the `pane-resize-end` dispatch (currently `pane-node-renderer.tsx:57-71`).

- [ ] **Step 1:** Extract sizing math into a pure module + test it (TDD): `splitSizesFromDrag(containerPx, startSizes, deltaPx, minPx)` returns clamped `[a,b]` percentages summing to 100.
- [ ] **Step 2:** Implement `pane-sash.tsx`: on pointer-down set `data-pane-resizing`; on pointer-move set the two sibling slot widths in px directly (no React state, no flex churn) and call `manager.layoutPane` suppressed (it's already suppressed during drag); on pointer-up compute final % via the tested math, write `resizePaneSplit(...)`, remove the attribute, dispatch `pane-resize-end`.
- [ ] **Step 3:** Replace the `react-resizable-panels` usage in `pane-node-renderer.tsx` with the recursive sash layout, preserving nested splits and the bottom-panel layout tree.
- [ ] **Step 4:** `bun run vitest run`; `bun remove react-resizable-panels` only after confirming no other importers (`grep -r react-resizable-panels web/src`).
- [ ] **Step 5 (WKWebView):** drag the editor splitter through a full sweep. Acceptance: **≤17ms/frame, ~0 dropped** (matches the layer-promotion baseline); sidebar resize **unchanged**; nested splits resize correctly. Paste: `__________`.
- [ ] **Step 6:** Commit `feat(panes): imperative pixel sash splitter (replaces flex-% panels)`.

---

## PHASE 5 — Tab DnD polish under model-swap

### Task 10: Verify + harden cross-pane tab move

**Files:**
- Modify (if needed): `web/src/features/panes/components/pane-container.tsx` (`handleSplitDrop` `:304-401`) and `web/src/features/tabs/...` (`useTabDrag`)

- [ ] **Step 1 (WKWebView):** drag a tab from pane A to pane B (center drop) and to an edge (creates split). Acceptance: destination shows the file with **no reload** (shared model), the moved tab's scroll/cursor are preserved (view-state keyed by new `(paneId,uri)` — confirm or carry over from the source key), source pane shows its next buffer. Note results: `__________`.
- [ ] **Step 2:** If view-state is lost on move, add `manager.transferViewState(fromPane, toPane, uri)` (copy the `(from,uri)` entry to `(to,uri)`) and call it from `handleSplitDrop`. Add a unit test for `transferViewState`.
- [ ] **Step 3:** If a reload happens, ensure the move path calls `showBuffer` (model-swap) rather than re-opening the buffer.
- [ ] **Step 4:** Commit `fix(panes): preserve shared model + view-state on cross-pane tab move`.

---

## PHASE 6 — Full verification

### Task 11: Before/after report + parity sweep

- [ ] **Step 1:** Re-run the full P0 harness and produce a table: tab-switch max frame, typing commits/keystroke, resize frame — before (P0) vs after. Acceptance: tab-switch ≤~20ms always, typing ~0 commits, resize ≤17ms.
- [ ] **Step 2:** Manual parity sweep (everything must behave as today): open/close files, split/unsplit, reorder tabs, drag tabs across panes, large file scroll/type, undo/redo, save, dirty indicators, preview mode, missing-file, multi-workspace switch, terminals/web-viewers still alive. Check the sidebar is byte-for-byte unchanged in behavior.
- [ ] **Step 3:** `cd web && bun run vitest run && bun run tsc --noEmit && bun run lint` all green.
- [ ] **Step 4:** Commit `test(editor): editor-hosting redesign verification report` (include the before/after table in the commit body or a short `docs/superpowers/plans/` note).

---

## Self-review notes

- **Spec coverage:** §2a→Tasks 1-2, §2b→Tasks 3-4, §2c→Task 7, §2d→Task 5, §2e→Task 8, §3 (data flow)→Tasks 5-6/8/10, §4 splitter→Task 9, §5 DnD→Task 10, §6 untouched→guarded in Tasks 5/9/11, §7 edge cases→manager idempotency (Task 3) + external edit (Task 8) + workspace switch (Task 4 dispose), §8 testing→Tasks 1-3/7-9, §10 phasing/acceptance→phase WKWebView gates. No gaps.
- **Type consistency:** `fileUri`/`uriToFsPath`, `ModelRegistry.acquire/release/get/disposeAll`, `EditorManager.mountPane/showBuffer/unmountPane/getEditor/layoutPane/disposeAll`, `IModelLike`/`IEditorLike` used consistently across tasks.
- **Deliberate granularity note:** Tasks 1-3 and 7-8 carry literal code/tests (self-contained units). Tasks 4-6, 9-11 are integration against existing files and are specified to exact files/sites/acceptance; their literal diffs are produced at execution time against the real (post-previous-task) code, because each is gated on the prior phase's WKWebView re-measurement per the design. This is intentional, not placeholder.
```
