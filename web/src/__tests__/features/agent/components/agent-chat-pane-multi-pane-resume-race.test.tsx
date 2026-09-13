/**
 * TWO PANES, ONE CHAT, ONE RESUME.
 *
 * Splitting a chat pane (Cmd+\ / mod+\, `createPaneBeside`) mounts a SECOND
 * AgentChatPane pointed at the identical chatId — a supported, ordinary gesture,
 * not a misuse. Both panes' attach effects run in the same commit; both read the
 * chat as dormant before either has adopted; and each pane's own per-component
 * `attemptedRef` budget let both call revive(), so both fired resumeChat for one
 * chat. The daemon's per-chat spawn gate (api/internal/.../gate/gate.go)
 * serialises the two requests rather than coalescing them, so the LOSING pane's
 * resumeChat either queues behind the winner's full DefaultStallQuiet wait or
 * answers stale — live-verified as "Couldn't resume Claude chat: The daemon did
 * not answer the resume" on the second panel of an ordinary split, for a chat
 * the first panel was reviving just fine.
 *
 * The fix shares one in-flight revive per chatId (module-level
 * `reviveInFlightByChatId` in agent-chat-pane.tsx) across every pane showing
 * it, so a second mount piggybacks on the first pane's outcome instead of
 * firing a second resumeChat the gate would only queue behind.
 */
import { createElement } from 'react'
import { act, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useStore } from 'zustand'
import type { AgentChat, AgentChatDetail, AgentProvider } from '@/features/agent/api/agent-api'
import type { AgentChatContent } from '@/features/panes/types/pane-content'
import {
  WorkspaceStoreContext,
  useWorkspaceStore,
} from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'

const { getChatFn, resumeChatFn, listMessagesFn, submitPromptFn, slashCatalogFn, saveReconnectFn } =
  vi.hoisted(() => ({
    getChatFn: vi.fn(),
    resumeChatFn: vi.fn(),
    listMessagesFn: vi.fn(),
    submitPromptFn: vi.fn(),
    slashCatalogFn: vi.fn(),
    saveReconnectFn: vi.fn(),
  }))

vi.mock('@/features/keymaps/hooks/use-effective-keymap', () => ({
  useEffectiveChordMap: () => ({ 'agent.toggleViewMode': 'mod+/' }),
}))

vi.mock('@/features/agent/api/agent-api', () => ({
  getPendingPrompt: vi.fn().mockResolvedValue(null),
  getChat: (...a: unknown[]) => getChatFn(...a),
  switchProvider: vi.fn(),
  resumeChat: (...a: unknown[]) => resumeChatFn(...a),
  listChatMessages: (...a: unknown[]) => listMessagesFn(...a),
  submitAgentPrompt: (...a: unknown[]) => submitPromptFn(...a),
  getSlashCatalog: (...a: unknown[]) => slashCatalogFn(...a),
  switchToTerminal: vi.fn(),
  switchToNative: vi.fn(),
}))

vi.mock('@/features/terminal/lib/terminal-reconnect-map', () => ({
  saveReconnect: (...a: unknown[]) => saveReconnectFn(...a),
}))

vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: vi.fn() },
}))

vi.mock('@/features/terminal/components/terminal', () => ({
  XtermTerminal: ({ sessionId }: { sessionId: string }) =>
    createElement('div', { 'data-testid': 'xterm', 'data-session-id': sessionId }),
}))

vi.mock('@/features/agent/composer/plate/chat-markdown-editor', () => ({
  ChatMarkdownEditor: ({ ariaLabel, placeholder }: { ariaLabel: string; placeholder: string }) =>
    createElement('textarea', { 'aria-label': ariaLabel, placeholder }),
}))

vi.mock('@/features/agent/components/provider-switch-dropdown', () => ({
  ProviderSwitchDropdown: () => createElement('button', { 'data-testid': 'provider-switch' }),
}))

import { AgentChatPane } from '@/features/agent/components/agent-chat-pane'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { useTerminalStore } from '@/features/terminal/stores/terminal-store'
import { useSettingsStore } from '@/features/settings/store'

const providers: AgentProvider[] = [
  {
    id: 'codex',
    displayName: 'Codex',
    icon: '<svg/>',
    connected: true,
    enabled: true,
    mcpEnabled: true,
    hasTerminal: true,
    hotswap: true,
  },
]

function dormantChat(o: { id: string }): AgentChat {
  return {
    id: o.id,
    workspaceId: 'w1',
    title: `Chat ${o.id}`,
    liveRunnerId: '',
    terminalSessionId: '',
    activeProviderId: 'codex',
    createdAt: '',
    order: 0,
  }
}

function liveChat(o: { id: string; runnerId: string; pty: string }): AgentChat {
  return {
    id: o.id,
    workspaceId: 'w1',
    title: `Chat ${o.id}`,
    liveRunnerId: o.runnerId,
    terminalSessionId: o.pty,
    activeProviderId: 'codex',
    createdAt: '',
    order: 0,
  }
}

function detail(chat: AgentChat): AgentChatDetail {
  return { ...chat, conversations: [] }
}

