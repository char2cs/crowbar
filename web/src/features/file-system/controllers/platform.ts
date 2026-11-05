import { apiFetch } from '@/lib/api/fetch'
import { isNotFoundError } from '@/lib/errors/api-error'
import { filesBaseForWorkspace } from '@/lib/workspace-scope-url'

// The real file operations: each talks to the daemon and lets a failure reject. Never answer a
// filesystem question with a fabricated success ('' content, [] entries, false); callers take it
// as fact. Each names its workspace: the active one may not own the file (other worktree/host).

/** One directory level, as the file explorer and breadcrumb consume it. */
export interface DirectoryEntry {
  name: string
  path: string
  isDirectory: boolean
  /** snake_case twin of `isDirectory` — the field the explorer walk reads. */
  is_dir: boolean
  isFile: boolean
}

interface FileTreeNode {
  name: string
  path: string
  type: 'file' | 'directory'
}

/** List ONE level of `path` ('' is the workspace root) via the daemon's lazy tree route, the one
 *  the explorer loads from; recursive walks drive it a level per call. Rejects on a daemon
 *  failure, which "Open All Files" relies on to fall back to the loaded tree. */
export async function readDirectory(workspaceId: string, path: string): Promise<DirectoryEntry[]> {
  const query = path ? `?path=${encodeURIComponent(path)}` : ''
  const nodes = await apiFetch<FileTreeNode[]>(`${filesBaseForWorkspace(workspaceId)}/tree${query}`)
  return nodes.map((node) => {
    const isDirectory = node.type === 'directory'
    return {
      name: node.name,
      path: node.path,
      isDirectory,
      is_dir: isDirectory,
      isFile: !isDirectory,
    }
  })
}

/** Whether `path` exists, by listing its parent: the daemon has no stat verb, and a lazy listing
 *  is far cheaper than reading the bytes. A missing parent (404) is `false`; any other failure
 *  rejects, so a daemon error is never shown as "the file is not there". */
export async function exists(workspaceId: string, path: string): Promise<boolean> {
  const normalized = path.replace(/^\/+/, '').replace(/\/+$/, '')
  // The workspace root is always present and has no parent to list.
  if (!normalized) return true

  const lastSlash = normalized.lastIndexOf('/')
  const parent = lastSlash === -1 ? '' : normalized.slice(0, lastSlash)

  try {
    const entries = await readDirectory(workspaceId, parent)
    return entries.some((entry) => entry.path === normalized)
  } catch (error) {
    if (isNotFoundError(error)) return false
    throw error
  }
}

export async function moveFile(workspaceId: string, src: string, dest: string): Promise<void> {
  await apiFetch(filesBaseForWorkspace(workspaceId), {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path: src, newPath: dest }),
  })
}
