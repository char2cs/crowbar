import type { GitFile } from '@/features/git/types/git-types'

export interface GitFolderNode {
  name: string
  fullPath: string
  folders: Map<string, GitFolderNode>
  files: GitFile[]
}

export const createFolderNode = (name: string, fullPath: string): GitFolderNode => ({
  name,
  fullPath,
  folders: new Map<string, GitFolderNode>(),
  files: [],
})

export const normalizePathSegments = (path: string): string[] =>
  path
    .replace(/\\/g, '/')
    .split('/')
    .map((segment) => segment.trim())
    .filter((segment) => segment.length > 0)

export const buildGitFolderTree = (fileList: GitFile[]): GitFolderNode => {
  const root = createFolderNode('', '')

  for (const file of fileList) {
    const segments = normalizePathSegments(file.path)
    if (segments.length === 0) continue

    let currentNode = root
    let currentPath = ''
    const directorySegments = segments.slice(0, -1)
    for (const segment of directorySegments) {
      currentPath = currentPath ? `${currentPath}/${segment}` : segment
      if (!currentNode.folders.has(segment)) {
        currentNode.folders.set(segment, createFolderNode(segment, currentPath))
      }
      currentNode = currentNode.folders.get(segment)!
    }

    currentNode.files.push(file)
  }

  return root
}

export const sortFoldersByName = (folders: Iterable<GitFolderNode>) =>
  Array.from(folders).sort((a, b) => a.name.localeCompare(b.name))

export const sortFilesByPath = (fileList: GitFile[]) =>
  [...fileList].sort((a, b) => a.path.localeCompare(b.path))

export const collectNodeFiles = (node: GitFolderNode): GitFile[] => [
  ...node.files,
  ...Array.from(node.folders.values()).flatMap((child) => collectNodeFiles(child)),
]

/**
 * Follow a folder down through single-child, file-less levels (`a` > `b` > `c`)
 * so the chain renders as one row. `node` is the deepest folder: its path is
 * the chain's identity.
 */
export const compactFolder = (start: GitFolderNode): { label: string; node: GitFolderNode } => {
  const names = [start.name]
  let node = start
  while (node.files.length === 0 && node.folders.size === 1) {
    node = node.folders.values().next().value!
    names.push(node.name)
  }
  return { label: names.join('/'), node }
}

export interface FolderAggregate {
  total: number
  viewed: number
}

/**
 * Descendant file and viewed counts for every folder below `root`, keyed by
 * `fullPath`. One post-order walk asks `isViewed` once per file.
 */
export const aggregateFolders = (
  root: GitFolderNode,
  isViewed: (file: GitFile) => boolean,
): Map<string, FolderAggregate> => {
  const out = new Map<string, FolderAggregate>()
  const walk = (node: GitFolderNode): FolderAggregate => {
    const agg = { total: node.files.length, viewed: 0 }
    for (const file of node.files) if (isViewed(file)) agg.viewed++
    for (const child of node.folders.values()) {
      const sub = walk(child)
      agg.total += sub.total
      agg.viewed += sub.viewed
    }
    out.set(node.fullPath, agg)
    return agg
  }
  for (const child of root.folders.values()) walk(child)
  return out
}
