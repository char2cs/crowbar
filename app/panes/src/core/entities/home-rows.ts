import { readSelect, selectHomeTree, selectHomeTrees } from '@/core/entities/select'
import type { HomeTree } from '@/core/entities/views-from-entities'
import { getHomeWorkspaceId } from '@/features/workspace/lib/home-workspace-resolver'

export function getHomeTree(projectId: string): HomeTree {
  return readSelect((s) => selectHomeTree(s, projectId))
}

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
