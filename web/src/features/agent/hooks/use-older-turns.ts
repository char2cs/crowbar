import { useEffect, useLayoutEffect } from 'react'
import { loadOlder } from '@/features/agent/stores/chat-feed'
import { useChatStore } from '@/features/agent/stores/chat-store'

/** Pages a chat's older turns in once the reader reaches the first loaded row,
 *  keeping their place as the rows land above them. Call before the commit that
 *  lands the view (the anchor's `afterCommit`), so that commit keeps the place. */
export function useOlderTurns(
  chatId: string,
  firstShown: number | undefined,
  keepPlace: () => void,
) {
  const from = useChatStore((s) => s.feeds[chatId]?.from ?? 0)
  useLayoutEffect(() => keepPlace(), [from, keepPlace])
  useEffect(() => {
    if (from > 0 && firstShown === 0) loadOlder(chatId)
  }, [chatId, from, firstShown])
}
