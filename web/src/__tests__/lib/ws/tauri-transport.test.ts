import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

// The transport mints a Tauri Channel and bridges its frames into the
// WebSocket-like onmessage callback. We mock @tauri-apps/api/core so a frame
// pushed at the captured Channel fires onmessage with the RAW DTO text, and so
// the open/send/close handlers invoke the right ws_* commands.

interface MockChannel {
  onmessage: ((data: string | ArrayBuffer) => void) | null
}

const channels: MockChannel[] = []
const invoke = vi.fn(async (_command: string, _args?: unknown): Promise<unknown> => undefined)

// convertFileSrc is only here because the manager imports the bridge, which re-exports it.
vi.mock('@tauri-apps/api/core', () => ({
  convertFileSrc: (path: string) => path,
  Channel: class {
    onmessage: ((data: string | ArrayBuffer) => void) | null = null
    constructor() {
      channels.push(this)
    }
  },
  invoke,
}))

beforeEach(() => {
  channels.length = 0
  invoke.mockClear()
  vi.stubGlobal('crypto', { randomUUID: () => 'conn-fixed-id' })
})

const { TauriWebSocket } = await import('@/lib/ws/tauri-transport')
const { createWSManager } = await import('@/lib/ws/manager')
const { useConnectionStore, resetConnectionState } = await import('@/lib/ws/connection-store')
const { followStreamFailures } = await import('@/features/hosts/lib/host-failures')
const { destroyHostRuntime, getHostRuntime, getOrCreateHostRuntime } =
  await import('@/features/hosts/stores/host-runtime-registry')
const { pairRemoteHost, resetHostWorld } = await import('@/__tests__/__fixtures__/remote-host')
const { startHostStatus } = await import('@/features/hosts/lib/host-status')

/** Holds the next ws_open unresolved until `resolve`: Rust's reader may already be forwarding. */
function gateOpen() {
  let resolve = () => {}
  let reject = (_err: unknown) => {}
  invoke.mockImplementationOnce(
    () =>
      new Promise<undefined>((res, rej) => {
        resolve = () => res(undefined)
        reject = rej
      }),
  )
  return { resolve: () => resolve(), reject: (err: unknown) => reject(err) }
}

/** Records the shim's events in the order the manager would hear them. */
function record(sock: InstanceType<typeof TauriWebSocket>) {
  const events: string[] = []
  sock.onopen = () => void events.push(`open@${sock.readyState}`)
  sock.onmessage = (e) => void events.push(`msg ${String(e.data)}@${sock.readyState}`)
  sock.onclose = () => void events.push('close')
  sock.onerror = () => void events.push('error')
  return events
}

const opened = (sock: InstanceType<typeof TauriWebSocket>) =>
  vi.waitFor(() => expect(sock.readyState).toBe(TauriWebSocket.OPEN))

