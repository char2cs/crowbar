import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { bindSocket } from '@/lib/ws/channel-handlers'
import { retryOf } from '@/lib/ws/open-channel'
import { fakeChannel, fakeContext, fakeSocket } from '@/__tests__/__fixtures__/ws-context'

function bound(resumed = false, over: Parameters<typeof fakeChannel>[0] = {}) {
  const socket = fakeSocket(WebSocket.CONNECTING)
  const ch = fakeChannel({ socket, ...over })
  const ctx = fakeContext({ channels: new Map([[ch.endpoint, ch]]) })
  bindSocket(ch, socket, ctx, resumed ? retryOf(ch, 1000) : undefined)
  return { socket, ch, ctx }
}

beforeEach(() => vi.useFakeTimers())
afterEach(() => vi.useRealTimers())

describe('bindSocket', () => {
  it('flushes the outbox, resolves waiters and resets the backoff on open', () => {
    const resolve = vi.fn()
    const { socket, ch } = bound(false, {
      outbox: ['a'],
      reconnectDelay: 8000,
      waiters: [{ resolve, reject: vi.fn() }],
    })
    socket.onopen?.()
    expect(socket.send).toHaveBeenCalledWith('a')
    expect(resolve).toHaveBeenCalledOnce()
    expect([ch.opened, ch.reconnectDelay]).toEqual([true, 1000])
  })

  it('tells subscribers a resumed stream is back, and a first open nothing', () => {
    const cb = vi.fn()
    const resumed = bound(true, { callbacks: new Set([cb]) })
    resumed.socket.onopen?.()
    expect(cb).toHaveBeenCalledWith({ reconnected: true })

    const cb2 = vi.fn()
    const first = bound(false, { callbacks: new Set([cb2]) })
    first.socket.onopen?.()
    expect(cb2).not.toHaveBeenCalled()
  })

  it.each([
    ['{"a":1}', { a: 1 }],
    ['plain', 'plain'],
  ])('delivers frame %s as %j', (raw, expected) => {
    const cb = vi.fn()
    const { socket } = bound(false, { callbacks: new Set([cb]) })
    socket.onmessage?.({ data: raw })
    expect(cb).toHaveBeenCalledWith(expected)
  })

  it('refuses waiters, tells drop listeners and reopens with doubled delay after a close', () => {
    const reject = vi.fn()
    const cb = vi.fn()
    const { socket, ch, ctx } = bound(false, {
      callbacks: new Set([cb]),
      opened: true,
      waiters: [{ resolve: vi.fn(), reject }],
    })
    const drop = vi.fn()
    ctx.dropListeners.set(ch.endpoint, new Set([drop]))

    socket.onclose?.()
    expect(reject).toHaveBeenCalledOnce()
    expect(drop).toHaveBeenCalledOnce()
    expect(ctx.reopen).not.toHaveBeenCalled()

    vi.advanceTimersByTime(1000)
    expect(ctx.reopen).toHaveBeenCalledWith(ch, 2000)
  })

  it('retires a channel whose subscribers left while it waited', () => {
    const { socket, ch, ctx } = bound(false, { callbacks: new Set([vi.fn()]) })
    socket.onclose?.()
    ch.callbacks.clear()
    vi.advanceTimersByTime(1000)
    expect(ctx.reopen).not.toHaveBeenCalled()
    expect(ctx.channels.has(ch.endpoint)).toBe(false)
  })
})
