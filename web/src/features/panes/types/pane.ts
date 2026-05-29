export interface PaneGroup {
  id: string;
  type: "group";
  bufferIds: string[];
  activeBufferId: string | null;
  mruBufferIds?: string[];
  previewBufferId?: string | null;
  pinnedBufferIds?: string[];
  locked?: boolean;
}

export interface PaneSplit {
  id: string;
  type: "split";
  direction: "horizontal" | "vertical";
  children: [PaneNode, PaneNode];
  sizes: [number, number];
}

export type PaneNode = PaneGroup | PaneSplit;

export type SplitDirection = "horizontal" | "vertical";
export type SplitPlacement = "before" | "after";

export interface PaneState {
  root: PaneNode;
  activePaneId: string;
  mostRecentActivePaneIds?: string[];
  fullscreenPaneId?: string | null;
}

export interface PanePosition {
  /** Left edge of this pane touches the absolute left of the content area. */
  atLeft: boolean;
  /** Top edge touches the absolute top of the content area (below tab bar). */
  atTop: boolean;
  /** Right edge touches the absolute right of the content area. */
  atRight: boolean;
  /** Bottom edge touches the absolute bottom (no visible pane below). */
  atBottom: boolean;
}

export const ROOT_PANE_POSITION: PanePosition = {
  atLeft: true,
  atTop: true,
  atRight: true,
  atBottom: true,
};
