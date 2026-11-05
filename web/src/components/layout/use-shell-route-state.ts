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

/** What a shell derives from the route, shared by the desktop IDE shell and the phone's: the
 *  active project and workspace, WorkspaceHost's retention inputs, the home workspace, and the
 *  one "switch to project" / create-space actions the sidebar's switcher and tree both use. */
export function useShellRouteState() {
  const pathname = useRouterState({ select: (state) => state.location.pathname })
  const navigate = useNavigate()

  // §7: route params are canonical for the active scope (the sidebar store lags on cold start).
  // Recorded SYNCHRONOUSLY in render: WorkspaceView builds workspaceBase() URLs during its own
  // render.
  const routeScope = recordWorkspaceScopeFromPath(pathname)
  const homeRouteMatch = routeScope ? null : pathname.match(/\/ide\/([^/]+)\/home$/)
  const activeProjectId = routeScope?.projectId ?? homeRouteMatch?.[1]
  const activeRepoId = routeScope?.repoId
  const activeWorkspaceId = routeScope?.wsId

  // The home workspace, resolved once per project, so WorkspaceHost retains it like any slot. Not
  // gated to the home route: a split can mix a home chat's pane with a repo route.
  const {
    wsId: homeWorkspaceId,
    owningChatId: homeOwningChatId,
    localPath: homeWorkspacePath,
  } = useHomeWorkspaceState(activeProjectId ?? null)
  // Recorded during render for the same reason as the route scope above. Home never appears
  // in a repo tree, so its owning chat travels here or a chat-scoped URL cannot be built.
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

  // The tree's only entry point for a SECOND space (spec §3 ruling): the
  // switcher's trailing `+` swaps the tree for CreateSpacePanel in place.
  const [creatingSpace, setCreatingSpace] = useState(false)
  // The created project is already in the entity store: its operation settled after it.
  const handleCreateSpace = useCallback(() => setCreatingSpace(false), [])
  // One "switch to project" action for the switcher and the tree (spec §4.1); it
  // also cancels a space creation in progress. Stable, so memoized consumers skip
  // renders it had no part in.
  const handleSelectProject = useCallback(
    (projectId: string) => {
      setCreatingSpace(false)
      void navigate({ to: '/ide/$projectId/home', params: { projectId } })
    },
    [navigate],
  )
  const handleCancelCreateSpace = useCallback(() => setCreatingSpace(false), [])
  // The create-space form lives in the Chats panel, so creating one shows Chats.
  const handleAddProject = useCallback(() => {
    treeUiStore.getState().setActiveTab('chats')
    setCreatingSpace(true)
  }, [])

  // BUG-003: the active project follows the route. THE ONE WRITER of the pane
  // store's own `activeProjectId` too (project-scoped panes, trap 4): a second
  // writer racing this effect is how the sidebar's project-switch bugs happened.
  useEffect(() => {
    if (!activeProjectId) return
    if (useProjectStore.getState().activeProjectId !== activeProjectId) {
      useProjectStore.getState().setActiveProject(activeProjectId)
    }
    windowPaneStore.getState().paneActions.setActiveProject(activeProjectId)
  }, [activeProjectId])

  // See the hook's own doc for the "viewing a project that no longer exists" invariant.
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
