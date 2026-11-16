import { describe, it, expect, vi } from 'vitest'

class FakeNative {
  static instances: FakeNative[] = []
  readyState = 0
  constructor(public url: string) {
    FakeNative.instances.push(this)
  }
}
vi.stubGlobal('WebSocket', FakeNative)

const { openChannel, retryOf } = await import('@/lib/ws/open-channel')
const { fakeChannel, fakeContext, fakePorts } = await import('@/__tests__/__fixtures__/ws-context')

describe('openChannel', () => {
  it('dials a dialable endpoint and registers the channel', () => {
    const ctx = fakeContext()
    const ch = openChannel(ctx, '/v0/ws/x')
    expect(ch.socket).not.toBeNull()
    expect(ctx.channels.get('/v0/ws/x')).toBe(ch)
  })

  it.each([
    ['crowbar://box/v0/ws/x', 'box'],
    ['crowbar://other/v0/ws/y', 'other'],
  ])('parks %s while its host may not be dialled and watches the host', (endpoint, hostId) => {
    const ctx = fakeContext({ ports: fakePorts({ mayDial: () => false }) })
    const ch = openChannel(ctx, endpoint)
    expect(ch.socket).toBeNull()
    expect(ctx.parked.watch).toHaveBeenCalledWith(hostId)
  })

  it('takes subscribers and waiters over from a retry', () => {
    const old = fakeChannel({
      callbacks: new Set([vi.fn()]),
      waiters: [{ resolve: vi.fn(), reject: vi.fn() }],
    })
    const ch = openChannel(fakeContext(), old.endpoint, retryOf(old, 4000))
    expect(ch.callbacks).toBe(old.callbacks)
    expect(ch.waiters).toHaveLength(1)
    expect(old.waiters).toHaveLength(0)
    expect(ch.reconnectDelay).toBe(4000)
  })
})
