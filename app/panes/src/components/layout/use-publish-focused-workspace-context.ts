import { useLayoutEffect } from 'react'
import { selectRepos, useSelect } from '@/core/entities/select'
import { publishFocusedWorkspaceContext } from '@/features/window/stores/focused-workspace-context-store'

interface PublishInput {
  workspaceId: string | null
  homeWorkspaceId: string | null
  projectId: string | null
  routeWorkspaceId: string | undefined
  routeRepoId: string | undefined
  isHomeRoute: boolean
  workspacePath: string
  projectPath: string
}

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
  const sidebarRepoId = useSelect((s) => {
    if (!workspaceId || isProjectHome) return null
    for (const repo of selectRepos(s)) {
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