describe('TauriWebSocket', () => {
  it('exposes the WebSocket readyState constants', () => {
    expect(TauriWebSocket.CONNECTING).toBe(0)
    expect(TauriWebSocket.OPEN).toBe(1)
    expect(TauriWebSocket.CLOSED).toBe(3)
  })

  it('invokes ws_open with the conn id, path, and Channel, then fires onopen and goes OPEN', async () => {
    const sock = new TauriWebSocket('/v0/projects/p/repos/r/workspaces')
    const onopen = vi.fn()
    sock.onopen = onopen

    // Let the ws_open invoke promise resolve.
    await vi.waitFor(() => expect(invoke).toHaveBeenCalled())
    await Promise.resolve()

    expect(invoke).toHaveBeenCalledWith('ws_open', {
      connId: 'conn-fixed-id',
      path: '/v0/projects/p/repos/r/workspaces',
      onMessage: channels[0],
      // No read-idle timeout unless the stream asks for one (only the terminal's does).
      idleTimeoutMs: null,
    })
    await vi.waitFor(() => expect(onopen).toHaveBeenCalled())
    expect(sock.readyState).toBe(TauriWebSocket.OPEN)
  })

  it('forwards a raw DTO frame from the Channel to onmessage without extracting .data', async () => {
    const sock = new TauriWebSocket('/v0/projects/p/repos/r/workspaces')
    const onmessage = vi.fn()
    sock.onmessage = onmessage

    await opened(sock)
    const dto = '{"id":"ws-1","status":"new","data":"not-extracted"}'
    channels[0].onmessage?.(dto)

    expect(onmessage).toHaveBeenCalledWith({ data: dto })
  })

  it('treats the close sentinel as a close (fires onclose, goes CLOSED) — not a message', async () => {
    const sock = new TauriWebSocket('/v0/projects/p/repos/r/workspaces')
    const onmessage = vi.fn()
    const onclose = vi.fn()
    sock.onmessage = onmessage
    sock.onclose = onclose

    await opened(sock)
    // The Rust reader pushes this exact sentinel when the daemon closes the stream.
    channels[0].onmessage?.('\u0000crowbar-ws-close')

    expect(onclose).toHaveBeenCalledTimes(1)
    expect(onmessage).not.toHaveBeenCalled()
    expect(sock.readyState).toBe(TauriWebSocket.CLOSED)
  })

  it('passes a read-idle timeout through to the bridge when asked', async () => {
    new TauriWebSocket('/v0/chats/c/terminals/s/ws', { idleTimeoutMs: 90_000 })
    await vi.waitFor(() =>
      expect(invoke).toHaveBeenCalledWith(
        'ws_open',
        expect.objectContaining({ idleTimeoutMs: 90_000 }),
      ),
    )
  })

  // 'local' (the default every existing caller keeps using) must send NO
  // hostId at all — Tauri Bridge plan Task 6's ws_open treats an omitted
  // host_id as the local daemon, so sending 'local' explicitly would be a
  // different (if harmless) wire shape than today's zero-arg callers produce.
  it('omits hostId from ws_open when the host is "local" (the default)', async () => {
    new TauriWebSocket('/v0/projects/p/repos/r/workspaces', { hostId: 'local' })
    // toHaveBeenCalledWith treats an `undefined`-valued key as absent, so this
    // also proves the real call carries no hostId at all for the local host.
    await vi.waitFor(() =>
      expect(invoke).toHaveBeenCalledWith('ws_open', {
        connId: 'conn-fixed-id',
        path: '/v0/projects/p/repos/r/workspaces',
        onMessage: expect.anything(),
        idleTimeoutMs: null,
      }),
    )
  })

  it('passes a non-local hostId through to ws_open', async () => {
    new TauriWebSocket('/v0/projects/p/repos/r/workspaces', { hostId: 'h1' })
    await vi.waitFor(() =>
      expect(invoke).toHaveBeenCalledWith('ws_open', expect.objectContaining({ hostId: 'h1' })),
    )
  })

  // ws_open rejects a remote host it cannot reach, or finds under another identity,
  // as `<x-crowbar-proxy value>: <reason>` (ws_bridge/remote.rs); Tauri hands that
  // string to JS as is. The app reads the marker from it, so it must not be altered.
  it('hands a refused open’s reason to onerror exactly as Rust phrased it, then closes', async () => {
    const reason = 'host-identity-changed: the certificate presented by box:3737 no longer matches'
    invoke.mockRejectedValueOnce(reason)
    const sock = new TauriWebSocket('/v0/projects', { hostId: 'h1' })
    const events: unknown[] = []
    sock.onerror = (e) => events.push(['error', e])
    sock.onclose = () => events.push(['close'])

    await vi.waitFor(() => expect(events).toHaveLength(2))

    expect(events).toEqual([['error', reason], ['close']])
    expect(sock.readyState).toBe(TauriWebSocket.CLOSED)
  })

  describe('through the manager', () => {
    beforeEach(() => {
      ;(window as unknown as Record<string, unknown>).__TAURI_INTERNALS__ = {
        invoke: async () => {},
      }
      resetConnectionState()
      // Only a paired host is dialled.
      pairRemoteHost('h1')
      pairRemoteHost('h2')
    })
    afterEach(() => {
      delete (window as unknown as Record<string, unknown>).__TAURI_INTERNALS__
      resetHostWorld()
    })

    it('tells a changed identity from an unreachable host on a remote stream', async () => {
      invoke.mockRejectedValueOnce('host-identity-changed: certificate mismatch')
      invoke.mockRejectedValueOnce('host-unreachable: cannot reach box2:3737: connection refused')
      const mgr = createWSManager()

      const stopChanged = mgr.subscribe('crowbar://h1/v0/projects', vi.fn())
      const stopDown = mgr.subscribe('crowbar://h2/v0/projects', vi.fn())

      await vi.waitFor(() =>
        expect(Object.keys(useConnectionStore.getState().failures)).toHaveLength(2),
      )
      const { failures } = useConnectionStore.getState()
      expect(failures['crowbar://h1/v0/projects']).toMatchObject({
        problem: { code: 'host_identity_changed' },
        hostId: 'h1',
      })
      expect(failures['crowbar://h2/v0/projects']).toMatchObject({
        problem: { code: 'host_unreachable' },
        hostId: 'h2',
      })
      // Armed with the bridge's read-idle close (two daemon pings): a half-open one reopens.
      expect(invoke).toHaveBeenCalledWith(
        'ws_open',
        expect.objectContaining({ hostId: 'h1', idleTimeoutMs: 60_000 }),
      )
      expect(invoke).toHaveBeenCalledWith('ws_open', expect.objectContaining({ hostId: 'h2' }))
      stopChanged()
      stopDown()
    })

    // A halted channel used to come back only once every subscriber had left and one came
    // back; now the host's own return reopens it, whoever holds it.
    it('opens nothing more for a host whose identity changed, and reopens it once it is up', async () => {
      vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
      pairRemoteHost('h1')
      getOrCreateHostRuntime('h1').getState().setStatus('up')
      const stopFailures = followStreamFailures()
      const opensOf = () =>
        invoke.mock.calls.filter(
          ([command, args]) =>
            command === 'ws_open' && (args as { hostId?: string }).hostId === 'h1',
        ).length
      try {
        invoke.mockRejectedValueOnce('host-identity-changed: certificate mismatch')
        const mgr = createWSManager()
        const frames = vi.fn()
        mgr.subscribe('crowbar://h1/v0/projects', frames)
        mgr.subscribe('crowbar://h1/v0/projects', vi.fn())
        await vi.waitFor(() => expect(getHostRuntime('h1')?.getState().status).toBe('needs-repair'))

        await vi.advanceTimersByTimeAsync(120_000)
        mgr.subscribe('crowbar://h1/v0/projects/p/repos', vi.fn())
        expect(opensOf()).toBe(1)

        getHostRuntime('h1')!.getState().setStatus('up')
        await vi.waitFor(() => expect(frames).toHaveBeenCalledWith({ reconnected: true }))
        expect(opensOf()).toBe(3)
      } finally {
        stopFailures()
        destroyHostRuntime('h1')
        resetHostWorld()
        vi.useRealTimers()
      }
    })

    // Measured: a remote daemon back after a short restart has its streams back within
    // seconds of answering, not a probe interval later.
    it('reopens a restarted host’s streams within seconds of it answering again', async () => {
      vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'Date'] })
      let daemonUp = true
      vi.stubGlobal(
        'fetch',
        vi.fn(async () =>
          daemonUp
            ? new Response(JSON.stringify({ success: true, data: {} }), { status: 200 })
            : new Response(null, {
                status: 502,
                headers: { 'x-crowbar-proxy': 'host-unreachable' },
              }),
        ),
      )
      invoke.mockImplementation(async (command: string) => {
        if (command === 'ws_open' && !daemonUp) throw 'host-unreachable: connection refused'
      })
      pairRemoteHost('h1')
      const stops = [startHostStatus()]
      const status = () => getHostRuntime('h1')?.getState().status
      try {
        await vi.waitFor(() => expect(status()).toBe('up'))
        const frames = vi.fn()
        stops.push(createWSManager().subscribe('crowbar://h1/v0/projects/p/repos', frames))
        await vi.waitFor(() => expect(channels).toHaveLength(1))

        daemonUp = false
        const stoppedAt = Date.now()
        channels[0].onmessage?.('\u0000crowbar-ws-close')
        await vi.advanceTimersByTimeAsync(1_500)
        expect(status()).toBe('down')
        daemonUp = true

        await vi.advanceTimersByTimeAsync(2_000)
        expect(frames).toHaveBeenCalledWith({ reconnected: true })
        expect(status()).toBe('up')
        expect(Date.now() - stoppedAt).toBe(3_500)
      } finally {
        stops.forEach((stop) => stop())
        invoke.mockImplementation(async () => undefined)
        destroyHostRuntime('h1')
        resetHostWorld()
        vi.unstubAllGlobals()
        vi.useRealTimers()
      }
    })
  })

  it('surfaces a binary frame (terminal output) as its ArrayBuffer', async () => {
    const sock = new TauriWebSocket('/v0/chats/c/terminals/s/ws')
    const onmessage = vi.fn()
    sock.onmessage = onmessage
    await opened(sock)
    const bytes = new Uint8Array([0, 104, 105]).buffer
    channels[channels.length - 1].onmessage?.(bytes)
    expect(onmessage).toHaveBeenCalledWith({ data: bytes })
  })

  it('send() invokes ws_send with the conn id and raw data', async () => {
    const sock = new TauriWebSocket('/v0/projects/p/repos/r/workspaces')
    await vi.waitFor(() => expect(invoke).toHaveBeenCalledWith('ws_open', expect.anything()))
    invoke.mockClear()

    sock.send('{"hello":true}')
    expect(invoke).toHaveBeenCalledWith('ws_send', {
      connId: 'conn-fixed-id',
      data: '{"hello":true}',
    })
  })

  it('close() invokes ws_close, fires onclose, and goes CLOSED', async () => {
    const sock = new TauriWebSocket('/v0/projects/p/repos/r/workspaces')
    const onclose = vi.fn()
    sock.onclose = onclose
    await vi.waitFor(() => expect(invoke).toHaveBeenCalledWith('ws_open', expect.anything()))
    invoke.mockClear()

    sock.close()
    expect(invoke).toHaveBeenCalledWith('ws_close', { connId: 'conn-fixed-id' })
    expect(onclose).toHaveBeenCalled()
    expect(sock.readyState).toBe(TauriWebSocket.CLOSED)
  })

  it('close() while CONNECTING defers ws_close until ws_open resolves and never fires onopen', async () => {
    // Gate the ws_open resolution so we can close() while the socket is still
    // CONNECTING — the StrictMode double-mount / rapid scope-switch race that
    // leaks a daemon FD + 2 tokio tasks if the deferred teardown is missing.
    let resolveOpen: (() => void) | undefined
    invoke.mockImplementationOnce(
      () =>
        new Promise<undefined>((resolve) => {
          resolveOpen = () => resolve(undefined)
        }),
    )

    const sock = new TauriWebSocket('/v0/projects/p/repos/r/workspaces')
    const onopen = vi.fn()
    sock.onopen = onopen

    // The Channel is minted, but ws_open has NOT resolved: still CONNECTING.
    await vi.waitFor(() => expect(channels.length).toBe(1))
    expect(sock.readyState).toBe(TauriWebSocket.CONNECTING)

    // Close before the handshake completes. Rust hasn't registered the
    // connection yet, so no eager ws_close (it would be a Rust-side no-op).
    sock.close()
    expect(sock.readyState).toBe(TauriWebSocket.CLOSED)
    expect(invoke).not.toHaveBeenCalledWith('ws_close', expect.anything())

    // ws_open now resolves: the connection is registered, so the deferred
    // ws_close fires to tear it down — and onopen must NOT fire.
    resolveOpen?.()
    await vi.waitFor(() =>
      expect(invoke).toHaveBeenCalledWith('ws_close', { connId: 'conn-fixed-id' }),
    )
    expect(onopen).not.toHaveBeenCalled()
    expect(sock.readyState).toBe(TauriWebSocket.CLOSED)
  })

  // Rust forwards frames as soon as the daemon sends them, and the Channel races the
  // ws_open reply: a frame may land while the shim still says CONNECTING.
  describe('a frame that arrives before ws_open resolves', () => {
    it('is held until onopen has fired, then delivered once, while OPEN', async () => {
      const gate = gateOpen()
      const sock = new TauriWebSocket('/v0/stream')
      const events = record(sock)

      channels[0].onmessage?.('{"t":"ready","epoch":"e1"}')
      expect(events).toEqual([])

      gate.resolve()
      await opened(sock)
      channels[0].onmessage?.('{"t":"synced","scope":"projects"}')

      expect(events).toEqual([
        'open@1',
        'msg {"t":"ready","epoch":"e1"}@1',
        'msg {"t":"synced","scope":"projects"}@1',
      ])
    })

    it('keeps the order of a 5,000-frame burst, each frame once', async () => {
      const gate = gateOpen()
      const sock = new TauriWebSocket('/v0/stream')
      const seen: string[] = []
      sock.onmessage = (e) => void seen.push(String(e.data))

      const burst = Array.from({ length: 5000 }, (_, i) => `{"n":${i}}`)
      for (const frame of burst) channels[0].onmessage?.(frame)
      gate.resolve()
      await opened(sock)

      expect(seen).toEqual(burst)
    })

    it('a daemon close among them closes after the frames before it, and drops the ones after', async () => {
      const gate = gateOpen()
      const sock = new TauriWebSocket('/v0/stream')
      const events = record(sock)

      channels[0].onmessage?.('a')
      channels[0].onmessage?.('\u0000crowbar-ws-close')
      channels[0].onmessage?.('b')
      gate.resolve()
      await vi.waitFor(() => expect(events).toContain('close'))

      expect(events).toEqual(['open@1', 'msg a@1', 'close'])
      expect(sock.readyState).toBe(TauriWebSocket.CLOSED)
    })

    it('is never delivered when the socket is closed while connecting', async () => {
      const gate = gateOpen()
      const sock = new TauriWebSocket('/v0/stream')
      const events = record(sock)

      channels[0].onmessage?.('a')
      sock.close()
      gate.resolve()
      await vi.waitFor(() =>
        expect(invoke).toHaveBeenCalledWith('ws_close', { connId: 'conn-fixed-id' }),
      )
      channels[0].onmessage?.('b')

      expect(events).toEqual(['close'])
    })

    it('is never delivered when ws_open fails', async () => {
      const gate = gateOpen()
      const sock = new TauriWebSocket('/v0/stream')
      const events = record(sock)

      channels[0].onmessage?.('a')
      gate.reject(new Error('ws upgrade: 503'))
      await vi.waitFor(() => expect(events).toContain('close'))

      expect(events).toEqual(['error', 'close'])
    })
  })

  it('drops a frame that arrives after close()', async () => {
    const sock = new TauriWebSocket('/v0/stream')
    await opened(sock)
    const events = record(sock)

    sock.close()
    channels[0].onmessage?.('late')

    expect(events).toEqual(['close'])
  })
})

