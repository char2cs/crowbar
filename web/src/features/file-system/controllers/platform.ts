import { apiFetch, isNotFoundError } from '@/lib/api'
import { filesBaseForWorkspace } from '@/lib/workspace-scope-url'
import {
  decodeFileContent,
  type FileContentPayload,
} from '@/features/file-system/utils/file-content-encoding'

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

// Every operation names its workspace. There is deliberately no variant that resolves
// the ACTIVE one: a save, a move or a read whose workspace is merely whichever is in
// view lands in the wrong worktree (or, on a paired install, the wrong host).

/**
 * Write a file to an explicit workspace. Buffers are window-level — a
 * save/autosave/LSP-edit call site holds a `buffer.workspaceId` that can disagree
 * with whichever workspace is merely ACTIVE right now.
 */
export async function writeWorkspaceFile(
  wsId: string,
  path: string,
  content: string,
): Promise<void> {
  await apiFetch(`${filesBaseForWorkspace(wsId)}/content`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path, content }),
  })
}

/**
 * Read a file from an explicit workspace. Async flows that outlive the user's
 * focus (session-restore reconciliation, external FS-event buffer sync,
 * reopen-closed-tab) must name theirs: for linked worktrees of one repo the same
 * relative path in a sibling checkout silently loads different content.
 */
export async function readWorkspaceFile(wsId: string, path: string): Promise<string> {
  const payload = await apiFetch<FileContentPayload>(
    `${filesBaseForWorkspace(wsId)}/content?path=${encodeURIComponent(path)}`,
  )
  return decodeFileContent(payload)
}

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
 * List ONE level of `path` (workspace-relative; '' is the workspace root) of `wsId` via the
 * daemon's lazy tree route — the same route the file explorer is populated from.
 * Callers that need a recursive walk drive it themselves, one level per call.
 *
 * Rejects on a daemon failure. Callers depend on that: "Open All Files" catches
 * and falls back to the already-loaded tree.
 */
export async function readWorkspaceDirectory(
  wsId: string,
  path: string,
): Promise<DirectoryEntry[]> {
  const query = path ? `?path=${encodeURIComponent(path)}` : ''
  const nodes = await apiFetch<FileTreeNode[]>(`${filesBaseForWorkspace(wsId)}/tree${query}`)
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
 * Whether `path` (workspace-relative) is present in workspace `wsId`, resolved by
 * listing its parent directory — the daemon exposes no stat/HEAD verb, and one
 * lazy directory listing is far cheaper than reading the file's bytes.
 *
 * A missing PARENT (404) is an honest `false`. Any other failure rejects: "the
 * daemon errored" must never be reported to the user as "the file is not there".
 */
export async function workspaceFileExists(wsId: string, path: string): Promise<boolean> {
  const normalized = path.replace(/^\/+/, '').replace(/\/+$/, '')
  // The workspace root is always present and has no parent to list.
  if (!normalized) return true

  const lastSlash = normalized.lastIndexOf('/')
  const parent = lastSlash === -1 ? '' : normalized.slice(0, lastSlash)

  try {
    const entries = await readWorkspaceDirectory(wsId, parent)
    return entries.some((entry) => entry.path === normalized)
  } catch (error) {
    if (isNotFoundError(error)) return false
    throw error
  }
}

export async function moveWorkspaceFile(wsId: string, src: string, dest: string): Promise<void> {
  await apiFetch(filesBaseForWorkspace(wsId), {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path: src, newPath: dest }),
  })
}
