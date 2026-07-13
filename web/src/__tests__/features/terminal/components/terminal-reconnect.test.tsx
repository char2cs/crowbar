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

  it('creates fresh when storeConnectionId has no transport and daemon does NOT confirm it', async () => {
    mocks.setHasTransport(false) // transport gone
    listSpy.mockResolvedValueOnce(['other-conn']) // non-empty list, stored id absent → genuinely gone
    const r = await resolveTerminalConnection({
      workspaceId: 'ws-1',
      tabSessionId: 'tab-1',
      storeConnectionId: 'dead-store-conn',
      base: '/base',
      listLiveSessions: listSpy,
      createTerminal: createSpy,
    })
    expect(createSpy).toHaveBeenCalledOnce()
    expect(r).toEqual({ connectionId: 'fresh-conn', reused: false })
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

  it('creates fresh when the persisted connectionId is no longer alive', async () => {
    saveReconnect('ws-1', 'tab-1', 'dead-conn')
    const r = await resolveTerminalConnection({
      workspaceId: 'ws-1',
      tabSessionId: 'tab-1',
      storeConnectionId: undefined,
      base: '/base',
      listLiveSessions: listSpy,
      createTerminal: createSpy,
    })
    expect(createSpy).toHaveBeenCalledOnce()
    expect(r).toEqual({ connectionId: 'fresh-conn', reused: false })
  })
})

describe('Fix B — empty live-session list retry', () => {
  it('retries once after 400ms on an empty list and re-attaches when found on retry', async () => {
    saveReconnect('ws-1', 'tab-1', 'conn-1')
    // First call: empty (daemon still loading sessions); second call: conn-1 is there.
    listSpy.mockResolvedValueOnce([]).mockResolvedValueOnce(['conn-1'])

    const promise = resolveTerminalConnection({
      workspaceId: 'ws-1',
      tabSessionId: 'tab-1',
      storeConnectionId: undefined,
      base: '/base',
      listLiveSessions: listSpy,
      createTerminal: createSpy,
    })
    // Fire the 400ms retry timer so the second listLiveSessions call proceeds.
    await vi.advanceTimersByTimeAsync(400)
    const r = await promise

    expect(listSpy).toHaveBeenCalledTimes(2)
    expect(mocks.attachSpy).toHaveBeenCalledWith('conn-1', '/base')
    expect(r).toEqual({ connectionId: 'conn-1', reused: true })
    expect(createSpy).not.toHaveBeenCalled()
  })

  it('creates fresh when retry also returns empty (id permanently absent)', async () => {
    saveReconnect('ws-1', 'tab-1', 'conn-1')
    // Both calls return empty — session is truly gone.
    listSpy.mockResolvedValue([])

    const promise = resolveTerminalConnection({
      workspaceId: 'ws-1',
      tabSessionId: 'tab-1',
      storeConnectionId: undefined,
      base: '/base',
      listLiveSessions: listSpy,
      createTerminal: createSpy,
    })
    await vi.advanceTimersByTimeAsync(400)
    const r = await promise

    expect(listSpy).toHaveBeenCalledTimes(2)
    expect(createSpy).toHaveBeenCalledOnce()
    expect(r).toEqual({ connectionId: 'fresh-conn', reused: false })
    expect(mocks.attachSpy).not.toHaveBeenCalled()
  })

  it('does NOT retry when the first live list is non-empty but missing the persisted id', async () => {
    // Non-empty list where the persisted id is simply absent → stale, no retry.
    saveReconnect('ws-1', 'tab-1', 'dead-conn')
    listSpy.mockResolvedValueOnce(['other-conn'])

    const promise = resolveTerminalConnection({
      workspaceId: 'ws-1',
      tabSessionId: 'tab-1',
      storeConnectionId: undefined,
      base: '/base',
      listLiveSessions: listSpy,
      createTerminal: createSpy,
    })
    await vi.advanceTimersByTimeAsync(400) // timer should never fire, but safe to advance
    const r = await promise

    expect(listSpy).toHaveBeenCalledTimes(1) // no second call
    expect(createSpy).toHaveBeenCalledOnce()
    expect(r).toEqual({ connectionId: 'fresh-conn', reused: false })
  })

  it('branch 1 (storeConnectionId, no transport): retries empty list and re-attaches the restored session', async () => {
    // The daemon-restart reconnect bug: store has the id, transport is gone, and
    // the just-restarted daemon returns [] on the first list (socket rebind window),
    // then the restored id on retry. Must re-attach, not create fresh.
    mocks.setHasTransport(false)
    listSpy.mockResolvedValueOnce([]).mockResolvedValueOnce(['conn-store'])

    const promise = resolveTerminalConnection({
      workspaceId: 'ws-1',
      tabSessionId: 'tab-1',
      storeConnectionId: 'conn-store',
      base: '/base',
      listLiveSessions: listSpy,
      createTerminal: createSpy,
    })
    await vi.advanceTimersByTimeAsync(400)
    const r = await promise

    expect(listSpy).toHaveBeenCalledTimes(2)
    expect(mocks.attachSpy).toHaveBeenCalledWith('conn-store', '/base')
    expect(r).toEqual({ connectionId: 'conn-store', reused: true })
    expect(createSpy).not.toHaveBeenCalled()
  })
})

describe('Fix D — branch-1 store id not in daemon list', () => {
  it('creates fresh when storeConnectionId has no transport and daemon list does NOT contain it', async () => {
    // Validates Fix D: branch 1 should fall through to createTerminal when the
    // stored connectionId is not in the live sessions list.
    mocks.setHasTransport(false)
    listSpy.mockResolvedValueOnce(['other-conn']) // non-empty but stored id absent
    const r = await resolveTerminalConnection({
      workspaceId: 'ws-1',
      tabSessionId: 'tab-1',
      storeConnectionId: 'dead-store-conn',
      base: '/base',
      listLiveSessions: listSpy,
      createTerminal: createSpy,
    })
    expect(createSpy).toHaveBeenCalledOnce()
    expect(r).toEqual({ connectionId: 'fresh-conn', reused: false })
    expect(mocks.attachSpy).not.toHaveBeenCalled()
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

  it('reports GONE (never creates) when the daemon list is empty even after the retry', async () => {
    mocks.setHasTransport(false)
    listSpy.mockResolvedValue([]) // both attempts empty — genuinely gone

    const promise = resolveTerminalConnection({
      workspaceId: 'ws-1',
      tabSessionId: 'agent-term',
      storeConnectionId: 'agent-term',
      base: '/base',
      listLiveSessions: listSpy,
      createTerminal: createSpy,
      attachOnly: true,
    })
    await vi.advanceTimersByTimeAsync(400)
    const r = await promise

    expect(listSpy).toHaveBeenCalledTimes(2) // the restart-window retry still applies
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

  it('NO REGRESSION: an ordinary terminal pane (attachOnly unset) still spawns a fresh shell', async () => {
    mocks.setHasTransport(false)
    listSpy.mockResolvedValueOnce(['other-conn']) // same dead-session inputs as above

    const r = await resolveTerminalConnection({
      workspaceId: 'ws-1',
      tabSessionId: 'tab-1',
      storeConnectionId: 'dead-store-conn',
      base: '/base',
      listLiveSessions: listSpy,
      createTerminal: createSpy,
    })

    expect(createSpy).toHaveBeenCalledOnce()
    expect(r).toEqual({ connectionId: 'fresh-conn', reused: false })
  })
})
