import { createElement } from 'react'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/lib/errors/api-error'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { chatStore } from '@/features/agent/stores/chat-store'
import { ledger, seedFeed } from '@/__tests__/__fixtures__/ledger-rows'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { setChatWorking, writeChat, setProviders } from '@/__tests__/__fixtures__/agent-chat'
import { deferred } from '@/__tests__/__fixtures__/deferred'
import {
  PaneHost,
  dormantChat,
  liveChat,
  liveChatNoTerminal,
  openChatPane,
  providers,
  renderPane,
  seedWorkspace,
  submitPromptFn,
  switchProviderFn,
  switchToNativeFn,
  switchToTerminalFn,
  toastErrorFn,
  typeAndSend,
} from '@/__tests__/__fixtures__/agent-chat-pane'

vi.mock(
  '@/utils/platform',
  async () => (await import('@/__tests__/__fixtures__/pane-platform')).platformModule,
)

describe('AgentChatPane view toggle', () => {
  // ── ⌘/ toggles the chat/terminal view, like the ViewSwitcher tabs ─────
  describe('toggle chat/terminal view chord', () => {
    const pressToggle = async () => {
      await act(async () => {
        window.dispatchEvent(new KeyboardEvent('keydown', { key: '/', metaKey: true }))
      })
    }

    it('flips a hotswap provider straight from Chat to Terminal', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      const paneId = openChatPane(store, 'c1', 'r1')
      await renderPane(store, paneId)

      expect(screen.getByRole('tab', { name: /^chat$/i })).toHaveAttribute('aria-selected', 'true')

      await pressToggle()

      expect(screen.getByRole('tab', { name: /^terminal$/i })).toHaveAttribute(
        'aria-selected',
        'true',
      )
    })

    it('flips back from Terminal to Chat on a second press', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      const paneId = openChatPane(store, 'c1', 'r1')
      await renderPane(store, paneId)

      await pressToggle()
      await pressToggle()

      expect(screen.getByRole('tab', { name: /^chat$/i })).toHaveAttribute('aria-selected', 'true')
    })

    it('still fires when the focused child swallows the key (xterm stopPropagation)', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      const paneId = openChatPane(store, 'c1', 'r1')
      await renderPane(store, paneId)

      // With a chat open the focus sits in its xterm, which stopPropagations the
      // keys it handles. A bubble-phase listener never sees the chord in the one
      // place the command exists to work — this stands that swallower in, so the
      // test fails if the listener ever goes back to the bubble phase.
      const swallow = (e: Event) => e.stopPropagation()
      document.body.addEventListener('keydown', swallow)
      try {
        await act(async () => {
          document.body.dispatchEvent(
            new KeyboardEvent('keydown', {
              key: '/',
              metaKey: true,
              bubbles: true,
              cancelable: true,
            }),
          )
        })
      } finally {
        document.body.removeEventListener('keydown', swallow)
      }

      expect(screen.getByRole('tab', { name: /^terminal$/i })).toHaveAttribute(
        'aria-selected',
        'true',
      )
    })

    // A retained workspace stays mounted with its window keydown listener, so only the active one
    // may act on the chord. Provider B (non-hotswap) makes the switch a call naming the chat that asked.
    it('a HIDDEN WORKSPACE ignores the chord — only the ACTIVE workspace may switch', async () => {
      switchToTerminalFn.mockResolvedValue('pty-attached')
      const hidden = seedWorkspace(
        [liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1', provider: 'provider-b' })],
        'w-hidden',
      )
      setProviders([providers[0], { ...providers[1], hotswap: false }])
      const hiddenPaneId = openChatPane(hidden, 'c1', 'r1', 'Chat', 'w-hidden')
      const shown = seedWorkspace(
        [liveChat({ id: 'c2', runnerId: 'r2', pty: 'pty2', provider: 'provider-b' })],
        'w-shown',
      )
      setProviders([providers[0], { ...providers[1], hotswap: false }])
      const shownPaneId = openChatPane(shown, 'c2', 'r2', 'Chat', 'w-shown')

      // Both panes are the active, visible tab of their own workspace — exactly
      // what a retained workspace looks like the instant it goes hidden.
      await renderPane(hidden, hiddenPaneId)
      await renderPane(shown, shownPaneId)
      setActiveWorkspaceId('w-shown')

      await pressToggle()

      await vi.waitFor(() => expect(switchToTerminalFn).toHaveBeenCalledTimes(1))
      expect(switchToTerminalFn).toHaveBeenCalledWith('w-shown', 'c2')
    })

    // THE BUG, as the user reported it: "Can't start a chat with providerB TUI
    // alone. It's only allowing me to start a thread on the native chat
    // interface, and then go TUI."
    //
    // A chat BORN on the terminal surface has no api connection behind it —
    // the daemon never opened one (spawnRunner's surfaceForSpawn) — so its own
    // PTY IS the conversation, with nothing to ask the daemon for.
    it('a terminal-BORN chat shows its own PTY without asking for an attach', async () => {
      const store = seedWorkspace([
        liveChat({
          id: 'c1',
          runnerId: 'r1',
          pty: 'pty1',
          provider: 'provider-b',
          surface: 'terminal',
        }),
      ])
      setProviders([providers[0], { ...providers[1], hotswap: false }])
      const paneId = openChatPane(store, 'c1', 'r1')
      await renderPane(store, paneId)

      await pressToggle()

      expect(switchToTerminalFn).not.toHaveBeenCalled()
      expect(screen.getByRole('tab', { name: /^terminal$/i })).toHaveAttribute(
        'aria-selected',
        'true',
      )
      expect(screen.getByTestId('xterm')).toHaveAttribute('data-session-id', 'pty1')
      expect(toastErrorFn).not.toHaveBeenCalled()
    })

    it('a HIDDEN chat ignores the chord — a background split must not switch', async () => {
      switchToTerminalFn.mockResolvedValue('pty-attached')
      const store = seedWorkspace([
        liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1', provider: 'provider-b' }),
      ])
      setProviders([providers[0], { ...providers[1], hotswap: false }])
      const paneId = openChatPane(store, 'c1', 'r1')
      // Every chat stays mounted for keep-alive, so without the isVisible gate a
      // hidden tab would swallow the chord and flip the view on a chat nobody can see.
      await act(async () => {
        render(
          createElement(
            WorkspaceStoreContext.Provider,
            { value: store },
            createElement(PaneHost, { paneId, isVisible: false }),
          ),
        )
      })

      await pressToggle()

      expect(switchToTerminalFn).not.toHaveBeenCalled()
    })
  })

  describe('React chat presentation', () => {
    it('defaults to Chat while retaining the native terminal as an attach-only fallback', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      await renderPane(store, openChatPane(store, 'c1', 'r1'))

      expect(screen.getByRole('tab', { name: /^chat$/i })).toHaveAttribute('aria-selected', 'true')
      expect(screen.getByRole('textbox', { name: /message the agent/i })).toBeInTheDocument()
      expect(screen.getByTestId('xterm')).toHaveAttribute('data-attach-only', 'true')

      fireEvent.click(screen.getByRole('tab', { name: /^terminal$/i }))
      expect(screen.getByRole('tab', { name: /^terminal$/i })).toHaveAttribute(
        'aria-selected',
        'true',
      )
      expect(screen.getByTestId('xterm')).toHaveAttribute('data-visible', 'true')
    })

    it('holds prompts queued in Chat while the Terminal shows: counts, cancels, sends on return', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      const turn = ledger('c1', 4)
      seedFeed('c1', [turn.start('turn-2')])
      await renderPane(store, openChatPane(store, 'c1', 'r1'))
      const queueThenShowTerminal = (text: string) => {
        typeAndSend(text)
        fireEvent.click(screen.getByRole('tab', { name: /^terminal$/i }))
        expect(screen.getByText(/1 prompt pending in Chat/i)).toBeInTheDocument()
      }
      queueThenShowTerminal('cancelled from there')
      fireEvent.click(screen.getByRole('button', { name: /cancel unsent prompts/i }))
      expect(screen.queryByText(/pending in Chat/i)).not.toBeInTheDocument()
      fireEvent.click(screen.getByRole('tab', { name: /^chat$/i }))
      queueThenShowTerminal('queued while busy')
      await act(async () => seedFeed('c1', [turn.end('turn-2')]))
      expect(submitPromptFn).not.toHaveBeenCalled()
      fireEvent.click(screen.getByRole('button', { name: /return to chat/i }))
      await vi.waitFor(() => expect(submitPromptFn).toHaveBeenCalledTimes(1))
      expect(submitPromptFn.mock.calls[0]?.[2]).toBe('queued while busy')
    })

    it('takes a non-hotswap chat back to its chat channel before sending what waited', async () => {
      const store = seedWorkspace([liveChatNoTerminal({ id: 'c1', runnerId: 'r1' })])
      setProviders([providers[0], { ...providers[1], hotswap: false }])
      switchToTerminalFn.mockImplementation(async () =>
        writeChat({ id: 'c1', surface: 'terminal' }),
      )
      switchToNativeFn.mockImplementation(async () => writeChat({ id: 'c1', surface: undefined }))
      await renderPane(store, openChatPane(store, 'c1', 'r1'))
      await act(async () => fireEvent.click(screen.getByRole('tab', { name: /^terminal$/i })))
      typeAndSend('held for the chat channel')
      fireEvent.click(screen.getByRole('button', { name: /return to chat/i }))
      await vi.waitFor(() => expect(submitPromptFn).toHaveBeenCalledTimes(1))
      expect(switchToNativeFn).toHaveBeenCalledWith('w1', 'c1')
    })

    // The dropdown lives on the terminal's status strip, and it must keep
    // refusing a switch across both the submission window and the hook
    // confirmation that follows it — the window a handover would lose the turn in.
    it('blocks dropdown provider switches through submission and hook confirmation', async () => {
      const submitted = deferred<{ runnerId: string; terminalSessionId: string }>()
      submitPromptFn.mockReturnValue(submitted.promise)
      const store = seedWorkspace([
        liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1', provider: 'provider-a' }),
      ])
      await renderPane(store, openChatPane(store, 'c1', 'r1'))

      typeAndSend('deliver exactly once')
      await vi.waitFor(() => expect(submitPromptFn).toHaveBeenCalledTimes(1))

      // Chat has no dropdown to press yet — cross to the provider's own view,
      // where the dropdown is, and find it refusing.
      expect(screen.queryByTestId('provider-switch')).toBeNull()
      await act(async () => {
        fireEvent.click(screen.getByLabelText('Terminal'))
      })
      expect(screen.getByTestId('provider-switch')).toBeDisabled()
      fireEvent.click(screen.getByTestId('provider-switch'))
      expect(switchProviderFn).not.toHaveBeenCalled()

      await act(async () => {
        submitted.resolve({ runnerId: 'r-prompt', terminalSessionId: 'pty-prompt' })
      })
      // Still refusing after the submission resolves: the prompt is delivered but
      // the hook has not confirmed it, and that window is exactly the one a
      // handover would lose the turn in.
      expect(screen.getByTestId('provider-switch')).toBeDisabled()
      fireEvent.click(screen.getByTestId('provider-switch'))
      expect(switchProviderFn).not.toHaveBeenCalled()
    })

    // The daemon refuses a switch while a turn runs; the pane offers none.
    it('offers no provider switch while the chat is working', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      setChatWorking('c1', true)
      await renderPane(store, openChatPane(store, 'c1', 'r1'))
      await act(async () => {
        fireEvent.click(screen.getByLabelText('Terminal'))
      })

      expect(screen.getByTestId('provider-switch')).toBeDisabled()
      await act(async () => setChatWorking('c1', false))
      expect(screen.getByTestId('provider-switch')).toBeEnabled()
    })

    it('reconciles a failed send to dormant instead of retaining a dead PTY attachment', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      submitPromptFn.mockRejectedValue(new ApiError('replacement failed', 500))
      await renderPane(store, openChatPane(store, 'c1', 'r1'))

      typeAndSend('trigger replacement')
      // The daemon's lifecycle frame says the runner is gone.
      await act(async () => writeChat(dormantChat({ id: 'c1' })))

      await vi.waitFor(() => expect(chatStore.getState().outbox.c1?.[0]?.state).toBe('failed'))
      expect(chatStore.getState().outbox.c1?.[0]?.error).toBe('replacement failed')
      expect(screen.queryByTestId('agent-session-note')).not.toBeInTheDocument()
      expect(screen.queryByTestId('xterm')).not.toBeInTheDocument()
    })
  })
})
