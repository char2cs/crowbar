import { describe, it, expect, vi, beforeEach } from 'vitest'

class MockWebSocket {
  // vi.stubGlobal replaces the real WebSocket, so the mock must carry the
  // standard readyState constants — production code compares against them.
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
  simulateOpen() {
    this.onopen?.()
  }
  simulateClose() {
    this.readyState = MockWebSocket.CLOSED
    this.onclose?.()
  }
}

vi.stubGlobal('WebSocket', MockWebSocket)

const { createWSManager } = await import('@/lib/ws/manager')
const { useConnectionStore, resetConnectionState, reportChannelState, reportDaemonRefusal } =
  await import('@/lib/ws/connection-store')
const { useDaemonHaltStore } = await import('@/features/window/stores/daemon-halt-store')

const status = () => useConnectionStore.getState().status

beforeEach(() => {
  MockWebSocket.instances = []
  resetConnectionState()
  useDaemonHaltStore.getState().clear()
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

  it('a daemon_migrating refusal is a state, not a drop', () => {
    reportChannelState('/a', false)
    reportDaemonRefusal({ code: 'daemon_migrating', message: 'Crowbar is starting' })

    expect(status()).toBe('migrating')
    expect(useConnectionStore.getState().migrating).toEqual({ message: 'Crowbar is starting' })
  })

  it('a daemon_halted refusal halts the daemon-halt store with its reason', () => {
    reportChannelState('/a', false)
    reportDaemonRefusal({ code: 'daemon_halted', message: 'layout 9 is newer' })

    expect(useDaemonHaltStore.getState().message).toBe('layout 9 is newer')
    expect(status()).toBe('disconnected')
  })

  it('an open socket clears the migrating state', () => {
    reportChannelState('/a', false)
    reportDaemonRefusal({ code: 'daemon_migrating', message: 'Crowbar is starting' })
    reportChannelState('/a', true)

    expect(status()).toBe('connected')
    expect(useConnectionStore.getState().migrating).toBeNull()
    reportChannelState('/a', false)
    expect(status()).toBe('disconnected')
  })

  it('a refusal while a socket is open does not change the status', () => {
    reportChannelState('/a', true)
    reportDaemonRefusal({ code: 'daemon_migrating', message: 'Crowbar is starting' })

    expect(status()).toBe('connected')
    expect(useConnectionStore.getState().migrating).toBeNull()
  })
})
