import type { PaneContent } from '@/features/panes/types/pane-content'
import type { FileEntry } from '@/features/file-system/types/app'

export function filterHiddenFiles(
  items: FileEntry[],
  isUserHidden: (path: string, isDir: boolean) => boolean,
  isGitIgnored: (path: string, isDir: boolean) => boolean,
): FileEntry[] {
  const result: FileEntry[] = []
  for (const item of items) {
    if (isUserHidden(item.path, item.isDir ?? false)) continue
    result.push({
      ...item,
      ignored: isGitIgnored(item.path, item.isDir ?? false),
      children: item.children
        ? filterHiddenFiles(item.children, isUserHidden, isGitIgnored)
        : undefined,
    })
  }
  return result
}

export function addNewItemToTree(
  items: FileEntry[],
  targetPath: string,
  newItem: FileEntry,
): FileEntry[] {
  return items.map((item) => {
    if (item.path === targetPath && item.isDir) {
      return {
        ...item,
        children: [...(item.children || []), newItem],
      }
    }
    if (item.children) {
      return {
        ...item,
        children: addNewItemToTree(item.children, targetPath, newItem),
      }
    }
    return item
  })
}

export function removeEditingItemsFromTree(items: FileEntry[]): FileEntry[] {
  const result: FileEntry[] = []
  for (const item of items) {
    if (item.isNewItem && item.isEditing) continue
    result.push({
      ...item,
      children: item.children ? removeEditingItemsFromTree(item.children) : undefined,
    })
  }
  return result
}

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

export function getExplorerTargetPath(activeBuffer: PaneContent | null): string | undefined {
  if (!activeBuffer) return undefined

  if (
    activeBuffer.type === 'markdownPreview' ||
    activeBuffer.type === 'htmlPreview' ||
    activeBuffer.type === 'csvPreview'
  ) {
    return activeBuffer.sourceFilePath
  }

  if (activeBuffer.type === 'editor' || activeBuffer.type === 'externalEditor') {
    return activeBuffer.path
  }

  return undefined
}
