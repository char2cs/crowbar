import { entityStore, patchEntities, placeEntities } from '@/core/entities/entity-store'
import type { FolderDTO } from '@/lib/types'

// A placement write answers with the daemon's committed rows, which its frames
// carry a moment later; applying the answer at once keeps the dragged row where
// it was dropped meanwhile. A frame replaces each row, since none moves a version.

/** A row's placement as a write's answer reports it. */
export interface RowPlacement {
  id: string
  /** A chat id, a folder id, or ''/absent for the root. */
  parentId?: string
  order: number
}

/** Applies placement answers. Rows of one level share an id space: a chat, a folder or a locked branch's workspace. */
function applyRows(rows: readonly RowPlacement[]): void {
  const { chat, folder, workspace } = entityStore.getState().byKind
  const chats: Record<string, { parentId: string; order: number }> = {}
  const folders: Record<string, { parentId: string; order: number }> = {}
  const workspaces: Record<string, { folderId: string; order: number }> = {}
  for (const row of rows) {
    const parentId = row.parentId ?? ''
    if (row.id in chat) chats[row.id] = { parentId, order: row.order }
    else if (row.id in folder) folders[row.id] = { parentId, order: row.order }
    else if (row.id in workspace) workspaces[row.id] = { folderId: parentId, order: row.order }
  }
  patchEntities('chat', chats)
  patchEntities('folder', folders)
  patchEntities('workspace', workspaces)
}

/** A chat placement: the moved chat and every sibling the renumber shifted. Resolves to
 *  the repo the chat lives in, or null for a project home's. */
export async function applyChatPlacement(
  chat: RowPlacement,
  shifted: readonly RowPlacement[] = [],
): Promise<string | null> {
  applyRows([chat, ...shifted])
  return entityStore.getState().byKind.chat[chat.id]?.data.repoId || null
}

/** A workspace's own placement answer and the siblings it shifted. */
export async function applyWorkspacePlacement(
  wsId: string,
  placement: { parentId: string; order: number },
  shifted: readonly RowPlacement[] = [],
): Promise<void> {
  patchEntities('workspace', { [wsId]: { folderId: placement.parentId, order: placement.order } })
  applyRows(shifted)
}

/** A folder write's answer: the folder rows it wrote, and the siblings of another kind it
 *  renumbered. A folder this client has not heard of yet is placed until its frame lands. */
export async function applyFolderPlacements(
  _repoId: string,
  folders: readonly FolderDTO[],
  shiftedRows: readonly RowPlacement[] = [],
): Promise<void> {
  placeEntities('folder', Object.fromEntries(folders.map((f) => [f.id, f])))
  applyRows(shiftedRows)
}
