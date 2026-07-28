import type { PaneContent } from '@/features/panes/types/pane-content'

// The set of buffer fields whose change should re-render a tab. Kept out of the
// tab-bar-item component file so that file stays Fast-Refresh-safe.
export function sameRenderedBuffer(a: PaneContent, b: PaneContent): boolean {
  if (a === b) return true
  if (
    a.id !== b.id ||
    a.type !== b.type ||
    a.name !== b.name ||
    a.path !== b.path ||
    a.isPinned !== b.isPinned ||
    a.isPreview !== b.isPreview ||
    a.isUncloseable !== b.isUncloseable
  ) {
    return false
  }
  if (a.type === 'editor' && b.type === 'editor') {
    return a.isDirty === b.isDirty
  }
  if (a.type === 'commitDiff' && b.type === 'commitDiff') {
    return a.wsId === b.wsId && a.sha === b.sha
  }
  if (a.type === 'agentChat' && b.type === 'agentChat') {
    return a.wsId === b.wsId && a.chatId === b.chatId
  }
  return true
}
