import { afterEach, describe, expect, it, vi } from 'vitest'
import { openShellSocket } from '@/lib/ws/shell-socket'

const sockets: { url: string; protocols: string[] }[] = []
class FakeWebSocket {
  constructor(
    public url: string,
    public protocols: string[],
  ) {
    sockets.push(this)
  }
}
vi.stubGlobal('WebSocket', FakeWebSocket)

function hostedByShell(ws: { url: string; token: string } | null) {
  const win = window as unknown as Record<string, unknown>
  win.crowbar = { post: vi.fn(), onmessage: null }
  win.__CROWBAR__ = { api: 'crowbar://local', host: 'local', ws }
}

afterEach(() => {
  const win = window as unknown as Record<string, unknown>
  delete win.crowbar
  delete win.__CROWBAR__
  sockets.length = 0
})

describe('openShellSocket', () => {
  it('dials the relay with the token in a subprotocol, never in the URL', () => {
    hostedByShell({ url: 'ws://127.0.0.1:41233', token: 'tok123' })
    openShellSocket('/v0/stream')
    expect(sockets).toEqual([
      {
        url: 'ws://127.0.0.1:41233/v0/stream',
        protocols: ['crowbar.relay.v1', 'crowbar.token.tok123'],
      },
    ])
    expect(sockets[0]!.url).not.toContain('tok123')
  })

  it('refuses when the shell issued no relay endpoint', () => {
    hostedByShell(null)
    expect(() => openShellSocket('/v0/stream')).toThrow(/no relay endpoint/)
  })
})
