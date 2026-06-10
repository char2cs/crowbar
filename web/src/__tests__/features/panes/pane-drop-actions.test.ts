import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { setActiveWorkspaceStoreRef } from '@/features/workspace/stores/workspace-store-ref'
import { getAllLeafIds } from '@/features/panes/utils/pane-layout'
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

describe('pane drop actions', () => {
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

  it('creates a split drop target from an edge zone', async () => {
    const { getOrCreatePaneDropTarget } = await import('@/features/panes/utils/pane-drop-actions')

    const targetPaneId = getOrCreatePaneDropTarget({ paneId: ROOT_PANE_ID, zone: 'right' })

    expect(targetPaneId).not.toBeNull()
    expect(targetPaneId).not.toBe(ROOT_PANE_ID)
    const rootIds = getAllLeafIds(wsStore.getState().rootLayout)
    expect(rootIds).toHaveLength(2)
  })

  it('moves buffers through a pane drop target', async () => {
    const { moveBufferToPaneDropTarget } = await import('@/features/panes/utils/pane-drop-actions')
    const paneActions = wsStore.getState().paneActions

    paneActions.addBufferToPane(ROOT_PANE_ID, 'buffer-a')
    paneActions.addBufferToPane(ROOT_PANE_ID, 'buffer-b')

    const targetPaneId = moveBufferToPaneDropTarget('buffer-a', ROOT_PANE_ID, {
      paneId: ROOT_PANE_ID,
      zone: 'right',
    })

    expect(targetPaneId).not.toBeNull()
    if (!targetPaneId) return
    expect(paneActions.getPaneById(ROOT_PANE_ID)?.bufferIds).toEqual(['buffer-b'])
    expect(paneActions.getPaneById(targetPaneId)?.bufferIds).toEqual(['buffer-a'])
    expect(wsStore.getState().activePaneId).toBe(targetPaneId)
  })

  it('adds buffers without duplicating existing target entries', async () => {
    const { ensureBufferInPaneDropTarget } =
      await import('@/features/panes/utils/pane-drop-actions')
    const paneActions = wsStore.getState().paneActions

    paneActions.addBufferToPane(ROOT_PANE_ID, 'buffer-a')

    expect(ensureBufferInPaneDropTarget('buffer-a', { paneId: ROOT_PANE_ID, zone: 'center' })).toBe(
      ROOT_PANE_ID,
    )
    expect(paneActions.getPaneById(ROOT_PANE_ID)?.bufferIds).toEqual(['buffer-a'])
  })
})
