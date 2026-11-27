import type { PaneGroup, ViewRecord } from '@/features/panes/types/pane'

const chatIndexCache = new WeakMap<Record<string, PaneGroup>, Map<string, string>>()

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

export function isPaneEmpty(pane: Pick<PaneGroup, 'chatId' | 'editorTabIds'> | undefined): boolean {
  if (!pane) return false
  return pane.chatId === null && pane.editorTabIds.length === 0
}

export function selectIsFocusedPaneEmpty(state: {
  activePaneId: string
  panes: Record<string, PaneGroup>
}): boolean {
  return isPaneEmpty(state.panes[state.activePaneId])
}
