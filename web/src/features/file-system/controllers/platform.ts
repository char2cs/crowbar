import { apiFetch } from '@/lib/api'
import { getActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'

// Backend file routes are workspace-scoped; resolve the active workspace and
// build its files base URL. Throws when no workspace is active so callers fail
// loudly rather than silently dropping a save.
function filesBase(): string {
  const wsId = getActiveWorkspaceId()
  if (!wsId) throw new Error('no active workspace for file operation')
  return filesBaseFor(wsId)
}

function filesBaseFor(wsId: string): string {
  return `/v0/workspaces/${encodeURIComponent(wsId)}/files`
}

export async function openFile(): Promise<string | null> {
  return null
}

export async function openDirectory(): Promise<string | null> {
  return null
}

export async function writeFile(path: string, content: string): Promise<void> {
  await apiFetch(`${filesBase()}/content`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path, content }),
  })
}

export async function readFile(path: string): Promise<string> {
  const payload = await apiFetch<{ content: string }>(
    `${filesBase()}/content?path=${encodeURIComponent(path)}`,
  )
  return payload.content
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
  const payload = await apiFetch<{ content: string }>(
    `${filesBaseFor(wsId)}/content?path=${encodeURIComponent(path)}`,
  )
  return payload.content
}

export async function readDirectory(
  _path: string,
): Promise<
  Array<{ name: string; path: string; isDirectory: boolean; is_dir: boolean; isFile: boolean }>
> {
  return []
}

export async function exists(_path: string): Promise<boolean> {
  return false
}

export function getHomePath(): string {
  return '/home'
}

export function getSeparator(): string {
  return '/'
}

export async function moveFile(src: string, dest: string): Promise<void> {
  await apiFetch(filesBase(), {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path: src, newPath: dest }),
  })
}

export async function copyFile(_src: string, _dest: string): Promise<void> {}

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
