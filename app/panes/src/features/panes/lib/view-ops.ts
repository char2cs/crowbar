import { nanoid } from 'nanoid'
import type { PaneGroup } from '@/features/panes/types/pane'
import { hasUnsavedEdits, type PaneContent } from '@/features/panes/types/pane-content'
import { settlePinnedOrder } from './view-pins'
import { paneChatWorkspaceId } from './pane-chat-workspace-id'
import {
  closeLayout,
  createLeaf,
  getAllLeafIds,
  getFirstLeafId,
  normalizeLayout,
  splitLayout,
} from '@/features/panes/utils/pane-layout'
import {
  forgetPaneId,
  settleFocus,
  homeOf,
  layoutOf,
  nextViewFor,
  resetBottom,
  resetStage,
  showView,
  viewHasChat,
  viewMembers,
  writeLayout,
  type InsertAt,
  type ViewState,
} from './view-state'

export function commitViewWrite<S extends ViewState>(state: S, recipe: (state: S) => void): void {
  recipe(state)
  settleFocus(state)
}

export function removeView(state: ViewState, viewId: string): void {
  const view = state.views[viewId]
  if (!view) return
  const shown = new Set(viewMembers(state, viewId).map((m) => m.workspaceId))
  for (const id of getAllLeafIds(view.layout)) {
    delete state.panes[id]
    forgetPaneId(state, id)
  }
  dropRecord(state, viewId, shown)
}

function dropRecord(state: ViewState, viewId: string, shown: ReadonlySet<string | null>): void {
  const view = state.views[viewId]
  if (!view) return
  delete state.views[viewId]
  state.viewOrder = state.viewOrder.filter((id) => id !== viewId)
  for (const [projectId, id] of Object.entries(state.activeViewByProject)) {
    if (id === viewId) delete state.activeViewByProject[projectId]
  }
  if (state.activeViewId === viewId) {
    showView(state, nextViewFor(state, state.activeProjectId ?? view.projectId, shown))
  }
}

function promoteStage(state: ViewState, projectId: string): string {
  const first = getFirstLeafId(state.stage)
  const id = state.views[first] ? nanoid() : first
  const layout = state.stage
  state.views[id] = { id, projectId, layout }
  for (const leaf of getAllLeafIds(layout)) state.panes[leaf].viewId = id
  state.viewOrder = [...state.viewOrder, id]
  const wasShowing = state.activeViewId === null
  resetStage(state)
  if (wasShowing) {
    state.activeViewId = id
    if (projectId) state.activeViewByProject[projectId] = id
  }
  return id
}

export function insertPane(
  state: ViewState,
  pane: PaneGroup,
  at: InsertAt,
): string | null | undefined {
  if (state.panes[pane.id]) return undefined
  if (at.kind === 'view') {
    if (!pane.chatId) return undefined
    const id = nanoid()
    state.panes[pane.id] = { ...pane, viewId: id }
    state.views[id] = { id, projectId: at.projectId, layout: createLeaf(pane.id) }
    const afterIndex = at.after ? state.viewOrder.indexOf(at.after) : -1
    const order = [...state.viewOrder]
    if (afterIndex === -1) order.push(id)
    else order.splice(afterIndex + 1, 0, id)
    state.viewOrder = order
    settlePinnedOrder(state)
    return id
  }
  const home = homeOf(state, at.targetPaneId)
  if (!home) return undefined
  if (pane.chatId && home.kind === 'bottom') return undefined
  const result = splitLayout(
    layoutOf(state, home),
    at.targetPaneId,
    at.direction,
    at.placement,
    pane.id,
  )
  if (!result) return undefined
  writeLayout(state, home, result.layout)
  const viewId = home.kind === 'view' ? home.viewId : null
  state.panes[pane.id] = { ...pane, viewId }
  if (pane.chatId && home.kind === 'stage') return promoteStage(state, state.activeProjectId ?? '')
  return viewId
}

function transferTabs(state: ViewState, closing: PaneGroup, survivorId: string): void {
  if (closing.editorTabIds.length === 0) return
  const survivor = state.panes[survivorId]
  if (!survivor) return
  const existing = new Set(survivor.editorTabIds)
  for (const tabId of closing.editorTabIds) {
    if (existing.has(tabId)) continue
    survivor.editorTabIds.push(tabId)
    existing.add(tabId)
  }
  if (survivor.editorTabIds.length > 0) survivor.editorOpen = true
  if (
    state.activePaneId === closing.id &&
    closing.activeEditorTabId &&
    survivor.editorTabIds.includes(closing.activeEditorTabId)
  ) {
    survivor.activeEditorTabId = closing.activeEditorTabId
  }
}

