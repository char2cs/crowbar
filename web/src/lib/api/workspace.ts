import { saveWorkspaceHierarchy } from '@/lib/persistence/workspace-hierarchy'
import { useSidebarStore } from '@/lib/store/sidebar'

/**
 * Stub: simulates a backend reparent call. Replace body with real HTTP call
 * when backend exists; keep handleWorkspaceReparented as the WS/SSE handler.
 */
export async function reparentWorkspace(
  wsId: string,
  newParentId: string | undefined,
  repoId: string,
): Promise<void> {
  await handleWorkspaceReparented(wsId, newParentId, repoId)
}

/**
 * Notification handler — called by the stub above and, in future, by the
 * real-time WS/SSE message from the backend.
 */
export async function handleWorkspaceReparented(
  wsId: string,
  newParentId: string | undefined,
  repoId: string,
): Promise<void> {
  useSidebarStore.getState().reparentWorkspace(wsId, newParentId)

  const repo = useSidebarStore.getState().repos.find((r) => r.id === repoId)
  if (!repo) return

  const entries = repo.workspaces.map((w) => ({
    wsId: w.id,
    ...(w.parentId !== undefined && { parentId: w.parentId }),
  }))

  await saveWorkspaceHierarchy(repoId, entries)
}
