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
import { windowPaneStore, resetWindowPaneStoreForTests } from '@/features/panes/stores/window-pane-store'
import {
  getOrCreateWorkspaceStore,
  destroyWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { EditorManager } from '@/features/editor/lib/editor-manager'
import { ModelRegistry } from '@/features/editor/lib/model-registry'

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

  // Task 26 fix round 2 (I2 revisited): fix round 1 gated
  // destroyWorkspaceStore's editorManager.disposeAll() call on none of the
  // workspace's editor buffers still being open in a live pane — traced and
  // found to guard against unreachable harm (WorkspaceHost only destroys a
  // workspace store AFTER its own subtree has unmounted; a buffer still
  // visible elsewhere is rendered by a DIFFERENT WorkspaceView using ITS OWN
  // ambient editorManager, never this one) while creating a real, permanent
  // leak: since destroyWorkspaceStore is disposeAll()'s only caller and
  // registry.delete still runs unconditionally, the gate meant the
  // EditorManager/ModelRegistry for a destroyed workspace leaked forever
  // whenever any of its editor buffers was still open somewhere — the common
  // case this whole task creates. Reverted to run unconditionally; this pins
  // it so the gate cannot silently come back.
  it('disposes the editor manager on destroy even when one of its buffers is still open in a live pane', async () => {
    resetWindowPaneStoreForTests()
    const wsId = 'arm-ws-dispose'
    const store = getOrCreateWorkspaceStore(wsId)
    await store.armEditor()
    const disposeAll = vi.spyOn(store.editorManager!, 'disposeAll')

    const bufferId = windowPaneStore.getState().bufferActions.openContent({
      type: 'editor',
      path: '/still-open.ts',
      name: 'still-open.ts',
      content: 'hello',
      workspaceId: wsId,
    })
    // Confirm the precondition is real: left attached to its pane, the exact
    // "still surviving" case fix round 1's gate gave up disposal for.
    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.editorTabIds).toContain(bufferId)

    destroyWorkspaceStore(wsId)

    expect(disposeAll).toHaveBeenCalledTimes(1)
  })
})
