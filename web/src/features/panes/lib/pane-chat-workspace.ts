import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { getAllLeafIds } from '@/features/panes/utils/pane-layout'
import { chatWorkspaceIn, showingLayout } from '@/features/panes/lib/view-state'

/**
 * THE workspace a chat belongs to, for a caller that is not a pane.
 *
 * A chat on screen answers from its pane record, where the gesture that
 * opened it recorded the workspace (C3). Anything else answers with the
 * caller's own `hint` — what its row already claims (a `SidebarRow`'s
 * `workspaceId`, the daemon's answer delivered with the chat). There is no
 * third, guessing source: null means nothing can name a workspace, and the
 * caller skips rather than opens a chat under the wrong one.
 */
export function resolveChatWorkspaceId(chatId: string, hint?: string | null): string | null {
  return chatWorkspaceIn(windowPaneStore.getState().panes, chatId) ?? hint ?? null
}

/**
 * Live-reported: "I open a file on a given chat, and the file gets open in
 * another chat from the same group." Root cause: the file explorer is one
 * global UI ("always renders the active workspace" — see
 * file-explorer-tree.tsx), but `activePaneId` (window-pane-store.ts) is a
 * single value for the whole window that only updates on a literal click
 * INSIDE a pane's own surface (pane-container.tsx's handlePaneMouseDownCapture)
 * — never on a click in the sidebar, which sits outside the pane tree
 * entirely. Two chats sharing a workspace (one view record) can both be on screen at once (`splitPane`/
 * `dropChatOnPane`); if the user's last literal pane click landed in chat
 * A and they then click a file meant for chat B, `openContent`
 * (buffer-slice.ts) would read the stale `activePaneId` and the file lands in
 * A. A drop names its pane; a plain sidebar click has no such target and
 * needs one resolved for it, which the caller passes as `openContent`'s
 * `paneId` (C8).
 *
 * Resolves which on-screen pane a file-explorer action should target for
 * `wsId`, or null when nothing needs to change (the active pane already
 * belongs to `wsId`, or no on-screen pane does — leaving `activePaneId`
 * alone is the safe default either way).
 *
 * Restricted to the SHOWING view's panes: reaching into another view would
 * make a plain file click switch the user's whole screen.
 */
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
