import { createElement } from 'react'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/lib/errors/api-error'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { chatStore } from '@/features/agent/stores/chat-store'
import { ledger, seedFeed, spokenIn } from '@/__tests__/__fixtures__/ledger-rows'
import { AgentChatPaneGate as AgentChatPane } from '@/features/agent/components/agent-chat-pane-gate'
import { seedChats, writeChat, setProviders } from '@/__tests__/__fixtures__/agent-chat'
import {
  dormantChat,
  landOnTerminal,
  liveChat,
  liveChatNoTerminal,
  openChatPane,
  providers,
  renderPane,
  resumeChatFn,
  seedWorkspace,
  startingChat,
  submitPromptFn,
  switchToTerminalFn,
  toastErrorFn,
  typeAndSend,
  type Store,
} from '@/__tests__/__fixtures__/agent-chat-pane'

vi.mock(
  '@/utils/platform',
  async () => (await import('@/__tests__/__fixtures__/pane-platform')).platformModule,
)

describe('AgentChatPane liveness and lifecycle', () => {
  // ── Liveness is the daemon's answer; the pane never infers or revives ──
  describe('liveness', () => {
    it('symptom 5: the composer accepts a prompt while the runner is spawning', async () => {
      const store = seedWorkspace([startingChat({ id: 'c1' })])
      seedFeed('c1', spokenIn('c1'))
      await renderPane(store, openChatPane(store, 'c1', ''))

      typeAndSend('while it starts')

      await vi.waitFor(() => expect(submitPromptFn).toHaveBeenCalledTimes(1))
      expect(submitPromptFn.mock.calls[0]?.[2]).toBe('while it starts')
    })

    it('never revives from the pending state (the chat list has not landed)', async () => {
      const store = seedWorkspace([]) // the seed is still in flight
      const paneId = openChatPane(store, 'c1', 'r1')
      await renderPane(store, paneId)

      // "Not known" is not "dormant". Reviving here would spawn a SECOND CLI onto a
      // chat that may well already have one.
      expect(resumeChatFn).not.toHaveBeenCalled()
      expect(screen.queryByText(/resuming this chat/i)).not.toBeInTheDocument()

      // ...and the moment the list lands with the chat LIVE, it attaches — no revive.
      await act(async () => {
        seedChats([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      })
      expect(resumeChatFn).not.toHaveBeenCalled()
      expect(await screen.findByTestId('xterm')).toHaveAttribute('data-session-id', 'pty1')
    })

    it('does not revive a chat whose runner merely moved away', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      await renderPane(store, openChatPane(store, 'c1', 'r1'))

      // c1 is now dormant — but its runner is not dead, it walked into c2, and the pane
      // walks with it. A dormant chat NOBODY IS LOOKING AT must not be revived.
      await act(async () => {
        seedChats([dormantChat({ id: 'c1' }), liveChat({ id: 'c2', runnerId: 'r1', pty: 'pty1' })])
      })

      expect(resumeChatFn).not.toHaveBeenCalled()
      expect(screen.getByTestId('xterm')).toHaveAttribute('data-session-id', 'pty1')
    })

    // An api-transport runner is live with an empty terminalSessionId until switched to its
    // TUI. Only liveRunnerId may mean "no runner"; reviving here would race a healthy runner and
    // a failed second resume would latch the chat into `idle: failed`.
    it('does not revive a live runner that simply has no terminal to attach', async () => {
      const store = seedWorkspace([liveChatNoTerminal({ id: 'c1', runnerId: 'r1' })])
      await renderPane(store, openChatPane(store, 'c1', 'r1'))

      expect(resumeChatFn).not.toHaveBeenCalled()
      expect(screen.queryByTestId('pane-resume')).not.toBeInTheDocument()
      expect(screen.queryByText(/could not restart this agent/i)).not.toBeInTheDocument()
      expect(screen.queryByTestId('xterm')).toBeNull()
    })

    // The terminal surface's own placeholder for this same state, visible once the
    // user actually looks at the terminal view for a runner with nothing attached.
    it('shows a plain placeholder, never Resume, in the terminal view of a live runner with no terminal', async () => {
      landOnTerminal()
      const store = seedWorkspace([liveChatNoTerminal({ id: 'c1', runnerId: 'r1' })])
      await renderPane(store, openChatPane(store, 'c1', 'r1'))

      expect(screen.queryByTestId('pane-resume')).not.toBeInTheDocument()
      expect(screen.queryByTestId('xterm')).toBeNull()
      expect(screen.getByText(/no terminal view attached/i)).toBeTruthy()
    })

    // The fixture default is hotswap, which never calls the endpoint; a non-hotswap
    // provider must ask the daemon for its terminal view.
    it('calls switchToTerminal (never a direct presentation flip) when an idle non-hotswap provider is asked for its terminal', async () => {
      const store = seedWorkspace([liveChatNoTerminal({ id: 'c1', runnerId: 'r1' })])
      setProviders([providers[0], { ...providers[1], hotswap: false }])
      // The daemon's snapshot frame carries the forked PTY; the pane reads nothing back.
      switchToTerminalFn.mockImplementation(async () => {
        await Promise.resolve()
        writeChat(
          liveChat({
            id: 'c1',
            runnerId: 'r1',
            pty: 'p',
            provider: 'provider-b',
            surface: 'terminal',
          }),
        )
      })
      await renderPane(store, openChatPane(store, 'c1', 'r1'))

      fireEvent.click(screen.getByRole('tab', { name: /^terminal$/i }))

      // Not a synchronous flip like the hotswap case — the tab does not read
      // selected until the switch actually resolves.
      expect(screen.getByRole('tab', { name: /^terminal$/i })).toHaveAttribute(
        'aria-selected',
        'false',
      )
      await vi.waitFor(() => expect(switchToTerminalFn).toHaveBeenCalledWith('w1', 'c1'))
      expect(await screen.findByTestId('xterm')).toHaveAttribute('data-session-id', 'p')
      await vi.waitFor(() =>
        expect(screen.getByRole('tab', { name: /^terminal$/i })).toHaveAttribute(
          'aria-selected',
          'true',
        ),
      )
    })

    // A refused switch (a turn still in flight: attach.go's ErrTurnInProgress)
    // must say so; a click that does nothing visible reads as broken.
    it('toasts when switchToTerminal is refused, instead of silently doing nothing', async () => {
      const store = seedWorkspace([liveChatNoTerminal({ id: 'c1', runnerId: 'r1' })])
      setProviders([providers[0], { ...providers[1], hotswap: false }])
      switchToTerminalFn.mockRejectedValue(
        new ApiError('agent: provider cannot hand a live turn to its native view: conflict', 409),
      )
      await renderPane(store, openChatPane(store, 'c1', 'r1'))

      fireEvent.click(screen.getByRole('tab', { name: /^terminal$/i }))

      await vi.waitFor(() => expect(toastErrorFn).toHaveBeenCalledTimes(1))
      const [title] = toastErrorFn.mock.calls[0] as [string, string]
      expect(title).toContain('Provider B')
      expect(screen.queryByTestId('xterm')).toBeNull()
      expect(screen.getByRole('tab', { name: /^terminal$/i })).toHaveAttribute(
        'aria-selected',
        'false',
      )
    })
  })

  describe('header clearance (overlay chat-blur header)', () => {
    function renderBelowOverlayHeader(store: Store, chatId: string, runnerId: string) {
      const paneId = openChatPane(store, chatId, runnerId)
      return act(() =>
        render(
          createElement(
            WorkspaceStoreContext.Provider,
            { value: store },
            createElement(AgentChatPane, {
              chatId,
              runnerId,
              wsId: 'w1',
              paneId,
              isActivePane: true,
              isVisible: true,
              belowOverlayHeader: true,
            }),
          ),
        ),
      )
    }

    it('clears the header for the reviving signpost when an overlay header sits above', async () => {
      chatStore.setState({ feeds: {} })

      const store = seedWorkspace([startingChat({ id: 'c1' })])
      await renderBelowOverlayHeader(store, 'c1', '')

      await screen.findByTestId('agent-reviving-banner')
      const section = document.querySelector('.agent-chat.chat') as HTMLElement
      expect(section.style.getPropertyValue('--agent-header-clearance')).toBe('50px')
    })

    it('still clears the header inside AgentChatView once the chat is attached and no banner covers it', async () => {
      chatStore.setState({ feeds: {} })

      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      await renderBelowOverlayHeader(store, 'c1', 'r1')

      await screen.findByTestId('agent-empty-document')
      expect(screen.queryByTestId('agent-idle-banner')).not.toBeInTheDocument()
      expect(screen.queryByTestId('agent-reviving-banner')).not.toBeInTheDocument()
      const section = document.querySelector('.agent-chat.chat') as HTMLElement
      expect(section.style.getPropertyValue('--agent-header-clearance')).toBe('50px')
    })

    // The transcript needs to clear the header's FULL EdgeDissolve zone
    // (ROW_HEIGHT_PX + CHAT_BLUR_EXTRA_PX = 98px Mac), not just the 50px
    // click-target the banners above clear — text left resting between the
    // two still renders visibly blurred by the dissolve's own mask layers.
    // See CHAT_BLUR_ZONE_PX in agent-chat-pane.tsx.
    it('hands the transcript its OWN, larger clearance — the full dissolve zone, not the banner click-target', async () => {
      chatStore.setState({ feeds: {} })

      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      await renderBelowOverlayHeader(store, 'c1', 'r1')

      await screen.findByTestId('agent-empty-document')
      const section = document.querySelector('.agent-chat.chat') as HTMLElement
      expect(section.style.getPropertyValue('--agent-transcript-header-clearance')).toBe('98px')
    })

    it('gives the transcript no extra clearance with no overlay header above', async () => {
      chatStore.setState({ feeds: {} })

      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      await renderPane(store, openChatPane(store, 'c1', 'r1'))

      await screen.findByTestId('agent-empty-document')
      const section = document.querySelector('.agent-chat.chat') as HTMLElement
      expect(section.style.getPropertyValue('--agent-transcript-header-clearance')).toBe('0px')
    })
  })

  // ── The "second input box" shape ────────────────────────────────────
  // The signpost wears ComposerSignpost's `.pill.halted` shape (as AgentComposer does once the
  // chat has messages): one truncated line, so it cannot wrap into the handle below it.
  describe('reviving signpost shape', () => {
    it('renders the reviving banner in the same pill shape', async () => {
      chatStore.setState({ feeds: {} })

      const store = seedWorkspace([startingChat({ id: 'c1' })])
      await renderPane(store, openChatPane(store, 'c1', ''))

      const banner = await screen.findByTestId('agent-reviving-banner')
      expect(banner.querySelector('.pill.halted')).not.toBeNull()
    })
  })

  // ── Keep-alive: a hidden tab stays mounted and spawns nothing ──────
  describe('hidden keep-alive tab', () => {
    // isVisible=false is the hidden tab; isActivePane=true proves the gate is on
    // VISIBILITY, not pane focus — a dormant chat sitting hidden inside the active pane
    // still must not spawn a CLI (Risk #4: the two flags are distinct).
    it('does not revive a hidden dormant chat', async () => {
      const store = seedWorkspace([dormantChat({ id: 'c1' })])
      const paneId = openChatPane(store, 'c1', '')
      await act(async () => {
        render(
          createElement(
            WorkspaceStoreContext.Provider,
            { value: store },
            createElement(AgentChatPane, {
              chatId: 'c1',
              runnerId: '',
              wsId: 'w1',
              paneId,
              isActivePane: true,
              isVisible: false,
            }),
          ),
        )
      })

      // Nothing spawned, and no spinner offering to — the chat just waits, hidden.
      expect(resumeChatFn).not.toHaveBeenCalled()
      expect(screen.queryByText(/resuming this chat/i)).not.toBeInTheDocument()
      expect(screen.queryByTestId('xterm')).toBeNull()
    })

    // Keep-alive's other half: an ALREADY-ATTACHED chat keeps its live PTY while hidden.
    // It has a sessionId, so it never reaches the revive gate — it seeds/attaches as
    // usual, just not focused and not visible.
    it('keeps an attached chat mounted while hidden (no revive, terminal stays)', async () => {
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
              isVisible: false,
            }),
          ),
        )
      })

      const xterm = await screen.findByTestId('xterm')
      expect(xterm).toHaveAttribute('data-session-id', 'pty1')
      expect(xterm.getAttribute('data-visible')).toBe('false')
      expect(xterm.getAttribute('data-active')).toBe('false')
      expect(resumeChatFn).not.toHaveBeenCalled()
    })
  })

  // ── The daemon owns the lifecycle; the pane only sends intents ──────
  describe('server-owned lifecycle', () => {
    it('never resumes a dormant chat on open, and still takes a message', async () => {
      const store = seedWorkspace([dormantChat({ id: 'c1' })])
      await renderPane(store, openChatPane(store, 'c1', ''))

      expect(resumeChatFn).not.toHaveBeenCalled()
      expect(screen.queryByTestId('agent-session-note')).toBeNull()
      typeAndSend('wake up')
      await waitFor(() => expect(submitPromptFn).toHaveBeenCalledTimes(1))
      expect(resumeChatFn).not.toHaveBeenCalled()
    })

    it.each(['stopped', 'retracted', 'resume_failed', 'daemon_restart'])(
      'says nothing about the last exit (%s): the next message continues',
      async (reason) => {
        const l = ledger('c1', 4)
        const exited = l.row('runner_exited', { runnerId: 'r1', reason, exitCode: 0 })
        seedFeed('c1', [...spokenIn('c1'), exited])
        const store = seedWorkspace([dormantChat({ id: 'c1' })])
        await renderPane(store, openChatPane(store, 'c1', ''))
        expect(screen.queryByTestId('agent-session-note')).toBeNull()
        expect(screen.queryByText(/Send a message to/)).toBeNull()
      },
    )

    it('starts a session from the terminal surface only when asked', async () => {
      landOnTerminal()
      const store = seedWorkspace([dormantChat({ id: 'c1' })])
      await renderPane(store, openChatPane(store, 'c1', ''))

      expect(resumeChatFn).not.toHaveBeenCalled()
      await act(async () => {
        fireEvent.click(screen.getByRole('button', { name: /start session/i }))
      })
      expect(resumeChatFn).toHaveBeenCalledWith('w1', 'c1')
    })

    it('toasts when starting a session fails', async () => {
      landOnTerminal()
      resumeChatFn.mockRejectedValue(new ApiError('terminal: command not found: provider-b', 424))
      const store = seedWorkspace([dormantChat({ id: 'c1' })])
      await renderPane(store, openChatPane(store, 'c1', ''))

      await act(async () => {
        fireEvent.click(screen.getByRole('button', { name: /start session/i }))
      })
      await waitFor(() => expect(toastErrorFn).toHaveBeenCalledTimes(1))
    })
  })
})
