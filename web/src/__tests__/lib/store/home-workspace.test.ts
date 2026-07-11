import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { WorkspaceDTO } from '@/lib/types'

// Drive WS frames directly instead of standing up a socket (mirrors
// entity-stream.test.ts).
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

const fetchHomeWorkspaceSpy = vi.fn()
vi.mock('@/lib/api', () => ({
  fetchHomeWorkspace: (projectId: string) => fetchHomeWorkspaceSpy(projectId) as unknown,
}))

const { useHomeWorkspaceStore, subscribeHomeWorkspace } = await import('@/lib/store/home-workspace')

function emit(data: unknown): void {
  subscribers.forEach((cb) => cb(data))
}

function homeDTO(working: boolean): WorkspaceDTO {
  return {
    id: 'home-1',
    // A home workspace rides no repo — this empty repoId is exactly why the
    // per-repo workspaces stream can never deliver its frames.
    repoId: '',
    projectId: 'p1',
    branch: 'home',
    status: 'new',
    working,
  } as WorkspaceDTO
}

/** Resolve once the store observes `working` — a real signal, never a sleep. */
function whenWorking(expected: boolean): Promise<void> {
  return vi.waitFor(() => {
    expect(useHomeWorkspaceStore.getState().workspace?.working).toBe(expected)
  })
}

beforeEach(() => {
  subscribers.clear()
  subscribeSpy.mockClear()
  unsubscribeSpy.mockClear()
  fetchHomeWorkspaceSpy.mockReset()
  useHomeWorkspaceStore.setState({ workspace: null })
})

describe('subscribeHomeWorkspace', () => {
  it('seeds the home workspace from GET /home', async () => {
    fetchHomeWorkspaceSpy.mockResolvedValue(homeDTO(false))

    const dispose = subscribeHomeWorkspace('p1')

    await whenWorking(false)
    expect(fetchHomeWorkspaceSpy).toHaveBeenCalledWith('p1')
    expect(useHomeWorkspaceStore.getState().workspace?.id).toBe('home-1')
    dispose()
  })

  it('listens on the project-scoped home agent-chat stream', () => {
    fetchHomeWorkspaceSpy.mockResolvedValue(homeDTO(false))

    const dispose = subscribeHomeWorkspace('p1')

    expect(subscribeSpy).toHaveBeenCalledWith(
      '/v0/projects/p1/home/agent/ws/chats',
      expect.any(Function),
    )
    dispose()
  })

  it('re-reads on turn_started so the home workspace starts working', async () => {
    fetchHomeWorkspaceSpy.mockResolvedValue(homeDTO(false))
    const dispose = subscribeHomeWorkspace('p1')
    await whenWorking(false)

    // The daemon flips the overlay; the frame is only the signal to re-read it.
    fetchHomeWorkspaceSpy.mockResolvedValue(homeDTO(true))
    emit({ chatId: 'c1', workspaceId: 'home-1', kind: 'turn_started' })

    await whenWorking(true)
    dispose()
  })

  it('re-reads on turn_stopped so the spinner clears', async () => {
    fetchHomeWorkspaceSpy.mockResolvedValue(homeDTO(true))
    const dispose = subscribeHomeWorkspace('p1')
    await whenWorking(true)

    fetchHomeWorkspaceSpy.mockResolvedValue(homeDTO(false))
    emit({ chatId: 'c1', workspaceId: 'home-1', kind: 'turn_stopped' })

    await whenWorking(false)
    dispose()
  })

  it('re-reads when a chat is deleted mid-turn (never wedges the spinner on)', async () => {
    fetchHomeWorkspaceSpy.mockResolvedValue(homeDTO(true))
    const dispose = subscribeHomeWorkspace('p1')
    await whenWorking(true)

    fetchHomeWorkspaceSpy.mockResolvedValue(homeDTO(false))
    emit({ chatId: 'c1', workspaceId: 'home-1', kind: 'deleted' })

    await whenWorking(false)
    dispose()
  })

  it('re-reads on the reconnect sentinel (turns may have flipped during the outage)', async () => {
    fetchHomeWorkspaceSpy.mockResolvedValue(homeDTO(false))
    const dispose = subscribeHomeWorkspace('p1')
    await whenWorking(false)

    fetchHomeWorkspaceSpy.mockResolvedValue(homeDTO(true))
    emit({ reconnected: true })

    await whenWorking(true)
    dispose()
  })

  it('ignores lifecycle frames that cannot move the working overlay', async () => {
    fetchHomeWorkspaceSpy.mockResolvedValue(homeDTO(false))
    const dispose = subscribeHomeWorkspace('p1')
    await whenWorking(false)
    expect(fetchHomeWorkspaceSpy).toHaveBeenCalledTimes(1)

    // A rename/segment frame changes nothing about `working` — re-reading would
    // be a pointless round trip on every title the agent sets.
    emit({ chatId: 'c1', workspaceId: 'home-1', kind: 'title_set' })
    emit({ chatId: 'c1', workspaceId: 'home-1', kind: 'segment_opened' })

    expect(fetchHomeWorkspaceSpy).toHaveBeenCalledTimes(1)
    dispose()
  })

  it('a failed read leaves the last known value in place', async () => {
    fetchHomeWorkspaceSpy.mockResolvedValue(homeDTO(true))
    const dispose = subscribeHomeWorkspace('p1')
    await whenWorking(true)

    fetchHomeWorkspaceSpy.mockRejectedValue(new Error('offline'))
    emit({ chatId: 'c1', workspaceId: 'home-1', kind: 'turn_stopped' })

    await vi.waitFor(() => {
      expect(fetchHomeWorkspaceSpy).toHaveBeenCalledTimes(2)
    })
    expect(useHomeWorkspaceStore.getState().workspace?.working).toBe(true)
    dispose()
  })

  it('teardown unsubscribes and drops the previous project home', async () => {
    fetchHomeWorkspaceSpy.mockResolvedValue(homeDTO(true))
    const dispose = subscribeHomeWorkspace('p1')
    await whenWorking(true)

    dispose()

    expect(unsubscribeSpy).toHaveBeenCalledTimes(1)
    // A stale spinner (or name) from the old project must not survive a switch.
    expect(useHomeWorkspaceStore.getState().workspace).toBeNull()
  })

  it('a read that resolves after teardown does not resurrect the old project', async () => {
    let resolveRead: (ws: WorkspaceDTO) => void = () => {}
    fetchHomeWorkspaceSpy.mockReturnValue(
      new Promise<WorkspaceDTO>((resolve) => {
        resolveRead = resolve
      }),
    )

    const dispose = subscribeHomeWorkspace('p1')
    dispose()
    resolveRead(homeDTO(true))

    await vi.waitFor(() => {
      expect(fetchHomeWorkspaceSpy).toHaveBeenCalledTimes(1)
    })
    expect(useHomeWorkspaceStore.getState().workspace).toBeNull()
  })
})
