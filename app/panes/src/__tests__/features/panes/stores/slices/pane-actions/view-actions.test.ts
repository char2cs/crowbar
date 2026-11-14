import { describe, it, expect, vi } from 'vitest'
import { ROOT_PANE_ID, BOTTOM_PANE_ID } from '@/features/panes/constants/pane'
import { showingLayout, viewChatIds } from '@/features/panes/lib/view-state'
import { chatPaneIndex, isPaneEmpty } from '@/features/panes/lib/view-selectors'
import { getAllLeafIds } from '@/features/panes/utils/pane-layout'
import { buildViewState, editorTab, makePaneOnlyStore } from '@/__tests__/__fixtures__/view-state'

vi.mock('@/features/panes/lib/release-closed-chat', () => ({
  releaseClosedChat: vi.fn(async () => {}),
}))

/** The stage is on screen with nothing in it — the "nothing is open" screen. */
function expectEmptyStage(store: ReturnType<typeof makePaneOnlyStore>): void {
  const state = store.getState()
  expect(state.activeViewId).toBeNull()
  const leaves = getAllLeafIds(state.stage)
  expect(leaves).toHaveLength(1)
  expect(isPaneEmpty(state.panes[leaves[0]])).toBe(true)
}

type Store = ReturnType<typeof makePaneOnlyStore>

function open(store: Store, chatId: string, projectId?: string) {
  store.getState().paneActions.openChat(chatId, { projectId })
  const paneId = chatPaneIndex(store.getState().panes).get(chatId)!
  return { paneId, viewId: store.getState().panes[paneId].viewId! }
}

describe('openChat', () => {
  it('the first chat promotes the showing stage into a row', () => {
    const store = makePaneOnlyStore()
    store.getState().paneActions.setActiveProject('p1')
    const { paneId, viewId } = open(store, 'chat-1')
    const state = store.getState()
    expect(paneId).toBe(ROOT_PANE_ID)
    expect(state.viewOrder).toEqual([viewId])
    expect(state.views[viewId].projectId).toBe('p1')
    expect(state.activeViewId).toBe(viewId)
  })

  it('a second chat is a NEW row that takes the screen; the first keeps its row', () => {
    const store = makePaneOnlyStore()
    const first = open(store, 'chat-1')
    const second = open(store, 'chat-2')
    const state = store.getState()
    expect(state.viewOrder).toEqual([first.viewId, second.viewId])
    expect(state.activeViewId).toBe(second.viewId)
    expect(getAllLeafIds(showingLayout(state))).toEqual([second.paneId])
    expect(state.panes[first.paneId].chatId).toBe('chat-1')
  })

  it('never replaces what a pane shows — even the active one', () => {
    const store = makePaneOnlyStore()
    const first = open(store, 'chat-1')
    store.getState().paneActions.setActivePane(first.paneId)
    open(store, 'chat-2')
    expect(store.getState().panes[first.paneId].chatId).toBe('chat-1')
  })

  it('an already-open chat is revealed, in place, never opened twice', () => {
    const store = makePaneOnlyStore()
    const first = open(store, 'chat-1')
    open(store, 'chat-2')
    store.getState().paneActions.openChat('chat-1')
    const state = store.getState()
    expect(state.activeViewId).toBe(first.viewId)
    expect(state.activePaneId).toBe(first.paneId)
    expect(state.viewOrder).toHaveLength(2)
  })

  it('carries the runner it was given', () => {
    const store = makePaneOnlyStore()
    store.getState().paneActions.openChat('chat-1', { runnerId: 'r1' })
    expect(store.getState().panes[ROOT_PANE_ID].runnerId).toBe('r1')
  })

  it('the stage keeps its editor tabs when a chat lands in it', () => {
    const store = makePaneOnlyStore()
    store.getState().paneActions.addEditorTabToPane(ROOT_PANE_ID, editorTab('file-1'))
    const { paneId } = open(store, 'chat-1')
    expect(paneId).toBe(ROOT_PANE_ID)
    expect(store.getState().panes[ROOT_PANE_ID].editorTabIds).toEqual(['file-1'])
  })
})

