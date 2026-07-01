// Tests for unexpected transport-drop detection in openBrowserSocket.
//
// Scenarios:
// 1. Unexpected WS close (daemon restart): entry still in `terminals` when
//    onclose fires → entry is removed + drop callbacks fire.
// 2. Clean terminalDetach: entry removed BEFORE ws.close(), so when the
//    browser later fires onclose, `terminals.has()` is false → drop callbacks
//    do NOT fire.
// 3. Unsubscribed callback is not called.

import { describe, it, expect, vi, beforeEach } from 'vitest'
import {
  terminalAttach,
  terminalCreate,
  terminalDetach,
  onTransportDrop,
  __getBridgeInternals,
} from '@/lib/crowbar-bridge'
import { setWorkspaceScope } from '@/lib/workspace-scope'

// FakeWebSocket that exposes onclose so tests can trigger it manually.
class FakeWebSocket {
  static instances: FakeWebSocket[] = []
  onopen: (() => void) | null = null
  onmessage: ((e: { data: string }) => void) | null = null
  onclose: (() => void) | null = null
  onerror: (() => void) | null = null
  readyState = 0
  constructor(public url: string) {
    FakeWebSocket.instances.push(this)
    queueMicrotask(() => this.onopen?.())
  }
  send = vi.fn()
  // Simulate a clean WS close without triggering onclose automatically —
  // the test controls when onclose fires to replicate browser async behaviour.
  close = vi.fn()
}

beforeEach(() => {
  FakeWebSocket.instances = []
  vi.stubGlobal('WebSocket', FakeWebSocket as unknown as typeof WebSocket)
  // Some helpers (e.g. terminalCreate) require a workspace scope.
  setWorkspaceScope({ projectId: 'p', repoId: 'r', wsId: 'ws-1' })
  // Stub fetch for terminalCreate POST.
  vi.stubGlobal(
    'fetch',
    vi.fn(
      async () =>
        new Response(JSON.stringify({ success: true, data: { sessionId: 'conn-1' } }), {
          status: 200,
        }),
    ),
  )
})

describe('openBrowserSocket — unexpected transport drop', () => {
  it('onclose removes the terminals entry and fires the registered drop callback', async () => {
    await terminalAttach('drop-1', '/base')
    const drop = vi.fn()
    const unsub = onTransportDrop('drop-1', drop)

    // Verify entry exists before the simulated unexpected close.
    expect(__getBridgeInternals().terminals.has('drop-1')).toBe(true)

    // Simulate daemon restart: browser fires ws.onclose while entry is still mapped.
    FakeWebSocket.instances[0].onclose?.()

    expect(__getBridgeInternals().terminals.has('drop-1')).toBe(false)
    expect(drop).toHaveBeenCalledOnce()

    unsub()
  })

  it('fires all registered drop callbacks for the same connectionId', async () => {
    await terminalAttach('drop-multi', '/base')
    const cb1 = vi.fn()
    const cb2 = vi.fn()
    const u1 = onTransportDrop('drop-multi', cb1)
    const u2 = onTransportDrop('drop-multi', cb2)

    FakeWebSocket.instances[0].onclose?.()

    expect(cb1).toHaveBeenCalledOnce()
    expect(cb2).toHaveBeenCalledOnce()

    u1()
    u2()
  })

  it('does NOT fire the drop callback after a clean terminalDetach (entry already removed)', async () => {
    // terminalCreate so we get a real sessionId through the normal path.
    const connectionId = await terminalCreate('ws-1')
    // Wait for the queueMicrotask onopen to have fired.
    await new Promise((r) => setTimeout(r, 0))

    const drop = vi.fn()
    onTransportDrop(connectionId, drop)

    // terminalDetach removes from map BEFORE calling ws.close().
    await terminalDetach(connectionId)
    expect(__getBridgeInternals().terminals.has(connectionId)).toBe(false)

    // Now simulate the browser firing onclose asynchronously after our explicit close.
    FakeWebSocket.instances[0].onclose?.()

    // Entry was already removed — onclose sees terminals.has() === false → no drop.
    expect(drop).not.toHaveBeenCalled()
  })

  it('unsubscribed callback is not called on drop', async () => {
    await terminalAttach('drop-unsub', '/base')
    const drop = vi.fn()
    const unsub = onTransportDrop('drop-unsub', drop)

    // Unsubscribe before the close fires.
    unsub()

    FakeWebSocket.instances[0].onclose?.()

    expect(drop).not.toHaveBeenCalled()
  })

  it('second unexpected close on same connectionId is a no-op (entry already removed)', async () => {
    await terminalAttach('drop-once', '/base')
    const drop = vi.fn()
    onTransportDrop('drop-once', drop)

    // First close: entry removed + callback fired.
    FakeWebSocket.instances[0].onclose?.()
    expect(drop).toHaveBeenCalledOnce()

    // Second close: entry already gone → drop is not called again.
    FakeWebSocket.instances[0].onclose?.()
    expect(drop).toHaveBeenCalledOnce()
  })
})
