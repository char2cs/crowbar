import { describe, it, expect, vi, beforeEach } from 'vitest'

// awaitEntity is built on wsManager.subscribe — mock it so the test drives WS
// frames directly without a real socket.
const subscribers = new Set<(data: unknown) => void>()
const unsubscribe = vi.fn()
const subscribe = vi.fn((_endpoint: string, cb: (data: unknown) => void) => {
  subscribers.add(cb)
  return () => {
    subscribers.delete(cb)
    unsubscribe()
  }
})

function emit(data: unknown): void {
  subscribers.forEach((cb) => cb(data))
}

// awaitEntity closes its snapshot window when the action() promise settles, on a
// microtask. Flushing the microtask queue lets a test deliver a "post-creation"
// frame that the primitive will treat as new rather than a snapshot replay.
async function flush(): Promise<void> {
  await Promise.resolve()
  await Promise.resolve()
}

vi.mock('@/lib/ws/manager', () => ({
  wsManager: {
    subscribe: (endpoint: string, cb: (data: unknown) => void) => subscribe(endpoint, cb),
    send: vi.fn(),
  },
}))

const { awaitEntity } = await import('@/lib/ws/await-entity')

beforeEach(() => {
  subscribers.clear()
  subscribe.mockClear()
  unsubscribe.mockClear()
})

describe('awaitEntity', () => {
  it('subscribes BEFORE firing the action (no frame dropped)', async () => {
    let subscribedWhenActionRan = false
    const action = vi.fn(async () => {
      // By the time the action (the 202 POST) runs, the WS subscription must
      // already be live so an immediate server push is never missed.
      subscribedWhenActionRan = subscribe.mock.calls.length > 0
    })

    const promise = awaitEntity<{ id: string; path: string }>({
      endpoint: '/v0/projects',
      match: (f) => f.path === '/tmp/p',
      action,
    })

    // subscription is registered synchronously, before the action runs
    expect(subscribe).toHaveBeenCalledOnce()

    await vi.waitFor(() => expect(action).toHaveBeenCalled())
    // The new entity is broadcast after the create round-trips (window closed).
    await flush()
    emit({ id: 'real-id', path: '/tmp/p' })

    const resolved = await promise
    expect(resolved).toEqual({ id: 'real-id', path: '/tmp/p' })
    expect(subscribedWhenActionRan).toBe(true)
  })

  it('acceptExisting resolves on a snapshot/pre-existing match (R4 regression)', async () => {
    // The awaited entity (a repo import's default workspace) was created by an
    // EARLIER action, so it arrives in the snapshot-on-subscribe burst — during
    // the window, before this action settles. With acceptExisting it must resolve
    // to that existing row, not bank it as pre-existing and time out.
    const promise = awaitEntity<{ id: string; repoId: string }>({
      endpoint: '/v0/projects/p/repos/r/workspaces',
      match: (f) => f.repoId === 'r',
      action: () => Promise.resolve(),
      acceptExisting: true,
      timeoutMs: 1000,
    })
    // Snapshot replay delivers the existing workspace immediately (window open).
    emit({ id: 'ws-existing', repoId: 'r' })
    const resolved = await promise
    expect(resolved).toEqual({ id: 'ws-existing', repoId: 'r' })
  })

  it('resolves with the first matching frame and ignores non-matches', async () => {
    const promise = awaitEntity<{ id: string; path: string }>({
      endpoint: '/v0/projects',
      match: (f) => f.path === '/tmp/want',
      action: async () => {},
    })
    // Wait for the snapshot window to close before delivering the created entity.
    await flush()
    emit({ id: 'other', path: '/tmp/nope' })
    emit({ id: 'mine', path: '/tmp/want' })
    await expect(promise).resolves.toEqual({ id: 'mine', path: '/tmp/want' })
    expect(unsubscribe).toHaveBeenCalledOnce()
  })

  it('resolves to the NEW entity, not a pre-existing snapshot match (H14)', async () => {
    // Repro of finding H14: a snapshot-on-subscribe frame for a PRE-EXISTING row
    // that already satisfies `match` (e.g. a workspace already on this branch)
    // arrives FIRST, before the freshly-created entity. awaitEntity must bank the
    // snapshot id and resolve only to the genuinely new entity.
    let actionFired = false
    const promise = awaitEntity<{ id: string; branch: string }>({
      endpoint: '/v0/projects/p1/repos/r1/workspaces',
      match: (w) => w.branch === 'feature',
      action: async () => {
        // The snapshot replay lands the instant we subscribe — before the POST
        // completes — so deliver the pre-existing match here.
        actionFired = true
        emit({ id: 'pre-existing', branch: 'feature' })
      },
    })

    await vi.waitFor(() => expect(actionFired).toBe(true))
    // Window closes once the action settles; now the created workspace arrives.
    await flush()
    emit({ id: 'freshly-created', branch: 'feature' })

    const resolved = await promise
    // It must be the new id, NOT the pre-existing snapshot row.
    expect(resolved.id).toBe('freshly-created')
    expect(resolved.id).not.toBe('pre-existing')
  })

  it('ignores the reconnect sentinel', async () => {
    const promise = awaitEntity<{ id: string; path: string }>({
      endpoint: '/v0/projects',
      match: (f) => f.path === '/tmp/want',
      action: async () => {},
    })
    await flush()
    emit({ reconnected: true })
    emit({ id: 'mine', path: '/tmp/want' })
    await expect(promise).resolves.toEqual({ id: 'mine', path: '/tmp/want' })
  })

  it('rejects (and unsubscribes) when the action rejects', async () => {
    const promise = awaitEntity<{ id: string }>({
      endpoint: '/v0/projects',
      match: () => true,
      action: async () => {
        throw new Error('disk full')
      },
    })
    await expect(promise).rejects.toThrow('disk full')
    expect(unsubscribe).toHaveBeenCalledOnce()
  })

  // A feed whose frames are lifecycle EVENTS rather than entity DTOs: the
  // worktree a repo import creates is announced on the chat socket, nested
  // inside a `worktree_state` event.
  describe('mapFrame', () => {
    const mapFrame = (raw: unknown): { id: string; repoId: string } | null => {
      const f = raw as { chatId?: string; kind?: string; worktree?: Record<string, string> }
      if (!f || f.kind !== 'worktree_state' || !f.worktree) return null
      if (f.worktree.owningChatId !== f.chatId) return null
      return { id: f.worktree.workspaceId, repoId: f.worktree.repoId }
    }

    const frame = (over: Record<string, unknown> = {}) => ({
      chatId: 'c1',
      kind: 'worktree_state',
      worktree: { owningChatId: 'c1', workspaceId: 'ws-1', repoId: 'r1' },
      ...over,
    })

    it('resolves with the MAPPED entity, and only after match accepts it', async () => {
      const promise = awaitEntity<{ id: string; repoId: string }>({
        endpoint: '/v0/projects/p1/repos/r1/chats/ws',
        mapFrame,
        match: (w) => w.repoId === 'r1',
        action: async () => {},
        acceptExisting: true,
      })
      await flush()

      // Every other kind on the socket maps to null and never reaches `match`.
      emit({ chatId: 'c1', kind: 'turn_started', working: true })
      emit({ reconnected: true })
      // A thread of the owning chat carries the same worktree; not its row.
      emit(frame({ chatId: 'c2' }))
      emit(frame())

      await expect(promise).resolves.toEqual({ id: 'ws-1', repoId: 'r1' })
    })

    it('still banks the snapshot burst by the MAPPED id when acceptExisting is off', async () => {
      let release: () => void = () => {}
      const promise = awaitEntity<{ id: string; repoId: string }>({
        endpoint: '/v0/projects/p1/repos/r1/chats/ws',
        mapFrame,
        match: (w) => w.repoId === 'r1',
        action: () => new Promise<void>((r) => (release = r)),
      })

      // Replayed on subscribe, while the action is still in flight: banked.
      emit(frame())
      release()
      await flush()
      // The same id again after the window closes is still the banked row.
      emit(frame())
      await flush()

      let settled = false
      void promise.then(() => (settled = true))
      await flush()
      expect(settled).toBe(false)

      emit(frame({ worktree: { owningChatId: 'c1', workspaceId: 'ws-2', repoId: 'r1' } }))
      await expect(promise).resolves.toEqual({ id: 'ws-2', repoId: 'r1' })
    })
  })

  it('rejects on timeout when no matching frame arrives', async () => {
    vi.useFakeTimers()
    const promise = awaitEntity<{ id: string }>({
      endpoint: '/v0/projects',
      match: () => false,
      action: async () => {},
      timeoutMs: 5000,
    })
    const assertion = expect(promise).rejects.toThrow(/timed out/)
    await vi.advanceTimersByTimeAsync(5000)
    await assertion
    vi.useRealTimers()
  })
})

