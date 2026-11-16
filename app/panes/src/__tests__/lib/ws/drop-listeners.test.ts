import { describe, it, expect, vi } from 'vitest'
import { notifyDrop, onDrop } from '@/lib/ws/drop-listeners'
import { fakeContext } from '@/__tests__/__fixtures__/ws-context'

describe('drop listeners', () => {
  it('tells each listener of the endpoint, and none of another', () => {
    const ctx = fakeContext()
    const mine = vi.fn()
    const other = vi.fn()
    onDrop(ctx, '/a', mine)
    onDrop(ctx, '/b', other)

    notifyDrop(ctx, '/a')

    expect(mine).toHaveBeenCalledOnce()
    expect(other).not.toHaveBeenCalled()
  })

  it('stops telling a listener once it is removed and forgets an empty endpoint', () => {
    const ctx = fakeContext()
    const listener = vi.fn()
    const off = onDrop(ctx, '/a', listener)

    off()
    notifyDrop(ctx, '/a')

    expect(listener).not.toHaveBeenCalled()
    expect(ctx.dropListeners.has('/a')).toBe(false)
  })
})
