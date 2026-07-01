import type { Workspace } from '@/lib/store/sidebar'

/**
 * A placeholder workspace is a protected branch that could not get a managed
 * worktree because a live worktree holds its branch. It is a `locked` row with
 * no on-disk worktree (spec §3.3). The status stays `locked` so every protection
 * guard applies; the placeholder is identified by the missing localPath.
 */
export function isPlaceholderWorkspace(ws: Workspace): boolean {
  return ws.status === 'locked' && !ws.localPath
}

/**
 * Reconstruct the human-readable reason a placeholder exists, from the branch
 * name + heldByPath (there is no persisted lastError — spec §3.3/§4/B7).
 */
export function placeholderReason(ws: Workspace): string {
  if (ws.heldByPath) {
    return `\`${ws.branch}\` is checked out at ${ws.heldByPath} — detach it to let Crowbar manage this branch.`
  }
  return `Crowbar couldn't set up \`${ws.branch}\`. Retry to provision it.`
}
