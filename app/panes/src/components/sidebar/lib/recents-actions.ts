import type { useNavigate } from '@tanstack/react-router'
import { openChatRoute } from '@/components/layout/open-actions'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { getAllLeafIds } from '@/features/panes/utils/pane-layout'
import { paneChatWorkspaceId } from '@/features/panes/lib/pane-chat-workspace-id'
import type { Repo } from '@/core/entities/repo-types'

type NavigateFn = ReturnType<typeof useNavigate>

// A Recents row's body: switch to its view, then route to the focused chat's workspace through
// the tree click's own home-or-repo resolve, so a project-home chat routes too.
export function focusRecent(viewId: string, repos: readonly Repo[], navigate: NavigateFn): void {
  const { views, panes, mostRecentActivePaneIds, paneActions } = windowPaneStore.getState()
  const view = views[viewId]
  if (!view) return
  const members = getAllLeafIds(view.layout).filter((id) => panes[id]?.chatId)
  const memberSet = new Set(members)
  const paneId = mostRecentActivePaneIds.find((id) => memberSet.has(id)) ?? members[0]
  const pane = paneId ? panes[paneId] : undefined
  if (!paneId || !pane?.chatId) return
  paneActions.setActivePane(paneId)
  openChatRoute(repos, pane.chatId, paneChatWorkspaceId(pane) ?? '', navigate)
}

/** The row's × — ends the whole view. Its agents keep running; only Kill ends one. */
export function closeRecent(viewId: string): void {
  windowPaneStore.getState().paneActions.closeView(viewId)
}
