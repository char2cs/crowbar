import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { getAllLeafIds } from '@/features/panes/utils/pane-layout'
import { chatWorkspaceIn, showingLayout } from '@/features/panes/lib/view-state'

// A chat's workspace for a non-pane caller: its pane record (C3), else the caller's `hint` (what
// its row claims). No guessing: null means the caller skips rather than open the wrong one.
export function resolveChatWorkspaceId(chatId: string, hint?: string | null): string | null {
  return chatWorkspaceIn(windowPaneStore.getState().panes, chatId) ?? hint ?? null
}

// The on-screen pane a sidebar file click for `wsId` should open into (`openContent`'s paneId):
// a sidebar click never moves `activePaneId`, which may sit in another chat's pane. Null keeps
// the active pane; only the showing view counts, so a file click never switches the screen.
export function resolveOnscreenPaneForWorkspace(wsId: string): string | null {
  const state = windowPaneStore.getState()
  const onscreenIds = getAllLeafIds(showingLayout(state))
  const onscreenIdSet = new Set(onscreenIds)
  const belongsToWorkspace = (paneId: string): boolean => {
    const pane = state.panes[paneId]
    return !!pane?.chatId && pane.workspaceId === wsId
  }

  if (onscreenIdSet.has(state.activePaneId) && belongsToWorkspace(state.activePaneId)) {
    return null
  }

  const recent = state.mostRecentActivePaneIds.find(
    (id) => onscreenIdSet.has(id) && belongsToWorkspace(id),
  )
  return recent ?? onscreenIds.find(belongsToWorkspace) ?? null
}
