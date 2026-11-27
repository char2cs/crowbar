import { act, fireEvent, screen } from '@testing-library/react'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { ApiError } from '@/lib/errors/api-error'
import { seedChats, writeChat, setProviders } from '@/__tests__/__fixtures__/agent-chat'
import {
  dormantChat,
  landOnTerminal,
  liveChat,
  liveChatNoTerminal,
  openChatPane,
  providers,
  renderPane,
  seedWorkspace,
  switchProviderFn,
  switchToTerminalFn,
  toastErrorFn,
} from '@/__tests__/__fixtures__/agent-chat-pane'

vi.mock(
  '@/utils/platform',
  async () => (await import('@/__tests__/__fixtures__/pane-platform')).platformModule,
)

describe('AgentChatPane provider switch', () => {
  describe('provider switch', () => {
    beforeEach(landOnTerminal)

    it('renders the switcher beneath the terminal at the chat provider', async () => {
      const store = seedWorkspace([
        liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1', provider: 'provider-b' }),
      ])
      await renderPane(store, openChatPane(store, 'c1', 'r1'))

      const footerControl = screen.getByTestId('provider-switch')
      expect(footerControl.getAttribute('data-current')).toBe('provider-b')
      expect(footerControl.getAttribute('data-count')).toBe('2')
    })

    it('is one flat surface — no card, and the switcher shares the terminal column', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      await renderPane(store, openChatPane(store, 'c1', 'r1'))

      expect(document.querySelector('[data-slot="frame"]')).toBeNull()
      expect(document.querySelector('[data-slot="frame-panel"]')).toBeNull()

      const term = screen.getByTestId('xterm')
      const pill = screen.getByTestId('provider-switch')
      const column = term.closest('.max-w-4xl')
      expect(column).toBe(pill.closest('.max-w-4xl'))
      expect(column?.className).toMatch(/px-\d/)
      expect(column?.className).toMatch(/max-w-/)

      expect(term.getAttribute('data-flush')).toBe('true')
    })

    it('switches the provider on the chat the runner is in NOW, not the one the tab opened on', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      const paneId = openChatPane(store, 'c1', 'r1')
      await renderPane(store, paneId)

      await act(async () => {
        seedChats([dormantChat({ id: 'c1' }), liveChat({ id: 'c2', runnerId: 'r1', pty: 'pty1' })])
      })

      await act(async () => {
        fireEvent.click(screen.getByTestId('provider-switch'))
      })

      expect(switchProviderFn).toHaveBeenCalledWith('w1', 'c2', 'provider-b', 'terminal')
    })

    it('surfaces a toast when the switch rejects (target CLI missing / spawn failed)', async () => {
      const err = vi.spyOn(console, 'error').mockImplementation(() => {})
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      switchProviderFn.mockRejectedValue(
        new ApiError('terminal: command not found: provider-b', 424),
      )
      await renderPane(store, openChatPane(store, 'c1', 'r1'))

      await act(async () => {
        fireEvent.click(screen.getByTestId('provider-switch'))
      })

      expect(toastErrorFn).toHaveBeenCalledTimes(1)
      const [title, description] = toastErrorFn.mock.calls[0] as [string, string]
      expect(title).toContain('Provider B')
      expect(title).toMatch(/isn.t installed/)
      expect(description).toMatch(/PATH/)
      expect(err).toHaveBeenCalled()
      err.mockRestore()
    })

    it('shows no toast when the switch succeeds', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      await renderPane(store, openChatPane(store, 'c1', 'r1'))

      await act(async () => {
        fireEvent.click(screen.getByTestId('provider-switch'))
      })

      expect(switchProviderFn).toHaveBeenCalledWith('w1', 'c1', 'provider-b', 'terminal')
      expect(toastErrorFn).not.toHaveBeenCalled()
    })

    it('re-requests an attach when switching TO a non-hotswap provider while already on the terminal surface', async () => {
      const store = seedWorkspace([
        liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1', provider: 'provider-a' }),
      ])
      setProviders([providers[0], { ...providers[1], hotswap: false }])
      switchProviderFn.mockImplementation(() => {
        writeChat(liveChatNoTerminal({ id: 'c1', runnerId: 'r2' }))
        return Promise.resolve('r2')
      })
      switchToTerminalFn.mockImplementation(async () => {
        await Promise.resolve()
        writeChat(
          liveChat({
            id: 'c1',
            runnerId: 'r2',
            pty: 'p',
            provider: 'provider-b',
            surface: 'terminal',
          }),
        )
      })
      await renderPane(store, openChatPane(store, 'c1', 'r1'))

      await act(async () => {
        fireEvent.click(screen.getByTestId('provider-switch'))
      })

      await vi.waitFor(() => expect(switchToTerminalFn).toHaveBeenCalledWith('w1', 'c1'))
      expect(await screen.findByTestId('xterm')).toHaveAttribute('data-session-id', 'p')
      expect(screen.queryByText(/no terminal view attached/i)).not.toBeInTheDocument()
    })
  })
})
