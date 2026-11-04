import { useEffect } from 'react'
import { chatAttachPathForWorkspace } from '@/lib/workspace-scope-url'
import { subscribeGuarded } from '@/lib/ws/subscribe-guarded'

/**
 * Attaches this window to `chatId` for as long as the pane is mounted. The
 * daemon counts the open socket as one viewer and nothing else: when it closes
 * — pane closed, window gone, phone asleep — the agent carries on.
 */
export function useChatAttachment(wsId: string, chatId: string): void {
  useEffect(() => {
    if (!chatId) return
    return subscribeGuarded(
      () => chatAttachPathForWorkspace(wsId, chatId),
      () => {},
    )
  }, [wsId, chatId])
}
