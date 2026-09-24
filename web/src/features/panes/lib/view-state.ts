import { nanoid } from 'nanoid'
import { BOTTOM_PANE_ID, ROOT_PANE_ID } from '@/features/panes/constants/pane'
import type {
  LayoutNode,
  PaneGroup,
  SplitDirection,
  SplitPlacement,
  ViewMember,
  ViewRecord,
} from '@/features/panes/types/pane'
import { createLeaf, findLeaf, getAllLeafIds } from '@/features/panes/utils/pane-layout'

/** The pane slice's structural state; see `view-ops.ts` for the only writers. */
export interface ViewState {
  panes: Record<string, PaneGroup>
  views: Record<string, ViewRecord>
  viewOrder: string[]
  activeViewId: string | null
  activeViewByProject: Record<string, string>
  activeProjectId: string | null
  stage: LayoutNode
  bottomLayout: LayoutNode
  activePaneId: string
  mostRecentActivePaneIds: string[]
  fullscreenPaneId: string | null
}

export type PaneHome = { kind: 'view'; viewId: string } | { kind: 'stage' } | { kind: 'bottom' }

export type InsertAt =
  | { kind: 'split'; targetPaneId: string; direction: SplitDirection; placement: SplitPlacement }
  | { kind: 'view'; projectId: string; after?: string | null }

export function makePane(
  id: string,
  viewId: string | null,
  init: Partial<Omit<PaneGroup, 'id' | 'viewId' | 'type'>> = {},
): PaneGroup {
  return {
    id,
    type: 'group',
    chatId: null,
    runnerId: null,
    workspaceId: null,
    editorTabIds: [],
    activeEditorTabId: null,
    editorOpen: false,
    chatSelected: true,
    ...init,
    viewId,
  }
}

export function initialViewState(): Pick<
  ViewState,
  | 'panes'
  | 'views'
  | 'viewOrder'
  | 'activeViewId'
  | 'activeViewByProject'
  | 'stage'
  | 'bottomLayout'
  | 'activePaneId'
  | 'mostRecentActivePaneIds'
  | 'fullscreenPaneId'
> {
  return {
    panes: {
      [ROOT_PANE_ID]: makePane(ROOT_PANE_ID, null),
      [BOTTOM_PANE_ID]: makePane(BOTTOM_PANE_ID, null),
    },
    views: {},
    viewOrder: [],
    activeViewId: null,
    activeViewByProject: {},
    stage: createLeaf(ROOT_PANE_ID),
    bottomLayout: createLeaf(BOTTOM_PANE_ID),
    activePaneId: ROOT_PANE_ID,
    mostRecentActivePaneIds: [ROOT_PANE_ID],
    fullscreenPaneId: null,
  }
}

/** The tree on screen: the active record's layout, or the stage. */
export function showingLayout(
  state: Pick<ViewState, 'views' | 'activeViewId' | 'stage'>,
): LayoutNode {
  const view = state.activeViewId ? state.views[state.activeViewId] : undefined
  return view ? view.layout : state.stage
}

export function homeOf(
  state: Pick<ViewState, 'panes' | 'views' | 'stage' | 'bottomLayout'>,
  paneId: string,
): PaneHome | null {
  const pane = state.panes[paneId]
  if (!pane) return null
  if (pane.viewId) return state.views[pane.viewId] ? { kind: 'view', viewId: pane.viewId } : null
  if (findLeaf(state.bottomLayout, paneId)) return { kind: 'bottom' }
  if (findLeaf(state.stage, paneId)) return { kind: 'stage' }
  return null
}

export function layoutOf(
  state: Pick<ViewState, 'views' | 'stage' | 'bottomLayout'>,
  home: PaneHome,
): LayoutNode {
  if (home.kind === 'view') return state.views[home.viewId].layout
  return home.kind === 'stage' ? state.stage : state.bottomLayout
}

export function writeLayout(state: ViewState, home: PaneHome, layout: LayoutNode): void {
  if (home.kind === 'view') state.views[home.viewId].layout = layout
  else if (home.kind === 'stage') state.stage = layout
  else state.bottomLayout = layout
}

/** A view's chats, in layout order. */
export function viewChatIds(state: Pick<ViewState, 'panes' | 'views'>, viewId: string): string[] {
  const view = state.views[viewId]
  if (!view) return []
  const out: string[] = []
  for (const id of getAllLeafIds(view.layout)) {
    const chatId = state.panes[id]?.chatId
    if (chatId) out.push(chatId)
  }
  return out
}

/** A view's members — its chats with their workspaces — in layout order. */
export function viewMembers(
  state: Pick<ViewState, 'panes' | 'views'>,
  viewId: string,
): ViewMember[] {
  const view = state.views[viewId]
  if (!view) return []
  const out: ViewMember[] = []
  for (const id of getAllLeafIds(view.layout)) {
    const pane = state.panes[id]
    if (pane?.chatId) out.push({ chatId: pane.chatId, workspaceId: pane.workspaceId ?? null })
  }
  return out
}