export function keepUnsavedTabs(
  state: ViewState & { buffers: PaneContent[] },
  paneId: string,
): void {
  const pane = state.panes[paneId]
  if (!pane?.viewId || pane.editorTabIds.length === 0 || !Array.isArray(state.buffers)) return
  const view = state.views[pane.viewId]
  if (!view || getAllLeafIds(view.layout).length > 1) return
  const kept = pane.editorTabIds.filter((id) =>
    state.buffers.some((b) => b.id === id && hasUnsavedEdits(b)),
  )
  if (kept.length === 0) return
  const stage = state.panes[getFirstLeafId(state.stage)]
  stage.editorTabIds = [...new Set([...stage.editorTabIds, ...kept])]
  stage.activeEditorTabId ??= kept[0]
  stage.editorOpen = true
}

export function removePane(state: ViewState, paneId: string): void {
  const pane = state.panes[paneId]
  const home = homeOf(state, paneId)
  if (!pane) return
  if (!home) {
    delete state.panes[paneId]
    forgetPaneId(state, paneId)
    return
  }
  const rest = closeLayout(layoutOf(state, home), paneId)
  const wasActive = state.activePaneId === paneId
  if (rest) {
    const next = normalizeLayout(rest)
    writeLayout(state, home, next)
    const survivor = getFirstLeafId(next)
    transferTabs(state, pane, survivor)
    if (wasActive) state.activePaneId = survivor
  }
  delete state.panes[paneId]
  forgetPaneId(state, paneId)

  if (home.kind === 'view') {
    if (!rest) dropRecord(state, home.viewId, new Set([paneChatWorkspaceId(pane)]))
    else if (!viewHasChat(state, home.viewId)) removeView(state, home.viewId)
    return
  }
  if (!rest) {
    if (home.kind === 'stage') resetStage(state)
    else resetBottom(state)
  }
}

export function movePane(state: ViewState, paneId: string, at: InsertAt): boolean {
  const pane = state.panes[paneId]
  const from = homeOf(state, paneId)
  if (!pane || !from) return false

  if (at.kind === 'view') {
    if (!pane.chatId) return false
    const rest = closeLayout(layoutOf(state, from), paneId)
    if (!rest) return false
    writeLayout(state, from, normalizeLayout(rest))
    const id = nanoid()
    pane.viewId = id
    state.views[id] = { id, projectId: at.projectId, layout: createLeaf(paneId) }
    const order = state.viewOrder.filter((v) => v !== id)
    const afterIndex = at.after ? order.indexOf(at.after) : -1
    if (afterIndex === -1) order.push(id)
    else order.splice(afterIndex + 1, 0, id)
    state.viewOrder = order
    if (from.kind === 'view' && !viewHasChat(state, from.viewId)) removeView(state, from.viewId)
    return true
  }

  const { targetPaneId, direction, placement } = at
  if (paneId === targetPaneId) return false
  const to = homeOf(state, targetPaneId)
  if (!to) return false
  if (pane.chatId && to.kind === 'bottom') return false

  const sameTree =
    from.kind === to.kind &&
    (from.kind !== 'view' || (to.kind === 'view' && from.viewId === to.viewId))
  if (sameTree) {
    const rest = closeLayout(layoutOf(state, from), paneId)
    if (!rest) return false
    const result = splitLayout(normalizeLayout(rest), targetPaneId, direction, placement, paneId)
    if (!result) return false
    writeLayout(state, to, result.layout)
    return true
  }

  const result = splitLayout(layoutOf(state, to), targetPaneId, direction, placement, paneId)
  if (!result) return false
  writeLayout(state, to, result.layout)
  const rest = closeLayout(layoutOf(state, from), paneId)
  if (rest) writeLayout(state, from, normalizeLayout(rest))
  pane.viewId = to.kind === 'view' ? to.viewId : null

  if (from.kind === 'view') {
    if (!rest) dropRecord(state, from.viewId, new Set([paneChatWorkspaceId(pane)]))
    else if (!viewHasChat(state, from.viewId)) removeView(state, from.viewId)
  } else if (!rest) {
    if (from.kind === 'stage') resetStage(state)
    else resetBottom(state)
  }
  if (pane.chatId && to.kind === 'stage') promoteStage(state, state.activeProjectId ?? '')
  return true
}

export function fillPane(
  state: ViewState,
  paneId: string,
  chatId: string,
  runnerId: string | null,
  projectId: string,
  workspaceId: string | null = null,
): string | undefined {
  const pane = state.panes[paneId]
  const home = homeOf(state, paneId)
  if (!pane || !home || pane.chatId || home.kind === 'bottom') return undefined
  pane.chatId = chatId
  pane.runnerId = runnerId
  pane.workspaceId = workspaceId
  if (home.kind === 'stage') return promoteStage(state, projectId)
  return home.viewId
}
