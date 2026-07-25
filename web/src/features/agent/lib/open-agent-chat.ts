import type { WorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'

/**
 * Open a chat, from wherever it was clicked — the Chats sidebar, or the New Tab
 * surface's recent list.
 *
 * Shared rather than duplicated because of the reveal branch below: it is the
 * kind of rule that gets re-derived slightly differently at a second call site
 * and then the two surfaces disagree about what clicking a chat does.
 *
 * `wsId` is passed rather than read off the store because the caller already
 * holds it, and the workspace a chat belongs to is not something this helper
 * should have an opinion about.
 */
export function openAgentChat(store: WorkspaceStore, wsId: string, chatId: string): void {
  const st = store.getState()
  const title = st.agentChats.chats.find((c) => c.id === chatId)?.title
  st.setActiveAgentChatId(chatId)

  // Already open somewhere? REVEAL it — focus the pane holding the tab and raise
  // it there. openContent would instead add the buffer to whatever pane happens
  // to be active, so clicking a chat that lives in the other half of a split
  // yanked its tab across the screen instead of taking the user to it.
  const existing = st.buffers.find((b) => b.type === 'agentChat' && b.chatId === chatId)
  const pane = existing ? st.paneActions.getPaneByBufferId(existing.id) : null
  if (existing && pane) {
    st.paneActions.setActivePane(pane.id)
    st.paneActions.activatePaneBuffer(pane.id, existing.id)
    return
  }

  // The pane tab must name an untitled chat the SAME as the sidebar row and the
  // New Tab surface — chat-label.ts's constant exists exactly so these don't
  // drift into looking like two different chats. (The old literal here was
  // 'Agent chat', which did drift.)
  st.bufferActions.openContent({
    type: 'agentChat',
    chatId,
    wsId,
    name: title || UNTITLED_CHAT_LABEL,
  })
}
