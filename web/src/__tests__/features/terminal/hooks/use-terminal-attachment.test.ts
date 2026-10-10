import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

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
    createGate: null as Promise<void> | null,
    // One gate per terminalCreate call, in call order; a call past the list uses createGate.
    createGates: [] as Promise<void>[],
    failList: false,
    /** What the next failing list throws; a plain error by default. */
    listError: null as unknown,
    refuseOpen: false,
    created: 0,
    openTerminal: (sessionId: string, base: string) => {
      if (bridge.refuseOpen) throw new Error('remote hosts need the desktop app')
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
      throw bridge.listError ?? new Error('daemon unreachable')
    }
    if (bridge.listGate) await bridge.listGate
    return [...bridge.live]
  },
  terminalCreate: async () => {
    const id = `fresh-${++bridge.created}`
    const gate = bridge.createGates.shift() ?? bridge.createGate
    if (gate) await gate
    return id
  },
}))

import { useTerminalAttachment } from '@/features/terminal/hooks/use-terminal-attachment'
import { useTerminalStore } from '@/features/terminal/stores/terminal-store'
import { toast } from '@/features/window/stores/toast-store'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { recordWorkspaceScope, __resetWorkspaceScopesForTest } from '@/lib/workspace-scope'
import {
  reportChannelState,
  resetConnectionState,
  useConnectionStore,
} from '@/lib/ws/connection-store'
import {
  destroyHostRuntime,
  getHostRuntime,
  getOrCreateHostRuntime,
} from '@/features/hosts/stores/host-runtime-registry'
import { proxyMarkerError } from '@/lib/api/errors'
import {
  loadProjects,
  pairRemoteHost,
  projectOn,
  resetHostWorld,
} from '@/__tests__/__fixtures__/remote-host'

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
  bridge.createGate = null
  bridge.createGates = []
  bridge.failList = false
  bridge.listError = null
  bridge.refuseOpen = false
  bridge.created = 0
  bridge.live = ['pty-1', 'pty-A', 'pty-B', 'pty-C']
  localStorage.clear()
  resetHostWorld()
  useTerminalStore.setState({ sessions: new Map() })
  __resetWorkspaceScopesForTest()
  recordWorkspaceScope({ projectId: 'p', repoId: 'r', wsId: 'ws-1', owningChatId: 'chat-1' })
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
    const { settle } = render({ sessionId: 'pty-1', chatId: 'chat-9', attachOnly: true })
    await settle()
    expect(bridge.opened[0]).toMatchObject({
      sessionId: 'pty-1',
      base: '/v0/chats/chat-9/terminals',
    })
  })

  // A sibling chat is no workspace's recorded owner, so only the workspace it is
  // opened in can say which daemon its PTY lives on.
  it('addresses an explicit chat on the host of the workspace it is opened in', async () => {
    pairRemoteHost('host-1')
    loadProjects(projectOn('p', 'host-1'), projectOn('p-l', 'local'))
    recordWorkspaceScope({
      projectId: 'p-l',
      repoId: 'r',
      wsId: 'ws-local',
      owningChatId: 'chat-l',
    })

    const remote = render({ sessionId: 'pty-1', chatId: 'chat-9', attachOnly: true })
    await remote.settle()
    const local = render({
      sessionId: 'pty-1',
      workspaceId: 'ws-local',
      chatId: 'chat-8',
      attachOnly: true,
    })
    await local.settle()

    expect(bridge.opened.map((c) => c.base)).toEqual([
      'crowbar://host-1/v0/chats/chat-9/terminals',
      '/v0/chats/chat-8/terminals',
    ])
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
    const { rerender, props, result, settle } = render({ sessionId: 'pty-A', attachOnly: true })
    await settle()
    const a = bridge.opened[0]
    rerender({ ...props, sessionId: 'pty-B' })
    await settle()
    expect(a.close).toHaveBeenCalled()
    expect(bridge.opened.map((c) => c.sessionId)).toEqual(['pty-A', 'pty-B'])
    expect(result.current.connection).toBe(bridge.opened[1])
  })

  it('racing swaps converge on the LATEST session; a superseded attempt opens nothing', async () => {
    let release = () => {}
    bridge.listGate = new Promise<void>((r) => {
      release = r
    })
    const { rerender, props, result, settle } = render({ sessionId: 'pty-A', attachOnly: true })
    rerender({ ...props, sessionId: 'pty-B' })
    rerender({ ...props, sessionId: 'pty-C' })
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

  // The daemon made the PTY the moment it answered; forgetting it because the view left
  // meanwhile would have the next mount make a second and leave this one running unseen.
  it('unmount while the daemon creates the PTY remembers it, so a remount attaches it rather than making another', async () => {
    let created = () => {}
    bridge.createGate = new Promise<void>((r) => {
      created = r
    })
    const first = render()
    await first.settle()

    first.unmount()
    created()
    await first.settle()

    bridge.live = [...bridge.live, 'fresh-1'] // the daemon is running the PTY it made
    const second = render()
    await second.settle()

    expect(bridge.created).toBe(1)
    expect(bridge.opened.map((c) => c.sessionId)).toEqual(['fresh-1'])
    expect(second.result.current.connection).toBe(bridge.opened[0])
  })

  // Mount, unmount, remount (StrictMode in dev, a quick remount anywhere) while the first
  // create is in flight: the live attempt answers first, the superseded one last. The
  // last must not replace the PTY the view shows, or a drop re-attaches to the wrong shell.
  it('a superseded create answering last never replaces the PTY the view is showing', async () => {
    const releases: Array<() => void> = []
    bridge.createGates = [0, 1].map(() => new Promise<void>((r) => releases.push(r)))
    const first = render()
    await first.settle()
    first.unmount()
    const second = render()
    await second.settle()

    releases[1]()
    await second.settle()
    releases[0]()
    await second.settle()

    expect(bridge.opened.map((c) => c.sessionId)).toEqual(['fresh-2'])
    expect(useTerminalStore.getState().getSession('tab-1')?.connectionId).toBe('fresh-2')

    bridge.live = [...bridge.live, 'fresh-1', 'fresh-2']
    act(() => bridge.opened[0].drop())
    await second.settle()

    expect(bridge.created).toBe(2)
    expect(bridge.opened.map((c) => c.sessionId)).toEqual(['fresh-2', 'fresh-2'])
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

// A paired install cold-boots with the project list unloaded: the terminal's
// host is unknown, so it may not address the daemon (any daemon) until it is.
describe('on an install that has paired a remote host', () => {
  const REMOTE_TERMINALS = 'crowbar://host-1/v0/chats/chat-r/terminals'
  const LOCAL_TERMINALS = '/v0/chats/chat-l/terminals'
  const REMOTE_CHANNEL = 'crowbar://host-1/v0/chats/chat-r/ws'

  beforeEach(() => {
    resetHostWorld()
    pairRemoteHost('host-1')
    resetConnectionState()
    __resetWorkspaceScopesForTest()
    recordWorkspaceScope({
      projectId: 'p-remote',
      repoId: 'r',
      wsId: 'ws-remote',
      owningChatId: 'chat-r',
    })
    recordWorkspaceScope({
      projectId: 'p-local',
      repoId: 'r',
      wsId: 'ws-local',
      owningChatId: 'chat-l',
    })
    // A tab already bound to a PTY, so every attach has to ask the daemon.
    useTerminalStore.getState().updateSession('tab-1', { connectionId: 'pty-1' })
  })

  afterEach(() => {
    destroyHostRuntime('host-1')
    resetHostWorld()
    vi.restoreAllMocks()
  })

  it('asks nothing and opens nothing until the project list says whose workspace it is', async () => {
    const remote = render({ workspaceId: 'ws-remote' })
    const local = render({ sessionId: 'tab-2', workspaceId: 'ws-local' })
    useTerminalStore.getState().updateSession('tab-2', { connectionId: 'pty-A' })
    await remote.settle()
    expect(bridge.listCalls).toEqual([])
    expect(bridge.opened).toHaveLength(0)
    expect(bridge.created).toBe(0)

    act(() => loadProjects(projectOn('p-remote', 'host-1'), projectOn('p-local', 'local')))
    await remote.settle()

    expect(bridge.opened.map((c) => c.base).sort()).toEqual(
      [LOCAL_TERMINALS, REMOTE_TERMINALS].sort(),
    )
    expect(remote.result.current.connection).not.toBeNull()
    expect(local.result.current.connection).not.toBeNull()
  })

  it('switching local to remote closes the local transport and attaches the remote one', async () => {
    loadProjects(projectOn('p-remote', 'host-1'), projectOn('p-local', 'local'))
    const { rerender, props, result, settle } = render({ workspaceId: 'ws-local' })
    await settle()
    const local = bridge.opened[0]

    rerender({ ...props, workspaceId: 'ws-remote' })
    await settle()

    expect(local.close).toHaveBeenCalled()
    expect(bridge.opened.map((c) => c.base)).toEqual([LOCAL_TERMINALS, REMOTE_TERMINALS])
    expect(result.current.connection).toBe(bridge.opened[1])
  })

  // One source says a remote host is back: its status. A failed list marks a host still read
  // as up down, so its next up is the return; channels opening re-attach nothing.
  it('retries a remote terminal when ITS host is up again, and on nothing else', async () => {
    loadProjects(projectOn('p-remote', 'host-1'))
    getOrCreateHostRuntime('host-1').getState().setStatus('up')
    bridge.failList = true
    bridge.listError = proxyMarkerError('host-unreachable', 'host-1')
    const { result, onEnded, settle } = render({ workspaceId: 'ws-remote' })
    await settle()
    expect(bridge.listCalls).toEqual([REMOTE_TERMINALS])
    expect(getHostRuntime('host-1')?.getState().status).toBe('down')

    act(() => reportChannelState('/v0/projects', true, 'local'))
    act(() => reportChannelState(REMOTE_CHANNEL, true, 'host-1'))
    await settle()
    expect(bridge.listCalls).toHaveLength(1)

    act(() => getHostRuntime('host-1')!.getState().setStatus('up'))
    await settle()
    expect(bridge.opened.map((c) => c.base)).toEqual([REMOTE_TERMINALS])
    expect(result.current.connection).toBe(bridge.opened[0])
    expect(onEnded).not.toHaveBeenCalled()
  })

  it('retries a local terminal when the local daemon reconnects, not when a remote host does', async () => {
    loadProjects(projectOn('p-local', 'local'))
    getOrCreateHostRuntime('host-1').getState().setStatus('down')
    bridge.failList = true
    const { settle } = render({ workspaceId: 'ws-local' })
    await settle()
    expect(bridge.listCalls).toHaveLength(1)

    act(() => reportChannelState(REMOTE_CHANNEL, true, 'host-1'))
    act(() => getHostRuntime('host-1')!.getState().setStatus('up'))
    await settle()
    expect(bridge.listCalls).toHaveLength(1)

    act(() => reportChannelState('/v0/projects', true, 'local'))
    await settle()
    expect(bridge.opened.map((c) => c.base)).toEqual([LOCAL_TERMINALS])
  })

  it('revoking the host lets go of its transport and attaches nowhere else, until it is paired back', async () => {
    loadProjects(projectOn('p-remote', 'host-1'))
    const { result, settle } = render({ workspaceId: 'ws-remote' })
    await settle()
    const remote = bridge.opened[0]

    act(() => useHostConfigStore.getState().removeHost('host-1'))
    await settle()

    expect(remote.close).toHaveBeenCalled()
    expect(result.current.connection).toBeNull()
    expect(bridge.listCalls).toEqual([REMOTE_TERMINALS])
    expect(bridge.opened).toHaveLength(1)

    act(() => pairRemoteHost('host-1'))
    await settle()
    expect(bridge.opened.map((c) => c.base)).toEqual([REMOTE_TERMINALS, REMOTE_TERMINALS])
    expect(result.current.connection).toBe(bridge.opened[1])
  })

  it('opens nothing for a host revoked while the daemon was being asked', async () => {
    loadProjects(projectOn('p-remote', 'host-1'))
    let release = () => {}
    bridge.listGate = new Promise<void>((r) => {
      release = r
    })
    const { result, settle } = render({ workspaceId: 'ws-remote' })
    await settle()
    expect(bridge.listCalls).toEqual([REMOTE_TERMINALS])

    act(() => useHostConfigStore.getState().removeHost('host-1'))
    release()
    await settle()

    expect(bridge.opened).toHaveLength(0)
    expect(result.current.connection).toBeNull()
  })

  // A PTY the daemon has made is the tab's, whether or not it could be opened: forgetting
  // it would have the next attach make a second one and leave the first running unseen.
  it('remembers a PTY created as the host was revoked, and re-attaches it when paired back', async () => {
    loadProjects(projectOn('p-remote', 'host-1'))
    useTerminalStore.setState({ sessions: new Map() })
    let created = () => {}
    bridge.createGate = new Promise<void>((r) => {
      created = r
    })
    const { result, settle } = render({ workspaceId: 'ws-remote' })
    await settle()

    act(() => useHostConfigStore.getState().removeHost('host-1'))
    created()
    await settle()

    expect(bridge.opened).toHaveLength(0)
    expect(useTerminalStore.getState().getSession('tab-1')?.connectionId).toBe('fresh-1')

    bridge.live = [...bridge.live, 'fresh-1'] // the daemon is running the PTY it made
    act(() => pairRemoteHost('host-1'))
    await settle()

    expect(bridge.created).toBe(1)
    expect(bridge.opened.map((c) => c.sessionId)).toEqual(['fresh-1'])
    expect(result.current.connection).toBe(bridge.opened[0])
  })

  it('tells the user instead of letting a refused transport escape the attach', async () => {
    loadProjects(projectOn('p-remote', 'host-1'))
    bridge.refuseOpen = true
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const toasted = vi.spyOn(toast, 'error').mockReturnValue('')
    const { result, settle } = render({ workspaceId: 'ws-remote' })

    await settle()

    expect(toasted).toHaveBeenCalledWith('Terminal unavailable', expect.any(String))
    expect(result.current.connection).toBeNull()
  })
})
