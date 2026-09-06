import type { PaneGroup } from '@/features/panes/types/pane'
import { viewIdOf } from '@/features/panes/lib/pane-views'
import type { RecentsEntry, RecentsEntryState } from '@/features/panes/types/recents-entry'

/** Live > working > dormant (spec §5.6). A dormant multi-chat entry is drawn
 *  as a 'set' rather than plain 'dormant' — recents-band.tsx's shell shape
 *  already keys off `chatIds.length`, this just gives the at-rest case its
 *  own state value (see RecentsBand's "a dormant (at-rest) set" case). */
function resolveState(
  chatIds: string[],
  liveChatIds: Set<string>,
  working: Record<string, boolean>,
): RecentsEntryState {
  if (chatIds.some((id) => liveChatIds.has(id))) return 'live'
  if (chatIds.some((id) => working[id])) return 'working'
  return chatIds.length >= 2 ? 'set' : 'dormant'
}

/**
 * Pure derivation of the Recents band's rows — spec §5.6: a chat appears at
 * most once, in the highest band that claims it (live, then working, then
 * dormant). Order is the user's: an entry's slot comes from where it was
 * FIRST seen (led by `dormantArrangements`, the persisted order), and it
 * keeps that slot as it changes kind — recomputing `state` never moves it.
 *
 * A row is a VIEW, never a pane. That is the whole reconciliation: grouping
 * used to be recorded twice — once as pane layout with no group concept at
 * all, once as `dormantArrangements` entries that a merge had to write by
 * hand — and the two could disagree about what was one view. `viewId` on the
 * panes is the single fact now, and `dormantArrangements` is left with only
 * what panes cannot answer: what used to be up and is not any more.
 *
 * `order` is spec §8.1's real, dragged order (`pane-slice.ts`'s
 * `recentsOrder`, written only by `reorderRecentsEntry`) — applied as a
 * final re-sort over whatever the population rules above produced: an id
 * named in `order` sorts by its position there; an id that has never been
 * dragged keeps its natural (append) position, after every ordered id. A
 * plain `Array.prototype.sort` is stable (guaranteed since ES2019), so the
 * unordered ids' own relative order survives untouched.
 */
export function deriveRecentsEntries(
  panes: PaneGroup[],
  working: Record<string, boolean>,
  dormantArrangements: RecentsEntry[],
  order: readonly string[] = [],
): RecentsEntry[] {
  const liveChatIds = new Set<string>()
  // Every chat sharing a view with this one, itself included — the live
  // group, keyed per member so a slot below can pull the rest of a view in
  // behind whichever member it names.
  const viewMates = new Map<string, string[]>()
  const chatsByView = new Map<string, string[]>()
  for (const pane of panes) {
    if (!pane.chatId) continue
    liveChatIds.add(pane.chatId)
    const viewId = viewIdOf(pane)
    const mates = chatsByView.get(viewId)
    if (mates) mates.push(pane.chatId)
    else chatsByView.set(viewId, [pane.chatId])
  }
  for (const mates of chatsByView.values()) {
    for (const chatId of mates) viewMates.set(chatId, mates)
  }

  const claimed = new Set<string>()
  const entries: RecentsEntry[] = []

  // The persisted slots lead — an arrangement that gains or loses a pane
  // still inherits the place it grew out of.
  for (const arrangement of dormantArrangements) {
    const chatIds = arrangement.chatIds.filter((id) => !claimed.has(id))
    if (chatIds.length === 0) continue // fully superseded by an earlier slot
    // A LIVE chat brings its whole view with it. Its own dormant record is
    // where the row is DRAWN (§5.6: "restoring a dormant one — the row stays
    // exactly where it sits", and `recentsOrder` is keyed by this id), but
    // what the row CONTAINS is a question only the panes answer. Without
    // this, merging into a chat that happened to have been closed once split
    // the view back across two rows — measured live: a merged pair drew as
    // the reopened chat at its old slot plus a second row for the chat that
    // joined it, which is exactly the "Recents shows panes, not views"
    // this whole model replaces.
    for (const id of [...chatIds]) {
      for (const mate of viewMates.get(id) ?? []) {
        if (!claimed.has(mate) && !chatIds.includes(mate)) chatIds.push(mate)
      }
    }
    for (const id of chatIds) claimed.add(id)
    entries.push({
      id: arrangement.id,
      chatIds,
      state: resolveState(chatIds, liveChatIds, working),
    })
  }

  // A live VIEW with no persisted slot is a brand-new row, appended in pane
  // order — ONE row per view, not per pane. Panes sharing a `viewId` are the
  // chats the user deliberately merged (the only gesture that does it is a
  // drop, see `openChatIntoPane`), and §8.2's "you asked for them side by
  // side" is a promise about this band as much as about the window: they
  // belong to one slot, together. A pane nobody merged with is a view of one
  // and lands here exactly as a single-chat row, which is why nothing here
  // has to notice a merged view dissolving back down.
  const liveViews = new Map<string, RecentsEntry>()
  for (const pane of panes) {
    if (!pane.chatId || claimed.has(pane.chatId)) continue
    claimed.add(pane.chatId)
    const viewId = viewIdOf(pane)
    const open = liveViews.get(viewId)
    if (open) {
      open.chatIds.push(pane.chatId)
      continue
    }
    const entry: RecentsEntry = { id: viewId, chatIds: [pane.chatId], state: 'live' }
    liveViews.set(viewId, entry)
    entries.push(entry)
  }

  // A working chat with no view and no persisted slot — same population rule.
  for (const chatId of Object.keys(working)) {
    if (!working[chatId] || claimed.has(chatId)) continue
    claimed.add(chatId)
    entries.push({ id: chatId, chatIds: [chatId], state: 'working' })
  }

  if (order.length === 0) return entries
  const rank = new Map(order.map((id, i) => [id, i]))
  return entries
    .map((entry, i) => ({ entry, i, rank: rank.get(entry.id) }))
    .sort((a, b) => {
      if (a.rank !== undefined && b.rank !== undefined) return a.rank - b.rank
      if (a.rank !== undefined) return -1
      if (b.rank !== undefined) return 1
      return a.i - b.i // both unordered: keep natural (append) order
    })
    .map(({ entry }) => entry)
}
