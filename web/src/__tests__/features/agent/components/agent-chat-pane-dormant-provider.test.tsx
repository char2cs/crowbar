import { createElement } from 'react'
import { act, render } from '@testing-library/react'
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

const {
  getChatFn,
  resumeChatFn,
  listMessagesFn,
  submitPromptFn,
  slashCatalogFn,
  saveReconnectFn,
  toastErrorFn,
  switchToTerminalFn,
  switchToNativeFn,
  switchProviderFn,
  listProvidersFn,
} = vi.hoisted(() => ({
  getChatFn: vi.fn(),
  resumeChatFn: vi.fn(),
  listMessagesFn: vi.fn(),
  submitPromptFn: vi.fn(),
  slashCatalogFn: vi.fn(),
  saveReconnectFn: vi.fn(),
  toastErrorFn: vi.fn(),
  switchToTerminalFn: vi.fn(),
  switchToNativeFn: vi.fn(),
  switchProviderFn: vi.fn(),
  listProvidersFn: vi.fn(),
}))

vi.mock('@/features/keymaps/hooks/use-effective-keymap', () => ({
  useEffectiveChordMap: () => ({ 'agent.toggleViewMode': 'mod+/' }),
}))

vi.mock('@/features/agent/api/agent-api', () => ({
  getPendingPrompt: vi.fn().mockResolvedValue(null),
  getChat: (...a: unknown[]) => getChatFn(...a),
  switchProvider: (...a: unknown[]) => switchProviderFn(...a),
  resumeChat: (...a: unknown[]) => resumeChatFn(...a),
  listChatMessages: (...a: unknown[]) => listMessagesFn(...a),
  submitAgentPrompt: (...a: unknown[]) => submitPromptFn(...a),
  getSlashCatalog: (...a: unknown[]) => slashCatalogFn(...a),
  switchToTerminal: (...a: unknown[]) => switchToTerminalFn(...a),
  switchToNative: (...a: unknown[]) => switchToNativeFn(...a),
  listProviders: (...a: unknown[]) => listProvidersFn(...a),
}))

vi.mock('@/features/terminal/lib/terminal-reconnect-map', () => ({
  saveReconnect: (...a: unknown[]) => saveReconnectFn(...a),
}))

vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: (...a: unknown[]) => toastErrorFn(...a) },
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
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { useSettingsStore } from '@/features/settings/store'

// claude FIRST and enabled — the exact catalogue order that made
// `providers.find((p) => p.enabled)` answer "claude" for every chat whose
// provider the daemon could not name.
const providers: AgentProvider[] = [
  {
    id: 'claude',
    displayName: 'Claude',
    icon: '<svg/>',
    connected: true,
    enabled: true,
    mcpEnabled: true,
    hasTerminal: true,
    hotswap: true,
    modelSelect: true,
    models: ['opus'],
  },
  {
    id: 'codex',
    displayName: 'Codex',
    icon: '<svg/>',
    connected: true,
    enabled: true,
    mcpEnabled: true,
    hasTerminal: true,
    hotswap: true,
    modelSelect: true,
    models: ['gpt-5'],
  },
]

/** The measured shape: a chat born on codex whose CLI was killed. It is
 *  dormant AND the daemon reports its provider as '' — codex never bound a
 *  conversation row and the chat never carried a provider_switched marker. */
function dormantWithUnknownProvider(id: string): AgentChat {
  return {
    id,
    workspaceId: 'w1',
    title: `Chat ${id}`,
    liveRunnerId: '',
    terminalSessionId: '',
    activeProviderId: '',
    createdAt: '',
    order: 0,
  }
}

function detail(chat: AgentChat): AgentChatDetail {
  return { ...chat, conversations: [] }
}

function seededWorkspace(chats: AgentChat[]) {
  const store = createWorkspaceStore('w1')
  store.getState().seedAgentChats(chats)
  store.getState().setAgentProviders(providers)
  return store
}

type Store = ReturnType<typeof seededWorkspace>

