import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

// vi.hoisted vars are initialized before vi.mock factories run (hoisting-safe).
const mocks = vi.hoisted(() => {
  let _hasTransport = true
  const attachSpy = vi.fn(async () => {})
  return {
    attachSpy,
    setHasTransport: (v: boolean) => {
      _hasTransport = v
    },
    terminalHasTransport: () => _hasTransport,
  }
})

vi.mock('@/lib/crowbar-bridge', () => ({
  terminalAttach: mocks.attachSpy,
  terminalHasTransport: mocks.terminalHasTransport,
}))

import { resolveTerminalConnection } from '@/features/terminal/components/resolve-terminal-connection'
import { saveReconnect, loadReconnect } from '@/features/terminal/lib/terminal-reconnect-map'

const createSpy = vi.fn(async () => 'fresh-conn')
const listSpy = vi.fn(async () => ['conn-1']) // daemon says conn-1 is alive

beforeEach(() => {
  mocks.attachSpy.mockClear()
  createSpy.mockClear()
  // mockReset (not mockClear) so an UNCONSUMED mockResolvedValueOnce from a
  // previous test cannot leak into the next one's first call; then restore the
  // default "daemon says conn-1 is alive" implementation.
  listSpy.mockReset()
  listSpy.mockResolvedValue(['conn-1'])
  localStorage.clear()
  mocks.setHasTransport(true)
  vi.useFakeTimers()
})

afterEach(() => {
  vi.useRealTimers()
})

describe('resolveTerminalConnection', () => {
  it('reuses a store connectionId WITH a live transport — no attach, no create', async () => {
    mocks.setHasTransport(true)
    const r = await resolveTerminalConnection({
      workspaceId: 'ws-1',
      tabSessionId: 'tab-1',
      storeConnectionId: 'conn-store',
      base: '/base',
      listLiveSessions: listSpy,
      createTerminal: createSpy,
    })
    expect(r).toEqual({ connectionId: 'conn-store', reused: true })
    expect(mocks.attachSpy).not.toHaveBeenCalled()
    expect(createSpy).not.toHaveBeenCalled()
  })

  it('re-attaches a store connectionId whose transport was detached on switch', async () => {
    mocks.setHasTransport(false) // detach closed the WS
    listSpy.mockResolvedValueOnce(['conn-store']) // daemon still has the PTY
    const r = await resolveTerminalConnection({
      workspaceId: 'ws-1',
      tabSessionId: 'tab-1',
      storeConnectionId: 'conn-store',
      base: '/base',
      listLiveSessions: listSpy,
      createTerminal: createSpy,
    })
    expect(mocks.attachSpy).toHaveBeenCalledWith('conn-store', '/base') // re-attached → scrollback replays
    expect(r).toEqual({ connectionId: 'conn-store', reused: true })
    expect(createSpy).not.toHaveBeenCalled()
  })

  // B7: a shell tab never spawns a PTY because an existing one exited.
  it('reports GONE (never creates) when storeConnectionId has no transport and the daemon no longer has it', async () => {
    mocks.setHasTransport(false) // transport gone
    listSpy.mockResolvedValueOnce(['other-conn']) // stored id absent → the session ended
    const r = await resolveTerminalConnection({
      workspaceId: 'ws-1',
      tabSessionId: 'tab-1',
      storeConnectionId: 'dead-store-conn',
      base: '/base',
      listLiveSessions: listSpy,
      createTerminal: createSpy,
    })
    expect(r).toEqual({ gone: true })
    expect(createSpy).not.toHaveBeenCalled()
    expect(mocks.attachSpy).not.toHaveBeenCalled()
  })

  it('attaches to a persisted connectionId that the daemon confirms is alive', async () => {
    saveReconnect('ws-1', 'tab-1', 'conn-1')
    const r = await resolveTerminalConnection({
      workspaceId: 'ws-1',
      tabSessionId: 'tab-1',
      storeConnectionId: undefined,
      base: '/base',
      listLiveSessions: listSpy,
      createTerminal: createSpy,
    })
    expect(mocks.attachSpy).toHaveBeenCalledWith('conn-1', '/base')
    expect(r).toEqual({ connectionId: 'conn-1', reused: true })
    expect(createSpy).not.toHaveBeenCalled()
  })

  it('reports GONE (never creates) when the persisted connectionId is no longer alive', async () => {
    saveReconnect('ws-1', 'tab-1', 'dead-conn')
    const r = await resolveTerminalConnection({
      workspaceId: 'ws-1',
      tabSessionId: 'tab-1',
      storeConnectionId: undefined,
      base: '/base',
      listLiveSessions: listSpy,
      createTerminal: createSpy,
    })
    expect(r).toEqual({ gone: true })
    expect(createSpy).not.toHaveBeenCalled()
    expect(loadReconnect('ws-1', 'tab-1')).toBeNull()
  })

  it('creates a PTY only for a tab that was never bound to one', async () => {
    const r = await resolveTerminalConnection({
      workspaceId: 'ws-1',
      tabSessionId: 'tab-new',
      storeConnectionId: undefined,
      base: '/base',
      listLiveSessions: listSpy,
      createTerminal: createSpy,
    })
    expect(createSpy).toHaveBeenCalledOnce()
    expect(listSpy).not.toHaveBeenCalled()
    expect(r).toEqual({ connectionId: 'fresh-conn', reused: false })
  })

  // The daemon restores its persisted sessions before it serves a request, so an
  // empty list is authoritative: one question, no timed retry.
  it('asks the daemon once — an empty list is an answer, not a reason to wait and retry', async () => {
    saveReconnect('ws-1', 'tab-1', 'conn-1')
    listSpy.mockResolvedValue([])
    const r = await resolveTerminalConnection({
      workspaceId: 'ws-1',
      tabSessionId: 'tab-1',
      storeConnectionId: undefined,
      base: '/base',
      listLiveSessions: listSpy,
      createTerminal: createSpy,
    })
    expect(listSpy).toHaveBeenCalledTimes(1)
    expect(r).toEqual({ gone: true })
  })
})

