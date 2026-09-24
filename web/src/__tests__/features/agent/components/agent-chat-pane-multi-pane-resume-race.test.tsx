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
import { nanoid } from 'nanoid'
import type { AgentChat, AgentChatDetail, AgentProvider } from '@/features/agent/api/agent-api'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import { seedChatPaneRecord } from '@/__tests__/__fixtures__/view-state'
import { chatPaneIndex } from '@/features/panes/lib/view-selectors'

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

vi.mock('@/features/terminal/components/lazy-terminal', () => ({
  LazyXtermTerminal: ({ sessionId }: { sessionId: string }) =>
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
import { nextVersion, seedChats } from '@/__tests__/__fixtures__/agent-chat'

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
    modelSelect: false,
    effortSelect: false,
    compaction: false,
    terminalStartHere: false,
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
    working: false,
    version: nextVersion(),
    phase: 'dormant',
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
    working: false,
    version: nextVersion(),
    phase: 'dormant',
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
  seedChats(store, chats)
  return store
}

type Store = ReturnType<typeof seedWorkspace>

// A chat is a PANE, not a buffer: panes are window-level (windowPaneStore) and
// carry chatId/runnerId as fields of their own. A Cmd+\ split is therefore TWO
// `PaneGroup`s holding the identical chatId — the exact shape this file is
// about — and PaneHost is what pane-container.tsx does with each of them: read
// the group, feed its chatId/runnerId back in as props.
//
// A real `PaneGroup` carries no workspace id (pane-container reads it from the
// ambient WorkspaceStoreContext), so the harness keeps it beside the pane.
const paneWorkspace = new Map<string, string>()
const shadowPanes = new Map<string, { chatId: string; runnerId: string }>()

function openChatPane(_store: Store, chatId: string, runnerId: string, wsId = 'w1') {
  const id = nanoid()
  // Law 4: the store holds one pane per chat. A second pane on the same chat
  // (the split this file is about) lives beside the store, in the harness.
  if (chatPaneIndex(windowPaneStore.getState().panes).has(chatId)) {
    shadowPanes.set(id, { chatId, runnerId })
  } else {
    seedChatPaneRecord(windowPaneStore, id, chatId, runnerId || null)
  }
  paneWorkspace.set(id, wsId)
  return id
}

function PaneHost({ paneId }: { paneId: string }) {
  const stored = useStore(windowPaneStore, (s) => s.panes[paneId])
  const shadow = shadowPanes.get(paneId)
  const group = stored ?? (shadow && { id: paneId, ...shadow })
  if (!group) return null
  return createElement(AgentChatPane, {
    chatId: group.chatId ?? '',
    runnerId: group.runnerId ?? '',
    wsId: paneWorkspace.get(paneId) ?? 'w1',
    paneId: group.id,
    isActivePane: true,
    isVisible: true,
  })
}

beforeEach(() => {
  resetWindowPaneStoreForTests()
  paneWorkspace.clear()
  shadowPanes.clear()
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
    const paneA = openChatPane(store, 'c1', '')
    const paneB = openChatPane(store, 'c1', '')

    // Both panes mount in the SAME commit — exactly what a split produces:
    // createPaneBeside adds the second `PaneGroup` to the window pane store and
    // both panes render in the next flush, not one after the other.
    await act(async () => {
      render(
        createElement(
          WorkspaceStoreContext.Provider,
          { value: store },
          createElement(
            'div',
            null,
            createElement(PaneHost, { paneId: paneA }),
            createElement(PaneHost, { paneId: paneB }),
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
    const paneA = openChatPane(store, 'c1', '')
    const paneB = openChatPane(store, 'c1', '')

    await act(async () => {
      render(
        createElement(
          WorkspaceStoreContext.Provider,
          { value: store },
          createElement(
            'div',
            null,
            createElement(PaneHost, { paneId: paneA }),
            createElement(PaneHost, { paneId: paneB }),
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
