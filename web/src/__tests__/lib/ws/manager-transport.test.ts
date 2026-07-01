import { describe, it, expect, vi, beforeEach } from 'vitest'

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
  constructor(public url: string) {
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
  onerror: (() => void) | null = null
  readyState = MockTauriWebSocket.OPEN
  send = vi.fn()
  close = vi.fn()
  constructor(public path: string) {
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

const { createWSManager } = await import('@/lib/ws/manager')

describe('WSManager transport selection', () => {
  it('uses the native WebSocket in the browser', () => {
    isTauri.mockReturnValue(false)
    const mgr = createWSManager()
    mgr.subscribe('/v0/projects/p/repos/r/workspaces', vi.fn())
    expect(MockNativeWebSocket.instances).toHaveLength(1)
    expect(tauriInstances).toHaveLength(0)
  })

  it('uses the TauriWebSocket shim on desktop', () => {
    isTauri.mockReturnValue(true)
    const mgr = createWSManager()
    mgr.subscribe('/v0/projects/p/repos/r/workspaces', vi.fn())
    expect(tauriInstances).toHaveLength(1)
    expect(MockNativeWebSocket.instances).toHaveLength(0)
    expect(tauriInstances[0].path).toBe('/v0/projects/p/repos/r/workspaces')
  })

  it('delivers frames through the Tauri shim onmessage to subscribers', () => {
    isTauri.mockReturnValue(true)
    const mgr = createWSManager()
    const cb = vi.fn()
    mgr.subscribe('/v0/projects/p/repos/r/workspaces', cb)
    tauriInstances[0].onmessage?.({ data: '{"id":"ws-1"}' })
    expect(cb).toHaveBeenCalledWith({ id: 'ws-1' })
  })
})
