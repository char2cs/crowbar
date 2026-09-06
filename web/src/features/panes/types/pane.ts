export interface PaneGroup {
  id: string
  type: 'group'
  /** The pane's one chat; null = the empty stage. */
  chatId: string | null
  /** The runner (vendor-CLI process) the chat is following, or null when dormant. */
  runnerId: string | null
  /** Everything the editor view holds: files, terminals, branch review — never chats or a "new tab" placeholder. */
  editorTabIds: string[]
  activeEditorTabId: string | null
  /** Split toggle state — chat-only vs. chat+editor. */
  editorOpen: boolean
  locked?: boolean
  /**
   * The VIEW this pane belongs to — a real, tagged grouping fact, never
   * inferred from where the pane happens to sit in the layout tree.
   *
   * A view is "as many chats as the user concentrated together". Two panes
   * carrying the SAME `viewId` were deliberately merged (the only gesture
   * that does it is a drag-and-drop, which splits inside the target's own
   * subtree — see `openChatIntoPane`); two panes that merely ended up
   * siblings because the window tiles that way carry DIFFERENT ones. That
   * distinction has no other expression: `rootLayout` is one shared tiling
   * tree for the whole window and cannot tell "these are one view" from
   * "these are two views side by side", which is why a plain click kept
   * reading as appending to whatever was already up.
   *
   * Read it through {@link viewIdOf}, never directly: a pane restored from a
   * layout written before views existed carries none, and an untagged pane
   * IS its own view. That fallback is also what makes a view dissolve for
   * free — a group of one is indistinguishable from ungrouped, so nothing
   * has to notice the last merge partner leaving.
   */
  viewId?: string
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
