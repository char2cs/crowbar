import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

// A fake bridge: each openTerminal is a new view transport the test can drop.
const bridge = vi.hoisted(() => {
  type Conn = {
    sessionId: string
    base: string
    alive: boolean
    close: ReturnType<typeof vi.fn>
    drop: () => void
    onDrop: (cb: () => void) => () => void
  }
  const opened: Conn[] = []
  return {
    opened,
    live: ['pty-1', 'pty-A', 'pty-B', 'pty-C'] as string[],
    listCalls: [] as string[],
    listGate: null as Promise<void> | null,
    failList: false,
    created: 0,
    openTerminal: (sessionId: string, base: string) => {
      const drops: Array<() => void> = []
      const conn: Conn = {
        sessionId,
        base,
        alive: true,
        close: vi.fn(() => {
          conn.alive = false
        }),
        drop: () => {
          conn.alive = false
          for (const cb of drops) cb()
        },
        onDrop: (cb) => {
          drops.push(cb)
          return () => {}
        },
      }
      opened.push(conn)
      return conn
    },
  }
})

vi.mock('@/lib/crowbar-bridge', () => ({
  openTerminal: bridge.openTerminal,
  terminalListLive: async (base: string) => {
    bridge.listCalls.push(base)
    if (bridge.failList) {
      bridge.failList = false
      throw new Error('daemon unreachable')
    }
    if (bridge.listGate) await bridge.listGate
    return [...bridge.live]
  },
  terminalCreate: async () => {
    bridge.created++
    return `fresh-${bridge.created}`
  },
}))

vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  getActiveWorkspaceId: () => 'ws-active',
}))

import { useTerminalAttachment } from '@/features/terminal/hooks/use-terminal-attachment'
import { useTerminalStore } from '@/features/terminal/stores/terminal-store'
import { saveReconnect } from '@/features/terminal/lib/terminal-reconnect-map'
import { recordWorkspaceScope, __resetWorkspaceScopesForTest } from '@/lib/workspace-scope'
import { useConnectionStore } from '@/lib/ws/connection-store'

type Props = Parameters<typeof useTerminalAttachment>[0]

function render(initial: Partial<Props> = {}) {
  const onEnded = vi.fn()
  const props: Props = {
    sessionId: 'tab-1',
    workspaceId: 'ws-1',
    attachOnly: false,
    enabled: true,
    onEnded,
    ...initial,
  }
  const hook = renderHook((p: Props) => useTerminalAttachment(p), { initialProps: props })
  const settle = () => act(async () => {})
  return { ...hook, props, onEnded, settle }
}

beforeEach(() => {
  bridge.opened.length = 0
  bridge.listCalls.length = 0
  bridge.listGate = null
  bridge.failList = false
  bridge.created = 0
  bridge.live = ['pty-1', 'pty-A', 'pty-B', 'pty-C']
  localStorage.clear()
  useTerminalStore.setState({ sessions: new Map() })
  __resetWorkspaceScopesForTest()
  recordWorkspaceScope({ projectId: 'p', repoId: 'r', wsId: 'ws-1', owningChatId: 'chat-1' })
  recordWorkspaceScope({ projectId: 'p', repoId: 'r', wsId: 'ws-active', owningChatId: 'chat-x' })
  useConnectionStore.setState({ status: 'connected' })
})

