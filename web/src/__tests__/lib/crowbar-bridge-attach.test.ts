import { describe, it, expect, vi, beforeEach } from 'vitest'
// Browser path is the jsdom default (no __TAURI_INTERNALS__); no mock needed.
import {
  terminalAttach,
  terminalListen,
  terminalHasTransport,
  onTransportDrop,
  __getBridgeInternals,
  type TerminalFrame,
} from '@/lib/crowbar-bridge'

class FakeWebSocket {
  static instances: FakeWebSocket[] = []
  onopen: (() => void) | null = null
  onmessage: ((e: { data: string }) => void) | null = null
  onclose: (() => void) | null = null
  constructor(public url: string) {
    FakeWebSocket.instances.push(this)
    queueMicrotask(() => this.onopen?.())
  }
  send = vi.fn()
  close = vi.fn()
}
beforeEach(() => {
  FakeWebSocket.instances = []
  vi.stubGlobal('WebSocket', FakeWebSocket as unknown as typeof WebSocket)
})

describe('terminalAttach', () => {
  it('opens a WS to the existing session path without POSTing and registers the transport', async () => {
    const fetchSpy = vi.fn()
    vi.stubGlobal('fetch', fetchSpy)
    const base = '/v0/chats/chat-1/terminals'

    await terminalAttach('conn-1', base)

    expect(fetchSpy).not.toHaveBeenCalled() // no POST
    expect(FakeWebSocket.instances[0].url).toContain('/conn-1/ws') // dialed existing PTY
    expect(__getBridgeInternals().terminals.has('conn-1')).toBe(true)
    expect(__getBridgeInternals().sessionBases.get('conn-1')).toBe(base)
  })

  it('delivers the replay snapshot to a later terminalListen', async () => {
    await terminalAttach('conn-2', '/base')
    const received: string[] = []
    terminalListen('conn-2', (frame) => {
      if (!frame.exit) received.push(frame.data)
    })
    FakeWebSocket.instances[0].onmessage?.({ data: JSON.stringify({ data: 'REPLAY' }) })
    expect(received).toContain('REPLAY')
  })

  it('parses the snapshot flag; absent means incremental output', async () => {
    await terminalAttach('conn-3', '/base')
    const frames: TerminalFrame[] = []
    terminalListen('conn-3', (frame) => frames.push(frame))
    FakeWebSocket.instances[0].onmessage?.({
      data: JSON.stringify({ data: 'REDRAW', snapshot: true }),
    })
    FakeWebSocket.instances[0].onmessage?.({ data: JSON.stringify({ data: 'tail' }) })
    expect(frames).toEqual([
      { data: 'REDRAW', snapshot: true },
      { data: 'tail', snapshot: false },
    ])
  })

  // B4: the daemon's exit frame is the only thing that ends a terminal. It reaches
  // the listener in order after the output, and the close that follows it is NOT
  // a transport drop — nothing may reconnect an exited session.
  it('delivers the exit frame and does not report the following close as a drop', async () => {
    await terminalAttach('conn-4', '/base')
    const frames: TerminalFrame[] = []
    terminalListen('conn-4', (frame) => frames.push(frame))
    const drop = vi.fn()
    const unsub = onTransportDrop('conn-4', drop)
    const ws = FakeWebSocket.instances[0]
    ws.onmessage?.({ data: JSON.stringify({ data: 'bye' }) })
    ws.onmessage?.({ data: JSON.stringify({ type: 'exit', code: 3 }) })
    ws.onclose?.()
    expect(frames).toEqual([
      { data: 'bye', snapshot: false },
      { exit: true, code: 3 },
    ])
    expect(drop).not.toHaveBeenCalled()
    expect(terminalHasTransport('conn-4')).toBe(false)
    unsub()
  })

  it('a close with no exit frame is a transport drop', async () => {
    await terminalAttach('conn-5', '/base')
    const drop = vi.fn()
    const unsub = onTransportDrop('conn-5', drop)
    FakeWebSocket.instances[0].onclose?.()
    expect(drop).toHaveBeenCalledOnce()
    unsub()
  })

  it('buffers an exit frame that lands before the listener registers', async () => {
    await terminalAttach('conn-6', '/base')
    FakeWebSocket.instances[0].onmessage?.({ data: JSON.stringify({ type: 'exit', code: 0 }) })
    FakeWebSocket.instances[0].onclose?.()
    const frames: TerminalFrame[] = []
    terminalListen('conn-6', (frame) => frames.push(frame))
    expect(frames).toEqual([{ exit: true, code: 0 }])
  })
})
