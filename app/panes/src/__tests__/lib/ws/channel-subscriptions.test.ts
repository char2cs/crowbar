import { describe, it, expect, vi } from 'vitest'
import { subscribeTo } from '@/lib/ws/channel-subscriptions'
import { useConnectionStore } from '@/lib/ws/connection-store'
import { fakeChannel, fakeContext, fakePorts } from '@/__tests__/__fixtures__/ws-context'

describe('subscribeTo', () => {
  it('joins an existing channel and closes its socket with the last subscriber', () => {
    const ch = fakeChannel()
    const ctx = fakeContext({ channels: new Map([[ch.endpoint, ch]]) })
    const a = vi.fn()
    const b = vi.fn()
    const offA = subscribeTo(ctx, ch.endpoint, a)
    const offB = subscribeTo(ctx, ch.endpoint, b)

    offA()
    expect(ch.socket?.close).not.toHaveBeenCalled()
    offB()
    expect(ch.socket?.close).toHaveBeenCalledOnce()
    expect(ctx.channels.has(ch.endpoint)).toBe(false)
    expect(ctx.parked.unwatchIfNoneParked).toHaveBeenCalledWith(ch.hostId)
  })

  it('refuses the waiters of a channel retired by its last subscriber', () => {
    const reject = vi.fn()
    const ch = fakeChannel({ waiters: [{ resolve: vi.fn(), reject }] })
    const ctx = fakeContext({ channels: new Map([[ch.endpoint, ch]]) })
    subscribeTo(ctx, ch.endpoint, vi.fn())()
    expect(reject).toHaveBeenCalledOnce()
  })

  it.each([['/v0/ws/x'], ['/v0/ws/y']])('does not dial local %s when the install has no local daemon', (endpoint) => {
    const ctx = fakeContext({ ports: fakePorts({ hasLocalDaemon: () => false }) })
    const off = subscribeTo(ctx, endpoint, vi.fn())
    expect(ctx.channels.size).toBe(0)
    expect(useConnectionStore.getState().status).toBe('idle')
    off()
  })
})
