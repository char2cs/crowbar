import type { FolderDTO } from '@/lib/types'

export interface ChatsFolderWireDTO {
  id: string
  type: string
  parentId: string
  title: string
  order: number
  createdAt?: string
}

export function partitionShifted(
  rows: ChatsFolderWireDTO[],
  projectId: string,
  repoId: string,
): { folders: FolderDTO[]; others: ChatsFolderWireDTO[] } {
  const folders: FolderDTO[] = []
  const others: ChatsFolderWireDTO[] = []
  for (const row of rows) {
    if (row.type === 'folder') folders.push(folderDTOFromWire(row, projectId, repoId))
    else others.push(row)
  }
  return { folders, others }
}

export function folderDTOFromWire(
  row: ChatsFolderWireDTO,
  projectId: string,
  repoId: string,
): FolderDTO {
  return {
    id: row.id,
    repoId,
    projectId,
    parentId: row.parentId,
    name: row.title,
    order: row.order,
    createdAt: row.createdAt,
  }
}