function seedWorkspace(chats: AgentChat[], wsId = 'w1') {
  const store = createWorkspaceStore(wsId)
  store.getState().setAgentProviders(providers)
  store.getState().seedAgentChats(chats)
  return store
}

type Store = ReturnType<typeof seedWorkspace>

function openBuffer(store: Store, chatId: string, runnerId: string) {
  return store
    .getState()
    .bufferActions.openContent({ type: 'agentChat', chatId, wsId: 'w1', name: 'Chat', runnerId })
}

function PaneHost({ bufferId }: { bufferId: string }) {
  const store = useWorkspaceStore()
  const buf = useStore(store, (s) => s.buffers.find((b) => b.id === bufferId)) as
    AgentChatContent | undefined
  if (!buf) return null
  return createElement(AgentChatPane, {
    chatId: buf.chatId,
    runnerId: buf.runnerId,
    wsId: buf.wsId,
    bufferId: buf.id,
    isActivePane: true,
    isVisible: true,
  })
}

beforeEach(() => {
  for (const f of [
    getChatFn,
    resumeChatFn,
    listMessagesFn,
    submitPromptFn,
    slashCatalogFn,
    saveReconnectFn,
  ])
    f.mockReset()
  listMessagesFn.mockResolvedValue({ cursor: 1, oldestCursor: 1, hasMore: false, items: [] })
  submitPromptFn.mockResolvedValue({ runnerId: 'r1', terminalSessionId: 'pty1' })
  slashCatalogFn.mockResolvedValue({
    providerId: 'codex',
    completeness: 'model_visible',
    items: [],
    warnings: [],
  })
  useTerminalStore.setState({ sessions: new Map() })
  useSettingsStore.setState((state) => ({
    settings: { ...state.settings, chatIsDefaultPresentation: true },
  }))
  localStorage.clear()
  setActiveWorkspaceId('w1')
})

describe('AgentChatPane: two panes sharing one chat (a Cmd+\\ split)', () => {
  it('fires exactly one resumeChat for the shared chat, not one per pane', async () => {
    let resolveResume!: (id: string) => void
    resumeChatFn.mockImplementation(
      () =>
        new Promise<string>((resolve) => {
          resolveResume = resolve
        }),
    )
    getChatFn.mockImplementation((_wsId: unknown, id: unknown) =>
      Promise.resolve(
        detail(liveChat({ id: String(id), runnerId: 'r-revived', pty: 'pty-revived' })),
      ),
    )

    const store = seedWorkspace([dormantChat({ id: 'c1' })])
    const bufferA = openBuffer(store, 'c1', '')
    const bufferB = openBuffer(store, 'c1', '')

    // Both panes mount in the SAME commit — exactly what a split produces:
    // createPaneBeside adds the second pane's buffer to the store and both
    // panes render in the next flush, not one after the other.
    await act(async () => {
      render(
        createElement(
          WorkspaceStoreContext.Provider,
          { value: store },
          createElement(
            'div',
            null,
            createElement(PaneHost, { bufferId: bufferA }),
            createElement(PaneHost, { bufferId: bufferB }),
          ),
        ),
      )
    })

    // THE REGRESSION ASSERTION: without the fix this is 2 — one resumeChat per
    // pane, racing the daemon's per-chat spawn gate against each other.
    expect(resumeChatFn).toHaveBeenCalledTimes(1)

    // Both panes show the buttonless spinner while the one shared request is
    // out — neither pane silently disowns it.
    expect(screen.getAllByText(/resuming this chat/i)).toHaveLength(2)

    // The single resume lands...
    await act(async () => {
      resolveResume('r-revived')
    })
    await act(async () => {})

    // ...and BOTH panes attach off it — the piggybacking pane included, not
    // just the one that actually made the request.
    const xterms = screen.getAllByTestId('xterm')
    expect(xterms).toHaveLength(2)
    for (const el of xterms) expect(el).toHaveAttribute('data-session-id', 'pty-revived')
  })

  it('settles the piggybacking pane onto Resume too when the shared resume fails', async () => {
    let rejectResume!: (err: unknown) => void
    resumeChatFn.mockImplementation(
      () =>
        new Promise<string>((_resolve, reject) => {
          rejectResume = reject
        }),
    )

    const store = seedWorkspace([dormantChat({ id: 'c1' })])
    const bufferA = openBuffer(store, 'c1', '')
    const bufferB = openBuffer(store, 'c1', '')

    await act(async () => {
      render(
        createElement(
          WorkspaceStoreContext.Provider,
          { value: store },
          createElement(
            'div',
            null,
            createElement(PaneHost, { bufferId: bufferA }),
            createElement(PaneHost, { bufferId: bufferB }),
          ),
        ),
      )
    })
    expect(resumeChatFn).toHaveBeenCalledTimes(1)

    await act(async () => {
      rejectResume(new Error('boom'))
    })
    await act(async () => {})

    // Neither pane is left spinning forever on a request only ONE of them
    // actually made — the piggybacker must also learn the outcome.
    expect(screen.queryByText(/resuming this chat/i)).not.toBeInTheDocument()
    expect(screen.getAllByTestId('pane-resume')).toHaveLength(2)
  })
})
