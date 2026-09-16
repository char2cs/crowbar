import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { setActiveWorkspaceStoreRef } from '@/features/workspace/stores/workspace-store-ref'
import {
  setActiveWorkspaceId,
  destroyWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'

// Task 26 hoisted buffers/panes out of the per-workspace store into one
// window-level store, and Task 2's ruling settled that preview/pinned are the
// TAB'S own `isPreview`/`isPinned` — `PaneGroup` never gained
// `previewBufferId`/`pinnedBufferIds` (the fields this suite asserted on).
// Migrated onto both: the real store, and the real place the flag lives.
//
// `openContent` resolves an unspecified `workspaceId` from
// `getActiveWorkspaceId()`, and scopes its dedup and its tab cap on it — so
// this registers a real active workspace rather than leaving it ''.

const createMockStorage = () => {
  const storage = new Map<string, string>()

  return {
    getItem: (key: string) => storage.get(key) ?? null,
    setItem: (key: string, value: string) => {
      storage.set(key, value)
    },
    removeItem: (key: string) => {
      storage.delete(key)
    },
    clear: () => {
      storage.clear()
    },
    key: (index: number) => Array.from(storage.keys())[index] ?? null,
    get length() {
      return storage.size
    },
  }
}

describe('buffer preview pane integration', () => {
  beforeEach(() => {
    vi.stubGlobal('localStorage', createMockStorage())
    vi.stubGlobal('window', {
      __TAURI_INTERNALS__: {
        invoke: vi.fn().mockResolvedValue([]),
        metadata: {
          currentWindow: { label: 'main' },
          currentWebview: { label: 'main' },
        },
      },
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })
    resetWindowPaneStoreForTests()
    const store = createWorkspaceStore('test-ws')
    setActiveWorkspaceStoreRef(store)
    setActiveWorkspaceId('test-ws')
  })

  afterEach(() => {
    setActiveWorkspaceStoreRef(null)
    destroyWorkspaceStore('test-ws')
    vi.unstubAllGlobals()
  })

  const bufferById = (id: string) => windowPaneStore.getState().buffers.find((b) => b.id === id)

  it('marks each preview on the tab, one preview slot per pane', () => {
    const { bufferActions, paneActions } = windowPaneStore.getState()

    const firstPreviewId = bufferActions.openContent({
      type: 'editor',
      path: '/workspace/a.ts',
      name: 'a.ts',
      content: 'a',
      isPreview: true,
    })
    const rightPaneId = paneActions.splitPane(ROOT_PANE_ID, 'horizontal')
    expect(rightPaneId).not.toBeNull()
    if (!rightPaneId) return

    // The split became the active pane, so the second preview opens there.
    const secondPreviewId = bufferActions.openContent({
      type: 'editor',
      path: '/workspace/b.ts',
      name: 'b.ts',
      content: 'b',
      isPreview: true,
    })

    expect(windowPaneStore.getState().buffers.map((buffer) => buffer.id)).toEqual([
      firstPreviewId,
      secondPreviewId,
    ])
    expect(paneActions.getPaneById(ROOT_PANE_ID)?.editorTabIds).toEqual([firstPreviewId])
    expect(paneActions.getPaneById(rightPaneId)?.editorTabIds).toEqual([secondPreviewId])
    // Each pane's own preview is marked, and marking one never clears the other
    // pane's (preview is per-pane, and the two panes hold different tabs).
    expect(bufferById(firstPreviewId)?.isPreview).toBe(true)
    expect(bufferById(secondPreviewId)?.isPreview).toBe(true)
  })

  it('clears the preview mark when a preview becomes definite', () => {
    const { bufferActions } = windowPaneStore.getState()

    const previewId = bufferActions.openContent({
      type: 'editor',
      path: '/workspace/preview.ts',
      name: 'preview.ts',
      content: 'preview',
      isPreview: true,
    })

    expect(bufferById(previewId)?.isPreview).toBe(true)

    bufferActions.promotePreview(previewId)

    expect(bufferById(previewId)?.isPreview).toBe(false)
  })

  it('pins a promoted preview', () => {
    const { bufferActions, paneActions } = windowPaneStore.getState()

    const previewId = bufferActions.openContent({
      type: 'editor',
      path: '/workspace/pinned.ts',
      name: 'pinned.ts',
      content: 'pinned',
      isPreview: true,
    })

    // Pin: promote the preview (clears isPreview everywhere) then pin the tab.
    bufferActions.promotePreview(previewId)
    paneActions.setEditorTabPinned(ROOT_PANE_ID, previewId, true)

    expect(bufferById(previewId)?.isPreview).toBe(false)
    expect(bufferById(previewId)?.isPinned).toBe(true)
  })

  // DELETED (final fix wave): 'opens a new tab placeholder in the active pane'.
  // The 'newTab' placeholder buffer no longer exists — Task 1 removed it from
  // PaneContent's union and Task 31 made a pane with zero `editorTabIds` render
  // the New Tab stage for free, so there is nothing to open and nothing to
  // assert. `new-tab-view.test.tsx` covers the surface that replaced it.
})
