import { describe, it, expect, vi, beforeEach } from 'vitest'

// vi.hoisted vars are initialized before vi.mock factories run (hoisting-safe).
const mocks = vi.hoisted(() => {
  let _hasTransport = true
  const attachSpy = vi.fn(async () => {})
  return {
    attachSpy,
    setHasTransport: (v: boolean) => { _hasTransport = v },
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
const listSpy = vi.fn(async () => ['conn-1'])  // daemon says conn-1 is alive

beforeEach(() => {
  mocks.attachSpy.mockClear()
  createSpy.mockClear()
  listSpy.mockClear()
  localStorage.clear()
  mocks.setHasTransport(true)
})

describe('resolveTerminalConnection', () => {
  it('reuses a store connectionId WITH a live transport — no attach, no create', async () => {
    mocks.setHasTransport(true)
    const r = await resolveTerminalConnection({
      workspaceId: 'ws-1', tabSessionId: 'tab-1', storeConnectionId: 'conn-store',
      base: '/base', listLiveSessions: listSpy, createTerminal: createSpy,
    })
    expect(r).toEqual({ connectionId: 'conn-store', reused: true })
    expect(mocks.attachSpy).not.toHaveBeenCalled()
    expect(createSpy).not.toHaveBeenCalled()
  })

  it('re-attaches a store connectionId whose transport was detached on switch', async () => {
    mocks.setHasTransport(false)  // detach closed the WS
    listSpy.mockResolvedValueOnce(['conn-store'])  // daemon still has the PTY
    const r = await resolveTerminalConnection({
      workspaceId: 'ws-1', tabSessionId: 'tab-1', storeConnectionId: 'conn-store',
      base: '/base', listLiveSessions: listSpy, createTerminal: createSpy,
    })
    expect(mocks.attachSpy).toHaveBeenCalledWith('conn-store', '/base')  // re-attached → scrollback replays
    expect(r).toEqual({ connectionId: 'conn-store', reused: true })
    expect(createSpy).not.toHaveBeenCalled()
  })

  it('creates fresh when storeConnectionId has no transport and daemon does NOT confirm it', async () => {
    mocks.setHasTransport(false)  // transport gone
    listSpy.mockResolvedValueOnce([])  // daemon no longer has this PTY
    const r = await resolveTerminalConnection({
      workspaceId: 'ws-1', tabSessionId: 'tab-1', storeConnectionId: 'dead-store-conn',
      base: '/base', listLiveSessions: listSpy, createTerminal: createSpy,
    })
    expect(createSpy).toHaveBeenCalledOnce()
    expect(r).toEqual({ connectionId: 'fresh-conn', reused: false })
    expect(mocks.attachSpy).not.toHaveBeenCalled()
  })

  it('attaches to a persisted connectionId that the daemon confirms is alive', async () => {
    saveReconnect('ws-1', 'tab-1', 'conn-1')
    const r = await resolveTerminalConnection({
      workspaceId: 'ws-1', tabSessionId: 'tab-1', storeConnectionId: undefined,
      base: '/base', listLiveSessions: listSpy, createTerminal: createSpy,
    })
    expect(mocks.attachSpy).toHaveBeenCalledWith('conn-1', '/base')
    expect(r).toEqual({ connectionId: 'conn-1', reused: true })
    expect(createSpy).not.toHaveBeenCalled()
  })

  it('creates fresh when the persisted connectionId is no longer alive', async () => {
    saveReconnect('ws-1', 'tab-1', 'dead-conn')
    const r = await resolveTerminalConnection({
      workspaceId: 'ws-1', tabSessionId: 'tab-1', storeConnectionId: undefined,
      base: '/base', listLiveSessions: listSpy, createTerminal: createSpy,
    })
    expect(createSpy).toHaveBeenCalledOnce()
    expect(r).toEqual({ connectionId: 'fresh-conn', reused: false })
  })
})
