import { createElement } from 'react'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createStore } from 'zustand'
import type { AgentChat, AgentChatDetail, AgentProvider } from '@/features/agent/api/agent-api'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import type { WorkspaceStore } from '@/features/workspace/stores/workspace-store'

// Hoisted fakes — declared before the vi.mock calls that reference them.
const {
  getChatFn,
  switchProviderFn,
  resumeChatFn,
  saveReconnectFn,
  terminalListLiveFn,
  toastErrorFn,
} = vi.hoisted(() => ({
  getChatFn: vi.fn(),
  switchProviderFn: vi.fn(),
  resumeChatFn: vi.fn(),
  saveReconnectFn: vi.fn(),
  terminalListLiveFn: vi.fn(),
  toastErrorFn: vi.fn(),
}))

vi.mock('@/features/agent/api/agent-api', () => ({
  getChat: (...a: unknown[]) => getChatFn(...a),
  switchProvider: (...a: unknown[]) => switchProviderFn(...a),
  resumeChat: (...a: unknown[]) => resumeChatFn(...a),
}))

vi.mock('@/features/terminal/lib/terminal-reconnect-map', () => ({
  saveReconnect: (...a: unknown[]) => saveReconnectFn(...a),
}))

// The daemon's live-session listing: the pane checks it before seeding, so a dead
// PTY can never be handed to resolveTerminalConnection (which would spawn a shell).
vi.mock('@/lib/crowbar-bridge', () => ({
  terminalListLive: (...a: unknown[]) => terminalListLiveFn(...a),
}))

vi.mock('@/lib/workspace-scope-url', () => ({
  workspaceBase: (wsId: string) => `/v0/projects/p1/repos/r1/workspaces/${wsId}`,
}))

vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: (...a: unknown[]) => toastErrorFn(...a) },
}))

