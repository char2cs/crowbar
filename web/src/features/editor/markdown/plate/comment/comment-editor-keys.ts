/** What a keystroke means to the comment composer. */
export type CommentEditorKeyAction = 'submit' | 'cancel' | null

/** The parts of a keyboard event this decision reads. */
export interface CommentEditorKeyEvent {
  key: string
  metaKey: boolean
  ctrlKey: boolean
}

export interface CommentEditorKeyContext {
  /** True while the floating link editor has the keyboard. */
  isLinkEditorOpen: boolean
}

/** What a keystroke does, apart from delivering it (jsdom never delivers keydown to an editable,
 *  so this is the testable half): bare Enter starts a paragraph rather than posting a half-written
 *  comment, and Escape belongs to an open popup before the draft. */
export function commentEditorKeyAction(
  event: CommentEditorKeyEvent,
  { isLinkEditorOpen }: CommentEditorKeyContext,
): CommentEditorKeyAction {
  // Cmd/Ctrl+Enter posts. A bare Enter is a new paragraph — in a box that
  // renders its own formatting, Enter is the most-used key there is.
  if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) return 'submit'

  // Escape belongs to the link editor while it is open. Cancelling the whole
  // draft because someone backed out of a link dialog would throw away
  // everything they had written.
  if (event.key === 'Escape') return isLinkEditorOpen ? null : 'cancel'

  return null
}
