import { compactChat } from '@/features/agent/api/agent-chat-commands'
import { report } from '@/lib/errors/report'

export function offeredCompact(wsId: string, chatId: string, idle: boolean) {
  if (!idle) return undefined
  return () =>
    void compactChat(wsId, chatId).catch((error: unknown) =>
      report(error, { action: 'Couldn’t compact the chat' }),
    )
}
