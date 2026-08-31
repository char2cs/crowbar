import { apiFetch, folderDTOFromWire, type ChatsFolderWireDTO } from '@/lib/api'
import type { FolderDTO } from '@/lib/types'

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

/** One folder mutation's answer: the row asked about, plus every sibling a
 *  dense renumber moved alongside it (folders and workspaces share one
 *  sibling space). Apply both — matches `agent-api.ts`'s `createChatFolder`/
 *  `updateChatFolder`, which read the same `{folder, shifted}` envelope off
 *  the same backend route family. */
interface FolderWriteResult {
  folder: FolderDTO
  shifted: FolderDTO[]
}

function toFolderWriteResult(
  raw: { folder: ChatsFolderWireDTO; shifted?: ChatsFolderWireDTO[] },
  projectId: string,
  repoId: string,
): FolderWriteResult {
  return {
    folder: folderDTOFromWire(raw.folder, projectId, repoId),
    shifted: (raw.shifted ?? []).map((row) => folderDTOFromWire(row, projectId, repoId)),
  }
}

/**
 * Create a folder, and answer with the created row plus its collateral.
 *
 * There is no dedicated push channel for folders any more (Task 34), so this
 * is not a seed for a later stream frame — it is the only confirmation the
 * caller gets. `row-actions.ts`'s `performCreateFolder` applies it to
 * `useSidebarStore` directly.
 */
export function createFolder(
  projectId: string,
  repoId: string,
  name: string,
  parentId: string,
): Promise<FolderWriteResult> {
  return apiFetch<{ folder: ChatsFolderWireDTO; shifted?: ChatsFolderWireDTO[] }>(
    `${repoBase(projectId, repoId)}/chats/folders`,
    {
      method: 'POST',
      headers: JSON_HEADERS,
      body: JSON.stringify({ name, parentId }),
    },
  ).then((raw) => toFolderWriteResult(raw, projectId, repoId))
}

export function placeFolder(
  projectId: string,
  repoId: string,
  folderId: string,
  placement: FolderPlacement,
): Promise<FolderWriteResult> {
  return apiFetch<{ folder: ChatsFolderWireDTO; shifted?: ChatsFolderWireDTO[] }>(
    `${repoBase(projectId, repoId)}/chats/folders/${folderId}`,
    {
      method: 'PATCH',
      headers: JSON_HEADERS,
      body: JSON.stringify(placement),
    },
  ).then((raw) => toFolderWriteResult(raw, projectId, repoId))
}

/** Delete a folder, and answer with the rows its children's promotion moved.
 *  Its children reparent to the folder's own parent — a folder holds no
 *  worktrees, so removing one is not removing what it held. */
export function deleteFolder(
  projectId: string,
  repoId: string,
  folderId: string,
  init?: RequestInit,
): Promise<FolderDTO[]> {
  return apiFetch<{ shifted?: ChatsFolderWireDTO[] } | null>(
    `${repoBase(projectId, repoId)}/chats/folders/${folderId}`,
    { method: 'DELETE', ...init },
  ).then((raw) => (raw?.shifted ?? []).map((row) => folderDTOFromWire(row, projectId, repoId)))
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
