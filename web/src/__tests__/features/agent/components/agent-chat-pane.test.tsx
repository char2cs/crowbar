import { createElement } from 'react'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useStore } from 'zustand'
import type { AgentChat, AgentChatDetail, AgentProvider } from '@/features/agent/api/agent-api'
import type { AgentChatContent } from '@/features/panes/types/pane-content'
import {
  WorkspaceStoreContext,
  useWorkspaceStore,
} from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'

// Hoisted fakes — declared before the vi.mock calls that reference them.
const { getChatFn, switchProviderFn, resumeChatFn, saveReconnectFn, toastErrorFn } = vi.hoisted(
  () => ({
    getChatFn: vi.fn(),
    switchProviderFn: vi.fn(),
    resumeChatFn: vi.fn(),
    saveReconnectFn: vi.fn(),
    toastErrorFn: vi.fn(),
  }),
)

vi.mock('@/features/agent/api/agent-api', () => ({
  getChat: (...a: unknown[]) => getChatFn(...a),
  switchProvider: (...a: unknown[]) => switchProviderFn(...a),
  resumeChat: (...a: unknown[]) => resumeChatFn(...a),
}))

vi.mock('@/features/terminal/lib/terminal-reconnect-map', () => ({
  saveReconnect: (...a: unknown[]) => saveReconnectFn(...a),
}))

vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: (...a: unknown[]) => toastErrorFn(...a) },
}))

// jsdom can't run xterm/WebGL — stub the terminal renderer to a passive marker
// that records the sessionId it was mounted with (that's what the attach seam is
// proven by) plus the isActive/isVisible/attachOnly props threaded from the pane.
// The marker is clickable so a test can fire the terminal's onSessionGone — the
// pane's half of the "the PTY died under a mounted pane" contract.
vi.mock('@/features/terminal/components/terminal', () => ({
  XtermTerminal: ({
    sessionId,
    isActive,
    isVisible,
    attachOnly,
    flush,
    onSessionGone,
  }: {
    sessionId: string
    isActive: boolean
    isVisible?: boolean
    attachOnly?: boolean
    flush?: boolean
    onSessionGone?: () => void
  }) =>
    createElement('div', {
      'data-testid': 'xterm',
      'data-session-id': sessionId,
      'data-active': String(isActive),
      'data-visible': String(isVisible),
      'data-attach-only': String(Boolean(attachOnly)),
      'data-flush': String(Boolean(flush)),
      onClick: () => onSessionGone?.(),
    }),
}))

// Stub the dropdown to expose its props and a one-click switch, so the footer
// wiring is asserted without the shared Dropdown's framer-motion machinery.
vi.mock('@/features/agent/components/provider-switch-dropdown', () => ({
  ProviderSwitchDropdown: ({
    providers,
    currentProviderId,
    onSwitch,
  }: {
    providers: AgentProvider[]
    currentProviderId: string
    onSwitch: (id: string) => void
  }) =>
    createElement(
      'button',
      {
        'data-testid': 'provider-switch',
        'data-current': currentProviderId,
        'data-count': String(providers.length),
        onClick: () => onSwitch('codex'),
      },
      'switch',
    ),
}))

import { AgentChatPane } from '@/features/agent/components/agent-chat-pane'
import { useTerminalStore } from '@/features/terminal/stores/terminal-store'

const providers: AgentProvider[] = [
  { id: 'claude', displayName: 'Claude', icon: '<svg/>' },
  { id: 'codex', displayName: 'Codex', icon: '<svg/>' },
]

// ── Wire fixtures ────────────────────────────────────────────────────
// A chat is LIVE exactly while a runner is placed on it. liveRunnerId is the whole
// liveness contract — no status flag exists that could disagree with it — and it
// carries that runner's PTY, which is what the pane attaches to.

function liveChat(o: {
  id: string
  runnerId: string
  pty: string
  title?: string
  provider?: string
}): AgentChat {
  return {
    id: o.id,
    workspaceId: 'w1',
    title: o.title ?? `Chat ${o.id}`,
    liveRunnerId: o.runnerId,
    terminalSessionId: o.pty,
    activeProviderId: o.provider ?? 'codex',
    createdAt: '',
  }
}

