import type { WorkspaceStore } from '@/features/workspace/stores/workspace-store'

/**
 * Open a chat, from wherever it was clicked — the Chats sidebar, or the New Tab
 * surface's recent list.
 *
 * Shared rather than duplicated because of the reveal branch below: it is the
 * kind of rule that gets re-derived slightly differently at a second call site
 * and then the two surfaces disagree about what clicking a chat does.
 *
 * `wsId` is unused directly here (a chat pane carries no `wsId` field of its
 * own) but kept in the signature — the workspace a chat belongs to is not
 * something this helper should silently stop caring about.
 */
export function openAgentChat(store: WorkspaceStore, _wsId: string, chatId: string): void {
  const st = store.getState()
  st.setActiveAgentChatId(chatId)

  // Already open somewhere? REVEAL it — focus the pane holding it and raise it
  // there. Landing it in the active pane instead would, for a chat that lives
  // in the other half of a split, yank it across the screen instead of taking
  // the user to it.
  const existingPane = Object.values(st.panes).find((p) => p.chatId === chatId)
  if (existingPane) {
    st.paneActions.setActivePane(existingPane.id)
    return
  }

  // Not open anywhere: land it as the ACTIVE pane's one chat. A pane holds at
  // most one, so this swaps out whatever chat (if any) that pane showed
  // before — `setPaneChat` itself archives that one into `dormantArrangements`
  // first (spec §8.4: "nothing you click ever costs you what you were looking
  // at"), so it is never silently lost. The runner is unknown here (`null`);
  // agent-chat-pane's own mount-time revive resolves and writes back the real
  // one.
  st.paneActions.setPaneChat(st.activePaneId, chatId, null)
}
