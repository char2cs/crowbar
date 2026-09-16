import { describe, expect, it, vi } from 'vitest'
import type { IEditorLike, MonacoEditorApi } from '@/features/editor/lib/editor-manager'
import type { IModelLike, MonacoModelApi } from '@/features/editor/lib/model-registry'

// The arm seam dynamic-imports `monaco-adapters` (which imports the multi-MB
// `monaco-editor`). Mock it with DOM-free fakes so this test exercises the seam
// — the dynamic-import arm path, idempotency, and getter wiring — without paying
// the real Monaco import cost (which is slow/flaky under the full suite's load).
// `EditorManager`/`ModelRegistry` themselves are Monaco-free, so they are still
// the REAL classes constructed with these fake backing APIs.
vi.mock('@/features/editor/lib/monaco-adapters', () => {
  const fakeModelApi = (): MonacoModelApi => {
    const models = new Map<string, IModelLike>()
    return {
      createModel: (value, _lang, uri) => {
        let text = value
        const m: IModelLike = {
          uri,
          dispose: () => models.delete(uri),
          getValue: () => text,
          setValueIfChanged: (next) => {
            text = next
          },
        }
        models.set(uri, m)
        return m
      },
      getModel: (uri) => models.get(uri) ?? null,
    }
  }
  const fakeEditorApi = (): MonacoEditorApi => ({
    create: (): IEditorLike => ({
      setModel: () => {},
      getModel: () => null,
      saveViewState: () => null,
      restoreViewState: () => {},
      layout: () => {},
      dispose: () => {},
      raw: () => null,
    }),
  })
  return {
    EDITOR_CREATE_OPTIONS: {},
    langForUri: (_uri: string) => 'plaintext',
    realModelApi: fakeModelApi,
    realEditorApi: fakeEditorApi,
  }
})