/** A dormant chat: no runner points at it, so there is nothing to attach. It keeps
 *  the provider of its last conversation — who Resume brings back. */
function dormantChat(o: { id: string; title?: string; provider?: string }): AgentChat {
  return {
    id: o.id,
    workspaceId: 'w1',
    title: o.title ?? `Chat ${o.id}`,
    liveRunnerId: '',
    terminalSessionId: '',
    activeProviderId: o.provider ?? 'codex',
    createdAt: '',
  }
}

function detail(chat: AgentChat): AgentChatDetail {
  return { ...chat, conversations: [] }
}

// ── Harness ──────────────────────────────────────────────────────────
// The REAL workspace store, so the pane's repoint writes land on a real buffer.
// PaneHost is exactly what pane-container does: read the buffer, feed its
// chatId/runnerId back in as props. That closes the loop the feature IS — the
// buffer is the pane's moving target, and the pane is what moves it.

function seedWorkspace(chats: AgentChat[]) {
  const store = createWorkspaceStore('w1')
  store.getState().setAgentProviders(providers)
  store.getState().seedAgentChats(chats)
  return store
}

type Store = ReturnType<typeof seedWorkspace>

function openBuffer(store: Store, chatId: string, runnerId: string, name = 'Chat') {
  return store.getState().bufferActions.openContent({
    type: 'agentChat',
    chatId,
    wsId: 'w1',
    name,
    runnerId,
  })
}

function PaneHost({ bufferId }: { bufferId: string }) {
  const store = useWorkspaceStore()
  const buf = useStore(store, (s) => s.buffers.find((b) => b.id === bufferId)) as
    | AgentChatContent
    | undefined
  if (!buf) return null
  return createElement(AgentChatPane, {
    chatId: buf.chatId,
    runnerId: buf.runnerId,
    wsId: buf.wsId,
    bufferId: buf.id,
    isActivePane: true,
  })
}

async function renderPane(store: Store, bufferId: string) {
  await act(async () => {
    render(
      createElement(
        WorkspaceStoreContext.Provider,
        { value: store },
        createElement(PaneHost, { bufferId }),
      ),
    )
  })
}

const buffer = (store: Store, id: string) =>
  store.getState().buffers.find((b) => b.id === id) as AgentChatContent | undefined

beforeEach(() => {
  getChatFn.mockReset()
  switchProviderFn.mockReset()
  resumeChatFn.mockReset()
  saveReconnectFn.mockReset()
  toastErrorFn.mockReset()
  switchProviderFn.mockResolvedValue('r-new')
  useTerminalStore.setState({ sessions: new Map() })
  localStorage.clear()
})

