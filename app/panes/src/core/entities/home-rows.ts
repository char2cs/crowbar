import { readSelect, selectHomeTree, selectHomeTrees } from '@/core/entities/select'
import type { HomeTree } from '@/core/entities/views-from-entities'
import { getHomeWorkspaceId } from '@/features/workspace/lib/home-workspace-resolver'

/** `projectId`'s home chats and folders as the entity store holds them now. */
export function getHomeTree(projectId: string): HomeTree {
  return readSelect((s) => selectHomeTree(s, projectId))
}

/** Which project home `id` (a chat or a folder) is a row of, with that home's workspace;
 *  null for a repo's row or before the home is known. Read through the home trees, which hold
 *  every host's rows by their own id: the store's keys carry a remote host's prefix. */
export function resolveHomeRowScope(
  id: string,
): { kind: 'chat' | 'folder'; projectId: string; homeWorkspaceId: string } | null {
  for (const [projectId, tree] of Object.entries(readSelect(selectHomeTrees))) {
    const kind = tree.chats.some((c) => c.id === id)
      ? 'chat'
      : tree.folders.some((f) => f.id === id)
        ? 'folder'
        : null
    const homeWorkspaceId = kind && getHomeWorkspaceId(projectId)
    if (kind && homeWorkspaceId) return { kind, projectId, homeWorkspaceId }
  }
  return null
}

export type { HomeTree } from '@/core/entities/views-from-entities'