// jsdom can't run xterm/WebGL — stub the terminal renderer to a passive marker
// that records the sessionId it was mounted with (that's what the attach seam
// is proven by) plus the isActive/isVisible/attachOnly props threaded from the
// pane. The marker is clickable so a test can fire the terminal's onSessionGone
// — the pane's half of the "the PTY died under a mounted pane" contract; the
// terminal's own half (never spawning on that path) lives in
// __tests__/features/terminal/components/terminal-attach-only.test.tsx.
vi.mock('@/features/terminal/components/terminal', () => ({
  XtermTerminal: ({
    sessionId,
    isActive,
    isVisible,
    attachOnly,
    onSessionGone,
  }: {
    sessionId: string
    isActive: boolean
    isVisible?: boolean
    attachOnly?: boolean
    onSessionGone?: () => void
  }) =>
    createElement('div', {
      'data-testid': 'xterm',
      'data-session-id': sessionId,
      'data-active': String(isActive),
      'data-visible': String(isVisible),
      'data-attach-only': String(Boolean(attachOnly)),
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

import { AgentChatPane, attachAgentSegment } from '@/features/agent/components/agent-chat-pane'
import { useTerminalStore } from '@/features/terminal/stores/terminal-store'

const providers: AgentProvider[] = [
  { id: 'claude', displayName: 'Claude', icon: '<svg/>' },
  { id: 'codex', displayName: 'Codex', icon: '<svg/>' },
]

function detail(overrides: Partial<AgentChatDetail> = {}): AgentChatDetail {
  return {
    id: 'c1',
    workspaceId: 'w1',
    title: 'Chat',
    activeSegmentId: 's2',
    activeProviderId: 'codex',
    createdAt: '',
    segments: [
      {
        id: 's1',
        terminalSessionId: 'term-1',
        status: 'ended',
        providerId: 'claude',
        providerSessionId: '',
        crowbarSegmentId: 's1',
        startedAt: '',
      },
      {
        id: 's2',
        terminalSessionId: 'term-2',
        status: 'active',
        providerId: 'codex',
        providerSessionId: '',
        crowbarSegmentId: 's2',
        startedAt: '',
      },
    ],
    ...overrides,
  }
}

beforeEach(() => {
  getChatFn.mockReset()
  switchProviderFn.mockReset()
  resumeChatFn.mockReset()
  saveReconnectFn.mockReset()
  terminalListLiveFn.mockReset()
  toastErrorFn.mockReset()
  // Default: the daemon still has both segments' PTYs. Tests that model a dead PTY
  // override this.
  terminalListLiveFn.mockResolvedValue(['term-1', 'term-2'])
  switchProviderFn.mockResolvedValue('seg-new')
  useTerminalStore.setState({ sessions: new Map() })
})

describe('attachAgentSegment', () => {
  it('pre-seeds the terminal-store mapping to the active segment PTY so resolve ATTACHES', async () => {
    getChatFn.mockResolvedValue(detail())

    const sid = await attachAgentSegment('w1', 'c1')

    expect(sid).toBe('term-2')
    // In-memory store now maps the tab's sessionId → its own id as connectionId:
    // the highest-priority reuse branch of resolveTerminalConnection.
    expect(useTerminalStore.getState().getSession('term-2')?.connectionId).toBe('term-2')
    // localStorage backstop mirrors the same mapping for a webview reload.
    expect(saveReconnectFn).toHaveBeenCalledWith('w1', 'term-2', 'term-2')
    expect(getChatFn).toHaveBeenCalledWith('w1', 'c1')
  })

  it('returns null when the active segment does not exist', async () => {
    getChatFn.mockResolvedValue(detail({ activeSegmentId: '', segments: [] }))
    expect(await attachAgentSegment('w1', 'c1')).toBeNull()
    expect(saveReconnectFn).not.toHaveBeenCalled()
  })

  it('returns null when the active segment has no terminal session', async () => {
    getChatFn.mockResolvedValue(
      detail({
        activeSegmentId: 's1',
        segments: [
          {
            id: 's1',
            terminalSessionId: '',
            status: 'active',
            providerId: 'claude',
            crowbarSegmentId: 's1',
            startedAt: '',
          },
        ],
      }),
    )
    expect(await attachAgentSegment('w1', 'c1')).toBeNull()
    expect(saveReconnectFn).not.toHaveBeenCalled()
  })

  // ── Dead-PTY guard ────────────────────────────────────────────────────────
  // resolveTerminalConnection's fallback for an unknown connection id is
  // createTerminal(). So seeding an id whose PTY is gone does not merely fail to
  // attach — it SPAWNS A BARE SHELL inside the agent pane and persists it to the
  // reconnect map. Both ways a segment can be stale must therefore return null.

  it('does NOT seed a PTY the daemon no longer lists as live (CLI died before EndSegment landed)', async () => {
    getChatFn.mockResolvedValue(detail()) // active segment s2 → term-2
    terminalListLiveFn.mockResolvedValue(['term-9']) // ...but term-2 is gone

    expect(await attachAgentSegment('w1', 'c1')).toBeNull()

    expect(saveReconnectFn).not.toHaveBeenCalled()
    expect(useTerminalStore.getState().getSession('term-2')).toBeUndefined()
    expect(terminalListLiveFn).toHaveBeenCalledWith(
      '/v0/projects/p1/repos/r1/workspaces/w1/terminals',
    )
  })

  it('does NOT seed when the active segment is already ended', async () => {
    // The chat still points at s1, but s1's CLI has exited — its status is `ended`.
    getChatFn.mockResolvedValue(detail({ activeSegmentId: 's1' })) // s1.status === 'ended'

    expect(await attachAgentSegment('w1', 'c1')).toBeNull()

    expect(saveReconnectFn).not.toHaveBeenCalled()
    expect(useTerminalStore.getState().getSession('term-1')).toBeUndefined()
  })

  it('does NOT seed when the liveness listing fails (never spawn on an unknown daemon state)', async () => {
    getChatFn.mockResolvedValue(detail())
    terminalListLiveFn.mockRejectedValue(new Error('daemon unreachable'))

    expect(await attachAgentSegment('w1', 'c1')).toBeNull()
    expect(saveReconnectFn).not.toHaveBeenCalled()
  })
})

// ── Component ────────────────────────────────────────────────────────
// The pane reads the chat from the workspace store and writes its tab label back
// through bufferActions, so the fake store models both: the agentChats slice and
// the two buffer actions the pane touches (a faithful, minimal stand-in for the
// real buffer slice, which is unit-tested separately).
type FakeBuffer = { id: string; name: string }
type FakeState = {
  agentChats: { chats: AgentChat[]; providers: AgentProvider[] }
  buffers: FakeBuffer[]
  bufferActions: {
    getBufferById: (id: string) => FakeBuffer | undefined
    renameBuffer: (id: string, name: string) => void
  }
}

const BUFFER_ID = 'buf-1'

function chat(overrides: Partial<AgentChat> = {}): AgentChat {
  return {
    id: 'c1',
    workspaceId: 'w1',
    title: 'Chat',
    activeSegmentId: 's2',
    activeProviderId: 'codex',
    createdAt: '',
    ...overrides,
  }
}

/** The tab starts life labelled with whatever openContent snapshotted — by
 *  default the chat's title at open time, i.e. already in sync. */
function makeStore(initial: AgentChat, tabName: string = initial.title) {
  return createStore<FakeState>((set, get) => ({
    agentChats: { chats: [initial], providers },
    buffers: [{ id: BUFFER_ID, name: tabName }],
    bufferActions: {
      getBufferById: (id) => get().buffers.find((b) => b.id === id),
      renameBuffer: (id, name) =>
        set((s) => ({ buffers: s.buffers.map((b) => (b.id === id ? { ...b, name } : b)) })),
    },
  }))
}

const tabName = (store: ReturnType<typeof makeStore>) =>
  store.getState().buffers.find((b) => b.id === BUFFER_ID)?.name

async function renderPane(store: ReturnType<typeof makeStore>, isActivePane = true) {
  let utils!: ReturnType<typeof render>
  await act(async () => {
    utils = render(
      createElement(
        WorkspaceStoreContext.Provider,
        { value: store as unknown as WorkspaceStore },
        createElement(AgentChatPane, {
          chatId: 'c1',
          wsId: 'w1',
          bufferId: BUFFER_ID,
          isActivePane,
        }),
      ),
    )
  })
  // Flush the attachAgentSegment promise chain → setSessionId → terminal mount.
  await act(async () => {})
  return utils
}

describe('AgentChatPane', () => {
  it('renders the headerless Frame footer with the provider dropdown at the chat provider', async () => {
    getChatFn.mockResolvedValue(detail())
    await renderPane(makeStore(chat()))

    const footerControl = screen.getByTestId('provider-switch')
    expect(footerControl.getAttribute('data-current')).toBe('codex')
    expect(footerControl.getAttribute('data-count')).toBe('2')
    // No frame header rendered.
    expect(document.querySelector('[data-slot="frame-panel-header"]')).toBeNull()
  })

  it('flushes the FramePanel chrome — no border, background, padding or shadow', async () => {
    getChatFn.mockResolvedValue(detail())
    await renderPane(makeStore(chat()))

    const panel = document.querySelector('[data-slot="frame-panel"]')
    expect(panel).not.toBeNull()
    // FramePanel's base class ships `shadow-xs/5` (a real box-shadow) plus the
    // `before:` pseudo-shadow; both must be neutralized or a faint ring shows
    // around the terminal, which the flush-pane spec forbids.
    for (const cls of ['shadow-none', 'before:hidden', 'border-0', 'bg-transparent', 'p-0']) {
      expect(panel?.classList.contains(cls)).toBe(true)
    }
  })

  it('pre-seeds the PTY mapping then mounts the terminal for the active segment', async () => {
    getChatFn.mockResolvedValue(detail())
    await renderPane(makeStore(chat()))

    const xterm = screen.getByTestId('xterm')
    expect(xterm.getAttribute('data-session-id')).toBe('term-2')
    expect(xterm.getAttribute('data-active')).toBe('true')
    expect(xterm.getAttribute('data-visible')).toBe('true')
    // The mapping that makes resolveTerminalConnection ATTACH exists at mount.
    expect(useTerminalStore.getState().getSession('term-2')?.connectionId).toBe('term-2')
    expect(saveReconnectFn).toHaveBeenCalledWith('w1', 'term-2', 'term-2')
  })

  it('switching the provider calls switchProvider(wsId, chatId, providerId)', async () => {
    getChatFn.mockResolvedValue(detail())
    await renderPane(makeStore(chat()))

    fireEvent.click(screen.getByTestId('provider-switch'))

    expect(switchProviderFn).toHaveBeenCalledTimes(1)
    expect(switchProviderFn).toHaveBeenCalledWith('w1', 'c1', 'codex')
  })

  it('re-seeds and remounts the terminal when the active segment changes (provider switch)', async () => {
    // After the switch, the NEW segment is the active one and the old is ended —
    // model that faithfully (the pane refuses to attach an `ended` segment).
    getChatFn.mockResolvedValueOnce(detail()).mockResolvedValueOnce(
      detail({
        activeSegmentId: 's1',
        activeProviderId: 'claude',
        segments: [
          {
            id: 's1',
            terminalSessionId: 'term-1',
            status: 'active',
            providerId: 'claude',
            providerSessionId: '',
            crowbarSegmentId: 's1',
            startedAt: '',
          },
          {
            id: 's2',
            terminalSessionId: 'term-2',
            status: 'ended',
            providerId: 'codex',
            providerSessionId: '',
            crowbarSegmentId: 's2',
            startedAt: '',
          },
        ],
      }),
    )
    const store = makeStore(chat())
    await renderPane(store)

    expect(screen.getByTestId('xterm').getAttribute('data-session-id')).toBe('term-2')

    // The WS refetch delivers the switch as an activeSegmentId change on the chat.
    await act(async () => {
      store.setState({
        agentChats: {
          chats: [chat({ activeSegmentId: 's1', activeProviderId: 'claude' })],
          providers,
        },
      })
    })
    await act(async () => {})

    const xterm = screen.getByTestId('xterm')
    expect(xterm.getAttribute('data-session-id')).toBe('term-1')
    expect(useTerminalStore.getState().getSession('term-1')?.connectionId).toBe('term-1')
    expect(saveReconnectFn).toHaveBeenCalledWith('w1', 'term-1', 'term-1')
  })

  it('renders the footer but no terminal when the active segment has no live session', async () => {
    getChatFn.mockResolvedValue(detail({ activeSegmentId: '', segments: [] }))
    await renderPane(makeStore(chat({ activeSegmentId: '' })))

    expect(screen.queryByTestId('xterm')).toBeNull()
    expect(screen.getByTestId('provider-switch')).toBeTruthy()
  })

  it('threads isActivePane=false through to the terminal', async () => {
    getChatFn.mockResolvedValue(detail())
    await renderPane(makeStore(chat()), false)

    expect(screen.getByTestId('xterm').getAttribute('data-active')).toBe('false')
  })

  it('falls back to empty provider/segment when the chat is not yet in the store', async () => {
    getChatFn.mockResolvedValue(detail())
    // Store holds a different chat id — selectors take their ?? null / ?? '' path.
    const store = makeStore(chat({ id: 'other' }), 'Codex chat')
    await renderPane(store)

    expect(screen.getByTestId('provider-switch').getAttribute('data-current')).toBe('')
    // The effect still attaches from getChat's detail regardless of store state.
    expect(screen.getByTestId('xterm').getAttribute('data-session-id')).toBe('term-2')
    // With no chat (hence no title) the tab label is left exactly as opened.
    expect(tabName(store)).toBe('Codex chat')
  })

  // ── Tab-title sync ─────────────────────────────────────────────────
  // openContent snapshots the tab label at open time; the chat's title changes
  // later (agent auto-title over WS `title_set`, or a user rename) and both land
  // on the store chat's `title`. The pane mirrors title → buffer name.
  describe('tab title tracks the chat title', () => {
    it('relabels the tab when the AGENT auto-titles the chat (WS title_set)', async () => {
      getChatFn.mockResolvedValue(detail())
      // Tab opened with the provider placeholder, in sync with the chat's title.
      const store = makeStore(chat({ title: 'Codex chat' }), 'Codex chat')
      await renderPane(store)
      expect(tabName(store)).toBe('Codex chat')

      // The WS title_set frame lands as a refetch+upsert of the chat.
      await act(async () => {
        store.setState({
          agentChats: { chats: [chat({ title: 'Fix the flaky test' })], providers },
        })
      })

      expect(tabName(store)).toBe('Fix the flaky test')
    })

    it('relabels the tab when the USER renames the chat', async () => {
      getChatFn.mockResolvedValue(detail())
      const store = makeStore(chat({ title: 'Codex chat' }), 'Codex chat')
      await renderPane(store)

      // The sidebar rename optimistically upserts the chat with the new title.
      await act(async () => {
        store.setState({
          agentChats: { chats: [chat({ title: 'Renamed by hand' })], providers },
        })
      })

      expect(tabName(store)).toBe('Renamed by hand')
    })

    it('syncs a tab opened with a stale placeholder on mount', async () => {
      getChatFn.mockResolvedValue(detail())
      // Tab label lags the chat's title (e.g. reopened from a persisted layout).
      const store = makeStore(chat({ title: 'Already titled' }), 'Codex chat')
      await renderPane(store)

      expect(tabName(store)).toBe('Already titled')
    })

    it('never blanks the tab when the chat title is empty', async () => {
      getChatFn.mockResolvedValue(detail())
      const store = makeStore(chat({ title: '' }), 'Codex chat')
      await renderPane(store)

      expect(tabName(store)).toBe('Codex chat')
    })
  })

  describe('dead PTY', () => {
    // Opening a chat whose CLI is gone REVIVES it: the ended segment still carries
    // the provider and the native session id it bound, so the backend can put the
    // user back exactly where they left off. This used to be a dead end — the pane
    // said "switch provider to start a new one" while the switch endpoint 404'd on
    // a chat with no active segment, so the chat could never be re-entered at all.
    it('revives the chat instead of mounting a terminal when the PTY is gone', async () => {
      getChatFn.mockResolvedValue(detail()) // active segment s2 → term-2
      terminalListLiveFn.mockResolvedValue([]) // the daemon has already reaped it
      resumeChatFn.mockResolvedValue('s3')

      await renderPane(makeStore(chat()))

      // The bug the attach guard prevents: mounting XtermTerminal with a dead id
      // makes resolveTerminalConnection fall through to createTerminal(), turning
      // the agent pane into a plain shell.
      expect(screen.queryByTestId('xterm')).toBeNull()
      expect(resumeChatFn).toHaveBeenCalledWith('w1', 'c1')
      expect(screen.getByTestId('provider-switch')).toBeTruthy()
    })

    it('attaches the revived PTY once the agent is back', async () => {
      // First resolution: PTY gone. After the revive, the daemon lists the new one.
      terminalListLiveFn.mockResolvedValueOnce([]).mockResolvedValue(['term-2'])
      getChatFn.mockResolvedValue(detail())
      resumeChatFn.mockResolvedValue('s3')

      await renderPane(makeStore(chat()))

      expect(resumeChatFn).toHaveBeenCalledTimes(1)
      expect(screen.getByTestId('xterm').getAttribute('data-session-id')).toBe('term-2')
    })

    // A revived CLI that dies on STARTUP (e.g. it was resumed into a session its
    // vendor never wrote) ends its segment at once, and every revive mints a NEW
    // segment id. A guard keyed on the segment would therefore see a fresh key each
    // round and respawn forever — so the cap must be a COUNT, not a key.
    it('stops respawning when every revived CLI dies immediately (new segment id each time)', async () => {
      getChatFn.mockResolvedValue(detail())
      terminalListLiveFn.mockResolvedValue([]) // every revived PTY is already gone
      let seg = 0
      resumeChatFn.mockImplementation(() => Promise.resolve(`s-revived-${++seg}`))

      const store = makeStore(chat())
      await renderPane(store)

      // Each revive lands a new active segment on the store, re-running the effect.
      for (const id of ['s-revived-1', 's-revived-2', 's-revived-3']) {
        await act(async () => {
          store.setState((st) => ({
            agentChats: {
              ...st.agentChats,
              chats: st.agentChats.chats.map((c) =>
                c.id === 'c1' ? { ...c, activeSegmentId: id } : c,
              ),
            },
          }))
        })
      }

      expect(resumeChatFn.mock.calls.length).toBeLessThanOrEqual(2)
      expect(screen.getByRole('button', { name: /resume/i })).toBeTruthy()
    })

    it('surfaces a toast and stops retrying when the agent cannot be restarted', async () => {
      const err = vi.spyOn(console, 'error').mockImplementation(() => {})
      getChatFn.mockResolvedValue(detail())
      terminalListLiveFn.mockResolvedValue([])
      resumeChatFn.mockRejectedValue(new Error('claude: not on PATH'))

      await renderPane(makeStore(chat()))

      expect(toastErrorFn).toHaveBeenCalledWith('Could not resume this chat', expect.any(String))
      // Exactly once: a CLI that cannot start must settle, never spawn-loop.
      expect(resumeChatFn).toHaveBeenCalledTimes(1)
      expect(screen.getByRole('button', { name: /resume/i })).toBeTruthy()
      err.mockRestore()
    })

    // The mount guard above only proves the PTY was alive at OPEN. The CLI can
    // die at ANY moment while the pane sits here — that was the live repro:
    // daemon restarted under an open pane, its transport dropped, and the
    // terminal's reconnect happily spawned a BARE SHELL into the agent frame.
    it('mounts the terminal ATTACH-ONLY, so a reconnect can never spawn a shell into the pane', async () => {
      getChatFn.mockResolvedValue(detail())
      await renderPane(makeStore(chat()))

      expect(screen.getByTestId('xterm').getAttribute('data-attach-only')).toBe('true')
    })

    // A CLI dying under the OPEN pane must NOT auto-revive: the user may have just
    // quit it themselves (/exit), and respawning it behind their back would be a
    // surprise. They get a button — and simply reopening the chat later revives it,
    // because that reopen is an explicit "I want this chat back".
    it('offers Resume (and does not respawn) when the PTY dies under the OPEN pane', async () => {
      getChatFn.mockResolvedValue(detail())
      await renderPane(makeStore(chat()))
      expect(screen.getByTestId('xterm')).toBeTruthy()

      // The terminal's attach-only reconnect found the session gone and said so.
      await act(async () => {
        fireEvent.click(screen.getByTestId('xterm'))
      })

      expect(screen.queryByTestId('xterm')).toBeNull()
      expect(screen.getByText(/this agent has exited/i)).toBeTruthy()
      expect(resumeChatFn).not.toHaveBeenCalled()
      expect(screen.getByTestId('provider-switch')).toBeTruthy()

      // ...and the button revives it on demand.
      terminalListLiveFn.mockResolvedValue(['term-2'])
      resumeChatFn.mockResolvedValue('s3')
      await act(async () => {
        fireEvent.click(screen.getByRole('button', { name: /resume/i }))
      })
      expect(resumeChatFn).toHaveBeenCalledWith('w1', 'c1')
      expect(screen.getByTestId('xterm')).toBeTruthy()
    })
  })

  describe('provider switch failure', () => {
    it('surfaces a toast when the switch rejects (target CLI missing / spawn failed)', async () => {
      const err = vi.spyOn(console, 'error').mockImplementation(() => {})
      getChatFn.mockResolvedValue(detail())
      switchProviderFn.mockRejectedValue(new Error('500: codex not installed'))
      await renderPane(makeStore(chat()))

      await act(async () => {
        fireEvent.click(screen.getByTestId('provider-switch'))
      })

      // Without a .catch this is an unhandled rejection: the dropdown just closes
      // and the user is told nothing.
      expect(toastErrorFn).toHaveBeenCalledTimes(1)
      const [title, description] = toastErrorFn.mock.calls[0] as [string, string]
      expect(title).toMatch(/could not switch provider/i)
      expect(description).toContain('Codex') // the target provider's display name
      expect(err).toHaveBeenCalled()
      err.mockRestore()
    })

    it('shows no toast when the switch succeeds', async () => {
      getChatFn.mockResolvedValue(detail())
      await renderPane(makeStore(chat()))

      await act(async () => {
        fireEvent.click(screen.getByTestId('provider-switch'))
      })

      expect(switchProviderFn).toHaveBeenCalledWith('w1', 'c1', 'codex')
      expect(toastErrorFn).not.toHaveBeenCalled()
    })
  })
})
