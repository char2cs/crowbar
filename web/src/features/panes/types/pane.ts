export interface PaneGroup {
  id: string
  type: 'group'
  /** The pane's one chat; null = the empty stage. */
  chatId: string | null
  /** The runner (vendor-CLI process) the chat is following, or null when dormant. */
  runnerId: string | null
  /** `chatId`'s workspace, set once by the gesture that placed it (C3) and never re-derived.
   *  With `chatId` it is the view's member record: on-screen workspace lookups read it here.
   *  Null for a chatless pane. */
  workspaceId?: string | null
  /** For a draft thread: the row its chat will be placed under, '' being the panel root. */
  draftParentId?: string
  /** Everything the editor view holds: files, terminals, branch review — never chats or a "new tab" placeholder. */
  editorTabIds: string[]
  activeEditorTabId: string | null
  /** Split toggle state — chat-only vs. chat+editor. */
  editorOpen: boolean
  /** Collapsed ('tabs') presentation: chat selected over the editor. Apart from activeEditorTabId
   *  so the editor stays mounted (scroll, undo, PTY) while the chat shows. Read as `!== false`:
   *  missing means the chat. */
  chatSelected?: boolean
  locked?: boolean
  /** The view record this pane belongs to; null for stage and bottom-tray panes. */
  viewId: string | null
}

/** One chat of a view, with its identity fixed at open (C3). */
export interface ViewMember {
  chatId: string
  workspaceId: string | null
}

/** A Recents row: one view, its project, its tiling tree. Members are the chat panes of
 *  `layout` (`viewMembers`), held on the panes rather than a second list so they can't drift. */
export interface ViewRecord {
  id: string
  projectId: string
  layout: LayoutNode
  /** Kept at the band's front, compact, without a ×. Absent on layouts saved before pinning. */
  pinned?: boolean
}

export interface LayoutLeaf {
  type: 'pane'
  id: string
}

export interface LayoutSplit {
  type: 'split'
  id: string
  direction: 'horizontal' | 'vertical'
  sizes: [number, number]
  first: LayoutNode
  second: LayoutNode
}

export type LayoutNode = LayoutLeaf | LayoutSplit

export type SplitDirection = 'horizontal' | 'vertical'
export type SplitPlacement = 'before' | 'after'

export interface PanePosition {
  /** Left edge of this pane touches the absolute left of the content area. */
  atLeft: boolean
  /** Top edge touches the absolute top of the content area (below tab bar). */
  atTop: boolean
  /** Right edge touches the absolute right of the content area. */
  atRight: boolean
  /** Bottom edge touches the absolute bottom (no visible pane below). */
  atBottom: boolean
}

export const ROOT_PANE_POSITION: PanePosition = {
  atLeft: true,
  atTop: true,
  atRight: true,
  atBottom: true,
}
