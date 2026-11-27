import { createElement } from 'react'
import { act, cleanup, render, screen } from '@testing-library/react'
import { platform } from '@/__tests__/__fixtures__/pane-platform'
import { describe, expect, it, vi } from 'vitest'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { heldScopes } from '@/__tests__/__fixtures__/stream-daemon'
import { wsManager } from '@/core/stream/ws-manager'
import { AgentChatPaneGate as AgentChatPane } from '@/features/agent/components/agent-chat-pane-gate'
import { useZoomStore } from '@/features/window/stores/zoom-store'
import { seedChats, setChatTerminalWait } from '@/__tests__/__fixtures__/agent-chat'
import {
  dormantChat,
  landOnTerminal,
  liveChat,
  openChatPane,
  paneOf,
  renderPane,
  resumeChatFn,
  seedWorkspace,
  unseededWorkspace,
} from '@/__tests__/__fixtures__/agent-chat-pane'

vi.mock(
  '@/utils/platform',
  async () => (await import('@/__tests__/__fixtures__/pane-platform')).platformModule,
)

describe('AgentChatPane', () => {
  it('drops the left-only chat gutter on the phone', async () => {
    const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
    const paneId = openChatPane(store, 'c1', 'r1')
    await renderPane(store, paneId)
    expect(document.querySelector('.agent-chat-pane')).not.toHaveAttribute('data-touch')
    cleanup()

    platform.mobile = true
    await renderPane(store, paneId)
    expect(document.querySelector('.agent-chat-pane')).toHaveAttribute('data-touch')
  })

  it('follows its runner to a new chat without remounting the terminal', async () => {
    const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
    const paneId = openChatPane(store, 'c1', 'r1')
    await renderPane(store, paneId)

    const term = await screen.findByTestId('xterm')
    expect(term).toHaveAttribute('data-session-id', 'pty1')

    await act(async () => {
      seedChats([
        dormantChat({ id: 'c1' }),
        liveChat({ id: 'c2', runnerId: 'r1', pty: 'pty1', title: 'Fresh' }),
      ])
    })

    expect(paneOf(store, paneId)).toMatchObject({ chatId: 'c1', runnerId: 'r1' })
    expect(await screen.findByTestId('xterm')).toBe(term)
    await act(async () => {
      windowPaneStore.getState().paneActions.retargetPane(paneId, 'c2', 'r1')
    })
    expect(paneOf(store, paneId)).toMatchObject({ chatId: 'c2', runnerId: 'r1' })
    expect(await screen.findByTestId('xterm')).toBe(term)
    expect(screen.queryByText(/this agent has exited/i)).not.toBeInTheDocument()
  })

  it('does not show the exited state when the runner merely moved', async () => {
    const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
    const paneId = openChatPane(store, 'c1', 'r1')
    await renderPane(store, paneId)

    await act(async () => {
      seedChats([dormantChat({ id: 'c1' }), liveChat({ id: 'c2', runnerId: 'r1', pty: 'pty1' })])
    })

    expect(screen.queryByText(/this agent has exited/i)).not.toBeInTheDocument()
    expect(screen.queryByTestId('pane-resume')).not.toBeInTheDocument()
    expect(resumeChatFn).not.toHaveBeenCalled()
    expect(screen.getByTestId('xterm')).toBeTruthy()
  })

  it('attaches the live runner PTY: mounts an attach-only terminal onto it', async () => {
    const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
    const paneId = openChatPane(store, 'c1', 'r1')
    await renderPane(store, paneId)

    const xterm = await screen.findByTestId('xterm')
    expect(xterm.getAttribute('data-session-id')).toBe('pty1')
    expect(xterm.getAttribute('data-active')).toBe('false')
    expect(xterm.getAttribute('data-visible')).toBe('false')
    expect(xterm.getAttribute('data-attach-only')).toBe('true')
  })

  it('renders nothing (not the exited state) while the chat list is still loading', async () => {
    const store = unseededWorkspace()
    const paneId = openChatPane(store, 'c1', 'r1')
    await renderPane(store, paneId)

    expect(screen.queryByTestId('xterm')).toBeNull()
    expect(screen.queryByTestId('pane-resume')).not.toBeInTheDocument()
    expect(screen.queryByText(/this agent has exited/i)).not.toBeInTheDocument()
  })

  it('reads a restored chat before the chat list lands, when its pane records the workspace', async () => {
    const store = unseededWorkspace()
    const paneId = openChatPane(store, 'c1', 'r1')
    windowPaneStore.setState((s) => {
      s.panes[paneId].workspaceId = 'w1'
      return s
    })
    await renderPane(store, paneId)

    expect(heldScopes('chat:')).toContain('chat:c1')
  })

  it('views a chat through its stream scope alone', async () => {
    const subscribe = vi.spyOn(wsManager, 'subscribe')
    const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
    await renderPane(store, openChatPane(store, 'c1', 'r1'))
    expect(heldScopes('chat:')).toContain('chat:c1')
    expect(subscribe.mock.calls.flat().join(' ')).not.toMatch(/\/attach\b/)
    subscribe.mockRestore()
  })

  it('opens the feed of a chat with no recorded workspace before the list lands', async () => {
    const store = unseededWorkspace()
    const paneId = openChatPane(store, 'c1', 'r1')
    await renderPane(store, paneId)

    expect(heldScopes('chat:')).toContain('chat:c1')
  })

  it('threads isActivePane=false through to the terminal', async () => {
    const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
    const paneId = openChatPane(store, 'c1', 'r1')
    await act(async () => {
      render(
        createElement(
          WorkspaceStoreContext.Provider,
          { value: store },
          createElement(AgentChatPane, {
            chatId: 'c1',
            runnerId: 'r1',
            wsId: 'w1',
            paneId,
            isActivePane: false,
            isVisible: true,
          }),
        ),
      )
    })

    const xterm = await screen.findByTestId('xterm')
    expect(xterm.getAttribute('data-active')).toBe('false')
    expect(xterm.getAttribute('data-visible')).toBe('false')
  })

  it('adopts the chat new runner in place — new PTY, but the SAME terminal (no remount)', async () => {
    landOnTerminal()
    const store = seedWorkspace([
      liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1', provider: 'provider-b' }),
    ])
    const paneId = openChatPane(store, 'c1', 'r1')
    await renderPane(store, paneId)

    const before = await screen.findByTestId('xterm')
    expect(before).toHaveAttribute('data-session-id', 'pty1')

    await act(async () => {
      seedChats([liveChat({ id: 'c1', runnerId: 'r2', pty: 'pty2', provider: 'provider-a' })])
    })

    const after = await screen.findByTestId('xterm')
    expect(after).toHaveAttribute('data-session-id', 'pty2')
    expect(after).toBe(before)
    expect(paneOf(store, paneId)).toMatchObject({ chatId: 'c1', runnerId: 'r2' })
    expect(screen.getByTestId('provider-switch').getAttribute('data-current')).toBe('provider-a')
  })

  describe('chat zoom', () => {
    it('applies the zoom-store level as CSS zoom on the chat surface', async () => {
      useZoomStore.setState({ zoom: 1.4 })
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      const paneId = openChatPane(store, 'c1', 'r1')
      await renderPane(store, paneId)

      expect(screen.getByTestId('agent-chat-surface')).toHaveStyle({ zoom: '1.4' })
    })

    it('follows the store live as it changes', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      const paneId = openChatPane(store, 'c1', 'r1')
      await renderPane(store, paneId)

      expect(screen.getByTestId('agent-chat-surface')).toHaveStyle({ zoom: '1' })

      await act(async () => {
        useZoomStore.getState().actions.zoomIn()
      })

      expect(screen.getByTestId('agent-chat-surface')).toHaveStyle({ zoom: '1.1' })
    })
  })

  describe('terminal_wait on a chat that already has messages', () => {
    it('mutates the composer into a signpost instead of duplicating a pane-level banner', async () => {
      const store = seedWorkspace([
        liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1', provider: 'provider-a' }),
      ])
      await renderPane(store, openChatPane(store, 'c1', 'r1'))
      expect(screen.getByRole('textbox', { name: /message the agent/i })).toBeInTheDocument()

      await act(async () => {
        setChatTerminalWait('c1', { kind: 'workspace_trust' })
      })

      expect(screen.getByText(/waiting for you to trust the workspace/i)).toBeInTheDocument()
      expect(screen.getByRole('button', { name: 'Terminal' })).toBeInTheDocument()
      expect(screen.queryByTestId('agent-terminal-wait')).not.toBeInTheDocument()
      expect(screen.queryByRole('textbox', { name: /message the agent/i })).not.toBeInTheDocument()
    })
  })
})
