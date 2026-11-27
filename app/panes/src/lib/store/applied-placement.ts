import { entityOf, entityStore, patchEntities, placeEntities } from '@/core/entities/entity-store'
import { entityKey } from '@/core/stream/host-scope'
import type { FolderDTO } from '@/lib/types'

export interface RowPlacement {
  id: string
  parentId?: string
  order: number
}

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

export async function applyChatPlacement(
  hostId: string,
  chat: RowPlacement,
  shifted: readonly RowPlacement[] = [],
): Promise<string | null> {
  applyRows(hostId, [chat, ...shifted])
  return entityOf(entityStore.getState(), 'chat', hostId, chat.id)?.repoId || null
}

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

export async function applyFolderPlacements(
  hostId: string,
  folders: readonly FolderDTO[],
  shiftedRows: readonly RowPlacement[] = [],
): Promise<void> {
  placeEntities('folder', Object.fromEntries(folders.map((f) => [entityKey(hostId, f.id), f])))
  applyRows(hostId, shiftedRows)
}
