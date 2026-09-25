import { useSidebarStore } from '@/lib/store/sidebar'
import {
  useActivePaneWorkspaceId,
  usePaneEditorWorkspaceIds,
  useViewWorkspaceIds,
} from '@/features/panes/hooks/use-chat-workspace-id'
import { useWorkspaceProviderStream } from '@/features/workspace/stores/hooks/use-workspace-provider-stream'
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

/**
 * Every workspace-id fact `WorkspaceHost`'s retention (`activeWsId`,
 * `paneWsIds`, `viewWsIds`) and the sidebar's file-explorer path need,
 * resolved off the active pane/route — pulled out of `IDEShell` itself,
 * which otherwise threaded ~10 intermediate values through its own body just
 * to reach the one component (`WorkspaceHost`) and the one selector
 * (`sidebarWorkspacePath`) that actually read them. Mirrors how
 * use-chat-workspace-id.ts already isolates each SINGLE-source resolution
 * this hook composes.
 */
export function useIdeShellWorkspaceRetention(
  activeWorkspaceId: string | undefined,
  homeWorkspaceId: string | null,
  activeProjectIdFromRoute: string | undefined,
  activeRepoIdFromRoute: string | undefined,
  isHomeRoute: boolean,
  homeWorkspacePath: string | null = null,
  projectPath = '',
): IdeShellWorkspaceRetention {
  // The workspace of the chat in the ACTIVE PANE — what everything meaning
  // "the workspace you're sitting in" follows (the file-explorer card, and
  // `WorkspaceHost`'s single active slot that mounts `WorkspaceActiveEffects`),
  // so clicking into a split's other pane moves them. Read off the pane's own
  // record (C3): clicking between two chats of one workspace re-renders
  // nothing here, and no sidebar or registry scan runs per frame.
  const activePaneWorkspaceId = useActivePaneWorkspaceId()
  // Every workspace a view member belongs to, and every workspace some pane's
  // editor tabs reference (an editor-only pane names no chat) — together, the
  // workspaces something on screen or in Recents still needs a store for.
  const viewWorkspaceIds = useViewWorkspaceIds()
  const paneEditorWorkspaceIds = usePaneEditorWorkspaceIds()
  // The workspace WorkspaceHost should treat as "active": the focused pane's
  // workspace wins on EVERY route, including home — the pane store's
  // integrity invariant guarantees `activePaneId` is a pane of the showing
  // view (or the chatless stage/tray), so it can never be a stale leftover
  // from another project. Falls back to the routed workspace (home:
  // `homeWorkspaceId`, else `activeWorkspaceId`), then the other one.
  const effectiveActiveWorkspaceId =
    activePaneWorkspaceId ??
    (isHomeRoute ? homeWorkspaceId : activeWorkspaceId) ??
    (isHomeRoute ? activeWorkspaceId : homeWorkspaceId) ??
    null
  // Open the per-:wsId workspace WS stream for the viewed workspace. Beyond data,
  // this is what starts the daemon's per-connection provider poll so a branch with
  // an open PR flips to the green pr-open icon (the list stream never starts it).
  useWorkspaceProviderStream(activeProjectIdFromRoute, activeRepoIdFromRoute, activeWorkspaceId)
  // The shell only needs one scalar from the sidebar tree. Subscribing to the
  // whole repos array made every live status/count frame rebuild the complete
  // IDE shell — sidebar provider, carousel, offscreen panels and workspace host
  // included. Returning the resolved path lets Zustand bail out unless the
  // active workspace's actual filesystem scope changed.
  // Same id as `effectiveActiveWorkspaceId` — the panel's path and scope must never diverge.
  const sidebarWorkspaceId = effectiveActiveWorkspaceId
  const sidebarWorkspacePath = useSidebarStore((s) => {
    // The home workspace is never one of `s.repos`' own workspaces (it rides
    // no repo — home-workspace-resolver.ts), so resolve its REAL on-disk path
    // directly (GET /home's own `localPath`) rather than falling through the
    // repo scan below and landing on some OTHER repo's directory as a
    // stand-in for "the project's root".
    if (sidebarWorkspaceId && sidebarWorkspaceId === homeWorkspaceId) {
      return (
        homeWorkspacePath ??
        s.repos.find((r) => r.projectId === activeProjectIdFromRoute)?.localPath ??
        ''
      )
    }
    if (sidebarWorkspaceId) {
      for (const repo of s.repos) {
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
      s.repos.find((r) => r.projectId === activeProjectIdFromRoute)?.localPath ??
      ''
    )
  })

  // The sidebar panel's one scope authority — see focused-workspace-context-store.ts.
  usePublishFocusedWorkspaceContext({
    workspaceId: effectiveActiveWorkspaceId,
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
