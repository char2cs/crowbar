import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const tauri = vi.hoisted(() => ({
  instances: [] as Array<{ path: string; options?: { idleTimeoutMs?: number } }>,
}))

// The desktop transport: a WebSocket-shaped shim over the Rust bridge.
vi.mock('@/lib/ws/tauri-transport', () => ({
  TauriWebSocket: class {
    onopen: (() => void) | null = null
    onmessage: ((e: { data: unknown }) => void) | null = null
    onclose: (() => void) | null = null
    send = vi.fn()
    close = vi.fn()
    constructor(path: string, options?: { idleTimeoutMs?: number }) {
      tauri.instances.push({ path, options })
    }
  },
}))

import {
  openTerminal,
  terminalKill,
  terminalListLive,
  TerminalConnection,
  type TerminalFrame,
} from '@/lib/crowbar-bridge'

// A socket the test drives by hand.
function fakeSocket() {
  const socket = {
    onopen: null as (() => void) | null,
    onmessage: null as ((e: { data: string | ArrayBuffer }) => void) | null,
    onclose: null as (() => void) | null,
    sent: [] as string[],
    send(data: string) {
      socket.sent.push(data)
    },
    close: vi.fn(),
    open() {
      socket.onopen?.()
    },
    output(tag: number, text: string) {
      const bytes = new TextEncoder().encode(text)
      const buf = new Uint8Array(bytes.length + 1)
      buf[0] = tag
      buf.set(bytes, 1)
      socket.onmessage?.({ data: buf.buffer })
    },
    exit(code: number) {
      socket.onmessage?.({ data: JSON.stringify({ type: 'exit', code }) })
    },
    drop() {
      socket.onclose?.()
    },
  }
  return socket
}

const text = (frame: TerminalFrame) =>
  frame.exit ? `exit:${frame.code}` : new TextDecoder().decode(frame.data)

describe('TerminalConnection', () => {
  it('decodes binary output and snapshot frames, holding them for the first listener', () => {
    const socket = fakeSocket()
    const conn = new TerminalConnection('s1', '/x', socket)
    socket.open()
    socket.output(1, 'REDRAW')
    socket.output(0, 'tail ✓')

    const frames: TerminalFrame[] = []
    conn.listen((f) => frames.push(f))
    expect(frames.map(text)).toEqual(['REDRAW', 'tail ✓'])
    expect(frames.map((f) => !f.exit && f.snapshot)).toEqual([true, false])
  })

  // B4: the exit frame is the only thing that ends a terminal; the close that
  // follows it is not a drop, so nothing reconnects an exited session.
  it('delivers the exit frame in order and does not report the following close as a drop', () => {
    const socket = fakeSocket()
    const conn = new TerminalConnection('s1', '/x', socket)
    const frames: TerminalFrame[] = []
    conn.listen((f) => frames.push(f))
    const drop = vi.fn()
    conn.onDrop(drop)
    socket.open()
    socket.output(0, 'bye')
    socket.exit(3)
    socket.drop()

    expect(frames.map(text)).toEqual(['bye', 'exit:3'])
    expect(drop).not.toHaveBeenCalled()
    expect(conn.state).toBe('exited')
    expect(conn.alive).toBe(false)
  })

  it('a close with no exit frame is a transport drop, reported once', () => {
    const socket = fakeSocket()
    const conn = new TerminalConnection('s1', '/x', socket)
    const drop = vi.fn()
    conn.onDrop(drop)
    socket.open()
    socket.drop()
    socket.drop()
    expect(drop).toHaveBeenCalledOnce()
    expect(conn.state).toBe('dropped')
  })

  it('queues input sent before the socket opens and flushes it in order; a theme coalesces', () => {
    const socket = fakeSocket()
    const conn = new TerminalConnection('s1', '/x', socket)
    conn.write('a')
    conn.resize(40, 120)
    conn.setTheme({ background: '#000', foreground: '#fff', dark: true })
    conn.setTheme({ background: '#fff', foreground: '#000', dark: false })
    conn.write('b')
    expect(socket.sent).toEqual([])

    socket.open()
    expect(socket.sent.map((s) => JSON.parse(s))).toEqual([
      { data: 'a' },
      { type: 'resize', cols: 120, rows: 40 },
      { data: 'b' },
      { type: 'theme', bg: '#fff', fg: '#000', dark: false },
    ])
  })

  it('close() detaches: the socket closes, nothing is reported, later frames are ignored', () => {
    const socket = fakeSocket()
    const conn = new TerminalConnection('s1', '/x', socket)
    const drop = vi.fn()
    const frames: TerminalFrame[] = []
    conn.listen((f) => frames.push(f))
    conn.onDrop(drop)
    socket.open()
    conn.close()
    socket.drop()
    socket.output(0, 'late')
    conn.write('ignored')

    expect(socket.close).toHaveBeenCalledOnce()
    expect(drop).not.toHaveBeenCalled()
    expect(frames).toEqual([])
    expect(socket.sent).toEqual([])
  })
})

