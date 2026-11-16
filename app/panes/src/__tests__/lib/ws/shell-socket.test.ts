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

type Lookup = { status: number; body: string }
type Dial = { behaviour: string; cached: string; lookup?: Lookup; token: string }

const dials: Dial[] = [
  {
    behaviour: 'dials the relay with the token in a subprotocol',
    cached: 'tok123',
    token: 'tok123',
  },
  {
    behaviour: 'asks the shell for a new token on every connect so a long-open pane can reconnect',
    cached: 'expired',
    lookup: { status: 200, body: '{"url":"ws://127.0.0.1:41233","token":"renewed"}' },
    token: 'renewed',
  },
  {
    behaviour: 'falls back to the cached endpoint when the lookup fails',
    cached: 'cached',
    lookup: { status: 503, body: '' },
    token: 'cached',
  },
]

describe('openShellSocket', () => {
  it.each(dials)('$behaviour', ({ cached, lookup, token }) => {
    hostedByShell({ url: 'ws://127.0.0.1:41233', token: cached })
    if (lookup) relayLookup(lookup.status, lookup.body)
    openShellSocket('/v0/stream')
    expect(sockets).toEqual([
      {
        url: 'ws://127.0.0.1:41233/v0/stream',
        protocols: ['crowbar.relay.v1', `crowbar.token.${token}`],
      },
    ])
    expect(sockets[0]!.url).not.toContain(token)
  })

  it('refuses when the shell issued no relay endpoint', () => {
    hostedByShell(null)
    expect(() => openShellSocket('/v0/stream')).toThrow(/no relay endpoint/)
  })
})
