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
  constructor(
    public url: string,
    public protocols?: string[],
  ) {
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

const { createWSManager } = await import('@/lib/ws/create-ws-manager')
const { hostPorts } = await import('@/features/hosts/lib/ws-host-ports')
const { useConnectionStore } = await import('@/lib/ws/connection-store')
const { destroyHostRuntime, getOrCreateHostRuntime } =
  await import('@/features/hosts/stores/host-runtime-registry')
const { pairRemoteHost, resetHostWorld } = await import('@/__tests__/__fixtures__/remote-host')

// Only a paired host is dialled: the remote hosts these tests reach are paired ones.
beforeEach(() => {
  pairRemoteHost('h1')
  pairRemoteHost('h2')
})
afterEach(() => resetHostWorld())

describe('WSManager transport selection', () => {
  it('uses the native WebSocket in the browser', () => {
    isTauri.mockReturnValue(false)
    const mgr = createWSManager(hostPorts)
    mgr.subscribe('/v0/projects/p/repos/r/workspaces', vi.fn())
    expect(MockNativeWebSocket.instances).toHaveLength(1)
    expect(tauriInstances).toHaveLength(0)
  })

  it('uses the TauriWebSocket shim on desktop', () => {
    isTauri.mockReturnValue(true)
    const mgr = createWSManager(hostPorts)
    mgr.subscribe('/v0/projects/p/repos/r/workspaces', vi.fn())
    expect(tauriInstances).toHaveLength(1)
    expect(MockNativeWebSocket.instances).toHaveLength(0)
    expect(tauriInstances[0].path).toBe('/v0/projects/p/repos/r/workspaces')
  })

  it('delivers frames through the Tauri shim onmessage to subscribers', () => {
    isTauri.mockReturnValue(true)
    const mgr = createWSManager(hostPorts)
    const cb = vi.fn()
    mgr.subscribe('/v0/projects/p/repos/r/workspaces', cb)
    tauriInstances[0].onmessage?.({ data: '{"id":"ws-1"}' })
    expect(cb).toHaveBeenCalledWith({ id: 'ws-1' })
  })
})

describe('WSManager in the GPUI shell', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.stubGlobal('WebSocket', MockNativeWebSocket)
    delete (window as unknown as Record<string, unknown>).crowbar
    delete (window as unknown as Record<string, unknown>).__CROWBAR__
  })

  it('dials the shell relay with the pane token as a subprotocol', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        throw new TypeError('lookup unreachable')
      }),
    )
    const win = window as unknown as Record<string, unknown>
    win.crowbar = { post: vi.fn(), onmessage: null }
    win.__CROWBAR__ = {
      api: 'crowbar://local',
      host: 'local',
      ws: { url: 'ws://127.0.0.1:4100', token: 'abc' },
    }
    createWSManager(hostPorts).subscribe('/v0/stream', vi.fn())
    await vi.waitFor(() => expect(MockNativeWebSocket.instances).toHaveLength(1))
    expect(MockNativeWebSocket.instances[0]).toMatchObject({
      url: 'ws://127.0.0.1:4100/v0/stream',
      protocols: ['crowbar.relay.v1', 'crowbar.token.abc'],
    })
    expect(tauriInstances).toHaveLength(0)
  })
})

