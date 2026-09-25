import type { AgentChat, AgentTerminalWait } from '@/features/agent/api/agent-api'
import type { WorkspaceStore } from '@/features/workspace/stores/workspace-store'

/**
 * Chat snapshots for tests. The store applies a chat only when its version is
 * newer than the one it holds (reduce-chat-frame.ts), exactly as the daemon's
 * frames and reads are applied — so every write a test makes goes through here
 * and gets a fresh, larger version, the way the daemon's would.
 */
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

/** Write a chat into a workspace store as a newer snapshot. */
export function writeChat(store: WorkspaceStore, chat: Partial<AgentChat> & { id: string }): void {
  const held = store.getState().agentChats.chats.find((c) => c.id === chat.id)
  store.getState().applyAgentChat(chatSnapshot({ ...held, ...chat, version: nextVersion() }))
}

/** The daemon reporting a chat's working flag moved. */
export function setChatWorking(store: WorkspaceStore, chatId: string, working: boolean): void {
  writeChat(store, { id: chatId, working })
}

/** The daemon reporting a chat's terminal-wait verdict moved. */
export function setChatTerminalWait(
  store: WorkspaceStore,
  chatId: string,
  wait: AgentTerminalWait | null,
): void {
  writeChat(store, { id: chatId, terminalWait: wait ?? undefined })
}

/** An authoritative list read, every row a fresh snapshot. */
export function seedChats(
  store: WorkspaceStore,
  chats: Array<Partial<AgentChat> & { id: string }>,
) {
  return store
    .getState()
    .seedAgentChats(chats.map((c) => chatSnapshot({ ...c, version: nextVersion() })))
}
