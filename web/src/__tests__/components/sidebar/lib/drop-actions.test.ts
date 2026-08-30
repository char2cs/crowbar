import { describe, expect, it, vi, afterEach, beforeEach } from 'vitest'

// Mocked so the real registry's store creation doesn't need a real
// IndexedDB/localStorage write path — same setup recents-actions.test.ts
// uses for exercising the real registry.
vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/features/editor/stores/buffer-session-persistence', () => ({
  saveSessionToStore: vi.fn(),
  clearQueuedWorkspaceSessionSave: vi.fn(),
}))
vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: vi.fn(), info: vi.fn(), success: vi.fn() },
}))
vi.mock('@/lib/api/sidebar-placement', () => ({
  placeWorkspace: vi.fn().mockResolvedValue(undefined),
  placeFolder: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/lib/api/workspace', () => ({
  reparentWorkspace: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/features/agent/api/agent-api', () => ({
  setChatPlacement: vi.fn().mockResolvedValue({ chat: {}, shifted: [] }),
}))

import { performSidebarPaneDrop, performSidebarDrop } from '@/components/sidebar/lib/drop-actions'
import { placeWorkspace, placeFolder } from '@/lib/api/sidebar-placement'
import { reparentWorkspace } from '@/lib/api/workspace'
import { setChatPlacement } from '@/features/agent/api/agent-api'
import { toast } from '@/features/window/stores/toast-store'
import {
  getOrCreateWorkspaceStore,
  destroyWorkspaceStore,
  getAllActiveWorkspaceIds,
  setActiveWorkspaceId,
} from '@/features/workspace/stores/workspace-store-registry'
import { getInitialState, useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import type { AgentChat } from '@/features/agent/api/agent-api'

/**
 * `performSidebarDrop` — the row-to-row half of spec §8.1 (Task 33). Adapts
 * `drop-plan.ts`'s (git show 9ad89156) container/fork-lineage math to
 * `SidebarRow`/`SIDEBAR_DROP_POLICY`, minus its `project`/`repo` subjects and
 * its optimistic `writes` half (this plan has no local optimistic paint).
 *
 * `performSidebarPaneDrop`'s own suite (below, unchanged from Task 22) covers
 * §8.1's other two targets — the middle/edge of a PANE.
 */

const branchRow = (id: string, over: Partial<SidebarRow> = {}): SidebarRow => ({
  id,
  kind: 'branch',
  parentId: null,
  order: 0,
  label: id,
  ownsWorktree: true,
  workspaceId: id,
  working: false,
  hasView: false,
  ...over,
})

const folderRow = (id: string, over: Partial<SidebarRow> = {}): SidebarRow => ({
  id,
  kind: 'folder',
  parentId: null,
  order: 0,
  label: id,
  ownsWorktree: true,
  workspaceId: null,
  working: false,
  hasView: false,
  ...over,
})

const chatRow = (id: string, wsId: string, over: Partial<SidebarRow> = {}): SidebarRow => ({
  id,
  kind: 'chat',
  parentId: null,
  order: 0,
  label: id,
  ownsWorktree: false,
  workspaceId: wsId,
  working: false,
  hasView: false,
  ...over,
})

/**
 * One repo, shared across the branch/folder scenarios below:
 *
 *   home-1 (repo header)
 *     ws-a
 *       ws-fork   (forked off ws-a)
 *     ws-b
 *     ws-c
 *     folder-1
 *     folder-2
 */
function makeRepo(): Repo {
  return {
    id: 'repo-1',
    projectId: 'proj-1',
    name: 'repo-1',
    avatarLabel: 'R',
    avatarColor: 'bg-indigo-700',
    defaultWorkspaceId: 'home-1',
    defaultBranch: 'main',
    workspaces: [
      { id: 'ws-a', branch: 'a', age: '', order: 0 },
      { id: 'ws-b', branch: 'b', age: '', order: 1 },
      { id: 'ws-c', branch: 'c', age: '', order: 2 },
      { id: 'ws-fork', branch: 'fork', age: '', order: 0, parentId: 'ws-a' },
    ],
    folders: [
      { id: 'folder-1', repoId: 'repo-1', name: 'Bugs', order: 3 },
      { id: 'folder-2', repoId: 'repo-1', name: 'Chores', order: 4 },
    ],
  }
}

beforeEach(() => {
  vi.clearAllMocks()
  useSidebarStore.setState({ ...getInitialState(), repos: [makeRepo()] })
  setActiveWorkspaceId('ws-1')
})

afterEach(() => {
  getAllActiveWorkspaceIds().forEach((id) => destroyWorkspaceStore(id))
})

describe('performSidebarDrop — reordering (no lineage change)', () => {
  it('reorders a workspace among its current siblings — one placement call, no reparent', async () => {
    await performSidebarDrop(
      [branchRow('ws-c')],
      branchRow('ws-a', { parentId: 'home-1' }),
      'before',
    )

    expect(placeWorkspace).toHaveBeenCalledTimes(1)
    expect(placeWorkspace).toHaveBeenCalledWith('proj-1', 'repo-1', 'ws-c', {
      folderId: '',
      order: 0,
    })
    expect(reparentWorkspace).not.toHaveBeenCalled()
  })

  it('reorders a folder among its siblings the same way', async () => {
    await performSidebarDrop(
      [folderRow('folder-2')],
      folderRow('folder-1', { parentId: 'home-1' }),
      'before',
    )

    expect(placeFolder).toHaveBeenCalledWith('proj-1', 'repo-1', 'folder-2', {
      parentId: '',
      order: 3,
    })
  })
})

describe('performSidebarDrop — filing into a folder', () => {
  it('drops a workspace into a folder — folder edge written, no lineage change', async () => {
    await performSidebarDrop(
      [branchRow('ws-b')],
      folderRow('folder-1', { parentId: 'home-1' }),
      'into',
    )

    expect(placeWorkspace).toHaveBeenCalledWith('proj-1', 'repo-1', 'ws-b', {
      folderId: 'folder-1',
      order: 0,
    })
    expect(reparentWorkspace).not.toHaveBeenCalled()
  })
})

describe('performSidebarDrop — crossing a fork parent', () => {
  it('reparents before placing when the destination is under a different fork parent', async () => {
    // ws-fork currently hangs off ws-a; dropped INTO ws-b it must rebase.
    await performSidebarDrop([branchRow('ws-fork')], branchRow('ws-b'), 'into')

    expect(reparentWorkspace).toHaveBeenCalledWith('proj-1', 'repo-1', 'ws-fork', 'ws-b')
    expect(placeWorkspace).toHaveBeenCalledWith('proj-1', 'repo-1', 'ws-fork', { order: 0 })
    expect(vi.mocked(reparentWorkspace).mock.invocationCallOrder[0]).toBeLessThan(
      vi.mocked(placeWorkspace).mock.invocationCallOrder[0],
    )
  })

  it('dropping "after" an EXPANDED row with children re-parents as its first child', async () => {
    // ws-a is expanded by default (nothing folded) and has ws-fork as a
    // child, so the gap right under it is the first-child slot, not a
    // sibling-after reorder.
    await performSidebarDrop(
      [branchRow('ws-b')],
      branchRow('ws-a', { parentId: 'home-1' }),
      'after',
    )

    expect(reparentWorkspace).toHaveBeenCalledWith('proj-1', 'repo-1', 'ws-b', 'ws-a')
    expect(placeWorkspace).toHaveBeenCalledWith('proj-1', 'repo-1', 'ws-b', { order: 0 })
  })

  it('the same "after" drop on a COLLAPSED row is a plain sibling reorder instead', async () => {
    useSidebarStore.setState({ collapsedChatRows: new Set(['ws-a']) })

    await performSidebarDrop(
      [branchRow('ws-b')],
      branchRow('ws-a', { parentId: 'home-1' }),
      'after',
    )

    expect(reparentWorkspace).not.toHaveBeenCalled()
    // ws-a sits at index 0 among the root siblings, so "after" it is index 1.
    expect(placeWorkspace).toHaveBeenCalledWith('proj-1', 'repo-1', 'ws-b', {
      folderId: '',
      order: 1,
    })
  })
})

describe('performSidebarDrop — multi-row moves', () => {
  it('fires each call in order, awaiting the previous one before the next starts', async () => {
    let resolveFirst!: () => void
    const pending = new Promise<void>((resolve) => {
      resolveFirst = resolve
    })
    vi.mocked(placeWorkspace).mockImplementationOnce(() => pending)

    const done = performSidebarDrop(
      [branchRow('ws-b'), branchRow('ws-c')],
      branchRow('ws-a', { parentId: 'home-1' }),
      'before',
    )

    // Let every already-settled microtask run without advancing past the
    // still-pending first call.
    await Promise.resolve()
    await Promise.resolve()
    expect(placeWorkspace).toHaveBeenCalledTimes(1)

    resolveFirst()
    await done

    expect(placeWorkspace).toHaveBeenCalledTimes(2)
    expect(placeWorkspace).toHaveBeenNthCalledWith(1, 'proj-1', 'repo-1', 'ws-b', {
      folderId: '',
      order: 0,
    })
    expect(placeWorkspace).toHaveBeenNthCalledWith(2, 'proj-1', 'repo-1', 'ws-c', {
      folderId: '',
      order: 1,
    })
  })
})

describe('performSidebarDrop — failures', () => {
  it('a failed API call produces a toast.error, not a thrown exception', async () => {
    vi.mocked(placeWorkspace).mockRejectedValueOnce(new Error('locked'))

    await expect(
      performSidebarDrop([branchRow('ws-c')], branchRow('ws-a', { parentId: 'home-1' }), 'before'),
    ).resolves.toBeUndefined()

    expect(toast.error).toHaveBeenCalledWith('locked')
  })
})

describe('performSidebarDrop — the repo home row', () => {
  it('dragging the repo’s own checkout is a no-op — it is a row but not a Workspace', async () => {
    await expect(
      performSidebarDrop(
        [branchRow('home-1', { parentId: null, workspaceId: 'home-1' })],
        branchRow('ws-a', { parentId: 'home-1' }),
        'after',
      ),
    ).resolves.toBeUndefined()

    expect(placeWorkspace).not.toHaveBeenCalled()
    expect(reparentWorkspace).not.toHaveBeenCalled()
  })
})

describe('performSidebarDrop — unresolvable rows', () => {
  it('a target the live store does not recognise is a no-op, not a throw', async () => {
    useSidebarStore.setState({ repos: [] })

    await expect(
      performSidebarDrop([branchRow('ws-a')], branchRow('ghost'), 'before'),
    ).resolves.toBeUndefined()

    expect(placeWorkspace).not.toHaveBeenCalled()
  })
})

// ── Chats: `AgentChat`'s own placement, not `placeWorkspace`/`placeFolder` ──

const chat = (id: string, wsId: string, over: Partial<AgentChat> = {}): AgentChat => ({
  id,
  workspaceId: wsId,
  title: id,
  liveRunnerId: '',
  terminalSessionId: '',
  activeProviderId: 'claude',
  createdAt: '2026-01-01T00:00:00Z',
  order: 0,
  parentId: '',
  ...over,
})

describe('performSidebarDrop — chats', () => {
  it('reorders a chat among its siblings in its own workspace via setChatPlacement', async () => {
    const store = getOrCreateWorkspaceStore('ws-x')
    store
      .getState()
      .seedAgentChats([
        chat('chat-a', 'ws-x', { order: 0 }),
        chat('chat-b', 'ws-x', { order: 1 }),
        chat('chat-c', 'ws-x', { order: 2 }),
      ])

    await performSidebarDrop([chatRow('chat-c', 'ws-x')], chatRow('chat-a', 'ws-x'), 'before')

    expect(setChatPlacement).toHaveBeenCalledWith('ws-x', 'chat-c', { parentId: '', order: 0 })
    expect(placeWorkspace).not.toHaveBeenCalled()
  })

  it('dropping a chat "into" another chat makes it one of its threads', async () => {
    const store = getOrCreateWorkspaceStore('ws-x')
    store.getState().seedAgentChats([chat('chat-a', 'ws-x'), chat('chat-b', 'ws-x')])

    await performSidebarDrop([chatRow('chat-b', 'ws-x')], chatRow('chat-a', 'ws-x'), 'into')

    expect(setChatPlacement).toHaveBeenCalledWith('ws-x', 'chat-b', {
      parentId: 'chat-a',
      order: 0,
    })
  })

  it('refuses a chat dropped onto a target in a different workspace — no placement endpoint can move it', async () => {
    await expect(
      performSidebarDrop([chatRow('c1', 'ws-x')], chatRow('c2', 'ws-y'), 'before'),
    ).resolves.toBeUndefined()

    expect(setChatPlacement).not.toHaveBeenCalled()
    expect(toast.error).toHaveBeenCalled()
  })

  it('a chat dropped onto a branch/folder row is a no-op — no shared placement concept exists yet', async () => {
    await expect(
      performSidebarDrop(
        [chatRow('c1', 'ws-x')],
        branchRow('ws-a', { parentId: 'home-1' }),
        'into',
      ),
    ).resolves.toBeUndefined()

    expect(setChatPlacement).not.toHaveBeenCalled()
  })
})

// ── performSidebarPaneDrop — spec §8.1's other two targets (unchanged, Task 22) ──

describe('performSidebarPaneDrop — non-chat subjects', () => {
  it('ignores branch/folder/workflow rows — no pane has an "open into" meaning for them yet', () => {
    const store = getOrCreateWorkspaceStore('ws-1')
    const before = store.getState().panes[ROOT_PANE_ID]

    performSidebarPaneDrop(
      [
        chatRow('branch-a', 'ws-1', { kind: 'branch' }),
        chatRow('folder-a', 'ws-1', { kind: 'folder' }),
        chatRow('flow-a', 'ws-1', { kind: 'workflow' }),
      ],
      ROOT_PANE_ID,
      'center',
    )

    expect(store.getState().panes[ROOT_PANE_ID]).toEqual(before)
  })

  it('is a no-op for a chat row with no owning workspace', () => {
    expect(() =>
      performSidebarPaneDrop(
        [chatRow('c1', 'ws-1', { workspaceId: null })],
        ROOT_PANE_ID,
        'center',
      ),
    ).not.toThrow()
  })
})

describe('performSidebarPaneDrop — plain open (spec §8.1 "middle of a pane")', () => {
  it('opens a chat that is not up anywhere into an empty pane, and focuses it', () => {
    const store = getOrCreateWorkspaceStore('ws-1')

    performSidebarPaneDrop([chatRow('c1', 'ws-1')], ROOT_PANE_ID, 'center')

    expect(store.getState().panes[ROOT_PANE_ID]?.chatId).toBe('c1')
    expect(store.getState().activePaneId).toBe(ROOT_PANE_ID)
  })
})

describe('performSidebarPaneDrop — already up (spec §8.2)', () => {
  it('a chat already live in another pane goes TO it — reveal, never a second setPaneChat', () => {
    const store = getOrCreateWorkspaceStore('ws-1')
    const otherPane = store.getState().paneActions.splitPane(ROOT_PANE_ID, 'horizontal')!
    store.getState().paneActions.setPaneChat(otherPane, 'c1', 'runner-1')
    store.getState().paneActions.setActivePane(ROOT_PANE_ID)

    performSidebarPaneDrop([chatRow('c1', 'ws-1')], ROOT_PANE_ID, 'center')

    // Never opened twice: ROOT_PANE_ID is untouched, and the reveal just
    // refocuses the pane that already has it.
    expect(store.getState().panes[ROOT_PANE_ID]?.chatId).toBeNull()
    expect(store.getState().panes[otherPane]?.chatId).toBe('c1')
    expect(store.getState().activePaneId).toBe(otherPane)
  })

  it('dropping a chat onto the exact pane already showing it is a harmless no-op', () => {
    const store = getOrCreateWorkspaceStore('ws-1')
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'c1', 'runner-1')

    performSidebarPaneDrop([chatRow('c1', 'ws-1')], ROOT_PANE_ID, 'right')

    expect(store.getState().panes[ROOT_PANE_ID]?.chatId).toBe('c1')
    expect(Object.keys(store.getState().panes)).toHaveLength(2) // root + bottom only — no split made
  })
})

