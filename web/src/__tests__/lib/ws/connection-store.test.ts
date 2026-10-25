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
const { useConnectionStore, resetConnectionState, reportChannelState, reportChannelGone } =
  await import('@/lib/ws/connection-store')

const status = () => useConnectionStore.getState().status
const reconnects = () => useConnectionStore.getState().reconnects
const degraded = () => useConnectionStore.getState().degraded

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

  it('counts a reconnect only when the backend comes back after being lost', () => {
    vi.useFakeTimers()
    try {
      const mgr = createWSManager()
      mgr.subscribe('/v0/ws/git', vi.fn())
      MockWebSocket.instances[0].simulateOpen()
      expect(reconnects()).toBe(0) // the first connect is not a reconnect

      MockWebSocket.instances[0].simulateClose()
      vi.advanceTimersByTime(1000)
      MockWebSocket.instances[1].simulateOpen()
      expect(reconnects()).toBe(1)

      MockWebSocket.instances[1].simulateClose()
      vi.advanceTimersByTime(2000)
      MockWebSocket.instances[2].simulateOpen()
      expect(reconnects()).toBe(2)
    } finally {
      vi.useRealTimers()
    }
  })

  it('a channel dropping while another stays open is degraded, not a lost backend', () => {
    vi.useFakeTimers()
    try {
      const mgr = createWSManager()
      mgr.subscribe('/v0/ws/git', vi.fn())
      mgr.subscribe('/v0/ws/files', vi.fn())
      MockWebSocket.instances[0].simulateOpen()
      MockWebSocket.instances[1].simulateOpen()
      expect(degraded()).toBe(false)

      MockWebSocket.instances[0].simulateClose()
      expect(degraded()).toBe(true)
      expect(status()).toBe('connected')

      vi.advanceTimersByTime(1000)
      MockWebSocket.instances[2].simulateOpen()
      expect(degraded()).toBe(false)
      expect(reconnects()).toBe(0) // the backend was never lost
    } finally {
      vi.useRealTimers()
    }
  })
})

// Whether a remote host's live streams are open: not whether the host is up (its status
// says that), but whether what the app shows of it is being kept current.
describe('live streams per remote host', () => {
  const remoteStatus = (hostId: string) => useConnectionStore.getState().hostStreams[hostId]

  it('tracks a remote host’s channels apart from the local backend', () => {
    reportChannelState('crowbar://h1/v0/projects', true, 'h1')
    expect(remoteStatus('h1')).toBe('connected')
    expect(status()).toBe('idle')

    reportChannelState('crowbar://h1/v0/projects', false, 'h1')
    expect(remoteStatus('h1')).toBe('disconnected')
    expect(status()).toBe('idle')

    reportChannelGone('crowbar://h1/v0/projects')
    expect(remoteStatus('h1')).toBeUndefined()
  })

  it('keeps one host’s channels from speaking for another’s', () => {
    reportChannelState('crowbar://h1/v0/projects', true, 'h1')
    reportChannelState('crowbar://h2/v0/projects', false, 'h2')
    expect(remoteStatus('h1')).toBe('connected')
    expect(remoteStatus('h2')).toBe('disconnected')
  })
})
