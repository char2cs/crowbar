import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

// The manager must pick the TauriWebSocket shim when running under Tauri
// (the crowbar:// unix-socket scheme has no native WebSocket) and the native
// WebSocket otherwise. Both satisfy the same onopen/onmessage/onclose/onerror/
// send/close surface, so the manager stays transport-agnostic.

const isTauri = vi.fn(() => false)
vi.mock('@/lib/crowbar-bridge', () => ({ isTauri }))

class MockNativeWebSocket {
  static CONNECTING = 0
  static OPEN = 1
  static CLOSING = 2
  static CLOSED = 3
  static instances: MockNativeWebSocket[] = []
  onmessage: ((e: MessageEvent) => void) | null = null
  onclose: (() => void) | null = null
  onopen: (() => void) | null = null
  onerror: (() => void) | null = null
  readyState = MockNativeWebSocket.OPEN
  send = vi.fn()
  close = vi.fn()
  addEventListener = vi.fn()
  constructor(public url: string) {
    MockNativeWebSocket.instances.push(this)
  }
}

const tauriInstances: MockTauriWebSocket[] = []
class MockTauriWebSocket {
  static CONNECTING = 0
  static OPEN = 1
  static CLOSED = 3
  onmessage: ((e: { data: string }) => void) | null = null
  onclose: (() => void) | null = null
  onopen: (() => void) | null = null
  onerror: ((e: unknown) => void) | null = null
  readyState = MockTauriWebSocket.OPEN
  send = vi.fn()
  close = vi.fn()
  constructor(
    public path: string,
    public options?: { hostId?: string },
  ) {
    tauriInstances.push(this)
  }
}

vi.mock('@/lib/ws/tauri-transport', () => ({ TauriWebSocket: MockTauriWebSocket }))

vi.stubGlobal('WebSocket', MockNativeWebSocket)

beforeEach(() => {
  MockNativeWebSocket.instances = []
  tauriInstances.length = 0
  isTauri.mockReturnValue(false)
})

const { createWSManager, wsManager } = await import('@/lib/ws/manager')
const { awaitEntity } = await import('@/lib/ws/await-entity')
const { useConnectionStore, resetConnectionState } = await import('@/lib/ws/connection-store')

describe('WSManager transport selection', () => {
  it('uses the native WebSocket in the browser', () => {
    isTauri.mockReturnValue(false)
    const mgr = createWSManager()
    mgr.subscribe('/v0/projects/p/repos/r/workspaces', vi.fn())
    expect(MockNativeWebSocket.instances).toHaveLength(1)
    expect(tauriInstances).toHaveLength(0)
  })

  it('uses the TauriWebSocket shim on desktop', () => {
    isTauri.mockReturnValue(true)
    const mgr = createWSManager()
    mgr.subscribe('/v0/projects/p/repos/r/workspaces', vi.fn())
    expect(tauriInstances).toHaveLength(1)
    expect(MockNativeWebSocket.instances).toHaveLength(0)
    expect(tauriInstances[0].path).toBe('/v0/projects/p/repos/r/workspaces')
  })

  it('delivers frames through the Tauri shim onmessage to subscribers', () => {
    isTauri.mockReturnValue(true)
    const mgr = createWSManager()
    const cb = vi.fn()
    mgr.subscribe('/v0/projects/p/repos/r/workspaces', cb)
    tauriInstances[0].onmessage?.({ data: '{"id":"ws-1"}' })
    expect(cb).toHaveBeenCalledWith({ id: 'ws-1' })
  })
})

