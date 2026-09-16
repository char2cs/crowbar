import { describe, expect, it } from 'vitest'
import {
  acceptChatRead,
  chatReadMark,
  chatReadsApplied,
  claimChatRead,
  forgetChatRead,
  noteChatListRead,
} from '@/features/agent/lib/chat-read-order'

// The registry is module state with NO reset hook, deliberately: the issue clock is
// monotonic for the life of the process, so every ticket handed out here is greater than
// every ticket any earlier case applied. Each case therefore starts from a state that
// cannot make a fresh read look stale — which is the same property that keeps it correct
// across a hook remount in the app. Ids are kept distinct per case anyway, so a failure
// reads as one case's fault rather than the previous one's.
describe('chat read ordering', () => {
  it('hands out strictly increasing tickets', () => {
    const a = claimChatRead()
    const b = claimChatRead()
    expect(b).toBeGreaterThan(a)
  })

  // The rule, and the whole point: two reads of one chat, resolving in the wrong order.
  it('rejects a read ISSUED earlier once one issued later has been applied', () => {
    const first = claimChatRead()
    const second = claimChatRead()

    expect(acceptChatRead('w1', 'later-wins', second)).toBe(true)
    expect(acceptChatRead('w1', 'later-wins', first)).toBe(false)
  })

  it('accepts reads that land in issue order', () => {
    const first = claimChatRead()
    const second = claimChatRead()

    expect(acceptChatRead('w1', 'in-order', first)).toBe(true)
    expect(acceptChatRead('w1', 'in-order', second)).toBe(true)
  })

  it('orders each chat independently — a read of one chat says nothing about another', () => {
    const first = claimChatRead()
    const second = claimChatRead()

    expect(acceptChatRead('w1', 'chat-a', second)).toBe(true)
    // Same ticket ordering, different chat: nothing has been applied to `chat-b`, so the
    // earlier read is still the newest answer anyone has for it.
    expect(acceptChatRead('w1', 'chat-b', first)).toBe(true)
  })

  it('scopes a chat to its workspace', () => {
    const first = claimChatRead()
    const second = claimChatRead()

    expect(acceptChatRead('w1', 'shared-id', second)).toBe(true)
    expect(acceptChatRead('w2', 'shared-id', first)).toBe(true)
  })

  // A list read is a fresher answer for every chat in it, so a single-chat read issued
  // before it must not walk over the reconcile it just published.
  it('a list read blocks single-chat reads issued before it', () => {
    const stale = claimChatRead()
    const list = claimChatRead()

    noteChatListRead('w1', ['listed'], list)

    expect(acceptChatRead('w1', 'listed', stale)).toBe(false)
    expect(acceptChatRead('w1', 'listed', claimChatRead())).toBe(true)
  })

  it('a list read never rolls a chat BACK to an older answer', () => {
    const list = claimChatRead()
    const fresher = claimChatRead()

    expect(acceptChatRead('w1', 'kept-fresh', fresher)).toBe(true)
    noteChatListRead('w1', ['kept-fresh'], list) // issued earlier: must not lower the mark

    // A read older than `fresher` is still refused — the list did not reset the mark.
    expect(acceptChatRead('w1', 'kept-fresh', list)).toBe(false)
  })

  // The seed is a full reconcile, so it is also the natural place to stop tracking chats
  // the server no longer has — otherwise the map only ever grows. Forgetting must not
  // mean forgetting EVERYTHING, though: a read of that id can still be in flight, and
  // re-admitting it resurrects the row the reconcile just dropped. The list floor is what
  // keeps a pruned chat from falling back to "no answer at all".
  it('a list read forgets a dropped chat WITHOUT re-admitting a read older than the list', () => {
    const inFlight = claimChatRead()
    expect(acceptChatRead('w2-prune', 'pruned', inFlight)).toBe(true)

    noteChatListRead('w2-prune', ['survivor'], claimChatRead())

    // The entry is gone, but the list that dropped it is still the newer answer.
    expect(acceptChatRead('w2-prune', 'pruned', inFlight)).toBe(false)
    // A read issued after that list is newer than it, and lands.
    expect(acceptChatRead('w2-prune', 'pruned', claimChatRead())).toBe(true)
  })

  it('the list floor covers a chat no read has ever been applied to', () => {
    const before = claimChatRead()
    noteChatListRead('w2-floor', ['known'], claimChatRead())

    expect(acceptChatRead('w2-floor', 'never-seen', before)).toBe(false)
    expect(acceptChatRead('w2-floor', 'never-seen', claimChatRead())).toBe(true)
  })

  it('forgetChatRead drops a deleted chat without re-admitting a pre-list read', () => {
    const older = claimChatRead()
    const newer = claimChatRead()
    expect(acceptChatRead('w2-del', 'deleted', newer)).toBe(true)
    expect(acceptChatRead('w2-del', 'deleted', older)).toBe(false)

    forgetChatRead('w2-del', 'deleted')
    // No list has been applied in this workspace, so there is no floor and the entry is
    // genuinely gone — the honest answer when nothing else is known.
    expect(acceptChatRead('w2-del', 'deleted', older)).toBe(true)
  })

  // ── chatReadMark: what upsertAgentChat's cross-chat eviction asks ──────────
  // Eviction writes a chat OTHER than the one handed in, so `acceptChatRead` — which
  // asks about the row being written — cannot speak for it.
  it('chatReadMark reports the newest answer held about a chat', () => {
    expect(chatReadMark('w2-mark', 'untouched')).toBe(0)

    const ticket = claimChatRead()
    acceptChatRead('w2-mark', 'touched', ticket)
    expect(chatReadMark('w2-mark', 'touched')).toBe(ticket)

    // …and falls through to the floor for a chat with no entry of its own.
    const list = claimChatRead()
    noteChatListRead('w2-mark', ['touched'], list)
    expect(chatReadMark('w2-mark', 'no-entry')).toBe(list)
  })

  // What a list seed snapshots before it asks. Counted per workspace so a busy workspace
  // cannot burn an idle one's bounded seed retries.
  it('counts applied single-chat reads per workspace, and only when applied', () => {
    const before = chatReadsApplied('w-count')
    const first = claimChatRead()
    const second = claimChatRead()

    acceptChatRead('w-count', 'c', second)
    expect(chatReadsApplied('w-count')).toBe(before + 1)

    acceptChatRead('w-count', 'c', first) // refused — nothing was written
    expect(chatReadsApplied('w-count')).toBe(before + 1)

    expect(chatReadsApplied('w-count-other')).toBe(0)
  })
})
