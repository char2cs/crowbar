import { beforeEach, describe, expect, it } from 'vitest'
import { BOTTOM_PANE_ID, ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { windowPaneStore, resetWindowPaneStoreForTests } from '@/features/panes/stores/window-pane-store'
import { getAllLeafIds } from '@/features/panes/utils/pane-layout'

describe('pane command actions', () => {
  beforeEach(() => {
    resetWindowPaneStoreForTests()
  })

  it('splits the active editor group with an editor buffer', async () => {
    const { splitActiveEditorGroup } = await import('@/features/panes/utils/pane-command-actions')
    const paneActions = windowPaneStore.getState().paneActions

    windowPaneStore.setState((state) => ({
      ...state,
      buffers: [
        {
          id: 'buffer-a',
          type: 'editor',
          path: '/workspace/a.ts',
          name: 'a.ts',
          isPinned: false,
          isPreview: false,
          isActive: true,
          content: '',
          savedContent: '',
          isDirty: false,
          isVirtual: false,
          tokens: [],
          workspaceId: 'test-ws',
        },
      ],
    }))

    paneActions.addBufferToPane(ROOT_PANE_ID, 'buffer-a')

    expect(splitActiveEditorGroup('horizontal')).toBe(true)

    const rootIds = getAllLeafIds(windowPaneStore.getState().rootLayout)
    expect(rootIds).toHaveLength(2)
    for (const id of rootIds) {
      expect(windowPaneStore.getState().panes[id]?.bufferIds).toContain('buffer-a')
    }
  })

  it('splits stateful buffers into an empty editor group', async () => {
    const { splitActiveEditorGroup } = await import('@/features/panes/utils/pane-command-actions')
    const paneActions = windowPaneStore.getState().paneActions

    windowPaneStore.setState((state) => ({
      ...state,
      buffers: [
        {
          id: 'terminal-a',
          type: 'terminal',
          path: 'terminal://terminal-a',
          name: 'Terminal',
          isPinned: false,
          isPreview: false,
          isActive: true,
          sessionId: 'terminal-a',
          workspaceId: 'test-ws',
        },
      ],
    }))

    paneActions.addBufferToPane(ROOT_PANE_ID, 'terminal-a')

    expect(splitActiveEditorGroup('horizontal')).toBe(true)

    const rootIds = getAllLeafIds(windowPaneStore.getState().rootLayout)
    expect(rootIds).toHaveLength(2)
    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.bufferIds).toEqual(['terminal-a'])
    const newPaneId = rootIds.find((id) => id !== ROOT_PANE_ID)
    expect(newPaneId).toBeDefined()
    if (newPaneId) expect(windowPaneStore.getState().panes[newPaneId]?.bufferIds).toEqual([])
  })

  it('closes only when another editor group can receive the buffers', async () => {
    const { closeActiveEditorGroup } = await import('@/features/panes/utils/pane-command-actions')
    const paneActions = windowPaneStore.getState().paneActions

    paneActions.addBufferToPane(ROOT_PANE_ID, 'buffer-a')
    expect(closeActiveEditorGroup()).toBe(false)

    const splitPaneId = paneActions.splitPane(ROOT_PANE_ID, 'horizontal')
    expect(splitPaneId).not.toBeNull()
    if (!splitPaneId) return

    paneActions.setActivePane(splitPaneId)
    expect(closeActiveEditorGroup()).toBe(true)
    const rootIds = getAllLeafIds(windowPaneStore.getState().rootLayout)
    expect(rootIds).toHaveLength(1)
    expect(paneActions.getPaneById(ROOT_PANE_ID)?.bufferIds).toEqual(['buffer-a'])
  })

  it('closes other editor groups into the active editor group', async () => {
    const { closeOtherEditorGroups } = await import('@/features/panes/utils/pane-command-actions')
    const paneActions = windowPaneStore.getState().paneActions

    paneActions.addBufferToPane(ROOT_PANE_ID, 'buffer-a')
    const rightPaneId = paneActions.splitPane(ROOT_PANE_ID, 'horizontal')
    expect(rightPaneId).not.toBeNull()
    if (!rightPaneId) return

    paneActions.addBufferToPane(rightPaneId, 'buffer-b')
    paneActions.setActivePane(ROOT_PANE_ID)

    expect(closeOtherEditorGroups()).toBe(true)
    const rootIds = getAllLeafIds(windowPaneStore.getState().rootLayout)
    expect(rootIds).toHaveLength(1)
    expect(paneActions.getPaneById(ROOT_PANE_ID)?.bufferIds).toContain('buffer-a')
    expect(paneActions.getPaneById(ROOT_PANE_ID)?.bufferIds).toContain('buffer-b')
    expect(windowPaneStore.getState().activePaneId).toBe(ROOT_PANE_ID)
  })

  it('resets nested editor group sizes', async () => {
    const { resetEditorGroupSizes } = await import('@/features/panes/utils/pane-command-actions')
    const paneActions = windowPaneStore.getState().paneActions

    const rightPaneId = paneActions.splitPane(ROOT_PANE_ID, 'horizontal')
    expect(rightPaneId).not.toBeNull()
    if (!rightPaneId) return

    const bottomRightPaneId = windowPaneStore.getState().paneActions.splitPane(rightPaneId, 'vertical')
    expect(bottomRightPaneId).not.toBeNull()
    if (!bottomRightPaneId) return

    const rootLayout = windowPaneStore.getState().rootLayout
    expect(rootLayout.type).toBe('split')
    if (rootLayout.type !== 'split') return

    paneActions.resizePaneSplit(rootLayout.id, 0, [75, 25])

    expect(resetEditorGroupSizes()).toBe(true)

    const nextRoot = windowPaneStore.getState().rootLayout
    expect(nextRoot.type).toBe('split')
    if (nextRoot.type !== 'split') return
    expect(nextRoot.sizes).toEqual([50, 50])
  })

  it('moves the active editor into the next and previous editor group', async () => {
    const { moveActiveEditorToAdjacentGroup } =
      await import('@/features/panes/utils/pane-command-actions')
    const paneActions = windowPaneStore.getState().paneActions

    paneActions.addBufferToPane(ROOT_PANE_ID, 'buffer-a')
    paneActions.addBufferToPane(ROOT_PANE_ID, 'buffer-b')
    const rightPaneId = paneActions.splitPane(ROOT_PANE_ID, 'horizontal')
    expect(rightPaneId).not.toBeNull()
    if (!rightPaneId) return

    paneActions.activatePaneBuffer(ROOT_PANE_ID, 'buffer-a')
    expect(moveActiveEditorToAdjacentGroup('next')).toBe(true)

    expect(paneActions.getPaneById(ROOT_PANE_ID)?.bufferIds).toEqual(['buffer-b'])
    expect(paneActions.getPaneById(rightPaneId)?.bufferIds).toEqual(['buffer-a'])
    expect(windowPaneStore.getState().activePaneId).toBe(rightPaneId)

    expect(moveActiveEditorToAdjacentGroup('previous')).toBe(true)

    expect(paneActions.getPaneById(ROOT_PANE_ID)?.bufferIds).toContain('buffer-a')
    expect(windowPaneStore.getState().activePaneId).toBe(ROOT_PANE_ID)
  })

  it('does not run editor group commands against bottom pane splits', async () => {
    const {
      closeActiveEditorGroup,
      moveActiveEditorToAdjacentGroup,
      splitActiveEditorGroup,
      toggleActiveEditorGroupLock,
    } = await import('@/features/panes/utils/pane-command-actions')
    const paneActions = windowPaneStore.getState().paneActions

    paneActions.addBufferToPane(BOTTOM_PANE_ID, 'terminal-a')
    const splitPaneId = paneActions.splitPane(BOTTOM_PANE_ID, 'horizontal')
    expect(splitPaneId).not.toBeNull()
    if (!splitPaneId) return

    paneActions.addBufferToPane(splitPaneId, 'terminal-b')
    paneActions.setActivePane(splitPaneId)

    expect(splitActiveEditorGroup('horizontal')).toBe(false)
    expect(closeActiveEditorGroup()).toBe(false)
    expect(moveActiveEditorToAdjacentGroup('previous')).toBe(false)
    expect(toggleActiveEditorGroupLock()).toBe(false)
    const bottomIds = getAllLeafIds(windowPaneStore.getState().bottomLayout)
    expect(bottomIds).toHaveLength(2)
    expect(paneActions.getPaneById(splitPaneId)?.locked).toBeFalsy()
  })
})
