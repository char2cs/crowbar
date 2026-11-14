import { describe, expect, it, vi, afterEach, beforeEach } from 'vitest'
import { stubDropBoundary } from '@/__tests__/__fixtures__/drop-boundary'
import { getHomeWorkspaceId } from '@/features/workspace/lib/home-workspace-resolver'

import { performSidebarDrop } from '@/components/sidebar/lib/drop-actions'
import {
  openChatInOwnPane,
  performSidebarPaneDrop,
} from '@/components/sidebar/lib/pane-drop-actions'
import { getAllLeafIds } from '@/features/panes/utils/pane-layout'
import {
  placeWorkspace,
  placeFolder,
  placeHomeFolder,
  placeRepo,
} from '@/lib/api/sidebar-placement'
import { reparentWorkspace } from '@/lib/api/workspace'
import { setChatPlacement } from '@/features/agent/api/agent-chat-commands'
import { HostNotResolvedError } from '@/lib/host-path'
import { toastManager } from '@/lib/toast-manager'
import { recordWorkspaceScope } from '@/lib/workspace-scope'
import { streamOperation, resetOperations } from '@/__tests__/__fixtures__/operations'
import { operationsState, type Operation } from '@/core/entities/operations'
import {
  destroyWorkspaceStore,
  getAllActiveWorkspaceIds,
  setActiveWorkspaceId,
} from '@/features/workspace/stores/workspace-store-registry'
import type { Chat, Folder, Repo } from '@/core/entities/repo-types'
import { treeUiStore } from '@/features/workspace/tree/tree-ui-store'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { resetWindowPaneStoreForTests } from '@/__tests__/__fixtures__/window-pane-store'
import { showingLayout, viewChatIds } from '@/features/panes/lib/view-state'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import { nextVersion, seedChats } from '@/__tests__/__fixtures__/agent-chat'
import type { AgentChat } from '@/features/agent/api/agent-chat-reads'
import {
  resetEntities,
  seedHomeTree,
  seedProjects,
  seedRepos,
} from '@/__tests__/__fixtures__/sidebar-entities'
import { readSelect, selectHomeTree, selectRepos } from '@/core/entities/select'
import { ApiError } from '@/lib/errors/api-error'

// `performSidebarDrop` is the row-to-row drop (container and fork-lineage math over
// `SidebarRow`s, no optimistic paint); `performSidebarPaneDrop` below is the drop onto a pane.

/** Which view record holds `chatId` — the grouping fact a merge writes and a
 *  click never shares. */
function liveViewOf(chatId: string): string | undefined {
  const pane = Object.values(windowPaneStore.getState().panes).find((p) => p.chatId === chatId)
  return pane?.viewId ?? undefined
}

function paneOfChat(chatId: string): string {
  return Object.values(windowPaneStore.getState().panes).find((p) => p.chatId === chatId)!.id
}

function viewOfPane(paneId: string): string {
  return windowPaneStore.getState().panes[paneId].viewId!
}

/** The chats of every Recents row, in band order — one row per view. */
function liveRecents(): string[][] {
  const state = windowPaneStore.getState()
  return state.viewOrder.map((id) => [...viewChatIds(state, id)].sort())
}

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

/** A drop failure reaches the user through `report()`, titled by the action. */
function expectReported(description: string): void {
  expect(toastManager.add).toHaveBeenCalledWith(expect.objectContaining({ description }))
}

/** The WS frame a landed reparent sends: a fresh `Workspace` with the new parent. */
function confirmReparent(wsId: string, parentId: string): void {
  seedRepos(
    readSelect(selectRepos).map((r) => ({
      ...r,
      workspaces: r.workspaces.map((w) => (w.id === wsId ? { ...w, parentId } : w)),
    })),
  )
}

beforeEach(() => {
  vi.clearAllMocks()
  stubDropBoundary()
  resetEntities()
  seedRepos([makeRepo()])
  treeUiStore.setState(treeUiStore.getInitialState(), true)
  setActiveWorkspaceId('ws-1')
  resetOperations()
  vi.spyOn(toastManager, 'add')
  // A reparent is addressed by the fork's owning chat (worktreeVerbBaseForWorkspace).
  for (const { id } of makeRepo().workspaces)
    recordWorkspaceScope({ projectId: 'proj-1', repoId: 'repo-1', wsId: id, owningChatId: id })
  // Default: the reparent lands within its request; the operation tests below
  // override this to answer 202 and settle it by frame.
  vi.mocked(reparentWorkspace).mockImplementation(async (wsId, parentId) => {
    confirmReparent(wsId, parentId)
    return { items: [] }
  })
})

