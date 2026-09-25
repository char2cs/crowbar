export interface PaneGroup {
  id: string
  type: 'group'
  /** The pane's one chat; null = the empty stage. */
  chatId: string | null
  /** The runner (vendor-CLI process) the chat is following, or null when dormant. */
  runnerId: string | null
  /**
   * The workspace `chatId` belongs to — set once, by the gesture that put the
   * chat here (C3), and never re-derived: the opener always knows it (a
   * sidebar row carries it). Together with `chatId` this is the view's member
   * record; every "which workspace is this chat in" question about something
   * on screen reads it here instead of scanning workspace stores. Null for a
   * chatless pane.
   */
  workspaceId?: string | null
  /** Everything the editor view holds: files, terminals, branch review — never chats or a "new tab" placeholder. */
  editorTabIds: string[]
  activeEditorTabId: string | null
  /** Split toggle state — chat-only vs. chat+editor. */
  editorOpen: boolean
  /**
   * In the collapsed ('tabs') presentation only: is the CHAT the selected
   * surface, or a real editor tab? Deliberately separate from
   * `activeEditorTabId` — that field must keep naming the tab editor-view
   * content actually renders (Monaco/terminal/etc.) even while the chat is
   * selected, or switching to chat and back would unmount and remount
   * whatever editor surface was showing, losing its live state (scroll,
   * undo history, a terminal's PTY). Optional (not every constructed/
   * persisted PaneGroup sets it, including a layout saved before this field
   * existed) — always read as `!== false` so a missing value defaults to
   * showing the chat, same as a fresh pane.
   */
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

/**
 * A Recents row: one view, its project, and its tiling tree. Its members are
 * the chat panes of `layout` (`viewMembers`), each carrying `{chatId,
 * workspaceId}` — held on the pane rather than in a second list beside the
 * layout, so membership has exactly one writer and cannot drift from it.
 */
export interface ViewRecord {
  id: string
  projectId: string
  layout: LayoutNode
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
