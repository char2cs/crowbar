import type { Problem } from '@/lib/errors/problem'
import type { Workspace } from '@/core/entities/repo-types'

// Why a workspace has no worktree, from the daemon's `provisioning` (not status): `own-checkout`
// is the repo's main folder holding the branch, every import's resting state (no alarm);
// `unprovisioned` is a failed create or another holder. Matched by branch: paths differ by symlink.
export type PlaceholderKind = 'none' | 'own-checkout' | 'unprovisioned'

export function placeholderKind(
  ws: Workspace,
  ownDefaultBranch: string | undefined,
): PlaceholderKind {
  if (ws.provisioning !== 'placeholder') return 'none'
  if (ownDefaultBranch && ws.branch === ownDefaultBranch) return 'own-checkout'
  return 'unprovisioned'
}

// The reason, matched to what the user can do. `own-checkout` offers Detach and never says
// "couldn't" (RetryProvision refuses it). For a failure a live holder wins, naming the checkout to
// detach; with none, the newest failed operation naming the workspace or its chat (`problem`).
export function placeholderReason(
  ws: Workspace,
  kind: PlaceholderKind,
  problem: Problem | null,
): string {
  if (kind === 'none') return ''
  if (kind === 'own-checkout') {
    return `\`${ws.branch}\` is checked out at ${ws.heldByPath} — detach it to hand this branch to Crowbar.`
  }
  if (ws.heldByPath) {
    return `\`${ws.branch}\` is checked out at ${ws.heldByPath} — detach it to let Crowbar manage this branch.`
  }
  const cause = problem?.message
  if (cause) {
    return `Crowbar couldn't set up \`${ws.branch}\`: ${cause}`
  }
  return `Crowbar couldn't set up \`${ws.branch}\`. Retry to provision it.`
}
