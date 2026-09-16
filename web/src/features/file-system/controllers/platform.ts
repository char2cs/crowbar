import { apiFetch, isNotFoundError } from '@/lib/api'
import { getActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
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

// Resolve the active workspace and build its files base URL. Throws when no
// workspace is active so callers fail loudly rather than silently dropping a
// save.
function filesBase(): string {
  const wsId = getActiveWorkspaceId()
  if (!wsId) throw new Error('no active workspace for file operation')
  return filesBaseForWorkspace(wsId)
}

export async function writeFile(path: string, content: string): Promise<void> {
  await apiFetch(`${filesBase()}/content`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path, content }),
  })
}

/**
 * Write a file to an explicit workspace. Buffers are window-level now (Task
 * 26) — a save/autosave/LSP-edit call site holds a `buffer.workspaceId` that
 * can disagree with whichever workspace is merely ACTIVE right now (the user
 * can switch workspaces while a different one's tab stays open, dirty, in
 * the shared pane tree). `writeFile`'s implicit active-workspace resolution
 * would silently write into the WRONG worktree's file in that case — mirrors
 * `readWorkspaceFile`'s own reasoning; every real write path must use this,
 * never `writeFile`, once a buffer's owning workspace is known.
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

export async function readFile(path: string): Promise<string> {
  const payload = await apiFetch<FileContentPayload>(
    `${filesBase()}/content?path=${encodeURIComponent(path)}`,
  )
  return decodeFileContent(payload)
}

/**
 * Read a file from an explicit workspace. Async flows that outlive the user's
 * focus (session-restore reconciliation, external FS-event buffer sync,
 * reopen-closed-tab) must use this instead of `readFile`: resolving the
 * *active* workspace at call time reads the same relative path from whichever
 * workspace the user switched to meanwhile — for linked worktrees of one repo
 * that silently loads a sibling checkout's content into the buffer.
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
 * List ONE level of `path` (workspace-relative; '' is the workspace root) via the
 * daemon's lazy tree route — the same route the file explorer is populated from.
 * Callers that need a recursive walk drive it themselves, one level per call.
 *
 * Rejects on a daemon failure. Callers depend on that: "Open All Files" catches
 * and falls back to the already-loaded tree.
 */
export async function readDirectory(path: string): Promise<DirectoryEntry[]> {
  const query = path ? `?path=${encodeURIComponent(path)}` : ''
  const nodes = await apiFetch<FileTreeNode[]>(`${filesBase()}/tree${query}`)
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
export async function exists(path: string): Promise<boolean> {
  const normalized = path.replace(/^\/+/, '').replace(/\/+$/, '')
  // The workspace root is always present and has no parent to list.
  if (!normalized) return true

  const lastSlash = normalized.lastIndexOf('/')
  const parent = lastSlash === -1 ? '' : normalized.slice(0, lastSlash)

  try {
    const entries = await readDirectory(parent)
    return entries.some((entry) => entry.path === normalized)
  } catch (error) {
    if (isNotFoundError(error)) return false
    throw error
  }
}

export async function moveFile(src: string, dest: string): Promise<void> {
  await apiFetch(filesBase(), {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path: src, newPath: dest }),
  })
}

export async function deleteFile(path: string): Promise<void> {
  await apiFetch(filesBase(), {
    method: 'DELETE',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path }),
  })
}

export async function createDirectory(path: string): Promise<void> {
  await apiFetch(filesBase(), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path, type: 'directory' }),
  })
}
