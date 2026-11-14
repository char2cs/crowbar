import { apiFetch } from '@/lib/api/fetch'
import { filesBaseForWorkspace } from '@/lib/workspace-scope-url'
import type { AppFile } from '@/features/file-system/types/app'

export interface FileNodeDTO {
  name: string
  path: string
  type: 'file' | 'directory'
  children?: FileNodeDTO[]
  gitStatus?: string
}

// Map a backend file node onto the explorer's AppFile. A directory without a `children` key is
// not yet loaded (`children: undefined`, the lazy loader's fetch sentinel); a loaded empty one
// is `children: []`.
export function toAppFile(node: FileNodeDTO): AppFile {
  const isDir = node.type === 'directory'
  return {
    name: node.name,
    path: node.path,
    isDir,
    isDirectory: isDir,
    isFile: !isDir,
    children: node.children ? node.children.map(toAppFile) : undefined,
    gitStatus: node.gitStatus,
  }
}

// fetchFileTree loads a single directory level of the workspace tree. An absent
// `path` returns the workspace root; the backend tree is lazy (one level per
// call), so the explorer fetches deeper levels on expand.
export async function fetchFileTree(wsId: string, path?: string): Promise<AppFile[]> {
  const query = path ? `?path=${encodeURIComponent(path)}` : ''
  const nodes = await apiFetch<FileNodeDTO[]>(`${filesBaseForWorkspace(wsId)}/tree${query}`)
  return nodes.map(toAppFile)
}

// Mutations on the workspace files endpoint; paths are workspace-relative, as on tree nodes.
// Callers do not refetch: the daemon's structural file_change on the workspace's files: scope
// is reconciled into the tree by use-workspace-file-tree.

/** Create a file (`type: 'file'`) or directory (`type: 'dir'`) at `path`. */
export async function createFileNode(
  wsId: string,
  path: string,
  type: 'file' | 'dir',
): Promise<void> {
  await apiFetch(filesBaseForWorkspace(wsId), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path, type }),
  })
}

/** Rename/move the node at `path` to `newPath` (both workspace-relative). */
export async function renameFileNode(wsId: string, path: string, newPath: string): Promise<void> {
  await apiFetch(filesBaseForWorkspace(wsId), {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path, newPath }),
  })
}

/** Delete the file or directory at `path`. */
export async function deleteFileNode(wsId: string, path: string): Promise<void> {
  await apiFetch(filesBaseForWorkspace(wsId), {
    method: 'DELETE',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path }),
  })
}

/** Copy `sourcePath` to `destPath` with the daemon's server-side copy: recursive, and
 *  byte-faithful where a client read/write would corrupt binaries through the base64 content
 *  read. An existing destination is a 409. */
export async function copyFileNode(
  wsId: string,
  sourcePath: string,
  destPath: string,
): Promise<void> {
  await apiFetch(`${filesBaseForWorkspace(wsId)}/copy`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ sourcePath, destPath }),
  })
}

export function findNode(tree: AppFile[], path: string): AppFile | null {
  for (const node of tree) {
    if (node.path === path) return node
    if (node.children) {
      const found = findNode(node.children, path)
      if (found) return found
    }
  }
  return null
}

// mergeChildren returns a new tree with the directory at `parentPath` carrying
// `children`. Untouched branches keep their identity so React can skip them.
export function mergeChildren(tree: AppFile[], parentPath: string, children: AppFile[]): AppFile[] {
  return tree.map((node) => {
    if (node.path === parentPath) return { ...node, children }
    if (node.children) {
      return { ...node, children: mergeChildren(node.children, parentPath, children) }
    }
    return node
  })
}
