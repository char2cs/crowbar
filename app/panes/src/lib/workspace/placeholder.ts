import type { Problem } from '@/lib/errors/problem'
import type { Workspace } from '@/core/entities/repo-types'

export type PlaceholderKind = 'none' | 'own-checkout' | 'unprovisioned'

export function placeholderKind(
  ws: Workspace,
  ownDefaultBranch: string | undefined,
): PlaceholderKind {
  if (ws.provisioning !== 'placeholder') return 'none'
  if (ownDefaultBranch && ws.branch === ownDefaultBranch) return 'own-checkout'
  return 'unprovisioned'
}

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
