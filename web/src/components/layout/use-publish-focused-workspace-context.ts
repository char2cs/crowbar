import { useLayoutEffect } from 'react'
import { useSidebarStore } from '@/lib/store/sidebar'
import { publishFocusedWorkspaceContext } from '@/features/window/stores/focused-workspace-context-store'

interface PublishInput {
  workspaceId: string | null
  homeWorkspaceId: string | null
  projectId: string | null
  /** The route's own workspace/repo — only used to seed `repoId` before the sidebar knows it. */
  routeWorkspaceId: string | undefined
  routeRepoId: string | undefined
  /** Only consulted while `workspaceId` is unresolved. */
  isHomeRoute: boolean
  /** The workspace's resolved on-disk path ('' if unknown). */
  workspacePath: string
  /** The project's own path — the Files root fallback for project home. */
  projectPath: string
}

/**
 * Derives the focused-workspace context from the resolved active workspace and
 * publishes it pre-paint, so the panel never renders one frame of stale scope.
 */
export function usePublishFocusedWorkspaceContext({
  workspaceId,
  homeWorkspaceId,
  projectId,
  routeWorkspaceId,
  routeRepoId,
  isHomeRoute,
  workspacePath,
  projectPath,
}: PublishInput): void {
  const isProjectHome = workspaceId ? workspaceId === homeWorkspaceId : isHomeRoute
  const sidebarRepoId = useSidebarStore((s) => {
    if (!workspaceId || isProjectHome) return null
    for (const repo of s.repos) {
      if (repo.defaultWorkspaceId === workspaceId) return repo.id
      if (repo.workspaces.some((w) => w.id === workspaceId)) return repo.id
    }
    return null
  })
  const repoId = isProjectHome
    ? null
    : (sidebarRepoId ??
      (workspaceId && workspaceId === routeWorkspaceId ? (routeRepoId ?? null) : null))
  const rootPath = workspacePath || (isProjectHome ? projectPath : '')

  useLayoutEffect(() => {
    publishFocusedWorkspaceContext({
      projectId,
      workspaceId,
      isProjectHome,
      repoId,
      repoPath: repoId ? rootPath || null : null,
      rootPath,
    })
  }, [projectId, workspaceId, isProjectHome, repoId, rootPath])
}
