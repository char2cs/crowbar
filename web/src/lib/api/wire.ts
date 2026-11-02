import type { FolderDTO } from '@/lib/types'

/**
 * The wire shape of one folder row a placement write answers with: a folder-typed
 * domain.Chat rendered through dto.AgentChatDTO (Task 34 — the dedicated
 * `/folders` resource was deleted; folders are Chat rows now). Neither repoId
 * nor projectId travel on it — the URL is the only place either is known —
 * and its display text is `title`, not `name`.
 */
export interface ChatsFolderWireDTO {
  id: string
  /** The row's kind: a folder, or — among a write's `shifted` siblings — a
   *  locked branch reported by its workspace id. Absent means folder. */
  type?: string
  parentId: string
  title: string
  order: number
  /** ISO creation time — the daemon's `order` tiebreak (tree.compareNodes). */
  createdAt?: string
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
