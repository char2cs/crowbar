import type { AgentChat } from '@/features/agent/api/agent-chat-reads'

// Every chat body the daemon sends, frame or GET, is a whole snapshot whose
// version the snapshot owner assigns under its lock, so one rule orders them
// all: a snapshot applies only if its version is newer than the one held.

/** One lifecycle frame, as far as chat state is concerned. */
export interface ChatFrame {
  chatId: string
  kind: string
  runnerId?: string
  version?: number
  /** Absent on a delete and on the tree-only kinds. */
  chat?: AgentChat
}

export type ChatFrameOutcome =
  | { kind: 'applied'; chat: AgentChat; previous: AgentChat | undefined }
  | { kind: 'deleted'; chatId: string }
  | { kind: 'stale' }
  | { kind: 'none' }

export interface ChatEntities {
  chats: Record<string, AgentChat>
  /** Chat ids by workspace, kept here so no selector builds an array. */
  byWorkspace: Record<string, string[]>
}

function unindex(byWorkspace: ChatEntities['byWorkspace'], wsId: string, chatId: string) {
  const ids = byWorkspace[wsId]
  if (!ids?.includes(chatId)) return byWorkspace
  const rest = ids.filter((id) => id !== chatId)
  if (rest.length > 0) return { ...byWorkspace, [wsId]: rest }
  const { [wsId]: _gone, ...others } = byWorkspace
  return others
}

/** Apply one snapshot (from a frame or a GET) under the version rule. */
export function applySnapshot(
  state: ChatEntities,
  incoming: AgentChat,
): { next: ChatEntities; outcome: ChatFrameOutcome } {
  const held = state.chats[incoming.id]
  if (held && incoming.version <= held.version) return { next: state, outcome: { kind: 'stale' } }
  let byWorkspace = state.byWorkspace
  if (held?.workspaceId !== incoming.workspaceId) {
    if (held) byWorkspace = unindex(byWorkspace, held.workspaceId, held.id)
    byWorkspace = {
      ...byWorkspace,
      [incoming.workspaceId]: [...(byWorkspace[incoming.workspaceId] ?? []), incoming.id],
    }
  }
  return {
    next: { chats: { ...state.chats, [incoming.id]: incoming }, byWorkspace },
    outcome: { kind: 'applied', chat: incoming, previous: held },
  }
}

export function removeChat(state: ChatEntities, chatId: string): ChatEntities {
  const held = state.chats[chatId]
  if (!held) return state
  const { [chatId]: _gone, ...chats } = state.chats
  return { chats, byWorkspace: unindex(state.byWorkspace, held.workspaceId, chatId) }
}

/** Apply one frame; `next` is the same object when nothing changed. */
export function reduceChatFrame(
  state: ChatEntities,
  frame: ChatFrame,
): { next: ChatEntities; outcome: ChatFrameOutcome } {
  if (frame.kind === 'deleted') {
    const held = state.chats[frame.chatId]
    if (held && frame.version !== undefined && frame.version <= held.version) {
      return { next: state, outcome: { kind: 'stale' } }
    }
    return {
      next: removeChat(state, frame.chatId),
      outcome: { kind: 'deleted', chatId: frame.chatId },
    }
  }
  if (!frame.chat) return { next: state, outcome: { kind: 'none' } }
  return applySnapshot(state, frame.chat)
}

/** Did this snapshot start the chat working (a rising edge)? */
export function startedWorking(outcome: ChatFrameOutcome): boolean {
  return outcome.kind === 'applied' && outcome.chat.working && !outcome.previous?.working
}

/** The runner this snapshot took off the chat, if it took one off. */
export function runnerLeft(outcome: ChatFrameOutcome): string | null {
  if (outcome.kind !== 'applied') return null
  const before = outcome.previous?.liveRunnerId
  if (!before || before === outcome.chat.liveRunnerId) return null
  return before
}
