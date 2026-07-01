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
