import { apiFetch, folderDTOFromWire, type ChatsFolderWireDTO } from '@/lib/api'
import { workspaceBase } from '@/lib/workspace-scope-url'
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

function homeBase(projectId: string): string {
  return `/v0/projects/${projectId}/home`
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

/**
 * File a LOCKED branch's own row into a folder, at an index.
 *
 * Addressed to the WORKSPACE itself — `PATCH .../workspaces/:wsId/placement`
 * (2026-09-09 sidebar-placement-unification, workspace-placement fix) — never
 * to the chat that owns its worktree. That chat still exists and still owns
 * every OTHER worktree verb (lock, sync, merge, reparent, ...), but a locked
 * branch's own sidebar position is a fact about the branch, not about any
 * conversation living inside it, and locked workspaces are not chats: routing
 * this through the chat-addressed placement route was tried and rejected
 * during this fix's own design (it would have meant "position" was the one
 * thing this whole migration extracted from Chat that stayed reachable only
 * through one).
 *
 * `folderId` is sent as the route's `parentId` — one field, because a
 * folder and a locked branch's own row hang off the same sibling space
 * within the branch's own repo. It stays named `folderId` on this side to
 * keep the caller's guarantee that a folder can never be mistaken for a fork
 * parent, which is a different edge written by `reparentWorkspace` next door.
 *
 * Answers the decided placement: no workspace frame carries one, so this is
 * the only confirmation the drop gets (`drop-actions.ts` applies it).
 */
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
 *  sibling space). Apply all of it — matches `agent-api.ts`'s
 *  `createChatFolder`/`updateChatFolder`, which read the same
 *  `{folder, shifted}` envelope off the same backend route family. */
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

/** {@link createFolder}, for the project-home workspace instead of a repo. */
export function createHomeFolder(
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

/** {@link placeFolder}, for the project-home workspace instead of a repo. */
export function placeHomeFolder(
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

/** {@link deleteFolder}, for the project-home workspace instead of a repo. */
export function deleteHomeFolder(
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
