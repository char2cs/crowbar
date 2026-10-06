import type { PaneGroup } from '@/features/panes/types/pane'

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
 *  - `hasViewChat` — it currently owns at least one chat held by some view
 *    record, showing or not, per {@link workspacesWithViewChat} below.
 *
 * NO TIME WINDOW. The old policy kept a workspace warm for a grace period
 * after it went inactive, on the theory the user might switch right back.
 * That period bridged a CLOCK; this rule needs none — whether a workspace
 * has a view-chat changes SYNCHRONOUSLY with view state (closing a view
 * removes its record in the same tick), so there is nothing left for a timer
 * to wait out. `workspace-host.tsx` reconciles whenever that state changes, not
 * on a schedule.
 *
 * NO COUNT LIMIT. Every workspace with a view stays mounted, however many:
 * a Recents row whose workspace was pushed out had to cold-mount on click.
 * Terminals render through xterm's DOM renderer, so there is no WebGL context
 * ceiling to protect.
 */

export interface RetentionEntry {
  wsId: string
  /** Whether this workspace currently owns at least one chat present in
   *  some Recents entry — see {@link workspacesWithViewChat}. The "in a
   *  view" test the new policy runs on. */
  hasViewChat: boolean
}

export interface RetentionPlan {
  /** Workspace ids to keep mounted, in the input's order. */
  retain: string[]
  /** Workspace ids to destroy now, in the input's order. */
  evict: string[]
}

export function planRetention(entries: RetentionEntry[], activeWsId: string | null): RetentionPlan {
  const retain: string[] = []
  const evict: string[] = []
  for (const entry of entries) {
    if (entry.wsId === activeWsId || entry.hasViewChat) retain.push(entry.wsId)
    else evict.push(entry.wsId)
  }
  return { retain, evict }
}

/**
 * Which workspaces own at least one chat held by a view record. Pure: the
 * caller builds `chatOwner` from the workspace stores that are live.
 */
/** @internal Exported for unit tests. */
export function workspacesWithViewChat(
  panes: readonly PaneGroup[],
  chatOwner: ReadonlyMap<string, string>,
): Set<string> {
  const owners = new Set<string>()
  for (const pane of panes) {
    if (!pane.viewId || !pane.chatId) continue
    const owner = chatOwner.get(pane.chatId)
    if (owner) owners.add(owner)
  }
  return owners
}