describe('closePane / closeView', () => {
  it('closing a view’s only chat removes its row — no leftover row, no second close', () => {
    const store = makePaneOnlyStore()
    const { paneId } = open(store, 'chat-1')
    store.getState().paneActions.closePane(paneId)
    expect(store.getState().viewOrder).toEqual([])
    expectEmptyStage(store)
  })

  it('closing the showing view REVEALS the one behind it', () => {
    const store = makePaneOnlyStore()
    const first = open(store, 'chat-1')
    const second = open(store, 'chat-2')
    store.getState().paneActions.closeView(second.viewId)
    expect(store.getState().activeViewId).toBe(first.viewId)
  })

  it('closeView ends every pane in the view, and never touches a sibling view', () => {
    const store = makePaneOnlyStore()
    const other = open(store, 'chat-0')
    const { viewId } = open(store, 'chat-1')
    store
      .getState()
      .paneActions.dropChatOnPane(
        'chat-2',
        chatPaneIndex(store.getState().panes).get('chat-1')!,
        'right',
      )
    store.getState().paneActions.closeView(viewId)
    const state = store.getState()
    expect(state.views[viewId]).toBeUndefined()
    expect(chatPaneIndex(state.panes).has('chat-1')).toBe(false)
    expect(chatPaneIndex(state.panes).has('chat-2')).toBe(false)
    expect(state.viewOrder).toEqual([other.viewId])
  })

  it('closing one member of a group leaves the rest showing and grouped', () => {
    const store = makePaneOnlyStore()
    const { paneId, viewId } = open(store, 'chat-1')
    store.getState().paneActions.dropChatOnPane('chat-2', paneId, 'right')
    store.getState().paneActions.dropChatOnPane('chat-3', paneId, 'bottom')
    store.getState().paneActions.closePane(chatPaneIndex(store.getState().panes).get('chat-2')!)
    expect(viewChatIds(store.getState(), viewId)).toEqual(['chat-1', 'chat-3'])
    expect(store.getState().activeViewId).toBe(viewId)
  })

  it('closing a split merges its tabs into the survivor, never its chat', () => {
    const store = makePaneOnlyStore()
    const actions = store.getState().paneActions
    actions.addEditorTabToPane(ROOT_PANE_ID, editorTab('root-tab'))
    const split = actions.splitPane(ROOT_PANE_ID, 'horizontal')!
    actions.addEditorTabToPane(split, editorTab('split-tab'))
    actions.closePane(split)
    expect(actions.getPaneById(ROOT_PANE_ID)?.editorTabIds).toEqual(['root-tab', 'split-tab'])
    expect(actions.getPaneById(split)).toBeNull()
  })

  it('does not duplicate a tab the survivor already holds', () => {
    const store = makePaneOnlyStore()
    const actions = store.getState().paneActions
    actions.addEditorTabToPane(ROOT_PANE_ID, editorTab('shared'))
    const split = actions.splitPane(ROOT_PANE_ID, 'horizontal', 'shared')!
    actions.closePane(split)
    expect(actions.getPaneById(ROOT_PANE_ID)?.editorTabIds).toEqual(['shared'])
  })

  it('closing the sole stage or bottom pane empties it, never deletes it', () => {
    const store = makePaneOnlyStore()
    const actions = store.getState().paneActions
    actions.addEditorTabToPane(ROOT_PANE_ID, editorTab('t'))
    actions.addEditorTabToPane(BOTTOM_PANE_ID, editorTab('b', 'terminal'))
    actions.closePane(ROOT_PANE_ID)
    actions.closePane(BOTTOM_PANE_ID)
    const state = store.getState()
    const stageLeaves = getAllLeafIds(state.stage)
    expect(stageLeaves).toHaveLength(1)
    expect(state.panes[stageLeaves[0]].editorTabIds).toEqual([])
    expect(state.panes[BOTTOM_PANE_ID].editorTabIds).toEqual([])
  })
})

describe('detachPane', () => {
  it('a group member becomes its own row, placed right after the group', () => {
    const store = makePaneOnlyStore()
    const other = open(store, 'chat-0')
    const { paneId, viewId } = open(store, 'chat-1')
    store.getState().paneActions.reorderView(viewId, other.viewId, 'before')
    store.getState().paneActions.dropChatOnPane('chat-2', paneId, 'right')
    const member = chatPaneIndex(store.getState().panes).get('chat-2')!

    store.getState().paneActions.detachPane(member)

    const state = store.getState()
    const detached = state.panes[member].viewId!
    expect(state.viewOrder).toEqual([viewId, detached, other.viewId])
    expect(viewChatIds(state, viewId)).toEqual(['chat-1'])
    expect(viewChatIds(state, detached)).toEqual(['chat-2'])
    expect(state.activeViewId).toBe(detached)
  })

  it('is a no-op for a view’s only chat', () => {
    const store = makePaneOnlyStore()
    const { paneId } = open(store, 'chat-1')
    const before = store.getState().views
    store.getState().paneActions.detachPane(paneId)
    expect(store.getState().views).toBe(before)
  })
})

