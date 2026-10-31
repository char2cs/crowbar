import { useEffect } from 'react'
import { holdScope } from '@/core/stream/stream'
import { subscribeEntityStream } from '@/lib/ws/entity-stream'
import { fetchWorkspace } from '@/lib/api'
import { workspaceDTOFromWorktreeFrame } from '@/lib/api/wire'
import { getOwningChatId } from '@/lib/workspace-scope'
import { chatBase } from '@/lib/workspace-scope-url'
import { syncSidebarFromCache } from '@/lib/store/sidebar-sync'
import { useSidebarStore } from '@/lib/store/sidebar'
import type { WorkspaceDTO } from '@/lib/types'

/**
 * Open the per-CHAT lifecycle WS stream while a workspace is being viewed, for
 * its `worktree_state` frames: `mapFrame` builds the workspace DTO from them so a
 * PR-status change reaches the sidebar. The daemon's forge poll that produces them
 * runs while the viewed workspace's forge: scope is held, so retained ones cost none.
 *
 * A workspace with no recorded owning chat is skipped rather than thrown on: this
 * runs on every render of the IDE shell, including before the sidebar has recorded
 * the chat the route's workspace belongs to.
 */
export function useWorkspaceProviderStream(
  projectId: string | undefined,
  repoId: string | undefined,
  wsId: string | undefined,
): void {
  // The owning chat, RE-READ whenever the sidebar tree changes, and an effect
  // dependency for that reason.
  //
  // Reading it once inside the effect would be a cold-load trap. The scope
  // registry this resolves through is populated BY these very store updates
  // (`recordRepoScopes` on setRepos/mergeRepos, and `applyWorkspaceDTO` per
  // frame), so on a fresh load the first render — the one the IDE shell mounts
  // this on — has no chat recorded for the route's workspace yet. The effect
  // would return early, none of its deps would ever change again, and the
  // subscription would never open: no PR-status frames for the whole session.
  //
  // The selector returns a plain string, so the effect re-runs only when the
  // ANSWER changes, not on every unrelated sidebar write.
  const owningChatId = useSidebarStore((state) => {
    void state.repos
    return (wsId ? getOwningChatId(wsId) : null) ?? ''
  })

  useEffect(() => (wsId ? holdScope(`forge:${wsId}`) : undefined), [wsId])

  useEffect(() => {
    if (!projectId || !repoId || !wsId || !owningChatId) return
    return subscribeEntityStream<WorkspaceDTO>({
      endpoint: `${chatBase(owningChatId)}/ws`,
      store: 'crowbar_workspaces',
      seed: () => fetchWorkspace(projectId, repoId, wsId).then((ws) => [ws]),
      mapFrame: (raw) => workspaceDTOFromWorktreeFrame(raw, projectId, repoId),
      onChange: () => void syncSidebarFromCache(),
      // This stream shares crowbar_workspaces with the repo-level LIST stream but
      // seeds with ONLY the viewed workspace — it is authoritative over that one
      // id, never its siblings. Without this scope its seed would prune every
      // other workspace and collapse the sidebar to the active row.
      pruneScope: (ws) => ws.id === wsId,
    })
  }, [projectId, repoId, wsId, owningChatId])
}
