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
vi.mock('@/lib/api', () => ({
  fetchHomeChats: (projectId: string) => fetchHomeChatsSpy(projectId) as unknown,
  fetchHomeFolders: (projectId: string) => fetchHomeFoldersSpy(projectId) as unknown,
}))

const { useHomeTreeStore, getHomeTree, subscribeHomeTree } = await import('@/lib/store/home-tree')

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
  useHomeTreeStore.setState({ trees: {} })
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