describe('performSidebarPaneDrop — merging (spec §8.1 "edge of a pane", §8.2)', () => {
  it('an edge drop onto an EMPTY pane still splits, with nothing to merge into', () => {
    const store = getOrCreateWorkspaceStore('ws-1')

    performSidebarPaneDrop([chatRow('c1', 'ws-1')], ROOT_PANE_ID, 'right')

    const panes = store.getState().panes
    const newPane = Object.values(panes).find((p) => p.chatId === 'c1')
    expect(newPane).toBeDefined()
    expect(newPane?.id).not.toBe(ROOT_PANE_ID)
    expect(store.getState().dormantArrangements).toEqual([])
  })

  it('a center drop onto an OCCUPIED pane never swaps — it merges instead (rule 1: every drop adds)', () => {
    const store = getOrCreateWorkspaceStore('ws-1')
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'c1', 'runner-1')

    performSidebarPaneDrop([chatRow('c2', 'ws-1')], ROOT_PANE_ID, 'center')

    // c1 is still exactly where it was — nothing was evicted.
    expect(store.getState().panes[ROOT_PANE_ID]?.chatId).toBe('c1')
    const newPane = Object.values(store.getState().panes).find((p) => p.chatId === 'c2')
    expect(newPane).toBeDefined()
  })

  it('an edge drop onto an occupied pane groups both chats into one Recents entry ("side by side")', () => {
    const store = getOrCreateWorkspaceStore('ws-1')
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'c1', 'runner-1')

    performSidebarPaneDrop([chatRow('c2', 'ws-1')], ROOT_PANE_ID, 'right')

    expect(store.getState().dormantArrangements).toHaveLength(1)
    const [entry] = store.getState().dormantArrangements
    expect([...entry.chatIds].sort()).toEqual(['c1', 'c2'])
  })

  it('merging into a pane already part of an arrangement GROWS it rather than nesting a second one', () => {
    const store = getOrCreateWorkspaceStore('ws-1')
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'c1', 'runner-1')
    performSidebarPaneDrop([chatRow('c2', 'ws-1')], ROOT_PANE_ID, 'right') // c1+c2 now grouped

    performSidebarPaneDrop([chatRow('c3', 'ws-1')], ROOT_PANE_ID, 'bottom')

    expect(store.getState().dormantArrangements).toHaveLength(1)
    const [entry] = store.getState().dormantArrangements
    expect([...entry.chatIds].sort()).toEqual(['c1', 'c2', 'c3'])
  })

  it('dropping an already-grouped chat elsewhere reveals it in place — the group is untouched', () => {
    const store = getOrCreateWorkspaceStore('ws-1')
    store.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'c1', 'runner-1')
    performSidebarPaneDrop([chatRow('c2', 'ws-1')], ROOT_PANE_ID, 'right') // c1+c2 now grouped
    const grouped = store.getState().dormantArrangements

    const freshPane = store.getState().paneActions.splitPane(ROOT_PANE_ID, 'vertical')!
    performSidebarPaneDrop([chatRow('c1', 'ws-1')], freshPane, 'center')

    expect(store.getState().panes[freshPane]?.chatId).toBeNull()
    expect(store.getState().activePaneId).toBe(ROOT_PANE_ID)
    expect(store.getState().dormantArrangements).toEqual(grouped)
  })
})

