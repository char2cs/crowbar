import { describe, it, expect, vi, beforeEach } from 'vitest'
// NOTE: crowbar-bridge declares isTauri()/tauriInvoke LOCALLY (it does not import
// '@/lib/tauri'). jsdom's window lacks __TAURI_INTERNALS__, so the browser path is
// the default — no mock needed. (To test the Tauri branch, set
// window.__TAURI_INTERNALS__ = { invoke: vi.fn() }.)
import { terminalCreate, terminalDetach, __getBridgeInternals } from '@/lib/crowbar-bridge'
import { setWorkspaceScope } from '@/lib/workspace-scope'

class FakeWebSocket {
  static instances: FakeWebSocket[] = []
  onopen: (() => void) | null = null
  onmessage: ((e: { data: string }) => void) | null = null
  readyState = 0
  closed = false
  constructor(public url: string) { FakeWebSocket.instances.push(this); queueMicrotask(() => this.onopen?.()) }
  send = vi.fn()
  close = vi.fn(() => { this.closed = true })
}

beforeEach(() => {
  FakeWebSocket.instances = []
  vi.stubGlobal('WebSocket', FakeWebSocket as unknown as typeof WebSocket)
  // terminalCreate calls workspaceBase(wsId) which requires a recorded scope.
  setWorkspaceScope({ projectId: 'p', repoId: 'r', wsId: 'ws-1' })
  // terminalCreate POSTs via apiFetch, which unwraps the {success,data} envelope.
  vi.stubGlobal('fetch', vi.fn(async () =>
    new Response(JSON.stringify({ success: true, data: { sessionId: 'conn-1' } }), { status: 200 })))
})

describe('terminalDetach', () => {
  it('closes the WS and drops the transport entry but keeps the DELETE base', async () => {
    const connectionId = await terminalCreate('ws-1')
    const ws = FakeWebSocket.instances[0]

    await terminalDetach(connectionId)

    expect(ws.close).toHaveBeenCalledOnce()
    const internals = __getBridgeInternals()
    expect(internals.terminals.has(connectionId)).toBe(false)   // transport removed
    expect(internals.sessionBases.has(connectionId)).toBe(true) // base kept for re-attach
  })

  it('does NOT call DELETE', async () => {
    const connectionId = await terminalCreate('ws-1')
    const fetchSpy = globalThis.fetch as ReturnType<typeof vi.fn>
    fetchSpy.mockClear()
    await terminalDetach(connectionId)
    const deleteCalls = fetchSpy.mock.calls.filter(([, init]) => (init as RequestInit | undefined)?.method === 'DELETE')
    expect(deleteCalls).toHaveLength(0)
  })
})
