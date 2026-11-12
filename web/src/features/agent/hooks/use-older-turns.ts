import { type RefObject, useEffect, useLayoutEffect } from 'react'
import { loadOlder } from '@/features/agent/stores/chat-feed'
import { useChatStore } from '@/features/agent/stores/chat-store'

/** Pages older turns in once the view rests within two viewports of the first loaded row (a write
 *  mid-momentum would cancel it), keeping the reader's place. Call before the anchor's commit. */
export function useOlderTurns(
  chatId: string,
  scrollRef: RefObject<HTMLElement | null>,
  keepPlace: () => void,
) {
  const from = useChatStore((s) => s.feeds[chatId]?.from ?? 0)
  useLayoutEffect(() => keepPlace(), [from, keepPlace])
  useEffect(() => {
    const el = scrollRef.current
    if (from === 0 || !el) return
    const load = () => el.scrollTop <= el.clientHeight * 2 && loadOlder(chatId)
    load()
    el.addEventListener('scrollend', load)
    return () => el.removeEventListener('scrollend', load)
  }, [chatId, from, scrollRef])
}
