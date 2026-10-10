import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

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
const { pairRemoteHost, projectOn, resetHostWorld } =
  await import('@/__tests__/__fixtures__/remote-host')
const { startHostStatus } = await import('@/features/hosts/lib/host-status')
const { connectProjectList, useProjectDataStore } = await import('@/lib/store/projects')
const { projectListTransport } = await import('@/lib/api/project-list-transport')

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

    await vi.waitFor(() => expect(channels.length).toBe(1))
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

    await vi.waitFor(() => expect(channels.length).toBe(1))
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
      ;(window as unknown as Record<string, unknown>).__TAURI_INTERNALS__ = {}
      resetConnectionState()
    })
    afterEach(() => {
      delete (window as unknown as Record<string, unknown>).__TAURI_INTERNALS__
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
        code: 'host_identity_changed',
        hostId: 'h1',
      })
      expect(failures['crowbar://h2/v0/projects']).toMatchObject({
        code: 'host_unreachable',
        hostId: 'h2',
      })
      expect(invoke).toHaveBeenCalledWith('ws_open', expect.objectContaining({ hostId: 'h1' }))
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

    // The app's whole path: a host's daemon is replaced, its project stream's redial is refused,
    // nothing dials it while it needs repair, and once it is back it alone is re-read and streamed.
    it('streams a host’s projects again once it is back from a change of identity', async () => {
      vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
      let replaced = false
      const fetchMock = vi.fn(async (url: string) => {
        if (replaced && url === 'crowbar://h1/v0/health') {
          return new Response(null, {
            status: 502,
            headers: { 'x-crowbar-proxy': 'host-identity-changed' },
          })
        }
        const rows = url === 'crowbar://h1/v0/projects' ? [projectOn('p-h1', 'h1')] : []
        return new Response(JSON.stringify({ success: true, data: rows }), { status: 200 })
      })
      vi.stubGlobal('fetch', fetchMock)
      const opens = () =>
        invoke.mock.calls.filter(
          ([command, args]) =>
            command === 'ws_open' && (args as { hostId?: string }).hostId === 'h1',
        ).length
      const reads = () =>
        fetchMock.mock.calls.filter(([url]) => url === 'crowbar://h1/v0/projects').length
      connectProjectList(projectListTransport)
      pairRemoteHost('h1')
      const stops = [useProjectDataStore.getState().startSync(), startHostStatus()]
      try {
        await useProjectDataStore.getState().fetch()
        await vi.waitFor(() => expect(opens()).toBe(1))
        const readsWhileUp = reads()

        replaced = true
        invoke.mockImplementation(async (command: string) => {
          if (command === 'ws_open') throw 'host-identity-changed: certificate mismatch'
        })
        channels.at(-1)!.onmessage?.('\u0000crowbar-ws-close')
        await vi.advanceTimersByTimeAsync(1_000)
        expect(getHostRuntime('h1')?.getState().status).toBe('needs-repair')
        await vi.advanceTimersByTimeAsync(60_000)
        expect(opens()).toBe(2)

        replaced = false
        invoke.mockImplementation(async () => undefined)
        await vi.advanceTimersByTimeAsync(15_000)
        await vi.waitFor(() => expect(opens()).toBe(3))
        await vi.waitFor(() => expect(reads()).toBe(readsWhileUp + 1))
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
})
