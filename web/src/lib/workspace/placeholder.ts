import type { Workspace } from '@/lib/store/sidebar'

/**
 * Why a workspace has no on-disk worktree.
 *
 * - `none` — it has one; nothing to say.
 * - `own-checkout` — the repo's OWN main folder has this branch checked out.
 *   That is the resting state of every imported repo (`adoptRepoHome` adopts
 *   repo.Path in place, on whatever branch it sits on, and provisioning then
 *   resolves `holder.HeldByHome` for that same branch), and handing the branch
 *   over is OPTIONAL — spec §3.5's detach-with-consent, not a failure. It has
 *   to be told apart from the case below, or every repo wears a permanent
 *   "couldn't set up" alarm for being in the state it is supposed to be in.
 * - `unprovisioned` — Crowbar tried and could not: another worktree holds the
 *   branch, or the worktree create itself failed. This is the one that needs
 *   the user's attention.
 *
 * The daemon's recorded `provisioning` IS the "no worktree" signal — status is
 * deliberately not part of it: an imported placeholder is not locked.
 *
 * `ownDefaultBranch` is `Repo.defaultBranch` — the branch of the repo's own
 * default workspace, which IS its main folder's checkout. Undefined for a
 * caller with no repo at all (project home), which can own no worktree and
 * therefore never reaches `own-checkout`. Compared by BRANCH, not by path:
 * `heldByPath` comes from `git worktree list` fully symlink-resolved while
 * `Repo.localPath` is the folder the user handed the importer, so the two are
 * routinely different spellings of the same directory (`/var` vs
 * `/private/var` on macOS).
 */
export type PlaceholderKind = 'none' | 'own-checkout' | 'unprovisioned'

export function placeholderKind(
  ws: Workspace,
  ownDefaultBranch: string | undefined,
): PlaceholderKind {
  if (ws.provisioning !== 'placeholder') return 'none'
  if (ownDefaultBranch && ws.branch === ownDefaultBranch) return 'own-checkout'
  return 'unprovisioned'
}

/**
 * The human-readable reason, matched to what the user can actually do about it.
 *
 * `own-checkout` states the fact and offers the one verb that works — Detach.
 * It never says "couldn't": nothing failed, and `RetryProvision` refuses this
 * case outright (`ErrBranchStillHeld`), so promising a Retry here would point
 * at a remedy coded never to succeed.
 *
 * For a real failure a live holder wins over a recorded cause, because it names
 * the checkout the user has to detach; a failure with no holder has no
 * reconstructable reason, so the cause the daemon recorded is the only
 * explanation there is (spec §3.3/§4/B7).
 */
export function placeholderReason(ws: Workspace, kind: PlaceholderKind): string {
  if (kind === 'none') return ''
  if (kind === 'own-checkout') {
    return `\`${ws.branch}\` is checked out at ${ws.heldByPath} — detach it to hand this branch to Crowbar.`
  }
  if (ws.heldByPath) {
    return `\`${ws.branch}\` is checked out at ${ws.heldByPath} — detach it to let Crowbar manage this branch.`
  }
  if (ws.lastError) {
    return `Crowbar couldn't set up \`${ws.branch}\`: ${ws.lastError}`
  }
  return `Crowbar couldn't set up \`${ws.branch}\`. Retry to provision it.`
}
