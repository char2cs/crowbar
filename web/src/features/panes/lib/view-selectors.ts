import type { LayoutNode, PaneGroup, ViewRecord } from '@/features/panes/types/pane'
import { getAllLeafIds } from '@/features/panes/utils/pane-layout'

const chatIndexCache = new WeakMap<Record<string, PaneGroup>, Map<string, string>>()

/** chatId → the pane holding it, memoized on the `panes` map's identity. */
export function chatPaneIndex(panes: Record<string, PaneGroup>): Map<string, string> {
  const cached = chatIndexCache.get(panes)
  if (cached) return cached
  const index = new Map<string, string>()
  for (const pane of Object.values(panes)) {
    if (pane.chatId) index.set(pane.chatId, pane.id)
  }
  chatIndexCache.set(panes, index)
  return index
}

interface BandInput {
  views: Record<string, ViewRecord>
  viewOrder: string[]
}

const lastIds = new Map<string, string[]>()
const bandCache = new WeakMap<
  Record<string, ViewRecord>,
  Map<string, { order: string[]; ids: string[] }>
>()

/**
 * The band's rows for `projectId`: its record ids in `viewOrder`. Memoized on
 * `views` + `viewOrder` identity, and returns the previous array when the ids
 * did not change, so a subscriber re-renders only when rows come, go or move.
 */
export function selectProjectViewIds(state: BandInput, projectId: string): string[] {
  let perProject = bandCache.get(state.views)
  if (!perProject) {
    perProject = new Map()
    bandCache.set(state.views, perProject)
  }
  const hit = perProject.get(projectId)
  if (hit && hit.order === state.viewOrder) return hit.ids
  const ids = state.viewOrder.filter((id) => state.views[id]?.projectId === projectId)
  const previous = lastIds.get(projectId)
  const stable = previous && sameIds(previous, ids) ? previous : ids
  lastIds.set(projectId, stable)
  perProject.set(projectId, { order: state.viewOrder, ids: stable })
  return stable
}

function sameIds(a: readonly string[], b: readonly string[]): boolean {
  return a.length === b.length && a.every((id, i) => id === b[i])
}

/** Nothing in it at all — no chat, no editor tabs. */
export function isPaneEmpty(pane: Pick<PaneGroup, 'chatId' | 'editorTabIds'> | undefined): boolean {
  if (!pane) return false
  return pane.chatId === null && pane.editorTabIds.length === 0
}

/** The stage is on screen with nothing in it — the "nothing is open" screen. */
export function selectIsShowingEmptyStage(state: {
  activeViewId: string | null
  stage: LayoutNode
  panes: Record<string, PaneGroup>
}): boolean {
  if (state.activeViewId !== null) return false
  const leaves = getAllLeafIds(state.stage)
  return leaves.length === 1 && isPaneEmpty(state.panes[leaves[0]])
}

/** Whether some pane holds `chatId`, off the memoized index. Subscribe per row. */
export function selectChatHasView(
  state: { panes: Record<string, PaneGroup> },
  chatId: string,
): boolean {
  return chatPaneIndex(state.panes).has(chatId)
}
