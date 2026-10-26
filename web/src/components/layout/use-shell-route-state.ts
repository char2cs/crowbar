import { useCallback, useEffect, useState } from 'react'
import { useNavigate, useRouterState } from '@tanstack/react-router'
import {
  EMPTY_PROJECTS,
  importProjectAndSync,
  useProjectDataStore,
  useProjectStore,
} from '@/lib/store/projects'
import type { Project } from '@/lib/types'
import { useSidebarStore } from '@/lib/store/sidebar'
import { recordWorkspaceScopeFromPath } from '@/lib/workspace-scope'
import { dataOf } from '@/lib/loadable'
import {
  ensureHomeWorkspaceResolved,
  useHomeWorkspaceState,
} from '@/features/workspace/lib/home-workspace-resolver'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { useIdeShellWorkspaceRetention } from './use-ide-shell-workspace-retention'
import { useRedirectIfProjectMissing } from './use-redirect-if-project-missing'

/**
 * What a shell derives from the route, shared by the desktop IDE shell and the phone's: the
 * active project and workspace, WorkspaceHost's retention inputs, the home workspace, and the
 * one "switch to project" / create-space actions the sidebar's switcher and tree both use.
 */
export function useShellRouteState() {
  const pathname = useRouterState({ select: (state) => state.location.pathname })
  const navigate = useNavigate()

  // §7: the TanStack /ide/:projectId/:repoId/:wsId route params are the
  // canonical source for the active project/repo/workspace — read them directly
  // rather than scanning the sidebar store (which lags the route on cold start).
  // Recording the scope here, SYNCHRONOUSLY during render, is load-bearing: the
  // WorkspaceView subtree builds workspace-scoped URLs via workspaceBase() during
  // its own render, so the scope must exist before then (§14 add-repo regression).
  const routeScope = recordWorkspaceScopeFromPath(pathname)
  const homeRouteMatch = routeScope ? null : pathname.match(/\/ide\/([^/]+)\/home$/)
  const activeProjectId = routeScope?.projectId ?? homeRouteMatch?.[1]
  const activeRepoId = routeScope?.repoId
  const activeWorkspaceId = routeScope?.wsId

  // Resolve the home workspace id ONCE per project (cached for the session, see
  // home-workspace-resolver.ts) so WorkspaceHost keeps its view mounted-but-hidden
  // like any other retained slot. Not gated to the home route: retention's
  // sidebar-path lookup needs this project's home id/path off it too (a split can
  // mix a home chat's pane with a repo route).
  const { wsId: homeWorkspaceId, localPath: homeWorkspacePath } = useHomeWorkspaceState(
    activeProjectId ?? null,
  )
  useEffect(() => {
    if (activeProjectId) ensureHomeWorkspaceResolved(activeProjectId)
  }, [activeProjectId])

  const allProjects = useProjectDataStore((s) => dataOf(s.data) ?? EMPTY_PROJECTS)
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
  const handleCreateSpace = useCallback((project: Project) => {
    importProjectAndSync(project)
    setCreatingSpace(false)
  }, [])
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
    useSidebarStore.getState().setActiveTab('chats')
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