describe('WSManager host routing', () => {
  it('dials a qualified endpoint on the desktop shim for that host, prefix stripped', () => {
    isTauri.mockReturnValue(true)
    createWSManager().subscribe('crowbar://h1/v0/projects', vi.fn())

    expect(tauriInstances).toHaveLength(1)
    expect(tauriInstances[0].path).toBe('/v0/projects')
    expect(tauriInstances[0].options).toEqual({ hostId: 'h1' })
  })

  it('dials an unqualified endpoint exactly as before: the path alone, no options', () => {
    isTauri.mockReturnValue(true)
    createWSManager().subscribe('/v0/projects', vi.fn())

    expect(tauriInstances[0].path).toBe('/v0/projects')
    expect(tauriInstances[0].options).toBeUndefined()
  })

  it('reads the desktop spelling of the local daemon as local', () => {
    isTauri.mockReturnValue(true)
    createWSManager().subscribe('crowbar://localhost/v0/projects', vi.fn())

    expect(tauriInstances[0].path).toBe('/v0/projects')
    expect(tauriInstances[0].options).toBeUndefined()
  })

  it('keeps one stream per host: the same path on two hosts shares nothing', () => {
    isTauri.mockReturnValue(true)
    const mgr = createWSManager()
    const local = vi.fn()
    const h1 = vi.fn()
    const h2 = vi.fn()
    mgr.subscribe('/v0/projects', local)
    mgr.subscribe('crowbar://h1/v0/projects', h1)
    mgr.subscribe('crowbar://h2/v0/projects', h2)
    mgr.subscribe('crowbar://h1/v0/projects', vi.fn())

    expect(tauriInstances.map((s) => s.options?.hostId)).toEqual([undefined, 'h1', 'h2'])

    tauriInstances[1].onmessage?.({ data: '{"id":"from-h1"}' })
    expect(h1).toHaveBeenCalledExactlyOnceWith({ id: 'from-h1' })
    expect(local).not.toHaveBeenCalled()
    expect(h2).not.toHaveBeenCalled()

    mgr.send('crowbar://h2/v0/projects', { ping: 1 })
    expect(tauriInstances[2].send).toHaveBeenCalledExactlyOnceWith('{"ping":1}')
    expect(tauriInstances[0].send).not.toHaveBeenCalled()
    expect(tauriInstances[1].send).not.toHaveBeenCalled()
  })

  it('cannot dial a remote host from the browser, and does not fall back to local', () => {
    isTauri.mockReturnValue(false)

    expect(() => createWSManager().subscribe('crowbar://h1/v0/projects', vi.fn())).toThrow(
      /desktop app/,
    )
    expect(MockNativeWebSocket.instances).toHaveLength(0)
  })

  it.each(['CROWBAR://h1/v0/projects', ' crowbar://h1/v0/projects', 'crowbar://h1//v0/projects'])(
    'refuses the malformed endpoint %j rather than dialling the local daemon',
    (endpoint) => {
      isTauri.mockReturnValue(true)

      expect(() => createWSManager().subscribe(endpoint, vi.fn())).toThrow()
      expect(tauriInstances).toHaveLength(0)
    },
  )
})

