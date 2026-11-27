import { apiFetch } from '@/lib/api/fetch'
import { folderDTOFromWire, partitionShifted, type ChatsFolderWireDTO } from '@/lib/api/wire'
import { projectBase, repoBase } from '@/lib/project-url'
import { workspaceBase } from '@/lib/workspace-scope-url'
import type { FolderDTO } from '@/lib/types'

const JSON_HEADERS = { 'Content-Type': 'application/json' }

function homeBase(projectId: string): string {
  return `${projectBase(projectId)}/home`
}

export interface WorkspacePlacement {
  folderId?: string
  order?: number
}

interface PlacedRow {
  id: string
  parentId: string
  order: number
}

export interface WorkspaceWriteResult {
  workspace: PlacedRow
  shifted: PlacedRow[]
}

function toPlacedRow(raw: { id: string; parentId?: string; order?: number }): PlacedRow {
  return { id: raw.id, parentId: raw.parentId ?? '', order: raw.order ?? 0 }
}

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

export interface FolderPlacement {
  name?: string
  parentId?: string
  order?: number
}

interface FolderWriteResult {
  folder: FolderDTO
  shifted: FolderDTO[]
  shiftedRows: PlacedRow[]
}

function toFolderWriteResult(
  raw: { folder: ChatsFolderWireDTO; shifted?: ChatsFolderWireDTO[] },
  projectId: string,
  repoId: string,
): FolderWriteResult {
  const { folders, others } = partitionShifted(raw.shifted ?? [], projectId, repoId)
  return {
    folder: folderDTOFromWire(raw.folder, projectId, repoId),
    shifted: folders,
    shiftedRows: others.map(toPlacedRow),
  }
}

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

export async function deleteFolder(
  projectId: string,
  repoId: string,
  folderId: string,
  init?: RequestInit,
): Promise<FolderDTO[]> {
  return apiFetch<{ shifted?: ChatsFolderWireDTO[] } | null>(
    `${repoBase(projectId, repoId)}/chats/folders/${folderId}`,
    { method: 'DELETE', ...init },
  ).then((raw) => partitionShifted(raw?.shifted ?? [], projectId, repoId).folders)
}

export async function deleteHomeFolder(
  projectId: string,
  folderId: string,
  init?: RequestInit,
): Promise<FolderDTO[]> {
  return apiFetch<{ shifted?: ChatsFolderWireDTO[] } | null>(
    `${homeBase(projectId)}/chats/folders/${folderId}`,
    { method: 'DELETE', ...init },
  ).then((raw) => partitionShifted(raw?.shifted ?? [], projectId, '').folders)
}

export interface RepoPlacement {
  order?: number
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
