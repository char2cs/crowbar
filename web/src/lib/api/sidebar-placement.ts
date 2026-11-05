import { apiFetch } from '@/lib/api/fetch'
import { folderDTOFromWire, type ChatsFolderWireDTO } from '@/lib/api/wire'
import { projectBase, repoBase } from '@/lib/project-url'
import { workspaceBase } from '@/lib/workspace-scope-url'
import type { FolderDTO } from '@/lib/types'

// The placement calls a drop fires: separate from workspace.ts's git mutations, since filing or
// reordering moves nothing on disk and answers synchronously. `order` is an insert index with the
// moved row lifted out, as the daemon computes it; a stale index clamps to an end, never refuses.

const JSON_HEADERS = { 'Content-Type': 'application/json' }

function homeBase(projectId: string): string {
  return `${projectBase(projectId)}/home`
}

/** A workspace's SIDEBAR placement. `folderId` is never a fork parent. */
export interface WorkspacePlacement {
  /** Owning folder, or '' for the repo root. Omitted leaves it where it is. */
  folderId?: string
  order?: number
}

/** One row's decided placement, as a placement PATCH reports it. */
export interface PlacedRow {
  id: string
  /** A chat id, a folder id, or '' for the root. */
  parentId: string
  order: number
}

/** {@link placeWorkspace}'s answer: the moved row (its `id` is the Node the
 *  daemon addressed — a locked branch's own workspace, or an ordinary fork's
 *  owning chat) plus every sibling the dense renumber shifted. Apply both. */
export interface WorkspaceWriteResult {
  workspace: PlacedRow
  shifted: PlacedRow[]
}

function toPlacedRow(raw: { id: string; parentId?: string; order?: number }): PlacedRow {
  return { id: raw.id, parentId: raw.parentId ?? '', order: raw.order ?? 0 }
}

// File a locked branch's row via `PATCH .../workspaces/:wsId/placement` — the workspace, not its
// owning chat: position is a fact about the branch. `folderId` goes as `parentId`, named so it is
// never mistaken for a fork parent. No frame carries placement, so the answer is the confirmation.
export function placeWorkspace(
  wsId: string,
  placement: WorkspacePlacement,
): Promise<WorkspaceWriteResult> {
  return apiFetch<{
    workspace: { id: string; parentId?: string; order?: number }
    shifted?: { id: string; parentId?: string; order?: number }[]
  }>(`${workspaceBase(wsId)}/placement`, {
    method: 'PATCH',
    headers: JSON_HEADERS,
    body: JSON.stringify({
      ...(placement.folderId !== undefined && { parentId: placement.folderId }),
      ...(placement.order !== undefined && { order: placement.order }),
    }),
  }).then((raw) => ({
    workspace: toPlacedRow(raw.workspace),
    shifted: (raw.shifted ?? []).map(toPlacedRow),
  }))
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
 *  sibling space). Apply all of it. */
interface FolderWriteResult {
  folder: FolderDTO
  /** The shifted FOLDER siblings. */
  shifted: FolderDTO[]
  /** The shifted siblings of any other kind — a locked branch by its
   *  workspace id — as bare placements. */
  shiftedRows: PlacedRow[]
}

function toFolderWriteResult(
  raw: { folder: ChatsFolderWireDTO; shifted?: ChatsFolderWireDTO[] },
  projectId: string,
  repoId: string,
): FolderWriteResult {
  const shifted: FolderDTO[] = []
  const shiftedRows: PlacedRow[] = []
  for (const row of raw.shifted ?? []) {
    if (row.type === undefined || row.type === 'folder') {
      shifted.push(folderDTOFromWire(row, projectId, repoId))
    } else {
      shiftedRows.push(toPlacedRow(row))
    }
  }
  return { folder: folderDTOFromWire(raw.folder, projectId, repoId), shifted, shiftedRows }
}

// Create a folder; answers the row plus its collateral, which performCreateFolder applies at once
// so the row shows before its folder entity frame lands.
export async function createFolder(
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

/** {@link createFolder}, for the project-home workspace instead of a repo. */
export async function createHomeFolder(
  projectId: string,
  name: string,
  parentId: string,
): Promise<FolderWriteResult> {
  return apiFetch<{ folder: ChatsFolderWireDTO; shifted?: ChatsFolderWireDTO[] }>(
    `${homeBase(projectId)}/chats/folders`,
    {
      method: 'POST',
      headers: JSON_HEADERS,
      body: JSON.stringify({ name, parentId }),
    },
  ).then((raw) => toFolderWriteResult(raw, projectId, ''))
}

export async function placeFolder(
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

/** {@link placeFolder}, for the project-home workspace instead of a repo. */
export async function placeHomeFolder(
  projectId: string,
  folderId: string,
  placement: FolderPlacement,
): Promise<FolderWriteResult> {
  return apiFetch<{ folder: ChatsFolderWireDTO; shifted?: ChatsFolderWireDTO[] }>(
    `${homeBase(projectId)}/chats/folders/${folderId}`,
    {
      method: 'PATCH',
      headers: JSON_HEADERS,
      body: JSON.stringify(placement),
    },
  ).then((raw) => toFolderWriteResult(raw, projectId, ''))
}

/** Delete a folder, and answer with the rows its children's promotion moved.
 *  Its children reparent to the folder's own parent — a folder holds no
 *  worktrees, so removing one is not removing what it held. */
export async function deleteFolder(
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

/** {@link deleteFolder}, for the project-home workspace instead of a repo. */
export async function deleteHomeFolder(
  projectId: string,
  folderId: string,
  init?: RequestInit,
): Promise<FolderDTO[]> {
  return apiFetch<{ shifted?: ChatsFolderWireDTO[] } | null>(
    `${homeBase(projectId)}/chats/folders/${folderId}`,
    { method: 'DELETE', ...init },
  ).then((raw) => (raw?.shifted ?? []).map((row) => folderDTOFromWire(row, projectId, '')))
}

/** A repo's index within its project's section, and the project-home folder
 *  its own entry is filed under. A repo's project is fixed at import. */
export interface RepoPlacement {
  order?: number
  /** A project-home folder id, or '' for the project's home root. Omitted
   *  leaves the repo in whichever folder it already sits in. */
  folderId?: string
}

export async function placeRepo(
  projectId: string,
  repoId: string,
  placement: RepoPlacement,
): Promise<void> {
  return apiFetch(repoBase(projectId, repoId), {
    method: 'PATCH',
    headers: JSON_HEADERS,
    body: JSON.stringify(placement),
  })
}

/** A project's index in the sidebar. */
export async function placeProject(projectId: string, order: number): Promise<void> {
  return apiFetch(projectBase(projectId), {
    method: 'PATCH',
    headers: JSON_HEADERS,
    body: JSON.stringify({ order }),
  })
}
