// The laws of `2026-09-18-project-scoped-panes-design.md` §4, pinned against
// the slice that enforces them. The user's own words this exists for: "I
// shouldn't be able to still see a chat that is inside another project when
// moving into a different one", and "I shouldn't be able to move a view into
// another project".
import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import { createStore } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import {
  createPaneSlice,
  selectIsShowingEmptyStage,
  type PaneSlice,
} from '@/features/panes/stores/slices/pane-slice'
import {
  getAllActiveWorkspaceIds,
  destroyWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import { resetWindowPaneStoreForTests } from '@/features/panes/stores/window-pane-store'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { getAllLeafIds } from '@/features/panes/utils/pane-layout'
import { viewIdOf } from '@/features/panes/lib/pane-views'

type Store = ReturnType<typeof makeStore>

function makeStore() {
  return createStore<PaneSlice>()(
    immer((set, get) => ({
      ...createPaneSlice(...([set, get, {}] as unknown as Parameters<typeof createPaneSlice>)),
    })),
  )
}

/** What a click does: a brand-new view of its own, holding one chat — the
 *  `addPane` + `setPaneChat` pair `openChatIdInOwnView` fires. Returns the
 *  pane id and the view id it minted. */
function openChatInNewView(store: Store, chatId: string): { paneId: string; viewId: string } {
  const paneId = store.getState().paneActions.addPane()!
  store.getState().paneActions.setPaneChat(paneId, chatId, null)
  return { paneId, viewId: viewIdOf(store.getState().panes[paneId]) }
}

afterEach(() => {
  getAllActiveWorkspaceIds().forEach((id) => destroyWorkspaceStore(id))
  resetWindowPaneStoreForTests()
})

describe('project-scoped panes', () => {
  let store: Store

  beforeEach(() => {
    store = makeStore()
  })

  describe('law 1 — a view belongs to exactly one project, as a tag', () => {
    it('a minted view is stamped with the project the screen is in', () => {
      store.getState().paneActions.setActiveProject('project-a')
      const { viewId } = openChatInNewView(store, 'chat-a')

      expect(store.getState().viewProjects[viewId]).toBe('project-a')
    })

    it('the boot empty stage carries no tag at all (law 6)', () => {
      store.getState().paneActions.setActiveProject('project-a')

      expect(selectIsShowingEmptyStage(store.getState())).toBe(true)
      expect(store.getState().viewProjects[store.getState().activeViewId]).toBeUndefined()
    })

    it('a chat landing in the shared empty stage makes it a real view of this project', () => {
      store.getState().paneActions.setActiveProject('project-a')
      store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-a', null)

      expect(store.getState().viewProjects[ROOT_PANE_ID]).toBe('project-a')
    })

    it('a split stays in the view it was carved from — one view, one project', () => {
      store.getState().paneActions.setActiveProject('project-a')
      const { paneId, viewId } = openChatInNewView(store, 'chat-a')

      const splitId = store.getState().paneActions.splitPane(paneId, 'horizontal')!
      store.getState().paneActions.setPaneChat(splitId, 'chat-b', null)

      expect(viewIdOf(store.getState().panes[splitId])).toBe(viewId)
      expect(Object.values(store.getState().viewProjects)).toEqual(['project-a'])
    })

    it("a detached pane INHERITS its source view's project, not the active one", () => {
      store.getState().paneActions.setActiveProject('project-a')
      const { paneId, viewId } = openChatInNewView(store, 'chat-a')
      const splitId = store.getState().paneActions.splitPane(paneId, 'horizontal')!
      store.getState().paneActions.setPaneChat(splitId, 'chat-b', null)

      store.getState().paneActions.detachPaneToOwnView(splitId)

      const ownViewId = viewIdOf(store.getState().panes[splitId])
      expect(ownViewId).not.toBe(viewId)
      expect(store.getState().viewProjects[ownViewId]).toBe('project-a')
    })
  })

  describe("law 2 — the screen shows one project's view", () => {
    it('switching project parks the view you were in and shows the empty stage', () => {
      store.getState().paneActions.setActiveProject('project-a')
      const { viewId } = openChatInNewView(store, 'chat-a')

      store.getState().paneActions.setActiveProject('project-b')

      expect(store.getState().activeViewId).not.toBe(viewId)
      expect(store.getState().parkedViews[viewId]).toBeDefined()
      expect(selectIsShowingEmptyStage(store.getState())).toBe(true)
    })

    it("the other project's chat is no longer on screen — the reported symptom", () => {
      store.getState().paneActions.setActiveProject('project-a')
      const { paneId } = openChatInNewView(store, 'chat-a')

      store.getState().paneActions.setActiveProject('project-b')

      const onScreen = getAllLeafIds(store.getState().rootLayout)
      expect(onScreen).not.toContain(paneId)
      expect(onScreen.map((id) => store.getState().panes[id].chatId)).toEqual([null])
    })

    it("activateView REFUSES another project's view and leaves the screen alone", () => {
      store.getState().paneActions.setActiveProject('project-a')
      const a = openChatInNewView(store, 'chat-a')
      store.getState().paneActions.setActiveProject('project-b')
      const b = openChatInNewView(store, 'chat-b')

      store.getState().paneActions.activateView(a.viewId)

      expect(store.getState().activeViewId).toBe(b.viewId)
      expect(store.getState().parkedViews[a.viewId]).toBeDefined()
    })

    it("setActivePane refuses a pane in another project's parked view", () => {
      store.getState().paneActions.setActiveProject('project-a')
      const a = openChatInNewView(store, 'chat-a')
      store.getState().paneActions.setActiveProject('project-b')
      const b = openChatInNewView(store, 'chat-b')

      store.getState().paneActions.setActivePane(a.paneId)

      expect(store.getState().activePaneId).toBe(b.paneId)
      expect(store.getState().activeViewId).toBe(b.viewId)
    })

    it("closing this project's last view never reveals another project's", () => {
      store.getState().paneActions.setActiveProject('project-a')
      const a = openChatInNewView(store, 'chat-a')
      store.getState().paneActions.setActiveProject('project-b')
      const b = openChatInNewView(store, 'chat-b')

      store.getState().paneActions.closePane(b.paneId)

      expect(selectIsShowingEmptyStage(store.getState())).toBe(true)
      expect(store.getState().parkedViews[a.viewId]).toBeDefined()
    })
  })

  describe('law 3 — parked is not closed', () => {
    it("a switch keeps the other project's panes, chats and trees alive", () => {
      store.getState().paneActions.setActiveProject('project-a')
      const a = openChatInNewView(store, 'chat-a')

      store.getState().paneActions.setActiveProject('project-b')

      expect(store.getState().panes[a.paneId]?.chatId).toBe('chat-a')
      expect(getAllLeafIds(store.getState().parkedViews[a.viewId])).toEqual([a.paneId])
      expect(store.getState().viewProjects[a.viewId]).toBe('project-a')
    })

    it('switching back is a RETURN to the view you left, not a reset', () => {
      store.getState().paneActions.setActiveProject('project-a')
      const a = openChatInNewView(store, 'chat-a')
      store.getState().paneActions.setActiveProject('project-b')
      openChatInNewView(store, 'chat-b')

      store.getState().paneActions.setActiveProject('project-a')

      expect(store.getState().activeViewId).toBe(a.viewId)
      expect(store.getState().activePaneId).toBe(a.paneId)
    })

    it('a refused cross-project reveal still lands ON that pane after the switch', () => {
      // Law 5's handshake: clicking a chat that lives in another space routes
      // there, and the route-driven switch is what puts it on screen.
      store.getState().paneActions.setActiveProject('project-a')
      const a1 = openChatInNewView(store, 'chat-a1')
      const a2 = openChatInNewView(store, 'chat-a2')
      store.getState().paneActions.setActiveProject('project-b')
      openChatInNewView(store, 'chat-b')

      store.getState().paneActions.setActivePane(a1.paneId)
      store.getState().paneActions.setActiveProject('project-a')

      expect(store.getState().activeViewId).toBe(a1.viewId)
      expect(store.getState().activePaneId).toBe(a1.paneId)
      expect(store.getState().parkedViews[a2.viewId]).toBeDefined()
    })
  })

  describe('law 4 — content never crosses a project', () => {
    it('a merge across two projects is refused outright', () => {
      store.getState().paneActions.setActiveProject('project-a')
      const a = openChatInNewView(store, 'chat-a')
      store.getState().paneActions.setActiveProject('project-b')
      const b = openChatInNewView(store, 'chat-b')

      store.getState().paneActions.mergePaneIntoView(a.paneId, b.paneId, 'horizontal', 'after')

      expect(viewIdOf(store.getState().panes[a.paneId])).toBe(a.viewId)
      expect(viewIdOf(store.getState().panes[b.paneId])).toBe(b.viewId)
      expect(getAllLeafIds(store.getState().rootLayout)).toEqual([b.paneId])
      expect(store.getState().parkedViews[a.viewId]).toBeDefined()
    })

    it('a merge WITHIN one project still works — the refusal is not a blanket', () => {
      store.getState().paneActions.setActiveProject('project-a')
      const first = openChatInNewView(store, 'chat-a')
      const second = openChatInNewView(store, 'chat-b')

      store
        .getState()
        .paneActions.mergePaneIntoView(first.paneId, second.paneId, 'horizontal', 'after')

      expect(viewIdOf(store.getState().panes[first.paneId])).toBe(second.viewId)
      expect(getAllLeafIds(store.getState().rootLayout).sort()).toEqual(
        [first.paneId, second.paneId].sort(),
      )
    })
  })

  describe('law 6 — one shared empty stage', () => {
    it('a project nothing was ever opened in shows the same untagged stage', () => {
      store.getState().paneActions.setActiveProject('project-a')
      openChatInNewView(store, 'chat-a')

      store.getState().paneActions.setActiveProject('project-b')
      const stageInB = store.getState().activeViewId
      store.getState().paneActions.setActiveProject('project-c')

      expect(selectIsShowingEmptyStage(store.getState())).toBe(true)
      expect(store.getState().viewProjects[store.getState().activeViewId]).toBeUndefined()
      // The stage left behind in B evaporated rather than being parked as a
      // view B could switch back to.
      expect(store.getState().parkedViews[stageInB]).toBeUndefined()
    })

    it('a chat opened straight into the ROOT pane is not gutted by a later stage', () => {
      // Trap 3: ROOT_PANE_ID is both a pane id and a view id. A project switch
      // that mints a fresh empty stage must not write over the parked view
      // still holding that pane.
      store.getState().paneActions.setActiveProject('project-a')
      store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-a', null)

      store.getState().paneActions.setActiveProject('project-b')

      expect(store.getState().panes[ROOT_PANE_ID]?.chatId).toBe('chat-a')
      expect(getAllLeafIds(store.getState().parkedViews[ROOT_PANE_ID])).toEqual([ROOT_PANE_ID])
      expect(selectIsShowingEmptyStage(store.getState())).toBe(true)
    })
  })

  describe('§8 — an untagged view is ADOPTED, never migrated', () => {
    it('the FIRST setActiveProject files every view hydrate could not', () => {
      // A layout persisted before views carried a project: real arrangements,
      // no tags. Zen's own `_shouldShowTab` rule — adopt, one branch, no
      // backfill — is what keeps them reachable instead of invisible.
      const { viewId } = openChatInNewView(store, 'chat-legacy')
      expect(store.getState().viewProjects).toEqual({})

      store.getState().paneActions.setActiveProject('project-a')

      expect(store.getState().viewProjects[viewId]).toBe('project-a')
      expect(store.getState().activeViewId).toBe(viewId)
    })

    it('adoption never files the empty stage — it belongs to nobody (law 6)', () => {
      store.getState().paneActions.setActiveProject('project-a')

      expect(store.getState().viewProjects).toEqual({})
    })

    it('a LATER switch adopts nothing — only the first one is hydrate time', () => {
      store.getState().paneActions.setActiveProject('project-a')
      const a = openChatInNewView(store, 'chat-a')

      store.getState().paneActions.setActiveProject('project-b')

      expect(store.getState().viewProjects[a.viewId]).toBe('project-a')
    })
  })

  describe('closing a view drops what named it', () => {
    it('closeView takes the tag and the per-project pointer with it', () => {
      store.getState().paneActions.setActiveProject('project-a')
      const a = openChatInNewView(store, 'chat-a')

      store.getState().paneActions.closeView(a.viewId)

      expect(store.getState().viewProjects[a.viewId]).toBeUndefined()
      expect(store.getState().activeViewByProject['project-a']).toBeUndefined()
    })

    it("a deleted project's views are CLOSED, never orphaned in parkedViews", () => {
      store.getState().paneActions.setActiveProject('project-a')
      const a1 = openChatInNewView(store, 'chat-a1')
      const a2 = openChatInNewView(store, 'chat-a2')
      store.getState().paneActions.setActiveProject('project-b')
      const b = openChatInNewView(store, 'chat-b')

      store.getState().paneActions.closeViewsForProject('project-a')

      expect(store.getState().panes[a1.paneId]).toBeUndefined()
      expect(store.getState().panes[a2.paneId]).toBeUndefined()
      expect(store.getState().parkedViews[a1.viewId]).toBeUndefined()
      expect(store.getState().parkedViews[a2.viewId]).toBeUndefined()
      expect(store.getState().viewProjects).toEqual({ [b.viewId]: 'project-b' })
      expect(store.getState().activeViewByProject['project-a']).toBeUndefined()
    })
  })
})
