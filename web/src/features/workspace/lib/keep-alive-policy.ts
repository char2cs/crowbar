import { deriveRecentsEntries } from '@/components/sidebar/lib/recents-entries'
import type { PaneGroup } from '@/features/panes/types/pane'
import type { RecentsEntry } from '@/features/panes/types/recents-entry'

/**
 * Pure retention policy for workspace keep-alive — "everything that is in a
 * view should be in memory, everything else shouldn't." There is no more
 * user-configurable keep-alive window (`workspaceKeepAliveMinutes` is gone).
 *
 * A workspace is retained iff:
 *  - it is the ACTIVE workspace (the one currently routed to / on screen),
 *    even if it owns no chat yet — a brand-new blank workspace has nothing
 *    in Recents to point at it, but destroying the thing the user is
 *    actively looking at is never correct; or
 *  - `hasViewChat` — it currently owns at least one chat present in some
 *    Recents entry, per {@link workspacesWithViewChat} below. That covers
 *    every state `deriveRecentsEntries` tracks (live, working, set, AND
 *    dormant) — a parked/remembered entry is still tracked by Recents, so
 *    per the user's own definition it still counts as "in a view", not just
 *    whatever is on screen this instant.
 *
 * NO TIME WINDOW. The old policy kept a workspace warm for a grace period
 * after it went inactive, on the theory the user might switch right back.
 * That period bridged a CLOCK; this rule needs none — whether a workspace
 * has a view-chat changes SYNCHRONOUSLY with view state (closing a view's
 * last chat strips it from every Recents entry in the same tick `closePane`
 * / `forgetDormantArrangement` run), so there is nothing left for a timer to
 * wait out. `workspace-host.tsx` reconciles whenever that state changes, not
 * on a schedule.
 *
 * `cap` still applies: Recents can track more workspaces at once than the
 * xterm WebGL context ceiling allows (many simultaneous multi-pane splits
 * across different projects/repos all counting as "in a view" together), so
 * retention still caps the retained set. `lastActiveAt` survives here ONLY
 * to break cap ties — a pure LRU signal among candidates — never to expire
 * an entry on its own; there is no expiry any more.
 */

/** Hard cap on simultaneously-retained workspaces (xterm WebGL contexts). */
export const RETENTION_CAP = 6

export interface RetentionEntry {
  wsId: string
  /** Whether this workspace currently owns at least one chat present in
   *  some Recents entry — see {@link workspacesWithViewChat}. The "in a
   *  view" test the new policy runs on. */
  hasViewChat: boolean
  /** Recency signal used only to break ties when candidates exceed `cap` —
   *  never for expiry (there is none any more). */
  lastActiveAt: number
}

export interface RetentionPlan {
  /** Workspace ids to keep mounted, in the input's order. */
  retain: string[]
  /** Workspace ids to destroy now, in the input's order. */
  evict: string[]
}

export function planRetention(
  entries: RetentionEntry[],
  activeWsId: string | null,
  cap: number = RETENTION_CAP,
): RetentionPlan {
  if (entries.length === 0) {
    return { retain: [], evict: [] }
  }

  // Index the input so we can retain deterministically (active first, then
  // recency, then input order for ties) while emitting output in the
  // original input order.
  const indexed = entries.map((entry, index) => ({ ...entry, index }))

  const candidates = indexed.filter((e) => e.wsId === activeWsId || e.hasViewChat)

  // Enforce the hard cap: the active workspace always wins a slot; beyond
  // that, most-recently-active first (input order breaks ties).
  const capped = [...candidates]
    .sort((a, b) => {
      if (a.wsId === activeWsId) return -1
      if (b.wsId === activeWsId) return 1
      return b.lastActiveAt - a.lastActiveAt || a.index - b.index
    })
    .slice(0, Math.max(1, cap))

  const retainIndexes = new Set(capped.map((e) => e.index))

  const retain: string[] = []
  const evict: string[] = []
  for (const entry of indexed) {
    if (retainIndexes.has(entry.index)) retain.push(entry.wsId)
    else evict.push(entry.wsId)
  }

  return { retain, evict }
}

/**
 * Which workspaces currently own at least one chat present in some Recents
 * entry — the "in a view" test {@link planRetention} runs on.
 *
 * Pure: derives entries the same way Recents itself always has (a live
 * pane, a working chat, or a persisted dormant/set arrangement —
 * {@link deriveRecentsEntries}) and maps each entry's chats back to their
 * owning workspace via `chatOwner`. The caller builds `chatOwner` from
 * whichever workspace stores are actually live (only a live store's
 * `agentChats.chats` can say who owns a chat) — this function never reads a
 * store itself, so it stays as deterministic and unit-testable as
 * `planRetention`.
 *
 * `order`/`activeViewId` (Recents' own display order and "which one is
 * showing" marker) are deliberately not inputs here — neither changes WHICH
 * chats end up in some entry, only how the band draws them, so retention has
 * no use for either.
 */
export function workspacesWithViewChat(
  panes: PaneGroup[],
  working: Record<string, boolean>,
  dormantArrangements: RecentsEntry[],
  chatOwner: ReadonlyMap<string, string>,
): Set<string> {
  const entries = deriveRecentsEntries(panes, working, dormantArrangements)
  const owners = new Set<string>()
  for (const entry of entries) {
    for (const chatId of entry.chatIds) {
      const owner = chatOwner.get(chatId)
      if (owner) owners.add(owner)
    }
  }
  return owners
}
