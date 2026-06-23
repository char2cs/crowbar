import { describe, it, expect, vi, beforeEach } from 'vitest'

// The transport mints a Tauri Channel and bridges its frames into the
// WebSocket-like onmessage callback. We mock @tauri-apps/api/core so a frame
// pushed at the captured Channel fires onmessage with the RAW DTO text, and so
// the open/send/close handlers invoke the right ws_* commands.

interface MockChannel {
  onmessage: ((data: string) => void) | null
}

const channels: MockChannel[] = []
const invoke = vi.fn(async () => undefined)

vi.mock('@tauri-apps/api/core', () => ({
  Channel: class {
    onmessage: ((data: string) => void) | null = null
    constructor() {
      channels.push(this)
    }
  },
  invoke,
}))

beforeEach(() => {
  channels.length = 0
  invoke.mockClear()
  vi.stubGlobal('crypto', { randomUUID: () => 'conn-fixed-id' })
})

const { TauriWebSocket } = await import('@/lib/ws/tauri-transport')

describe('TauriWebSocket', () => {
  it('exposes the WebSocket readyState constants', () => {
    expect(TauriWebSocket.CONNECTING).toBe(0)
    expect(TauriWebSocket.OPEN).toBe(1)
    expect(TauriWebSocket.CLOSED).toBe(3)
  })

  it('invokes ws_open with the conn id, path, and Channel, then fires onopen and goes OPEN', async () => {
    const sock = new TauriWebSocket('/v0/projects/p/repos/r/workspaces')
    const onopen = vi.fn()
    sock.onopen = onopen

    // Let the ws_open invoke promise resolve.
    await vi.waitFor(() => expect(invoke).toHaveBeenCalled())
    await Promise.resolve()

    expect(invoke).toHaveBeenCalledWith('ws_open', {
      connId: 'conn-fixed-id',
      path: '/v0/projects/p/repos/r/workspaces',
      onMessage: channels[0],
    })
    await vi.waitFor(() => expect(onopen).toHaveBeenCalled())
    expect(sock.readyState).toBe(TauriWebSocket.OPEN)
  })

  it('forwards a raw DTO frame from the Channel to onmessage without extracting .data', async () => {
    const sock = new TauriWebSocket('/v0/projects/p/repos/r/workspaces')
    const onmessage = vi.fn()
    sock.onmessage = onmessage

    await vi.waitFor(() => expect(channels.length).toBe(1))
    const dto = '{"id":"ws-1","status":"new","data":"not-extracted"}'
    channels[0].onmessage?.(dto)

    expect(onmessage).toHaveBeenCalledWith({ data: dto })
  })

  it('treats the close sentinel as a close (fires onclose, goes CLOSED) — not a message', async () => {
    const sock = new TauriWebSocket('/v0/projects/p/repos/r/workspaces')
    const onmessage = vi.fn()
    const onclose = vi.fn()
    sock.onmessage = onmessage
    sock.onclose = onclose

    await vi.waitFor(() => expect(channels.length).toBe(1))
    // The Rust reader pushes this exact sentinel when the daemon closes the stream.
    channels[0].onmessage?.('\u0000crowbar-ws-close')

    expect(onclose).toHaveBeenCalledTimes(1)
    expect(onmessage).not.toHaveBeenCalled()
    expect(sock.readyState).toBe(TauriWebSocket.CLOSED)
  })

  it('send() invokes ws_send with the conn id and raw data', async () => {
    const sock = new TauriWebSocket('/v0/projects/p/repos/r/workspaces')
    await vi.waitFor(() => expect(invoke).toHaveBeenCalledWith('ws_open', expect.anything()))
    invoke.mockClear()

    sock.send('{"hello":true}')
    expect(invoke).toHaveBeenCalledWith('ws_send', {
      connId: 'conn-fixed-id',
      data: '{"hello":true}',
    })
  })

  it('close() invokes ws_close, fires onclose, and goes CLOSED', async () => {
    const sock = new TauriWebSocket('/v0/projects/p/repos/r/workspaces')
    const onclose = vi.fn()
    sock.onclose = onclose
    await vi.waitFor(() => expect(invoke).toHaveBeenCalledWith('ws_open', expect.anything()))
    invoke.mockClear()

    sock.close()
    expect(invoke).toHaveBeenCalledWith('ws_close', { connId: 'conn-fixed-id' })
    expect(onclose).toHaveBeenCalled()
    expect(sock.readyState).toBe(TauriWebSocket.CLOSED)
  })

  it('close() while CONNECTING defers ws_close until ws_open resolves and never fires onopen', async () => {
    // Gate the ws_open resolution so we can close() while the socket is still
    // CONNECTING — the StrictMode double-mount / rapid scope-switch race that
    // leaks a daemon FD + 2 tokio tasks if the deferred teardown is missing.
    let resolveOpen: (() => void) | undefined
    invoke.mockImplementationOnce(
      () =>
        new Promise<undefined>((resolve) => {
          resolveOpen = () => resolve(undefined)
        }),
    )

    const sock = new TauriWebSocket('/v0/projects/p/repos/r/workspaces')
    const onopen = vi.fn()
    sock.onopen = onopen

    // The Channel is minted, but ws_open has NOT resolved: still CONNECTING.
    await vi.waitFor(() => expect(channels.length).toBe(1))
    expect(sock.readyState).toBe(TauriWebSocket.CONNECTING)

    // Close before the handshake completes. Rust hasn't registered the
    // connection yet, so no eager ws_close (it would be a Rust-side no-op).
    sock.close()
    expect(sock.readyState).toBe(TauriWebSocket.CLOSED)
    expect(invoke).not.toHaveBeenCalledWith('ws_close', expect.anything())

    // ws_open now resolves: the connection is registered, so the deferred
    // ws_close fires to tear it down — and onopen must NOT fire.
    resolveOpen?.()
    await vi.waitFor(() =>
      expect(invoke).toHaveBeenCalledWith('ws_close', { connId: 'conn-fixed-id' }),
    )
    expect(onopen).not.toHaveBeenCalled()
    expect(sock.readyState).toBe(TauriWebSocket.CLOSED)
  })
})