describe('WSManager host routing', () => {
  it('dials a qualified endpoint on the desktop shim for that host, prefix stripped', () => {
    isTauri.mockReturnValue(true)
    createWSManager(hostPorts).subscribe('crowbar://h1/v0/projects', vi.fn())

    expect(tauriInstances).toHaveLength(1)
    expect(tauriInstances[0].path).toBe('/v0/projects')
    expect(tauriInstances[0].options).toEqual({ hostId: 'h1', idleTimeoutMs: 60_000 })
  })

  // The daemon pings every stream each 30 s, but a network change or sleep can leave a remote
  // socket half-open: silent, never closed. The local daemon's unix socket has no such hop.
  it('has a remote stream silent for two pings closed and reopened, its subscribers caught up', () => {
    vi.useFakeTimers()
    isTauri.mockReturnValue(true)
    const mgr = createWSManager(hostPorts)
    const remote = vi.fn()
    const local = vi.fn()
    mgr.subscribe('crowbar://h1/v0/projects/p/repos', remote)
    mgr.subscribe('/v0/projects', local)
    expect(tauriInstances.map((s) => s.options)).toEqual([
      { hostId: 'h1', idleTimeoutMs: 60_000 },
      undefined,
    ])
    tauriInstances.forEach((s) => s.onopen?.())

    // The bridge's read-idle timeout: the shim reports it as the daemon closing the stream.
    tauriInstances[0].onclose?.()
    vi.advanceTimersByTime(1_000)
    tauriInstances[2].onopen?.()

    expect(tauriInstances[2].options).toEqual({ hostId: 'h1', idleTimeoutMs: 60_000 })
    expect(remote).toHaveBeenCalledWith({ reconnected: true })
    expect(local).not.toHaveBeenCalled()
    vi.useRealTimers()
  })

  it('dials an unqualified endpoint exactly as before: the path alone, no options', () => {
    isTauri.mockReturnValue(true)
    createWSManager(hostPorts).subscribe('/v0/projects', vi.fn())

    expect(tauriInstances[0].path).toBe('/v0/projects')
    expect(tauriInstances[0].options).toBeUndefined()
  })

  it('reads the desktop spelling of the local daemon as local', () => {
    isTauri.mockReturnValue(true)
    createWSManager(hostPorts).subscribe('crowbar://localhost/v0/projects', vi.fn())

    expect(tauriInstances[0].path).toBe('/v0/projects')
    expect(tauriInstances[0].options).toBeUndefined()
  })

  it('keeps one stream per host: the same path on two hosts shares nothing', () => {
    isTauri.mockReturnValue(true)
    const mgr = createWSManager(hostPorts)
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

    expect(() => createWSManager(hostPorts).subscribe('crowbar://h1/v0/projects', vi.fn())).toThrow(
      /desktop app/,
    )
    expect(MockNativeWebSocket.instances).toHaveLength(0)
  })

  it.each(['CROWBAR://h1/v0/projects', ' crowbar://h1/v0/projects', 'crowbar://h1//v0/projects'])(
    'refuses the malformed endpoint %j rather than dialling the local daemon',
    (endpoint) => {
      isTauri.mockReturnValue(true)

      expect(() => createWSManager(hostPorts).subscribe(endpoint, vi.fn())).toThrow()
      expect(tauriInstances).toHaveLength(0)
    },
  )
})

