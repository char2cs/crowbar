import type { AgentChat, AgentTerminalWait } from '@/features/agent/api/agent-chat-reads'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import { applyChat, chatStore, setChatScope } from '@/features/agent/stores/chat-store'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'

// The store applies a chat only when its version is newer than the one held,
// so every write a test makes goes through here and gets a larger version.
let clock = 1_000

/** A version larger than any this module has handed out. */
export function nextVersion(): number {
  clock += 1
  return clock
}

/** A chat snapshot with every required field defaulted. */
export function chatSnapshot(chat: Partial<AgentChat> & { id: string }): AgentChat {
  return {
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
  chatStore.setState({ feeds: {}, chats: {}, byWorkspace: {}, scopes: {}, telemetry: {} })
}

/** Writes a chat into the ChatStore as a newer snapshot of what it holds. */
export function writeChat(chat: Partial<AgentChat> & { id: string }): void {
  const held = chatStore.getState().chats[chat.id]
  applyChat(chatSnapshot({ ...held, ...chat, version: nextVersion() }))
}

/** The daemon reporting a chat's working flag moved. */
export function setChatWorking(chatId: string, working: boolean): void {
  writeChat({ id: chatId, working })
}

/** The daemon reporting a chat's terminal-wait verdict moved. */
export function setChatTerminalWait(chatId: string, wait: AgentTerminalWait | null): void {
  writeChat({ id: chatId, terminalWait: wait ?? undefined })
}

/** A landed list for `wsId`: every row a fresh snapshot, and the scope ready. */
export function seedChats(chats: Array<Partial<AgentChat> & { id: string }>, wsId = 'w1'): void {
  for (const chat of chats) writeChat(chat)
  setChatScope(wsId, 'ready')
}

/** The machine's provider list, loaded. */
export function setProviders(providers: AgentProvider[]): void {
  useAgentProvidersStore.setState({ status: 'ready', providers })
}