describe('AgentChatPane', () => {
  // ── THE HEADLINE ───────────────────────────────────────────────────
  // The user's bug: they type /clear inside the CLI, the CLI switches conversation,
  // and Crowbar moves the running process to a DIFFERENT chat. The pane used to be
  // pinned to a chatId for life, so it went "This agent has exited" — with a Resume
  // button that would spawn a SECOND CLI — while the first was alive and well in a
  // chat the user had to go find. The tab is a VIEWPORT on a moving target: it
  // follows the runner, and because the terminal is keyed by the PTY (which a move
  // does not change), the conversation changes WITHOUT changing the terminal.
  it('follows its runner to a new chat without remounting the terminal', async () => {
    const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
    const bufferId = openBuffer(store, 'c1', 'r1')
    await renderPane(store, bufferId)

    const term = await screen.findByTestId('xterm')
    expect(term).toHaveAttribute('data-session-id', 'pty1')

    // The runner /clears into a brand-new chat — carrying the SAME pty.
    await act(async () => {
      store
        .getState()
        .seedAgentChats([
          dormantChat({ id: 'c1' }),
          liveChat({ id: 'c2', runnerId: 'r1', pty: 'pty1', title: 'Fresh' }),
        ])
    })

    // The tab re-points at the chat the runner is in NOW...
    expect(buffer(store, bufferId)).toMatchObject({ chatId: 'c2', runnerId: 'r1' })
    // ...and relabels to that chat's title...
    expect(buffer(store, bufferId)?.name).toBe('Fresh')
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
    const bufferId = openBuffer(store, 'c1', 'r1')
    await renderPane(store, bufferId)

    await act(async () => {
      store
        .getState()
        .seedAgentChats([
          dormantChat({ id: 'c1' }),
          liveChat({ id: 'c2', runnerId: 'r1', pty: 'pty1' }),
        ])
    })

    expect(screen.queryByText(/this agent has exited/i)).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /resume/i })).not.toBeInTheDocument()
    expect(resumeChatFn).not.toHaveBeenCalled()
    expect(screen.getByTestId('xterm')).toBeTruthy()
  })

  it('shows Resume only when no runner holds the chat', async () => {
    const store = seedWorkspace([dormantChat({ id: 'c1' })])
    const bufferId = openBuffer(store, 'c1', '')
    await renderPane(store, bufferId)

    expect(await screen.findByRole('button', { name: /resume/i })).toBeTruthy()
    expect(screen.getByText(/this agent has exited/i)).toBeTruthy()
    expect(screen.queryByTestId('xterm')).toBeNull()
    // Nothing implicit: opening a dormant chat does NOT spawn a CLI behind the
    // user's back. The auto-revive machinery is gone.
    expect(resumeChatFn).not.toHaveBeenCalled()
  })

  // ── Attaching ──────────────────────────────────────────────────────
  it('attaches the live runner PTY: seeds the mapping, then mounts the terminal', async () => {
    const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
    const bufferId = openBuffer(store, 'c1', 'r1')
    await renderPane(store, bufferId)

    const xterm = await screen.findByTestId('xterm')
    expect(xterm.getAttribute('data-session-id')).toBe('pty1')
    expect(xterm.getAttribute('data-active')).toBe('true')
    expect(xterm.getAttribute('data-visible')).toBe('true')
    // Attach-only: a reconnect can never spawn a bare shell into the agent frame.
    expect(xterm.getAttribute('data-attach-only')).toBe('true')
    // The mapping that makes resolveTerminalConnection ATTACH exists at mount.
    expect(useTerminalStore.getState().getSession('pty1')?.connectionId).toBe('pty1')
    expect(saveReconnectFn).toHaveBeenCalledWith('w1', 'pty1', 'pty1')
    // liveRunnerId IS the liveness answer — no second round trip asks the daemon.
    expect(getChatFn).not.toHaveBeenCalled()
  })

  it('renders nothing (not the exited state) while the chat list is still loading', async () => {
    // The seed is in flight: the store does not know this chat yet. "Not known" is
    // not "dormant" — flashing Resume here would offer a button that spawns a
    // second CLI onto a chat that may well be live.
    const store = seedWorkspace([])
    const bufferId = openBuffer(store, 'c1', 'r1')
    await renderPane(store, bufferId)

    expect(screen.queryByTestId('xterm')).toBeNull()
    expect(screen.queryByRole('button', { name: /resume/i })).not.toBeInTheDocument()
    expect(screen.queryByText(/this agent has exited/i)).not.toBeInTheDocument()
  })

  it('threads isActivePane=false through to the terminal', async () => {
    const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
    const bufferId = openBuffer(store, 'c1', 'r1')
    await act(async () => {
      render(
        createElement(
          WorkspaceStoreContext.Provider,
          { value: store },
          createElement(AgentChatPane, {
            chatId: 'c1',
            runnerId: 'r1',
            wsId: 'w1',
            bufferId,
            isActivePane: false,
          }),
        ),
      )
    })

    expect((await screen.findByTestId('xterm')).getAttribute('data-active')).toBe('false')
  })

  // ── Adopting a new runner on the same chat ─────────────────────────
  // A provider switch replaces the runner IN PLACE: same chat, new CLI, new PTY.
  // The pane's runner is gone from everywhere, so it adopts whoever is on its chat
  // now — and this one DOES remount the terminal, because the PTY genuinely changed.
  it('adopts the chat new runner after a provider switch (new PTY, so the terminal remounts)', async () => {
    const store = seedWorkspace([
      liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1', provider: 'codex' }),
    ])
    const bufferId = openBuffer(store, 'c1', 'r1')
    await renderPane(store, bufferId)

    const before = await screen.findByTestId('xterm')
    expect(before).toHaveAttribute('data-session-id', 'pty1')

    await act(async () => {
      store
        .getState()
        .seedAgentChats([liveChat({ id: 'c1', runnerId: 'r2', pty: 'pty2', provider: 'claude' })])
    })

    const after = await screen.findByTestId('xterm')
    expect(after).toHaveAttribute('data-session-id', 'pty2')
    expect(after).not.toBe(before) // a different PTY IS a different terminal
    expect(buffer(store, bufferId)).toMatchObject({ chatId: 'c1', runnerId: 'r2' })
    expect(useTerminalStore.getState().getSession('pty2')?.connectionId).toBe('pty2')
    expect(screen.getByTestId('provider-switch').getAttribute('data-current')).toBe('claude')
  })

  it('lets go of a dead runner id when its chat goes dormant', async () => {
    const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
    const bufferId = openBuffer(store, 'c1', 'r1')
    await renderPane(store, bufferId)

    await act(async () => {
      store.getState().seedAgentChats([dormantChat({ id: 'c1' })])
    })

    // The buffer must not go on pointing at a runner that no longer exists.
    expect(buffer(store, bufferId)).toMatchObject({ chatId: 'c1', runnerId: '' })
    expect(screen.getByRole('button', { name: /resume/i })).toBeTruthy()
  })

  // ── Resume (explicit, never implicit) ──────────────────────────────
  describe('Resume', () => {
    it('spawns a runner and attaches its PTY', async () => {
      const store = seedWorkspace([dormantChat({ id: 'c1' })])
      const bufferId = openBuffer(store, 'c1', '')
      await renderPane(store, bufferId)

      resumeChatFn.mockResolvedValue('r9')
      getChatFn.mockResolvedValue(detail(liveChat({ id: 'c1', runnerId: 'r9', pty: 'pty9' })))

      await act(async () => {
        fireEvent.click(screen.getByRole('button', { name: /resume/i }))
      })

      expect(resumeChatFn).toHaveBeenCalledWith('w1', 'c1')
      const xterm = await screen.findByTestId('xterm')
      expect(xterm).toHaveAttribute('data-session-id', 'pty9')
      expect(buffer(store, bufferId)).toMatchObject({ chatId: 'c1', runnerId: 'r9' })
      expect(useTerminalStore.getState().getSession('pty9')?.connectionId).toBe('pty9')
    })

    it('surfaces a toast and stays resumable when the agent cannot be restarted', async () => {
      const err = vi.spyOn(console, 'error').mockImplementation(() => {})
      const store = seedWorkspace([dormantChat({ id: 'c1' })])
      const bufferId = openBuffer(store, 'c1', '')
      await renderPane(store, bufferId)

      resumeChatFn.mockRejectedValue(new Error('claude: not on PATH'))
      await act(async () => {
        fireEvent.click(screen.getByRole('button', { name: /resume/i }))
      })

      expect(toastErrorFn).toHaveBeenCalledWith('Could not resume this chat', expect.any(String))
      expect(screen.getByText(/could not restart this agent/i)).toBeTruthy()
      expect(screen.getByRole('button', { name: /resume/i })).toBeTruthy()
      // Exactly once: a CLI that cannot start must settle, never spawn-loop.
      expect(resumeChatFn).toHaveBeenCalledTimes(1)
      err.mockRestore()
    })
  })

  // ── The PTY dying under an open pane ───────────────────────────────
  // The terminal runs attach-only and reports the session gone rather than spawning
  // a shell. The pane shows the exited state — and does NOT respawn: the user may
  // have just typed /exit, and bringing the agent straight back is the opposite of
  // what they asked for.
  it('offers Resume (and does not respawn) when the PTY dies under the open pane', async () => {
    const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
    const bufferId = openBuffer(store, 'c1', 'r1')
    await renderPane(store, bufferId)
    expect(await screen.findByTestId('xterm')).toBeTruthy()

    await act(async () => {
      fireEvent.click(screen.getByTestId('xterm')) // onSessionGone
    })

    expect(screen.queryByTestId('xterm')).toBeNull()
    expect(screen.getByText(/this agent has exited/i)).toBeTruthy()
    expect(resumeChatFn).not.toHaveBeenCalled()
    expect(screen.getByTestId('provider-switch')).toBeTruthy()
  })

  // ── Footer / provider switch ───────────────────────────────────────
  describe('provider switch', () => {
    it('renders the switcher beneath the terminal at the chat provider', async () => {
      const store = seedWorkspace([
        liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1', provider: 'codex' }),
      ])
      await renderPane(store, openBuffer(store, 'c1', 'r1'))

      const footerControl = screen.getByTestId('provider-switch')
      expect(footerControl.getAttribute('data-current')).toBe('codex')
      expect(footerControl.getAttribute('data-count')).toBe('2')
    })

    it('is one flat opaque surface — no card, and the switcher shares the terminal column', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      await renderPane(store, openBuffer(store, 'c1', 'r1'))

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
      const column = term.parentElement?.parentElement
      expect(column).toBe(pill.parentElement?.parentElement)
      expect(column?.className).toMatch(/px-\d/)
      expect(column?.className).toMatch(/max-w-/)

      // The terminal's own 16px inset would double the column's and shove the agent out
      // of line with the switcher again — the pane opts out of it.
      expect(term.getAttribute('data-flush')).toBe('true')
    })

    it('switches the provider on the chat the runner is in NOW, not the one the tab opened on', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      const bufferId = openBuffer(store, 'c1', 'r1')
      await renderPane(store, bufferId)

      // The runner /clears into c2 — the tab follows it.
      await act(async () => {
        store
          .getState()
          .seedAgentChats([
            dormantChat({ id: 'c1' }),
            liveChat({ id: 'c2', runnerId: 'r1', pty: 'pty1' }),
          ])
      })

      fireEvent.click(screen.getByTestId('provider-switch'))

      // The switch must target c2. Targeting c1 would hand a CLI the conversation
      // the user has already left, and leave the live one running unattended.
      expect(switchProviderFn).toHaveBeenCalledWith('w1', 'c2', 'codex')
    })

    it('surfaces a toast when the switch rejects (target CLI missing / spawn failed)', async () => {
      const err = vi.spyOn(console, 'error').mockImplementation(() => {})
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      switchProviderFn.mockRejectedValue(new Error('500: codex not installed'))
      await renderPane(store, openBuffer(store, 'c1', 'r1'))

      await act(async () => {
        fireEvent.click(screen.getByTestId('provider-switch'))
      })

      expect(toastErrorFn).toHaveBeenCalledTimes(1)
      const [title, description] = toastErrorFn.mock.calls[0] as [string, string]
      expect(title).toMatch(/could not switch provider/i)
      expect(description).toContain('Codex') // the target provider's display name
      expect(err).toHaveBeenCalled()
      err.mockRestore()
    })

    it('shows no toast when the switch succeeds', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      await renderPane(store, openBuffer(store, 'c1', 'r1'))

      await act(async () => {
        fireEvent.click(screen.getByTestId('provider-switch'))
      })

      expect(switchProviderFn).toHaveBeenCalledWith('w1', 'c1', 'codex')
      expect(toastErrorFn).not.toHaveBeenCalled()
    })
  })

  // ── Tab title ──────────────────────────────────────────────────────
  // openContent snapshots the label at open time; the chat's title changes later
  // (the agent auto-titles it over WS `title_set`, or the user renames it) and both
  // land on the store chat's `title`. The pane mirrors title → buffer name.
  describe('tab title tracks the chat title', () => {
    it('relabels the tab when the chat is titled', async () => {
      const store = seedWorkspace([
        liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1', title: 'Codex chat' }),
      ])
      const bufferId = openBuffer(store, 'c1', 'r1', 'Codex chat')
      await renderPane(store, bufferId)

      await act(async () => {
        store
          .getState()
          .seedAgentChats([
            liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1', title: 'Fix the flaky test' }),
          ])
      })

      expect(buffer(store, bufferId)?.name).toBe('Fix the flaky test')
    })

    it('never blanks the tab when the chat title is empty', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1', title: '' })])
      const bufferId = openBuffer(store, 'c1', 'r1', 'Codex chat')
      await renderPane(store, bufferId)

      expect(buffer(store, bufferId)?.name).toBe('Codex chat')
    })
  })
})