describe('WSManager remote stream failures', () => {
  const failureOn = (endpoint: string) => useConnectionStore.getState().failures[endpoint]

  beforeEach(() => {
    vi.useFakeTimers()
    isTauri.mockReturnValue(true)
    resetConnectionState()
  })
  afterEach(() => vi.useRealTimers())

  it('surfaces a changed host identity, not just that the stream closed', () => {
    createWSManager().subscribe('crowbar://h1/v0/projects', vi.fn())

    tauriInstances[0].onerror?.('host-identity-changed: the certificate no longer matches')
    tauriInstances[0].onclose?.()

    expect(failureOn('crowbar://h1/v0/projects')).toMatchObject({
      code: 'host_identity_changed',
      hostId: 'h1',
    })
  })

  it('surfaces an unreachable host as a different failure', () => {
    createWSManager().subscribe('crowbar://h1/v0/projects', vi.fn())

    tauriInstances[0].onerror?.('host-unreachable: cannot reach box:3737: connection refused')
    tauriInstances[0].onclose?.()

    expect(failureOn('crowbar://h1/v0/projects')).toMatchObject({
      code: 'host_unreachable',
      hostId: 'h1',
    })
  })

  it('records nothing for a failure that carries no marker', () => {
    createWSManager().subscribe('crowbar://h1/v0/projects', vi.fn())

    tauriInstances[0].onerror?.('ws upgrade: HTTP error: 401 Unauthorized')
    tauriInstances[0].onclose?.()

    expect(useConnectionStore.getState().failures).toEqual({})
  })

  it('keeps the failure through reconnect attempts and drops it once the stream opens', () => {
    createWSManager().subscribe('crowbar://h1/v0/projects', vi.fn())
    tauriInstances[0].onerror?.('host-unreachable: dial timed out')
    tauriInstances[0].onclose?.()

    vi.advanceTimersByTime(1000)
    expect(tauriInstances).toHaveLength(2)
    expect(failureOn('crowbar://h1/v0/projects')?.code).toBe('host_unreachable')

    tauriInstances[1].onopen?.()
    expect(useConnectionStore.getState().failures).toEqual({})
  })

  it('drops the failure when the last subscriber leaves', () => {
    const unsubscribe = createWSManager().subscribe('crowbar://h1/v0/projects', vi.fn())
    tauriInstances[0].onerror?.('host-identity-changed: x')
    tauriInstances[0].onclose?.()

    unsubscribe()

    expect(useConnectionStore.getState().failures).toEqual({})
  })

  it('keeps each host’s failure apart', () => {
    const mgr = createWSManager()
    mgr.subscribe('crowbar://h1/v0/projects', vi.fn())
    mgr.subscribe('crowbar://h2/v0/projects', vi.fn())

    tauriInstances[1].onerror?.('host-identity-changed: x')

    expect(failureOn('crowbar://h1/v0/projects')).toBeUndefined()
    expect(failureOn('crowbar://h2/v0/projects')?.hostId).toBe('h2')
  })

  it('never lets a remote outage look like a local one', () => {
    createWSManager().subscribe('crowbar://h1/v0/projects', vi.fn())

    tauriInstances[0].onopen?.()
    tauriInstances[0].onerror?.('host-identity-changed: certificate mismatch')
    tauriInstances[0].onclose?.()

    expect(useConnectionStore.getState().status).toBe('idle')
  })

  it('stops redialling a host whose identity changed, until someone subscribes again', () => {
    const mgr = createWSManager()
    const unsubscribe = mgr.subscribe('crowbar://h1/v0/projects', vi.fn())
    tauriInstances[0].onerror?.('host-identity-changed: certificate mismatch')
    tauriInstances[0].onclose?.()

    vi.advanceTimersByTime(120_000)
    expect(tauriInstances).toHaveLength(1)

    unsubscribe()
    mgr.subscribe('crowbar://h1/v0/projects', vi.fn())
    expect(tauriInstances).toHaveLength(2)
  })

  it('keeps redialling an unreachable host, backing off', () => {
    createWSManager().subscribe('crowbar://h1/v0/projects', vi.fn())

    for (const [attempt, wait] of [
      [1, 1000],
      [2, 2000],
      [3, 4000],
    ]) {
      tauriInstances[attempt - 1].onerror?.('host-unreachable: connection refused')
      tauriInstances[attempt - 1].onclose?.()
      vi.advanceTimersByTime(wait - 1)
      expect(tauriInstances).toHaveLength(attempt)
      vi.advanceTimersByTime(1)
      expect(tauriInstances).toHaveLength(attempt + 1)
    }
  })

  it('does not let a remote stream stand in for the local backend’s connection', () => {
    const mgr = createWSManager()
    mgr.subscribe('/v0/projects', vi.fn())
    mgr.subscribe('crowbar://h1/v0/projects', vi.fn())

    tauriInstances[1].onopen?.()
    expect(useConnectionStore.getState().status).toBe('idle')

    tauriInstances[0].onopen?.()
    expect(useConnectionStore.getState().status).toBe('connected')

    tauriInstances[0].onclose?.()
    expect(useConnectionStore.getState().status).toBe('disconnected')
  })
})

describe('awaitEntity on a qualified endpoint', () => {
  beforeEach(() => isTauri.mockReturnValue(true))

  // Creating a project on a remote host used to wait on the LOCAL stream and hang
  // until the timeout: its frame arrives on the remote host's own.
  it('resolves from the remote host’s stream, never the local one', async () => {
    const stopLocal = wsManager.subscribe('/v0/projects', vi.fn())
    const created = vi.fn()
    const pending = awaitEntity<{ id: string; path: string }>({
      endpoint: 'crowbar://h1/v0/projects',
      match: (frame) => frame.path === '/srv/app',
      action: async () => {},
    }).then(created)
    await Promise.resolve()
    await Promise.resolve()
    const [local, remote] = tauriInstances
    expect(remote.options).toEqual({ hostId: 'h1' })

    local.onmessage?.({ data: '{"id":"local-twin","path":"/srv/app"}' })
    await Promise.resolve()
    expect(created).not.toHaveBeenCalled()

    remote.onmessage?.({ data: '{"id":"remote-one","path":"/srv/app"}' })
    await pending
    expect(created).toHaveBeenCalledExactlyOnceWith({ id: 'remote-one', path: '/srv/app' })

    stopLocal()
  })
})
