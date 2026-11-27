import { readSelect, selectAgentChat } from '@/core/entities/select'
import type { PaneGroup } from '@/features/panes/types/pane'

export function paneChatWorkspaceId(
  pane: Pick<PaneGroup, 'chatId' | 'workspaceId'> | undefined,
): string | null {
  if (!pane?.chatId) return null
  const chatId = pane.chatId
  return readSelect((s) => selectAgentChat(s, chatId)?.workspaceId) || pane.workspaceId || null
}
