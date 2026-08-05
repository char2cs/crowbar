import { apiFetch } from '@/lib/api'

/**
 * Where a row sits in the sidebar, for every level of it.
 *
 * These are the placement half of the entity endpoints — the calls a drop
 * fires. They are deliberately separate from the git-shaped mutations next
 * door in `workspace.ts`: filing a row into a folder or nudging it up a slot
 * moves nothing on disk and answers synchronously, where a fork re-parent
 * rebases and answers 202.
 *
 * `order` is an INSERT INDEX in the destination's sibling list, computed with
 * the moved row itself lifted out — the same arithmetic the daemon applies
 * before it re-densifies the level. A stale index is clamped to an end rather
 * than refused, so a drop never fails because the tree moved under it.
 */

const JSON_HEADERS = { 'Content-Type': 'application/json' }

function repoBase(projectId: string, repoId: string): string {
  return `/v0/projects/${projectId}/repos/${repoId}`
}

/** A workspace's SIDEBAR placement. `folderId` is never a fork parent. */
export interface WorkspacePlacement {
  /** Owning folder, or '' for the repo root. Omitted leaves it where it is. */
  folderId?: string
  order?: number
}

export function placeWorkspace(
  projectId: string,
  repoId: string,
  wsId: string,
  placement: WorkspacePlacement,
): Promise<void> {
  return apiFetch(`${repoBase(projectId, repoId)}/workspaces/${wsId}`, {
    method: 'PATCH',
    headers: JSON_HEADERS,
    body: JSON.stringify(placement),
  })
}

/** A folder's name and placement; every field is optional and only what is
 *  present is changed. */
export interface FolderPlacement {
  name?: string
  /** A workspace id, another folder id, or '' for the repo root. */
  parentId?: string
  order?: number
}

/**
 * Create a folder, and answer with its id.
 *
 * The FolderDTO itself arrives on the folders stream like every other entity —
 * the id comes back here only because filing rows INTO a new folder needs
 * something to address, and waiting for the stream first would make grouping a
 * two-round-trip gesture with a visible gap in the middle.
 */
export function createFolder(
  projectId: string,
  repoId: string,
  name: string,
  parentId: string,
): Promise<{ id: string }> {
  return apiFetch(`${repoBase(projectId, repoId)}/folders`, {
    method: 'POST',
    headers: JSON_HEADERS,
    body: JSON.stringify({ name, parentId }),
  })
}

export function placeFolder(
  projectId: string,
  repoId: string,
  folderId: string,
  placement: FolderPlacement,
): Promise<void> {
  return apiFetch(`${repoBase(projectId, repoId)}/folders/${folderId}`, {
    method: 'PATCH',
    headers: JSON_HEADERS,
    body: JSON.stringify(placement),
  })
}

/** Delete a folder. Its children reparent to the folder's own parent — a
 *  folder holds no worktrees, so removing one is not removing what it held. */
export function deleteFolder(projectId: string, repoId: string, folderId: string): Promise<void> {
  return apiFetch(`${repoBase(projectId, repoId)}/folders/${folderId}`, { method: 'DELETE' })
}

/** A repo's owning project and its index within that project's section. */
export interface RepoPlacement {
  projectId?: string
  order?: number
}

export function placeRepo(
  projectId: string,
  repoId: string,
  placement: RepoPlacement,
): Promise<void> {
  return apiFetch(`${repoBase(projectId, repoId)}`, {
    method: 'PATCH',
    headers: JSON_HEADERS,
    body: JSON.stringify(placement),
  })
}

/** A project's index in the sidebar. */
export function placeProject(projectId: string, order: number): Promise<void> {
  return apiFetch(`/v0/projects/${projectId}`, {
    method: 'PATCH',
    headers: JSON_HEADERS,
    body: JSON.stringify({ order }),
  })
}
