import type { useNavigate } from '@tanstack/react-router'
import { resolveRow } from '@/components/layout/space-content-actions'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { viewIdOf } from '@/features/panes/lib/pane-views'
import type { Repo } from '@/lib/store/sidebar'
import type { RecentsBandEntry } from '@/components/sidebar/recents-band'

type NavigateFn = ReturnType<typeof useNavigate>

/**
 * The Recents band's row body (spec §5.7: "Recents row *focuses*"): go to
 * the workspace the entry's chats live in, and — for a LIVE entry — bring
 * the pane(s) already holding it to the front.
 *
 * THE BAND IS THE VIEW SWITCHER. With one view on screen at a time there has
 * to be a way back to the others, and Recents already draws exactly one row
 * per view — so this row body is that way back, rather than a second, parallel
 * tab strip listing the same views again. It needs no switching code of its
 * own: `setActivePane` brings the owning view over before focusing the pane
 * (pane-slice.ts), so "focus the pane holding this chat" and "switch to the
 * view holding this chat" became the same act.
 */
export function focusRecent(
  entry: RecentsBandEntry,
  repos: readonly Repo[],
  navigate: NavigateFn,
): void {
  // THE SWITCH FIRST, AND UNCONDITIONALLY. This used to run after the
  // navigation below and behind its early return, so a row whose workspace
  // `resolveRow` could not place — a repo's own checkout, a project-home
  // workspace, anything not sitting in `repo.workspaces` — silently did
  // nothing at all. That was survivable when the row only "focused" a pane
  // already on screen; as the way back to a view that is NOT on screen it is
  // the whole gesture failing, and failing invisibly. Routing is a separate
  // concern and gets to fail on its own.
  //
  // Task 26: panes are window-level — chat ids are globally unique, so
  // finding "the pane holding one of this entry's chats" no longer needs the
  // entry's own workspace's store at all (there is only one pane store).
  const { paneActions } = windowPaneStore.getState()
  const pane = paneActions
    .getAllPaneGroups()
    .find((p) => p.chatId != null && entry.chatIds.includes(p.chatId))
  if (pane) paneActions.setActivePane(pane.id)

  const found = resolveRow(repos, entry.workspaceId)
  if (!found?.repo.projectId) return
  navigate({
    to: '/ide/$projectId/$repoId/$wsId',
    params: { projectId: found.repo.projectId, repoId: found.repo.id, wsId: entry.workspaceId },
  })
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
  // The VIEW the row stands for, resolved from any member still up — not
  // `entry.id`, which for a row that inherited a dormant record's slot is
  // that record's id rather than the live view's. `closeView` then ends every
  // pane in it, including one holding only editor tabs, which a
  // chatIds-driven loop would have left behind holding the view open.
  const member = paneActions
    .getAllPaneGroups()
    .find((p) => p.chatId != null && entry.chatIds.includes(p.chatId))
  if (member) {
    paneActions.closeView(viewIdOf(member))
    return
  }
  // `localId` equals `id` now that panes are window-level (Task 26 removed
  // the old workspace-qualification these ids used to need — see
  // recents-for-project.ts) — kept as its own field since RecentsBandEntry
  // still declares it, and it's the id the one pane store's own
  // `dormantArrangements` are keyed by.
  paneActions.forgetDormantArrangement(entry.localId)
}
