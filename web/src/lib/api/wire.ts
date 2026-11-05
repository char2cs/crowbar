import type { FolderDTO } from '@/lib/types'

// A row a folder route answers with: the daemon renders folders as folder-typed chat rows
// (dto.AgentChatDTO), unlike the projects-scope folder entity (FolderDTO). It carries no
// repoId/projectId (only the URL knows them), and its display text is `title`, not `name`.
export interface ChatsFolderWireDTO {
  id: string
  /** The row's kind: 'folder', or, among a write's `shifted` siblings, any other row the
   *  renumber moved (a chat, or a locked branch reported by its workspace id). */
  type: string
  parentId: string
  title: string
  order: number
  /** ISO creation time — the daemon's `order` tiebreak (tree.compareNodes). */
  createdAt?: string
}

/** A write's `shifted` rows split into the folders and every other row kind, which must never
 *  be applied as folder entities. */
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

/** `ChatsFolderWireDTO` -> the sidebar's own `FolderDTO`, filling in the
 *  repo/project scope the wire row doesn't carry. `sidebar-placement.ts`'s
 *  write verbs read these rows back off their own mutation responses. */
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