import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import {
  getOrCreateWorkspaceStore,
  destroyWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { EditorManager } from '@/features/editor/lib/editor-manager'
import { ModelRegistry } from '@/features/editor/lib/model-registry'
import { useHistoryStore } from '@/features/editor/stores/history-store'

// Task 4b: Monaco loads via a dynamic-import seam on first ACTUAL editor need
// (EditorPane mount → store.armEditor()), NOT at store creation. createWorkspaceStore
// must construct NO Monaco-backed handles eagerly, so the workspace-store chain
// (main.tsx → route tree → pane-container → workspace-store) stays off the static
// monaco-editor import graph and out of the entry chunk.
describe('workspace-store editor arming seam', () => {
  it('does NOT construct editorManager/modelRegistry at store creation', () => {
    const store = createWorkspaceStore('arm-ws')
    expect(store.editorManager).toBeUndefined()
    expect(store.modelRegistry).toBeUndefined()
    // The Monaco-free active-editor registry stays eager (satellite UI reads it
    // without ever loading monaco).
    expect(store.activeEditorRegistry).toBeDefined()
  })

  it('arms the Monaco-backed handles on armEditor() and is idempotent', async () => {
    const store = createWorkspaceStore('arm-ws')
    await store.armEditor()
    const manager = store.editorManager
    const registry = store.modelRegistry
    expect(manager).toBeInstanceOf(EditorManager)
    expect(registry).toBeInstanceOf(ModelRegistry)

    // A second arm resolves without rebuilding the handles.
    await store.armEditor()
    expect(store.editorManager).toBe(manager)
    expect(store.modelRegistry).toBe(registry)
  })

  it('shares a single construction across concurrent arm callers', async () => {
    const store = createWorkspaceStore('arm-ws')
    await Promise.all([store.armEditor(), store.armEditor()])
    const manager = store.editorManager
    await store.armEditor()
    expect(store.editorManager).toBe(manager)
  })

  it('tolerates editor buffer lifecycle before arming (slices no-op, no monaco load)', () => {
    resetWindowPaneStoreForTests()
    const store = createWorkspaceStore('arm-ws')
    // Task 26: panes/buffers are window-level now.
    const id = windowPaneStore.getState().bufferActions.openContent({
      type: 'editor',
      path: '/a.ts',
      name: 'a.ts',
      content: 'hello',
      workspaceId: 'arm-ws',
    })
    // Closing / removing an editor buffer routes through the slices'
    // `editorManager?.closeBuffer(...)` release path. Pre-arm there is no manager
    // and no held model, so this must be a safe no-op — and must NOT force a load.
    expect(() => {
      windowPaneStore
        .getState()
        .paneActions.removeEditorTabFromPane(windowPaneStore.getState().activePaneId, id)
      windowPaneStore.getState().bufferActions.closeBuffer(id)
    }).not.toThrow()
    expect(store.editorManager).toBeUndefined()
  })

  // Bug fix (unify-sidebar keep-alive audit): Task 26 fix round 2 (I2
  // revisited) made this gate unconditional, reasoning that a buffer still
  // open elsewhere was always rendered by a DIFFERENT WorkspaceView against
  // ITS OWN ambient EditorManager — never the one being destroyed — because
  // editor-surface.tsx resolved the manager from the ambient
  // WorkspaceStoreContext. editor-pane.tsx/editor-surface.tsx now resolve it
  // via `getWorkspaceStore(buf.workspaceId)` instead (the fix for the
  // wrong-ambient-hidden-copy leak that round 2's reasoning rested on), which
  // makes round 2's premise false: a still-open buffer's REAL manager is once
  // again this one, wherever it is rendered from. The gate is restored so
  // disposeAll() cannot yank a live widget's model out from under the user.
  it('does NOT dispose the editor manager on destroy while one of its buffers is still open in a live pane', async () => {
    resetWindowPaneStoreForTests()
    const wsId = 'arm-ws-dispose-open'
    const store = getOrCreateWorkspaceStore(wsId)
    await store.armEditor()
    const disposeAll = vi.spyOn(store.editorManager!, 'disposeAll')

    const openId = windowPaneStore.getState().bufferActions.openContent({
      type: 'editor',
      path: '/still-open.ts',
      name: 'still-open.ts',
      content: 'hello',
      workspaceId: wsId,
    })
    const closedId = windowPaneStore.getState().bufferActions.openContent({
      type: 'editor',
      path: '/already-closed.ts',
      name: 'already-closed.ts',
      content: 'hello',
      workspaceId: wsId,
    })
    // Detach the second buffer from every pane WITHOUT sweeping it from the
    // flat buffer list — the exact "closed everywhere, not yet swept" case
    // the teardown's own buffers filter targets — so the async teardown has
    // real, independently-observable work to do: it clears this buffer's undo
    // history, which this test can wait on for a real completion signal
    // instead of a sleep.
    windowPaneStore
      .getState()
      .paneActions.removeEditorTabFromPane(windowPaneStore.getState().activePaneId, closedId)
    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.editorTabIds).toContain(openId)
    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.editorTabIds).not.toContain(closedId)

    const { pushHistory, getHistoryState } = useHistoryStore.getState().actions
    pushHistory(closedId, { content: 'hello', timestamp: Date.now() })
    expect(getHistoryState(closedId)?.past).toHaveLength(1)

    destroyWorkspaceStore(wsId)

    // The async teardown (dynamic-imports window-pane-store) clears undo
    // history for the no-longer-referenced buffer as its last step before the
    // disposeAll gate in the SAME callback — once this has fired, that gate
    // has necessarily already been evaluated too.
    await vi.waitFor(() => expect(getHistoryState(closedId)?.past).toHaveLength(0))
    expect(disposeAll).not.toHaveBeenCalled()
  })

  it('disposes the editor manager on destroy once none of its buffers are open in a live pane', async () => {
    resetWindowPaneStoreForTests()
    const wsId = 'arm-ws-dispose-closed'
    const store = getOrCreateWorkspaceStore(wsId)
    await store.armEditor()
    const disposeAll = vi.spyOn(store.editorManager!, 'disposeAll')

    const bufferId = windowPaneStore.getState().bufferActions.openContent({
      type: 'editor',
      path: '/closing.ts',
      name: 'closing.ts',
      content: 'hello',
      workspaceId: wsId,
    })
    windowPaneStore
      .getState()
      .paneActions.removeEditorTabFromPane(windowPaneStore.getState().activePaneId, bufferId)
    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.editorTabIds).not.toContain(bufferId)

    destroyWorkspaceStore(wsId)

    await vi.waitFor(() => expect(disposeAll).toHaveBeenCalledTimes(1))
  })
})
