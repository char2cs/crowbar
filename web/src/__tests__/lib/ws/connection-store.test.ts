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
  simulateOpen() { this.onopen?.() }
  simulateClose() { this.readyState = WebSocket.CLOSED; this.onclose?.() }
}

vi.stubGlobal('WebSocket', MockWebSocket)

const { createWSManager } = await import('@/lib/ws/manager')
const { useConnectionStore, resetConnectionState } = await import('@/lib/ws/connection-store')

const status = () => useConnectionStore.getState().status

beforeEach(() => {
  MockWebSocket.instances = []
  resetConnectionState()
})

describe('connection store', () => {
  it('starts idle and becomes connected when a channel opens', () => {
    const mgr = createWSManager()
    expect(status()).toBe('idle')
    mgr.subscribe('/v0/ws/git', vi.fn())
    expect(status()).toBe('idle') // connecting, not yet reported
    MockWebSocket.instances[0].simulateOpen()
    expect(status()).toBe('connected')
  })

  it('goes disconnected when every channel is closed, connected when any is open', () => {
    const mgr = createWSManager()
    mgr.subscribe('/v0/ws/git', vi.fn())
    mgr.subscribe('/v0/ws/files', vi.fn())
    MockWebSocket.instances[0].simulateOpen()
    MockWebSocket.instances[1].simulateOpen()
    expect(status()).toBe('connected')

    MockWebSocket.instances[0].simulateClose()
    expect(status()).toBe('connected') // one channel still alive

    MockWebSocket.instances[1].simulateClose()
    expect(status()).toBe('disconnected') // backend gone
  })

  it('returns to connected after a successful reconnect', () => {
    vi.useFakeTimers()
    try {
      const mgr = createWSManager()
      mgr.subscribe('/v0/ws/git', vi.fn())
      MockWebSocket.instances[0].simulateOpen()
      MockWebSocket.instances[0].simulateClose()
      expect(status()).toBe('disconnected')

      vi.advanceTimersByTime(1000)
      expect(MockWebSocket.instances).toHaveLength(2)
      expect(status()).toBe('disconnected') // still connecting
      MockWebSocket.instances[1].simulateOpen()
      expect(status()).toBe('connected')
    } finally {
      vi.useRealTimers()
    }
  })

  it('returns to idle when the last subscriber leaves', () => {
    const mgr = createWSManager()
    const unsub = mgr.subscribe('/v0/ws/git', vi.fn())
    MockWebSocket.instances[0].simulateOpen()
    expect(status()).toBe('connected')
    unsub()
    expect(status()).toBe('idle')
  })

  it('a stale socket closing after reconnect does not flag the endpoint as down', () => {
    vi.useFakeTimers()
    try {
      const mgr = createWSManager()
      mgr.subscribe('/v0/ws/git', vi.fn())
      MockWebSocket.instances[0].simulateClose()
      vi.advanceTimersByTime(1000)
      MockWebSocket.instances[1].simulateOpen()
      expect(status()).toBe('connected')
      // Late close event from the replaced socket must be ignored.
      MockWebSocket.instances[0].simulateClose()
      expect(status()).toBe('connected')
    } finally {
      vi.useRealTimers()
    }
  })
})
