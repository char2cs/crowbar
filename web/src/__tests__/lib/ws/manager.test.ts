import { describe, it, expect, vi, beforeEach } from 'vitest'

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
  onerror: (() => void) | null = null
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
})

vi.stubGlobal('WebSocket', MockWebSocket)

const { createWSManager } = await import('@/lib/ws/manager')

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

  // Regression: the backend restarting must not kill subscriptions silently.
  // The manager reopens the socket, keeps the same subscribers, and emits a
  // { reconnected: true } frame so they can refetch missed state.
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
    function connecting(mgr: ReturnType<typeof createWSManager>, endpoint = '/v0/documents/ws') {
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
      const waiting = settled(mgr.opened('/v0/documents/ws'))

      socket.simulateOpen()

      expect(await waiting).toBe('opened')
      expect(await settled(mgr.opened('/v0/documents/ws'))).toBe('opened')
    })

    it('rejects when the socket closes before it opens', async () => {
      vi.useFakeTimers()
      try {
        const mgr = createWSManager()
        const { socket } = connecting(mgr)
        const waiting = settled(mgr.opened('/v0/documents/ws'))

        socket.simulateClose()

        expect(await waiting).toBe('refused')
      } finally {
        vi.useRealTimers()
      }
    })

    it('rejects when nothing subscribes to the endpoint, or the last subscriber leaves first', async () => {
      const mgr = createWSManager()
      expect(await settled(mgr.opened('/v0/documents/ws'))).toBe('refused')

      const { unsubscribe } = connecting(mgr)
      const waiting = settled(mgr.opened('/v0/documents/ws'))
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

        const back = settled(mgr.opened('/v0/documents/ws'))
        vi.advanceTimersByTime(1000)
        MockWebSocket.instances[1].simulateOpen()
        expect(await back).toBe('opened')

        MockWebSocket.instances[1].simulateClose()
        const refused = settled(mgr.opened('/v0/documents/ws'))
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
        mgr.onDrop('/v0/documents/ws', dropped)
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

  it('subscribed holds while anything subscribes, through a reconnect', () => {
    vi.useFakeTimers()
    try {
      const mgr = createWSManager()
      expect(mgr.subscribed('/v0/documents/ws')).toBe(false)
      const unsubscribe = mgr.subscribe('/v0/documents/ws', vi.fn())
      MockWebSocket.instances[0].simulateClose()
      expect(mgr.subscribed('/v0/documents/ws')).toBe(true)
      vi.advanceTimersByTime(1000)
      expect(mgr.subscribed('/v0/documents/ws')).toBe(true)

      unsubscribe()

      expect(mgr.subscribed('/v0/documents/ws')).toBe(false)
    } finally {
      vi.useRealTimers()
    }
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
      mgr.subscribe('/v0/documents/ws', vi.fn())

      await expect(mgr.opened('/v0/documents/ws')).rejects.toThrow()
      expect(MockWebSocket.instances).toHaveLength(0)
    } finally {
      vi.doUnmock('@/lib/ws/url')
      vi.resetModules()
    }
  })
})
