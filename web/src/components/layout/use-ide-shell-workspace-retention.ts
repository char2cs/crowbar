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
  /** The workspace WorkspaceHost should treat as "active" — see field doc
   *  below at the computation site. */
  effectiveActiveWorkspaceId: string | null
  /** Every workspace some pane currently holds a chat OR editor tab for —
   *  already unioned/deduped, `WorkspaceHost`'s own `paneWsIds` retention
   *  input. */
  paneWsIds: string[]
  /** Every workspace Recents currently tracks a chat for — `WorkspaceHost`'s
   *  `viewWsIds` retention input. */
  viewWsIds: string[]
  /** The active workspace's resolved filesystem path, for the sidebar's
   *  file-explorer card. */
  sidebarWorkspacePath: string
}

// Every workspace-id fact `WorkspaceHost`'s retention and the sidebar's file-explorer path need,
// resolved off the active pane and route, so `IDEShell` threads none of the intermediates.
export function useIdeShellWorkspaceRetention(
  activeWorkspaceId: string | undefined,
  homeWorkspaceId: string | null,
  activeProjectIdFromRoute: string | undefined,
  activeRepoIdFromRoute: string | undefined,
  isHomeRoute: boolean,
  homeWorkspacePath: string | null = null,
  projectPath = '',
): IdeShellWorkspaceRetention {
  // The ACTIVE PANE's workspace: "the workspace you're in" for the file-explorer card and the
  // host's active slot, so focusing a split's other pane moves them. Read off the pane record, so
  // clicking between two chats of one workspace re-renders nothing.
  const activePaneWorkspaceId = useActivePaneWorkspaceId()
  // Every workspace a view member belongs to, and every workspace some pane's
  // editor tabs reference (an editor-only pane names no chat) — together, the
  // workspaces something on screen or in Recents still needs a store for.
  const viewWorkspaceIds = useViewWorkspaceIds()
  const paneEditorWorkspaceIds = usePaneEditorWorkspaceIds()
  // The focused pane's workspace wins on EVERY route (the pane store guarantees it belongs to the
  // showing view); else the routed one (home: `homeWorkspaceId`), then the other.
  const effectiveActiveWorkspaceId =
    activePaneWorkspaceId ??
    (isHomeRoute ? homeWorkspaceId : activeWorkspaceId) ??
    (isHomeRoute ? activeWorkspaceId : homeWorkspaceId) ??
    null
  // Holding the viewed workspace's forge scope starts the daemon's provider poll, so a
  // branch with an open PR flips to its pr-open icon; the workspace frames carry it.
  const forgeHost = useResolvedHost({ wsId: activeWorkspaceId ?? '' })
  useEffect(
    () =>
      activeWorkspaceId && forgeHost
        ? holdScope(forgeHost, `forge:${activeWorkspaceId}`)
        : undefined,
    [activeWorkspaceId, forgeHost],
  )
  // Select only the resolved path, so live status frames don't rebuild the whole shell. Same id as
  // `effectiveActiveWorkspaceId`: the panel's path and scope must never diverge.
  const sidebarWorkspaceId = effectiveActiveWorkspaceId
  const sidebarWorkspacePath = useSelect((s) => {
    const repos = selectRepos(s)
    // The home workspace rides no repo, so use its real `localPath` rather than letting the repo
    // scan below land on another repo's directory.
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
    // Nothing resolved yet (e.g. `homeWorkspaceId` itself is still in flight)
    // — same home-path-first fallback, for the home route only.
    if (!isHomeRoute) return ''
    return (
      homeWorkspacePath ??
      repos.find((r) => r.projectId === activeProjectIdFromRoute)?.localPath ??
      ''
    )
  })

  // The empty stage shows no workspace, so the panel has none to scope to —
  // even though WorkspaceHost keeps the routed workspace active behind it.
  const isFocusedPaneEmpty = useStore(windowPaneStore, selectIsFocusedPaneEmpty)
  // The sidebar panel's one scope authority — see focused-workspace-context-store.ts.
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
