// The project-scoped panes laws, against the view records that enforce them:
// "I shouldn't be able to still see a chat that is inside another project
// when moving into a different one."
import { describe, it, expect, vi } from 'vitest'
import { chatPaneIndex, selectIsShowingEmptyStage } from '@/features/panes/lib/view-selectors'
import { showingLayout, viewChatIds } from '@/features/panes/lib/view-state'
import { getAllLeafIds } from '@/features/panes/utils/pane-layout'
import { makePaneOnlyStore } from '@/__tests__/__fixtures__/view-state'

vi.mock('@/features/panes/lib/release-closed-chat', () => ({
  releaseClosedChat: vi.fn(async () => {}),
}))

type Store = ReturnType<typeof makePaneOnlyStore>

function open(store: Store, chatId: string, projectId?: string) {
  store.getState().paneActions.openChat(chatId, { projectId })
  const paneId = chatPaneIndex(store.getState().panes).get(chatId)!
  return { paneId, viewId: store.getState().panes[paneId].viewId! }
}

describe('law 1 — a record belongs to one project, fixed at creation', () => {
  it('a new record takes the project the screen is in', () => {
    const store = makePaneOnlyStore()
    store.getState().paneActions.setActiveProject('project-a')
    const { viewId } = open(store, 'chat-a')
    expect(store.getState().views[viewId].projectId).toBe('project-a')
  })

  it('a split stays in the record it was carved from', () => {
    const store = makePaneOnlyStore()
    store.getState().paneActions.setActiveProject('project-a')
    const { paneId, viewId } = open(store, 'chat-a')
    const split = store.getState().paneActions.splitPane(paneId, 'horizontal')!
    expect(store.getState().panes[split].viewId).toBe(viewId)
  })

  it("a detached member keeps its group's project, not the active one", () => {
    const store = makePaneOnlyStore()
    store.getState().paneActions.setActiveProject('project-a')
    const { paneId } = open(store, 'chat-a')
    store.getState().paneActions.dropChatOnPane('chat-b', paneId, 'right')
    store.getState().paneActions.setActiveProject('project-b')
    store.getState().paneActions.detachPane(chatPaneIndex(store.getState().panes).get('chat-b')!)
    const detached = store.getState().panes[chatPaneIndex(store.getState().panes).get('chat-b')!]
    expect(store.getState().views[detached.viewId!].projectId).toBe('project-a')
  })

  it("opening another project's chat files it there without taking this screen", () => {
    const store = makePaneOnlyStore()
    store.getState().paneActions.setActiveProject('project-a')
    const a = open(store, 'chat-a')
    const b = open(store, 'chat-b', 'project-b')
    expect(store.getState().views[b.viewId].projectId).toBe('project-b')
    expect(store.getState().activeViewId).toBe(a.viewId)
    store.getState().paneActions.setActiveProject('project-b')
    expect(store.getState().activeViewId).toBe(b.viewId)
  })
})

describe("law 2 — the screen shows one project's view", () => {
  it('switching to a project with nothing open shows the empty stage', () => {
    const store = makePaneOnlyStore()
    store.getState().paneActions.setActiveProject('project-a')
    const a = open(store, 'chat-a')
    store.getState().paneActions.setActiveProject('project-b')
    expect(selectIsShowingEmptyStage(store.getState())).toBe(true)
    expect(getAllLeafIds(showingLayout(store.getState()))).not.toContain(a.paneId)
  })

  it("activateView and setActivePane refuse another project's view", () => {
    const store = makePaneOnlyStore()
    store.getState().paneActions.setActiveProject('project-a')
    const a = open(store, 'chat-a')
    store.getState().paneActions.setActiveProject('project-b')
    const b = open(store, 'chat-b')
    store.getState().paneActions.activateView(a.viewId)
    store.getState().paneActions.setActivePane(a.paneId)
    expect(store.getState().activeViewId).toBe(b.viewId)
    expect(store.getState().activeViewByProject['project-a']).toBe(a.viewId)
  })

  it("closing this project's last view never reveals another project's", () => {
    const store = makePaneOnlyStore()
    store.getState().paneActions.setActiveProject('project-a')
    open(store, 'chat-a')
    store.getState().paneActions.setActiveProject('project-b')
    const b = open(store, 'chat-b')
    store.getState().paneActions.closeView(b.viewId)
    expect(store.getState().activeViewId).toBeNull()
  })
})

