import type { PaneContent } from '@/features/panes/types/pane-content'

function getParentPath(path: string): string | null {
  const lastSlashIndex = Math.max(path.lastIndexOf('/'), path.lastIndexOf('\\'))
  if (lastSlashIndex <= 0) return null

  const parentPath = path.slice(0, lastSlashIndex)
  return parentPath && parentPath !== path ? parentPath : null
}

export function getAncestorDirectoryPaths(targetPath: string, stopAtPath?: string): string[] {
  const ancestors: string[] = []
  let currentPath = getParentPath(targetPath)

  while (currentPath) {
    ancestors.unshift(currentPath)
    if (stopAtPath && currentPath === stopAtPath) {
      break
    }
    currentPath = getParentPath(currentPath)
  }

  return ancestors
}

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
