import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { probeDaemon } from '@/lib/ws/daemon-probe'

vi.mock('@/lib/ws/daemon-probe', () => ({
  noteSocketOpened: vi.fn(),
  probeDaemon: vi.fn(() => Promise.resolve()),
}))

class MockWebSocket {
  // vi.stubGlobal replaces the real WebSocket, so the mock must carry the
  // standard readyState constants — production code compares against them
  // (e.g. the CONNECTING-vs-OPEN branch in closeSocketQuietly).
  static CONNECTING = 0
  static OPEN = 1
  static CLOSING = 2
  static CLOSED = 3
  static instances: MockWebSocket[] = []
  onmessage: ((e: MessageEvent) => void) | null = null
  onclose: (() => void) | null = null
  onopen: (() => void) | null = null
  onerror: ((event: unknown) => void) | null = null
  readyState: number = MockWebSocket.OPEN
  send = vi.fn()
  close = vi.fn()
  addEventListener = vi.fn()
  constructor(public url: string) {
    MockWebSocket.instances.push(this)
  }
  simulateMessage(data: string) {
    this.onmessage?.({ data } as MessageEvent)
  }
  simulateOpen() {
    this.readyState = MockWebSocket.OPEN
    this.onopen?.()
  }
  simulateClose() {
    this.readyState = MockWebSocket.CLOSED
    this.onclose?.()
  }
}

beforeEach(() => {
  MockWebSocket.instances = []
  vi.mocked(probeDaemon).mockClear()
})

vi.stubGlobal('WebSocket', MockWebSocket)

const install = vi.hoisted(() => ({ localDaemon: true }))
vi.mock('@/features/hosts/lib/local-daemon', () => ({ hasLocalDaemon: () => install.localDaemon }))

const { createWSManager } = await import('@/lib/ws/manager')
const { useConnectionStore, resetConnectionState } = await import('@/lib/ws/connection-store')

// The phone app: a local endpoint has no daemon behind it, so it is neither dialled nor
// reported down (which would raise "Backend unavailable" for a daemon that never existed).
describe('WSManager with no local daemon', () => {
  beforeEach(() => {
    install.localDaemon = false
    resetConnectionState()
  })
  afterEach(() => {
    install.localDaemon = true
  })

  it('never dials a local endpoint and reports no local outage', () => {
    const mgr = createWSManager()
    const unsubscribe = mgr.subscribe('/v0/ws/git', vi.fn())

    expect(MockWebSocket.instances).toHaveLength(0)
    expect(useConnectionStore.getState().status).toBe('idle')
    unsubscribe()
    expect(useConnectionStore.getState().status).toBe('idle')
  })
})