describe('WSManager remote stream failures', () => {
  const failureOn = (endpoint: string) => useConnectionStore.getState().failures[endpoint]

  beforeEach(() => {
    vi.useFakeTimers()
    isTauri.mockReturnValue(true)
    useConnectionStore.setState(useConnectionStore.getInitialState())
  })
  afterEach(() => {
    vi.useRealTimers()
    destroyHostRuntime('h1')
  })

  it('surfaces a changed host identity, not just that the stream closed', () => {
    createWSManager(hostPorts).subscribe('crowbar://h1/v0/projects', vi.fn())

    tauriInstances[0].onerror?.('host-identity-changed: the certificate no longer matches')
    tauriInstances[0].onclose?.()

    expect(failureOn('crowbar://h1/v0/projects')).toMatchObject({
      problem: { code: 'host_identity_changed' },
      hostId: 'h1',
    })
  })

  it('surfaces an unreachable host as a different failure', () => {
    createWSManager(hostPorts).subscribe('crowbar://h1/v0/projects', vi.fn())

    tauriInstances[0].onerror?.('host-unreachable: cannot reach box:3737: connection refused')
    tauriInstances[0].onclose?.()

    expect(failureOn('crowbar://h1/v0/projects')).toMatchObject({
      problem: { code: 'host_unreachable' },
      hostId: 'h1',
    })
  })

  it('records nothing for a failure that carries no marker', () => {
    createWSManager(hostPorts).subscribe('crowbar://h1/v0/projects', vi.fn())

    tauriInstances[0].onerror?.('ws upgrade: HTTP error: 401 Unauthorized')
    tauriInstances[0].onclose?.()

    expect(useConnectionStore.getState().failures).toEqual({})
  })

  it('keeps the failure through reconnect attempts and drops it once the stream opens', () => {
    createWSManager(hostPorts).subscribe('crowbar://h1/v0/projects', vi.fn())
    tauriInstances[0].onerror?.('host-unreachable: dial timed out')
    tauriInstances[0].onclose?.()

    vi.advanceTimersByTime(1000)
    expect(tauriInstances).toHaveLength(2)
    expect(failureOn('crowbar://h1/v0/projects')?.problem.code).toBe('host_unreachable')

    tauriInstances[1].onopen?.()
    expect(useConnectionStore.getState().failures).toEqual({})
  })

  it('drops the failure when the last subscriber leaves', () => {
    const unsubscribe = createWSManager(hostPorts).subscribe('crowbar://h1/v0/projects', vi.fn())
    tauriInstances[0].onerror?.('host-identity-changed: x')
    tauriInstances[0].onclose?.()

    unsubscribe()

    expect(useConnectionStore.getState().failures).toEqual({})
  })

  it('keeps each host’s failure apart', () => {
    const mgr = createWSManager(hostPorts)
    mgr.subscribe('crowbar://h1/v0/projects', vi.fn())
    mgr.subscribe('crowbar://h2/v0/projects', vi.fn())

    tauriInstances[1].onerror?.('host-identity-changed: x')

    expect(failureOn('crowbar://h1/v0/projects')).toBeUndefined()
    expect(failureOn('crowbar://h2/v0/projects')?.hostId).toBe('h2')
  })

  it('never lets a remote outage look like a local one', () => {
    createWSManager(hostPorts).subscribe('crowbar://h1/v0/projects', vi.fn())

    tauriInstances[0].onopen?.()
    tauriInstances[0].onerror?.('host-identity-changed: certificate mismatch')
    tauriInstances[0].onclose?.()

    expect(useConnectionStore.getState().status).toBe('idle')
  })

  // The host's status, not the socket, says whether it is worth dialling: a host that needs
  // repair refuses every attempt, so its channels wait for it rather than redial.
  it('opens nothing for a host that needs repair, and reopens every channel once it is up', () => {
    const h1 = getOrCreateHostRuntime('h1').getState()
    h1.setStatus('up')
    const mgr = createWSManager(hostPorts)
    const projects = vi.fn()
    const repos = vi.fn()
    mgr.subscribe('crowbar://h1/v0/projects', projects)
    mgr.subscribe('crowbar://h1/v0/projects/p/repos', repos)
    tauriInstances.forEach((s) => s.onopen?.())

    h1.setStatus('needs-repair')
    tauriInstances[0].onerror?.('host-identity-changed: certificate mismatch')
    tauriInstances[0].onclose?.()
    tauriInstances[1].onclose?.()
    vi.advanceTimersByTime(120_000)
    mgr.subscribe('crowbar://h1/v0/chats/c/ws', vi.fn())
    mgr.subscribe('crowbar://h1/v0/projects', vi.fn())
    expect(tauriInstances).toHaveLength(2)

    h1.setStatus('up')
    expect(tauriInstances.slice(2).map((s) => [s.path, s.options?.hostId])).toEqual([
      ['/v0/projects', 'h1'],
      ['/v0/projects/p/repos', 'h1'],
      ['/v0/chats/c/ws', 'h1'],
    ])
    // A reopened channel tells its subscribers to catch up, as after any outage.
    tauriInstances[2].onopen?.()
    tauriInstances[3].onopen?.()
    expect(projects).toHaveBeenCalledWith({ reconnected: true })
    expect(repos).toHaveBeenCalledWith({ reconnected: true })
    expect(useConnectionStore.getState().failures).toEqual({})
  })

  it('does not redial a host that is down, until it is up again', () => {
    const h1 = getOrCreateHostRuntime('h1').getState()
    h1.setStatus('up')
    createWSManager(hostPorts).subscribe('crowbar://h1/v0/projects', vi.fn())

    h1.setStatus('down')
    tauriInstances[0].onerror?.('host-unreachable: connection refused')
    tauriInstances[0].onclose?.()
    vi.advanceTimersByTime(120_000)
    h1.setStatus('down')
    expect(tauriInstances).toHaveLength(1)

    h1.setStatus('up')
    expect(tauriInstances).toHaveLength(2)
  })

  it('reopens nothing nobody holds any more', () => {
    const h1 = getOrCreateHostRuntime('h1').getState()
    h1.setStatus('needs-repair')
    const unsubscribe = createWSManager(hostPorts).subscribe('crowbar://h1/v0/projects', vi.fn())

    unsubscribe()
    h1.setStatus('up')

    expect(tauriInstances).toHaveLength(0)
  })

  // Something may hold a removed host's channel a while longer (a view not yet unmounted):
  // it must not keep dialling a host nobody has paired.
  it('never dials a host that is not configured, however long its channel is held', () => {
    const h1 = getOrCreateHostRuntime('h1').getState()
    h1.setStatus('up')
    const mgr = createWSManager(hostPorts)
    mgr.subscribe('crowbar://h1/v0/projects', vi.fn())
    tauriInstances[0].onopen?.()

    resetHostWorld()
    tauriInstances[0].onclose?.()
    vi.advanceTimersByTime(60_000)
    h1.setStatus('up')
    destroyHostRuntime('h1')
    mgr.subscribe('crowbar://h1/v0/projects/p/repos', vi.fn())
    mgr.subscribe('crowbar://h9/v0/projects', vi.fn())

    expect(tauriInstances).toHaveLength(1)
  })

  it('holds back no other host and never the local daemon', () => {
    getOrCreateHostRuntime('h1').getState().setStatus('needs-repair')
    const mgr = createWSManager(hostPorts)

    mgr.subscribe('/v0/projects', vi.fn())
    mgr.subscribe('crowbar://h2/v0/projects', vi.fn())
    mgr.subscribe('crowbar://h1/v0/projects', vi.fn())

    expect(tauriInstances.map((s) => s.options?.hostId)).toEqual([undefined, 'h2'])
  })

  it('keeps redialling an unreachable host, backing off', () => {
    createWSManager(hostPorts).subscribe('crowbar://h1/v0/projects', vi.fn())

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
    const mgr = createWSManager(hostPorts)
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
