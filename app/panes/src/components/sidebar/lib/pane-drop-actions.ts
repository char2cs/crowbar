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
    if (paneChatSubject(subject)?.chatId === targetChatId) continue
    openChatIntoPane(subject, targetPaneId, 'center')
  }
}

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

interface PaneChatSubject {
  chatId: string
  workspaceId: string
}

function paneChatSubject(row: SidebarRow): PaneChatSubject | null {
  if (row.kind === 'chat') {
    return row.workspaceId ? { chatId: row.id, workspaceId: row.workspaceId } : null
  }
  if (row.kind !== 'branch' || !row.workspaceId) return null
  const owner = owningChatIdOfWorkspace(readSelect(selectRepos), row.workspaceId)
  if (!owner) return null
  return { chatId: owner, workspaceId: row.workspaceId }
}

export function performSidebarPaneDrop(
  subjects: SidebarRow[],
  paneId: string,
  zone: SidebarPaneZone,
): void {
  for (const subject of subjects) {
    openChatIntoPane(subject, paneId, zone)
  }
}

export function openChatInOwnPane(subject: SidebarRow): void {
  const resolved = paneChatSubject(subject)
  if (!resolved) return
  windowPaneStore.getState().paneActions.openChat(resolved.chatId, {
    projectId: resolveChatProjectId(resolved.chatId, resolved.workspaceId) ?? undefined,
    workspaceId: resolved.workspaceId,
  })
}

function openChatIntoPane(subject: SidebarRow, paneId: string, zone: SidebarPaneZone): void {
  const resolved = paneChatSubject(subject)
  if (!resolved) return
  const { panes, views, paneActions } = windowPaneStore.getState()
  const target = panes[paneId]
  if (!target) return

  const targetProject = target.viewId ? views[target.viewId]?.projectId : undefined
  const subjectProject = resolveChatProjectId(resolved.chatId, resolved.workspaceId)
  if (targetProject && subjectProject && targetProject !== subjectProject) {
    toast.error('That chat belongs to a different space')
    return
  }
  paneActions.dropChatOnPane(resolved.chatId, paneId, zone, resolved.workspaceId)
}
