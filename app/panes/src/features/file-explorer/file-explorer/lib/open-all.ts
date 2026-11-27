import { findFileInTree } from '@/features/file-system/controllers/file-tree-utils'
import type { DirectoryEntry } from '@/features/file-system/controllers/platform'
import type { FileEntry } from '@/features/file-system/types/app'

export const OPEN_ALL_CONFIRM_THRESHOLD = 100

const ALWAYS_HIDDEN_FILE_NAMES = new Set(['.ds_store'])

export const isAlwaysHiddenFileName = (name: string): boolean =>
  ALWAYS_HIDDEN_FILE_NAMES.has(name.toLowerCase())

export const isHiddenFileTreeName = (name: string): boolean =>
  name.startsWith('.') && name.length > 1

export const getPathBaseName = (path: string): string => {
  const trimmedPath = path.replace(/[\\/]+$/, '')
  if (!trimmedPath) return path
  const segments = trimmedPath.split(/[\\/]/)
  return segments[segments.length - 1] || path
}

export function collectLoadedFilesInDirectory(
  tree: FileEntry[],
  directoryPath: string,
  rootFolderPath: string | undefined,
): string[] {
  let entries: FileEntry[] | undefined
  if (!directoryPath || directoryPath === rootFolderPath) {
    entries = tree
  } else {
    const directory = findFileInTree(tree, directoryPath)
    if (!directory?.isDir) return []
    entries = directory.children
  }

  const collected: string[] = []
  const walk = (items?: FileEntry[]) => {
    for (const entry of items ?? []) {
      if (entry.isDir) walk(entry.children)
      else collected.push(entry.path)
    }
  }
  walk(entries)
  return collected
}

export async function collectLocalFilesInDirectory(
  directoryPath: string,
  readDirectory: (path: string) => Promise<Array<Pick<DirectoryEntry, 'path' | 'is_dir'>>>,
  isVisible: (path: string, name: string, isDir: boolean) => boolean,
): Promise<string[]> {
  const collected: string[] = []
  const stack: string[] = [directoryPath]

  while (stack.length > 0) {
    const currentPath = stack.pop()
    if (!currentPath) continue

    // react-doctor-disable-next-line async-await-in-loop -- depth-first walk: each read discovers the next directories to read.
    const entries = await readDirectory(currentPath)
    for (const entry of entries) {
      if (!entry.path) continue
      const isDir = entry.is_dir
      const name = getPathBaseName(entry.path)
      if (isAlwaysHiddenFileName(name) || !isVisible(entry.path, name, isDir)) continue
      if (isDir) stack.push(entry.path)
      else collected.push(entry.path)
    }
  }

  return collected
}
