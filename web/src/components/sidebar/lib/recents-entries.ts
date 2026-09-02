import type { PaneGroup } from '@/features/panes/types/pane'
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
  for (const pane of panes) {
    if (pane.chatId) liveChatIds.add(pane.chatId)
  }

  const claimed = new Set<string>()
  const entries: RecentsEntry[] = []

  // The persisted slots lead — an arrangement that gains or loses a pane
  // still inherits the place it grew out of.
  for (const arrangement of dormantArrangements) {
    const chatIds = arrangement.chatIds.filter((id) => !claimed.has(id))
    if (chatIds.length === 0) continue // fully superseded by an earlier slot
    for (const id of chatIds) claimed.add(id)
    entries.push({
      id: arrangement.id,
      chatIds,
      state: resolveState(chatIds, liveChatIds, working),
    })
  }

  // A live view with no persisted slot is a brand-new row, appended in pane order.
  for (const pane of panes) {
    if (!pane.chatId || claimed.has(pane.chatId)) continue
    claimed.add(pane.chatId)
    entries.push({ id: pane.id, chatIds: [pane.chatId], state: 'live' })
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