describe('law 3 — off screen is not closed', () => {
  it('switching back is a RETURN to the view you left', () => {
    const store = makePaneOnlyStore()
    store.getState().paneActions.setActiveProject('project-a')
    open(store, 'chat-a1')
    const a2 = open(store, 'chat-a2')
    store.getState().paneActions.setActiveProject('project-b')
    open(store, 'chat-b')
    store.getState().paneActions.setActiveProject('project-a')
    expect(store.getState().activeViewId).toBe(a2.viewId)
    expect(store.getState().panes[a2.paneId].chatId).toBe('chat-a2')
  })

  it('a refused cross-project reveal lands ON that pane after the switch', () => {
    const store = makePaneOnlyStore()
    store.getState().paneActions.setActiveProject('project-a')
    const a = open(store, 'chat-a')
    open(store, 'chat-a2')
    store.getState().paneActions.setActiveProject('project-b')
    store.getState().paneActions.openChat('chat-a')
    store.getState().paneActions.setActiveProject('project-a')
    expect(store.getState().activeViewId).toBe(a.viewId)
    expect(store.getState().activePaneId).toBe(a.paneId)
  })

  it('a project switch changes no rows', () => {
    const store = makePaneOnlyStore()
    store.getState().paneActions.setActiveProject('project-a')
    open(store, 'chat-a')
    store.getState().paneActions.setActiveProject('project-b')
    open(store, 'chat-b')
    const rows = store.getState().viewOrder
    store.getState().paneActions.setActiveProject('project-a')
    store.getState().paneActions.setActiveProject('project-b')
    expect(store.getState().viewOrder).toBe(rows)
  })
})

describe('boot — records minted before the window knew its project', () => {
  it('the FIRST setActiveProject files records with no project', () => {
    const store = makePaneOnlyStore()
    const { viewId } = open(store, 'chat-early')
    expect(store.getState().views[viewId].projectId).toBe('')
    store.getState().paneActions.setActiveProject('project-a')
    expect(store.getState().views[viewId].projectId).toBe('project-a')
    expect(store.getState().activeViewId).toBe(viewId)
  })

  it('an already-showing view survives the bootstrap over a stale remembered pointer', () => {
    const store = makePaneOnlyStore()
    const old = open(store, 'chat-old')
    store.setState((s) => {
      s.views[old.viewId].projectId = 'project-a'
      s.activeViewByProject['project-a'] = old.viewId
    })
    const fresh = open(store, 'chat-new-thread')
    store.getState().paneActions.setActiveProject('project-a')
    expect(store.getState().activeViewId).toBe(fresh.viewId)
  })

  it('a LATER switch re-files nothing', () => {
    const store = makePaneOnlyStore()
    store.getState().paneActions.setActiveProject('project-a')
    const a = open(store, 'chat-a')
    store.getState().paneActions.setActiveProject('project-b')
    expect(store.getState().views[a.viewId].projectId).toBe('project-a')
  })
})

describe('closing a project', () => {
  it("closeViewsForProject closes that project's records and their pointer", () => {
    const store = makePaneOnlyStore()
    store.getState().paneActions.setActiveProject('project-a')
    const a1 = open(store, 'chat-a1')
    const a2 = open(store, 'chat-a2')
    store.getState().paneActions.setActiveProject('project-b')
    const b = open(store, 'chat-b')

    store.getState().paneActions.closeViewsForProject('project-a')

    const state = store.getState()
    expect(state.panes[a1.paneId]).toBeUndefined()
    expect(state.panes[a2.paneId]).toBeUndefined()
    expect(state.viewOrder).toEqual([b.viewId])
    expect(viewChatIds(state, b.viewId)).toEqual(['chat-b'])
    expect(state.activeViewByProject['project-a']).toBeUndefined()
  })
})