class FakeWebSocket {
  static instances: FakeWebSocket[] = []
  binaryType = 'blob'
  onopen: (() => void) | null = null
  onmessage: ((e: { data: unknown }) => void) | null = null
  onclose: (() => void) | null = null
  constructor(public url: string) {
    FakeWebSocket.instances.push(this)
  }
  send = vi.fn()
  close = vi.fn()
}

describe('openTerminal / terminalKill / terminalListLive', () => {
  beforeEach(() => {
    FakeWebSocket.instances = []
    tauri.instances = []
    vi.stubGlobal('WebSocket', FakeWebSocket as unknown as typeof WebSocket)
  })
  afterEach(() => {
    vi.unstubAllGlobals()
    delete (window as unknown as Record<string, unknown>).__TAURI_INTERNALS__
  })

  it('dials the session under its base, receiving binary frames as ArrayBuffers', () => {
    openTerminal('s 1', '/v0/chats/c1/terminals')
    expect(FakeWebSocket.instances[0].url).toContain('/v0/chats/c1/terminals/s%201/ws')
    expect(FakeWebSocket.instances[0].binaryType).toBe('arraybuffer')
  })

  it('each view gets its own transport, so closing one never touches another', () => {
    const a = openTerminal('s1', '/b')
    const b = openTerminal('s1', '/b')
    expect(FakeWebSocket.instances).toHaveLength(2)
    a.close()
    expect(FakeWebSocket.instances[0].close).toHaveBeenCalled()
    expect(FakeWebSocket.instances[1].close).not.toHaveBeenCalled()
    expect(b.alive).toBe(true)
  })

  it('on desktop, dials through the Rust bridge with the half-open idle timeout armed', () => {
    ;(window as unknown as Record<string, unknown>).__TAURI_INTERNALS__ = {}
    openTerminal('s1', '/v0/chats/c1/terminals')
    expect(FakeWebSocket.instances).toHaveLength(0)
    expect(tauri.instances[0].path).toBe('/v0/chats/c1/terminals/s1/ws')
    expect(tauri.instances[0].options?.idleTimeoutMs).toBeGreaterThan(45_000)
  })

  it('kills under the base the session was opened on; an unknown session sends nothing', async () => {
    const fetchSpy = vi.fn().mockResolvedValue({
      ok: true,
      status: 202,
      json: async () => ({ success: true, error: null, data: { id: 's9' } }),
    })
    vi.stubGlobal('fetch', fetchSpy)
    openTerminal('s9', '/v0/chats/c9/terminals')
    await terminalKill('s9')
    expect(String(fetchSpy.mock.calls[0][0])).toContain('/v0/chats/c9/terminals/s9')
    expect(fetchSpy.mock.calls[0][1]).toMatchObject({ method: 'DELETE' })

    fetchSpy.mockClear()
    await terminalKill('never-seen')
    expect(fetchSpy).not.toHaveBeenCalled()
  })

  it('lists live session ids from the one DTO shape every terminals route serves', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        status: 200,
        json: async () => ({
          success: true,
          error: null,
          data: [
            { id: 'a', status: 'active' },
            { id: 'b', status: 'ended' },
            { id: 'c', status: 'suspended' },
          ],
        }),
      }),
    )
    await expect(terminalListLive('/v0/chats/c1/terminals')).resolves.toEqual(['a', 'c'])
  })
})
