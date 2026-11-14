import type { PaneGroup } from '@/features/panes/types/pane'

// A workspace stays in memory iff it is active (even with no chat yet) or owns a chat held by a
// view record. No time window (view state changes synchronously, so there is nothing to wait out)
// and no count limit (xterm's DOM renderer has no WebGL context ceiling).

export interface RetentionEntry {
  wsId: string
  /** Owns at least one chat held by a view record ({@link workspacesWithViewChat}). */
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

/** @internal Workspaces owning a chat held by a view record; `chatOwner` comes from live stores. */
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
