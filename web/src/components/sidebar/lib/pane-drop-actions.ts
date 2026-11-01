import { toast } from '@/features/window/stores/toast-store'
import { type DropMode } from '@/components/tree-dnd/drop-core'
import type { SidebarPaneZone } from '@/components/sidebar/hooks/use-sidebar-drag'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { chatPaneIndex } from '@/features/panes/lib/view-selectors'
import { viewChatIds } from '@/features/panes/lib/view-state'
import { resolveChatProjectId } from '@/features/panes/lib/chat-project'
import { owningChatIdOfWorkspace } from '@/components/sidebar/lib/branch-row-id'
import { readSelect, selectRepos } from '@/core/entities/select'

/**
 * Middle of a Recents row (spec §8.1: "into that view, opened"): every
 * dragged chat joins the target's view through `openChatIntoPane`'s own rules.
 */
function openRecentsEntryThenMerge(
  target: SidebarRow,
  targetChatId: string,
  dragged: readonly SidebarRow[],
): void {
  const findPaneFor = (chatId: string) =>
    chatPaneIndex(windowPaneStore.getState().panes).get(chatId)

  let targetPaneId = findPaneFor(targetChatId)
  if (!targetPaneId) {
    openChatInOwnPane(target)
    targetPaneId = findPaneFor(targetChatId)
  }
  if (!targetPaneId) return
  for (const subject of dragged) {
    // Compared by resolved CHAT id: a Recents row whose chat owns a workspace
    // is a `branch` row, and the tree's copy may name it by workspace id.
    if (paneChatSubject(subject)?.chatId === targetChatId) continue
    openChatIntoPane(subject, targetPaneId, 'center')
  }
}

/**
 * Above/below a Recents row (spec §8.1: "it moves to that slot"). A member
 * dragged out of a group leaves it first (`detachPane`); a whole row moves
 * within `viewOrder`.
 */
function reorderRecentsEntries(
  subjectChatIds: readonly string[],
  targetChatId: string,
  mode: 'before' | 'after',
): void {
  const viewOfChat = (chatId: string) => {
    const { panes } = windowPaneStore.getState()
    const paneId = chatPaneIndex(panes).get(chatId)
    return paneId ? panes[paneId]?.viewId : null
  }
  const targetViewId = viewOfChat(targetChatId)
  if (!targetViewId) return

  const moved = new Set<string>()
  for (const chatId of subjectChatIds) {
    const state = windowPaneStore.getState()
    const paneId = chatPaneIndex(state.panes).get(chatId)
    const sourceViewId = paneId ? state.panes[paneId]?.viewId : null
    if (!paneId || !sourceViewId || moved.has(sourceViewId)) continue
    if (viewChatIds(state, sourceViewId).length > 1) {
      state.paneActions.detachPane(paneId)
    } else if (sourceViewId === targetViewId) {
      continue
    }
    const viewId = viewOfChat(chatId)
    if (!viewId || viewId === targetViewId) continue
    moved.add(viewId)
    windowPaneStore.getState().paneActions.reorderView(viewId, targetViewId, mode)
  }
}

/**
 * A drop whose TARGET lives in a Recents band, not the tree — spec §8.1's
 * table gives this geometry a different meaning than the same drop over a
 * tree row: the middle opens the dragged chat(s) into that view, and
 * above/below reorders the band itself rather than writing a tree
 * placement.
 *
 * A Recents row is ALWAYS a chat — the band renders nothing else — but its
 * `kind` is not always `'chat'`: `recents-band.tsx` spreads `chatIconIndex`'s
 * fields onto the row so a chat that owns a workspace draws the tree's real
 * branch/lock/PR glyph, and `kind: 'branch'` rides along with them. Filtering
 * on `kind === 'chat'` here therefore dropped every such row on the floor,
 * both as a subject and as a target — and since a workspace-owning chat is the
 * common Recents row in a real install, that read as "Recents rows can't be
 * reordered" (live-reported). `paneChatSubject` is the resolver this file
 * already uses for the same question one gesture over (a row dropped onto a
 * pane): it answers with the chat a row names for both kinds, and null for a
 * folder or a workspace row with no chat behind it.
 */
