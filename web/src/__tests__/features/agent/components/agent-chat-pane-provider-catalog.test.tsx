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
import {
  setActiveWorkspaceId,
  getOrCreateWorkspaceStore,
  destroyWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import { useSettingsStore } from '@/features/settings/store'

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
    models: ['opus', 'sonnet'],
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

/** An already-run conversation: it is dormant, and it remembers the provider
 *  its last conversation ran under. */
function dormantChat(id: string, provider: string): AgentChat {
  return {
    id,
    workspaceId: 'w1',
    title: `Chat ${id}`,
    liveRunnerId: '',
    terminalSessionId: '',
    activeProviderId: provider,
    createdAt: '',
    order: 0,
  }
}

function detail(chat: AgentChat): AgentChatDetail {
  return {
    ...chat,
    conversations: [{ chatId: chat.id, providerId: 'claude', sessionId: 's1', firstSeenAt: '' }],
  }
}

/**
 * A workspace store that exists but was NEVER MOUNTED — exactly what
 * `getOrCreateWorkspaceStore` mints for a workspace whose `WorkspaceView` has
 * not run (a chat opened from Recents, or a pane outliving its workspace's
 * visibility). `seedProviders` lives in that hook, so `agentChats.providers`
 * is `[]` here while the machine-level list is known globally.
 */
function unmountedWorkspace(chats: AgentChat[], wsId = 'w1') {
  const store = createWorkspaceStore(wsId)
  store.getState().seedAgentChats(chats)
  return store
}

type Store = ReturnType<typeof unmountedWorkspace>

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
  listMessagesFn.mockResolvedValue({
    cursor: 1,
    oldestCursor: 1,
    hasMore: false,
    items: [{ id: 'm1', role: 'user', text: 'hi', createdAt: '', cursor: 1 }],
  })
  slashCatalogFn.mockResolvedValue([])
  listProvidersFn.mockResolvedValue(providers)
  useAgentProvidersStore.setState({ providers: [], status: 'idle' })
  useSettingsStore.setState((state) => ({
    settings: { ...state.settings, chatIsDefaultPresentation: true },
  }))
  setActiveWorkspaceId('w1')
})

describe('reopening an already-run conversation', () => {
  it('names its provider when only the GLOBAL catalogue is loaded', async () => {
    // The machine-level list IS known — Settings, ⌘N and every other global
    // surface can see it. Only this workspace's own copy never seeded.
    useAgentProvidersStore.getState().setProviders(providers)

    const chat = dormantChat('c1', 'claude')
    const store = unmountedWorkspace([chat])
    getChatFn.mockResolvedValue(detail(chat))

    const paneId = openChatPane('c1')
    await renderPane(store, paneId)
    await settle()

    const picker = screen.queryByTestId('agent-selection-picker')
    expect(
      picker,
      'the provider control must exist — a chat that ran under a provider can always name it',
    ).not.toBeNull()
    expect(picker?.getAttribute('aria-label')).toContain('Claude')
  })

  it('loads the catalogue itself when no copy is populated yet', async () => {
    // Neither copy has it: the very first surface to need it must ask, rather
    // than render a chat that can never name its own provider.
    const chat = dormantChat('c2', 'claude')
    const store = unmountedWorkspace([chat])
    getChatFn.mockResolvedValue(detail(chat))

    const paneId = openChatPane('c2')
    await renderPane(store, paneId)
    await settle()

    expect(listProvidersFn).toHaveBeenCalledWith('w1')
    expect(screen.queryByTestId('agent-selection-picker')).not.toBeNull()
  })
})

/**
 * The same defect through its REAL entry point, end to end: the production
 * registry, not a hand-built store.
 *
 * `getOrCreateWorkspaceStore` is what a pane pointed at another workspace's
 * chat actually calls, and it mints a store with no providers for a workspace
 * whose `WorkspaceView` has never mounted — the Recents case the user hits.
 * Workspace A is the mounted, seeded one; the pane shows a conversation that
 * belongs to B.
 */
describe('a pane on a workspace the app never mounted (real registry)', () => {
  it('still names the provider its conversation ran under', async () => {
    useAgentProvidersStore.getState().setProviders(providers)

    // A: mounted and seeded, exactly as WorkspaceView leaves it.
    const mounted = getOrCreateWorkspaceStore('w-mounted')
    mounted.getState().setAgentProviders(providers)
    setActiveWorkspaceId('w-mounted')

    // B: minted on demand by the pane. Nothing ever seeded it.
    const unmounted = getOrCreateWorkspaceStore('w-unmounted')
    const chat = { ...dormantChat('c3', 'claude'), workspaceId: 'w-unmounted' }
    unmounted.getState().seedAgentChats([chat])
    expect(unmounted.getState().agentChats.providers).toEqual([])

    getChatFn.mockResolvedValue(detail(chat))
    const paneId = openChatPane('c3', 'w-unmounted')
    await renderPane(unmounted as unknown as Store, paneId)
    await settle()

    const picker = screen.queryByTestId('agent-selection-picker')
    expect(picker).not.toBeNull()
    expect(picker?.getAttribute('aria-label')).toContain('Claude')

    destroyWorkspaceStore('w-mounted')
    destroyWorkspaceStore('w-unmounted')
  })
})
