import { act, render, screen } from '@testing-library/react'
import { createElement, createRef } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const bridge = vi.hoisted(() => ({
  opened: [] as Array<{ sessionId: string; base: string }>,
  live: [] as string[],
  unreachable: false,
  hung: false,
  written: [] as string[],
}))

vi.mock('@/lib/crowbar-bridge', () => ({
  isTauri: () => false,
  openTerminal: (sessionId: string, base: string) => {
    bridge.opened.push({ sessionId, base })
    return {
      sessionId,
      alive: true,
      close: vi.fn(),
      onDrop: () => () => {},
      listen: () => () => {},
      setTheme: vi.fn(),
      resize: vi.fn(),
      write: vi.fn((data: string) => bridge.written.push(data)),
    }
  },
  terminalListLive: async () => {
    if (bridge.hung) return new Promise<string[]>(() => {})
    if (bridge.unreachable) throw new Error('cannot reach the host')
    return [...bridge.live]
  },
  terminalCreate: async () => 'fresh',
}))

import { XtermTerminal, type TerminalFocusHandle } from '@/features/terminal/components/terminal'
import { useTerminalStore } from '@/features/terminal/stores/terminal-store'
import { hostDotLabel } from '@/features/hosts/lib/host-tone'
import { getOrCreateHostRuntime } from '@/features/hosts/stores/host-runtime-registry'
import { reportChannelState, useConnectionStore } from '@/lib/ws/connection-store'
import { recordWorkspaceScope } from '@/lib/workspace-scope'
import {
  loadProjects,
  pairRemoteHost,
  projectOn,
  resetHostWorld,
} from '@/__tests__/__fixtures__/remote-host'
import { useWorkspaceScopeStore } from '@/lib/workspace-scope-store'

beforeEach(() => {
  bridge.opened.length = 0
  bridge.live = []
  bridge.unreachable = false
  bridge.hung = false
  bridge.written = []
  localStorage.clear()
  useTerminalStore.setState({ sessions: new Map() })
  useWorkspaceScopeStore.setState(useWorkspaceScopeStore.getInitialState())
  resetHostWorld()
})

async function mount(props: Record<string, unknown>) {
  let result!: ReturnType<typeof render>
  await act(async () => {
    result = render(
      createElement(XtermTerminal, { sessionId: 'tab-1', isActive: true, ...props } as never),
    )
  })
  return result
}

