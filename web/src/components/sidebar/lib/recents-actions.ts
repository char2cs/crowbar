import type { useNavigate } from '@tanstack/react-router'
import { resolveRow } from '@/components/layout/space-content-actions'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { viewIdOf } from '@/features/panes/lib/pane-views'
import { stopChat } from '@/features/agent/api/agent-api'
import type { Repo } from '@/lib/store/sidebar'
import type { RecentsBandEntry } from '@/components/sidebar/recents-band'

type NavigateFn = ReturnType<typeof useNavigate>

/**
 * Stop a chat's backend runner for a DORMANT Recents close (no live pane, so
 * `closePane`'s own `releaseClosedChat` — the path that normally makes
 * `stopChat` fire — never runs at all).
 *
 * A dormant Recents entry is not proof the chat's CLI is gone: `setPaneChat`
 * archives an evicted-but-IDLE chat into `dormantArrangements` on every
 * hotswap-away (spec §8.4) without ever asking its runner to stop — idle is
 * not the same fact as stopped. Without this, that chat's process (and its
 * companion PTY) keeps running in the daemon forever, indistinguishable from
 * a live entry except that Recents no longer shows a row for it at all.
 *
 * Fire-and-forget, matching `releaseClosedChat`'s own `stopChat` call: this
 * runs only from a Recents row's ×, a rare, explicit, user-initiated click —
 * never a render or poll path — and a chat whose CLI is already gone is a
 * backend no-op (`stopChat`'s own doc).
 */
function stopDormantChat(entry: RecentsBandEntry, chatId: string): void {
  const wsId = entry.chatWorkspaces?.[chatId] ?? entry.workspaceId
  stopChat(wsId, chatId).catch((err) => {
    if (import.meta.env.DEV) console.warn('stop dormant recents chat failed:', err)
  })
}

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
  const chatIds = new Set(entry.chatIds)
  const pane = paneActions.getAllPaneGroups().find((p) => p.chatId != null && chatIds.has(p.chatId))
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
 * - a DORMANT/SET entry (no live pane) → no pane to close, but the backend
 *   runner behind each chat can still be very much alive (`stopDormantChat`'s
 *   own doc) — stop it, then forget the remembered arrangement outright.
 */
export function closeRecent(entry: RecentsBandEntry): void {
  const { paneActions } = windowPaneStore.getState()
  // The VIEW the row stands for, resolved from any member still up — not
  // `entry.id`, which for a row that inherited a dormant record's slot is
  // that record's id rather than the live view's. `closeView` then ends every
  // pane in it, including one holding only editor tabs, which a
  // chatIds-driven loop would have left behind holding the view open.
  const chatIds = new Set(entry.chatIds)
  const member = paneActions
    .getAllPaneGroups()
    .find((p) => p.chatId != null && chatIds.has(p.chatId))
  if (member) {
    paneActions.closeView(viewIdOf(member))
    return
  }
  // No live pane means `closePane`'s own `stopChat` teardown never runs for
  // ANY of this entry's chats — see `stopDormantChat`'s own doc for why that
  // is a real gap, not a redundant call.
  for (const chatId of entry.chatIds) stopDormantChat(entry, chatId)
  // `localId` equals `id` now that panes are window-level (Task 26 removed
  // the old workspace-qualification these ids used to need — see
  // recents-for-project.ts) — kept as its own field since RecentsBandEntry
  // still declares it, and it's the id the one pane store's own
  // `dormantArrangements` are keyed by.
  paneActions.forgetDormantArrangement(entry.localId)
}

/**
 * Recents' per-chat × (feedback: "closes that chat from that group, but
 * doesn't dissolve the group, it just removes that one chat from it") —
 * narrower than `closeRecent` above, which always ends the WHOLE entry.
 *
 * - `chatId` has a live pane → `closePane` alone, never `closeView` (which
 *   would take every member of the view down with it). `closePane` already
 *   strips this one chat's id out of every `dormantArrangements` record on
 *   its own (pane-slice.ts's own note by its `dormantArrangements` edit), so
 *   there's nothing left to do once the pane itself is gone.
 * - no live pane (a dormant/'set' entry) → stop that one chat's backend
 *   runner (`stopDormantChat`'s own doc), then strip it from the one
 *   persisted arrangement this entry's `localId` names, leaving its
 *   remaining members exactly as they were.
 */
export function closeRecentChat(entry: RecentsBandEntry, chatId: string): void {
  const { paneActions } = windowPaneStore.getState()
  const pane = paneActions.getAllPaneGroups().find((p) => p.chatId === chatId)
  if (pane) {
    paneActions.closePane(pane.id)
    return
  }
  // Same gap as `closeRecent`'s own dormant branch, narrowed to this one
  // chat — see `stopDormantChat`'s doc.
  stopDormantChat(entry, chatId)
  paneActions.removeChatFromDormantArrangement(entry.localId, chatId)
}
