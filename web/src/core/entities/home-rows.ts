import { entityStore } from '@/core/entities/entity-store'
import { readSelect, selectHomeTree } from '@/core/entities/select'
import type { HomeTree } from '@/core/entities/views-from-entities'
import { getHomeWorkspaceId } from '@/features/workspace/lib/home-workspace-resolver'

/** `projectId`'s home chats and folders as the entity store holds them now. */
export function getHomeTree(projectId: string): HomeTree {
  return readSelect((s) => selectHomeTree(s, projectId))
}

/** Which project home `id` (a chat or a folder) is a row of, with that home's workspace;
 *  null for a repo's row or before the home is known. */
export function resolveHomeRowScope(
  id: string,
): { kind: 'chat' | 'folder'; projectId: string; homeWorkspaceId: string } | null {
  const { chat, folder } = entityStore.getState().byKind
  const row = chat[id]?.data ?? folder[id]?.data
  if (!row || row.repoId) return null
  const homeWorkspaceId = getHomeWorkspaceId(row.projectId)
  if (!homeWorkspaceId) return null
  return { kind: id in chat ? 'chat' : 'folder', projectId: row.projectId, homeWorkspaceId }
}

export type { HomeTree } from '@/core/entities/views-from-entities'