describe('XtermTerminal', () => {
  it("waits for its workspace's owning chat before attaching, then attaches under it", async () => {
    await mount({ workspaceId: 'ws-2' })
    expect(bridge.opened).toHaveLength(0)

    await act(async () => {
      recordWorkspaceScope({ projectId: 'p', repoId: 'r', wsId: 'ws-2', owningChatId: 'chat-2' })
    })
    expect(bridge.opened).toEqual([{ sessionId: 'fresh', base: '/v0/chats/chat-2/terminals' }])
  })

  it('a never-shown tab attaches nothing until it is first shown', async () => {
    recordWorkspaceScope({ projectId: 'p', repoId: 'r', wsId: 'ws-1', owningChatId: 'chat-1' })
    const { rerender } = await mount({ workspaceId: 'ws-1', isVisible: false })
    expect(bridge.opened).toHaveLength(0)
    await act(async () => {
      rerender(
        createElement(XtermTerminal, {
          sessionId: 'tab-1',
          isActive: true,
          workspaceId: 'ws-1',
          isVisible: true,
        }),
      )
    })
    expect(bridge.opened).toHaveLength(1)
  })

  it("routes a shell tab's ended session to onTerminalExit", async () => {
    recordWorkspaceScope({ projectId: 'p', repoId: 'r', wsId: 'ws-1', owningChatId: 'chat-1' })
    useTerminalStore.getState().updateSession('tab-1', { connectionId: 'pty-gone' })
    const onTerminalExit = vi.fn()
    const onSessionGone = vi.fn()
    await mount({ workspaceId: 'ws-1', onTerminalExit, onSessionGone })
    expect(onTerminalExit).toHaveBeenCalledExactlyOnceWith('tab-1')
    expect(onSessionGone).not.toHaveBeenCalled()
    expect(bridge.opened).toHaveLength(0)
  })

  it("routes an agent view's ended session to onSessionGone", async () => {
    const onTerminalExit = vi.fn()
    const onSessionGone = vi.fn()
    await mount({
      sessionId: 'agent-pty',
      workspaceId: 'ws-1',
      chatId: 'chat-9',
      attachOnly: true,
      onTerminalExit,
      onSessionGone,
    })
    expect(onSessionGone).toHaveBeenCalledExactlyOnceWith('agent-pty')
    expect(onTerminalExit).not.toHaveBeenCalled()
  })

  it('hands its owner a focus handle, and takes it back on unmount', async () => {
    const ref = createRef<TerminalFocusHandle>()
    const { unmount } = await mount({
      sessionId: 'agent-pty',
      workspaceId: 'ws-1',
      chatId: 'chat-9',
      attachOnly: true,
      ref,
    })
    expect(ref.current?.focus).toBeTypeOf('function')
    unmount()
    expect(ref.current).toBeNull()
  })

  it('hands its owner a handle that sends input to the session', async () => {
    recordWorkspaceScope({ projectId: 'p', repoId: 'r', wsId: 'ws-1', owningChatId: 'chat-1' })
    const ref = createRef<TerminalFocusHandle>()
    await mount({ workspaceId: 'ws-1', ref })
    await act(async () => ref.current?.send('\x1b[A'))
    expect(bridge.written).toEqual(['\x1b[A'])
  })

  describe('while its host is not there', () => {
    function remoteWorkspace(status: 'up' | 'down' | 'needs-repair') {
      pairRemoteHost('h1', 'Build box')
      loadProjects(projectOn('p', 'h1'))
      recordWorkspaceScope({ projectId: 'p', repoId: 'r', wsId: 'ws-r', owningChatId: 'chat-r' })
      getOrCreateHostRuntime('h1').getState().setStatus(status)
      useTerminalStore.getState().updateSession('tab-1', { connectionId: 'pty-1' })
      bridge.unreachable = true
    }

    it('names the host that is down, and goes once the terminal attaches', async () => {
      remoteWorkspace('down')
      await mount({ workspaceId: 'ws-r' })

      expect(screen.getByRole('status').textContent).toBe(
        `Build box is not available right now${hostDotLabel('down')}`,
      )
      bridge.unreachable = false
      bridge.live = ['pty-1']
      await act(async () => getOrCreateHostRuntime('h1').getState().setStatus('up'))
      expect(bridge.opened).toEqual([
        { sessionId: 'pty-1', base: 'crowbar://h1/v0/chats/chat-r/terminals' },
      ])
      expect(screen.queryByRole('status')).toBeNull()
    })

    it('says nothing over a terminal still attached when a probe finds its host down', async () => {
      remoteWorkspace('down')
      bridge.unreachable = false
      bridge.live = ['pty-1']
      getOrCreateHostRuntime('h1').getState().setStatus('up')
      await mount({ workspaceId: 'ws-r' })
      expect(bridge.opened).toHaveLength(1)

      await act(async () => getOrCreateHostRuntime('h1').getState().setStatus('down'))
      expect(screen.queryByRole('status')).toBeNull()
    })

    it('says a host needing repair is not available, and why', async () => {
      remoteWorkspace('needs-repair')
      await mount({ workspaceId: 'ws-r' })
      expect(screen.getByRole('status').textContent).toBe(
        `Build box is not available right now${hostDotLabel('needs-repair')}`,
      )
    })

    it('says nothing while a terminal on an up host is attaching', async () => {
      remoteWorkspace('up')
      bridge.unreachable = false
      bridge.hung = true
      await mount({ workspaceId: 'ws-r' })

      expect(bridge.opened).toHaveLength(0)
      expect(screen.queryByRole('status')).toBeNull()
    })

    it('says a host with no live stream cannot attach this terminal', async () => {
      remoteWorkspace('up')
      reportChannelState('crowbar://h1/v0/projects', false, 'h1')
      try {
        await mount({ workspaceId: 'ws-r' })

        expect(screen.getByRole('status').textContent).toBe(
          'Build box: no live stream — this terminal can’t attach',
        )
      } finally {
        useConnectionStore.setState(useConnectionStore.getInitialState())
      }
    })

    it('says it waits while the workspace’s host is not known yet', async () => {
      pairRemoteHost('h1', 'Build box')
      recordWorkspaceScope({ projectId: 'p', repoId: 'r', wsId: 'ws-r', owningChatId: 'chat-r' })
      await mount({ workspaceId: 'ws-r' })
      expect(screen.getByRole('status').textContent).toBe('Waiting for this terminal’s host…')
      expect(bridge.opened).toHaveLength(0)
    })

    it('says nothing for a Local terminal, attached or not', async () => {
      await mount({ workspaceId: 'ws-2' })
      expect(bridge.opened).toHaveLength(0)
      expect(screen.queryByRole('status')).toBeNull()
    })
  })
})
