import type { useNavigate } from '@tanstack/react-router'
import { resolveRow } from '@/components/layout/space-content-actions'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import type { Repo } from '@/lib/store/sidebar'
import type { RecentsBandEntry } from '@/components/sidebar/recents-band'

type NavigateFn = ReturnType<typeof useNavigate>

/**
 * The Recents band's row body (spec §5.7: "Recents row *focuses*"): go to
 * the workspace the entry's chats live in, and — for a LIVE entry — bring
 * the pane(s) already holding it to the front, exactly as clicking that
 * pane's own tab would.
 */
export function focusRecent(
  entry: RecentsBandEntry,
  repos: readonly Repo[],
  navigate: NavigateFn,
): void {
  const found = resolveRow(repos, entry.workspaceId)
  if (!found?.repo.projectId) return
  navigate({
    to: '/ide/$projectId/$repoId/$wsId',
    params: { projectId: found.repo.projectId, repoId: found.repo.id, wsId: entry.workspaceId },
  })
  const { paneActions } = getOrCreateWorkspaceStore(entry.workspaceId).getState()
  const pane = paneActions
    .getAllPaneGroups()
    .find((p) => p.chatId != null && entry.chatIds.includes(p.chatId))
  if (pane) paneActions.setActivePane(pane.id)
}

/**
 * The Recents band's × (spec §5.4): "end this view, never touch the chat."
 *
 * - a LIVE entry (chats are actually in a pane) → close every pane holding
 *   one of its chats. `closePane` itself already decides whether that
 *   leaves a dormant record (idle chat) or none (still working) — see
 *   pane-slice.ts.
 * - a DORMANT/SET entry (no live pane) → nothing to close; forget the
 *   remembered arrangement outright.
 */
export function closeRecent(entry: RecentsBandEntry): void {
  const { paneActions } = getOrCreateWorkspaceStore(entry.workspaceId).getState()
  const panes = paneActions
    .getAllPaneGroups()
    .filter((p) => p.chatId != null && entry.chatIds.includes(p.chatId))
  if (panes.length > 0) {
    for (const pane of panes) paneActions.closePane(pane.id)
    return
  }
  // `entry.id` is workspace-qualified for cross-workspace display uniqueness
  // (see recents-for-project.ts) — `localId` is the real id the owning
  // store's own `dormantArrangements` are keyed by.
  paneActions.forgetDormantArrangement(entry.localId)
}
