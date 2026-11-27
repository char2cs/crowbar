import { type RefObject, useEffect, useLayoutEffect } from 'react'
import { nearsLoadedTop } from '@/features/agent/lib/feed-window'
import { loadOlder } from '@/features/agent/stores/chat-feed'
import { useChatStore } from '@/features/agent/stores/chat-store'

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
    const load = () => {
      if (nearsLoadedTop(el.scrollTop, el.clientHeight)) void loadOlder(chatId)
    }
    if (el.scrollHeight <= el.clientHeight) load()
    el.addEventListener('scrollend', load)
    return () => el.removeEventListener('scrollend', load)
  }, [chatId, from, scrollRef])
}
