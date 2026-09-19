import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { ChatDTO, FolderDTO } from '@/lib/types'

// Drive WS frames directly instead of standing up a socket (mirrors
// home-workspace.test.ts / entity-stream.test.ts).
const subscribers = new Set<(data: unknown) => void>()
const unsubscribeSpy = vi.fn()
const subscribeSpy = vi.fn((_endpoint: string, cb: (data: unknown) => void) => {
  subscribers.add(cb)
  return () => {
    subscribers.delete(cb)
    unsubscribeSpy()
  }
})

vi.mock('@/lib/ws/manager', () => ({
  wsManager: {
    subscribe: (endpoint: string, cb: (data: unknown) => void) => subscribeSpy(endpoint, cb),
    send: vi.fn(),
  },
}))

const fetchHomeChatsSpy = vi.fn()
const fetchHomeFoldersSpy = vi.fn()
const fetchReposSpy = vi.fn()
vi.mock('@/lib/api', () => ({
  fetchHomeChats: (projectId: string) => fetchHomeChatsSpy(projectId) as unknown,
  fetchHomeFolders: (projectId: string) => fetchHomeFoldersSpy(projectId) as unknown,
  fetchRepos: (projectId: string) => fetchReposSpy(projectId) as unknown,
  fetchHomeWorkspace: vi.fn().mockResolvedValue({ id: 'ws-home', owningChatId: 'c-home' }),
  assetURL: (path: string) => path,
}))

const { useHomeTreeStore, getHomeTree, subscribeHomeTree, applyHomeFolders, removeHomeFolder } =
  await import('@/lib/store/home-tree')
const { useSidebarStore, getInitialState } = await import('@/lib/store/sidebar')

function emit(data: unknown): void {
  subscribers.forEach((cb) => cb(data))
}

function chatDTO(over: Partial<ChatDTO> & { id: string; title: string }): ChatDTO {
  return { repoId: '', projectId: 'p1', workspaceId: '', ownsWorktree: false, order: 0, ...over }
}

function folderDTO(over: Partial<FolderDTO> & { id: string; name: string }): FolderDTO {
  return { repoId: '', projectId: 'p1', order: 0, ...over }
}

/** Resolve once the store observes the given chat count — a real signal. */
function whenChatsLength(projectId: string, expected: number): Promise<void> {
  return vi.waitFor(() => {
    expect(getHomeTree(projectId).chats).toHaveLength(expected)
  })
}

beforeEach(() => {
  subscribers.clear()
  subscribeSpy.mockClear()
  unsubscribeSpy.mockClear()
  fetchHomeChatsSpy.mockReset()
  fetchHomeFoldersSpy.mockReset()
  fetchReposSpy.mockReset()
  fetchReposSpy.mockResolvedValue([])
  useHomeTreeStore.setState({ trees: {} })
  useSidebarStore.setState(getInitialState())
})