export function performRecentsDrop(
  subjects: SidebarRow[],
  target: SidebarRow,
  mode: DropMode,
): void {
  const targetChatId = paneChatSubject(target)?.chatId
  if (!targetChatId) return
  const subjectChatIds = subjects.flatMap((s) => {
    const chatId = paneChatSubject(s)?.chatId
    return chatId ? [chatId] : []
  })
  if (subjectChatIds.length === 0) return
  if (mode === 'into') {
    openRecentsEntryThenMerge(target, targetChatId, subjects)
    return
  }
  reorderRecentsEntries(subjectChatIds, targetChatId, mode)
}

/** The chat a row opens into a pane, and the workspace that chat belongs to. */
interface PaneChatSubject {
  chatId: string
  workspaceId: string
}

/**
 * What a dragged row means to the PANE system: one chat, and its owning
 * workspace — or null for a row that names no chat at all.
 *
 * Two row kinds resolve, and they are the two the user can actually drag onto
 * a pane:
 *
 *   - a **chat** row (the tree's bubbles, and every Recents row — a Recents
 *     SET renders each member as its own draggable chat row, so a drag from
 *     the band always grabs exactly one chat, which is what makes "one chat
 *     per pane" fall out for free);
 *   - a **branch** row — a WORKSPACE row, and already a chat row wearing a
 *     workspace's clothes: `deriveRows` gives every workspace-owning
 *     row the id of the CHAT that owns its worktree. This used to be refused
 *     outright ("no pane has an 'open into' meaning for them yet"), which is
 *     exactly why dragging a workspace row onto the pane area did nothing.
 *     The chat is re-resolved through the sidebar's own tree
 *     (`owningChatIdOfWorkspace`, the same source the delete path reads)
 *     rather than trusting the row id blindly: a workspace whose owning chat
 *     has not arrived keeps its own workspace id as the row id, and opening
 *     THAT into a pane would point the pane at a chat that does not exist.
 *
 * A folder only folds and no 'workflow' row is produced anywhere yet, so both
 * stay null — a no-op rather than a guess at behaviour nothing has defined.
 */
function paneChatSubject(row: SidebarRow): PaneChatSubject | null {
  // The row carries the daemon's own answer for its workspace: that is what
  // the view member records (C3).
  if (row.kind === 'chat') {
    return row.workspaceId ? { chatId: row.id, workspaceId: row.workspaceId } : null
  }
  if (row.kind !== 'branch' || !row.workspaceId) return null
  const owner = owningChatIdOfWorkspace(readSelect(selectRepos), row.workspaceId)
  if (!owner) return null
  return { chatId: owner, workspaceId: row.workspaceId }
}

/**
 * A row dropped onto a pane — spec §8.1/§8.2. Every drop here ADDS; nothing
 * this reaches for can remove a pane or evict a chat that is already showing
 * (the dwell-to-remove gesture this replaced is gone — Task 22).
 */
export function performSidebarPaneDrop(
  subjects: SidebarRow[],
  paneId: string,
  zone: SidebarPaneZone,
): void {
  for (const subject of subjects) {
    openChatIntoPane(subject, paneId, zone)
  }
}

/**
 * One chat, CLICKED — spec §8.4: "clicking a chat makes its own view". A
 * click never merges; merging is the drag-and-drop gesture alone.
 */
export function openChatInOwnPane(subject: SidebarRow): void {
  const resolved = paneChatSubject(subject)
  if (!resolved) return
  windowPaneStore.getState().paneActions.openChat(resolved.chatId, {
    projectId: resolveChatProjectId(resolved.chatId, resolved.workspaceId) ?? undefined,
    workspaceId: resolved.workspaceId,
  })
}

/**
 * One chat, DROPPED onto one pane — spec §8.1/§8.2, the only gesture that
 * puts two chats in one view. An already-open chat is moved, never opened a
 * second time.
 */
function openChatIntoPane(subject: SidebarRow, paneId: string, zone: SidebarPaneZone): void {
  const resolved = paneChatSubject(subject)
  if (!resolved) return
  const { panes, views, paneActions } = windowPaneStore.getState()
  const target = panes[paneId]
  if (!target) return

  // LAW 4: content never crosses a project. An unresolvable side is allowed
  // through — a sidebar one frame behind must not break a same-project drop.
  const targetProject = target.viewId ? views[target.viewId]?.projectId : undefined
  const subjectProject = resolveChatProjectId(resolved.chatId, resolved.workspaceId)
  if (targetProject && subjectProject && targetProject !== subjectProject) {
    toast.error('That chat belongs to a different space')
    return
  }
  paneActions.dropChatOnPane(resolved.chatId, paneId, zone, resolved.workspaceId)
}
