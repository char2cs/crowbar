import type { PaneGroup, LayoutNode } from '../types/pane'
import { getAllLeafIds } from './pane-layout'

export function getPaneScopeForPaneId(
  rootLayout: LayoutNode,
  bottomLayout: LayoutNode,
  panes: Record<string, PaneGroup>,
  paneId: string,
): PaneGroup[] {
  const rootIds = getAllLeafIds(rootLayout)
  if (rootIds.includes(paneId)) {
    return rootIds.flatMap((id) => (panes[id] ? [panes[id]] : []))
  }
  return getAllLeafIds(bottomLayout).flatMap((id) => (panes[id] ? [panes[id]] : []))
}

export interface WritablePaneRoutingInput {
  activePane: PaneGroup | null
  bufferId?: string
  mostRecentActivePaneIds: string[]
  paneScope: PaneGroup[]
}

export function resolveWritablePaneForBuffer({
  activePane,
  bufferId,
  mostRecentActivePaneIds,
  paneScope,
}: WritablePaneRoutingInput): PaneGroup | null {
  if (!activePane) return null

  // I8 (Task 26 fix round 1): bufferIds has not existed on PaneGroup since
  // Task 1's editorTabIds rename.
  if ((bufferId && activePane.editorTabIds.includes(bufferId)) || !activePane.locked) {
    return activePane
  }

  const paneById = new Map(paneScope.map((pane) => [pane.id, pane] as const))
  return (
    mostRecentActivePaneIds
      .map((paneId) => paneById.get(paneId))
      .find((pane) => pane && pane.id !== activePane.id && !pane.locked) ??
    [...paneById.values()].find((pane) => pane.id !== activePane.id && !pane.locked) ??
    null
  )
}
