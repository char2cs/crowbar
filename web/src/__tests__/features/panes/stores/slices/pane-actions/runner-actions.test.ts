import { describe, it, expect } from 'vitest'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { showingLayout, viewChatIds } from '@/features/panes/lib/view-state'
import { getAllLeafIds } from '@/features/panes/utils/pane-layout'
import { buildViewState, editorTab, makePaneOnlyStore } from '@/__tests__/__fixtures__/view-state'

function orphanFree(state: ReturnType<ReturnType<typeof makePaneOnlyStore>['getState']>) {
  for (const id of getAllLeafIds(showingLayout(state))) {
    const pane = state.panes[id]
    const siblings = getAllLeafIds(showingLayout(state)).length > 1
    if (siblings && pane.chatId === null && pane.editorTabIds.length > 0) return false
  }
  return true
}

describe('retargetPane (the runner stream’s `moved` frame)', () => {
  it('/clear in a grouped view: same row, same group, new conversation', () => {
    const store = makePaneOnlyStore()
    store.setState(
      buildViewState({
        views: [
          {
            id: 'group',
            panes: [
              { id: 'a', chatId: 'chat-a', runnerId: 'r1' },
              { id: 'b', chatId: 'chat-b', runnerId: 'r2' },
            ],
          },
        ],
        activeProjectId: 'p1',
      }),
    )

    store.getState().paneActions.retargetPane('a', 'chat-fresh', 'r1')

    const state = store.getState()
    expect(state.viewOrder).toEqual(['group'])
    expect(viewChatIds(state, 'group')).toEqual(['chat-fresh', 'chat-b'])
    expect(state.panes.a.runnerId).toBe('r1')
    expect(state.activeViewId).toBe('group')
  })

  it('Law 4: a pane that already showed the entered chat goes, tabs to its survivor', () => {
    const store = makePaneOnlyStore()
    store.setState(
      buildViewState({
        views: [
          { id: 'v1', panes: [{ id: 'taker', chatId: 'chat-a', runnerId: 'r1' }] },
          {
            id: 'v2',
            panes: [
              { id: 'held', chatId: 'chat-x', editorTabIds: ['file-1'] },
              { id: 'mate', chatId: 'chat-m' },
            ],
          },
        ],
      }),
    )

    store.getState().paneActions.retargetPane('taker', 'chat-x', 'r1')

    const state = store.getState()
    expect(state.panes.held).toBeUndefined()
    expect(state.panes.mate.editorTabIds).toContain('file-1')
    expect(viewChatIds(state, 'v1')).toEqual(['chat-x'])
    expect(viewChatIds(state, 'v2')).toEqual(['chat-m'])
    expect(orphanFree(state)).toBe(true)
  })

  it('the evicted pane’s view goes when that was its last chat', () => {
    const store = makePaneOnlyStore()
    store.setState(
      buildViewState({
        views: [
          { id: 'v1', panes: [{ id: 'taker', chatId: 'chat-a', runnerId: 'r1' }] },
          { id: 'v2', panes: [{ id: 'held', chatId: 'chat-x' }] },
        ],
      }),
    )
    store.getState().paneActions.retargetPane('taker', 'chat-x', 'r1')
    expect(store.getState().viewOrder).toEqual(['v1'])
  })

  it('refuses a chatless pane (only a following pane can be retargeted)', () => {
    const store = makePaneOnlyStore()
    store.getState().paneActions.retargetPane(ROOT_PANE_ID, 'chat-x', 'r1')
    expect(store.getState().panes[ROOT_PANE_ID].chatId).toBeNull()
  })
})

describe('setPaneRunner', () => {
  it('changes only the runner; rows and chats are untouched', () => {
    const store = makePaneOnlyStore()
    store.getState().paneActions.openChat('chat-1')
    const before = store.getState().viewOrder
    store.getState().paneActions.setPaneRunner(ROOT_PANE_ID, 'r9')
    expect(store.getState().panes[ROOT_PANE_ID].runnerId).toBe('r9')
    expect(store.getState().panes[ROOT_PANE_ID].chatId).toBe('chat-1')
    expect(store.getState().viewOrder).toBe(before)
  })
})

describe('forgetChat (spec §9)', () => {
  it('removes the deleted chat’s pane; its tabs go to the survivor', () => {
    const store = makePaneOnlyStore()
    const actions = store.getState().paneActions
    actions.openChat('chat-1')
    actions.addEditorTabToPane(ROOT_PANE_ID, editorTab('file-1'))
    actions.dropChatOnPane('chat-2', ROOT_PANE_ID, 'right')
    const sibling = actions.getAllPaneGroups().find((p) => p.chatId === 'chat-2')!.id

    actions.forgetChat('chat-1')

    const state = store.getState()
    expect(getAllLeafIds(showingLayout(state))).toEqual([sibling])
    expect(state.panes[ROOT_PANE_ID]).toBeUndefined()
    expect(state.panes[sibling].editorTabIds).toContain('file-1')
    expect(orphanFree(state)).toBe(true)
  })

  it('removes the view when that was its last chat, and the stage comes back', () => {
    const store = makePaneOnlyStore()
    store.getState().paneActions.openChat('chat-1')
    store.getState().paneActions.forgetChat('chat-1')
    expect(store.getState().viewOrder).toEqual([])
    expect(store.getState().activeViewId).toBeNull()
  })

  it('is a no-op for a chat no pane holds', () => {
    const store = makePaneOnlyStore()
    store.getState().paneActions.openChat('chat-1')
    const before = store.getState()
    store.getState().paneActions.forgetChat('chat-unknown')
    expect(store.getState().panes).toBe(before.panes)
  })

  it('never sweeps a legitimate chatless editor split', () => {
    const store = makePaneOnlyStore()
    const actions = store.getState().paneActions
    actions.openChat('chat-1')
    const editorSibling = actions.splitPane(ROOT_PANE_ID, 'horizontal', 'file-1')!
    actions.dropChatOnPane('chat-2', ROOT_PANE_ID, 'left')
    actions.forgetChat('chat-2')
    expect(store.getState().panes[editorSibling].editorTabIds).toEqual(['file-1'])
  })
})

describe('adoptBackgroundChat', () => {
  it('a working chat with no pane gets a record, appended and not shown', () => {
    const store = makePaneOnlyStore()
    store.getState().paneActions.setActiveProject('p1')
    store.getState().paneActions.openChat('chat-1')
    const showing = store.getState().activeViewId

    store.getState().paneActions.adoptBackgroundChat('chat-bg', 'p1')

    const state = store.getState()
    expect(state.viewOrder).toHaveLength(2)
    const adopted = state.viewOrder[1]
    expect(viewChatIds(state, adopted)).toEqual(['chat-bg'])
    expect(state.views[adopted].projectId).toBe('p1')
    expect(state.activeViewId).toBe(showing)
  })

  it('never mints a duplicate for a chat that already has a pane', () => {
    const store = makePaneOnlyStore()
    store.getState().paneActions.openChat('chat-1')
    store.getState().paneActions.adoptBackgroundChat('chat-1', 'p1')
    expect(store.getState().viewOrder).toHaveLength(1)
  })
})