describe('reorderView / activateView', () => {
  function three() {
    const store = makePaneOnlyStore()
    store.setState(
      buildViewState({
        views: ['v1', 'v2', 'v3'].map((id) => ({
          id,
          panes: [{ id: `p-${id}`, chatId: `c-${id}` }],
        })),
        activeProjectId: 'p1',
      }),
    )
    return store
  }

  it('moves an id before / after another in viewOrder', () => {
    const store = three()
    store.getState().paneActions.reorderView('v3', 'v1', 'before')
    expect(store.getState().viewOrder).toEqual(['v3', 'v1', 'v2'])
    store.getState().paneActions.reorderView('v3', 'v2', 'after')
    expect(store.getState().viewOrder).toEqual(['v1', 'v2', 'v3'])
  })

  it('reordering never adds or removes a row', () => {
    const store = three()
    store.getState().paneActions.reorderView('v1', 'missing', 'after')
    store.getState().paneActions.reorderView('v1', 'v1', 'after')
    expect(store.getState().viewOrder).toEqual(['v1', 'v2', 'v3'])
  })

  it('activateView is a pointer write: the showing layout is the record’s', () => {
    const store = three()
    const layout = store.getState().views.v2.layout
    store.getState().paneActions.activateView('v2')
    expect(store.getState().activeViewId).toBe('v2')
    expect(showingLayout(store.getState())).toBe(layout)
    expect(store.getState().activePaneId).toBe('p-v2')
  })

  it('a switched-to view restores the pane last focused in it', () => {
    const store = three()
    store.getState().paneActions.dropChatOnPane('c-new', 'p-v2', 'right')
    store.getState().paneActions.setActivePane('p-v2')
    store.getState().paneActions.activateView('v1')
    store.getState().paneActions.activateView('v2')
    expect(store.getState().activePaneId).toBe('p-v2')
  })
})

describe('setViewPinned', () => {
  function three() {
    const store = makePaneOnlyStore()
    store.setState(
      buildViewState({
        views: ['v1', 'v2', 'v3'].map((id) => ({
          id,
          panes: [{ id: `p-${id}`, chatId: `c-${id}` }],
        })),
        activeProjectId: 'p1',
      }),
    )
    return store
  }

  it('moves a pinned view to the end of the pinned group, and back to the start of the rest', () => {
    const store = three()
    store.getState().paneActions.setViewPinned('v3', true)
    expect(store.getState().viewOrder).toEqual(['v3', 'v1', 'v2'])
    store.getState().paneActions.setViewPinned('v2', true)
    expect(store.getState().viewOrder).toEqual(['v3', 'v2', 'v1'])
    store.getState().paneActions.setViewPinned('v3', false)
    expect(store.getState().viewOrder).toEqual(['v2', 'v3', 'v1'])
    expect(store.getState().views.v3.pinned).toBeUndefined()
    expect(store.getState().views.v2.pinned).toBe(true)
  })

  it('reordering never crosses the pinned boundary', () => {
    const store = three()
    store.getState().paneActions.setViewPinned('v3', true)
    store.getState().paneActions.setViewPinned('v2', true)
    store.getState().paneActions.reorderView('v1', 'v3', 'before')
    expect(store.getState().viewOrder).toEqual(['v3', 'v2', 'v1'])
    store.getState().paneActions.reorderView('v2', 'v1', 'after')
    expect(store.getState().viewOrder).toEqual(['v3', 'v2', 'v1'])
    store.getState().paneActions.reorderView('v3', 'v2', 'after')
    expect(store.getState().viewOrder).toEqual(['v2', 'v3', 'v1'])
  })

  it('a chat detached from a pinned group becomes an unpinned view behind the pinned ones', () => {
    const store = three()
    store.getState().paneActions.setViewPinned('v1', true)
    store.getState().paneActions.dropChatOnPane('c-new', 'p-v1', 'right')
    store.getState().paneActions.detachPane(chatPaneIndex(store.getState().panes).get('c-new')!)
    const { viewOrder, views } = store.getState()
    expect(viewOrder[0]).toBe('v1')
    expect(views.v1.pinned).toBe(true)
    expect(viewOrder.slice(1).every((id) => !views[id].pinned)).toBe(true)
  })
})
