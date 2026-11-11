import { useShallow } from 'zustand/react/shallow'
import { useChatStore } from '@/features/agent/stores/chat-store'
import { shellsOf } from '@/features/agent/stores/lib/fold-shells'
import type { ShellItem } from '@/features/agent/stores/lib/transcript-items'

const NONE: ShellItem[] = []

/** The background commands the chat's main agent started, oldest first, each
 *  running or how it ended. */
export function useChatShells(chatId: string): ShellItem[] {
  return useChatStore(
    useShallow((s) => {
      const feed = s.feeds[chatId]
      return feed ? shellsOf(feed) : NONE
    }),
  )
}