describe('subscribeHomeTree', () => {
  it('seeds chats and folders from GET /home/chats + /home/chats/folders', async () => {
    fetchHomeChatsSpy.mockResolvedValue([chatDTO({ id: 'c1', title: 'Fix the thing' })])
    fetchHomeFoldersSpy.mockResolvedValue([folderDTO({ id: 'f1', name: 'Notes' })])

    const dispose = subscribeHomeTree('p1')

    await whenChatsLength('p1', 1)
    expect(fetchHomeChatsSpy).toHaveBeenCalledWith('p1')
    expect(fetchHomeFoldersSpy).toHaveBeenCalledWith('p1')
    expect(getHomeTree('p1').folders).toHaveLength(1)
    dispose()
  })

  // 2026-09-08 sidebar-placement-unification Task 5 moved a home folder's
  // identity onto domain.Folder and its position onto domain.Node, but kept
  // the wire field names (`parentId`/`order`) identical — this is the
  // regression that migration could have introduced: a folder's REAL
  // Node-sourced position surviving the fetch -> store round trip verbatim,
  // not silently reset to a default/index-derived value the way it would if
  // this store still assumed a folder's placement came bundled with a
  // Chat-typed row.
  it("carries a folder's real wire parentId/order through untouched", async () => {
    fetchHomeChatsSpy.mockResolvedValue([])
    fetchHomeFoldersSpy.mockResolvedValue([
      folderDTO({ id: 'f1', name: 'Notes', order: 5, parentId: 'f-parent' }),
    ])

    const dispose = subscribeHomeTree('p1')

    await vi.waitFor(() => {
      expect(getHomeTree('p1').folders).toHaveLength(1)
    })
    const folder = getHomeTree('p1').folders[0]
    expect(folder.order).toBe(5)
    expect(folder.parentId).toBe('f-parent')
    dispose()
  })

  it('listens on the project-scoped home agent-chat stream', () => {
    fetchHomeChatsSpy.mockResolvedValue([])
    fetchHomeFoldersSpy.mockResolvedValue([])

    const dispose = subscribeHomeTree('p1')

    expect(subscribeSpy).toHaveBeenCalledWith('/v0/projects/p1/home/chats/ws', expect.any(Function))
    dispose()
  })

  it('reseeds on a structural frame (created)', async () => {
    fetchHomeChatsSpy.mockResolvedValue([])
    fetchHomeFoldersSpy.mockResolvedValue([])
    const dispose = subscribeHomeTree('p1')
    await whenChatsLength('p1', 0)

    fetchHomeChatsSpy.mockResolvedValue([chatDTO({ id: 'c1', title: 'New' })])
    emit({ chatId: 'c1', workspaceId: 'home-1', kind: 'created' })

    await whenChatsLength('p1', 1)
    dispose()
  })

  // REGRESSION (K3): a home chat placement renumbers the REPO headers sharing
  // its level server-side, but the repos channel only announces what the
  // daemon decided to broadcast — so the client re-reads the project's repos
  // itself on a placement frame rather than trusting a single subject frame.
  it('re-reads the project repos on a placement frame, so shifted repo headers move too', async () => {
    fetchHomeChatsSpy.mockResolvedValue([])
    fetchHomeFoldersSpy.mockResolvedValue([])
    useSidebarStore.getState().setRepos([
      {
        id: 'r1',
        projectId: 'p1',
        name: 'repo-alpha',
        avatarLabel: 'R',
        avatarColor: 'bg-indigo-700',
        order: 0,
        workspaces: [{ id: 'ws-1', branch: 'main', age: '' }],
      },
    ])
    const dispose = subscribeHomeTree('p1')
    await whenChatsLength('p1', 0)
    expect(fetchReposSpy).not.toHaveBeenCalled()

    fetchReposSpy.mockResolvedValue([
      { id: 'r1', projectId: 'p1', name: 'repo-alpha', path: '/r1', order: 3, folderId: '' },
    ])
    emit({ chatId: 'c1', workspaceId: 'home-1', kind: 'placement_set' })

    await vi.waitFor(() => {
      expect(useSidebarStore.getState().repos.find((r) => r.id === 'r1')?.order).toBe(3)
    })
    expect(fetchReposSpy).toHaveBeenCalledWith('p1')
    // The merge keeps what the repos stream already populated.
    expect(useSidebarStore.getState().repos[0].workspaces).toHaveLength(1)
    dispose()
  })

  it('does not re-read repos on a frame that moves no row (a create, a turn)', async () => {
    fetchHomeChatsSpy.mockResolvedValue([])
    fetchHomeFoldersSpy.mockResolvedValue([])
    const dispose = subscribeHomeTree('p1')
    await whenChatsLength('p1', 0)

    emit({ chatId: 'c1', workspaceId: 'home-1', kind: 'turn_started' })
    emit({ chatId: 'c1', workspaceId: 'home-1', kind: 'created' })
    await vi.waitFor(() => expect(fetchHomeChatsSpy).toHaveBeenCalledTimes(2))
    expect(fetchReposSpy).not.toHaveBeenCalled()
    dispose()
  })

  it('reseeds on the reconnect sentinel', async () => {
    fetchHomeChatsSpy.mockResolvedValue([])
    fetchHomeFoldersSpy.mockResolvedValue([])
    const dispose = subscribeHomeTree('p1')
    await whenChatsLength('p1', 0)

    fetchHomeChatsSpy.mockResolvedValue([chatDTO({ id: 'c1', title: 'New' })])
    emit({ reconnected: true })

    await whenChatsLength('p1', 1)
    dispose()
  })

  it('ignores non-structural frames (turn_started, message_delta, worktree_state)', async () => {
    fetchHomeChatsSpy.mockResolvedValue([])
    fetchHomeFoldersSpy.mockResolvedValue([])
    const dispose = subscribeHomeTree('p1')
    await whenChatsLength('p1', 0)
    expect(fetchHomeChatsSpy).toHaveBeenCalledTimes(1)

    emit({ chatId: 'c1', workspaceId: 'home-1', kind: 'turn_started' })
    emit({ chatId: 'c1', workspaceId: 'home-1', kind: 'message_delta' })
    emit({ chatId: 'c1', workspaceId: 'home-1', kind: 'worktree_state' })

    expect(fetchHomeChatsSpy).toHaveBeenCalledTimes(1)
    dispose()
  })

  it('a failed reseed leaves the last known tree in place', async () => {
    fetchHomeChatsSpy.mockResolvedValue([chatDTO({ id: 'c1', title: 'Keep me' })])
    fetchHomeFoldersSpy.mockResolvedValue([])
    const dispose = subscribeHomeTree('p1')
    await whenChatsLength('p1', 1)

    fetchHomeChatsSpy.mockRejectedValue(new Error('offline'))
    emit({ chatId: 'c2', workspaceId: 'home-1', kind: 'created' })

    await vi.waitFor(() => {
      expect(fetchHomeChatsSpy).toHaveBeenCalledTimes(2)
    })
    expect(getHomeTree('p1').chats).toHaveLength(1)
    dispose()
  })

  it('teardown unsubscribes and clears the project tree', async () => {
    fetchHomeChatsSpy.mockResolvedValue([chatDTO({ id: 'c1', title: 'Gone soon' })])
    fetchHomeFoldersSpy.mockResolvedValue([])
    const dispose = subscribeHomeTree('p1')
    await whenChatsLength('p1', 1)

    dispose()

    expect(unsubscribeSpy).toHaveBeenCalledTimes(1)
    expect(getHomeTree('p1')).toEqual({ chats: [], folders: [] })
  })

  it('a different project is unaffected by this one tearing down', async () => {
    fetchHomeChatsSpy.mockImplementation((projectId: string) =>
      Promise.resolve([chatDTO({ id: `c-${projectId}`, title: projectId })]),
    )
    fetchHomeFoldersSpy.mockResolvedValue([])
    const disposeP1 = subscribeHomeTree('p1')
    const disposeP2 = subscribeHomeTree('p2')
    await whenChatsLength('p1', 1)
    await whenChatsLength('p2', 1)

    disposeP1()

    expect(getHomeTree('p1')).toEqual({ chats: [], folders: [] })
    expect(getHomeTree('p2').chats).toHaveLength(1)
    disposeP2()
  })

  it('a read that resolves after teardown does not resurrect the tree', async () => {
    let resolveChats: (chats: ChatDTO[]) => void = () => {}
    fetchHomeChatsSpy.mockReturnValue(
      new Promise<ChatDTO[]>((resolve) => {
        resolveChats = resolve
      }),
    )
    fetchHomeFoldersSpy.mockResolvedValue([])

    const dispose = subscribeHomeTree('p1')
    dispose()
    resolveChats([chatDTO({ id: 'c1', title: 'Too late' })])

    await vi.waitFor(() => {
      expect(fetchHomeChatsSpy).toHaveBeenCalledTimes(1)
    })
    expect(getHomeTree('p1')).toEqual({ chats: [], folders: [] })
  })
})