// The REST seed. It exists because a feed with NO snapshot-on-subscribe cannot
// satisfy acceptExisting at all: the chat lifecycle socket replays nothing on
// connect, so an entity that already exists is never announced again.
describe('awaitEntity seed (streams with no snapshot-on-subscribe)', () => {
  // REGRESSION. Repo import awaits its default worktree on the chat feed, and
  // that worktree is created by the repo POST in the PRECEDING await — so by the
  // time this subscribes it already exists and no frame is ever coming. Without
  // the seed the import hung to the 30s timeout on the common path.
  it('resolves from the seed when no frame will ever arrive', async () => {
    const pending = awaitEntity<{ id: string; repoId: string }>({
      endpoint: '/v0/projects/p1/repos/r1/chats/ws',
      match: (w) => w.repoId === 'r1',
      action: () => Promise.resolve(),
      acceptExisting: true,
      seed: async () => [{ id: 'w1', repoId: 'r1' }],
    })

    await expect(pending).resolves.toEqual({ id: 'w1', repoId: 'r1' })
    expect(unsubscribe).toHaveBeenCalledTimes(1)
  })

  // The socket stays authoritative for the other ordering: the worktree is still
  // being provisioned when we subscribe, the seed comes back without it, and the
  // frame is what resolves.
  it('still resolves from a frame when the seed does not have it yet', async () => {
    const pending = awaitEntity<{ id: string; repoId: string }>({
      endpoint: '/v0/projects/p1/repos/r1/chats/ws',
      match: (w) => w.repoId === 'r1',
      action: () => Promise.resolve(),
      acceptExisting: true,
      seed: async () => [],
    })
    await flush()

    emit({ id: 'w2', repoId: 'r1' })

    await expect(pending).resolves.toEqual({ id: 'w2', repoId: 'r1' })
  })

  // A failed read is not a failed await — the frame may still be coming, and the
  // timeout is the real deadline. A transient 5xx must not turn a working live
  // path into a hard failure.
  it('survives a seed that rejects and still takes the frame', async () => {
    const pending = awaitEntity<{ id: string; repoId: string }>({
      endpoint: '/v0/projects/p1/repos/r1/chats/ws',
      match: (w) => w.repoId === 'r1',
      action: () => Promise.resolve(),
      acceptExisting: true,
      seed: async () => {
        throw new Error('read failed')
      },
    })
    await flush()

    emit({ id: 'w3', repoId: 'r1' })

    await expect(pending).resolves.toEqual({ id: 'w3', repoId: 'r1' })
  })
})
