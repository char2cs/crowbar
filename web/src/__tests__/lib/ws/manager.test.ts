import { describe, it, expect, vi, beforeEach } from 'vitest'

class MockWebSocket {
  static instances: MockWebSocket[] = []
  onmessage: ((e: MessageEvent) => void) | null = null
  onclose: (() => void) | null = null
  onopen: (() => void) | null = null
  onerror: (() => void) | null = null
  readyState: number = WebSocket.OPEN
  send = vi.fn()
  close = vi.fn()
  constructor(public url: string) { MockWebSocket.instances.push(this) }
  simulateMessage(data: string) { this.onmessage?.({ data } as MessageEvent) }
  simulateOpen() { this.onopen?.() }
  simulateClose() { this.readyState = WebSocket.CLOSED; this.onclose?.() }
}

beforeEach(() => { MockWebSocket.instances = [] })

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
      expect(cb).toHaveBeenCalledWith({ reconnected: true })
      MockWebSocket.instances[1].simulateMessage('{"after":"reconnect"}')
      expect(cb).toHaveBeenCalledWith({ after: 'reconnect' })
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
})
