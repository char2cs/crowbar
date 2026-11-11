import { readSelect, selectAgentChat } from '@/core/entities/select'
import type { PaneGroup } from '@/features/panes/types/pane'

/** Where a pane's chat runs now: the chat's own workspace, which branch-out and a move change,
 *  else the pane's record of where it opened until the chat is known; null for a chatless pane.
 *  For event handlers and store actions; render code uses `usePaneChatWorkspaceId`. */
export function paneChatWorkspaceId(
  pane: Pick<PaneGroup, 'chatId' | 'workspaceId'> | undefined,
): string | null {
  if (!pane?.chatId) return null
  const chatId = pane.chatId
  return readSelect((s) => selectAgentChat(s, chatId)?.workspaceId) || pane.workspaceId || null
}