/** The workspace recorded for `chatId` by the pane showing it, if any. */
export function chatWorkspaceIn(
  panes: Readonly<Record<string, PaneGroup>>,
  chatId: string,
): string | null {
  for (const pane of Object.values(panes)) {
    if (pane.chatId === chatId) return pane.workspaceId ?? null
  }
  return null
}

export function viewHasChat(state: ViewState, viewId: string): boolean {
  return viewChatIds(state, viewId).length > 0
}

export function touchPane(state: ViewState, paneId: string): void {
  state.mostRecentActivePaneIds = [
    paneId,
    ...state.mostRecentActivePaneIds.filter((id) => id !== paneId),
  ]
}

export function forgetPaneId(state: ViewState, paneId: string): void {
  state.mostRecentActivePaneIds = state.mostRecentActivePaneIds.filter((id) => id !== paneId)
  if (state.fullscreenPaneId === paneId) state.fullscreenPaneId = null
}

/** Put `viewId` (or the stage, for null) on screen and focus a pane in it. */
export function showView(state: ViewState, viewId: string | null, focusPaneId?: string): void {
  state.activeViewId = viewId && state.views[viewId] ? viewId : null
  const leaves = getAllLeafIds(showingLayout(state))
  const leafSet = new Set(leaves)
  const focus =
    focusPaneId && leafSet.has(focusPaneId)
      ? focusPaneId
      : (state.mostRecentActivePaneIds.find((id) => leafSet.has(id)) ?? leaves[0])
  state.activePaneId = focus
  touchPane(state, focus)
  const projectId = state.activeViewId ? state.views[state.activeViewId].projectId : ''
  if (state.activeViewId && projectId) state.activeViewByProject[projectId] = state.activeViewId
}

/** The view to fall back to in `projectId` (any project for null): the most
 *  recently focused one, else the first in band order. */
export function nextViewFor(state: ViewState, projectId: string | null): string | null {
  const eligible = (viewId: string | null | undefined): viewId is string =>
    !!viewId &&
    !!state.views[viewId] &&
    (projectId === null || state.views[viewId].projectId === projectId)
  for (const paneId of state.mostRecentActivePaneIds) {
    const viewId = state.panes[paneId]?.viewId
    if (eligible(viewId)) return viewId
  }
  return state.viewOrder.find(eligible) ?? null
}

/**
 * Focus is derived, not maintained: after a write, an `activePaneId` that no
 * longer names a pane on screen (or in the bottom tray) falls back to the most
 * recently focused one that does, else the first leaf showing. Applied once
 * per write by `commitViewWrite` and once per load by `repairViewState`.
 */
export function settleFocus(state: ViewState): void {
  const showing = getAllLeafIds(showingLayout(state))
  const bottom = getAllLeafIds(state.bottomLayout)
  if (showing.includes(state.activePaneId) || bottom.includes(state.activePaneId)) return
  const showingSet = new Set(showing)
  state.activePaneId = state.mostRecentActivePaneIds.find((id) => showingSet.has(id)) ?? showing[0]
}

export function freshPaneId(state: ViewState): string {
  return state.panes[ROOT_PANE_ID] || state.views[ROOT_PANE_ID] ? nanoid() : ROOT_PANE_ID
}

export function resetStage(state: ViewState): void {
  const id = freshPaneId(state)
  state.panes[id] = makePane(id, null)
  state.stage = createLeaf(id)
}

export function projectOf(state: ViewState, viewId: string | null | undefined): string {
  return (viewId && state.views[viewId]?.projectId) || ''
}

/** Law 2: whether the active project may show `viewId` here. */
export function mayShow(state: ViewState, viewId: string): boolean {
  const project = projectOf(state, viewId)
  return !project || !state.activeProjectId || project === state.activeProjectId
}

/** Focus `paneId`, bringing its view on screen — or, for another project's
 *  view, only remembering it so the route's switch lands there. */
export function focusPane(state: ViewState, paneId: string): void {
  const home = homeOf(state, paneId)
  if (!home) return
  if (home.kind === 'view') {
    if (!mayShow(state, home.viewId)) {
      state.activeViewByProject[projectOf(state, home.viewId)] = home.viewId
      touchPane(state, paneId)
      return
    }
    showView(state, home.viewId, paneId)
    return
  }
  if (home.kind === 'stage' && state.activeViewId !== null) {
    showView(state, null, paneId)
    return
  }
  state.activePaneId = paneId
  touchPane(state, paneId)
}

export function resetBottom(state: ViewState): void {
  state.panes[BOTTOM_PANE_ID] = makePane(BOTTOM_PANE_ID, null)
  state.bottomLayout = createLeaf(BOTTOM_PANE_ID)
}
