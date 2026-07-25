import type { Workspace } from '@/lib/store/sidebar'

/**
 * A placeholder workspace is one that has no on-disk worktree: its branch could
 * not be materialised, most often because a live worktree already has it checked
 * out (spec §3.3). The missing localPath IS the signal — status is deliberately
 * not part of it. Protected-branch placeholders are seeded `locked` to inherit
 * the protection guards, but an IMPORTED feature branch must stay unlocked
 * (locked survives provisioning and would block merge/rename/delete forever), so
 * requiring `locked` here left every failed import row unrecognised: no reason,
 * no Retry/Detach…, and no toast.
 */
export function isPlaceholderWorkspace(ws: Workspace): boolean {
  return !ws.localPath
}

/**
 * Reconstruct the human-readable reason a placeholder exists. A live holder is
 * the actionable case and wins, because it names the checkout the user has to
 * detach. Otherwise fall back to the cause the daemon recorded on the row — a
 * failure with no holder has no reconstructable reason, and the generic line
 * would hide the only explanation there is (spec §3.3/§4/B7).
 */
export function placeholderReason(ws: Workspace): string {
  if (ws.heldByPath) {
    return `\`${ws.branch}\` is checked out at ${ws.heldByPath} — detach it to let Crowbar manage this branch.`
  }
  if (ws.lastError) {
    return `Crowbar couldn't set up \`${ws.branch}\`: ${ws.lastError}`
  }
  return `Crowbar couldn't set up \`${ws.branch}\`. Retry to provision it.`
}