describe('applyHomeFolders', () => {
  it('seeds a fresh project with the given folders, real order/parentId intact', () => {
    applyHomeFolders('p1', [
      { id: 'f1', repoId: '', name: 'Notes', order: 3, parentId: 'f-parent' },
    ])

    expect(getHomeTree('p1').folders).toEqual([
      { id: 'f1', repoId: '', name: 'Notes', order: 3, parentId: 'f-parent' },
    ])
  })

  it('upserts an existing folder by id rather than duplicating it', () => {
    applyHomeFolders('p1', [{ id: 'f1', repoId: '', name: 'Notes', order: 0 }])
    applyHomeFolders('p1', [{ id: 'f1', repoId: '', name: 'Renamed', order: 2 }])

    const folders = getHomeTree('p1').folders
    expect(folders).toHaveLength(1)
    expect(folders[0]).toEqual({ id: 'f1', repoId: '', name: 'Renamed', order: 2 })
  })

  it('leaves an unrelated sibling folder untouched', () => {
    applyHomeFolders('p1', [
      { id: 'f1', repoId: '', name: 'Notes', order: 0 },
      { id: 'f2', repoId: '', name: 'Other', order: 1 },
    ])
    applyHomeFolders('p1', [{ id: 'f1', repoId: '', name: 'Renamed', order: 0 }])

    const byId = new Map(getHomeTree('p1').folders.map((f) => [f.id, f]))
    expect(byId.get('f1')?.name).toBe('Renamed')
    expect(byId.get('f2')?.name).toBe('Other')
  })

  it("does not disturb the project's chats", async () => {
    fetchHomeChatsSpy.mockResolvedValue([chatDTO({ id: 'c1', title: 'Existing' })])
    fetchHomeFoldersSpy.mockResolvedValue([])
    const dispose = subscribeHomeTree('p1')
    await whenChatsLength('p1', 1)

    applyHomeFolders('p1', [{ id: 'f1', repoId: '', name: 'Notes', order: 0 }])

    expect(getHomeTree('p1').chats).toHaveLength(1)
    expect(getHomeTree('p1').folders).toHaveLength(1)
    dispose()
  })
})