describe('the stream over the desktop transport', () => {
  const sent = () =>
    (invoke.mock.calls as unknown as [string, { data: string }][])
      .filter(([cmd]) => cmd === 'ws_send')
      .map(([, args]) => JSON.parse(args.data) as unknown)

  beforeAll(() => {
    ;(window as unknown as Record<string, unknown>).__TAURI_INTERNALS__ = {}
  })
  afterAll(() => {
    delete (window as unknown as Record<string, unknown>).__TAURI_INTERNALS__
  })

  it('subscribes every held scope on a ready that beat ws_open, applies the snapshot, and again after a daemon restart', async () => {
    // The real manager and stream, not setup's fake stream daemon.
    vi.doUnmock('@/lib/ws/manager')
    vi.doUnmock('@/core/stream/stream')
    vi.resetModules()
    const stream = await import('@/core/stream/stream')
    const frames: string[] = []
    const releases = [
      stream.registerScopeKind('projects', { frame: (_h, f) => void frames.push(f.t) }),
      stream.registerScopeKind('ws', { frame: () => {} }),
      stream.holdScope('local', 'projects'),
      stream.holdScope('local', 'ws:w1'),
    ]
    const synced = vi.fn()
    releases.push(stream.watchScope('local', 'projects', { synced }))

    const first = gateOpen()
    releases.push(stream.startStream())
    channels[0].onmessage?.('{"t":"ready","epoch":"e1"}')
    first.resolve()
    await vi.waitFor(() => expect(sent()).toHaveLength(2))
    expect(sent()).toEqual([
      { op: 'sub', scope: 'projects' },
      { op: 'sub', scope: 'ws:w1' },
    ])

    channels[0].onmessage?.(
      JSON.stringify({
        t: 'entity',
        scope: 'projects',
        kind: 'operation',
        id: 'op1',
        v: 1,
        data: { id: 'op1', kind: 'repo.add', subject: { kind: 'project', id: 'p1' }, items: [] },
      }),
    )
    channels[0].onmessage?.('{"t":"synced","scope":"projects"}')
    expect(frames).toEqual(['entity', 'synced'])
    expect(synced).toHaveBeenCalledTimes(1)

    vi.useFakeTimers({ toFake: ['setTimeout'] })
    try {
      invoke.mockClear()
      const second = gateOpen()
      channels[0].onmessage?.('\u0000crowbar-ws-close')
      vi.advanceTimersByTime(1000)
      expect(channels).toHaveLength(2)
      channels[1].onmessage?.('{"t":"ready","epoch":"e2"}')
      second.resolve()
    } finally {
      vi.useRealTimers()
    }
    await vi.waitFor(() => expect(sent()).toHaveLength(2))
    expect(sent()).toEqual([
      { op: 'sub', scope: 'projects' },
      { op: 'sub', scope: 'ws:w1' },
    ])
    releases.reverse().forEach((release) => release())
  })
})
