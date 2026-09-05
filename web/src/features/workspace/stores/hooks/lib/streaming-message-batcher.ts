export interface StreamingMessage {
  id: string
  text: string
}

export interface StreamingMessageBatcher {
  /** Records the latest message for `(chatId, message.id)`, superseding any
   *  still pending for that SAME id, and arms a flush if none is already
   *  scheduled. A different id for the same chat is a SEPARATE entry, not a
   *  replacement — Codex can have more than one message item open in one
   *  turn, and collapsing by chatId alone silently drops whichever one
   *  didn't win the last write. */
  schedule(chatId: string, message: StreamingMessage): void
  /** Cancels a pending flush without applying it — the pending delta is
   *  transient (the ledger holds the finished message once the turn ends;
   *  see AgentStreamEvent.message), so dropping the last one on teardown
   *  costs nothing a live chat won't immediately re-send. */
  dispose(): void
}

/**
 * Coalesces `message_delta` frames into at most one store write per
 * `(chat, message id)` per animation frame.
 *
 * Each WS `message` event is its own top-level browser callback — outside
 * anything React 18 batches automatically — so a fast provider emitting
 * several deltas inside one 16ms tick used to cost one Zustand `set()`, and
 * therefore one full store-subscriber re-render, per delta. Buffering to the
 * next frame collapses that burst into one write per id, keyed by chat so
 * two chats streaming at once do not clobber each other — and keyed by
 * message id WITHIN a chat so two co-open items don't either. The inner map
 * is expected to hold at most a handful of entries (concurrently-open
 * message items in one turn), never the size of the transcript, so this
 * stays O(open items), not O(messages).
 */
export function createStreamingMessageBatcher(
  flush: (chatId: string, message: StreamingMessage) => void,
  raf: (cb: () => void) => number = requestAnimationFrame,
  cancelRaf: (handle: number) => void = cancelAnimationFrame,
): StreamingMessageBatcher {
  const pending = new Map<string, Map<string, StreamingMessage>>()
  let frame: number | null = null

  const runFlush = () => {
    frame = null
    for (const [chatId, byId] of pending) {
      for (const message of byId.values()) flush(chatId, message)
    }
    pending.clear()
  }

  return {
    schedule(chatId, message) {
      let byId = pending.get(chatId)
      if (!byId) pending.set(chatId, (byId = new Map()))
      byId.set(message.id, message)
      frame ??= raf(runFlush)
    },
    dispose() {
      if (frame !== null) cancelRaf(frame)
      frame = null
      pending.clear()
    },
  }
}
