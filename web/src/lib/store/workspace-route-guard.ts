import type { Loadable } from '@/lib/loadable'
import type { Repo } from '@/lib/store/sidebar'

/**
 * BUG-010: decide whether a `/workspaces/<wsId>` route points at a workspace
 * that does not exist, once the workspace list has actually loaded.
 *
 * - While the list is idle/loading (or only has stale data after an error),
 *   never redirect — the id may simply not have arrived yet, and redirecting
 *   on a transient backend failure would kick the user out of a live route.
 * - Presence is checked against the sidebar repos, which include optimistic
 *   additions (a just-created workspace must not be treated as unknown).
 */
export function shouldRedirectUnknownWorkspace(
  listStatus: Loadable<unknown>['status'],
  repos: Repo[],
  wsId: string | undefined,
): boolean {
  if (!wsId) return false
  if (listStatus !== 'success') return false
  return !repos.some((repo) => repo.workspaces.some((ws) => ws.id === wsId))
}
