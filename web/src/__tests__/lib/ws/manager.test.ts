import { describe, it, expect, vi, beforeEach } from 'vitest'

class MockWebSocket {
  static instances: MockWebSocket[] = []
  onmessage: ((e: MessageEvent) => void) | null = null
  onclose: (() => void) | null = null
  onopen: (() => void) | null = null
  onerror: (() => void) | null = null
  readyState = WebSocket.OPEN
  send = vi.fn()
  close = vi.fn()
  constructor(public url: string) { MockWebSocket.instances.push(this) }
  simulateMessage(data: string) { this.onmessage?.({ data } as MessageEvent) }
  simulateOpen() { this.onopen?.() }
}

beforeEach(() => { MockWebSocket.instances = [] })

vi.stubGlobal('WebSocket', MockWebSocket)

const { createWSManager } = await import('@/lib/ws/manager')

describe('WSManager', () => {
  it('opens one socket for two subscribers to the same endpoint', () => {
    const mgr = createWSManager()
    const cb1 = vi.fn()
    const cb2 = vi.fn()
    mgr.subscribe('/api/v0/ws/git', cb1)
    mgr.subscribe('/api/v0/ws/git', cb2)
    expect(MockWebSocket.instances).toHaveLength(1)
  })

  it('calls all subscribers when a message arrives', () => {
    const mgr = createWSManager()
    const cb1 = vi.fn()
    const cb2 = vi.fn()
    mgr.subscribe('/api/v0/ws/git', cb1)
    mgr.subscribe('/api/v0/ws/git', cb2)
    MockWebSocket.instances[0].simulateMessage('{"changed":true}')
    expect(cb1).toHaveBeenCalledWith({ changed: true })
    expect(cb2).toHaveBeenCalledWith({ changed: true })
  })

  it('closes socket when last subscriber unsubscribes', () => {
    const mgr = createWSManager()
    const unsub1 = mgr.subscribe('/api/v0/ws/git', vi.fn())
    const unsub2 = mgr.subscribe('/api/v0/ws/git', vi.fn())
    unsub1()
    expect(MockWebSocket.instances[0].close).not.toHaveBeenCalled()
    unsub2()
    expect(MockWebSocket.instances[0].close).toHaveBeenCalled()
  })
})
