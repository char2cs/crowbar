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

function relayLookup(status: number, body: string) {
  class FakeRequest {
    status = status
    responseText = body
    open = vi.fn()
    send = vi.fn()
  }
  vi.stubGlobal('XMLHttpRequest', FakeRequest)
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.stubGlobal('WebSocket', FakeWebSocket)
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

  it('asks the shell for a new token on every connect so a long-open pane can reconnect', () => {
    hostedByShell({ url: 'ws://127.0.0.1:41233', token: 'expired' })
    relayLookup(200, '{"url":"ws://127.0.0.1:41233","token":"renewed"}')
    openShellSocket('/v0/stream')
    expect(sockets[0]!.protocols).toEqual(['crowbar.relay.v1', 'crowbar.token.renewed'])
  })

  it('falls back to the cached endpoint when the lookup fails', () => {
    hostedByShell({ url: 'ws://127.0.0.1:41233', token: 'cached' })
    relayLookup(503, '')
    openShellSocket('/v0/stream')
    expect(sockets[0]!.protocols).toEqual(['crowbar.relay.v1', 'crowbar.token.cached'])
  })
})
