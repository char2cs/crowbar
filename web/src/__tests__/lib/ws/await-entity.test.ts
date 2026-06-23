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
