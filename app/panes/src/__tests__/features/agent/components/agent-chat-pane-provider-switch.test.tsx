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
  // ── Footer / provider switch ───────────────────────────────────────
  describe('provider switch', () => {
    // The switcher is the terminal surface's chrome. See landOnTerminal.
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

      // This pane was built on CossUI's Frame first, and seeing it live is what killed
      // the idea: a Frame LIFTS a panel off its background, and a chat pane must not be
      // lifted off anything. The bordered card framed the agent's empty middle rather
      // than hiding it, and the switcher — sitting outside the card — read as a stray
      // button on the desktop. No card is allowed back in.
      expect(document.querySelector('[data-slot="frame"]')).toBeNull()
      expect(document.querySelector('[data-slot="frame-panel"]')).toBeNull()

      // The switcher and the terminal MUST be inset by the same box. Every time the
      // padding lived on one of them instead of on their shared parent, the switcher
      // drifted out of line with the agent's first character and had to be re-tuned by
      // hand (16px, then 17px, then 1px...). Their common ancestor carries the inset, so
      // the alignment cannot rot.
      const term = screen.getByTestId('xterm')
      const pill = screen.getByTestId('provider-switch')
      const column = term.closest('.max-w-4xl')
      expect(column).toBe(pill.closest('.max-w-4xl'))
      expect(column?.className).toMatch(/px-\d/)
      expect(column?.className).toMatch(/max-w-/)

      // The terminal's own 16px inset would double the column's and shove the agent out
      // of line with the switcher again — the pane opts out of it.
      expect(term.getAttribute('data-flush')).toBe('true')
    })

    it('switches the provider on the chat the runner is in NOW, not the one the tab opened on', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      const paneId = openChatPane(store, 'c1', 'r1')
      await renderPane(store, paneId)

      // The runner /clears into c2 — the tab follows it.
      await act(async () => {
        seedChats([dormantChat({ id: 'c1' }), liveChat({ id: 'c2', runnerId: 'r1', pty: 'pty1' })])
      })

      await act(async () => {
        fireEvent.click(screen.getByTestId('provider-switch'))
      })

      // The switch must target c2. Targeting c1 would hand a CLI the conversation
      // the user has already left, and leave the live one running unattended.
      expect(switchProviderFn).toHaveBeenCalledWith('w1', 'c2', 'provider-b', 'terminal')
    })

    it('surfaces a toast when the switch rejects (target CLI missing / spawn failed)', async () => {
      const err = vi.spyOn(console, 'error').mockImplementation(() => {})
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      // The production failure verbatim: the daemon could not find the providerB binary and
      // answered 424 Failed Dependency.
      switchProviderFn.mockRejectedValue(
        new ApiError('terminal: command not found: provider-b', 424),
      )
      await renderPane(store, openChatPane(store, 'c1', 'r1'))

      await act(async () => {
        fireEvent.click(screen.getByTestId('provider-switch'))
      })

      expect(toastErrorFn).toHaveBeenCalledTimes(1)
      const [title, description] = toastErrorFn.mock.calls[0] as [string, string]
      expect(title).toContain('Provider B') // the target provider's display name
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

    // A switch keeps the surface, and only an attach request gives a non-hotswap
    // provider a terminal view to show on it.
    it('re-requests an attach when switching TO a non-hotswap provider while already on the terminal surface', async () => {
      const store = seedWorkspace([
        liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1', provider: 'provider-a' }),
      ])
      setProviders([providers[0], { ...providers[1], hotswap: false }])
      // Each intent's answer arrives as the daemon's snapshot frame.
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
