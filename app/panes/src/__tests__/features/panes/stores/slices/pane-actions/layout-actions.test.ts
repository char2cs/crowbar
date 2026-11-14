import { describe, it, expect } from 'vitest'
import { ROOT_PANE_ID, BOTTOM_PANE_ID } from '@/features/panes/constants/pane'
import { showingLayout } from '@/features/panes/lib/view-state'
import {
  createLeaf,
  createSplit,
  findSplit,
  getAllLeafIds,
} from '@/features/panes/utils/pane-layout'
import { buildViewState, editorTab, makePaneOnlyStore } from '@/__tests__/__fixtures__/view-state'

describe('splitPane', () => {
  it('returns a new chatless pane and splits the showing layout', () => {
    const store = makePaneOnlyStore()
    const id = store.getState().paneActions.splitPane(ROOT_PANE_ID, 'horizontal')
    expect(id).not.toBeNull()
    expect(showingLayout(store.getState()).type).toBe('split')
    const pane = store.getState().paneActions.getPaneById(id!)
    expect(pane?.chatId).toBeNull()
    expect(pane?.editorTabIds).toEqual([])
    expect(pane?.editorOpen).toBe(false)
    expect(store.getState().activePaneId).toBe(id)
  })

  it('seeds the new pane with the given tab, even one another pane shares', () => {
    const actions = makePaneOnlyStore().getState().paneActions
    actions.addEditorTabToPane(ROOT_PANE_ID, editorTab('tab-1'))
    const id = actions.splitPane(ROOT_PANE_ID, 'horizontal', 'tab-1')!
    const pane = actions.getPaneById(id)
    expect(pane?.editorTabIds).toEqual(['tab-1'])
    expect(pane?.activeEditorTabId).toBe('tab-1')
    expect(pane?.editorOpen).toBe(true)
  })

  it('a split inside a view joins that view and adds no row', () => {
    const store = makePaneOnlyStore()
    store.getState().paneActions.openChat('chat-1')
    const viewId = store.getState().activeViewId!
    const id = store.getState().paneActions.splitPane(ROOT_PANE_ID, 'vertical')!
    expect(store.getState().panes[id].viewId).toBe(viewId)
    expect(store.getState().viewOrder).toEqual([viewId])
  })

  it('splitting a view that is not on screen leaves focus where it was', () => {
    const store = makePaneOnlyStore()
    store.setState(
      buildViewState({
        views: [
          { id: 'v1', panes: [{ id: 'a', chatId: 'chat-a' }] },
          { id: 'v2', panes: [{ id: 'b', chatId: 'chat-b' }] },
        ],
        active: 'v1',
      }),
    )
    store.getState().paneActions.splitPane('b', 'horizontal')
    expect(store.getState().activePaneId).toBe('a')
  })
})

