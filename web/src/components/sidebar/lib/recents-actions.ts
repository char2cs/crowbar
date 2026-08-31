import type { useNavigate } from '@tanstack/react-router'
import { resolveRow } from '@/components/layout/space-content-actions'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
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
  // Task 26: panes are window-level now — chat ids are globally unique, so
  // finding "the pane holding one of this entry's chats" no longer needs the
  // entry's own workspace's store at all (there is only one pane store).
  const { paneActions } = windowPaneStore.getState()
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
  const { paneActions } = windowPaneStore.getState()
  const panes = paneActions
    .getAllPaneGroups()
    .filter((p) => p.chatId != null && entry.chatIds.includes(p.chatId))
  if (panes.length > 0) {
    for (const pane of panes) paneActions.closePane(pane.id)
    return
  }
  // `localId` equals `id` now that panes are window-level (Task 26 removed
  // the old workspace-qualification these ids used to need — see
  // recents-for-project.ts) — kept as its own field since RecentsBandEntry
  // still declares it, and it's the id the one pane store's own
  // `dormantArrangements` are keyed by.
  paneActions.forgetDormantArrangement(entry.localId)
}
