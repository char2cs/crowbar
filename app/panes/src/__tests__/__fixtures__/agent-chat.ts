import type {
  AgentTerminalWait,
  SlashCatalog,
  SlashCatalogItem,
} from '@/features/agent/api/agent-chat-reads'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import { forgetOutbox } from '@/features/agent/stores/chat-outbox'
import { chatStore } from '@/features/agent/stores/chat-store'
import { entityStore } from '@/core/entities/entity-store'
import type { ChatEntity } from '@/core/entities/chat-entity'
import { markProjectsSynced } from '@/__tests__/__fixtures__/sidebar-entities'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'

let clock = 1_000

export function nextVersion(): number {
  clock += 1
  return clock
}

const UNLISTED_REPO = 'unlisted-repo'

export function chatSnapshot(chat: Partial<ChatEntity> & { id: string }): ChatEntity {
  return {
    projectId: 'p1',
    repoId: UNLISTED_REPO,
    workspaceId: 'w1',
    title: '',
    liveRunnerId: '',
    terminalSessionId: '',
    activeProviderId: '',
    working: false,
    createdAt: '2026-01-01T00:00:00Z',
    order: 0,
    phase: chat.liveRunnerId ? 'live' : 'dormant',
    ...chat,
    version: chat.version ?? nextVersion(),
  }
}

export function resetChatStore(): void {
  for (const chatId of Object.keys(chatStore.getState().outbox)) forgetOutbox(chatId)
  chatStore.setState({ feeds: {}, outbox: {}, presentations: {} })
  entityStore.setState((s) => ({ byKind: { ...s.byKind, chat: {}, telemetry: {} } }))
}

export function writeChat(chat: Partial<ChatEntity> & { id: string }): void {
  const held = entityStore.getState().byKind.chat[chat.id]?.data
  const data = chatSnapshot({ ...held, ...chat, version: nextVersion() })
  entityStore.setState((s) => ({
    byKind: { ...s.byKind, chat: { ...s.byKind.chat, [chat.id]: { v: data.version, data } } },
  }))
}

export function setChatWorking(chatId: string, working: boolean): void {
  writeChat({ id: chatId, working })
}

export function setChatTerminalWait(chatId: string, wait: AgentTerminalWait | null): void {
  writeChat({ id: chatId, terminalWait: wait ?? undefined })
}

export function seedChats(chats: Array<Partial<ChatEntity> & { id: string }>): void {
  for (const chat of chats) writeChat(chat)
  markProjectsSynced()
}

export function setProviders(providers: AgentProvider[]): void {
  useAgentProvidersStore.setState({ status: 'ready', providers })
}

export function provider(id: string, displayName: string, over: Partial<AgentProvider> = {}) {
  return {
    id,
    displayName,
    icon: '<svg/>',
    connected: true,
    enabled: true,
    mcpEnabled: true,
    hotswap: true,
    modelSelect: false,
    effortSelect: false,
    compaction: false,
    hasTerminal: true,
    terminalStartHere: false,
    ...over,
  } satisfies AgentProvider
}

export function slashCatalog(
  items: SlashCatalogItem[] = [],
  terminal: string[] = [],
): SlashCatalog {
  const builtins = terminal.map((name) => ({
    ...{ id: name, kind: 'builtin' as const, label: `/${name}`, description: '' },
    ...{ insertText: `/${name} `, source: '', terminal: true },
  }))
  return {
    providerId: 'provider-b',
    completeness: 'model_visible',
    items: [...items, ...builtins],
    warnings: [],
  }
}
