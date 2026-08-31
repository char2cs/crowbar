/**
 * ORDERING FOR THE READS THAT DECIDE WHETHER A CHAT IS LIVE.
 *
 * `liveRunnerId` is the whole liveness contract (agent-api.ts), and it is written from
 * HTTP reads that resolve in whatever order the daemon serves them. Issue order is not
 * resolution order, and — the part that makes this a correctness problem rather than a
 * jitter problem — issue order is not even ANSWER order: the daemon can serve a read from
 * before a runner placement it has ALREADY announced on the socket. So a read issued at T
 * can carry an answer older than one issued at T-1.
 *
 * Applied wholesale, last-writer-wins, that blanks `liveRunnerId` on a chat with a live
 * CLI on it. Nothing refetches afterwards, so it LATCHES: the pane has by then spent its
 * one revive, and it renders "This agent has exited. Resume it…" over an agent that is
 * alive — until a page reload reseeds. Confirmed live against a running daemon whose own
 * answer for that chat carried a real runner id.
 *
 * The rule this enforces is the one the rest of the agent stack already applies under
 * other names (`listSeq`, `folderSeq`, `providerWriteGeneration`): A READ ISSUED EARLIER
 * MAY NEVER BE APPLIED AFTER ONE ISSUED LATER. It is stated once, HERE, and not inside
 * any one caller, because the two racing reads belong to DIFFERENT callers — the WS hook
 * refetching off a `started` frame, and the chat pane's own `adopt()` after a resume —
 * and a guard private to either one cannot see the other's writes.
 *
 * Ticket, not timestamp: a clock can tie, and two reads issued in one tick are exactly
 * the case that matters.
 */

/** Monotonic issue clock. NEVER reset — not even between tests. Every ticket handed out
 *  is greater than every ticket already applied, so a fresh read is never mistaken for a
 *  stale one, and no suite needs to reset this registry to stay isolated. */
let issued = 0

/** Workspace to chat to the issue ticket of the newest read APPLIED to that chat. Nested
 *  rather than a composite string key so no id can be parsed back out wrong, and so a
 *  workspace's whole set is reachable for the list read's prune. */
const appliedAt = new Map<string, Map<string, number>>()

/** Workspace to how many single-chat reads have been applied in it. A list seed snapshots
 *  this before asking and refuses to publish a snapshot a per-chat read has overtaken;
 *  counted per workspace so one workspace's traffic cannot burn another's seed retries. */
const appliedCount = new Map<string, number>()

function chatsOf(wsId: string): Map<string, number> {
  const known = appliedAt.get(wsId)
  if (known) return known
  const fresh = new Map<string, number>()
  appliedAt.set(wsId, fresh)
  return fresh
}

/** Take a ticket for a read about to be ISSUED. Call it immediately before the request,
 *  never after awaiting it — the ticket records when we ASKED, which is the only thing
 *  that orders two answers. */
export function claimChatRead(): number {
  return ++issued
}

/**
 * May this landed single-chat read be applied? False when a read issued LATER has already
 * been applied to this chat, which makes this answer strictly older than what the store
 * already holds.
 *
 * Accepting also counts as a write, so an in-flight list seed learns it was overtaken.
 * Callers must therefore treat accept and apply as one step: never accept a read and then
 * decline to write it, or the seed will stand down for a write that never happened.
 */
export function acceptChatRead(wsId: string, chatId: string, ticket: number): boolean {
  const chats = chatsOf(wsId)
  if ((chats.get(chatId) ?? 0) > ticket) return false
  chats.set(chatId, ticket)
  appliedCount.set(wsId, (appliedCount.get(wsId) ?? 0) + 1)
  return true
}

/** Single-chat reads applied in this workspace so far — the seed's overtaken check. */
export function chatReadsApplied(wsId: string): number {
  return appliedCount.get(wsId) ?? 0
}

/**
 * A LIST read has just been applied: every chat in it now carries an answer at least as
 * fresh as `ticket`, so a single-chat read issued before it must no longer overwrite one.
 *
 * Chats absent from the snapshot are forgotten. The seed is a full reconcile that DROPS
 * them from the store, so this is the natural place to stop tracking them — without it
 * the map only ever grows.
 */
export function noteChatListRead(wsId: string, chatIds: readonly string[], ticket: number): void {
  const chats = chatsOf(wsId)
  const present = new Set(chatIds)
  for (const chatId of chats.keys()) {
    if (!present.has(chatId)) chats.delete(chatId)
  }
  for (const chatId of present) {
    // Never LOWER a mark: a per-chat read issued after this list request is newer still,
    // and rolling it back would re-open the door this closes.
    if ((chats.get(chatId) ?? 0) < ticket) chats.set(chatId, ticket)
  }
}

/** Drop a deleted chat's entry. Its id is never reused, so keeping it would only leak. */
export function forgetChatRead(wsId: string, chatId: string): void {
  appliedAt.get(wsId)?.delete(chatId)
}
