import { useEffect } from 'react'
import { useStore } from 'zustand'
import { selectRepos, useSelect } from '@/core/entities/select'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { selectIsFocusedPaneEmpty } from '@/features/panes/lib/view-selectors'
import {
  useActivePaneWorkspaceId,
  usePaneEditorWorkspaceIds,
  useViewWorkspaceIds,
} from '@/features/panes/hooks/use-chat-workspace-id'
import { holdScope } from '@/core/stream/stream'
import { useResolvedHost } from '@/lib/host-resolve'
import { usePublishFocusedWorkspaceContext } from './use-publish-focused-workspace-context'

export interface IdeShellWorkspaceRetention {
  effectiveActiveWorkspaceId: string | null
  paneWsIds: string[]
  viewWsIds: string[]
  sidebarWorkspacePath: string
}

export function useIdeShellWorkspaceRetention(
  activeWorkspaceId: string | undefined,
  homeWorkspaceId: string | null,
  activeProjectIdFromRoute: string | undefined,
  activeRepoIdFromRoute: string | undefined,
  isHomeRoute: boolean,
  homeWorkspacePath: string | null = null,
  projectPath = '',
): IdeShellWorkspaceRetention {
  const activePaneWorkspaceId = useActivePaneWorkspaceId()
  const viewWorkspaceIds = useViewWorkspaceIds()
  const paneEditorWorkspaceIds = usePaneEditorWorkspaceIds()
  const effectiveActiveWorkspaceId =
    activePaneWorkspaceId ??
    (isHomeRoute ? homeWorkspaceId : activeWorkspaceId) ??
    (isHomeRoute ? activeWorkspaceId : homeWorkspaceId) ??
    null
  const forgeHost = useResolvedHost({ wsId: activeWorkspaceId ?? '' })
  useEffect(
    () =>
      activeWorkspaceId && forgeHost
        ? holdScope(forgeHost, `forge:${activeWorkspaceId}`)
        : undefined,
    [activeWorkspaceId, forgeHost],
  )
  const sidebarWorkspaceId = effectiveActiveWorkspaceId
  const sidebarWorkspacePath = useSelect((s) => {
    const repos = selectRepos(s)
    if (sidebarWorkspaceId && sidebarWorkspaceId === homeWorkspaceId) {
      return (
        homeWorkspacePath ??
        repos.find((r) => r.projectId === activeProjectIdFromRoute)?.localPath ??
        ''
      )
    }
    if (sidebarWorkspaceId) {
      for (const repo of repos) {
        if (repo.defaultWorkspaceId === sidebarWorkspaceId) return repo.localPath ?? ''
        const ws = repo.workspaces.find((w) => w.id === sidebarWorkspaceId)
        if (ws) return ws.localPath || repo.localPath || ''
      }
    }
    if (!isHomeRoute) return ''
    return (
      homeWorkspacePath ??
      repos.find((r) => r.projectId === activeProjectIdFromRoute)?.localPath ??
      ''
    )
  })

  const isFocusedPaneEmpty = useStore(windowPaneStore, selectIsFocusedPaneEmpty)
  usePublishFocusedWorkspaceContext({
    workspaceId: isFocusedPaneEmpty ? null : effectiveActiveWorkspaceId,
    homeWorkspaceId,
    projectId: activeProjectIdFromRoute ?? null,
    routeWorkspaceId: activeWorkspaceId,
    routeRepoId: activeRepoIdFromRoute,
    isHomeRoute,
    workspacePath: sidebarWorkspacePath,
    projectPath,
  })

  return {
    effectiveActiveWorkspaceId,
    paneWsIds: [...new Set([...viewWorkspaceIds, ...paneEditorWorkspaceIds])],
    viewWsIds: viewWorkspaceIds,
    sidebarWorkspacePath,
  }
}
