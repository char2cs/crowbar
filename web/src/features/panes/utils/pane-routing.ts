import type { PaneGroup, LayoutNode } from "../types/pane";
import { getAllLeafIds } from "./pane-layout";

export function getPaneScopeForPaneId(
  rootLayout: LayoutNode,
  bottomLayout: LayoutNode,
  panes: Record<string, PaneGroup>,
  paneId: string,
): PaneGroup[] {
  const rootIds = getAllLeafIds(rootLayout);
  if (rootIds.includes(paneId)) {
    return rootIds.map(id => panes[id]).filter(Boolean) as PaneGroup[];
  }
  return getAllLeafIds(bottomLayout)
    .map(id => panes[id])
    .filter(Boolean) as PaneGroup[];
}

export interface WritablePaneRoutingInput {
  activePane: PaneGroup | null;
  bufferId?: string;
  mostRecentActivePaneIds: string[];
  paneScope: PaneGroup[];
}

export function resolveWritablePaneForBuffer({
  activePane,
  bufferId,
  mostRecentActivePaneIds,
  paneScope,
}: WritablePaneRoutingInput): PaneGroup | null {
  if (!activePane) return null;

  if ((bufferId && activePane.bufferIds.includes(bufferId)) || !activePane.locked) {
    return activePane;
  }

  const paneById = new Map(paneScope.map((pane) => [pane.id, pane] as const));
  return (
    mostRecentActivePaneIds
      .map((paneId) => paneById.get(paneId))
      .find((pane) => pane && pane.id !== activePane.id && !pane.locked) ??
    [...paneById.values()].find((pane) => pane.id !== activePane.id && !pane.locked) ??
    null
  );
}