// Regression: `applyHomeFolders` only ever upserts (a plain `Folder` carries
// no `status` field to branch a delete on, unlike the wire `FolderDTO`
// `useSidebarStore.applyFolderDTO` reads) — a deleted home folder needs its
// own removal, or a removal-tray commit has nothing that ever takes it back
// out of the tree.
describe('removeHomeFolder', () => {
  it('removes the folder by id, leaving an unrelated sibling untouched', () => {
    applyHomeFolders('p1', [
      { id: 'f1', repoId: '', name: 'Notes', order: 0 },
      { id: 'f2', repoId: '', name: 'Other', order: 1 },
    ])

    removeHomeFolder('p1', 'f1')

    expect(getHomeTree('p1').folders).toEqual([{ id: 'f2', repoId: '', name: 'Other', order: 1 }])
  })

  it('is a no-op for a folder id this project’s tree does not hold', () => {
    applyHomeFolders('p1', [{ id: 'f1', repoId: '', name: 'Notes', order: 0 }])

    removeHomeFolder('p1', 'no-such-id')

    expect(getHomeTree('p1').folders).toHaveLength(1)
  })

  it('does not disturb the project’s chats', async () => {
    fetchHomeChatsSpy.mockResolvedValue([chatDTO({ id: 'c1', title: 'Existing' })])
    fetchHomeFoldersSpy.mockResolvedValue([])
    const dispose = subscribeHomeTree('p1')
    await whenChatsLength('p1', 1)
    applyHomeFolders('p1', [{ id: 'f1', repoId: '', name: 'Notes', order: 0 }])

    removeHomeFolder('p1', 'f1')

    expect(getHomeTree('p1').chats).toHaveLength(1)
    expect(getHomeTree('p1').folders).toEqual([])
    dispose()
  })
})