const paneWorkspace = new Map<string, string>()

function openChatPane(chatId: string, wsId = 'w1') {
  const id = nanoid()
  seedChatPaneRecord(windowPaneStore, id, chatId, null)
  paneWorkspace.set(id, wsId)
  return id
}

function PaneHost({ paneId }: { paneId: string }) {
  const group = useStore(windowPaneStore, (s) => s.panes[paneId])
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

async function renderPane(store: Store, paneId: string) {
  await act(async () => {
    render(
      createElement(
        WorkspaceStoreContext.Provider,
        { value: store },
        createElement(PaneHost, { paneId }),
      ),
    )
  })
}

async function settle(rounds = 12) {
  for (let index = 0; index < rounds; index++) {
    await act(async () => {
      await Promise.resolve()
    })
  }
}

beforeEach(() => {
  resetWindowPaneStoreForTests()
  paneWorkspace.clear()
  for (const f of [
    getChatFn,
    resumeChatFn,
    listMessagesFn,
    submitPromptFn,
    slashCatalogFn,
    saveReconnectFn,
    toastErrorFn,
    switchToTerminalFn,
    switchToNativeFn,
    switchProviderFn,
    listProvidersFn,
  ]) {
    f.mockReset()
  }
  listMessagesFn.mockResolvedValue({ cursor: 0, oldestCursor: 0, hasMore: false, items: [] })
  slashCatalogFn.mockResolvedValue([])
  listProvidersFn.mockResolvedValue(providers)
  resumeChatFn.mockResolvedValue('r-revived')
  switchProviderFn.mockResolvedValue('r-switched')
  useAgentProvidersStore.setState({ providers: [], status: 'idle' })
  useSettingsStore.setState((state) => ({
    settings: { ...state.settings, chatIsDefaultPresentation: true },
  }))
  setActiveWorkspaceId('w1')
})

/**
 * Re-opening a dormant chat must never change which vendor it runs.
 *
 * Live-reproduced 3/3: a codex chat whose process was killed reports
 * activeProviderId '' — and the pane read that absence as "never ran", picked
 * the FIRST ENABLED provider (claude) and POSTed .../switch. The chat came back
 * as claude, keeping its codex transcript, with no warning and no marker
 * recording the conversion.
 */
describe('re-opening a dormant chat whose provider the daemon cannot name', () => {
  it('never invents a provider to switch it to', async () => {
    useAgentProvidersStore.getState().setProviders(providers)
    const chat = dormantWithUnknownProvider('c1')
    const store = seededWorkspace([chat])
    getChatFn.mockResolvedValue(detail(chat))

    await renderPane(store, openChatPane('c1'))
    await settle()

    expect(
      switchProviderFn,
      'opening a chat is not asking to change its provider — the switch converted a codex chat to claude',
    ).not.toHaveBeenCalled()
  })

  it('asks the daemon to resume it instead, which is what resolves its provider', async () => {
    useAgentProvidersStore.getState().setProviders(providers)
    const chat = dormantWithUnknownProvider('c2')
    const store = seededWorkspace([chat])
    getChatFn.mockResolvedValue(detail(chat))

    await renderPane(store, openChatPane('c2'))
    await settle()

    expect(resumeChatFn).toHaveBeenCalled()
    expect(resumeChatFn.mock.calls[0]?.[1]).toBe('c2')
  })

  // The control the live repro also ran: a dormant chat the daemon CAN name
  // already resumes rather than switching. It must keep doing so.
  it('still resumes a dormant chat whose provider IS known', async () => {
    useAgentProvidersStore.getState().setProviders(providers)
    const chat = { ...dormantWithUnknownProvider('c3'), activeProviderId: 'codex' }
    const store = seededWorkspace([chat])
    getChatFn.mockResolvedValue(detail(chat))

    await renderPane(store, openChatPane('c3'))
    await settle()

    expect(switchProviderFn).not.toHaveBeenCalled()
    expect(resumeChatFn).toHaveBeenCalled()
  })
})
