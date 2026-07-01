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
import { saveReconnect } from '@/features/terminal/lib/terminal-reconnect-map'

const createSpy = vi.fn(async () => 'fresh-conn')
const listSpy = vi.fn(async () => ['conn-1']) // daemon says conn-1 is alive

beforeEach(() => {
  mocks.attachSpy.mockClear()
  createSpy.mockClear()
  listSpy.mockClear()
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
