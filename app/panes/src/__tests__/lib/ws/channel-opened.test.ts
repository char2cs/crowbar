import { describe, it, expect } from 'vitest'
import { openedOn } from '@/lib/ws/channel-opened'
import { fakeChannel, fakeContext, fakeSocket } from '@/__tests__/__fixtures__/ws-context'

describe('openedOn', () => {
  it('rejects for an endpoint with no channel', async () => {
    await expect(openedOn(fakeContext(), '/none')).rejects.toThrow('no socket can carry it')
  })

  it.each([[WebSocket.OPEN]])('settles at once for a socket in state %i', async (state) => {
    const ch = fakeChannel({ socket: fakeSocket(state) })
    const ctx = fakeContext({ channels: new Map([[ch.endpoint, ch]]) })
    await expect(openedOn(ctx, ch.endpoint)).resolves.toBeUndefined()
  })

  it('waits on a socket that is not open yet', async () => {
    const ch = fakeChannel({ socket: fakeSocket(WebSocket.CONNECTING) })
    const ctx = fakeContext({ channels: new Map([[ch.endpoint, ch]]) })
    const pending = openedOn(ctx, ch.endpoint)
    expect(ch.waiters).toHaveLength(1)
    ch.waiters[0].resolve()
    await expect(pending).resolves.toBeUndefined()
  })
})
