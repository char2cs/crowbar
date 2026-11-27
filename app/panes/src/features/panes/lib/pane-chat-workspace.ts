import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { getAllLeafIds } from '@/features/panes/utils/pane-layout'
import { chatWorkspaceIn, showingLayout } from '@/features/panes/lib/view-state'
import { paneChatWorkspaceId } from '@/features/panes/lib/pane-chat-workspace-id'
import { readSelect, selectAgentChat } from '@/core/entities/select'

export function resolveChatWorkspaceId(chatId: string, hint?: string | null): string | null {
  return (
    readSelect((s) => selectAgentChat(s, chatId)?.workspaceId) ||
    chatWorkspaceIn(windowPaneStore.getState().panes, chatId) ||
    hint ||
    null
  )
}

export function resolveOnscreenPaneForWorkspace(wsId: string): string | null {
  const state = windowPaneStore.getState()
  const onscreenIds = getAllLeafIds(showingLayout(state))
  const onscreenIdSet = new Set(onscreenIds)
  const belongsToWorkspace = (paneId: string): boolean => {
    return paneChatWorkspaceId(state.panes[paneId]) === wsId
  }

  if (onscreenIdSet.has(state.activePaneId) && belongsToWorkspace(state.activePaneId)) {
    return null
  }

  const recent = state.mostRecentActivePaneIds.find(
    (id) => onscreenIdSet.has(id) && belongsToWorkspace(id),
  )
  return recent ?? onscreenIds.find(belongsToWorkspace) ?? null
}
