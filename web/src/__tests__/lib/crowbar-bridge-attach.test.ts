import { describe, it, expect, vi, beforeEach } from 'vitest'
// Browser path is the jsdom default (no __TAURI_INTERNALS__); no mock needed.
import { terminalAttach, terminalListen, __getBridgeInternals } from '@/lib/crowbar-bridge'

class FakeWebSocket {
  static instances: FakeWebSocket[] = []
  onopen: (() => void) | null = null
  onmessage: ((e: { data: string }) => void) | null = null
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
    terminalListen('conn-2', (frame) => received.push(frame.data))
    FakeWebSocket.instances[0].onmessage?.({ data: JSON.stringify({ data: 'REPLAY' }) })
    expect(received).toContain('REPLAY')
  })

  it('parses the snapshot flag; absent means incremental output', async () => {
    await terminalAttach('conn-3', '/base')
    const frames: { data: string; snapshot: boolean }[] = []
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
})
