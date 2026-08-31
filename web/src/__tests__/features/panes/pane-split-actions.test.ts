import { beforeEach, describe, expect, it } from 'vitest'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { windowPaneStore, resetWindowPaneStoreForTests } from '@/features/panes/stores/window-pane-store'
import { getAllLeafIds } from '@/features/panes/utils/pane-layout'

describe('pane split actions', () => {
  beforeEach(() => {
    resetWindowPaneStoreForTests()
  })

  it('creates an adjacent pane and activates it', async () => {
    const { createPaneBeside } = await import('@/features/panes/utils/pane-split-actions')

    const paneId = createPaneBeside(ROOT_PANE_ID, 'horizontal')

    expect(paneId).not.toBeNull()
    const rootIds = getAllLeafIds(windowPaneStore.getState().rootLayout)
    expect(rootIds).toHaveLength(2)
    expect(windowPaneStore.getState().activePaneId).toBe(paneId)
  })

  it('can seed the adjacent pane with a shared buffer', async () => {
    const { createPaneBeside } = await import('@/features/panes/utils/pane-split-actions')
    const paneActions = windowPaneStore.getState().paneActions

    paneActions.addEditorTabToPane(ROOT_PANE_ID, {
      id: 'buffer-a',
      type: 'editor',
      name: 'a.ts',
      workspaceId: 'test-ws',
    })

    const paneId = createPaneBeside(ROOT_PANE_ID, 'horizontal', 'after', 'buffer-a')

    expect(paneId).not.toBeNull()
    if (!paneId) return
    expect(paneActions.getPaneById(paneId)?.editorTabIds).toEqual(['buffer-a'])
  })
})
