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
    emit({ id: 'real-id', path: '/tmp/p' })

    const resolved = await promise
    expect(resolved).toEqual({ id: 'real-id', path: '/tmp/p' })
    expect(subscribedWhenActionRan).toBe(true)
  })

  it('resolves with the first matching frame and ignores non-matches', async () => {
    const promise = awaitEntity<{ id: string; path: string }>({
      endpoint: '/v0/projects',
      match: (f) => f.path === '/tmp/want',
      action: async () => {},
    })
    emit({ id: 'other', path: '/tmp/nope' })
    emit({ id: 'mine', path: '/tmp/want' })
    await expect(promise).resolves.toEqual({ id: 'mine', path: '/tmp/want' })
    expect(unsubscribe).toHaveBeenCalledOnce()
  })

  it('ignores the reconnect sentinel', async () => {
    const promise = awaitEntity<{ id: string; path: string }>({
      endpoint: '/v0/projects',
      match: (f) => f.path === '/tmp/want',
      action: async () => {},
    })
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
