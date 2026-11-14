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