// ── Attach-only (no-spawn) mode ──────────────────────────────────────────────
// The agent chat pane's terminal is not a shell tab: it is a view onto ONE
// vendor-CLI process. When that PTY is gone, spawning a replacement does not
// degrade gracefully — the pane silently becomes a BARE SHELL wearing the
// agent's frame (and the shell is then persisted into the reconnect map as if it
// were the agent). Under attachOnly the resolver must report the session GONE
// instead, on EVERY branch that would otherwise reach createTerminal:
//
//   - the storeConnectionId branch — the RECONNECT path, entered when the WS
//     transport drops out from under a mounted pane (daemon restart, CLI exit);
//   - the persisted/reconnect-map branch — the INITIAL resolve after a reload.
//
// The ordinary (unset) mode must keep spawning: shell tabs are fungible.
describe('attachOnly — an agent pane must never spawn a shell', () => {
  it('reports the session GONE instead of creating on the RECONNECT path (store id, transport dropped)', async () => {
    mocks.setHasTransport(false) // the WS just dropped: daemon restarted / CLI died
    listSpy.mockResolvedValueOnce(['other-conn']) // the agent's PTY is not among the live ones

    const r = await resolveTerminalConnection({
      workspaceId: 'ws-1',
      tabSessionId: 'agent-term',
      storeConnectionId: 'agent-term',
      base: '/base',
      listLiveSessions: listSpy,
      createTerminal: createSpy,
      attachOnly: true,
    })

    expect(r).toEqual({ gone: true })
    expect(createSpy).not.toHaveBeenCalled() // ← the bare shell that used to appear
    expect(mocks.attachSpy).not.toHaveBeenCalled()
  })

  it('reports the session GONE instead of creating on the INITIAL resolve (persisted id, daemon reaped it)', async () => {
    saveReconnect('ws-1', 'agent-term', 'agent-term')
    listSpy.mockResolvedValueOnce(['other-conn'])

    const r = await resolveTerminalConnection({
      workspaceId: 'ws-1',
      tabSessionId: 'agent-term',
      storeConnectionId: undefined,
      base: '/base',
      listLiveSessions: listSpy,
      createTerminal: createSpy,
      attachOnly: true,
    })

    expect(r).toEqual({ gone: true })
    expect(createSpy).not.toHaveBeenCalled()
    expect(mocks.attachSpy).not.toHaveBeenCalled()
  })

  it('reports GONE (never creates) when the daemon list is empty', async () => {
    mocks.setHasTransport(false)
    listSpy.mockResolvedValue([])

    const r = await resolveTerminalConnection({
      workspaceId: 'ws-1',
      tabSessionId: 'agent-term',
      storeConnectionId: 'agent-term',
      base: '/base',
      listLiveSessions: listSpy,
      createTerminal: createSpy,
      attachOnly: true,
    })

    expect(r).toEqual({ gone: true })
    expect(createSpy).not.toHaveBeenCalled()
  })

  it('never creates even for an unbound agent view', async () => {
    const r = await resolveTerminalConnection({
      workspaceId: 'ws-1',
      tabSessionId: 'agent-new',
      storeConnectionId: undefined,
      base: '/base',
      listLiveSessions: listSpy,
      createTerminal: createSpy,
      attachOnly: true,
    })
    expect(r).toEqual({ gone: true })
    expect(createSpy).not.toHaveBeenCalled()
  })

  it('drops the stale reconnect mapping when it reports GONE, so no later mount re-attempts the dead id', async () => {
    saveReconnect('ws-1', 'agent-term', 'agent-term')
    mocks.setHasTransport(false)
    listSpy.mockResolvedValueOnce(['other-conn'])

    await resolveTerminalConnection({
      workspaceId: 'ws-1',
      tabSessionId: 'agent-term',
      storeConnectionId: 'agent-term',
      base: '/base',
      listLiveSessions: listSpy,
      createTerminal: createSpy,
      attachOnly: true,
    })

    expect(loadReconnect('ws-1', 'agent-term')).toBeNull()
  })

  it('still ATTACHES when the agent PTY is alive — attachOnly forbids spawning, not attaching', async () => {
    mocks.setHasTransport(false)
    listSpy.mockResolvedValueOnce(['agent-term']) // the CLI is still running

    const r = await resolveTerminalConnection({
      workspaceId: 'ws-1',
      tabSessionId: 'agent-term',
      storeConnectionId: 'agent-term',
      base: '/base',
      listLiveSessions: listSpy,
      createTerminal: createSpy,
      attachOnly: true,
    })

    expect(mocks.attachSpy).toHaveBeenCalledWith('agent-term', '/base')
    expect(r).toEqual({ connectionId: 'agent-term', reused: true })
    expect(createSpy).not.toHaveBeenCalled()
  })

  // THE DEAD-AGENT-CHAT REGRESSION. Crowbar does not portal terminals, so an
  // attach-only xterm is BRAND-NEW on every (re)mount — it holds no screen. The
  // daemon paints a client only at ATTACH, so a live in-memory transport that is
  // merely REUSED (no terminalAttach) leaves that fresh xterm blank; and when the
  // survivor is a corpse (a refcount that skipped its detach, a half-open socket)
  // every keystroke is dropped against it. So — UNLIKE a shell tab — attach-only
  // must re-attach even when terminalHasTransport() is true. This is the case the
  // prior code silently reused (setHasTransport(true) below), and the fix re-attaches.
  it('attachOnly RE-ATTACHES even when a live in-memory transport is present (never silent-reuse)', async () => {
    mocks.setHasTransport(true) // a transport SURVIVED the remount (refcount skip / co-view)
    listSpy.mockResolvedValueOnce(['agent-term']) // the agent PTY is still live on the daemon

    const r = await resolveTerminalConnection({
      workspaceId: 'ws-1',
      tabSessionId: 'agent-term',
      storeConnectionId: 'agent-term',
      base: '/base',
      listLiveSessions: listSpy,
      createTerminal: createSpy,
      attachOnly: true,
    })

    // The load-bearing assertion: it re-attached (pulling the daemon snapshot and a
    // fresh live transport) rather than short-circuiting on the surviving transport.
    expect(mocks.attachSpy).toHaveBeenCalledWith('agent-term', '/base')
    expect(r).toEqual({ connectionId: 'agent-term', reused: true })
    expect(createSpy).not.toHaveBeenCalled()
  })

  it('attachOnly with a live in-memory transport but a daemon-reaped PTY reports GONE (no phantom reuse)', async () => {
    mocks.setHasTransport(true) // stale/phantom transport lingers in the map
    listSpy.mockResolvedValue([]) // ...but the daemon has actually reaped the PTY

    const r = await resolveTerminalConnection({
      workspaceId: 'ws-1',
      tabSessionId: 'agent-term',
      storeConnectionId: 'agent-term',
      base: '/base',
      listLiveSessions: listSpy,
      createTerminal: createSpy,
      attachOnly: true,
    })

    // Reusing the phantom would have handed back a dead connection; instead it must
    // report gone so the pane renders its dormant/Resume state.
    expect(r).toEqual({ gone: true })
    expect(mocks.attachSpy).not.toHaveBeenCalled()
    expect(createSpy).not.toHaveBeenCalled()
  })

  it('NO REGRESSION: a SHELL tab with a live transport still reuses in place (no re-attach)', async () => {
    mocks.setHasTransport(true)
    const r = await resolveTerminalConnection({
      workspaceId: 'ws-1',
      tabSessionId: 'tab-1',
      storeConnectionId: 'conn-store',
      base: '/base',
      listLiveSessions: listSpy,
      createTerminal: createSpy,
      // attachOnly unset — a portaled shell tab whose xterm outlives layout changes.
    })
    expect(r).toEqual({ connectionId: 'conn-store', reused: true })
    expect(mocks.attachSpy).not.toHaveBeenCalled() // fast in-place reuse preserved
    expect(listSpy).not.toHaveBeenCalled()
  })
})
