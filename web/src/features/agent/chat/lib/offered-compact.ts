import { compactChat } from '@/features/agent/api/agent-chat-commands'
import { report } from '@/lib/errors/report'

/** Compact, offered only while the agent is idle; a turn that starts before the
 *  click still gets the refusal said, not thrown at the page. */
export function offeredCompact(wsId: string, chatId: string, idle: boolean) {
  if (!idle) return undefined
  return () =>
    void compactChat(wsId, chatId).catch((error: unknown) =>
      report(error, { action: 'Couldn’t compact the chat' }),
    )
}
