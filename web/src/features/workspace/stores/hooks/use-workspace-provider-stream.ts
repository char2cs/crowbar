import { useEffect } from 'react'
import { subscribeEntityStream } from '@/lib/ws/entity-stream'
import { fetchWorkspace } from '@/lib/api'
import { syncSidebarFromCache } from '@/lib/store/sidebar-sync'

/**
 * Open the per-:wsId workspace WS stream while a workspace is being viewed.
 *
 * This is load-bearing BEYOND data delivery: the daemon starts its per-connection
 * provider poll (the GitHub/GitLab PR-status detection that flips a workspace to
 * `pr-open`/`pr-merged`/`pr-closed`) ONLY when a client subscribes to the
 * single-workspace stream `/projects/:p/repos/:r/workspaces/:wsId`. The repo-level
 * workspace LIST stream (`.../workspaces`, no :wsId) resolves an empty scope, so it
 * never starts the poll — which is why a workspace whose branch had an open PR
 * stayed `new` (plain branch glyph) indefinitely.
 *
 * Subscribing here, keyed on the viewed workspace, mirrors the daemon's refcounted
 * per-connection poll lifecycle: the 0->1 subscribe transition starts the poll and
 * the last unsubscribe (navigate away / unmount) stops it. The resulting status DTO
 * arrives over the entity-cache path and the sidebar icon updates; we also rebuild
 * the sidebar directly on each frame so a single-workspace update is reflected even
 * if the list-stream scope did not also carry it.
 */
export function useWorkspaceProviderStream(
  projectId: string | undefined,
  repoId: string | undefined,
  wsId: string | undefined,
): void {
  useEffect(() => {
    if (!projectId || !repoId || !wsId) return
    return subscribeEntityStream({
      endpoint: `/v0/projects/${projectId}/repos/${repoId}/workspaces/${wsId}`,
      store: 'crowbar_workspaces',
      seed: () => fetchWorkspace(projectId, repoId, wsId).then((ws) => [ws]),
      onChange: () => void syncSidebarFromCache(),
      // This stream shares crowbar_workspaces with the repo-level LIST stream but
      // seeds with ONLY the viewed workspace — it is authoritative over that one
      // id, never its siblings. Without this scope its seed would prune every
      // other workspace and collapse the sidebar to the active row.
      pruneScope: (ws) => ws.id === wsId,
    })
  }, [projectId, repoId, wsId])
}
