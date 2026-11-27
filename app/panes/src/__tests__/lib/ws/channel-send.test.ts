import { describe, it, expect } from 'vitest'
import { sendOn } from '@/lib/ws/channel-send'
import { fakeChannel, fakeContext, fakeSocket } from '@/__tests__/__fixtures__/ws-context'

function contextWith(readyState: number | null) {
  const ch = fakeChannel({ socket: readyState === null ? null : fakeSocket(readyState) })
  return { ch, ctx: fakeContext({ channels: new Map([[ch.endpoint, ch]]) }) }
}

describe('sendOn', () => {
  it('sends JSON on an open socket', () => {
    const { ch, ctx } = contextWith(WebSocket.OPEN)
    sendOn(ctx, ch.endpoint, { a: 1 })
    expect(ch.socket?.send).toHaveBeenCalledWith('{"a":1}')
  })

  it('queues while the socket connects', () => {
    const { ch, ctx } = contextWith(WebSocket.CONNECTING)
    sendOn(ctx, ch.endpoint, { a: 1 })
    expect(ch.outbox).toEqual(['{"a":1}'])
    expect(ch.socket?.send).not.toHaveBeenCalled()
  })

  it.each([[null], [WebSocket.CLOSED]])('drops it for a socket in state %s', (state) => {
    const { ch, ctx } = contextWith(state)
    sendOn(ctx, ch.endpoint, { a: 1 })
    expect(ch.outbox).toEqual([])
  })

  it('ignores an endpoint with no channel', () => {
    expect(() => sendOn(fakeContext(), '/none', 1)).not.toThrow()
  })
})
