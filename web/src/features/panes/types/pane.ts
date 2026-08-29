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
