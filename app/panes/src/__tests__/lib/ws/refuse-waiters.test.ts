import { describe, it, expect, vi } from 'vitest'
import { refuseWaiters } from '@/lib/ws/refuse-waiters'

describe('refuseWaiters', () => {
  it('rejects every waiter with the endpoint, the reason and the cause, and empties the list', () => {
    const reject = vi.fn()
    const waiters = [
      { resolve: vi.fn(), reject },
      { resolve: vi.fn(), reject },
    ]
    const cause = new Error('boom')

    refuseWaiters(waiters, '/v0/ws/x', 'gone', cause)

    expect(waiters).toHaveLength(0)
    expect(reject).toHaveBeenCalledTimes(2)
    const reason = reject.mock.calls[0][0] as Error
    expect(reason.message).toBe('/v0/ws/x: gone')
    expect(reason.cause).toBe(cause)
  })
})
