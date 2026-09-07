import { describe, expect, it, vi, afterEach } from 'vitest'

// Mocked so the real registry's store creation doesn't need a real
// IndexedDB write path — same setup workspace-store-registry.test.ts uses
// for exercising the real registry.
vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/features/editor/stores/buffer-session-persistence', () => ({
  saveSessionToStore: vi.fn(),
  clearQueuedWorkspaceSessionSave: vi.fn(),
}))

import { focusRecent, closeRecent } from '@/components/sidebar/lib/recents-actions'
import {
  getAllActiveWorkspaceIds,
  destroyWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import type { RecentsBandEntry } from '@/components/sidebar/recents-band'
import type { Repo } from '@/lib/store/sidebar'

const repo = (over: Partial<Repo> = {}): Repo => ({
  id: 'r1',
  projectId: 'p1',
  name: 'crowbar',
  avatarLabel: 'C',
  avatarColor: 'bg-indigo-700',
  workspaces: [{ id: 'ws-1', branch: 'alpha', age: '', order: 0 }],
  ...over,
})

afterEach(() => {
  getAllActiveWorkspaceIds().forEach((id) => destroyWorkspaceStore(id))
  // Task 26: panes/dormantArrangements are a window-level singleton now,
  // never destroyed by destroyWorkspaceStore — reset between tests.
  resetWindowPaneStoreForTests()
})

describe('focusRecent', () => {
  it("navigates to the entry's owning workspace", () => {
    const navigate = vi.fn()
    const entry: RecentsBandEntry = {
      id: 'e1',
      localId: 'e1',
      chatIds: ['chat-1'],
      state: 'dormant',
      workspaceId: 'ws-1',
    }
    focusRecent(entry, [repo()], navigate)
    expect(navigate).toHaveBeenCalledWith({
      to: '/ide/$projectId/$repoId/$wsId',
      params: { projectId: 'p1', repoId: 'r1', wsId: 'ws-1' },
    })
  })

  it('brings a live pane already holding the chat to the front', () => {
    const navigate = vi.fn()
    windowPaneStore.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    const otherPane = windowPaneStore.getState().paneActions.splitPane(ROOT_PANE_ID, 'horizontal')!
    windowPaneStore.getState().paneActions.setActivePane(otherPane)

    const entry: RecentsBandEntry = {
      id: 'e1',
      localId: 'e1',
      chatIds: ['chat-1'],
      state: 'live',
      workspaceId: 'ws-1',
    }
    focusRecent(entry, [repo()], navigate)

    expect(windowPaneStore.getState().activePaneId).toBe(ROOT_PANE_ID)
  })

  it('does not navigate for a workspace not found in the given repos', () => {
    const navigate = vi.fn()
    const entry: RecentsBandEntry = {
      id: 'e1',
      localId: 'e1',
      chatIds: ['chat-1'],
      state: 'dormant',
      workspaceId: 'nope',
    }
    focusRecent(entry, [repo()], navigate)
    expect(navigate).not.toHaveBeenCalled()
  })

  /**
   * Recents is the VIEW SWITCHER, so this row body is the only way back to a
   * view that is off screen. It used to run the switch AFTER the navigation
   * and behind its early return, which meant a row whose workspace
   * `resolveRow` could not place — a repo's own checkout, a project-home
   * workspace, anything outside `repo.workspaces` — did nothing at all.
   * Observed live: clicking such a row left the screen exactly as it was.
   */
  it('switches to the view even when the route cannot be resolved', () => {
    const navigate = vi.fn()
    const { paneActions } = windowPaneStore.getState()
    paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    const elsewhere = paneActions.addPane()!
    paneActions.setPaneChat(elsewhere, 'chat-2', null)
    expect(windowPaneStore.getState().activePaneId).toBe(elsewhere)

    focusRecent(
      { id: 'e1', localId: 'e1', chatIds: ['chat-1'], state: 'live', workspaceId: 'unplaceable' },
      [repo()],
      navigate,
    )

    expect(navigate).not.toHaveBeenCalled()
    expect(windowPaneStore.getState().activePaneId).toBe(ROOT_PANE_ID)
    expect(windowPaneStore.getState().activeViewId).toBe(ROOT_PANE_ID)
  })

  it('brings a view that is OFF SCREEN back onto it', () => {
    const navigate = vi.fn()
    const { paneActions } = windowPaneStore.getState()
    paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    const elsewhere = paneActions.addPane()!
    paneActions.setPaneChat(elsewhere, 'chat-2', null)

    focusRecent(
      { id: 'e1', localId: 'e1', chatIds: ['chat-1'], state: 'live', workspaceId: 'ws-1' },
      [repo()],
      navigate,
    )

    expect(windowPaneStore.getState().activeViewId).toBe(ROOT_PANE_ID)
    expect(Object.keys(windowPaneStore.getState().parkedViews)).toEqual([elsewhere])
  })
})

describe('closeRecent', () => {
  it("closes every pane holding one of a LIVE entry's chats", () => {
    windowPaneStore.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')

    const entry: RecentsBandEntry = {
      id: 'e1',
      localId: 'e1',
      chatIds: ['chat-1'],
      state: 'live',
      workspaceId: 'ws-1',
    }
    closeRecent(entry)

    // closePane on the sole root pane empties it rather than deleting it
    // (spec §5.4: "closing the last pane empties it rather than refusing").
    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBeNull()
  })

  it('an idle live chat becomes a dormant arrangement on close, never lost', () => {
    windowPaneStore.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')

    const entry: RecentsBandEntry = {
      id: 'e1',
      localId: 'e1',
      chatIds: ['chat-1'],
      state: 'live',
      workspaceId: 'ws-1',
    }
    closeRecent(entry)

    expect(windowPaneStore.getState().dormantArrangements).toEqual([
      { id: ROOT_PANE_ID, chatIds: ['chat-1'], state: 'dormant' },
    ])
  })

  it('forgets a DORMANT entry outright — there is no pane to close', () => {
    windowPaneStore.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    windowPaneStore.getState().paneActions.closePane(ROOT_PANE_ID) // seeds one dormant arrangement keyed ROOT_PANE_ID
    expect(windowPaneStore.getState().dormantArrangements).toHaveLength(1)

    // Task 26: ids are already globally unique (one pane store), so
    // recents-for-project.ts no longer mints a workspace-qualified `.id`
    // distinct from `.localId` — both are the store's own real arrangement
    // id now. `closeRecent` still forgets by `.localId` (the field the store
    // is actually keyed by), which continues to be the correct field to use.
    const entry: RecentsBandEntry = {
      id: ROOT_PANE_ID,
      localId: ROOT_PANE_ID,
      chatIds: ['chat-1'],
      state: 'dormant',
      workspaceId: 'ws-1',
    }
    closeRecent(entry)

    expect(windowPaneStore.getState().dormantArrangements).toEqual([])
  })
})

/**
 * The × on a Recents row is "end this view" (spec §5.4), and a view is now
 * routinely more than one pane — so it has to take every member with it.
 * `closeView` is what guarantees that: one `closePane` per member, each
 * running the real teardown (`releaseClosedChat` → `stopChat` → workspace
 * eviction), rather than only the pane the gesture happened to name.
 */
describe('closeRecent — a view of any size', () => {
  it('ends every member of a MERGED view, and remembers them all', () => {
    const { paneActions } = windowPaneStore.getState()
    paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    const b = paneActions.splitPane(ROOT_PANE_ID, 'horizontal')!
    paneActions.setPaneChat(b, 'chat-2', null)
    const c = paneActions.splitPane(b, 'vertical')!
    paneActions.setPaneChat(c, 'chat-3', null)

    closeRecent({
      id: 'e1',
      localId: 'e1',
      chatIds: ['chat-1', 'chat-2', 'chat-3'],
      state: 'live',
      showing: true,
      workspaceId: 'ws-1',
    })

    expect(Object.values(windowPaneStore.getState().panes).filter((p) => p.chatId)).toEqual([])
    expect(windowPaneStore.getState().panes[b]).toBeUndefined()
    expect(windowPaneStore.getState().panes[c]).toBeUndefined()
    const remembered = windowPaneStore
      .getState()
      .dormantArrangements.flatMap((e) => e.chatIds)
      .sort()
    expect(remembered).toEqual(['chat-1', 'chat-2', 'chat-3'])
  })

  // The row's own id is the DORMANT RECORD's when it inherited one, not the
  // live view's — so the view has to be resolved from a member that is up.
  it('ends the right view when the row sits at a remembered slot', () => {
    const { paneActions } = windowPaneStore.getState()
    paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    const other = paneActions.addPane()!
    paneActions.setPaneChat(other, 'chat-2', null)

    closeRecent({
      id: 'a-slot-id-that-is-not-a-view',
      localId: 'a-slot-id-that-is-not-a-view',
      chatIds: ['chat-1'],
      state: 'live',
      workspaceId: 'ws-1',
    })

    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]).toBeUndefined()
    // The sibling view is untouched and now the one on screen.
    expect(windowPaneStore.getState().panes[other]?.chatId).toBe('chat-2')
  })

  it('closing a view that is off screen never disturbs the one that is', () => {
    const { paneActions } = windowPaneStore.getState()
    paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    const showing = paneActions.addPane()!
    paneActions.setPaneChat(showing, 'chat-2', null)

    closeRecent({
      id: ROOT_PANE_ID,
      localId: ROOT_PANE_ID,
      chatIds: ['chat-1'],
      state: 'live',
      workspaceId: 'ws-1',
    })

    expect(windowPaneStore.getState().parkedViews).toEqual({})
    expect(windowPaneStore.getState().activePaneId).toBe(showing)
    expect(windowPaneStore.getState().panes[showing]?.chatId).toBe('chat-2')
  })
})
