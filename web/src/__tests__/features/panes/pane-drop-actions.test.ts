import { beforeEach, describe, expect, it } from 'vitest'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { windowPaneStore, resetWindowPaneStoreForTests } from '@/features/panes/stores/window-pane-store'
import { getAllLeafIds } from '@/features/panes/utils/pane-layout'

describe('pane drop actions', () => {
  beforeEach(() => {
    resetWindowPaneStoreForTests()
  })

  it('creates a split drop target from an edge zone', async () => {
    const { getOrCreatePaneDropTarget } = await import('@/features/panes/utils/pane-drop-actions')

    const targetPaneId = getOrCreatePaneDropTarget({ paneId: ROOT_PANE_ID, zone: 'right' })

    expect(targetPaneId).not.toBeNull()
    expect(targetPaneId).not.toBe(ROOT_PANE_ID)
    const rootIds = getAllLeafIds(windowPaneStore.getState().rootLayout)
    expect(rootIds).toHaveLength(2)
  })

  it('moves buffers through a pane drop target', async () => {
    const { moveBufferToPaneDropTarget } = await import('@/features/panes/utils/pane-drop-actions')
    const paneActions = windowPaneStore.getState().paneActions

    paneActions.addEditorTabToPane(ROOT_PANE_ID, {
      id: 'buffer-a',
      type: 'editor',
      name: 'a.ts',
      workspaceId: 'test-ws',
    })
    paneActions.addEditorTabToPane(ROOT_PANE_ID, {
      id: 'buffer-b',
      type: 'editor',
      name: 'b.ts',
      workspaceId: 'test-ws',
    })

    const targetPaneId = moveBufferToPaneDropTarget('buffer-a', ROOT_PANE_ID, {
      paneId: ROOT_PANE_ID,
      zone: 'right',
    })

    expect(targetPaneId).not.toBeNull()
    if (!targetPaneId) return
    expect(paneActions.getPaneById(ROOT_PANE_ID)?.editorTabIds).toEqual(['buffer-b'])
    expect(paneActions.getPaneById(targetPaneId)?.editorTabIds).toEqual(['buffer-a'])
    expect(windowPaneStore.getState().activePaneId).toBe(targetPaneId)
  })

  it('adds buffers without duplicating existing target entries', async () => {
    const { ensureBufferInPaneDropTarget } =
      await import('@/features/panes/utils/pane-drop-actions')
    const paneActions = windowPaneStore.getState().paneActions

    paneActions.addEditorTabToPane(ROOT_PANE_ID, {
      id: 'buffer-a',
      type: 'editor',
      name: 'a.ts',
      workspaceId: 'test-ws',
    })

    expect(ensureBufferInPaneDropTarget('buffer-a', { paneId: ROOT_PANE_ID, zone: 'center' })).toBe(
      ROOT_PANE_ID,
    )
    expect(paneActions.getPaneById(ROOT_PANE_ID)?.editorTabIds).toEqual(['buffer-a'])
  })
})