afterEach(() => {
  getAllActiveWorkspaceIds().forEach((id) => destroyWorkspaceStore(id))
  // Panes/buffers are a window-level singleton destroyWorkspaceStore never touches.
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
    expect(placeWorkspace).toHaveBeenCalledWith('ws-c', {
      folderId: '',
      order: 0,
    })
    expect(reparentWorkspace).not.toHaveBeenCalled()
  })

  // Caught live: a locked branch dragged past a sibling PATCHed 200 and
  // nothing moved until a reload. A branch row's order comes off its
  // WorkspaceDTO, which no placement frame refreshes, so the drop applies
  // the PATCH answer itself: the moved row and every shifted sibling —
  // folders and the locked branches it renumbered past, by workspace id.
  it('applies the PATCH answer — the moved row and its shifted folder and branch siblings — with no re-read', async () => {
    vi.mocked(placeWorkspace).mockResolvedValueOnce({
      workspace: { id: 'ws-c', parentId: '', order: 0 },
      shifted: [
        { id: 'folder-1', parentId: '', order: 4 },
        { id: 'ws-a', parentId: '', order: 1 },
        { id: 'ws-b', parentId: '', order: 2 },
      ],
    })

    await performSidebarDrop(
      [branchRow('ws-c')],
      branchRow('ws-a', { parentId: 'home-1' }),
      'before',
    )

    const repo = readSelect(selectRepos).find((r) => r.id === 'repo-1')!
    const orderOf = (id: string) => repo.workspaces.find((w) => w.id === id)?.order
    expect([orderOf('ws-c'), orderOf('ws-a'), orderOf('ws-b')]).toEqual([0, 1, 2])
    expect(repo.folders?.find((f) => f.id === 'folder-1')?.order).toBe(4)
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

  // A level interleaves branches, folders AND chats on one order, so a branch dropped beside a
  // CHAT sibling indexes over all three.
  describe('a branch beside its CHAT siblings', () => {
    beforeEach(() => {
      seedRepos([
        {
          ...makeRepo(),
          chats: [
            repoChat('chat-1', { repoId: 'repo-1', workspaceId: 'home-1', order: 5 }),
            repoChat('chat-2', { repoId: 'repo-1', workspaceId: 'home-1', order: 6 }),
            repoChat('thread-b', {
              repoId: 'repo-1',
              workspaceId: 'ws-b',
              parentId: 'ws-b',
              order: 0,
            }),
          ],
        },
      ])
    })

    it('indexes a branch dropped past a root-level chat over the rows the tree draws', async () => {
      // Root: [ws-a, ws-b, ws-c, folder-1, folder-2, chat-1, chat-2] — ws-a
      // lifted, after chat-1 -> 5 (the chat-less tree said 4).
      await performSidebarDrop(
        [branchRow('ws-a')],
        chatRow('chat-1', 'home-1', { parentId: 'home-1' }),
        'after',
      )

      expect(placeWorkspace).toHaveBeenCalledWith('ws-a', { folderId: '', order: 5 })
    })

    it('"after" an expanded branch whose only children are chats re-parents as its first child', async () => {
      await performSidebarDrop(
        [branchRow('ws-c')],
        branchRow('ws-b', { parentId: 'home-1' }),
        'after',
      )

      expect(reparentWorkspace).toHaveBeenCalledWith('ws-c', 'ws-b', expect.any(String))
      expect(placeWorkspace).toHaveBeenCalledWith('ws-c', { order: 0 })
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

    expect(placeWorkspace).toHaveBeenCalledWith('ws-b', {
      folderId: 'folder-1',
      order: 0,
    })
    expect(reparentWorkspace).not.toHaveBeenCalled()
  })

  it('drops a FORK into a folder outside its own lineage — reparents before placing', async () => {
    // ws-fork hangs off ws-a; folder-1 sits at the repo root, no lineage of
    // its own — dropping the fork there must rebase it onto the repo's
    // default checkout, same as dropping it on the repo home row, AND file
    // it into folder-1.
    await performSidebarDrop(
      [branchRow('ws-fork')],
      folderRow('folder-1', { parentId: 'home-1' }),
      'into',
    )

    expect(reparentWorkspace).toHaveBeenCalledWith('ws-fork', 'home-1', expect.any(String))
    expect(placeWorkspace).toHaveBeenCalledWith('ws-fork', {
      folderId: 'folder-1',
      order: 0,
    })
  })

  it('drops a plain branch into a folder nested under a DIFFERENT branch — adopts that lineage', async () => {
    // ws-b has no fork parent of its own; folder-3 is nested under ws-a —
    // dropping ws-b there must rebase it onto ws-a AND file it into folder-3.
    await performSidebarDrop(
      [branchRow('ws-b')],
      folderRow('folder-3', { parentId: 'ws-a' }),
      'into',
    )

    expect(reparentWorkspace).toHaveBeenCalledWith('ws-b', 'ws-a', expect.any(String))
    expect(placeWorkspace).toHaveBeenCalledWith('ws-b', {
      folderId: 'folder-3',
      order: 1,
    })
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
    // appends after both. ws-a owns no chat, so the row is addressed by its
    // own workspace id (its Node row) — never bare '', which is the repo
    // ROOT, a different level whose siblings the daemon would densify
    // against (regression: A/B under a chatless branch kept tied orders).
    expect(placeWorkspace).toHaveBeenCalledWith('ws-d', {
      folderId: 'ws-a',
      order: 2,
    })
  })

  it('reordering a fork past its sibling under a CHATLESS branch files it under that branch, not the root', async () => {
    await performSidebarDrop(
      [branchRow('ws-d')],
      branchRow('ws-fork', { parentId: 'ws-a' }),
      'before',
    )

    expect(reparentWorkspace).not.toHaveBeenCalled()
    expect(placeWorkspace).toHaveBeenCalledWith('ws-d', { folderId: 'ws-a', order: 0 })
  })
})

// A row directly under a locked branch has that branch's OWNING CHAT as its Node
// parent, never bare '' (the project root, a different level the row would leave).
describe('performSidebarDrop — folder edge for a workspace container that owns a chat', () => {
  it("landing directly under a workspace container writes that workspace's OWNING CHAT id, never bare ''", async () => {
    seedRepos([
      {
        ...makeRepo(),
        workspaces: [
          { id: 'ws-a', branch: 'a', age: '', order: 0, owningChatId: 'chat-a' },
          { id: 'ws-fork', branch: 'fork', age: '', order: 0, parentId: 'ws-a' },
          { id: 'ws-e', branch: 'e', age: '', order: 1, parentId: 'ws-a' },
        ],
      },
    ])

    // The tree draws ws-a by its owner's id (deriveRows folds on the
    // DTO's `owningChatId` alone), so its children's rows carry that parent.
    await performSidebarDrop(
      [branchRow('ws-fork')],
      branchRow('ws-e', { parentId: 'chat-a' }),
      'before',
    )

    expect(reparentWorkspace).not.toHaveBeenCalled()
    expect(placeWorkspace).toHaveBeenCalledWith('ws-fork', { folderId: 'chat-a', order: 1 })
  })
})

// A folder filed directly under a branch row carries that row's owning-chat id as
// `parentId`; the fork-lineage walk must resolve it to the workspace, not fall to
// the root and rebase the fork onto the default checkout.
describe('performSidebarDrop — folder nested under a workspace container that owns a chat', () => {
  it('filing a fork into a folder under its OWN locked fork parent is a placement only — no reparent', async () => {
    seedRepos([
      {
        ...makeRepo(),
        workspaces: [
          { id: 'ws-a', branch: 'a', age: '', order: 0, owningChatId: 'chat-a' },
          { id: 'ws-fork', branch: 'fork', age: '', order: 0, parentId: 'ws-a' },
        ],
        folders: [
          { id: 'folder-nested', repoId: 'repo-1', name: 'Nested', parentId: 'chat-a', order: 0 },
        ],
      },
    ])

    await performSidebarDrop(
      [branchRow('ws-fork')],
      folderRow('folder-nested', { parentId: 'chat-a' }),
      'into',
    )

    expect(reparentWorkspace).not.toHaveBeenCalled()
    expect(placeWorkspace).toHaveBeenCalledWith('ws-fork', { folderId: 'folder-nested', order: 0 })
  })
})

// A pure reorder of a folder within its own siblings sends no `parentId`: a
// recomputed one can disagree with the stored anchor and 400 as a context move.
describe('performSidebarDrop — reordering a folder within its own current container', () => {
  it('sends no parentId at all when the folder is not actually changing container', async () => {
    seedRepos([
      {
        ...makeRepo(),
        workspaces: [{ id: 'ws-a', branch: 'a', age: '', order: 0, owningChatId: 'chat-a' }],
        folders: [
          { id: 'folder-first', repoId: 'repo-1', name: 'First', parentId: 'chat-a', order: 0 },
          { id: 'folder-second', repoId: 'repo-1', name: 'Second', parentId: 'chat-a', order: 1 },
        ],
      },
    ])

    // Drag "folder-second" to become the FIRST item of the same sibling list
    // — dropped "before" "folder-first", the row currently occupying that
    // slot, both already filed under the identical container ("chat-a").
    await performSidebarDrop(
      [folderRow('folder-second', { parentId: 'chat-a' })],
      folderRow('folder-first', { parentId: 'chat-a' }),
      'before',
    )

    expect(placeFolder).toHaveBeenCalledWith('proj-1', 'repo-1', 'folder-second', { order: 0 })
    expect(placeFolder).not.toHaveBeenCalledWith(
      'proj-1',
      'repo-1',
      'folder-second',
      expect.objectContaining({ parentId: expect.anything() }),
    )
  })

  it('still sends parentId for a genuine cross-container move', async () => {
    seedRepos([
      {
        ...makeRepo(),
        workspaces: [{ id: 'ws-a', branch: 'a', age: '', order: 0, owningChatId: 'chat-a' }],
        folders: [
          { id: 'folder-nested', repoId: 'repo-1', name: 'Nested', parentId: 'chat-a', order: 0 },
          // folder-1 stays at the bare repo root — a genuinely different
          // container than folder-nested's own ("chat-a").
          { id: 'folder-1', repoId: 'repo-1', name: 'Bugs', order: 0 },
        ],
      },
    ])

    // folder-1/folder-2 sit at the bare repo root — a genuinely different
    // container than folder-nested's own ("chat-a").
    await performSidebarDrop(
      [folderRow('folder-nested', { parentId: 'chat-a' })],
      folderRow('folder-1', { parentId: 'home-1' }),
      'before',
    )

    expect(placeFolder).toHaveBeenCalledWith('proj-1', 'repo-1', 'folder-nested', {
      parentId: '',
      order: expect.any(Number),
    })
  })
})

describe('performSidebarDrop — crossing a fork parent', () => {
  it('reparents before placing when the destination is under a different fork parent', async () => {
    // ws-fork currently hangs off ws-a; dropped INTO ws-b it must rebase.
    await performSidebarDrop([branchRow('ws-fork')], branchRow('ws-b'), 'into')

    expect(reparentWorkspace).toHaveBeenCalledWith('ws-fork', 'ws-b', expect.any(String))
    expect(placeWorkspace).toHaveBeenCalledWith('ws-fork', { order: 0 })
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

    expect(reparentWorkspace).toHaveBeenCalledWith('ws-b', 'ws-a', expect.any(String))
    expect(placeWorkspace).toHaveBeenCalledWith('ws-b', { order: 0 })
  })

  it('the same "after" drop on a COLLAPSED row is a plain sibling reorder instead', async () => {
    treeUiStore.setState({ collapsed: new Set(['ws-a']) })

    await performSidebarDrop(
      [branchRow('ws-b')],
      branchRow('ws-a', { parentId: 'home-1' }),
      'after',
    )

    expect(reparentWorkspace).not.toHaveBeenCalled()
    // ws-a sits at index 0 among the root siblings, so "after" it is index 1.
    expect(placeWorkspace).toHaveBeenCalledWith('ws-b', {
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

    expect(reparentWorkspace).toHaveBeenCalledWith('ws-fork', 'home-1', expect.any(String))
    // Root-level siblings (5 of them) plus this one landing at the end.
    expect(placeWorkspace).toHaveBeenCalledWith('ws-fork', { order: 5 })
  })
})

describe('performSidebarDrop — a reparent runs as an operation', () => {
  /** Answers the reparent 202 with an accepted operation; `settle` sends its terminal frame. */
  function acceptReparent(): { settle: (change: Partial<Operation>) => void } {
    let op: Operation
    vi.mocked(reparentWorkspace).mockImplementationOnce(async (wsId, _parentId, id) => {
      op = { id, kind: 'workspace.reparent', subject: { kind: 'workspace', id: wsId } } as Operation
      return { operation: { ...op, state: 'accepted', items: [], version: 1, createdAt: '' } }
    })
    return { settle: (change) => streamOperation({ ...op, items: [], version: 2, ...change }) }
  }

  it('a reparent the daemon refuses reports its reason and skips the placement call', async () => {
    const reparent = acceptReparent()
    const done = performSidebarDrop([branchRow('ws-fork')], branchRow('ws-b'), 'into')
    await vi.waitFor(() => expect(reparentWorkspace).toHaveBeenCalled())

    reparent.settle({ state: 'failed', error: { code: 'conflict', message: 'has fork children' } })
    await done

    expect(placeWorkspace).not.toHaveBeenCalled()
    expect(Object.values(operationsState().ops)[0].error?.message).toBe('has fork children')
    // The operations watcher toasts it; the drop does not toast it twice.
    expect(toastManager.add).not.toHaveBeenCalled()
  })

  it('a reparent that succeeds places the workspace after the row moved', async () => {
    const reparent = acceptReparent()
    const done = performSidebarDrop([branchRow('ws-fork')], branchRow('ws-b'), 'into')
    await vi.waitFor(() =>
      expect(reparentWorkspace).toHaveBeenCalledWith('ws-fork', 'ws-b', expect.any(String)),
    )
    expect(placeWorkspace).not.toHaveBeenCalled()

    confirmReparent('ws-fork', 'ws-b')
    reparent.settle({ state: 'succeeded' })
    await done

    expect(placeWorkspace).toHaveBeenCalledWith('ws-fork', { order: 0 })
  })

  // The daemon refuses a fork parent with no checkout (guardReparent); the
  // DTO says which branches those are, so the drop is refused before any call.
  it('refuses a reparent onto a placeholder branch before calling the daemon', async () => {
    seedRepos(
      readSelect(selectRepos).map((r) => ({
        ...r,
        workspaces: r.workspaces.map((w) =>
          w.id === 'ws-b' ? { ...w, provisioning: 'placeholder' as const } : w,
        ),
      })),
    )

    await performSidebarDrop([branchRow('ws-fork')], branchRow('ws-b'), 'into')

    expect(reparentWorkspace).not.toHaveBeenCalled()
    expectReported("That branch hasn't been checked out yet — try again once it has")
  })
})

describe('performSidebarDrop — multi-row moves', () => {
  it('fires each call in order, awaiting the previous one before the next starts', async () => {
    let resolveFirst!: () => void
    const pending = new Promise<Awaited<ReturnType<typeof placeWorkspace>>>((resolve) => {
      resolveFirst = () =>
        resolve({ workspace: { id: 'ws-b', parentId: '', order: 0 }, shifted: [] })
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
    expect(placeWorkspace).toHaveBeenNthCalledWith(1, 'ws-b', {
      folderId: '',
      order: 0,
    })
    expect(placeWorkspace).toHaveBeenNthCalledWith(2, 'ws-c', { folderId: '', order: 1 })
  })
})

describe('performSidebarDrop — failures', () => {
  it('a failed API call is reported, not thrown', async () => {
    vi.mocked(placeWorkspace).mockRejectedValueOnce(new ApiError('locked', 500))

    await expect(
      performSidebarDrop([branchRow('ws-c')], branchRow('ws-a', { parentId: 'home-1' }), 'before'),
    ).resolves.toBeUndefined()

    expectReported('locked')
  })

  it('a move whose host is not known yet says the projects are loading, not why inside', async () => {
    vi.mocked(placeWorkspace).mockRejectedValueOnce(
      new HostNotResolvedError('workspace', 'ws-c', 'no scope is recorded for the workspace'),
    )

    await performSidebarDrop(
      [branchRow('ws-c')],
      branchRow('ws-a', { parentId: 'home-1' }),
      'before',
    )

    expectReported('Still loading your projects')
  })
})

describe('performSidebarDrop — the repo home row', () => {
  // Without `repoIcon` this row is indistinguishable from an ordinary branch
  // that merely shares the home workspace's id — `planTreeRowDrop`'s own
  // early return (it is not a member of `repo.workspaces`) is the fallback
  // that keeps THAT case a no-op rather than constructing a bogus
  // `placeWorkspace` call. A row carrying `repoIcon` (see the describe block
  // below) takes an entirely different path now.
  it('a workspace-kind row sharing the home id but carrying no repoIcon is a no-op', async () => {
    await expect(
      performSidebarDrop(
        [branchRow('home-1', { parentId: null, workspaceId: 'home-1' })],
        branchRow('ws-a', { parentId: 'home-1' }),
        'after',
      ),
    ).resolves.toBeUndefined()

    expect(placeWorkspace).not.toHaveBeenCalled()
    expect(reparentWorkspace).not.toHaveBeenCalled()
    expect(placeRepo).not.toHaveBeenCalled()
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
    expect(placeWorkspace).toHaveBeenCalledWith('ws-b', {
      folderId: '',
      order: 4,
    })
  })

  it('dropping "after" the EXPANDED home row lands as the FIRST root-level row, not the last', async () => {
    // The home row is a row like any other, but it is not a node in
    // `legacyTreeIndex`'s own graph — its rendered children ARE the
    // tree's roots. Naively reusing `findNode` for it would always report
    // zero children and silently turn this into an append-at-the-end.
    await performSidebarDrop(
      [branchRow('ws-c')],
      branchRow('home-1', { parentId: null, workspaceId: 'home-1' }),
      'after',
    )

    expect(reparentWorkspace).not.toHaveBeenCalled()
    expect(placeWorkspace).toHaveBeenCalledWith('ws-c', {
      folderId: '',
      order: 0,
    })
  })
})

describe('performSidebarDrop — unresolvable rows', () => {
  it('a target the live store does not recognise is a no-op, not a throw', async () => {
    seedRepos([])

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
    treeUiStore.setState({ hiddenIds: new Set(['ws-fork', 'folder-3', 'ws-d']) })

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
    expect(placeWorkspace).toHaveBeenCalledWith('ws-b', {
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
  working: false,
  version: nextVersion(),
  phase: 'dormant',
  createdAt: '2026-01-01T00:00:00Z',
  order: 0,
  parentId: '',
  ...over,
})

/**
 * A repo whose (chatless) main checkout `ws-x` holds the chats/folders a test
 * seeds — drawn by `deriveRows` under its header row, id'd `ws-x`. The
 * sidebar store IS what the tree draws; a drop plans over those rows and never
 * reads (or mints) the per-workspace store, which is empty unless that
 * workspace's pane happens to be mounted.
 */
function seedRepoChats(chats: Chat[], folders: Folder[] = []): void {
  seedRepos([
    ...readSelect(selectRepos).filter((r) => r.id !== 'repo-chat-x'),
    {
      id: 'repo-chat-x',
      projectId: 'proj-1',
      name: 'repo-chat-x',
      avatarLabel: 'X',
      avatarColor: 'bg-indigo-700',
      defaultWorkspaceId: 'ws-x',
      workspaces: [],
      chats,
      folders,
    },
  ])
}

const repoChat = (id: string, over: Partial<Chat> = {}): Chat => ({
  id,
  repoId: 'repo-chat-x',
  workspaceId: 'ws-x',
  title: id,
  order: 0,
  ...over,
})

/** A chat row as `deriveRows` draws one at repo-chat-x's root: under the header. */
const rootChatRow = (id: string, over: Partial<SidebarRow> = {}) =>
  chatRow(id, 'ws-x', { parentId: 'ws-x', ...over })

describe('performSidebarDrop — chats', () => {
  it('reorders a chat among its siblings in its own workspace via setChatPlacement', async () => {
    seedRepoChats([
      repoChat('chat-a', { order: 0 }),
      repoChat('chat-b', { order: 1 }),
      repoChat('chat-c', { order: 2 }),
    ])

    await performSidebarDrop([rootChatRow('chat-c')], rootChatRow('chat-a'), 'before')

    expect(setChatPlacement).toHaveBeenCalledWith('ws-x', 'chat-c', {
      parentId: 'ws-x',
      order: 0,
    })
    expect(placeWorkspace).not.toHaveBeenCalled()
  })

  // The index comes off the rendered tree, never a mounted workspace's store.
  it('indexes a repo chat off the rendered tree while its workspace is NOT mounted', async () => {
    seedRepoChats([
      repoChat('chat-a', { order: 0 }),
      repoChat('chat-b', { order: 1 }),
      repoChat('chat-c', { order: 2 }),
    ])
    expect(getAllActiveWorkspaceIds()).not.toContain('ws-x')

    await performSidebarDrop([rootChatRow('chat-a')], rootChatRow('chat-c'), 'after')

    // a lifted out of [a, b, c] -> [b, c]; after c -> index 2.
    expect(setChatPlacement).toHaveBeenCalledWith('ws-x', 'chat-a', {
      parentId: 'ws-x',
      order: 2,
    })
    expect(getAllActiveWorkspaceIds()).not.toContain('ws-x')
  })

  it('dropping a chat "into" another chat makes it one of its threads', async () => {
    seedRepoChats([repoChat('chat-a'), repoChat('chat-b')])

    await performSidebarDrop([rootChatRow('chat-b')], rootChatRow('chat-a'), 'into')

    expect(setChatPlacement).toHaveBeenCalledWith('ws-x', 'chat-b', {
      parentId: 'chat-a',
      order: 0,
    })
  })

  // The gap under an EXPANDED parent is the slot before its first child: where the drop line is
  // drawn, and what every other planner means by 'after' there.
  describe('"after" an expanded chat with threads', () => {
    beforeEach(() => {
      seedRepoChats([
        repoChat('chat-a', { order: 0 }),
        repoChat('chat-t', { parentId: 'chat-a', order: 0 }),
        repoChat('chat-b', { order: 1 }),
      ])
    })

    it('files the chat as its FIRST child', async () => {
      await performSidebarDrop([rootChatRow('chat-b')], rootChatRow('chat-a'), 'after')

      expect(setChatPlacement).toHaveBeenCalledWith('ws-x', 'chat-b', {
        parentId: 'chat-a',
        order: 0,
      })
    })

    it('is a plain sibling reorder once that parent is COLLAPSED', async () => {
      treeUiStore.setState({ collapsed: new Set(['chat-a']) })

      await performSidebarDrop([rootChatRow('chat-b')], rootChatRow('chat-a'), 'after')

      expect(setChatPlacement).toHaveBeenCalledWith('ws-x', 'chat-b', {
        parentId: 'ws-x',
        order: 1,
      })
    })
  })

  // `setChatPlacement`'s response is applied to the entity store `deriveRows`
  // reads; discarded, the PATCH landed but nothing on screen moved.
  it('applies its own setChatPlacement response directly to the sidebar tree store', async () => {
    seedRepos([
      ...readSelect(selectRepos),
      {
        id: 'repo-chat-x',
        projectId: 'proj-1',
        name: 'repo-chat-x',
        avatarLabel: 'X',
        avatarColor: 'bg-indigo-700',
        defaultWorkspaceId: 'ws-x',
        workspaces: [],
        chats: [{ id: 'chat-c', repoId: 'repo-chat-x', workspaceId: 'ws-x', title: 'c', order: 2 }],
      },
    ])
    seedChats([
      chat('chat-a', 'ws-x', { order: 0 }),
      chat('chat-b', 'ws-x', { order: 1 }),
      chat('chat-c', 'ws-x', { order: 2 }),
    ])

    await performSidebarDrop([chatRow('chat-c', 'ws-x')], chatRow('chat-a', 'ws-x'), 'into')

    const patched = readSelect(selectRepos)
      .find((r) => r.id === 'repo-chat-x')
      ?.chats?.find((c) => c.id === 'chat-c')
    expect(patched).toMatchObject({ parentId: 'chat-a', order: 0 })
  })

  it('computes the insert index against the REAL sibling order, not a raw [...chats, ...folders] concat', async () => {
    // Real order (order ascending, ties by arrival): folder-z(0), chat-x(1),
    // chat-y(2). A naive concat instead pushes every folder after every chat
    // regardless of `order`, seeing [chat-x, chat-y, folder-z] — a DIFFERENT
    // list, so a DIFFERENT index.
    seedRepoChats(
      [repoChat('chat-x', { order: 1 }), repoChat('chat-y', { order: 2 })],
      [{ id: 'folder-z', repoId: 'repo-chat-x', name: 'z', order: 0 }],
    )

    await performSidebarDrop([rootChatRow('chat-x')], rootChatRow('chat-y'), 'before')

    // Real siblings minus chat-x: [folder-z, chat-y] — "before" chat-y is
    // index 1. (The naive concat would have computed 0.)
    expect(setChatPlacement).toHaveBeenCalledWith('ws-x', 'chat-x', {
      parentId: 'ws-x',
      order: 1,
    })
  })

  // Which workspace a container runs in is drop-plan.test.ts's; here, what the PATCH carries.
  const intoBranchA = () =>
    performSidebarDrop([chatRow('c1', 'ws-x')], branchRow('ws-a', { parentId: 'home-1' }), 'into')

  it('moves a chat dropped into a branch row to run in that branch, named by its branch', async () => {
    await intoBranchA()

    expect(setChatPlacement).toHaveBeenCalledWith('ws-x', 'c1', {
      parentId: 'ws-a',
      order: 2,
      workspaceId: 'ws-a',
      label: 'a',
    })
  })

  it('says why the daemon refused a move, in its words', async () => {
    vi.mocked(setChatPlacement).mockRejectedValueOnce(
      new ApiError("A branch's own thread moves with its branch", 409),
    )

    await intoBranchA()

    expectReported("A branch's own thread moves with its branch")
  })

  // A folder row is the same `AgentChatFolder` a chat files into, so the drop is a real placement.
  it('a chat dropped onto a folder row files it there', async () => {
    seedRepoChats(
      [repoChat('chat-a')],
      [{ id: 'folder-x', repoId: 'repo-chat-x', name: 'x', order: 1 }],
    )

    await performSidebarDrop(
      [rootChatRow('chat-a')],
      { ...rootChatRow('folder-x'), kind: 'folder', workspaceId: null },
      'into',
    )

    expect(setChatPlacement).toHaveBeenCalledWith('ws-x', 'chat-a', {
      parentId: 'folder-x',
      order: 0,
    })
  })

  // A folder row carries no `workspaceId`: a home folder's tree names the home workspace.
  it('a chat dropped onto a home folder resolves the home workspace off the folder’s tree', async () => {
    vi.mocked(getHomeWorkspaceId).mockReturnValue('home-ws-1')
    seedHomeTree('proj-1', {
      chats: [{ id: 'c1', repoId: '', workspaceId: 'home-ws-1', title: 'c1', order: 0 }],
      folders: [{ id: 'home-folder-1', repoId: '', name: 'F', order: 1 }],
    })

    await performSidebarDrop(
      [chatRow('c1', 'home-ws-1')],
      { ...chatRow('home-folder-1', 'home-ws-1'), kind: 'folder', workspaceId: null },
      'into',
    )

    expect(setChatPlacement).toHaveBeenCalledWith('home-ws-1', 'c1', {
      parentId: 'home-folder-1',
      order: 0,
    })
  })

  // Beside a branch row is its parent's level, inside it the branch's: indexed over the drawn rows.
  describe('reordering past a branch row', () => {
    /** repo-1's header row as `deriveRows` draws it: `repoIcon` marks it. */
    const repoHeader = (id: string) =>
      branchRow(id, {
        parentId: null,
        workspaceId: 'home-1',
        repoIcon: {
          repoId: 'repo-1',
          projectId: 'proj-1',
          name: 'repo-1',
          avatarLabel: 'R',
          avatarColor: 'bg-indigo-700',
        },
      })

    beforeEach(() => {
      vi.mocked(getHomeWorkspaceId).mockReturnValue('home-ws-1')
      seedHomeTree('proj-1', {
        chats: [
          {
            id: 'home-owning-chat',
            repoId: '',
            ownsWorktree: true,
            workspaceId: 'home-ws-1',
            title: '',
            order: 0,
          },
          {
            id: 'home-chat-1',
            repoId: '',
            ownsWorktree: false,
            workspaceId: 'home-ws-1',
            title: 'a home chat',
            parentId: '',
            order: 1,
          },
        ],
        folders: [],
      })
    })

    it('reorders a home chat to land BEFORE the repo header row sharing its level', async () => {
      await performSidebarDrop(
        [chatRow('home-chat-1', 'home-ws-1')],
        repoHeader('home-1'),
        'before',
      )

      expect(setChatPlacement).toHaveBeenCalledWith('home-ws-1', 'home-chat-1', {
        parentId: '',
        order: 0,
      })
    })

    it('reorders a home chat to land AFTER the collapsed repo header row sharing its level', async () => {
      treeUiStore.setState({ collapsed: new Set(['home-1']) })
      await performSidebarDrop([chatRow('home-chat-1', 'home-ws-1')], repoHeader('home-1'), 'after')

      expect(setChatPlacement).toHaveBeenCalledWith('home-ws-1', 'home-chat-1', {
        parentId: '',
        order: 1,
      })
    })

    // A repo whose main checkout has been chatted in draws its header id'd by that chat, so the
    // sibling list holds the id the row actually carries.
    it('finds a repo header id’d by its owning chat among the home siblings', async () => {
      seedRepos(
        readSelect(selectRepos).map((r) =>
          r.id === 'repo-1' ? { ...r, defaultOwningChatId: 'chat-main-1' } : r,
        ),
      )

      await performSidebarDrop(
        [chatRow('home-chat-1', 'home-ws-1')],
        repoHeader('chat-main-1'),
        'before',
      )

      expect(setChatPlacement).toHaveBeenCalledWith('home-ws-1', 'home-chat-1', {
        parentId: '',
        order: 0,
      })
    })

    // Inside a repo header is its checkout's level: the home chat moves to run there.
    it('moves a home chat dropped INTO a repo header to the repo checkout', async () => {
      await performSidebarDrop([chatRow('home-chat-1', 'home-ws-1')], repoHeader('home-1'), 'into')

      expect(setChatPlacement).toHaveBeenCalledWith('home-ws-1', 'home-chat-1', {
        parentId: 'home-1',
        order: 5,
        workspaceId: 'home-1',
        label: 'repo-1',
      })
    })

    // Two threads under a locked branch, reordered while a DIFFERENT workspace's pane is active.
    it('reorders a thread past its sibling under an unmounted locked branch, indexed off the rendered rows', async () => {
      seedRepos([
        {
          ...makeRepo(),
          workspaces: [
            { id: 'ws-a', branch: 'a', age: '', order: 0, status: 'locked' },
            { id: 'ws-fork', branch: 'fork', age: '', order: 0, parentId: 'ws-a' },
          ],
          folders: [],
          chats: [
            repoChat('t1', { repoId: 'repo-1', workspaceId: 'ws-a', parentId: 'ws-a', order: 1 }),
            repoChat('t2', { repoId: 'repo-1', workspaceId: 'ws-a', parentId: 'ws-a', order: 2 }),
          ],
        },
      ])
      expect(getAllActiveWorkspaceIds()).not.toContain('ws-a')

      // ws-a's level: [ws-fork(0), t1(1), t2(2)] — t1 lifted, after t2 -> 2.
      await performSidebarDrop(
        [chatRow('t1', 'ws-a', { parentId: 'ws-a' })],
        chatRow('t2', 'ws-a', { parentId: 'ws-a' }),
        'after',
      )

      expect(setChatPlacement).toHaveBeenCalledWith('ws-a', 't1', { parentId: 'ws-a', order: 2 })
    })
  })
})

// A project-home FOLDER as the dragged SUBJECT: `planTreeRowDrop` is `Repo`-shaped and never
// sees one, so home folders have their own planner.
describe('performSidebarDrop — a project-home folder as the dragged subject', () => {
  // `planHomeFolderDrop` resolves the row owning `home-ws-1` off `ownsWorktree`, never
  // `type: 'branch'`.
  const HOME_OWNING_CHAT = {
    id: 'home-branch-row',
    repoId: '',
    ownsWorktree: true,
    workspaceId: 'home-ws-1',
    title: '',
    order: 0,
  }

  const homeFolderRow = (id: string, over: Partial<SidebarRow> = {}): SidebarRow => ({
    id,
    kind: 'folder',
    parentId: null,
    order: 0,
    label: id,
    ownsWorktree: false,
    workspaceId: null,
    working: false,
    hasView: false,
    ...over,
  })

  beforeEach(() => {
    vi.mocked(getHomeWorkspaceId).mockReturnValue('home-ws-1')
  })

  it('reorders a home folder among its siblings via placeHomeFolder', async () => {
    seedHomeTree('proj-1', {
      chats: [HOME_OWNING_CHAT],
      folders: [
        { id: 'home-folder-a', repoId: '', name: 'a', order: 0 },
        { id: 'home-folder-b', repoId: '', name: 'b', order: 1 },
      ],
    })

    await performSidebarDrop(
      [homeFolderRow('home-folder-b')],
      homeFolderRow('home-folder-a'),
      'before',
    )

    // A pure reorder sends no `parentId`: both folders share the root, and a recomputed one can
    // disagree with the backend's `MoveInput`.
    expect(placeHomeFolder).toHaveBeenCalledWith('proj-1', 'home-folder-b', {
      order: 0,
    })
    expect(placeFolder).not.toHaveBeenCalled()
  })

  it('files a home folder into another home folder', async () => {
    seedHomeTree('proj-1', {
      chats: [HOME_OWNING_CHAT],
      folders: [
        { id: 'home-folder-a', repoId: '', name: 'a', order: 0 },
        { id: 'home-folder-b', repoId: '', name: 'b', order: 1 },
      ],
    })

    await performSidebarDrop(
      [homeFolderRow('home-folder-b')],
      homeFolderRow('home-folder-a'),
      'into',
    )

    expect(placeHomeFolder).toHaveBeenCalledWith('proj-1', 'home-folder-b', {
      parentId: 'home-folder-a',
      order: 0,
    })
  })

  it('applies the response directly to useHomeTreeStore — no dedicated push channel to wait on', async () => {
    seedHomeTree('proj-1', {
      chats: [HOME_OWNING_CHAT],
      folders: [
        { id: 'home-folder-a', repoId: '', name: 'a', order: 0 },
        { id: 'home-folder-b', repoId: '', name: 'b', order: 1 },
      ],
    })

    await performSidebarDrop(
      [homeFolderRow('home-folder-a')],
      homeFolderRow('home-folder-b'),
      'into',
    )

    expect(
      readSelect((st) => selectHomeTree(st, 'proj-1')).folders.find((f) => f.id === 'home-folder-a')
        ?.parentId,
    ).toBe('home-folder-b')
  })

  // A repo header is a root-level sibling of a home folder, never its container: 'after' the
  // EXPANDED header is the slot after it among the root rows, not "first child".
  describe('relative to a repo header row', () => {
    const repoHeader = branchRow('home-1', {
      parentId: null,
      workspaceId: 'home-1',
      repoIcon: {
        repoId: 'repo-1',
        projectId: 'proj-1',
        name: 'repo-1',
        avatarLabel: 'R',
        avatarColor: 'bg-indigo-700',
      },
    })

    beforeEach(() => {
      // Root rows as drawn: repo-1's header (order 0), then home-folder-a.
      seedHomeTree('proj-1', {
        chats: [HOME_OWNING_CHAT],
        folders: [{ id: 'home-folder-a', repoId: '', name: 'a', order: 1 }],
      })
    })

    it('lands BEFORE the header at the root', async () => {
      await performSidebarDrop([homeFolderRow('home-folder-a')], repoHeader, 'before')

      // No `parentId` — the folder already sits at the bare root, same as the
      // header it is reordering past.
      expect(placeHomeFolder).toHaveBeenCalledWith('proj-1', 'home-folder-a', {
        order: 0,
      })
    })

    it('lands AFTER the expanded header at the root, never as its first child', async () => {
      await performSidebarDrop([homeFolderRow('home-folder-a')], repoHeader, 'after')

      expect(placeHomeFolder).toHaveBeenCalledWith('proj-1', 'home-folder-a', {
        order: 1,
      })
    })

    it('never files a folder INTO a repo header', async () => {
      await performSidebarDrop([homeFolderRow('home-folder-a')], repoHeader, 'into')

      expect(placeHomeFolder).not.toHaveBeenCalled()
    })
  })
})

// A repo header as the dragged subject: its one legal destination is project home
// (reordered among home rows or filed into a home folder), placed via `placeRepo`.
describe('performSidebarDrop — a repo header row as the dragged subject', () => {
  // Same reasoning as the home-folder-subject block above: `resolveHomeOwnerId`
  // resolves the row that owns the project's home workspace off
  // `ownsWorktree`, never `type: 'branch'`.
  const HOME_OWNING_CHAT = {
    id: 'home-branch-row',
    repoId: '',
    ownsWorktree: true,
    workspaceId: 'home-ws-1',
    title: '',
    order: 0,
  }

  const repoHeaderRow = (
    id: string,
    projectId: string,
    repoId: string,
    over: Partial<SidebarRow> = {},
  ): SidebarRow =>
    branchRow(id, {
      parentId: null,
      workspaceId: id,
      repoIcon: { repoId, projectId, name: repoId, avatarLabel: 'R', avatarColor: 'bg-indigo-700' },
      ...over,
    })

  beforeEach(() => {
    vi.mocked(getHomeWorkspaceId).mockReturnValue('home-ws-1')
  })

  it('files a repo into a project-home folder', async () => {
    seedHomeTree('proj-1', {
      chats: [HOME_OWNING_CHAT],
      folders: [{ id: 'home-folder-1', repoId: '', name: 'Projects', order: 0 }],
    })

    await performSidebarDrop(
      [repoHeaderRow('home-1', 'proj-1', 'repo-1')],
      folderRow('home-folder-1', { parentId: null, workspaceId: null }),
      'into',
    )

    expect(placeRepo).toHaveBeenCalledWith('proj-1', 'repo-1', {
      folderId: 'home-folder-1',
      order: 0,
    })
  })

  it('reorders relative to a home chat, in the same project', async () => {
    seedHomeTree('proj-1', {
      chats: [HOME_OWNING_CHAT, { id: 'home-chat-1', repoId: '', title: 'testing', order: 0 }],
      folders: [],
    })

    await performSidebarDrop(
      [repoHeaderRow('home-1', 'proj-1', 'repo-1')],
      chatRow('home-chat-1', '', { parentId: null }),
      'after',
    )

    expect(placeRepo).toHaveBeenCalledWith('proj-1', 'repo-1', { folderId: '', order: 1 })
  })

  it('reorders relative to another repo header in the same project', async () => {
    seedRepos([
      ...readSelect(selectRepos),
      {
        id: 'repo-2',
        projectId: 'proj-1',
        name: 'repo-2',
        avatarLabel: 'B',
        avatarColor: 'bg-indigo-700',
        defaultWorkspaceId: 'home-2',
        workspaces: [],
      },
    ])
    seedHomeTree('proj-1', { chats: [HOME_OWNING_CHAT], folders: [] })

    await performSidebarDrop(
      [repoHeaderRow('home-1', 'proj-1', 'repo-1')],
      repoHeaderRow('home-2', 'proj-1', 'repo-2'),
      'before',
    )

    expect(placeRepo).toHaveBeenCalledWith('proj-1', 'repo-1', { folderId: '', order: 0 })
  })

  it('a failed placement is reported, not thrown', async () => {
    vi.mocked(placeRepo).mockRejectedValueOnce(new ApiError('locked', 500))
    seedHomeTree('proj-1', {
      chats: [HOME_OWNING_CHAT],
      folders: [{ id: 'home-folder-1', repoId: '', name: 'Projects', order: 0 }],
    })

    await expect(
      performSidebarDrop(
        [repoHeaderRow('home-1', 'proj-1', 'repo-1')],
        folderRow('home-folder-1', { parentId: null, workspaceId: null }),
        'into',
      ),
    ).resolves.toBeUndefined()

    expectReported('locked')
  })

  // The policy refuses a repo beside a home folder nested in a CHAT; this pins
  // `planRepoHomeDrop`'s own backstop for the same case.
  it('refuses (no placeRepo call) reordering past a home folder nested inside a CHAT', async () => {
    seedHomeTree('proj-1', {
      chats: [HOME_OWNING_CHAT, { id: 'home-chat-1', repoId: '', title: 'testing', order: 0 }],
      folders: [
        { id: 'home-folder-1', repoId: '', name: 'Notes', parentId: 'home-chat-1', order: 0 },
      ],
    })

    await performSidebarDrop(
      [repoHeaderRow('home-1', 'proj-1', 'repo-1')],
      folderRow('home-folder-1', { parentId: 'home-chat-1', workspaceId: null }),
      'after',
    )

    expect(placeRepo).not.toHaveBeenCalled()
  })
})

// ── performSidebarPaneDrop — spec §8.1's other two targets ──
// Panes are window-level (`windowPaneStore`), not per-workspace stores.

describe('performSidebarPaneDrop — rows that name no chat', () => {
  it('ignores folder/workflow rows — a folder only folds, and nothing produces a workflow row yet', () => {
    const before = windowPaneStore.getState().panes[ROOT_PANE_ID]

    performSidebarPaneDrop(
      [
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

/** A WORKSPACE row dropped onto a pane: `deriveRows` ids a workspace-owning row by
 *  the chat that owns its worktree, so it opens and splits like a chat row. */
describe('performSidebarPaneDrop — a workspace row', () => {
  /** A row exactly as `deriveRows` builds one for a fork: id'd from the
   *  owning chat, carrying the workspace it owns. */
  const workspaceRow = (chatId: string, wsId: string) => branchRow(chatId, { workspaceId: wsId })

  beforeEach(() => {
    seedRepos([
      {
        ...makeRepo(),
        workspaces: [
          { id: 'ws-a', branch: 'a', age: '', order: 0, owningChatId: 'owner-a' },
          // No owner resolvable — its row keeps its own WORKSPACE id.
          { id: 'ws-b', branch: 'b', age: '', order: 1 },
        ],
      },
    ])
  })

  it('opens the chat that owns the workspace', () => {
    performSidebarPaneDrop([workspaceRow('owner-a', 'ws-a')], ROOT_PANE_ID, 'center')

    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBe('owner-a')
  })

  it('splits an occupied pane exactly as a chat row does, into ONE view', () => {
    windowPaneStore.getState().paneActions.openChat('c1', { runnerId: 'runner-1' })

    performSidebarPaneDrop([workspaceRow('owner-a', 'ws-a')], ROOT_PANE_ID, 'right')

    const opened = Object.values(windowPaneStore.getState().panes).find(
      (p) => p.chatId === 'owner-a',
    )
    expect(opened?.id).toBeDefined()
    expect(opened?.id).not.toBe(ROOT_PANE_ID)
    expect(getAllLeafIds(showingLayout(windowPaneStore.getState()))).toHaveLength(2)
    expect(liveViewOf('owner-a')).toBe(liveViewOf('c1'))
  })

  it('is a no-op when no owning chat can be resolved — that row id is a WORKSPACE id, not a chat', () => {
    performSidebarPaneDrop([workspaceRow('ws-b', 'ws-b')], ROOT_PANE_ID, 'center')

    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBeNull()
  })
})

/** A drop onto a pane's EDGE splits even when the chat already has a (parked)
 *  pane: that pane MOVES with its live state, never a second pane for one chat. */
describe('performSidebarPaneDrop — a chat that already has a view of its own', () => {
  /** The state a Recents row for a single-chat view describes: `chatId` is up
   *  in a view of its own, currently parked behind `showing`. */
  function parkChatInOwnView(chatId: string, showing: string) {
    openChatInOwnPane(chatRow(chatId, 'ws-1'))
    openChatInOwnPane(chatRow(showing, 'ws-1'))
  }

  it('splits the showing pane, rather than switching to the dragged chat’s parked view', () => {
    parkChatInOwnView('c1', 'c2')
    expect(getAllLeafIds(showingLayout(windowPaneStore.getState()))).toHaveLength(1)

    performSidebarPaneDrop(
      [chatRow('c1', 'ws-1')],
      windowPaneStore.getState().activePaneId,
      'right',
    )

    const leaves = getAllLeafIds(showingLayout(windowPaneStore.getState()))
    expect(leaves).toHaveLength(2)
    expect(leaves.map((id) => windowPaneStore.getState().panes[id]?.chatId).sort()).toEqual([
      'c1',
      'c2',
    ])
  })

  it('leaves both panes in ONE view, and nothing parked behind them', () => {
    parkChatInOwnView('c1', 'c2')

    performSidebarPaneDrop(
      [chatRow('c1', 'ws-1')],
      windowPaneStore.getState().activePaneId,
      'right',
    )

    expect(liveViewOf('c1')).toBe(liveViewOf('c2'))
    // c1's own view held nothing else, so it is gone rather than left empty.
    expect(liveRecents()).toEqual([['c1', 'c2']])
  })

  it('MOVES the pane it already had — same pane, same runner, never a second one', () => {
    parkChatInOwnView('c1', 'c2')
    const before = Object.values(windowPaneStore.getState().panes).find((p) => p.chatId === 'c1')!
    windowPaneStore.getState().paneActions.setPaneRunner(before.id, 'runner-1')

    performSidebarPaneDrop(
      [chatRow('c1', 'ws-1')],
      windowPaneStore.getState().activePaneId,
      'right',
    )

    const holding = Object.values(windowPaneStore.getState().panes).filter((p) => p.chatId === 'c1')
    expect(holding).toHaveLength(1)
    expect(holding[0].id).toBe(before.id)
    expect(holding[0].runnerId).toBe('runner-1')
  })

  it('gives every pane exactly one chat — a split is never a second chat in one pane', () => {
    parkChatInOwnView('c1', 'c2')

    performSidebarPaneDrop(
      [chatRow('c1', 'ws-1')],
      windowPaneStore.getState().activePaneId,
      'right',
    )

    for (const pane of Object.values(windowPaneStore.getState().panes)) {
      expect(typeof pane.chatId === 'string' || pane.chatId === null).toBe(true)
    }
    expect(
      Object.values(windowPaneStore.getState().panes).filter((p) => p.chatId !== null),
    ).toHaveLength(2)
  })

  it('honours the zone: a left drop puts the arriving chat FIRST', () => {
    parkChatInOwnView('c1', 'c2')

    performSidebarPaneDrop([chatRow('c1', 'ws-1')], windowPaneStore.getState().activePaneId, 'left')

    const leaves = getAllLeafIds(showingLayout(windowPaneStore.getState()))
    expect(windowPaneStore.getState().panes[leaves[0]]?.chatId).toBe('c1')
  })

  it('a cross-workspace row already up elsewhere merges the same way', () => {
    openChatInOwnPane(chatRow('c1', 'ws-offscreen'))
    openChatInOwnPane(chatRow('c2', 'ws-visible'))
    setActiveWorkspaceId('ws-visible')

    performSidebarPaneDrop(
      [chatRow('c1', 'ws-offscreen')],
      windowPaneStore.getState().activePaneId,
      'bottom',
    )

    expect(getAllLeafIds(showingLayout(windowPaneStore.getState()))).toHaveLength(2)
    expect(liveViewOf('c1')).toBe(liveViewOf('c2'))
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
    windowPaneStore.getState().paneActions.dropChatOnPane('c1', otherPane, 'center')
    windowPaneStore.getState().paneActions.setActivePane(ROOT_PANE_ID)

    performSidebarPaneDrop([chatRow('c1', 'ws-1')], ROOT_PANE_ID, 'center')

    // Never opened twice: ROOT_PANE_ID is untouched, and the reveal just
    // refocuses the pane that already has it.
    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBeNull()
    expect(windowPaneStore.getState().panes[otherPane]?.chatId).toBe('c1')
    expect(windowPaneStore.getState().activePaneId).toBe(otherPane)
  })

  it('dropping a chat onto the exact pane already showing it is a harmless no-op', () => {
    windowPaneStore.getState().paneActions.openChat('c1', { runnerId: 'runner-1' })

    performSidebarPaneDrop([chatRow('c1', 'ws-1')], ROOT_PANE_ID, 'right')

    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBe('c1')
    // One pane in one view — no split made.
    expect(Object.values(windowPaneStore.getState().panes).filter((p) => p.viewId)).toHaveLength(1)
  })
})

describe('performSidebarPaneDrop — merging (spec §8.1 "edge of a pane", §8.2)', () => {
  it('an edge drop onto an EMPTY pane still splits, with nothing to merge into', () => {
    performSidebarPaneDrop([chatRow('c1', 'ws-1')], ROOT_PANE_ID, 'right')

    const panes = windowPaneStore.getState().panes
    const newPane = Object.values(panes).find((p) => p.chatId === 'c1')
    expect(newPane).toBeDefined()
    expect(newPane?.id).not.toBe(ROOT_PANE_ID)
    expect(windowPaneStore.getState().viewOrder).toHaveLength(1)
  })

  it('a center drop onto an OCCUPIED pane never swaps — it merges instead (rule 1: every drop adds)', () => {
    windowPaneStore.getState().paneActions.openChat('c1', { runnerId: 'runner-1' })

    performSidebarPaneDrop([chatRow('c2', 'ws-1')], ROOT_PANE_ID, 'center')

    // c1 is still exactly where it was — nothing was evicted.
    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBe('c1')
    const newPane = Object.values(windowPaneStore.getState().panes).find((p) => p.chatId === 'c2')
    expect(newPane).toBeDefined()
  })

  it('an edge drop onto an occupied pane puts both chats in ONE view ("side by side")', () => {
    windowPaneStore.getState().paneActions.openChat('c1', { runnerId: 'runner-1' })

    performSidebarPaneDrop([chatRow('c2', 'ws-1')], ROOT_PANE_ID, 'right')

    expect(liveViewOf('c2')).toBe(liveViewOf('c1'))
    // And Recents draws the merged view as one row carrying both — the
    // grouping is read off the panes, never written to Recents separately.
    expect(liveRecents()).toEqual([['c1', 'c2']])
  })

  it('merging into a pane already part of a view GROWS that view rather than starting a second', () => {
    windowPaneStore.getState().paneActions.openChat('c1', { runnerId: 'runner-1' })
    performSidebarPaneDrop([chatRow('c2', 'ws-1')], ROOT_PANE_ID, 'right') // c1+c2 now one view

    performSidebarPaneDrop([chatRow('c3', 'ws-1')], ROOT_PANE_ID, 'bottom')

    expect(liveViewOf('c3')).toBe(liveViewOf('c1'))
    expect(liveRecents()).toEqual([['c1', 'c2', 'c3']])
  })

  it('dropping an already-grouped chat elsewhere reveals it in place — the view is untouched', () => {
    windowPaneStore.getState().paneActions.openChat('c1', { runnerId: 'runner-1' })
    performSidebarPaneDrop([chatRow('c2', 'ws-1')], ROOT_PANE_ID, 'right') // c1+c2 now one view
    const view = liveViewOf('c1')

    windowPaneStore.getState().paneActions.openChat('c9')
    const freshPane = windowPaneStore
      .getState()
      .paneActions.splitPane(paneOfChat('c9'), 'horizontal')!
    performSidebarPaneDrop([chatRow('c1', 'ws-1')], freshPane, 'center')

    // Revealed where it is: the chatless pane the drop was aimed at is not
    // filled with a second copy.
    expect(windowPaneStore.getState().panes[freshPane]?.chatId).toBeNull()
    expect(windowPaneStore.getState().activePaneId).toBe(ROOT_PANE_ID)
    expect(liveViewOf('c1')).toBe(view)
    expect(liveViewOf('c2')).toBe(view)
  })
})

/** `openChatInOwnPane` is the CLICK: a row opens into its own view. Merging two
 *  views is a drag-and-drop gesture only. */
describe('openChatInOwnPane — a click makes its own view (spec §8.4)', () => {
  beforeEach(() => setActiveWorkspaceId('ws-1'))

  it('fills the empty pane a fresh window starts with, rather than opening a second one beside it', () => {
    openChatInOwnPane(chatRow('c1', 'ws-1'))

    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBe('c1')
    expect(getAllLeafIds(showingLayout(windowPaneStore.getState()))).toEqual([ROOT_PANE_ID])
    expect(windowPaneStore.getState().activePaneId).toBe(ROOT_PANE_ID)
  })

  it('gives a second chat a pane of its OWN — the active pane keeps what it was showing', () => {
    openChatInOwnPane(chatRow('c1', 'ws-1'))

    openChatInOwnPane(chatRow('c2', 'ws-1'))

    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBe('c1')
    const opened = Object.values(windowPaneStore.getState().panes).find((p) => p.chatId === 'c2')
    expect(opened).toBeDefined()
    expect(windowPaneStore.getState().activePaneId).toBe(opened?.id)
  })

  it('mints a BRAND-NEW view id every time — no two clicks share one', () => {
    openChatInOwnPane(chatRow('c1', 'ws-1'))
    openChatInOwnPane(chatRow('c2', 'ws-1'))
    openChatInOwnPane(chatRow('c3', 'ws-1'))

    const views = ['c1', 'c2', 'c3'].map(liveViewOf)
    expect(views.every(Boolean)).toBe(true)
    expect(new Set(views).size).toBe(3)
  })

  it('NEVER merges clicked chats into one Recents row — that is the drop’s gesture, not the click’s', () => {
    openChatInOwnPane(chatRow('c1', 'ws-1'))
    openChatInOwnPane(chatRow('c2', 'ws-1'))
    openChatInOwnPane(chatRow('c3', 'ws-1'))

    // The identical sequence through the DROP path produces ONE row holding
    // all three (see "an edge drop onto an occupied pane puts both chats in
    // ONE view" above) — three clicks are three separate rows.
    expect(liveRecents()).toEqual([['c1'], ['c2'], ['c3']])
  })

  // The subtler half of "makes its own view": the pane a click REUSES can
  // already be one member of a view somebody merged earlier, and filling it
  // in place would silently have added this chat to that group — the same
  // "it appended to what I was looking at" complaint, one level down.
  it('never fills a chatless pane of an existing view — a click is a row of its own', () => {
    openChatInOwnPane(chatRow('c1', 'ws-1'))
    const merged = windowPaneStore.getState().paneActions.splitPane(ROOT_PANE_ID, 'horizontal')!
    expect(windowPaneStore.getState().panes[merged].viewId).toBe(liveViewOf('c1'))

    openChatInOwnPane(chatRow('c2', 'ws-1'))

    expect(windowPaneStore.getState().panes[merged]?.chatId).toBeNull()
    expect(liveViewOf('c2')).not.toBe(liveViewOf('c1'))
    expect(liveRecents()).toEqual([['c1'], ['c2']])
  })

  it('each clicked view REPLACES the one on screen — three clicks, one view showing', () => {
    openChatInOwnPane(chatRow('c1', 'ws-1'))
    openChatInOwnPane(chatRow('c2', 'ws-1'))
    openChatInOwnPane(chatRow('c3', 'ws-1'))

    // Each click is its own view: only the last is on screen, not three columns of one tiling tree.
    const showing = getAllLeafIds(showingLayout(windowPaneStore.getState()))
    expect(showing).toHaveLength(1)
    expect(windowPaneStore.getState().panes[showing[0]]?.chatId).toBe('c3')

    // Nothing was swapped out or lost: the other two are open, off screen,
    // each still holding exactly the chat it was opened with.
    const state = windowPaneStore.getState()
    const offScreen = state.viewOrder.filter((id) => id !== state.activeViewId)
    expect(offScreen).toHaveLength(2)
    expect(offScreen.flatMap((id) => viewChatIds(state, id)).sort()).toEqual(['c1', 'c2'])
    // And every one of them keeps its Recents row — a switcher needs targets.
    expect(liveRecents()).toEqual([['c1'], ['c2'], ['c3']])
  })

  it('clicking a chat that is already open SWITCHES to its view, never duplicates it', () => {
    openChatInOwnPane(chatRow('c1', 'ws-1'))
    const c1Pane = windowPaneStore.getState().activePaneId
    openChatInOwnPane(chatRow('c2', 'ws-1'))
    expect(getAllLeafIds(showingLayout(windowPaneStore.getState()))).not.toContain(c1Pane)

    openChatInOwnPane(chatRow('c1', 'ws-1'))

    expect(getAllLeafIds(showingLayout(windowPaneStore.getState()))).toEqual([c1Pane])
    expect(windowPaneStore.getState().activePaneId).toBe(c1Pane)
    expect(
      Object.values(windowPaneStore.getState().panes).filter((p) => p.chatId === 'c1'),
    ).toHaveLength(1)
  })

  it('a chat already up is gone TO, never opened twice', () => {
    openChatInOwnPane(chatRow('c1', 'ws-1'))
    openChatInOwnPane(chatRow('c2', 'ws-1'))
    const paneCount = getAllLeafIds(showingLayout(windowPaneStore.getState())).length

    openChatInOwnPane(chatRow('c1', 'ws-1'))

    expect(getAllLeafIds(showingLayout(windowPaneStore.getState()))).toHaveLength(paneCount)
    expect(windowPaneStore.getState().activePaneId).toBe(ROOT_PANE_ID)
    expect(
      Object.values(windowPaneStore.getState().panes).filter((p) => p.chatId === 'c1'),
    ).toHaveLength(1)
  })

  it('fills the empty stage in place: the first click promotes it into a row', () => {
    openChatInOwnPane(chatRow('c1', 'ws-1'))

    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBe('c1')
    expect(getAllLeafIds(showingLayout(windowPaneStore.getState()))).toEqual([ROOT_PANE_ID])
    expect(windowPaneStore.getState().viewOrder).toEqual([ROOT_PANE_ID])
  })

  it('opens a chat whose workspace is not the routed one — the resolver replaced that refusal', () => {
    setActiveWorkspaceId('ws-visible')

    openChatInOwnPane(chatRow('c1', 'ws-offscreen'))

    expect(Object.values(windowPaneStore.getState().panes).some((p) => p.chatId === 'c1')).toBe(
      true,
    )
  })

  it('is a no-op for a chat row naming no workspace at all', () => {
    expect(() => openChatInOwnPane(chatRow('c1', 'ws-1', { workspaceId: null }))).not.toThrow()
    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBeNull()
  })
})

/** CROSS-WORKSPACE drops: Recents spans a whole project, so most rows belong to a
 *  workspace other than the routed one; a pane resolves its chat's workspace
 *  through `resolveChatWorkspaceId`, so the drop is a drop. */
describe('performSidebarPaneDrop — cross-workspace', () => {
  it('splits for a chat whose workspace is not the routed one', () => {
    setActiveWorkspaceId('ws-visible') // ws-visible is what's on screen
    windowPaneStore.getState().paneActions.openChat('already-here', { runnerId: 'runner-1' })

    performSidebarPaneDrop([chatRow('c1', 'ws-offscreen')], ROOT_PANE_ID, 'right')

    const newPane = Object.values(windowPaneStore.getState().panes).find((p) => p.chatId === 'c1')
    expect(newPane).toBeDefined()
    expect(newPane?.id).not.toBe(ROOT_PANE_ID)
    // Nothing was evicted to make room, and the two are ONE view.
    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBe('already-here')
    expect(liveViewOf('c1')).toBe(liveViewOf('already-here'))
  })

  it('still works normally once the chat and the active workspace agree', () => {
    setActiveWorkspaceId('ws-visible')

    performSidebarPaneDrop([chatRow('c1', 'ws-visible')], ROOT_PANE_ID, 'center')

    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBe('c1')
  })
})

/** A branch row is addressed by its workspace's owning chat; everything planned
 *  here is in the workspace id space, so the id is translated first. */
// ── targetInRecents (Recents-row targets) ──

function makeChatRepo(): Repo {
  return {
    id: 'repo-2',
    projectId: 'proj-2',
    name: 'repo-2',
    avatarLabel: 'R',
    avatarColor: 'bg-indigo-700',
    defaultWorkspaceId: 'home-2',
    defaultBranch: 'main',
    workspaces: [{ id: 'ws-x', branch: 'x', age: '', order: 0 }],
  }
}

describe('performSidebarDrop — targetInRecents', () => {
  beforeEach(() => {
    seedRepos([...readSelect(selectRepos), makeChatRepo()])
  })

  it('middle of an already-LIVE Recents entry merges the dragged chat beside it, not a tree placement', async () => {
    setActiveWorkspaceId('ws-x')
    seedChats([chat('chat-a', 'ws-x'), chat('chat-b', 'ws-x')])
    windowPaneStore.getState().paneActions.openChat('chat-a', { runnerId: 'runner-1' })

    await performSidebarDrop([chatRow('chat-b', 'ws-x')], chatRow('chat-a', 'ws-x'), 'into', true)

    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBe('chat-a')
    const newPane = Object.values(windowPaneStore.getState().panes).find(
      (p) => p.chatId === 'chat-b',
    )
    expect(newPane).toBeDefined()
    // One view holding both — Recents draws them as a single grouped row.
    expect(liveViewOf('chat-b')).toBe(liveViewOf('chat-a'))
    expect(liveRecents()).toEqual([['chat-a', 'chat-b']])
    // A merge, never a tree/chat-tree placement write.
    expect(setChatPlacement).not.toHaveBeenCalled()
  })

  it('middle of a DORMANT Recents entry opens it first, then merges the dragged chat beside it', async () => {
    setActiveWorkspaceId('ws-x')
    seedChats([chat('chat-a', 'ws-x'), chat('chat-b', 'ws-x')])

    await performSidebarDrop([chatRow('chat-b', 'ws-x')], chatRow('chat-a', 'ws-x'), 'into', true)

    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBe('chat-a')
    const newPane = Object.values(windowPaneStore.getState().panes).find(
      (p) => p.chatId === 'chat-b',
    )
    expect(newPane).toBeDefined()
  })

  it('dropping a chat that is already up onto a Recents target just reveals it — never opened twice', async () => {
    setActiveWorkspaceId('ws-x')
    seedChats([chat('chat-a', 'ws-x'), chat('chat-b', 'ws-x')])
    windowPaneStore.getState().paneActions.openChat('chat-a', { runnerId: 'runner-1' })
    windowPaneStore.getState().paneActions.openChat('chat-b', { runnerId: 'runner-2' })
    const otherPane = paneOfChat('chat-b')

    await performSidebarDrop([chatRow('chat-b', 'ws-x')], chatRow('chat-a', 'ws-x'), 'into', true)

    // chat-b stayed exactly where it already was — no new pane, no swap.
    expect(windowPaneStore.getState().panes[otherPane]?.chatId).toBe('chat-b')
    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBe('chat-a')
    expect(Object.values(windowPaneStore.getState().panes).filter((p) => p.viewId)).toHaveLength(2)
  })

  it('above/below a Recents entry reorders the persisted Recents order instead of writing a tree placement', async () => {
    seedChats([chat('chat-a', 'ws-x'), chat('chat-b', 'ws-x')])
    windowPaneStore.getState().paneActions.openChat('chat-a', { runnerId: 'runner-1' })
    // `addPane`, not `splitPane` — two INDEPENDENT views, which is what two
    // Recents rows to reorder means. A split would merge them into one.
    windowPaneStore.getState().paneActions.openChat('chat-b', { runnerId: 'runner-2' })
    const otherPane = paneOfChat('chat-b')

    await performSidebarDrop([chatRow('chat-b', 'ws-x')], chatRow('chat-a', 'ws-x'), 'before', true)

    expect(setChatPlacement).not.toHaveBeenCalled()
    expect(windowPaneStore.getState().viewOrder).toEqual([otherPane, ROOT_PANE_ID].map(viewOfPane))
  })

  it('a drag carrying every chat of a split moves the split whole instead of breaking it up', async () => {
    seedChats([chat('chat-a', 'ws-x'), chat('chat-b', 'ws-x'), chat('chat-c', 'ws-x')])
    const { paneActions } = windowPaneStore.getState()
    paneActions.openChat('chat-a', { runnerId: 'runner-1' })
    paneActions.dropChatOnPane('chat-b', paneOfChat('chat-a'), 'right', 'ws-x')
    paneActions.openChat('chat-c', { runnerId: 'runner-3' })

    await performSidebarDrop(
      [chatRow('chat-a', 'ws-x'), chatRow('chat-b', 'ws-x')],
      chatRow('chat-c', 'ws-x'),
      'after',
      true,
    )

    expect(liveRecents()).toEqual([['chat-c'], ['chat-a', 'chat-b']])
  })

  it('a second reorder only moves the dragged entry, leaving every other tracked id in place', async () => {
    seedChats([chat('chat-a', 'ws-x'), chat('chat-b', 'ws-x'), chat('chat-c', 'ws-x')])
    windowPaneStore.getState().paneActions.openChat('chat-a', { runnerId: 'runner-1' })
    windowPaneStore.getState().paneActions.openChat('chat-b', { runnerId: 'runner-2' })
    const paneB = paneOfChat('chat-b')
    windowPaneStore.getState().paneActions.openChat('chat-c', { runnerId: 'runner-3' })
    const paneC = paneOfChat('chat-c')
    // Natural order: [ROOT(a), paneB(b), paneC(c)]. Move c before a.
    await performSidebarDrop([chatRow('chat-c', 'ws-x')], chatRow('chat-a', 'ws-x'), 'before', true)
    expect(windowPaneStore.getState().viewOrder).toEqual(
      [paneC, ROOT_PANE_ID, paneB].map(viewOfPane),
    )

    // Now move b to sit after c — a and c's relative order must not change.
    await performSidebarDrop([chatRow('chat-b', 'ws-x')], chatRow('chat-c', 'ws-x'), 'after', true)

    expect(windowPaneStore.getState().viewOrder).toEqual(
      [paneC, paneB, ROOT_PANE_ID].map(viewOfPane),
    )
  })

  // Recents draws workspace-owning chats with `kind: 'branch'`, so the band's rows
  // are not all `kind: 'chat'` and none may be filtered out by kind.
  describe('a Recents row whose chat owns a workspace (kind: branch)', () => {
    /** `ws-x` with its owning chat recorded, so a branch-kind row for
     *  `chat-b` resolves to that chat the same way the tree's own copy does. */
    function ownedByChatB(): void {
      seedRepos(
        readSelect(selectRepos).map((r) =>
          r.id === 'repo-2'
            ? {
                ...r,
                workspaces: [
                  { id: 'ws-x', branch: 'x', age: '', order: 0, owningChatId: 'chat-b' },
                ],
              }
            : r,
        ),
      )
    }

    /** Two independent views, `chat-a` in root and `chat-b` in its own —
     *  the two Recents rows a reorder needs. Returns the second pane's id. */
    function twoLiveViews(): string {
      seedChats([chat('chat-a', 'ws-x'), chat('chat-b', 'ws-x')])
      windowPaneStore.getState().paneActions.openChat('chat-a', { runnerId: 'runner-1' })
      windowPaneStore.getState().paneActions.openChat('chat-b', { runnerId: 'runner-2' })
      const otherPane = paneOfChat('chat-b')
      return otherPane
    }

    const ownerRow = chatRow('chat-b', 'ws-x', { kind: 'branch', ownsWorktree: true })

    it('moves to the slot when DRAGGED past a plain chat entry', async () => {
      ownedByChatB()
      const otherPane = twoLiveViews()

      await performSidebarDrop([ownerRow], chatRow('chat-a', 'ws-x'), 'before', true)

      expect(setChatPlacement).not.toHaveBeenCalled()
      expect(windowPaneStore.getState().viewOrder).toEqual(
        [otherPane, ROOT_PANE_ID].map(viewOfPane),
      )
    })

    it('takes a drop as the TARGET of a reorder', async () => {
      ownedByChatB()
      const otherPane = twoLiveViews()

      await performSidebarDrop([chatRow('chat-a', 'ws-x')], ownerRow, 'before', true)

      expect(setChatPlacement).not.toHaveBeenCalled()
      expect(windowPaneStore.getState().viewOrder).toEqual(
        [ROOT_PANE_ID, otherPane].map(viewOfPane),
      )
    })

    it('merges into the view when dropped on its middle', async () => {
      ownedByChatB()
      const otherPane = twoLiveViews()

      await performSidebarDrop([chatRow('chat-a', 'ws-x')], ownerRow, 'into', true)

      expect(liveViewOf('chat-a')).toBe(liveViewOf('chat-b'))
      expect(windowPaneStore.getState().panes[otherPane]?.chatId).toBe('chat-b')
    })
  })

  // A home chat's Recents entry carries the project-HOME workspace, which `resolveRowRepo`
  // (repo id spaces only) cannot resolve.
  it('reorders above/below a HOME chat’s Recents entry — its workspace is the project home, not a repo', async () => {
    vi.mocked(getHomeWorkspaceId).mockReturnValue('home-ws-2')
    seedHomeTree('proj-2', {
      chats: [{ id: 'home-chat', repoId: '', workspaceId: 'home-ws-2', title: 'h', order: 0 }],
      folders: [],
    })
    seedChats([chat('chat-a', 'ws-x')])
    seedChats([chat('home-chat', 'home-ws-2')])
    windowPaneStore.getState().paneActions.openChat('chat-a', { runnerId: 'runner-1' })
    windowPaneStore.getState().paneActions.openChat('home-chat', { runnerId: 'runner-2' })
    const homePane = paneOfChat('home-chat')

    await performSidebarDrop(
      [chatRow('chat-a', 'ws-x')],
      chatRow('home-chat', 'home-ws-2'),
      'after',
      true,
    )

    expect(setChatPlacement).not.toHaveBeenCalled()
    expect(windowPaneStore.getState().viewOrder).toEqual([homePane, ROOT_PANE_ID].map(viewOfPane))
  })

  // A home chat's Recents entry is stamped from the resolver, not the home tree, so its project
  // resolves the same way even before that project's home tree loads.
  it('reorders beside a HOME chat entry whose project has no home tree loaded', async () => {
    vi.mocked(getHomeWorkspaceId).mockImplementation((projectId: string) =>
      projectId === 'proj-2' ? 'home-ws-2' : null,
    )
    resetEntities()
    seedProjects([
      { id: 'proj-2', name: 'p2', path: '', lastActivity: new Date(), hostId: 'local' },
    ])
    seedChats([chat('chat-a', 'ws-x')])
    seedChats([chat('home-chat', 'home-ws-2')])
    windowPaneStore.getState().paneActions.openChat('chat-a', { runnerId: 'runner-1' })
    windowPaneStore.getState().paneActions.openChat('home-chat', { runnerId: 'runner-2' })
    const homePane = paneOfChat('home-chat')

    await performSidebarDrop(
      [chatRow('chat-a', 'ws-x')],
      chatRow('home-chat', 'home-ws-2'),
      'after',
      true,
    )

    expect(setChatPlacement).not.toHaveBeenCalled()
    expect(windowPaneStore.getState().viewOrder).toEqual([homePane, ROOT_PANE_ID].map(viewOfPane))
  })
})

describe('performSidebarDrop — a branch row is addressed by its owning chat', () => {
  /** The repo above plus its owning chats: one for the home workspace, one for the locked
   *  branch `ws-a` (never `type: 'branch'`). */
  function repoWithBranchRows(): Repo {
    const base = makeRepo()
    return {
      ...base,
      defaultOwningChatId: 'home-row',
      workspaces: base.workspaces.map((w) =>
        w.id === 'ws-a' ? { ...w, status: 'locked', owningChatId: 'ws-a-row' } : w,
      ),
      chats: [
        {
          id: 'home-row',
          repoId: 'repo-1',
          ownsWorktree: true,
          workspaceId: 'home-1',
          title: '',
          order: 0,
        },
        {
          id: 'ws-a-row',
          repoId: 'repo-1',
          ownsWorktree: true,
          workspaceId: 'ws-a',
          title: '',
          order: 0,
        },
      ],
    }
  }

  beforeEach(() => {
    seedRepos([repoWithBranchRows()])
  })

  it('dropping INTO the repo-home row places at the repo root, not silently nothing', async () => {
    await performSidebarDrop(
      [branchRow('ws-c')],
      branchRow('home-row', { workspaceId: 'home-1' }),
      'into',
    )

    expect(placeWorkspace).toHaveBeenCalledOnce()
    expect(placeWorkspace).toHaveBeenCalledWith('ws-c', {
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

    expect(reparentWorkspace).toHaveBeenCalledWith('ws-b', 'ws-a', expect.any(String))
  })

  it('a branch row as the SUBJECT is placed by its workspace id', async () => {
    await performSidebarDrop(
      [branchRow('ws-a-row', { workspaceId: 'ws-a' })],
      folderRow('folder-1', { parentId: 'home-row' }),
      'into',
    )

    expect(placeWorkspace).toHaveBeenCalledWith(
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
    expect(placeWorkspace).toHaveBeenCalledWith('ws-c', expect.objectContaining({ order: 0 }))
  })
})

/** Recents is the view switcher: its rows are drop targets for views that are
 *  usually off screen, merged into that view's own tree. */
describe('performSidebarDrop — dropping onto a Recents row (spec §8.1)', () => {
  beforeEach(() => setActiveWorkspaceId('ws-1'))

  it('merges into a view that is NOT on screen, and brings it over', () => {
    openChatInOwnPane(chatRow('c1', 'ws-1'))
    const c1Pane = windowPaneStore.getState().activePaneId
    openChatInOwnPane(chatRow('c2', 'ws-1'))
    // c1's view is parked; c2's is showing.
    expect(getAllLeafIds(showingLayout(windowPaneStore.getState()))).not.toContain(c1Pane)

    // `targetInRecents` is what `use-sidebar-drag.ts`'s hit test threads
    // through when the drop lands on a band row rather than a tree row.
    void performSidebarDrop([chatRow('c3', 'ws-1')], chatRow('c1', 'ws-1'), 'into', true)

    // It really landed in c1's own arrangement...
    expect(liveViewOf('c3')).toBe(liveViewOf('c1'))
    expect(liveRecents()).toEqual([['c1', 'c3'], ['c2']])
    // ...and the user is looking at the thing they just acted on, with both
    // chats genuinely on screen together.
    const showing = getAllLeafIds(showingLayout(windowPaneStore.getState()))
    expect(showing).toHaveLength(2)
    const showingChats = showing.map((id) => windowPaneStore.getState().panes[id]?.chatId)
    expect([...showingChats].sort()).toEqual(['c1', 'c3'])
  })

  it('merging into an off-screen view leaves every OTHER view alone', () => {
    openChatInOwnPane(chatRow('c1', 'ws-1'))
    openChatInOwnPane(chatRow('c2', 'ws-1'))
    const c2View = liveViewOf('c2')

    void performSidebarDrop([chatRow('c3', 'ws-1')], chatRow('c1', 'ws-1'), 'into', true)

    expect(liveViewOf('c2')).toBe(c2View)
    expect(liveViewOf('c2')).not.toBe(liveViewOf('c1'))
  })

  it('merges into the SHOWING view without disturbing anything either', () => {
    openChatInOwnPane(chatRow('c1', 'ws-1'))

    void performSidebarDrop([chatRow('c2', 'ws-1')], chatRow('c1', 'ws-1'), 'into', true)

    expect(liveViewOf('c2')).toBe(liveViewOf('c1'))
    expect(getAllLeafIds(showingLayout(windowPaneStore.getState()))).toHaveLength(2)
  })
})
