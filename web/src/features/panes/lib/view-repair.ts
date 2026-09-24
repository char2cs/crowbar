import { nanoid } from 'nanoid'
import { BOTTOM_PANE_ID, ROOT_PANE_ID } from '@/features/panes/constants/pane'
import type { LayoutNode, PaneGroup, ViewRecord } from '@/features/panes/types/pane'
import { createLeaf, getAllLeafIds } from '@/features/panes/utils/pane-layout'
import { makePane, settleFocus, type ViewState } from './view-state'

/** Drop the leaves `keep` refuses, collapsing emptied splits; null when none survive. */
function pruneLayout(node: LayoutNode, keep: (paneId: string) => boolean): LayoutNode | null {
  if (!node || typeof node !== 'object') return null
  if (node.type === 'pane') return typeof node.id === 'string' && keep(node.id) ? node : null
  if (node.type !== 'split' || !node.first || !node.second) return null
  const first = pruneLayout(node.first, keep)
  const second = pruneLayout(node.second, keep)
  if (!first) return second
  if (!second) return first
  return first === node.first && second === node.second ? node : { ...node, first, second }
}

/**
 * The largest part of `input` that satisfies `viewIntegrityViolations`: every
 * leaf, pane, record, order id and pointer that breaks an invariant is dropped;
 * everything else, including band order, is kept. Pure.
 */
export function repairViewState(input: Partial<ViewState>): ViewState {
  const source = input.panes ?? {}
  const panes: Record<string, PaneGroup> = {}
  const chatsSeen = new Set<string>()

  const claim = (paneId: string, viewId: string | null): boolean => {
    const pane = source[paneId]
    if (!pane || panes[paneId] || (pane.viewId ?? null) !== viewId) return false
    if (viewId === null && pane.chatId) return false
    if (pane.chatId) {
      if (chatsSeen.has(pane.chatId)) return false
      chatsSeen.add(pane.chatId)
    }
    panes[paneId] = { ...pane, id: paneId, viewId }
    return true
  }

  const inputViews = input.views ?? {}
  const orderIds = [...new Set([...(input.viewOrder ?? []), ...Object.keys(inputViews)])].filter(
    (id) => inputViews[id],
  )
  const views: Record<string, ViewRecord> = {}
  const viewOrder: string[] = []
  for (const id of orderIds) {
    const view = inputViews[id]
    const layout = pruneLayout(view.layout, (paneId) => claim(paneId, id))
    const leaves = layout ? getAllLeafIds(layout) : []
    if (!layout || !leaves.some((paneId) => panes[paneId].chatId)) {
      for (const paneId of leaves) {
        const chatId = panes[paneId].chatId
        if (chatId) chatsSeen.delete(chatId)
        delete panes[paneId]
      }
      continue
    }
    views[id] = { ...view, id, layout }
    viewOrder.push(id)
  }

  const freshId = (preferred: string) =>
    panes[preferred] || views[preferred] ? nanoid() : preferred
  let stage = input.stage ? pruneLayout(input.stage, (paneId) => claim(paneId, null)) : null
  if (!stage) {
    const id = freshId(ROOT_PANE_ID)
    panes[id] = makePane(id, null)
    stage = createLeaf(id)
  }
  let bottomLayout = input.bottomLayout
    ? pruneLayout(input.bottomLayout, (paneId) => claim(paneId, null))
    : null
  if (!bottomLayout) {
    const id = freshId(BOTTOM_PANE_ID)
    panes[id] = makePane(id, null)
    bottomLayout = createLeaf(id)
  }

  const activeViewId = input.activeViewId && views[input.activeViewId] ? input.activeViewId : null
  const activeViewByProject: Record<string, string> = {}
  for (const [projectId, viewId] of Object.entries(input.activeViewByProject ?? {})) {
    if (views[viewId]?.projectId === projectId) activeViewByProject[projectId] = viewId
  }
  const mostRecentActivePaneIds = [...new Set(input.mostRecentActivePaneIds ?? [])].filter(
    (id) => panes[id],
  )
  const repaired: ViewState = {
    panes,
    views,
    viewOrder,
    activeViewId,
    activeViewByProject,
    activeProjectId: input.activeProjectId ?? null,
    stage,
    bottomLayout,
    activePaneId: input.activePaneId ?? '',
    mostRecentActivePaneIds,
    fullscreenPaneId:
      input.fullscreenPaneId && panes[input.fullscreenPaneId] ? input.fullscreenPaneId : null,
  }
  settleFocus(repaired)
  return repaired
}
