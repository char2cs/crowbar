import { useEffect } from 'react'
import { subscribeEntityStream } from '@/lib/ws/entity-stream'
import { fetchWorkspace, workspaceDTOFromWorktreeFrame } from '@/lib/api'
import { getOwningChatId } from '@/lib/workspace-scope'
import { chatBase } from '@/lib/workspace-scope-url'
import { syncSidebarFromCache } from '@/lib/store/sidebar-sync'
import { useSidebarStore } from '@/lib/store/sidebar'
import type { WorkspaceDTO } from '@/lib/types'

/**
 * Open the per-CHAT lifecycle WS stream while a workspace is being viewed.
 *
 * This is load-bearing BEYOND data delivery: the daemon starts its per-connection
 * provider poll (the GitHub/GitLab PR-status detection that flips a workspace to
 * `pr-open`/`pr-merged`/`pr-closed`) ONLY when a client subscribes to a stream
 * that resolves ONE worktree — now `/v0/chats/:chatId/ws`, where the daemon
 * resolves that chat's worktree and refcounts the poll on it. The repo-wide chat
 * stream (`.../repos/:r/chats/ws`, which app-sync-provider opens) resolves no
 * single workspace, so it still never starts the poll — which is why a workspace
 * whose branch had an open PR stayed `new` (plain branch glyph) indefinitely
 * without this.
 *
 * Subscribing here, keyed on the viewed workspace, mirrors the daemon's refcounted
 * per-connection poll lifecycle: the 0->1 subscribe transition starts the poll and
 * the last unsubscribe (navigate away / unmount) stops it. The frames are chat
 * lifecycle EVENTS, not workspace DTOs, so `mapFrame` picks the `worktree_state`
 * ones out and builds the DTO; the resulting status arrives over the entity-cache
 * path and the sidebar icon updates.
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
  // subscription would never open: no frames, and, far worse, the daemon's
  // PR-status poll never started for the whole session, leaving every branch
  // glyph stuck on `new`. That is the exact regression the per-chat mount
  // exists to prevent.
  //
  // The selector returns a plain string, so the effect re-runs only when the
  // ANSWER changes, not on every unrelated sidebar write.
  const owningChatId = useSidebarStore((state) => {
    void state.repos
    return (wsId ? getOwningChatId(wsId) : null) ?? ''
  })

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
