import type { PaneContent } from '@/features/panes/types/pane-content'

/** The workspace path the explorer reveals for `activeBuffer`; none for a host document. */
export function getExplorerTargetPath(activeBuffer: PaneContent | null): string | undefined {
  if (activeBuffer?.type === 'markdownPreview' || activeBuffer?.type === 'htmlPreview') {
    return activeBuffer.source.authority === 'workspace' ? activeBuffer.source.path : undefined
  }
  if (activeBuffer?.type === 'editor' && activeBuffer.authority === 'workspace') {
    return activeBuffer.path
  }
  return undefined
}
