import { describe, it, expect } from 'vitest'
import { ROOT_PANE_ID, BOTTOM_PANE_ID } from '@/features/panes/constants/pane'
import { showingLayout, viewChatIds } from '@/features/panes/lib/view-state'
import { getAllLeafIds } from '@/features/panes/utils/pane-layout'
import { buildViewState, editorTab, makePaneOnlyStore } from '@/__tests__/__fixtures__/view-state'

const twoViews = () => {
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
  return store
}

describe('dropChatOnPane', () => {
  it('the middle of the empty stage fills it in place and makes it a row', () => {
    const store = makePaneOnlyStore()
    store.getState().paneActions.setActiveProject('p1')
    store.getState().paneActions.dropChatOnPane('chat-1', ROOT_PANE_ID, 'center')
    const state = store.getState()
    expect(state.panes[ROOT_PANE_ID].chatId).toBe('chat-1')
    expect(state.viewOrder).toEqual([state.panes[ROOT_PANE_ID].viewId])
    expect(state.views[state.viewOrder[0]].projectId).toBe('p1')
    expect(state.activePaneId).toBe(ROOT_PANE_ID)
  })

  it('the middle of a chatless editor split joins that view', () => {
    const store = makePaneOnlyStore()
    const actions = store.getState().paneActions
    actions.openChat('chat-1')
    const editorPane = actions.splitPane(ROOT_PANE_ID, 'horizontal', 'file-1')!
    actions.dropChatOnPane('chat-2', editorPane, 'center')
    const viewId = store.getState().panes[ROOT_PANE_ID].viewId!
    expect(viewChatIds(store.getState(), viewId)).toEqual(['chat-1', 'chat-2'])
    expect(store.getState().viewOrder).toEqual([viewId])
  })

  it('an edge of an occupied pane splits it — the chat joins the target’s view', () => {
    const store = twoViews()
    store.getState().paneActions.dropChatOnPane('chat-new', 'a', 'left')
    const state = store.getState()
    expect(viewChatIds(state, 'v1')).toEqual(['chat-new', 'chat-a'])
    expect(state.viewOrder).toEqual(['v1', 'v2'])
    expect(state.panes[state.activePaneId].chatId).toBe('chat-new')
  })

  it('the middle of an occupied pane never swaps it — it adds on the right', () => {
    const store = twoViews()
    store.getState().paneActions.dropChatOnPane('chat-new', 'a', 'center')
    expect(viewChatIds(store.getState(), 'v1')).toEqual(['chat-a', 'chat-new'])
  })

  it('an already-open chat is MOVED; its old view goes when it held nothing else', () => {
    const store = twoViews()
    store.getState().paneActions.dropChatOnPane('chat-a', 'b', 'right')
    const state = store.getState()
    expect(state.views.v1).toBeUndefined()
    expect(viewChatIds(state, 'v2')).toEqual(['chat-b', 'chat-a', 'chat-c'])
    expect(state.panes.a.viewId).toBe('v2')
    expect(state.activeViewId).toBe('v2')
    expect(state.activePaneId).toBe('a')
  })

  it('a move keeps a multi-chat source view standing, minus the pane that left', () => {
    const store = twoViews()
    store.getState().paneActions.dropChatOnPane('chat-c', 'a', 'bottom')
    expect(viewChatIds(store.getState(), 'v2')).toEqual(['chat-b'])
    expect(viewChatIds(store.getState(), 'v1')).toEqual(['chat-a', 'chat-c'])
  })

  it('carries the moved pane’s own state across — no close, no reopen', () => {
    const store = twoViews()
    store.getState().paneActions.addEditorTabToPane('a', editorTab('file-a'))
    store.getState().paneActions.setPaneRunner('a', 'runner-a')
    store.getState().paneActions.dropChatOnPane('chat-a', 'b', 'right')
    const moved = store.getState().panes.a
    expect(moved.runnerId).toBe('runner-a')
    expect(moved.editorTabIds).toEqual(['file-a'])
  })

  it('dropping onto the pane already showing the chat just focuses it', () => {
    const store = twoViews()
    const before = store.getState().views
    store.getState().paneActions.dropChatOnPane('chat-b', 'b', 'right')
    expect(store.getState().views).toBe(before)
    expect(store.getState().activeViewId).toBe('v2')
  })

  it('the middle of an empty pane reveals an already-open chat instead of duplicating it', () => {
    const store = twoViews()
    store.getState().paneActions.splitPane('a', 'horizontal', 'file-1')
    const editorPane = getAllLeafIds(showingLayout(store.getState())).find(
      (id) => store.getState().panes[id].chatId === null,
    )!
    store.getState().paneActions.dropChatOnPane('chat-c', editorPane, 'center')
    expect(store.getState().panes[editorPane].chatId).toBeNull()
    expect(store.getState().activeViewId).toBe('v2')
  })

  it('never lands a chat in the bottom tray', () => {
    const store = twoViews()
    const before = store.getState().panes
    store.getState().paneActions.dropChatOnPane('chat-new', BOTTOM_PANE_ID, 'center')
    store.getState().paneActions.dropChatOnPane('chat-new', BOTTOM_PANE_ID, 'top')
    expect(store.getState().panes).toBe(before)
  })
})
