import { beforeEach, describe, expect, it } from 'vitest'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { resetWindowPaneStoreForTests } from '@/__tests__/__fixtures__/window-pane-store'
import { seatTab } from '@/__tests__/__fixtures__/view-state'
import { showingLayout } from '@/features/panes/lib/view-state'
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
    const rootIds = getAllLeafIds(showingLayout(windowPaneStore.getState()))
    expect(rootIds).toHaveLength(2)
  })

  it('adds buffers without duplicating existing target entries', async () => {
    const { ensureBufferInPaneDropTarget } =
      await import('@/features/panes/utils/pane-drop-actions')
    const paneActions = windowPaneStore.getState().paneActions

    seatTab(windowPaneStore, ROOT_PANE_ID, {
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
