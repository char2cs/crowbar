import type { useNavigate } from '@tanstack/react-router'
import { openChatRoute } from '@/components/layout/space-content-actions'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { chatPaneIndex } from '@/features/panes/lib/view-selectors'
import { getAllLeafIds } from '@/features/panes/utils/pane-layout'
import { useHomeTreeStore } from '@/lib/store/home-tree'
import type { Repo } from '@/lib/store/sidebar'
import { recentsChatWorkspaceId } from './recents-for-project'

type NavigateFn = ReturnType<typeof useNavigate>

/**
 * A Recents row's body: switch to the view, then route to the workspace of
 * the chat it focuses. The band is the view switcher — `setActivePane` brings
 * the owning view over — and `openChatRoute` is the tree click's own
 * home-or-repo resolve+open, so a project-home chat routes as well as a repo
 * one.
 */
export function focusRecent(viewId: string, repos: readonly Repo[], navigate: NavigateFn): void {
  const { views, panes, mostRecentActivePaneIds, paneActions } = windowPaneStore.getState()
  const view = views[viewId]
  if (!view) return
  const members = getAllLeafIds(view.layout).filter((id) => panes[id]?.chatId)
  const memberSet = new Set(members)
  const paneId = mostRecentActivePaneIds.find((id) => memberSet.has(id)) ?? members[0]
  const chatId = paneId ? panes[paneId]?.chatId : null
  if (!paneId || !chatId) return
  paneActions.setActivePane(paneId)
  const wsId = recentsChatWorkspaceId(
    repos,
    useHomeTreeStore.getState().trees,
    view.projectId,
    chatId,
  )
  openChatRoute(repos, chatId, wsId, navigate)
}

/** The row's × — ends the whole view, with the full per-chat teardown. */
export function closeRecent(viewId: string): void {
  windowPaneStore.getState().paneActions.closeView(viewId)
}

/** A group member's × — closes that one chat; the rest of the group stays. */
export function closeRecentChat(chatId: string): void {
  const { panes, paneActions } = windowPaneStore.getState()
  const paneId = chatPaneIndex(panes).get(chatId)
  if (paneId) paneActions.closePane(paneId)
}
