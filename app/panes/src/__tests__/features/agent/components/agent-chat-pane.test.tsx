import { createElement } from 'react'
import { act, cleanup, render, screen } from '@testing-library/react'
import { platform } from '@/__tests__/__fixtures__/pane-platform'
import { describe, expect, it, vi } from 'vitest'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { heldScopes } from '@/__tests__/__fixtures__/stream-daemon'
import { wsManager } from '@/lib/ws/manager'
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

  // /clear in the CLI moves the running process to another chat: the tab follows its runner, and
  // the terminal, keyed by the PTY a move does not change, stays mounted.
  it('follows its runner to a new chat without remounting the terminal', async () => {
    const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
    const paneId = openChatPane(store, 'c1', 'r1')
    await renderPane(store, paneId)

    const term = await screen.findByTestId('xterm')
    expect(term).toHaveAttribute('data-session-id', 'pty1')

    // The runner /clears into a brand-new chat — carrying the SAME pty.
    await act(async () => {
      seedChats([
        dormantChat({ id: 'c1' }),
        liveChat({ id: 'c2', runnerId: 'r1', pty: 'pty1', title: 'Fresh' }),
      ])
    })

    // The surface follows the runner at once; the PANE record is retargeted by
    // the stream's `moved` frame alone (one writer), never by this component.
    expect(paneOf(store, paneId)).toMatchObject({ chatId: 'c1', runnerId: 'r1' })
    expect(await screen.findByTestId('xterm')).toBe(term)
    await act(async () => {
      windowPaneStore.getState().paneActions.retargetPane(paneId, 'c2', 'r1')
    })
    expect(paneOf(store, paneId)).toMatchObject({ chatId: 'c2', runnerId: 'r1' })
    // ...while the terminal is the SAME DOM NODE. Not a remount: the very same
    // xterm instance, still attached to the same live PTY.
    expect(await screen.findByTestId('xterm')).toBe(term)
    expect(screen.queryByText(/this agent has exited/i)).not.toBeInTheDocument()
  })

  // Losing your runner because it MOVED is not your CLI dying. The old pane could
  // not tell those apart, so it offered a Resume button that spawned a SECOND CLI
  // on the old conversation while the first kept running.
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
    // Chat is the default presentation. The PTY remains attach-only and mounted,
    // but xterm may neither focus nor resize while it is behind Chat.
    expect(xterm.getAttribute('data-active')).toBe('false')
    expect(xterm.getAttribute('data-visible')).toBe('false')
    // Attach-only: a reconnect can never spawn a bare shell into the agent frame.
    expect(xterm.getAttribute('data-attach-only')).toBe('true')
    // liveRunnerId IS the liveness answer — no second round trip asks the daemon.
  })

  it('renders nothing (not the exited state) while the chat list is still loading', async () => {
    // The seed is in flight: the store does not know this chat yet. "Not known" is
    // not "dormant" — flashing Resume here would offer a button that spawns a
    // second CLI onto a chat that may well be live.
    //
    // NO SEED HAS RUN, which is the actual condition being described. Seeding an
    // empty list is a different fact — the daemon answering "there are none" —
    // and a pane pointed at a chat that answer does not carry resolves it rather
    // than waiting (see agent-chat-pane-unknown-chat-wedge.test.tsx).
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

  // Holding the chat's stream scope is what counts a viewer: no socket of its own.
  it('views a chat through its stream scope alone', async () => {
    const subscribe = vi.spyOn(wsManager, 'subscribe')
    const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
    await renderPane(store, openChatPane(store, 'c1', 'r1'))
    expect(heldScopes('chat:')).toContain('chat:c1')
    expect(subscribe.mock.calls.flat().join(' ')).not.toMatch(/\/attach\b/)
    subscribe.mockRestore()
  })

  // The feed is addressed by chat id alone, so no workspace has to vouch for it.
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

    // Chat is selected, so xterm is neither active nor visible regardless of pane focus.
    const xterm = await screen.findByTestId('xterm')
    expect(xterm.getAttribute('data-active')).toBe('false')
    expect(xterm.getAttribute('data-visible')).toBe('false')
  })

  // ── Adopting a new runner on the same chat ─────────────────────────
  // A runner replacement lands a new PTY on the same chat. The pane adopts it without remounting:
  // XtermTerminal swaps the attachment imperatively on the same DOM node instead of tearing down
  // socket, listeners and observers.
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
    expect(after).toBe(before) // SAME node: the attachment swapped, the terminal did not remount
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

  // The banner and the composer's signpost render off the same `waiting` signal;
  // once a composer can show it, the signpost is its one occupant. The blank-chat
  // half is in agent-chat-pane-terminal-wait.test.tsx.
  describe('terminal_wait on a chat that already has messages', () => {
    it('mutates the composer into a signpost instead of duplicating a pane-level banner', async () => {
      const store = seedWorkspace([
        liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1', provider: 'provider-a' }),
      ])
      await renderPane(store, openChatPane(store, 'c1', 'r1'))
      // The default beforeEach seeds one finished turn, so this chat
      // is NOT blank — AgentComposer, not AgentEmptyDocument, is mounted.
      expect(screen.getByRole('textbox', { name: /message the agent/i })).toBeInTheDocument()

      await act(async () => {
        setChatTerminalWait('c1', { kind: 'workspace_trust' })
      })

      // The composer itself became the signpost...
      expect(screen.getByText(/waiting for you to trust the workspace/i)).toBeInTheDocument()
      expect(screen.getByRole('button', { name: 'Terminal' })).toBeInTheDocument()
      // ...and the separate pane-level banner did NOT also render.
      expect(screen.queryByTestId('agent-terminal-wait')).not.toBeInTheDocument()
      // The input itself is gone — one occupant, not an input rendered dead
      // beneath the question.
      expect(screen.queryByRole('textbox', { name: /message the agent/i })).not.toBeInTheDocument()
    })
  })
})