describe('useTerminalAttachment', () => {
  it("gives a never-bound tab its first PTY under its OWN workspace's chat, and binds the tab to it", async () => {
    const { result, settle } = render()
    await settle()
    expect(bridge.opened).toHaveLength(1)
    expect(bridge.opened[0]).toMatchObject({
      sessionId: 'fresh-1',
      base: '/v0/chats/chat-1/terminals',
    })
    expect(result.current.connection).toBe(bridge.opened[0])
    expect(result.current.created).toBe(true)
    expect(useTerminalStore.getState().getSession('tab-1')?.connectionId).toBe('fresh-1')
  })

  it('an explicit chatId owns the PTY: the chat base is used, whatever the workspace', async () => {
    saveReconnect('ws-1', 'agent', 'pty-1')
    const { settle } = render({ sessionId: 'agent', chatId: 'chat-9', attachOnly: true })
    await settle()
    expect(bridge.opened[0]).toMatchObject({
      sessionId: 'pty-1',
      base: '/v0/chats/chat-9/terminals',
    })
  })

  it('falls back to the active workspace when no workspaceId is threaded', async () => {
    const { settle } = render({ workspaceId: undefined })
    await settle()
    expect(bridge.opened[0].base).toBe('/v0/chats/chat-x/terminals')
  })

  it('does nothing until enabled (a never-shown tab, or no owning chat yet)', async () => {
    const { rerender, props, settle } = render({ enabled: false })
    await settle()
    expect(bridge.opened).toHaveLength(0)
    rerender({ ...props, enabled: true })
    await settle()
    expect(bridge.opened).toHaveLength(1)
  })

  it('re-attaches after a transport drop — the same session, a new transport, never a new PTY', async () => {
    useTerminalStore.getState().updateSession('tab-1', { connectionId: 'pty-1' })
    const { result, settle } = render()
    await settle()
    const first = bridge.opened[0]
    act(() => first.drop())
    await settle()
    expect(bridge.opened).toHaveLength(2)
    expect(bridge.opened[1].sessionId).toBe('pty-1')
    expect(result.current.connection).toBe(bridge.opened[1])
    expect(bridge.created).toBe(0)
  })

  // B7: a tab never spawns a PTY because an existing one exited.
  it('a drop whose session has ended reports it ended and spawns nothing', async () => {
    useTerminalStore.getState().updateSession('tab-1', { connectionId: 'pty-1' })
    const { result, onEnded, settle } = render()
    await settle()
    bridge.live = []
    act(() => bridge.opened[0].drop())
    await settle()
    expect(onEnded).toHaveBeenCalledExactlyOnceWith('tab-1')
    expect(bridge.created).toBe(0)
    expect(bridge.opened).toHaveLength(1)
    expect(result.current.connection).toBeNull()
  })

  it('an agent view never spawns, even with nothing bound', async () => {
    const { onEnded, settle } = render({ sessionId: 'agent', attachOnly: true })
    await settle()
    expect(bridge.created).toBe(0)
    expect(onEnded).toHaveBeenCalledExactlyOnceWith('agent')
  })

  it('a swap closes the outgoing transport and attaches the incoming session', async () => {
    useTerminalStore.getState().updateSession('A', { connectionId: 'pty-A' })
    useTerminalStore.getState().updateSession('B', { connectionId: 'pty-B' })
    const { rerender, props, result, settle } = render({ sessionId: 'A', attachOnly: true })
    await settle()
    const a = bridge.opened[0]
    rerender({ ...props, sessionId: 'B' })
    await settle()
    expect(a.close).toHaveBeenCalled()
    expect(bridge.opened.map((c) => c.sessionId)).toEqual(['pty-A', 'pty-B'])
    expect(result.current.connection).toBe(bridge.opened[1])
  })

  it('racing swaps converge on the LATEST session; a superseded attempt opens nothing', async () => {
    for (const id of ['A', 'B', 'C'])
      useTerminalStore.getState().updateSession(id, { connectionId: `pty-${id}` })
    let release = () => {}
    bridge.listGate = new Promise<void>((r) => {
      release = r
    })
    const { rerender, props, result, settle } = render({ sessionId: 'A', attachOnly: true })
    rerender({ ...props, sessionId: 'B' })
    rerender({ ...props, sessionId: 'C' })
    release()
    await settle()
    expect(bridge.opened.map((c) => c.sessionId)).toEqual(['pty-C'])
    expect(result.current.connection?.sessionId).toBe('pty-C')
  })

  it('unmount detaches: the transport closes, the PTY is left running', async () => {
    const { unmount, settle } = render()
    await settle()
    unmount()
    expect(bridge.opened[0].close).toHaveBeenCalled()
  })

  it('a daemon that cannot be asked is retried when the app reconnects to it — no timer', async () => {
    useTerminalStore.getState().updateSession('tab-1', { connectionId: 'pty-1' })
    useConnectionStore.setState({ status: 'disconnected' })
    bridge.failList = true
    const { result, onEnded, settle } = render()
    await settle()
    expect(bridge.opened).toHaveLength(0)
    expect(onEnded).not.toHaveBeenCalled() // could not ask ≠ gone

    act(() => useConnectionStore.setState({ status: 'connected' }))
    await settle()
    expect(bridge.opened.map((c) => c.sessionId)).toEqual(['pty-1'])
    expect(result.current.connection).toBe(bridge.opened[0])
  })

  it('unmount while waiting for the daemon releases the wait: a later reconnect asks nothing', async () => {
    useTerminalStore.getState().updateSession('tab-1', { connectionId: 'pty-1' })
    useConnectionStore.setState({ status: 'disconnected' })
    bridge.failList = true
    const { unmount, settle } = render()
    await settle()
    expect(bridge.listCalls).toHaveLength(1)

    unmount()
    act(() => useConnectionStore.setState({ status: 'connected' }))
    await act(async () => {})
    expect(bridge.listCalls).toHaveLength(1)
    expect(bridge.opened).toHaveLength(0)
  })
})
