import { nanoid } from 'nanoid'
import type { PaneGroup } from '@/features/panes/types/pane'
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

/**
 * The only structural writers. Every write that adds, removes or moves a pane
 * goes through `insertPane` / `removePane` / `movePane` (and `fillPane` for a
 * chat landing in a chatless pane), so the invariants `view-integrity.ts`
 * checks are kept in exactly one place. None of them repairs focus: every
 * write is applied through `commitViewWrite`, which settles it once at the
 * end, so no writer can forget to.
 */

/** Apply one pane write; afterwards `activePaneId` names a pane on screen. */
export function commitViewWrite<S extends ViewState>(state: S, recipe: (state: S) => void): void {
  recipe(state)
  settleFocus(state)
}

/** Drop a record and every pane still in its layout; when it was the one on
 *  screen, show the next view of the same project and workspace. */
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

/** `shown`: the workspaces the dropped view showed. The route names that
 *  workspace and only a gesture moves it, so another workspace's view never
 *  comes forward in its place — the stage does. */
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

/** The stage holds a chat now: its layout becomes a new record. */
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

/**
 * Add `pane` to the window. `split` carves it out of the target's share and
 * makes it a member of the target's view (a chat landing in the stage promotes
 * the stage); `view` mints a new record for it after `after`, or at the end.
 * Returns the view id the pane joined, `null` for stage/bottom, or undefined
 * when refused. Focus is the caller's decision.
 */
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

/** Hand a leaving pane's editor tabs to the pane that takes its place. */
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

/**
 * Take `paneId` out of the window. Its editor tabs go to the pane that takes
 * its place; a view left without a chat is removed with it (invariant 2); an
 * emptied stage or bottom tray is reseeded with one empty pane.
 */
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
    if (!rest) dropRecord(state, home.viewId, new Set([pane.workspaceId ?? null]))
    else if (!viewHasChat(state, home.viewId)) removeView(state, home.viewId)
    return
  }
  if (!rest) {
    if (home.kind === 'stage') resetStage(state)
    else resetBottom(state)
  }
}

/**
 * Re-home an existing pane: as a split of a target (joining the target's
 * view), or as a new record of the same project placed after `after`. The
 * view it left is removed when that took its last chat.
 */
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
    if (!rest) dropRecord(state, from.viewId, new Set([pane.workspaceId ?? null]))
    else if (!viewHasChat(state, from.viewId)) removeView(state, from.viewId)
  } else if (!rest) {
    if (from.kind === 'stage') resetStage(state)
    else resetBottom(state)
  }
  if (pane.chatId && to.kind === 'stage') promoteStage(state, state.activeProjectId ?? '')
  return true
}

/**
 * A chat lands in a chatless pane — not a change of chat (invariant 5). A
 * stage pane promotes the stage into a record of `projectId`. Returns the
 * view the pane belongs to, or undefined when refused.
 */
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
