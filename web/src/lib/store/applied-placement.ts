import { entityOf, entityStore, patchEntities, placeEntities } from '@/core/entities/entity-store'
import { entityKey } from '@/core/stream/host-scope'
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

/** Applies `hostId`'s placement answers. Rows of one level share an id space: a chat, a
 *  folder or a locked branch's workspace. */
function applyRows(hostId: string, rows: readonly RowPlacement[]): void {
  const { chat, folder, workspace } = entityStore.getState().byKind
  const chats: Record<string, { parentId: string; order: number }> = {}
  const folders: Record<string, { parentId: string; order: number }> = {}
  const workspaces: Record<string, { folderId: string; order: number }> = {}
  for (const row of rows) {
    const parentId = row.parentId ?? ''
    const key = entityKey(hostId, row.id)
    if (key in chat) chats[key] = { parentId, order: row.order }
    else if (key in folder) folders[key] = { parentId, order: row.order }
    else if (key in workspace) workspaces[key] = { folderId: parentId, order: row.order }
  }
  patchEntities('chat', chats)
  patchEntities('folder', folders)
  patchEntities('workspace', workspaces)
}

/** A chat placement: the moved chat and every sibling the renumber shifted. Resolves to
 *  the repo the chat lives in, or null for a project home's. */
export async function applyChatPlacement(
  hostId: string,
  chat: RowPlacement,
  shifted: readonly RowPlacement[] = [],
): Promise<string | null> {
  applyRows(hostId, [chat, ...shifted])
  return entityOf(entityStore.getState(), 'chat', hostId, chat.id)?.repoId || null
}

/** A workspace's own placement answer and the siblings it shifted. */
export async function applyWorkspacePlacement(
  hostId: string,
  wsId: string,
  placement: { parentId: string; order: number },
  shifted: readonly RowPlacement[] = [],
): Promise<void> {
  patchEntities('workspace', {
    [entityKey(hostId, wsId)]: { folderId: placement.parentId, order: placement.order },
  })
  applyRows(hostId, shifted)
}

/** A folder write's answer: the folder rows it wrote, and the siblings of another kind it
 *  renumbered. A folder this client has not heard of yet is placed until its frame lands. */
export async function applyFolderPlacements(
  hostId: string,
  folders: readonly FolderDTO[],
  shiftedRows: readonly RowPlacement[] = [],
): Promise<void> {
  placeEntities('folder', Object.fromEntries(folders.map((f) => [entityKey(hostId, f.id), f])))
  applyRows(hostId, shiftedRows)
}
