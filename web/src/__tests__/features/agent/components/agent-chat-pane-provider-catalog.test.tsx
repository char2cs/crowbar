import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'
import { seedFeed, spokenIn } from '@/__tests__/__fixtures__/ledger-rows'
import { createElement } from 'react'
import { act, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useStore } from 'zustand'
import { nanoid } from 'nanoid'
import type { AgentChat, AgentChatDetail } from '@/features/agent/api/agent-chat-reads'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
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
  submitPromptFn: vi.fn(),
  slashCatalogFn: vi.fn(),
  saveReconnectFn: vi.fn(),
  toastErrorFn: vi.fn(),
  switchToTerminalFn: vi.fn(),
  switchToNativeFn: vi.fn(),
  switchProviderFn: vi.fn(),
  listProvidersFn: vi.fn(),
}))

import * as useEffectiveKeymapMod from '@/features/keymaps/hooks/use-effective-keymap'
vi.spyOn(useEffectiveKeymapMod, 'useEffectiveChordMap').mockImplementation(() => ({
  'agent.toggleViewMode': 'mod+/',
}))

import * as readsApi from '@/features/agent/api/agent-chat-reads'
vi.spyOn(readsApi, 'getChat').mockImplementation(getChatFn)
vi.spyOn(readsApi, 'getSlashCatalog').mockImplementation(slashCatalogFn)
vi.spyOn(readsApi, 'getChatTelemetry').mockResolvedValue(null)
import * as commandsApi from '@/features/agent/api/agent-chat-commands'
vi.spyOn(commandsApi, 'switchProvider').mockImplementation(switchProviderFn)
vi.spyOn(commandsApi, 'resumeChat').mockImplementation(resumeChatFn)
vi.spyOn(commandsApi, 'submitAgentPrompt').mockImplementation(submitPromptFn)
vi.spyOn(commandsApi, 'switchToTerminal').mockImplementation(switchToTerminalFn)
vi.spyOn(commandsApi, 'switchToNative').mockImplementation(switchToNativeFn)
import * as providersApi from '@/features/agent/api/agent-providers-api'
vi.spyOn(providersApi, 'listProviders').mockImplementation(listProvidersFn)

import * as terminalReconnectMapMod from '@/features/terminal/lib/terminal-reconnect-map'
vi.spyOn(terminalReconnectMapMod, 'saveReconnect').mockImplementation((...a: unknown[]) =>
  saveReconnectFn(...a),
)

import * as toastStoreMod from '@/features/window/stores/toast-store'
vi.spyOn(toastStoreMod.toast, 'error').mockImplementation((...a: unknown[]) => toastErrorFn(...a))

import * as lazyTerminalMod from '@/features/terminal/components/lazy-terminal'
vi.spyOn(lazyTerminalMod, 'LazyXtermTerminal').mockImplementation(
  ({ sessionId }: { sessionId: string }) =>
    createElement('div', { 'data-testid': 'xterm', 'data-session-id': sessionId }),
)

import * as chatMarkdownEditorMod from '@/features/agent/composer/plate/chat-markdown-editor'
vi.spyOn(chatMarkdownEditorMod, 'ChatMarkdownEditor').mockImplementation(
  ({ ariaLabel, placeholder }: { ariaLabel: string; placeholder: string }) =>
    createElement('textarea', { 'aria-label': ariaLabel, placeholder }),
)

import * as providerSwitchDropdownMod from '@/features/agent/components/provider-switch-dropdown'
vi.spyOn(providerSwitchDropdownMod, 'ProviderSwitchDropdown').mockImplementation(() =>
  createElement('button', { 'data-testid': 'provider-switch' }),
)

import { AgentChatPane } from '@/features/agent/components/agent-chat-pane'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { nextVersion, seedChats, resetChatStore } from '@/__tests__/__fixtures__/agent-chat'
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
    effortSelect: false,
    compaction: false,
    terminalStartHere: false,
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
    effortSelect: false,
    compaction: false,
    terminalStartHere: false,
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
    working: false,
    version: nextVersion(),
    phase: 'dormant',
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

/** A workspace whose `WorkspaceView` never ran: the chat is known, and the
 *  provider list is whatever the machine-level store holds. */
function unmountedWorkspace(chats: AgentChat[], wsId = 'w1') {
  const store = createWorkspaceStore(wsId)
  seedChats(chats, wsId)
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
  resetChatStore()
  for (const id of ['c1', 'c2', 'c3']) seedFeed(id, spokenIn(id))
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