describe('performSidebarPaneDrop — cross-workspace safety (Fix round 1)', () => {
  it('refuses a drop when the dragged chat belongs to a workspace other than the one whose pane was actually hit', () => {
    const visible = getOrCreateWorkspaceStore('ws-visible')
    const offscreen = getOrCreateWorkspaceStore('ws-offscreen')
    setActiveWorkspaceId('ws-visible') // ws-visible is what's on screen
    visible.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'already-here', 'runner-1')
    const visibleBefore = visible.getState()
    const offscreenBefore = offscreen.getState()

    performSidebarPaneDrop([chatRow('c1', 'ws-offscreen')], ROOT_PANE_ID, 'right')

    expect(visible.getState().panes).toEqual(visibleBefore.panes)
    expect(visible.getState().rootLayout).toEqual(visibleBefore.rootLayout)
    expect(offscreen.getState().panes).toEqual(offscreenBefore.panes)
    expect(offscreen.getState().rootLayout).toEqual(offscreenBefore.rootLayout)
    expect(offscreen.getState().dormantArrangements).toEqual([])
  })

  it('still works normally once the chat and the active workspace agree', () => {
    const store = getOrCreateWorkspaceStore('ws-visible')
    setActiveWorkspaceId('ws-visible')

    performSidebarPaneDrop([chatRow('c1', 'ws-visible')], ROOT_PANE_ID, 'center')

    expect(store.getState().panes[ROOT_PANE_ID]?.chatId).toBe('c1')
  })
})
