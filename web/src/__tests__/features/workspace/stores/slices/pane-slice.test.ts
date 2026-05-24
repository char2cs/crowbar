// web/src/__tests__/features/workspace/stores/slices/pane-slice.test.ts
import { describe, it, expect, beforeEach } from 'vitest'
import { createStore } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import { createPaneSlice, type PaneSlice } from '@/features/workspace/stores/slices/pane-slice'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { getAllPaneGroups } from '@/features/panes/utils/pane-tree'

function makeStore() {
  return createStore<PaneSlice>()(immer((set, get) => createPaneSlice(set as any, get as any, {} as any)))
}

describe('pane-slice', () => {
  let store: ReturnType<typeof makeStore>

  beforeEach(() => {
    store = makeStore()
  })

  it('initialises with a single empty root group', () => {
    const { paneRoot } = store.getState()
    expect(paneRoot.type).toBe('group')
    expect(paneRoot.id).toBe(ROOT_PANE_ID)
    if (paneRoot.type === 'group') {
      expect(paneRoot.bufferIds).toEqual([])
      expect(paneRoot.activeBufferId).toBeNull()
    }
  })

  it('splitPane returns a new pane ID and updates root to a split', () => {
    const newPaneId = store.getState().paneActions.splitPane(ROOT_PANE_ID, 'horizontal')
    expect(newPaneId).not.toBeNull()
    const root = store.getState().paneRoot
    expect(root.type).toBe('split')
    const groups = getAllPaneGroups(root)
    expect(groups.map(g => g.id)).toContain(newPaneId!)
  })

  it('addBufferToPane adds bufferId to the correct group', () => {
    store.getState().paneActions.addBufferToPane(ROOT_PANE_ID, 'buf-1', true)
    const root = store.getState().paneRoot
    expect(root.type).toBe('group')
    if (root.type === 'group') {
      expect(root.bufferIds).toContain('buf-1')
      expect(root.activeBufferId).toBe('buf-1')
    }
  })

  it('removeBufferFromPane removes bufferId from the group', () => {
    const actions = store.getState().paneActions
    actions.addBufferToPane(ROOT_PANE_ID, 'buf-1', true)
    actions.addBufferToPane(ROOT_PANE_ID, 'buf-2', false)
    actions.removeBufferFromPane(ROOT_PANE_ID, 'buf-1', true)
    const root = store.getState().paneRoot
    if (root.type === 'group') {
      expect(root.bufferIds).not.toContain('buf-1')
      expect(root.bufferIds).toContain('buf-2')
    }
  })

  it('getAllPaneGroups returns all leaf groups', () => {
    const actions = store.getState().paneActions
    actions.splitPane(ROOT_PANE_ID, 'horizontal')
    const groups = actions.getAllPaneGroups()
    expect(groups).toHaveLength(2)
  })

  it('togglePaneFullscreen sets fullscreenPaneId', () => {
    store.getState().paneActions.togglePaneFullscreen(ROOT_PANE_ID)
    expect(store.getState().fullscreenPaneId).toBe(ROOT_PANE_ID)
  })

  it('exitPaneFullscreen clears fullscreenPaneId', () => {
    store.getState().paneActions.togglePaneFullscreen(ROOT_PANE_ID)
    store.getState().paneActions.exitPaneFullscreen()
    expect(store.getState().fullscreenPaneId).toBeNull()
  })
})
