import { type RefObject, useEffect, useLayoutEffect } from 'react'
import { loadOlder } from '@/features/agent/stores/chat-feed'
import { useChatStore } from '@/features/agent/stores/chat-store'

/** Pages a chat's older turns in once the reader is at the first loaded row,
 *  keeping their place as the rows land above them. Call before the commit that
 *  lands the view (the anchor's `afterCommit`), so that commit keeps the place. */
export function useOlderTurns(
  chatId: string,
  firstShown: number | undefined,
  scrollRef: RefObject<HTMLElement | null>,
  keepPlace: () => void,
) {
  const from = useChatStore((s) => s.feeds[chatId]?.from ?? 0)
  useLayoutEffect(() => keepPlace(), [from, keepPlace])
  useEffect(() => {
    // The first commit draws row 0 before the anchor lands the view: where the
    // scroller is now says whether the reader is at the top.
    const el = scrollRef.current
    if (from > 0 && firstShown === 0 && el && el.scrollTop <= el.clientHeight) loadOlder(chatId)
  }, [chatId, from, firstShown, scrollRef])
}
