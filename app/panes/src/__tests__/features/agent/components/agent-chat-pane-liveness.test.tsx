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
      const store = seedWorkspace([])
      const paneId = openChatPane(store, 'c1', 'r1')
      await renderPane(store, paneId)

      expect(resumeChatFn).not.toHaveBeenCalled()
      expect(screen.queryByText(/resuming this chat/i)).not.toBeInTheDocument()

      await act(async () => {
        seedChats([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      })
      expect(resumeChatFn).not.toHaveBeenCalled()
      expect(await screen.findByTestId('xterm')).toHaveAttribute('data-session-id', 'pty1')
    })

    it('does not revive a chat whose runner merely moved away', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      await renderPane(store, openChatPane(store, 'c1', 'r1'))

      await act(async () => {
        seedChats([dormantChat({ id: 'c1' }), liveChat({ id: 'c2', runnerId: 'r1', pty: 'pty1' })])
      })

      expect(resumeChatFn).not.toHaveBeenCalled()
      expect(screen.getByTestId('xterm')).toHaveAttribute('data-session-id', 'pty1')
    })

    it('does not revive a live runner that simply has no terminal to attach', async () => {
      const store = seedWorkspace([liveChatNoTerminal({ id: 'c1', runnerId: 'r1' })])
      await renderPane(store, openChatPane(store, 'c1', 'r1'))

      expect(resumeChatFn).not.toHaveBeenCalled()
      expect(screen.queryByTestId('pane-resume')).not.toBeInTheDocument()
      expect(screen.queryByText(/could not restart this agent/i)).not.toBeInTheDocument()
      expect(screen.queryByTestId('xterm')).toBeNull()
    })

    it('shows a plain placeholder, never Resume, in the terminal view of a live runner with no terminal', async () => {
      landOnTerminal()
      const store = seedWorkspace([liveChatNoTerminal({ id: 'c1', runnerId: 'r1' })])
      await renderPane(store, openChatPane(store, 'c1', 'r1'))

      expect(screen.queryByTestId('pane-resume')).not.toBeInTheDocument()
      expect(screen.queryByTestId('xterm')).toBeNull()
      expect(screen.getByText(/no terminal view attached/i)).toBeTruthy()
    })

    it('calls switchToTerminal (never a direct presentation flip) when an idle non-hotswap provider is asked for its terminal', async () => {
      const store = seedWorkspace([liveChatNoTerminal({ id: 'c1', runnerId: 'r1' })])
      setProviders([providers[0], { ...providers[1], hotswap: false }])
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

    it('hands the transcript its OWN, larger clearance — the full dissolve zone, not the banner click-target', async () => {
      chatStore.setState({ feeds: {} })

      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      await renderBelowOverlayHeader(store, 'c1', 'r1')

      await screen.findByTestId('agent-empty-document')
      const section = document.querySelector('.agent-chat.chat') as HTMLElement
      expect(section.style.getPropertyValue('--agent-transcript-header-clearance')).toBe('98px')
    })

    it.each([
      ['an overlay header above', true, '42px'],
      ['no overlay header', false, '0px'],
    ])('starts the terminal surface below the header row with %s', async (_, overlay, inset) => {
      landOnTerminal()
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      if (overlay) await renderBelowOverlayHeader(store, 'c1', 'r1')
      else await renderPane(store, openChatPane(store, 'c1', 'r1'))

      const surface = await screen.findByTestId('agent-terminal-surface')
      expect(surface.style.paddingTop).toBe(inset)
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

  describe('reviving signpost shape', () => {
    it('renders the reviving banner in the same pill shape', async () => {
      chatStore.setState({ feeds: {} })

      const store = seedWorkspace([startingChat({ id: 'c1' })])
      await renderPane(store, openChatPane(store, 'c1', ''))

      const banner = await screen.findByTestId('agent-reviving-banner')
      expect(banner.querySelector('.pill.halted')).not.toBeNull()
    })
  })

  describe('hidden keep-alive tab', () => {
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

      expect(resumeChatFn).not.toHaveBeenCalled()
      expect(screen.queryByText(/resuming this chat/i)).not.toBeInTheDocument()
      expect(screen.queryByTestId('xterm')).toBeNull()
    })

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
