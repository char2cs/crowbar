import { apiFetch } from '@/lib/api'
import { filesBaseForWorkspace } from '@/lib/workspace-scope-url'
import type { AppFile } from '@/features/file-system/types/app'

export interface FileNodeDTO {
  name: string
  path: string
  type: 'file' | 'directory'
  children?: FileNodeDTO[]
  gitStatus?: string
}

// Map a backend file node onto the AppFile shape the explorer renders. A
// directory with no `children` key is "not yet loaded" — represented as
// `children: undefined` — which the lazy loader uses as its fetch sentinel; an
// empty loaded directory becomes `children: []`.
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

// The file-change WS is the `.../files/ws` leaf of the same base the REST calls
// above use — a chat-scoped `/v0/chats/:chatId/files/ws` for a worktree-backed
// workspace, the project's own home leaf for the home one. It is its own route
// rather than a dual-serve of `/tree`, so the path is a real suffix, not an
// upgrade on an existing GET.
export function filesWsEndpoint(wsId: string): string {
  return `${filesBaseForWorkspace(wsId)}/ws`
}

// File-tree mutations against the workspace files endpoint. Paths are
// workspace-relative (the same `path` the tree nodes carry). On success the tree
// refreshes automatically: the daemon emits a structural FileChangeEvent over the
// files-WS, which use-workspace-effects reconciles — so callers do NOT refetch.

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

/**
 * Overwrite an existing file's content (the node must already exist). Pass
 * `encoding: 'base64'` with a base64-encoded `content` to write raw bytes
 * byte-faithfully — the daemon decodes it, so binary uploads survive intact (a
 * plain UTF-8 string body corrupts any non-UTF-8 bytes). The default omits the
 * field and writes `content` as UTF-8 text.
 */
export async function writeFileContent(
  wsId: string,
  path: string,
  content: string,
  encoding?: 'base64',
): Promise<void> {
  await apiFetch(`${filesBaseForWorkspace(wsId)}/content`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(encoding ? { path, content, encoding } : { path, content }),
  })
}

/**
 * Copy `sourcePath` to `destPath` (both workspace-relative) via the daemon's
 * server-side copy verb: byte-faithful (binary files stay intact — a client-side
 * read/write composition would round-trip them through the base64 content read
 * and corrupt them) and recursive for directories. An existing destination is a
 * 409; the daemon's structural FileChangeEvent reconciles the tree.
 */
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
