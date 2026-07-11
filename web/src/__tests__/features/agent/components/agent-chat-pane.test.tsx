import { createElement } from 'react'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createStore } from 'zustand'
import type { AgentChat, AgentChatDetail, AgentProvider } from '@/features/agent/api/agent-api'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import type { WorkspaceStore } from '@/features/workspace/stores/workspace-store'

// Hoisted fakes — declared before the vi.mock calls that reference them.
const { getChatFn, switchProviderFn, saveReconnectFn } = vi.hoisted(() => ({
  getChatFn: vi.fn(),
  switchProviderFn: vi.fn(),
  saveReconnectFn: vi.fn(),
}))

vi.mock('@/features/agent/api/agent-api', () => ({
  getChat: (...a: unknown[]) => getChatFn(...a),
  switchProvider: (...a: unknown[]) => switchProviderFn(...a),
}))

vi.mock('@/features/terminal/lib/terminal-reconnect-map', () => ({
  saveReconnect: (...a: unknown[]) => saveReconnectFn(...a),
}))

// jsdom can't run xterm/WebGL — stub the terminal renderer to a passive marker
// that records the sessionId it was mounted with (that's what the attach seam
// is proven by) plus the isActive/isVisible props threaded from the pane.
vi.mock('@/features/terminal/components/terminal', () => ({
  XtermTerminal: ({
    sessionId,
    isActive,
    isVisible,
  }: {
    sessionId: string
    isActive: boolean
    isVisible?: boolean
  }) =>
    createElement('div', {
      'data-testid': 'xterm',
      'data-session-id': sessionId,
      'data-active': String(isActive),
      'data-visible': String(isVisible),
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
  saveReconnectFn.mockReset()
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
})

// ── Component ────────────────────────────────────────────────────────
type FakeState = { agentChats: { chats: AgentChat[]; providers: AgentProvider[] } }

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

function makeStore(initial: AgentChat) {
  return createStore<FakeState>(() => ({
    agentChats: { chats: [initial], providers },
  }))
}

async function renderPane(store: ReturnType<typeof makeStore>, isActivePane = true) {
  let utils!: ReturnType<typeof render>
  await act(async () => {
    utils = render(
      createElement(
        WorkspaceStoreContext.Provider,
        { value: store as unknown as WorkspaceStore },
        createElement(AgentChatPane, { chatId: 'c1', wsId: 'w1', isActivePane }),
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
    getChatFn
      .mockResolvedValueOnce(detail())
      .mockResolvedValueOnce(detail({ activeSegmentId: 's1', activeProviderId: 'claude' }))
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
    const store = createStore<FakeState>(() => ({
      agentChats: { chats: [chat({ id: 'other' })], providers },
    }))
    await act(async () => {
      render(
        createElement(
          WorkspaceStoreContext.Provider,
          { value: store as unknown as WorkspaceStore },
          createElement(AgentChatPane, { chatId: 'c1', wsId: 'w1', isActivePane: true }),
        ),
      )
    })
    await act(async () => {})

    expect(screen.getByTestId('provider-switch').getAttribute('data-current')).toBe('')
    // The effect still attaches from getChat's detail regardless of store state.
    expect(screen.getByTestId('xterm').getAttribute('data-session-id')).toBe('term-2')
  })
})
