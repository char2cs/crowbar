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
  // Echoes the call's own args back as the {folder, shifted} envelope the
  // real .../chats/folders PATCH answers with (Task 34) — fireRowPlacementCall
  // applies `folder` straight to the sidebar store, so this has to resolve to
  // something shaped like a real FolderDTO rather than `undefined`.
  placeFolder: vi.fn(
    async (
      projectId: string,
      repoId: string,
      folderId: string,
      placement: { parentId?: string; order?: number },
    ) => ({
      folder: {
        id: folderId,
        repoId,
        projectId,
        name: folderId,
        parentId: placement.parentId ?? '',
        order: placement.order ?? 0,
      },
      shifted: [],
    }),
  ),
}))
vi.mock('@/lib/api/workspace', () => ({
  reparentWorkspace: vi.fn(),
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
import { getInitialRemovalState, useRemovalTrayStore } from '@/lib/store/sidebar-removal'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import type { AgentChat, AgentChatFolder } from '@/features/agent/api/agent-api'

/**
 * `performSidebarDrop` — the row-to-row half of spec §8.1 (Task 33). Adapts
 * `drop-plan.ts`'s (git show 9ad89156) container/fork-lineage math to
 * `SidebarRow`/`SIDEBAR_DROP_POLICY`, minus its `project`/`repo` subjects and
 * its optimistic `writes` half (this plan has no local optimistic paint).
 *
 * `performSidebarPaneDrop`'s own suite (below, unchanged from Task 22) covers
 * §8.1's other two targets — the middle/edge of a PANE.
 *
 * `reparent-settle.test.ts` unit-tests `watchReparent` itself (the immediate/
 * subscription/failure/timeout paths); the "waits for real confirmation"
 * describe block below only proves it is actually WIRED into the placement
 * sequence here.
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
 *       ws-fork    (forked off ws-a, no folder)
 *       folder-3   (a folder nested under ws-a)
 *         ws-d     (forked off ws-a too, filed into folder-3)
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
      { id: 'ws-d', branch: 'd', age: '', order: 0, parentId: 'ws-a', folderId: 'folder-3' },
    ],
    folders: [
      { id: 'folder-1', repoId: 'repo-1', name: 'Bugs', order: 3 },
      { id: 'folder-2', repoId: 'repo-1', name: 'Chores', order: 4 },
      { id: 'folder-3', repoId: 'repo-1', name: 'Nested', parentId: 'ws-a', order: 1 },
    ],
  }
}

/** Simulates the WS frame a real reparent lands on success: a fresh
 *  `Workspace` object reporting the new `parentId`. */
function confirmReparent(wsId: string, parentId: string): void {
  useSidebarStore.setState((s) => ({
    repos: s.repos.map((r) => ({
      ...r,
      workspaces: r.workspaces.map((w) => (w.id === wsId ? { ...w, parentId } : w)),
    })),
  }))
}

beforeEach(() => {
  vi.clearAllMocks()
  useSidebarStore.setState({ ...getInitialState(), repos: [makeRepo()] })
  useRemovalTrayStore.setState(getInitialRemovalState())
  setActiveWorkspaceId('ws-1')
  // Default: the reparent POST's background job "succeeds" and its
  // confirming WS frame lands essentially at once — most tests below care
  // about the PLACEMENT sequencing this unblocks, not the settle mechanism
  // itself. The dedicated "waits for real confirmation"/"refusal" tests
  // below override this per-call to prove the wait is real.
  vi.mocked(reparentWorkspace).mockImplementation(async (_projectId, _repoId, wsId, parentId) => {
    confirmReparent(wsId, parentId)
  })
})

afterEach(() => {
  getAllActiveWorkspaceIds().forEach((id) => destroyWorkspaceStore(id))
  // Task 26: panes/buffers are a window-level singleton now, never destroyed
  // by destroyWorkspaceStore — reset it to a pristine store between tests.
  resetWindowPaneStoreForTests()
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

describe('performSidebarDrop — clearing a stale folder edge', () => {
  it('landing directly under the current fork parent drops the folder edge, with no lineage change', async () => {
    // ws-d already forks off ws-a and sits filed in folder-3 (also under
    // ws-a). Dropped directly INTO ws-a itself, its lineage does not
    // change (ws-a was already its fork parent) but the folder edge must
    // still be explicitly cleared.
    await performSidebarDrop([branchRow('ws-d')], branchRow('ws-a', { parentId: 'home-1' }), 'into')

    expect(reparentWorkspace).not.toHaveBeenCalled()
    // ws-a's own children are [ws-fork, folder-3] — landing "into" ws-a
    // appends after both.
    expect(placeWorkspace).toHaveBeenCalledWith('proj-1', 'repo-1', 'ws-d', {
      folderId: '',
      order: 2,
    })
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
    // ws-a is expanded by default (nothing folded) and has children, so the
    // gap right under it is the first-child slot, not a sibling-after
    // reorder.
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

  it('reparenting onto the repo home row rebases onto the repo checkout itself', async () => {
    // ws-fork forks off ws-a; dropped onto the repo's own header it must
    // rebase onto the (hidden-from-the-tree) default workspace, same as the
    // old root-drop path.
    await performSidebarDrop(
      [branchRow('ws-fork')],
      branchRow('home-1', { parentId: null, workspaceId: 'home-1' }),
      'into',
    )

    expect(reparentWorkspace).toHaveBeenCalledWith('proj-1', 'repo-1', 'ws-fork', 'home-1')
    // Root-level siblings (5 of them) plus this one landing at the end.
    expect(placeWorkspace).toHaveBeenCalledWith('proj-1', 'repo-1', 'ws-fork', { order: 5 })
  })
})

describe('performSidebarDrop — waits for a real reparent confirmation, not just the 202', () => {
  it('does not fire the placement call until a WS frame actually confirms the new parentId', async () => {
    // This one call resolves the POST with no confirming side effect —
    // exactly what the real 202-then-background-job endpoint does.
    vi.mocked(reparentWorkspace).mockResolvedValueOnce(undefined)

    const done = performSidebarDrop([branchRow('ws-fork')], branchRow('ws-b'), 'into')

    await Promise.resolve()
    await Promise.resolve()
    await Promise.resolve()
    expect(reparentWorkspace).toHaveBeenCalledWith('proj-1', 'repo-1', 'ws-fork', 'ws-b')
    // The POST resolved, but nothing has confirmed the move landed yet.
    expect(placeWorkspace).not.toHaveBeenCalled()

    confirmReparent('ws-fork', 'ws-b')
    await done

    expect(placeWorkspace).toHaveBeenCalledWith('proj-1', 'repo-1', 'ws-fork', { order: 0 })
  })

  it('a server-side refusal (lastError set, e.g. guardReparent declining) surfaces as toast.error — the placement call never fires', async () => {
    vi.mocked(reparentWorkspace).mockResolvedValueOnce(undefined)

    const done = performSidebarDrop([branchRow('ws-fork')], branchRow('ws-b'), 'into')
    await Promise.resolve()
    await Promise.resolve()

    // Simulate the background job's own refusal landing on the entity —
    // this is the ONLY channel `guardReparent`'s error reaches.
    useSidebarStore.setState((s) => ({
      repos: s.repos.map((r) => ({
        ...r,
        workspaces: r.workspaces.map((w) =>
          w.id === 'ws-fork' ? { ...w, lastError: 'workspace has fork children' } : w,
        ),
      })),
    }))
    await done

    expect(placeWorkspace).not.toHaveBeenCalled()
    expect(toast.error).toHaveBeenCalledWith(
      'reparent of ws-fork failed: workspace has fork children',
    )
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

  it('dropping directly into the home row is the same as landing at the repo root', async () => {
    await performSidebarDrop(
      [branchRow('ws-b')],
      branchRow('home-1', { parentId: null, workspaceId: 'home-1' }),
      'into',
    )

    expect(reparentWorkspace).not.toHaveBeenCalled()
    // Root siblings minus ws-b itself: ws-a, ws-c, folder-1, folder-2 — ws-b
    // lands after all four.
    expect(placeWorkspace).toHaveBeenCalledWith('proj-1', 'repo-1', 'ws-b', {
      folderId: '',
      order: 4,
    })
  })

  it('dropping "after" the EXPANDED home row lands as the FIRST root-level row, not the last', async () => {
    // The home row is a row like any other, but it is not a node in
    // `buildSidebarTree`'s own graph — its rendered children ARE the
    // tree's roots. Naively reusing `findNode` for it would always report
    // zero children and silently turn this into an append-at-the-end.
    await performSidebarDrop(
      [branchRow('ws-c')],
      branchRow('home-1', { parentId: null, workspaceId: 'home-1' }),
      'after',
    )

    expect(reparentWorkspace).not.toHaveBeenCalled()
    expect(placeWorkspace).toHaveBeenCalledWith('proj-1', 'repo-1', 'ws-c', {
      folderId: '',
      order: 0,
    })
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

describe('performSidebarDrop — removal-tray hold', () => {
  it('plans against the removal-filtered tree the user actually saw, not the raw store', async () => {
    // ws-a's own children (ws-fork, folder-3 holding ws-d) are all held for
    // removal — hidden from the tree, but still sitting in the raw store
    // until the hold either commits or is cancelled.
    useRemovalTrayStore.setState({ hiddenIds: new Set(['ws-fork', 'folder-3', 'ws-d']) })

    // Planned against the RAW repos, ws-a would still read as having
    // children and — expanded — "after" it would reparent ws-b as its
    // first child. Filtered the way the user actually saw the tree, ws-a
    // has no visible children left, so this is a plain sibling reorder.
    await performSidebarDrop(
      [branchRow('ws-b')],
      branchRow('ws-a', { parentId: 'home-1' }),
      'after',
    )

    expect(reparentWorkspace).not.toHaveBeenCalled()
    // Root siblings minus ws-b: ws-a, ws-c, folder-1, folder-2 — "after"
    // ws-a (index 0) is index 1.
    expect(placeWorkspace).toHaveBeenCalledWith('proj-1', 'repo-1', 'ws-b', {
      folderId: '',
      order: 1,
    })
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

const chatFolder = (
  id: string,
  wsId: string,
  over: Partial<AgentChatFolder> = {},
): AgentChatFolder => ({
  id,
  workspaceId: wsId,
  name: id,
  parentId: '',
  order: 0,
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

  it('computes the insert index against the REAL sibling order, not a raw [...chats, ...folders] concat', async () => {
    // Real order (`compareSiblings`: order ascending, folders above chats on
    // a tie): folder-z(0), chat-x(1), chat-y(2). A naive concat instead
    // pushes every folder after every chat regardless of `order`, seeing
    // [chat-x, chat-y, folder-z] — a DIFFERENT list, so a DIFFERENT index.
    const store = getOrCreateWorkspaceStore('ws-x')
    store
      .getState()
      .seedAgentChats([chat('chat-x', 'ws-x', { order: 1 }), chat('chat-y', 'ws-x', { order: 2 })])
    store.getState().seedAgentChatFolders([chatFolder('folder-z', 'ws-x', { order: 0 })])

    await performSidebarDrop([chatRow('chat-x', 'ws-x')], chatRow('chat-y', 'ws-x'), 'before')

    // Real siblings minus chat-x: [folder-z, chat-y] — "before" chat-y is
    // index 1. (The naive concat would have computed 0.)
    expect(setChatPlacement).toHaveBeenCalledWith('ws-x', 'chat-x', { parentId: '', order: 1 })
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

// ── performSidebarPaneDrop — spec §8.1's other two targets ──
//
// Task 26: panes are window-level now (`windowPaneStore`, one flat store for
// every workspace — see window-pane-store.ts), not one of the many
// per-workspace `getOrCreateWorkspaceStore(wsId)` stores. Every assertion
// below moved from `getOrCreateWorkspaceStore('ws-1').getState()` to
// `windowPaneStore.getState()` for that reason.

describe('performSidebarPaneDrop — non-chat subjects', () => {
  it('ignores branch/folder/workflow rows — no pane has an "open into" meaning for them yet', () => {
    const before = windowPaneStore.getState().panes[ROOT_PANE_ID]

    performSidebarPaneDrop(
      [
        chatRow('branch-a', 'ws-1', { kind: 'branch' }),
        chatRow('folder-a', 'ws-1', { kind: 'folder' }),
        chatRow('flow-a', 'ws-1', { kind: 'workflow' }),
      ],
      ROOT_PANE_ID,
      'center',
    )

    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]).toEqual(before)
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
    performSidebarPaneDrop([chatRow('c1', 'ws-1')], ROOT_PANE_ID, 'center')

    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBe('c1')
    expect(windowPaneStore.getState().activePaneId).toBe(ROOT_PANE_ID)
  })
})

describe('performSidebarPaneDrop — already up (spec §8.2)', () => {
  it('a chat already live in another pane goes TO it — reveal, never a second setPaneChat', () => {
    const otherPane = windowPaneStore.getState().paneActions.splitPane(ROOT_PANE_ID, 'horizontal')!
    windowPaneStore.getState().paneActions.setPaneChat(otherPane, 'c1', 'runner-1')
    windowPaneStore.getState().paneActions.setActivePane(ROOT_PANE_ID)

    performSidebarPaneDrop([chatRow('c1', 'ws-1')], ROOT_PANE_ID, 'center')

    // Never opened twice: ROOT_PANE_ID is untouched, and the reveal just
    // refocuses the pane that already has it.
    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBeNull()
    expect(windowPaneStore.getState().panes[otherPane]?.chatId).toBe('c1')
    expect(windowPaneStore.getState().activePaneId).toBe(otherPane)
  })

  it('dropping a chat onto the exact pane already showing it is a harmless no-op', () => {
    windowPaneStore.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'c1', 'runner-1')

    performSidebarPaneDrop([chatRow('c1', 'ws-1')], ROOT_PANE_ID, 'right')

    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBe('c1')
    expect(Object.keys(windowPaneStore.getState().panes)).toHaveLength(2) // root + bottom only — no split made
  })
})

describe('performSidebarPaneDrop — merging (spec §8.1 "edge of a pane", §8.2)', () => {
  it('an edge drop onto an EMPTY pane still splits, with nothing to merge into', () => {
    performSidebarPaneDrop([chatRow('c1', 'ws-1')], ROOT_PANE_ID, 'right')

    const panes = windowPaneStore.getState().panes
    const newPane = Object.values(panes).find((p) => p.chatId === 'c1')
    expect(newPane).toBeDefined()
    expect(newPane?.id).not.toBe(ROOT_PANE_ID)
    expect(windowPaneStore.getState().dormantArrangements).toEqual([])
  })

  it('a center drop onto an OCCUPIED pane never swaps — it merges instead (rule 1: every drop adds)', () => {
    windowPaneStore.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'c1', 'runner-1')

    performSidebarPaneDrop([chatRow('c2', 'ws-1')], ROOT_PANE_ID, 'center')

    // c1 is still exactly where it was — nothing was evicted.
    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBe('c1')
    const newPane = Object.values(windowPaneStore.getState().panes).find((p) => p.chatId === 'c2')
    expect(newPane).toBeDefined()
  })

  it('an edge drop onto an occupied pane groups both chats into one Recents entry ("side by side")', () => {
    windowPaneStore.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'c1', 'runner-1')

    performSidebarPaneDrop([chatRow('c2', 'ws-1')], ROOT_PANE_ID, 'right')

    expect(windowPaneStore.getState().dormantArrangements).toHaveLength(1)
    const [entry] = windowPaneStore.getState().dormantArrangements
    expect([...entry.chatIds].sort()).toEqual(['c1', 'c2'])
  })

  it('merging into a pane already part of an arrangement GROWS it rather than nesting a second one', () => {
    windowPaneStore.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'c1', 'runner-1')
    performSidebarPaneDrop([chatRow('c2', 'ws-1')], ROOT_PANE_ID, 'right') // c1+c2 now grouped

    performSidebarPaneDrop([chatRow('c3', 'ws-1')], ROOT_PANE_ID, 'bottom')

    expect(windowPaneStore.getState().dormantArrangements).toHaveLength(1)
    const [entry] = windowPaneStore.getState().dormantArrangements
    expect([...entry.chatIds].sort()).toEqual(['c1', 'c2', 'c3'])
  })

  it('dropping an already-grouped chat elsewhere reveals it in place — the group is untouched', () => {
    windowPaneStore.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'c1', 'runner-1')
    performSidebarPaneDrop([chatRow('c2', 'ws-1')], ROOT_PANE_ID, 'right') // c1+c2 now grouped
    const grouped = windowPaneStore.getState().dormantArrangements

    const freshPane = windowPaneStore.getState().paneActions.splitPane(ROOT_PANE_ID, 'vertical')!
    performSidebarPaneDrop([chatRow('c1', 'ws-1')], freshPane, 'center')

    expect(windowPaneStore.getState().panes[freshPane]?.chatId).toBeNull()
    expect(windowPaneStore.getState().activePaneId).toBe(ROOT_PANE_ID)
    expect(windowPaneStore.getState().dormantArrangements).toEqual(grouped)
  })
})

describe('performSidebarPaneDrop — cross-workspace (Task 26 fix round 1, Critical 2)', () => {
  // The panes/buffers DATA hoist made this look safe to drop (there is
  // exactly one pane store now, no more "wrong store" to mutate), but the
  // RENDER side was never rebuilt to match: a pane resolves "is this chat
  // known" through the AMBIENT WorkspaceStoreContext of whichever
  // WorkspaceView happens to render it, not the chat's real owning
  // workspace — no chatId->workspace lookup exists in the render path. A
  // chat from an off-screen workspace landed here would never be found in
  // the on-screen workspace's agentChats.chats: the pane renders permanently
  // blank, no CLI ever spawns, and — since setPaneChat persists to
  // IndexedDB — it SURVIVES RELOAD. Refused until that resolution mechanism
  // is actually built.
  it('refuses a drop when the dragged chat belongs to a workspace other than the one whose pane was actually hit', () => {
    setActiveWorkspaceId('ws-visible') // ws-visible is what's on screen
    windowPaneStore.getState().paneActions.setPaneChat(ROOT_PANE_ID, 'already-here', 'runner-1')

    performSidebarPaneDrop([chatRow('c1', 'ws-offscreen')], ROOT_PANE_ID, 'right')

    const newPane = Object.values(windowPaneStore.getState().panes).find((p) => p.chatId === 'c1')
    expect(newPane).toBeUndefined()
    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBe('already-here')
  })

  it('still works normally once the chat and the active workspace agree', () => {
    setActiveWorkspaceId('ws-visible')

    performSidebarPaneDrop([chatRow('c1', 'ws-visible')], ROOT_PANE_ID, 'center')

    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBe('c1')
  })
})

/**
 * The FOURTH id-space consumer of a branch row's id, after
 * `resolveChatRow`/`resolveRow`/`performRenameRow`. A branch row is addressed
 * by the chat that owns its workspace (`rows-from-repo.ts`), and everything
 * this file computes with — `buildSidebarTree`'s nodes, `placeWorkspace`,
 * `reparentWorkspace` — is in the WORKSPACE id space. Untranslated,
 * `resolveRowRepo` returned null for the repo-home row and every locked
 * branch, `planTreeRowDrop` returned `[]`, and the drop the indicator had just
 * promised fired no request at all.
 */
describe('performSidebarDrop — a branch row is addressed by its owning chat', () => {
  /** The repo above, plus the `branch` rows the boot backfill mints: one for
   *  the home workspace, one for the locked branch `ws-a`. */
  function repoWithBranchRows(): Repo {
    const base = makeRepo()
    return {
      ...base,
      workspaces: base.workspaces.map((w) => (w.id === 'ws-a' ? { ...w, status: 'locked' } : w)),
      chats: [
        {
          id: 'home-row',
          repoId: 'repo-1',
          type: 'branch',
          workspaceId: 'home-1',
          title: '',
          order: 0,
        },
        {
          id: 'ws-a-row',
          repoId: 'repo-1',
          type: 'branch',
          workspaceId: 'ws-a',
          title: '',
          order: 0,
        },
      ],
    }
  }

  beforeEach(() => {
    useSidebarStore.setState({ ...getInitialState(), repos: [repoWithBranchRows()] })
  })

  it('dropping INTO the repo-home row places at the repo root, not silently nothing', async () => {
    await performSidebarDrop(
      [branchRow('ws-c')],
      branchRow('home-row', { workspaceId: 'home-1' }),
      'into',
    )

    expect(placeWorkspace).toHaveBeenCalledOnce()
    expect(placeWorkspace).toHaveBeenCalledWith('proj-1', 'repo-1', 'ws-c', {
      folderId: '',
      order: expect.any(Number),
    })
  })

  it('dropping INTO a locked branch row reparents onto its WORKSPACE, not its chat id', async () => {
    await performSidebarDrop(
      [branchRow('ws-b')],
      branchRow('ws-a-row', { workspaceId: 'ws-a', parentId: 'home-row' }),
      'into',
    )

    expect(reparentWorkspace).toHaveBeenCalledWith('proj-1', 'repo-1', 'ws-b', 'ws-a')
  })

  it('a branch row as the SUBJECT is placed by its workspace id', async () => {
    await performSidebarDrop(
      [branchRow('ws-a-row', { workspaceId: 'ws-a' })],
      folderRow('folder-1', { parentId: 'home-row' }),
      'into',
    )

    expect(placeWorkspace).toHaveBeenCalledWith(
      'proj-1',
      'repo-1',
      'ws-a',
      expect.objectContaining({ folderId: 'folder-1' }),
    )
  })

  it('reordering BEFORE a branch row lands in its real sibling space', async () => {
    await performSidebarDrop(
      [branchRow('ws-c')],
      branchRow('ws-a-row', { workspaceId: 'ws-a', parentId: 'home-row' }),
      'before',
    )

    // ws-a sits at index 0 of the repo root, so `ws-c` takes that slot — the
    // index is only computable if the target resolved to `ws-a` at all.
    expect(placeWorkspace).toHaveBeenCalledWith(
      'proj-1',
      'repo-1',
      'ws-c',
      expect.objectContaining({ order: 0 }),
    )
  })
})
