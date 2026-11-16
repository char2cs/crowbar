import { describe, it, expect, vi, beforeEach } from 'vitest'

const state = vi.hoisted(() => ({ tauri: false, shell: false }))
vi.mock('@/lib/crowbar-bridge', () => ({ isTauri: () => state.tauri }))
vi.mock('@/lib/ws/in-shell', () => ({ inShell: () => state.shell }))
vi.mock('@/lib/ws/shell-socket', () => ({
  openShellSocket: vi.fn((path: string) => ({ shell: path })),
}))

class FakeNative {
  static instances: FakeNative[] = []
  constructor(public url: string) {
    FakeNative.instances.push(this)
  }
}
vi.stubGlobal('WebSocket', FakeNative)

const { createTransport } = await import('@/lib/ws/create-transport')
const { TauriWebSocket } = await import('@/lib/ws/tauri-transport')

const isLocal = (hostId: string) => hostId === 'local'

beforeEach(() => {
  state.tauri = false
  state.shell = false
  FakeNative.instances = []
})

describe('createTransport', () => {
  it('opens the native WebSocket in a browser', () => {
    createTransport('/v0/ws/x', isLocal)
    expect(FakeNative.instances).toHaveLength(1)
    expect(FakeNative.instances[0].url).toContain('/v0/ws/x')
  })

  it('opens the shell relay for a local endpoint in the shell', () => {
    state.shell = true
    expect(createTransport('/v0/ws/x', isLocal)).toEqual({ shell: '/v0/ws/x' })
  })

  it('has no relay for a remote host in the shell', () => {
    state.shell = true
    expect(() => createTransport('crowbar://box/v0/ws/x', isLocal)).toThrow(/no relay/)
  })

  it.each([['/v0/ws/x'], ['crowbar://box/v0/ws/x']])(
    'opens the unix-socket shim under Tauri for %s',
    (endpoint) => {
      state.tauri = true
      expect(createTransport(endpoint, isLocal)).toBeInstanceOf(TauriWebSocket)
    },
  )
})