describe('focus and geometry', () => {
  it('getAllPaneGroups includes stage, split and bottom panes', () => {
    const actions = makePaneOnlyStore().getState().paneActions
    actions.splitPane(ROOT_PANE_ID, 'horizontal')
    const ids = actions.getAllPaneGroups().map((g) => g.id)
    expect(ids).toHaveLength(3)
    expect(ids).toContain(ROOT_PANE_ID)
    expect(ids).toContain(BOTTOM_PANE_ID)
  })

  it('fullscreen toggles and exits; lock sets the flag', () => {
    const store = makePaneOnlyStore()
    const actions = store.getState().paneActions
    actions.togglePaneFullscreen(ROOT_PANE_ID)
    expect(store.getState().fullscreenPaneId).toBe(ROOT_PANE_ID)
    actions.exitPaneFullscreen()
    expect(store.getState().fullscreenPaneId).toBeNull()
    actions.setPaneLocked(ROOT_PANE_ID, true)
    expect(actions.getPaneById(ROOT_PANE_ID)?.locked).toBe(true)
  })

  it('getActivePane follows setActivePane', () => {
    const actions = makePaneOnlyStore().getState().paneActions
    const id = actions.splitPane(ROOT_PANE_ID, 'horizontal')!
    expect(actions.getActivePane()?.id).toBe(id)
    actions.setActivePane(ROOT_PANE_ID)
    expect(actions.getActivePane()?.id).toBe(ROOT_PANE_ID)
  })

  it('focusing a pane in another view of the project brings that view on screen', () => {
    const store = makePaneOnlyStore()
    store.setState(
      buildViewState({
        views: [
          { id: 'v1', panes: [{ id: 'a', chatId: 'chat-a' }] },
          {
            id: 'v2',
            panes: [
              { id: 'b', chatId: 'chat-b' },
              { id: 'c', chatId: 'chat-c' },
            ],
          },
        ],
        active: 'v1',
        activeProjectId: 'p1',
      }),
    )
    store.getState().paneActions.setActivePane('c')
    expect(store.getState().activeViewId).toBe('v2')
    expect(store.getState().activePaneId).toBe('c')
  })

  it("focusing another project's pane only remembers it (Law 2)", () => {
    const store = makePaneOnlyStore()
    store.setState(
      buildViewState({
        views: [
          { id: 'v1', projectId: 'p1', panes: [{ id: 'a', chatId: 'chat-a' }] },
          { id: 'v2', projectId: 'p2', panes: [{ id: 'b', chatId: 'chat-b' }] },
        ],
        active: 'v1',
        activeProjectId: 'p1',
      }),
    )
    store.getState().paneActions.setActivePane('b')
    expect(store.getState().activeViewId).toBe('v1')
    expect(store.getState().activeViewByProject.p2).toBe('v2')
  })

  it('navigateToPane moves focus to the adjacent showing pane', () => {
    const store = makePaneOnlyStore()
    const actions = store.getState().paneActions
    const right = actions.splitPane(ROOT_PANE_ID, 'horizontal')!
    actions.setActivePane(ROOT_PANE_ID)
    actions.navigateToPane('right')
    expect(store.getState().activePaneId).toBe(right)
  })
})

describe('resizePaneSplit on a chained same-direction layout', () => {
  function chained() {
    const store = makePaneOnlyStore()
    const inner = createSplit('horizontal', createLeaf('a'), createLeaf('b'), [40, 60])
    const outer = createSplit('horizontal', inner, createLeaf('c'), [70, 30])
    const state = buildViewState({
      views: [
        {
          id: 'v1',
          panes: [
            { id: 'a', chatId: 'chat-a' },
            { id: 'b', chatId: 'chat-b' },
            { id: 'c', chatId: 'chat-c' },
          ],
        },
      ],
    })
    state.views.v1.layout = outer
    store.setState(state)
    return { store, inner, outer }
  }

  it('resizing the outer split leaves the inner split untouched', () => {
    const { store, inner, outer } = chained()
    store.getState().paneActions.resizePaneSplit(outer.id, [55, 45])
    const root = store.getState().views.v1.layout
    if (root.type !== 'split') throw new Error('expected split')
    expect(root.sizes[0]).toBeCloseTo(55)
    expect(findSplit(root, inner.id)?.sizes[0]).toBeCloseTo(40)
  })

  it('resizing the inner split leaves the outer split untouched', () => {
    const { store, inner, outer } = chained()
    store.getState().paneActions.resizePaneSplit(inner.id, [20, 80])
    const root = store.getState().views.v1.layout
    if (root.type !== 'split') throw new Error('expected split')
    expect(root.sizes[0]).toBeCloseTo(70)
    expect(findSplit(root, inner.id)?.sizes[0]).toBeCloseTo(20)
    expect(findSplit(root, outer.id)).not.toBeNull()
  })

  it('distributePaneSplit evens a split in the stage', () => {
    const store = makePaneOnlyStore()
    store.getState().paneActions.splitPane(ROOT_PANE_ID, 'horizontal')
    const split = store.getState().stage
    if (split.type !== 'split') throw new Error('expected split')
    store.getState().paneActions.resizePaneSplit(split.id, [30, 70])
    store.getState().paneActions.distributePaneSplit(split.id)
    const after = store.getState().stage
    if (after.type !== 'split') throw new Error('expected split')
    expect(after.sizes[0]).toBeCloseTo(50)
    expect(getAllLeafIds(after)).toHaveLength(2)
  })
})
