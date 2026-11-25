import { afterEach, describe, expect, it, vi } from 'vitest'
import { ShellWebSocket } from '@/lib/ws/shell-web-socket'

class FakeWebSocket {
  static instances: FakeWebSocket[] = []
  binaryType = 'blob'
  onopen: (() => void) | null = null
  onmessage: ((event: { data: unknown }) => void) | null = null
  onclose: (() => void) | null = null
  onerror: ((event: unknown) => void) | null = null
  send = vi.fn()
  close = vi.fn()
  constructor(
    public url: string,
    public protocols: string[],
  ) {
    FakeWebSocket.instances.push(this)
  }
}
vi.stubGlobal('WebSocket', FakeWebSocket)

function hostedByShell(ws: { url: string; token: string } | null) {
  const win = window as unknown as Record<string, unknown>
  win.crowbar = { post: vi.fn(), onmessage: null }
  win.__CROWBAR__ = { api: 'crowbar://local', host: 'local', ws }
}

const RELAY = '{"url":"ws://127.0.0.1:41233","token":"tok"}'

function relayLookup(status: number, body: string) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => new Response(body, { status })),
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.stubGlobal('WebSocket', FakeWebSocket)
  const win = window as unknown as Record<string, unknown>
  delete win.crowbar
  delete win.__CROWBAR__
  FakeWebSocket.instances = []
})

async function dialled(): Promise<FakeWebSocket> {
  await vi.waitFor(() => expect(FakeWebSocket.instances).toHaveLength(1))
  return FakeWebSocket.instances[0]!
}

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
    behaviour: 'falls back to the cached endpoint when the lookup cannot be reached',
    cached: 'cached',
    token: 'cached',
  },
]

const refusals: { behaviour: string; lookup: Lookup }[] = [
  {
    behaviour: 'a refused lookup does not fall back to a cached token',
    lookup: { status: 503, body: '' },
  },
  {
    behaviour: 'an answer without a url is no relay',
    lookup: { status: 200, body: '{"token":"t"}' },
  },
  {
    behaviour: 'an answer with a non-string token is no relay',
    lookup: { status: 200, body: '{"url":"ws://h","token":7}' },
  },
  {
    behaviour: 'an answer that is not an object is no relay',
    lookup: { status: 200, body: 'null' },
  },
]

describe('ShellWebSocket', () => {
  it.each(dials)('$behaviour', async ({ cached, lookup, token }) => {
    hostedByShell({ url: 'ws://127.0.0.1:41233', token: cached })
    const answer = async () =>
      lookup
        ? new Response(lookup.body, { status: lookup.status })
        : Promise.reject(new Error('offline'))
    vi.stubGlobal('fetch', vi.fn(answer))
    new ShellWebSocket('/v0/stream', 'local')
    const socket = await dialled()
    expect(socket.url).toBe('ws://127.0.0.1:41233/v0/stream')
    expect(socket.protocols).toEqual(['crowbar.relay.v1', `crowbar.token.${token}`])
    expect(socket.url).not.toContain(token)
  })

  it.each(refusals)('$behaviour', async ({ lookup }) => {
    hostedByShell({ url: 'ws://127.0.0.1:41233', token: 'expired' })
    relayLookup(lookup.status, lookup.body)
    const seen: string[] = []
    const shell = new ShellWebSocket('/v0/stream', 'local')
    shell.onerror = () => seen.push('error')
    shell.onclose = () => seen.push('close')
    await vi.waitFor(() => expect(seen).toEqual(['error', 'close']))
    expect(FakeWebSocket.instances).toHaveLength(0)
  })

  it('reports an error then a close when the shell issued no relay endpoint', async () => {
    hostedByShell(null)
    const seen: string[] = []
    const shell = new ShellWebSocket('/v0/stream', 'local')
    shell.onerror = () => seen.push('error')
    shell.onclose = () => seen.push('close')
    await vi.waitFor(() => expect(seen).toEqual(['error', 'close']))
    expect(shell.readyState).toBe(3)
    expect(FakeWebSocket.instances).toHaveLength(0)
  })

  it("asks the named host's relay and never dials it with this host's cached token", async () => {
    hostedByShell({ url: 'ws://127.0.0.1:41233', token: 'local-token' })
    const lookup = vi.fn(async () => Promise.reject(new Error('offline')))
    vi.stubGlobal('fetch', lookup)
    const seen: string[] = []
    const shell = new ShellWebSocket('/v0/stream', 'box')
    shell.onclose = () => seen.push('close')
    await vi.waitFor(() => expect(seen).toEqual(['close']))
    expect(lookup).toHaveBeenCalledWith('crowbar://box/__crowbar/relay')
    expect(FakeWebSocket.instances).toHaveLength(0)
  })

  it("dials the named host's relay with the token it issued", async () => {
    hostedByShell({ url: 'ws://127.0.0.1:41233', token: 'local-token' })
    relayLookup(200, '{"url":"ws://127.0.0.1:41299","token":"box-token"}')
    new ShellWebSocket('/v0/stream', 'box')
    const socket = await dialled()
    expect(socket.url).toBe('ws://127.0.0.1:41299/v0/stream')
    expect(socket.protocols).toEqual(['crowbar.relay.v1', 'crowbar.token.box-token'])
    const config = (window as unknown as { __CROWBAR__: { ws: { token: string } } }).__CROWBAR__
    expect(config.ws.token, "this host's cached relay").toBe('local-token')
  })

  it('forwards the native socket once it is dialled', async () => {
    hostedByShell({ url: 'ws://127.0.0.1:41233', token: 'tok' })
    relayLookup(200, RELAY)
    const shell = new ShellWebSocket('/v0/stream', 'local')
    shell.binaryType = 'arraybuffer'
    const received: unknown[] = []
    const lifecycle: string[] = []
    shell.onopen = () => lifecycle.push('open')
    shell.onmessage = (event) => received.push(event.data)
    shell.onclose = () => lifecycle.push('close')
    const socket = await dialled()
    expect(socket.binaryType).toBe('arraybuffer')
    expect(shell.readyState).toBe(0)
    socket.onopen?.()
    expect(shell.readyState).toBe(1)
    socket.onmessage?.({ data: 'frame' })
    shell.send('out')
    expect(socket.send).toHaveBeenCalledWith('out')
    shell.close()
    expect(socket.close).toHaveBeenCalled()
    socket.onclose?.()
    expect(lifecycle).toEqual(['open', 'close'])
    expect(received).toEqual(['frame'])
    expect(shell.readyState).toBe(3)
  })

  it('refuses to send before the relay is dialled', () => {
    hostedByShell({ url: 'ws://127.0.0.1:41233', token: 'tok' })
    relayLookup(200, RELAY)
    expect(() => new ShellWebSocket('/v0/stream', 'local').send('early')).toThrow(/not connected/)
  })

  it('never dials when closed before the relay answers', async () => {
    hostedByShell({ url: 'ws://127.0.0.1:41233', token: 'tok' })
    const lookup = vi.fn(async () => new Response(RELAY, { status: 200 }))
    vi.stubGlobal('fetch', lookup)
    const shell = new ShellWebSocket('/v0/stream', 'local')
    shell.close()
    await vi.waitFor(() => expect(lookup).toHaveBeenCalled())
    await lookup.mock.results[0]!.value
    expect(shell.readyState).toBe(3)
    expect(FakeWebSocket.instances).toHaveLength(0)
  })
})
