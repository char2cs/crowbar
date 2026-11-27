export type CommentEditorKeyAction = 'submit' | 'cancel' | null

export interface CommentEditorKeyEvent {
  key: string
  metaKey: boolean
  ctrlKey: boolean
}

export interface CommentEditorKeyContext {
  isLinkEditorOpen: boolean
}

export function commentEditorKeyAction(
  event: CommentEditorKeyEvent,
  { isLinkEditorOpen }: CommentEditorKeyContext,
): CommentEditorKeyAction {
  if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) return 'submit'

  if (event.key === 'Escape') return isLinkEditorOpen ? null : 'cancel'

  return null
}
