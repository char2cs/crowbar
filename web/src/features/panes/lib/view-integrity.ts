import { getAllLeafIds } from '@/features/panes/utils/pane-layout'
import { showingLayout, type ViewState } from './view-state'

/** Every broken invariant (views-as-tabs spec §Invariants 1-4), as text. */
export function viewIntegrityViolations(state: ViewState): string[] {
  const out: string[] = []
  const placed = new Map<string, string>()
  const place = (paneId: string, where: string, viewId: string | null) => {
    const pane = state.panes[paneId]
    if (!pane) {
      out.push(`${where}: leaf ${paneId} names no pane`)
      return
    }
    const prior = placed.get(paneId)
    if (prior) out.push(`pane ${paneId} is in both ${prior} and ${where}`)
    placed.set(paneId, where)
    if (pane.viewId !== viewId) out.push(`pane ${paneId} in ${where} carries viewId ${pane.viewId}`)
    if (viewId === null && pane.chatId)
      out.push(`${where} pane ${paneId} holds chat ${pane.chatId}`)
  }

  for (const [key, view] of Object.entries(state.views)) {
    if (view.id !== key) out.push(`view ${key} records id ${view.id}`)
    const leaves = getAllLeafIds(view.layout)
    for (const id of leaves) place(id, `view ${key}`, key)
    if (!leaves.some((id) => state.panes[id]?.chatId)) out.push(`view ${key} holds no chat`)
  }
  for (const id of getAllLeafIds(state.stage)) place(id, 'stage', null)
  for (const id of getAllLeafIds(state.bottomLayout)) place(id, 'bottom', null)
  for (const id of Object.keys(state.panes)) {
    if (!placed.has(id)) out.push(`pane ${id} is in no layout`)
  }

  const chatPane = new Map<string, string>()
  for (const pane of Object.values(state.panes)) {
    if (!pane.chatId) continue
    const other = chatPane.get(pane.chatId)
    if (other) out.push(`chat ${pane.chatId} is in panes ${other} and ${pane.id}`)
    chatPane.set(pane.chatId, pane.id)
  }

  const order = new Set(state.viewOrder)
  if (order.size !== state.viewOrder.length) out.push('viewOrder has duplicates')
  for (const id of state.viewOrder)
    if (!state.views[id]) out.push(`viewOrder names missing view ${id}`)
  for (const id of Object.keys(state.views))
    if (!order.has(id)) out.push(`view ${id} missing from viewOrder`)

  if (state.activeViewId !== null && !state.views[state.activeViewId]) {
    out.push(`activeViewId ${state.activeViewId} names no view`)
  }
  const reachable = new Set([
    ...getAllLeafIds(showingLayout(state)),
    ...getAllLeafIds(state.bottomLayout),
  ])
  if (!reachable.has(state.activePaneId))
    out.push(`activePaneId ${state.activePaneId} is not on screen`)
  for (const [projectId, viewId] of Object.entries(state.activeViewByProject)) {
    if (state.views[viewId]?.projectId !== projectId) {
      out.push(`activeViewByProject[${projectId}] names ${viewId}`)
    }
  }
  if (state.fullscreenPaneId !== null && !state.panes[state.fullscreenPaneId]) {
    out.push(`fullscreenPaneId ${state.fullscreenPaneId} names no pane`)
  }
  return out
}

export function assertViewIntegrity(state: ViewState): void {
  const violations = viewIntegrityViolations(state)
  if (violations.length > 0) throw new Error(`view integrity: ${violations.join('; ')}`)
}
