import { useCallback, useEffect, useState } from 'react'
import { useNavigate, useRouterState } from '@tanstack/react-router'
import { useProjectStore } from '@/core/entities/active-project'
import { selectProjects, useSelect } from '@/core/entities/select'
import { treeUiStore } from '@/features/workspace/tree/tree-ui-store'
import { recordWorkspaceScopeFromPath, setWorkspaceScope } from '@/lib/workspace-scope'
import { useHomeWorkspaceState } from '@/features/workspace/lib/home-workspace-resolver'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { useIdeShellWorkspaceRetention } from './use-ide-shell-workspace-retention'
import { useRedirectIfProjectMissing } from './use-redirect-if-project-missing'

export function useShellRouteState() {
  const pathname = useRouterState({ select: (state) => state.location.pathname })
  const navigate = useNavigate()

  const routeScope = recordWorkspaceScopeFromPath(pathname)
  const homeRouteMatch = routeScope ? null : pathname.match(/\/ide\/([^/]+)\/home$/)
  const activeProjectId = routeScope?.projectId ?? homeRouteMatch?.[1]
  const activeRepoId = routeScope?.repoId
  const activeWorkspaceId = routeScope?.wsId

  const {
    wsId: homeWorkspaceId,
    owningChatId: homeOwningChatId,
    localPath: homeWorkspacePath,
  } = useHomeWorkspaceState(activeProjectId ?? null)
  if (homeRouteMatch && activeProjectId && homeWorkspaceId) {
    setWorkspaceScope({
      projectId: activeProjectId,
      repoId: '',
      wsId: homeWorkspaceId,
      owningChatId: homeOwningChatId ?? undefined,
    })
  }

  const allProjects = useSelect(selectProjects)
  const projectPath = allProjects.find((p) => p.id === activeProjectId)?.path ?? ''
  const retention = useIdeShellWorkspaceRetention(
    activeWorkspaceId,
    homeWorkspaceId,
    activeProjectId,
    activeRepoId,
    Boolean(homeRouteMatch),
    homeWorkspacePath,
    projectPath,
  )

  const [creatingSpace, setCreatingSpace] = useState(false)
  const handleCreateSpace = useCallback(() => setCreatingSpace(false), [])
  const handleSelectProject = useCallback(
    (projectId: string) => {
      setCreatingSpace(false)
      void navigate({ to: '/ide/$projectId/home', params: { projectId } })
    },
    [navigate],
  )
  const handleCancelCreateSpace = useCallback(() => setCreatingSpace(false), [])
  const handleAddProject = useCallback(() => {
    treeUiStore.getState().setActiveTab('chats')
    setCreatingSpace(true)
  }, [])

  useEffect(() => {
    if (!activeProjectId) return
    if (useProjectStore.getState().activeProjectId !== activeProjectId) {
      useProjectStore.getState().setActiveProject(activeProjectId)
    }
    windowPaneStore.getState().paneActions.setActiveProject(activeProjectId)
    const hostId = allProjects.find((p) => p.id === activeProjectId)?.hostId
    if (hostId) treeUiStore.getState().rememberHostProject(hostId, activeProjectId)
  }, [activeProjectId, allProjects])

  useRedirectIfProjectMissing(activeProjectId, navigate)

  return {
    activeProjectId,
    activeWorkspaceId,
    homeWorkspaceId,
    allProjects,
    ...retention,
    creatingSpace,
    handleCreateSpace,
    handleSelectProject,
    handleCancelCreateSpace,
    handleAddProject,
  }
}
