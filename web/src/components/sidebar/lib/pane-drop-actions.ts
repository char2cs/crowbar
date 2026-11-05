import { toast } from '@/features/window/stores/toast-store'
import { type DropMode } from '@/components/tree-dnd/drop-core'
import type { SidebarPaneZone } from '@/components/sidebar/lib/sidebar-drag-hover'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { chatPaneIndex } from '@/features/panes/lib/view-selectors'
import { viewChatIds } from '@/features/panes/lib/view-state'
import { resolveChatProjectId } from '@/features/panes/lib/chat-project'
import { owningChatIdOfWorkspace } from '@/components/sidebar/lib/branch-row-id'
import { readSelect, selectRepos } from '@/core/entities/select'

// Middle of a Recents row (spec §8.1): every dragged chat joins the target's view.
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

// Above/below a Recents row (spec §8.1): a member dragged out of a group detaches first; a
// whole view moves within `viewOrder`.
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
  const subjectSet = new Set(subjectChatIds)
  for (const chatId of subjectChatIds) {
    const state = windowPaneStore.getState()
    const paneId = chatPaneIndex(state.panes).get(chatId)
    const sourceViewId = paneId ? state.panes[paneId]?.viewId : null
    if (!paneId || !sourceViewId || moved.has(sourceViewId)) continue
    // A drag carrying every chat of a view moves the view; a partial one
    // pulls its chats out of the group first.
    const viewChats = viewChatIds(state, sourceViewId)
    if (viewChats.length > 1 && !viewChats.every((id) => subjectSet.has(id))) {
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

// A drop onto a Recents row (spec §8.1): the middle opens the dragged chats into that view, the
// edges reorder the band — no tree placement. Recents rows are always chats, but one owning a
// workspace wears `kind: 'branch'`, so resolve through paneChatSubject, never `kind === 'chat'`.
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

// What a dragged row means to the pane system: one chat and its workspace, or null. A chat row
// answers directly; a branch row resolves its owning chat from the sidebar tree, since one whose
// owner hasn't arrived would point the pane at no chat. Folders and workflows: null.
function paneChatSubject(row: SidebarRow): PaneChatSubject | null {
  // The row carries the daemon's own answer for its workspace, which the view member records.
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
 * this reaches for can remove a pane or evict a chat that is already showing.
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
