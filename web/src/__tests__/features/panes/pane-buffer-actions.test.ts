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

  it('adds missing buffers to an existing pane', async () => {
    const { ensureBufferInPane } = await import('@/features/panes/utils/pane-buffer-actions')

    expect(ensureBufferInPane(ROOT_PANE_ID, 'buffer-a')).toBe(ROOT_PANE_ID)
    expect(wsStore.getState().panes[ROOT_PANE_ID]?.bufferIds).toEqual(['buffer-a'])
    expect(wsStore.getState().activePaneId).toBe(ROOT_PANE_ID)
  })

  it('activates existing buffers without duplicating them', async () => {
    const { ensureBufferInPane } = await import('@/features/panes/utils/pane-buffer-actions')
    const paneActions = wsStore.getState().paneActions

    paneActions.addBufferToPane(ROOT_PANE_ID, 'buffer-a')
    paneActions.addBufferToPane(ROOT_PANE_ID, 'buffer-b')

    expect(ensureBufferInPane(ROOT_PANE_ID, 'buffer-a')).toBe(ROOT_PANE_ID)
    expect(paneActions.getPaneById(ROOT_PANE_ID)?.bufferIds).toEqual(['buffer-a', 'buffer-b'])
    expect(paneActions.getPaneById(ROOT_PANE_ID)?.activeBufferId).toBe('buffer-a')
  })

  it('returns null for missing panes', async () => {
    const { ensureBufferInPane } = await import('@/features/panes/utils/pane-buffer-actions')

    expect(ensureBufferInPane('missing-pane', 'buffer-a')).toBeNull()
  })
})
