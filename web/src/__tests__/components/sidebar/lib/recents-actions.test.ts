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
  getOrCreateWorkspaceStore,
  destroyWorkspaceStore,
  getAllActiveWorkspaceIds,
} from '@/features/workspace/stores/workspace-store-registry'
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
})

describe('focusRecent', () => {
  it("navigates to the entry's owning workspace", () => {
    const navigate = vi.fn()
    const entry: RecentsBandEntry = {
      id: 'e1',
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
    const store = getOrCreateWorkspaceStore('ws-1')
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    const otherPane = store.getState().paneActions.splitPane(ROOT_PANE_ID, 'horizontal')!
    store.getState().paneActions.setActivePane(otherPane)

    const entry: RecentsBandEntry = {
      id: 'e1',
      chatIds: ['chat-1'],
      state: 'live',
      workspaceId: 'ws-1',
    }
    focusRecent(entry, [repo()], navigate)

    expect(store.getState().activePaneId).toBe(ROOT_PANE_ID)
  })

  it('is a no-op for a workspace not found in the given repos', () => {
    const navigate = vi.fn()
    const entry: RecentsBandEntry = {
      id: 'e1',
      chatIds: ['chat-1'],
      state: 'dormant',
      workspaceId: 'nope',
    }
    focusRecent(entry, [repo()], navigate)
    expect(navigate).not.toHaveBeenCalled()
  })
})

describe('closeRecent', () => {
  it("closes every pane holding one of a LIVE entry's chats", () => {
    const store = getOrCreateWorkspaceStore('ws-1')
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')

    const entry: RecentsBandEntry = {
      id: 'e1',
      chatIds: ['chat-1'],
      state: 'live',
      workspaceId: 'ws-1',
    }
    closeRecent(entry)

    // closePane on the sole root pane empties it rather than deleting it
    // (spec §5.4: "closing the last pane empties it rather than refusing").
    expect(store.getState().panes[ROOT_PANE_ID]?.chatId).toBeNull()
  })

  it('an idle live chat becomes a dormant arrangement on close, never lost', () => {
    const store = getOrCreateWorkspaceStore('ws-1')
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')

    const entry: RecentsBandEntry = {
      id: 'e1',
      chatIds: ['chat-1'],
      state: 'live',
      workspaceId: 'ws-1',
    }
    closeRecent(entry)

    expect(store.getState().dormantArrangements).toEqual([
      { id: ROOT_PANE_ID, chatIds: ['chat-1'], state: 'dormant' },
    ])
  })

  it('forgets a DORMANT entry outright — there is no pane to close', () => {
    const store = getOrCreateWorkspaceStore('ws-1')
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'chat-1', 'runner-1')
    store.getState().paneActions.closePane(ROOT_PANE_ID) // seeds one dormant arrangement
    expect(store.getState().dormantArrangements).toHaveLength(1)

    const entry: RecentsBandEntry = {
      id: ROOT_PANE_ID,
      chatIds: ['chat-1'],
      state: 'dormant',
      workspaceId: 'ws-1',
    }
    closeRecent(entry)

    expect(store.getState().dormantArrangements).toEqual([])
  })
})
