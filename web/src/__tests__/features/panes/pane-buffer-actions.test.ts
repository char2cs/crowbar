import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { setActiveWorkspaceStoreRef } from '@/features/workspace/stores/workspace-store-ref'
import type { WorkspaceStore } from '@/features/workspace/stores/workspace-store'

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

describe('pane buffer actions', () => {
  let wsStore: WorkspaceStore

  beforeEach(() => {
    vi.stubGlobal('localStorage', createMockStorage())
    vi.stubGlobal('window', {
      __TAURI_INTERNALS__: {
        invoke: vi.fn().mockResolvedValue([]),
        metadata: { currentWindow: { label: 'main' }, currentWebview: { label: 'main' } },
      },
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })
    wsStore = createWorkspaceStore('test-ws')
    setActiveWorkspaceStoreRef(wsStore)
  })

  afterEach(() => {
    setActiveWorkspaceStoreRef(null)
    vi.unstubAllGlobals()
  })

  it('adds a missing buffer to an existing pane', async () => {
    const { ensureBufferInPane } = await import('@/features/panes/utils/pane-buffer-actions')
    // addEditorTabToPane only registers a REFERENCE (see the comment on
    // ensureBufferInPane) — the buffer itself must already exist.
    wsStore.setState((state) => {
      state.buffers.push({
        id: 'buffer-a',
        type: 'editor',
        path: '/buffer-a.ts',
        name: 'buffer-a.ts',
        content: '',
        savedContent: '',
        isDirty: false,
        isVirtual: false,
        tokens: [],
        isPinned: false,
        isPreview: false,
      })
      return state
    })

    expect(ensureBufferInPane(ROOT_PANE_ID, 'buffer-a')).toBe(ROOT_PANE_ID)
    expect(wsStore.getState().panes[ROOT_PANE_ID]?.editorTabIds).toEqual(['buffer-a'])
    expect(wsStore.getState().activePaneId).toBe(ROOT_PANE_ID)
  })

  it('activates existing buffers without duplicating them', async () => {
    const { ensureBufferInPane } = await import('@/features/panes/utils/pane-buffer-actions')
    const paneActions = wsStore.getState().paneActions

    paneActions.addEditorTabToPane(ROOT_PANE_ID, { id: 'buffer-a', type: 'editor', name: 'a.ts' })
    paneActions.addEditorTabToPane(ROOT_PANE_ID, { id: 'buffer-b', type: 'editor', name: 'b.ts' })

    expect(ensureBufferInPane(ROOT_PANE_ID, 'buffer-a')).toBe(ROOT_PANE_ID)
    expect(paneActions.getPaneById(ROOT_PANE_ID)?.editorTabIds).toEqual(['buffer-a', 'buffer-b'])
    expect(paneActions.getPaneById(ROOT_PANE_ID)?.activeEditorTabId).toBe('buffer-a')
  })

  it('does nothing when the buffer does not exist anywhere (nothing to reference)', async () => {
    const { ensureBufferInPane } = await import('@/features/panes/utils/pane-buffer-actions')

    expect(ensureBufferInPane(ROOT_PANE_ID, 'ghost-buffer')).toBe(ROOT_PANE_ID)
    expect(wsStore.getState().panes[ROOT_PANE_ID]?.editorTabIds).toEqual([])
  })

  it('returns null for missing panes', async () => {
    const { ensureBufferInPane } = await import('@/features/panes/utils/pane-buffer-actions')

    expect(ensureBufferInPane('missing-pane', 'buffer-a')).toBeNull()
  })
})
