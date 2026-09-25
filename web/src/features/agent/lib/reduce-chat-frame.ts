import type { AgentChat } from '@/features/agent/api/agent-api'

/**
 * THE ONE RULE FOR WRITING A CHAT (spec §7-A target 1, invariant A6).
 *
 * Every chat body the daemon sends — on a WS frame or in a GET — is a whole,
 * versioned snapshot built by one server-side owner, under the same lock that
 * assigns its version. A higher version was read later and describes a state
 * at least as new. So the client needs exactly one rule, whatever the source:
 *
 *     apply a snapshot only if its version is newer than the one held.
 *
 * That rule replaces the read-order ticket registry, list sequencing, the
 * reconnect/`keepWorking`/`bornLive` flags and the reseed retry loop: each
 * existed to guess freshness from the ORDER reads were issued in, because the
 * answers carried nothing to compare. They do now.
 *
 * Pure: no store, no clock, no I/O — table-tested directly.
 */

/** One chat frame off the feed, as far as chat state is concerned. */
export interface ChatFrame {
  chatId: string
  kind: string
  runnerId?: string
  /** The snapshot's version; present on every frame the snapshot owner sends. */
  version?: number
  /** The chat's full snapshot; absent on a delete and on the tree-only kinds. */
  chat?: AgentChat
}

export type ChatFrameOutcome =
  /** The snapshot was newer and is now the chat. `previous` is what it replaced. */
  | { kind: 'applied'; chat: AgentChat; previous: AgentChat | undefined }
  /** The chat is gone. */
  | { kind: 'deleted'; chatId: string }
  /** An older (or equal) version than the one held — dropped. */
  | { kind: 'stale' }
  /** Nothing about chat state (a tree-only kind). */
  | { kind: 'none' }

/** Is `incoming` newer than `held`? A chat nobody holds takes anything. */
function isNewerSnapshot(held: AgentChat | undefined, incoming: AgentChat): boolean {
  return held === undefined || incoming.version > held.version
}

/**
 * Apply one frame to the held chats. Returns the next array (the SAME array
 * when nothing changed) and what happened, so the caller can run the effects
 * an edge implies (a pane following a runner, a background record) without
 * re-deriving the edge.
 */
export function reduceChatFrame(
  chats: readonly AgentChat[],
  frame: ChatFrame,
): { chats: readonly AgentChat[]; outcome: ChatFrameOutcome } {
  const idx = chats.findIndex((c) => c.id === frame.chatId)
  const held = idx === -1 ? undefined : chats[idx]

  if (frame.kind === 'deleted') {
    if (held === undefined) return { chats, outcome: { kind: 'deleted', chatId: frame.chatId } }
    if (frame.version !== undefined && frame.version <= held.version) {
      return { chats, outcome: { kind: 'stale' } }
    }
    return {
      chats: chats.filter((c) => c.id !== frame.chatId),
      outcome: { kind: 'deleted', chatId: frame.chatId },
    }
  }
  if (!frame.chat) return { chats, outcome: { kind: 'none' } }
  return applySnapshot(chats, frame.chat)
}

/** Apply one snapshot (from a frame or a GET) under the version rule. */
export function applySnapshot(
  chats: readonly AgentChat[],
  incoming: AgentChat,
): { chats: readonly AgentChat[]; outcome: ChatFrameOutcome } {
  const idx = chats.findIndex((c) => c.id === incoming.id)
  const held = idx === -1 ? undefined : chats[idx]
  if (!isNewerSnapshot(held, incoming)) return { chats, outcome: { kind: 'stale' } }
  const next = chats.slice()
  if (idx === -1) next.push(incoming)
  else next[idx] = incoming
  return { chats: next, outcome: { kind: 'applied', chat: incoming, previous: held } }
}

/** Did this snapshot start the chat working (a rising edge)? */
export function startedWorking(outcome: ChatFrameOutcome): boolean {
  return outcome.kind === 'applied' && outcome.chat.working && !outcome.previous?.working
}

/**
 * Did this snapshot end a turn — any working → idle edge?
 * @internal Exported for unit tests.
 */
export function stoppedWorking(outcome: ChatFrameOutcome): boolean {
  return outcome.kind === 'applied' && !outcome.chat.working && outcome.previous?.working === true
}

/** The runner this snapshot took off the chat, if it took one off. */
export function runnerLeft(outcome: ChatFrameOutcome): string | null {
  if (outcome.kind !== 'applied') return null
  const before = outcome.previous?.liveRunnerId
  if (!before || before === outcome.chat.liveRunnerId) return null
  return before
}
