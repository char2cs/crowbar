import { apiFetch, isNotFoundError } from '@/lib/api'
import { filesBaseForWorkspace } from '@/lib/workspace-scope-url'

// This module holds the REAL file operations. Everything here talks to the
// daemon and lets a failure reject.
//
// The rule the rest of this file exists to keep: NEVER answer a filesystem
// question with a fabricated success value. A previous `readFileContent` stub
// returned '' unconditionally, so callers got a plausible empty file instead of
// an error and every failure was silent — jump navigation reopened closed files
// as blank buffers for as long as it lived. `readDirectory` returning [] and
// `exists` returning false were the same bug wearing different types: "Open All
// Files" opened nothing (the [] skipped both the caller's catch-and-fall-back
// and its length guard) and every relative link in the Markdown preview was
// dead. Do not reintroduce a reader that fabricates a success value — if the
// operation cannot be performed, throw.
//
// Every operation names its workspace: the active one may not own the file.

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

/**
 * List ONE level of `path` (workspace-relative; '' is the workspace root) via the
 * daemon's lazy tree route — the same route the file explorer is populated from.
 * Callers that need a recursive walk drive it themselves, one level per call.
 *
 * Rejects on a daemon failure. Callers depend on that: "Open All Files" catches
 * and falls back to the already-loaded tree.
 */
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

/**
 * Whether `path` (workspace-relative) is present in the workspace, resolved by
 * listing its parent directory — the daemon exposes no stat/HEAD verb, and one
 * lazy directory listing is far cheaper than reading the file's bytes.
 *
 * A missing PARENT (404) is an honest `false`. Any other failure rejects: "the
 * daemon errored" must never be reported to the user as "the file is not there".
 */
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