describe('WSManager', () => {
  it('opens one socket for two subscribers to the same endpoint', () => {
    const mgr = createWSManager()
    const cb1 = vi.fn()
    const cb2 = vi.fn()
    mgr.subscribe('/v0/ws/git', cb1)
    mgr.subscribe('/v0/ws/git', cb2)
    expect(MockWebSocket.instances).toHaveLength(1)
  })

  it('calls all subscribers when a message arrives', () => {
    const mgr = createWSManager()
    const cb1 = vi.fn()
    const cb2 = vi.fn()
    mgr.subscribe('/v0/ws/git', cb1)
    mgr.subscribe('/v0/ws/git', cb2)
    MockWebSocket.instances[0].simulateMessage('{"changed":true}')
    expect(cb1).toHaveBeenCalledWith({ changed: true })
    expect(cb2).toHaveBeenCalledWith({ changed: true })
  })

  it('closes socket when last subscriber unsubscribes', () => {
    const mgr = createWSManager()
    const unsub1 = mgr.subscribe('/v0/ws/git', vi.fn())
    const unsub2 = mgr.subscribe('/v0/ws/git', vi.fn())
    unsub1()
    expect(MockWebSocket.instances[0].close).not.toHaveBeenCalled()
    unsub2()
    expect(MockWebSocket.instances[0].close).toHaveBeenCalled()
  })

  // A backend restart must not kill subscriptions silently: the manager reopens the socket, keeps
  // the subscribers, and emits { reconnected: true } so they can refetch missed state.
  it('reopens the socket and keeps subscribers after a close', () => {
    vi.useFakeTimers()
    try {
      const mgr = createWSManager()
      const cb = vi.fn()
      mgr.subscribe('/v0/ws/git', cb)
      MockWebSocket.instances[0].simulateClose()
      expect(MockWebSocket.instances).toHaveLength(1)
      vi.advanceTimersByTime(1000)
      expect(MockWebSocket.instances).toHaveLength(2)
      MockWebSocket.instances[1].simulateOpen()
      expect(cb).toHaveBeenCalledWith({ reconnected: true })
      MockWebSocket.instances[1].simulateMessage('{"after":"reconnect"}')
      expect(cb).toHaveBeenCalledWith({ after: 'reconnect' })
    } finally {
      vi.useRealTimers()
    }
  })

  // A refetch while the daemon is still down only fails (and retries): the
  // sentinel means "the stream is back", so each outage emits it once, on open.
  it('emits the reconnect sentinel once the stream is back, not per failed attempt', () => {
    vi.useFakeTimers()
    try {
      const mgr = createWSManager()
      const cb = vi.fn()
      mgr.subscribe('/v0/ws/git', cb)
      MockWebSocket.instances[0].simulateClose()
      vi.advanceTimersByTime(1000)
      MockWebSocket.instances[1].simulateClose()
      vi.advanceTimersByTime(2000)
      MockWebSocket.instances[2].simulateClose()
      vi.advanceTimersByTime(4000)
      expect(MockWebSocket.instances).toHaveLength(4)
      expect(cb).not.toHaveBeenCalled()

      MockWebSocket.instances[3].simulateOpen()
      expect(cb).toHaveBeenCalledTimes(1)
      expect(cb).toHaveBeenCalledWith({ reconnected: true })
    } finally {
      vi.useRealTimers()
    }
  })

  it('does not reopen when the last subscriber left during the reconnect delay', () => {
    vi.useFakeTimers()
    try {
      const mgr = createWSManager()
      const unsub = mgr.subscribe('/v0/ws/git', vi.fn())
      MockWebSocket.instances[0].simulateClose()
      unsub()
      vi.advanceTimersByTime(60_000)
      expect(MockWebSocket.instances).toHaveLength(1)
    } finally {
      vi.useRealTimers()
    }
  })

  it('unsubscribing after a reconnect closes the live socket, not the stale one', () => {
    vi.useFakeTimers()
    try {
      const mgr = createWSManager()
      const unsub = mgr.subscribe('/v0/ws/git', vi.fn())
      MockWebSocket.instances[0].simulateClose()
      vi.advanceTimersByTime(1000)
      expect(MockWebSocket.instances).toHaveLength(2)
      unsub()
      expect(MockWebSocket.instances[1].close).toHaveBeenCalled()
    } finally {
      vi.useRealTimers()
    }
  })

  it('backs off across consecutive failed reconnects', () => {
    vi.useFakeTimers()
    try {
      const mgr = createWSManager()
      mgr.subscribe('/v0/ws/git', vi.fn())
      MockWebSocket.instances[0].simulateClose()
      vi.advanceTimersByTime(1000)
      expect(MockWebSocket.instances).toHaveLength(2)
      // Second socket dies immediately (backend still down): the next attempt
      // waits 2s, not 1s.
      MockWebSocket.instances[1].simulateClose()
      vi.advanceTimersByTime(1999)
      expect(MockWebSocket.instances).toHaveLength(2)
      vi.advanceTimersByTime(1)
      expect(MockWebSocket.instances).toHaveLength(3)
      // A successful open resets the backoff to the base delay.
      MockWebSocket.instances[2].simulateOpen()
      MockWebSocket.instances[2].simulateClose()
      vi.advanceTimersByTime(1000)
      expect(MockWebSocket.instances).toHaveLength(4)
    } finally {
      vi.useRealTimers()
    }
  })

  describe('opened', () => {
    /** A subscription whose socket is still connecting. */
    function connecting(mgr: ReturnType<typeof createWSManager>, endpoint = '/v0/chats/c1/ws') {
      const unsubscribe = mgr.subscribe(endpoint, vi.fn())
      const socket = MockWebSocket.instances.at(-1)!
      socket.readyState = MockWebSocket.CONNECTING
      return { socket, unsubscribe }
    }

    const settled = (p: Promise<void>) =>
      p.then(
        () => 'opened',
        () => 'refused',
      )

    it('resolves when the endpoint socket opens, and at once when it already is', async () => {
      const mgr = createWSManager()
      const { socket } = connecting(mgr)
      const waiting = settled(mgr.opened('/v0/chats/c1/ws'))

      socket.simulateOpen()

      expect(await waiting).toBe('opened')
      expect(await settled(mgr.opened('/v0/chats/c1/ws'))).toBe('opened')
    })

    it('rejects when the socket closes before it opens', async () => {
      vi.useFakeTimers()
      try {
        const mgr = createWSManager()
        const { socket } = connecting(mgr)
        const waiting = settled(mgr.opened('/v0/chats/c1/ws'))

        socket.simulateClose()

        expect(await waiting).toBe('refused')
      } finally {
        vi.useRealTimers()
      }
    })

    it('rejects with why the transport failed, as its cause', async () => {
      vi.useFakeTimers()
      try {
        const mgr = createWSManager()
        const { socket } = connecting(mgr)
        const waiting = mgr.opened('/v0/chats/c1/ws').catch((error: unknown) => error)

        socket.onerror?.('ws upgrade: HTTP error: 507 Insufficient Storage')
        socket.simulateClose()

        expect(((await waiting) as Error).cause).toBe(
          'ws upgrade: HTTP error: 507 Insufficient Storage',
        )
      } finally {
        vi.useRealTimers()
      }
    })

    it('rejects when nothing subscribes to the endpoint, or the last subscriber leaves first', async () => {
      const mgr = createWSManager()
      expect(await settled(mgr.opened('/v0/chats/c1/ws'))).toBe('refused')

      const { unsubscribe } = connecting(mgr)
      const waiting = settled(mgr.opened('/v0/chats/c1/ws'))
      unsubscribe()

      expect(await waiting).toBe('refused')
    })

    it('waits on a dropped socket for the reconnect it is waiting to make, and that one alone', async () => {
      vi.useFakeTimers()
      try {
        const mgr = createWSManager()
        const { socket } = connecting(mgr)
        socket.simulateOpen()
        socket.simulateClose()

        const back = settled(mgr.opened('/v0/chats/c1/ws'))
        vi.advanceTimersByTime(1000)
        MockWebSocket.instances[1].simulateOpen()
        expect(await back).toBe('opened')

        MockWebSocket.instances[1].simulateClose()
        const refused = settled(mgr.opened('/v0/chats/c1/ws'))
        vi.advanceTimersByTime(1000)
        MockWebSocket.instances[2].simulateClose()
        expect(await refused).toBe('refused')
      } finally {
        vi.useRealTimers()
      }
    })

    it('tells a drop listener when an open socket drops, never when its last subscriber leaves', () => {
      vi.useFakeTimers()
      try {
        const mgr = createWSManager()
        const dropped = vi.fn()
        mgr.onDrop('/v0/chats/c1/ws', dropped)
        const { socket, unsubscribe } = connecting(mgr)
        socket.simulateClose()
        expect(dropped).not.toHaveBeenCalled()

        vi.advanceTimersByTime(1000)
        MockWebSocket.instances[1].simulateOpen()
        MockWebSocket.instances[1].simulateClose()
        expect(dropped).toHaveBeenCalledTimes(1)

        vi.advanceTimersByTime(2000)
        MockWebSocket.instances[2].simulateOpen()
        MockWebSocket.instances[2].close.mockImplementation(() =>
          MockWebSocket.instances[2].simulateClose(),
        )
        unsubscribe()
        expect(dropped).toHaveBeenCalledTimes(1)
      } finally {
        vi.useRealTimers()
      }
    })
  })

  describe('send', () => {
    it('holds what is sent while the socket connects and sends it once, in order, on open', () => {
      const mgr = createWSManager()
      mgr.subscribe('/v0/stream', vi.fn())
      const socket = MockWebSocket.instances[0]
      socket.readyState = MockWebSocket.CONNECTING

      mgr.send('/v0/stream', { op: 'sub', scope: 'projects' })
      mgr.send('/v0/stream', { op: 'sub', scope: 'ws:w1' })
      expect(socket.send).not.toHaveBeenCalled()

      socket.simulateOpen()
      mgr.send('/v0/stream', { op: 'unsub', scope: 'ws:w1' })

      expect(socket.send.mock.calls).toEqual([
        ['{"op":"sub","scope":"projects"}'],
        ['{"op":"sub","scope":"ws:w1"}'],
        ['{"op":"unsub","scope":"ws:w1"}'],
      ])
    })

    // The reconnect's own open is where its subscribers say again what they need.
    it('never replays on the reconnect what a socket that closed before opening held', () => {
      vi.useFakeTimers()
      try {
        const mgr = createWSManager()
        mgr.subscribe('/v0/stream', vi.fn())
        const socket = MockWebSocket.instances[0]
        socket.readyState = MockWebSocket.CONNECTING
        mgr.send('/v0/stream', { op: 'sub', scope: 'projects' })

        socket.simulateClose()
        vi.advanceTimersByTime(1000)
        MockWebSocket.instances[1].simulateOpen()

        expect(socket.send).not.toHaveBeenCalled()
        expect(MockWebSocket.instances[1].send).not.toHaveBeenCalled()
      } finally {
        vi.useRealTimers()
      }
    })
  })

  it('opened rejects when the transport cannot carry a socket', async () => {
    vi.resetModules()
    // The real manager, loaded afresh over a transport that cannot carry a socket.
    vi.doUnmock('@/lib/ws/manager')
    vi.doMock('@/lib/ws/url', async (importOriginal) => ({
      ...(await importOriginal<typeof import('@/lib/ws/url')>()),
      isWebSocketCapable: () => false,
    }))
    try {
      const fresh = await import('@/lib/ws/manager')
      const mgr = fresh.createWSManager()
      mgr.subscribe('/v0/chats/c1/ws', vi.fn())

      await expect(mgr.opened('/v0/chats/c1/ws')).rejects.toThrow()
      expect(MockWebSocket.instances).toHaveLength(0)
    } finally {
      vi.doUnmock('@/lib/ws/url')
      vi.resetModules()
    }
  })

  it('a socket that closes before it opened probes the daemon once', () => {
    const mgr = createWSManager()
    const unsubscribe = mgr.subscribe('/v0/ws/git', vi.fn())

    MockWebSocket.instances[0].simulateClose()

    expect(probeDaemon).toHaveBeenCalledTimes(1)
    unsubscribe()
  })

  it('a socket that closed after opening does not probe until a reconnect fails', () => {
    vi.useFakeTimers()
    try {
      const mgr = createWSManager()
      const unsubscribe = mgr.subscribe('/v0/ws/git', vi.fn())
      MockWebSocket.instances[0].simulateOpen()
      MockWebSocket.instances[0].simulateClose()
      expect(probeDaemon).not.toHaveBeenCalled()

      vi.advanceTimersByTime(1000)
      MockWebSocket.instances[1].simulateClose()

      expect(probeDaemon).toHaveBeenCalledTimes(1)
      unsubscribe()
    } finally {
      vi.useRealTimers()
    }
  })
})
