import { describe, it, expect, vi } from 'vitest'
import { closeSocketQuietly } from '@/lib/ws/close-socket-quietly'
import { fakeSocket } from '@/__tests__/__fixtures__/ws-context'

describe('closeSocketQuietly', () => {
  it.each([[WebSocket.OPEN], [WebSocket.CLOSED]])(
    'closes a socket in state %i at once',
    (state) => {
      const socket = fakeSocket(state)
      closeSocketQuietly(socket)
      expect(socket.close).toHaveBeenCalledOnce()
    },
  )

  it('defers closing a connecting socket until it opens', () => {
    const socket = fakeSocket(WebSocket.CONNECTING)
    const addEventListener = vi.fn()
    socket.addEventListener = addEventListener
    closeSocketQuietly(socket)
    expect(socket.close).not.toHaveBeenCalled()
    const [type, listener, options] = addEventListener.mock.calls[0]
    expect([type, options]).toEqual(['open', { once: true }])
    listener()
    expect(socket.close).toHaveBeenCalledOnce()
  })

  it('closes a connecting socket that cannot listen', () => {
    const socket = fakeSocket(WebSocket.CONNECTING)
    closeSocketQuietly(socket)
    expect(socket.close).toHaveBeenCalledOnce()
  })
})
