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
 *
 * ── WHAT THIS DOES NOT SETTLE ────────────────────────────────────────────────────
 *
 * ISSUE ORDER IS A PROXY FOR FRESHNESS, NOT A MEASURE OF IT. The premise above cuts both
 * ways and this registry only honours it in one direction: `noteChatListRead` stamps every
 * chat in a list response with the LIST's ticket, which treats a later-issued list read as
 * unconditionally fresher than an earlier-issued per-chat read. But `seedChats`'s own
 * long-standing comment says a list read can be served pre-placement too — so that is an
 * assumption, not a fact.
 *
 * The window it needs is narrow: a seed landing INSIDE a per-chat read's round trip, and
 * answering from before a placement the per-chat read did see. Concretely — `adopt()`
 * claims T1; a `created`/reconnect reseed claims T2 > T1; the list answers pre-placement,
 * lands first, seeds the chat dormant and stamps mark = T2; `adopt()`'s later-landing TRUE
 * answer is then refused as stale. Before this registry existed that read would have
 * applied and self-corrected, so in that one sequence this is a regression: the pane can
 * reach `idle: 'failed'` with its retry budget spent.
 *
 * It is not fixable from the client. Deciding it properly needs the server to stamp each
 * chat row with its own revision, so freshness is compared on the ANSWER rather than on
 * when we asked; the ticket would then only break ties. Until then this trades a narrow,
 * self-announcing failure (a Resume button that works when pressed) for the wide, silent,
 * latching one it was written to end (a dead pane over a live CLI that no button repairs).
 * Do not read the guarantees below as a settled precedence between the two kinds of read.
 */

/** Monotonic issue clock. NEVER reset — not even between tests. Every ticket handed out
 *  is greater than every ticket already applied, so a fresh read is never mistaken for a
 *  stale one, and no suite needs to reset this registry to stay isolated. */
let issued = 0

/** Workspace to chat to the issue ticket of the newest read APPLIED to that chat. Nested
 *  rather than a composite string key so no id can be parsed back out wrong, and so a
 *  workspace's whole set is reachable for the list read's prune.
 *
 *  DELIBERATELY NOT CLEARED when a workspace store is destroyed, even though that leaves
 *  a bounded amount of dead state behind. Clearing it would set every mark back to 0 and
 *  re-admit any read still in flight over the workspace's next mount — reinstating exactly
 *  the stale write this exists to refuse. Pruning happens where it is SAFE instead: a list
 *  read drops the chats its snapshot no longer carries, and `deleted` drops one by id. */
const appliedAt = new Map<string, Map<string, number>>()

/** Workspace to how many single-chat reads have been applied in it. A list seed snapshots
 *  this before asking and refuses to publish a snapshot a per-chat read has overtaken;
 *  counted per workspace so one workspace's traffic cannot burn another's seed retries. */
const appliedCount = new Map<string, number>()

/**
 * Workspace to the ticket of the newest LIST read applied in it — the floor under every
 * chat's mark, including chats no entry is held for.
 *
 * Without it, forgetting a chat (a list snapshot that dropped it, or a `deleted` frame)
 * resets its mark to 0 and re-admits any read of that id still in flight, resurrecting a
 * row the reconcile had just removed. A chat the newest list did not carry cannot have an
 * answer older than that list worth applying, so the floor is the honest default.
 */
const listFloor = new Map<string, number>()

function chatsOf(wsId: string): Map<string, number> {
  const known = appliedAt.get(wsId)
  if (known) return known
  const fresh = new Map<string, number>()
  appliedAt.set(wsId, fresh)
  return fresh
}

/** The newest answer we hold about this chat, expressed as the ticket of the read that
 *  produced it — falling back to the workspace's list floor, then to nothing. */
function markOf(wsId: string, chatId: string): number {
  const own = appliedAt.get(wsId)?.get(chatId)
  if (own !== undefined) return own
  return listFloor.get(wsId) ?? 0
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
  if (markOf(wsId, chatId) > ticket) return false
  chatsOf(wsId).set(chatId, ticket)
  appliedCount.set(wsId, (appliedCount.get(wsId) ?? 0) + 1)
  return true
}

/** Single-chat reads applied in this workspace so far — the seed's overtaken check. */
export function chatReadsApplied(wsId: string): number {
  return appliedCount.get(wsId) ?? 0
}

/**
 * The issue ticket of the newest answer held about this chat (the workspace's list floor
 * when no per-chat entry survives).
 *
 * Read by `upsertAgentChat`'s cross-chat eviction, which is the one write that changes a
 * chat OTHER than the one it was handed. `acceptChatRead` cannot answer that question —
 * it asks about the row being written, and eviction is about the row being trampled.
 */
export function chatReadMark(wsId: string, chatId: string): number {
  return markOf(wsId, chatId)
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
  if ((listFloor.get(wsId) ?? 0) < ticket) listFloor.set(wsId, ticket)
}

/** Drop a deleted chat's entry. Its id is never reused, so keeping it would only leak. */
export function forgetChatRead(wsId: string, chatId: string): void {
  appliedAt.get(wsId)?.delete(chatId)
}
