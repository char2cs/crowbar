import { act, render } from '@testing-library/react'
import { createElement, createRef } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

// The component wiring around the attachment: when a view may attach at all, and
// where a session's end is routed. jsdom lays nothing out, so xterm itself is
// never built here — attaching does not wait for it.
const bridge = vi.hoisted(() => ({
  opened: [] as Array<{ sessionId: string; base: string }>,
  live: [] as string[],
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
      write: vi.fn(),
    }
  },
  terminalListLive: async () => [...bridge.live],
  terminalCreate: async () => 'fresh',
}))

vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  getActiveWorkspaceId: () => 'ws-active',
}))

import { XtermTerminal, type TerminalFocusHandle } from '@/features/terminal/components/terminal'
import { useTerminalStore } from '@/features/terminal/stores/terminal-store'
import { recordWorkspaceScope, __resetWorkspaceScopesForTest } from '@/lib/workspace-scope'

beforeEach(() => {
  bridge.opened.length = 0
  bridge.live = []
  localStorage.clear()
  useTerminalStore.setState({ sessions: new Map() })
  __resetWorkspaceScopesForTest()
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
      chatId: 'chat-9',
      attachOnly: true,
      ref,
    })
    expect(ref.current?.focus).toBeTypeOf('function')
    unmount()
    expect(ref.current).toBeNull()
  })
})
