// web/src/__tests__/features/workspace/stores/slices/pane-slice.test.ts
import { describe, it, expect, beforeEach } from 'vitest'
import { createStore } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import { createPaneSlice, type PaneSlice } from '@/features/workspace/stores/slices/pane-slice'
import { ROOT_PANE_ID, BOTTOM_PANE_ID } from '@/features/panes/constants/pane'
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
    const rootGroup = store.getState().paneActions.getPaneById(ROOT_PANE_ID)
    expect(rootGroup).not.toBeNull()
    expect(rootGroup?.id).toBe(ROOT_PANE_ID)
    expect(rootGroup?.bufferIds).toEqual([])
    expect(rootGroup?.activeBufferId).toBeNull()
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
    const rootGroup = store.getState().paneActions.getPaneById(ROOT_PANE_ID)
    expect(rootGroup?.bufferIds).toContain('buf-1')
    expect(rootGroup?.activeBufferId).toBe('buf-1')
  })

  it('removeBufferFromPane removes bufferId from the group', () => {
    const actions = store.getState().paneActions
    actions.addBufferToPane(ROOT_PANE_ID, 'buf-1', true)
    actions.addBufferToPane(ROOT_PANE_ID, 'buf-2', false)
    actions.removeBufferFromPane(ROOT_PANE_ID, 'buf-1', true)
    const rootGroup = store.getState().paneActions.getPaneById(ROOT_PANE_ID)
    expect(rootGroup?.bufferIds).not.toContain('buf-1')
    expect(rootGroup?.bufferIds).toContain('buf-2')
  })

  it('getAllPaneGroups returns all leaf groups from paneRoot and bottomRoot', () => {
    const actions = store.getState().paneActions
    actions.splitPane(ROOT_PANE_ID, 'horizontal')
    const groups = actions.getAllPaneGroups()
    // 2 from the paneRoot split + 1 from bottomRoot
    expect(groups).toHaveLength(3)
    const ids = groups.map(g => g.id)
    expect(ids).toContain(ROOT_PANE_ID)
    expect(ids).toContain(BOTTOM_PANE_ID)
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

describe('pane-slice bottomRoot routing', () => {
  let store: ReturnType<typeof makeStore>

  beforeEach(() => {
    store = makeStore()
  })

  it('addBufferToPane adds to bottomRoot when paneId is BOTTOM_PANE_ID', () => {
    store.getState().paneActions.addBufferToPane(BOTTOM_PANE_ID, 'buf-1', true)
    const bottomGroup = store.getState().paneActions.getPaneById(BOTTOM_PANE_ID)
    expect(bottomGroup?.bufferIds).toContain('buf-1')
  })

  it('getAllPaneGroups includes groups from both paneRoot and bottomRoot', () => {
    const groups = store.getState().paneActions.getAllPaneGroups()
    const ids = groups.map(g => g.id)
    expect(ids).toContain(ROOT_PANE_ID)
    expect(ids).toContain(BOTTOM_PANE_ID)
  })

  it('getPaneById finds pane in bottomRoot', () => {
    const pane = store.getState().paneActions.getPaneById(BOTTOM_PANE_ID)
    expect(pane).not.toBeNull()
    expect(pane?.id).toBe(BOTTOM_PANE_ID)
  })

  it('activatePaneBuffer works for bottomRoot pane', () => {
    store.getState().paneActions.addBufferToPane(BOTTOM_PANE_ID, 'buf-bottom', false)
    store.getState().paneActions.activatePaneBuffer(BOTTOM_PANE_ID, 'buf-bottom')
    const bottomGroup = store.getState().paneActions.getPaneById(BOTTOM_PANE_ID)
    expect(bottomGroup?.activeBufferId).toBe('buf-bottom')
  })

  it('removeBufferFromPane removes buffer from bottomRoot pane', () => {
    const actions = store.getState().paneActions
    actions.addBufferToPane(BOTTOM_PANE_ID, 'buf-1', true)
    actions.addBufferToPane(BOTTOM_PANE_ID, 'buf-2', false)
    actions.removeBufferFromPane(BOTTOM_PANE_ID, 'buf-1', true)
    const bottomGroup = store.getState().paneActions.getPaneById(BOTTOM_PANE_ID)
    expect(bottomGroup?.bufferIds).not.toContain('buf-1')
    expect(bottomGroup?.bufferIds).toContain('buf-2')
  })

  it('getPaneByBufferId finds buffer in bottomRoot', () => {
    store.getState().paneActions.addBufferToPane(BOTTOM_PANE_ID, 'buf-bottom', true)
    const pane = store.getState().paneActions.getPaneByBufferId('buf-bottom')
    expect(pane).not.toBeNull()
    expect(pane?.id).toBe(BOTTOM_PANE_ID)
  })

  it('moveBufferToPane moves buffer across trees (bottomRoot -> paneRoot)', () => {
    const actions = store.getState().paneActions
    actions.addBufferToPane(BOTTOM_PANE_ID, 'buf-x', true)
    actions.moveBufferToPane('buf-x', BOTTOM_PANE_ID, ROOT_PANE_ID)
    const rootPaneGroup = store.getState().paneActions.getPaneById(ROOT_PANE_ID)
    expect(rootPaneGroup?.bufferIds).toContain('buf-x')
    const bottomPaneGroup = store.getState().paneActions.getPaneById(BOTTOM_PANE_ID)
    expect(bottomPaneGroup?.bufferIds).not.toContain('buf-x')
  })
})
