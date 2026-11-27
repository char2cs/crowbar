import type { PaneGroup } from '@/features/panes/types/pane'

export interface RetentionEntry {
  wsId: string
  hasViewChat: boolean
}

export interface RetentionPlan {
  retain: string[]
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

/** @internal */
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
