import { useEffect } from 'react'
import { attachFeed } from '@/features/agent/stores/chat-feed'
import { useChatHost } from '@/lib/host-resolve'
import { useChatStore } from '@/features/agent/stores/chat-store'
import {
  liveKey,
  type ChatFeed,
  type TranscriptItem,
  type TurnFacts,
} from '@/features/agent/stores/lib/transcript-items'
import { displayRows, type DisplayRow } from '@/features/agent/transcript/lib/tool-runs'

const EMPTY_IDS: string[] = []

/** Holds the chat's feed open, on its host once that resolves, for as long as the calling
 *  view is mounted. */
export function useChatFeed(chatId: string | undefined) {
  const hostId = useChatHost(chatId ?? '')
  useEffect(() => (chatId && hostId ? attachFeed(hostId, chatId) : undefined), [chatId, hostId])
}

const feedOf = (feeds: Record<string, ChatFeed>, chatId: string | undefined) =>
  chatId ? feeds[chatId] : undefined

export function useChatOrder(chatId: string | undefined): string[] {
  return useChatStore((s) => feedOf(s.feeds, chatId)?.order ?? EMPTY_IDS)
}

const NO_ROWS: DisplayRow[] = []
// An order array is replaced whenever an item joins it, and the grouping reads
// only facts an item never changes, so one result per order array is exact.
const drawn = new WeakMap<string[], DisplayRow[]>()

/** The main-scope order as drawn: hidden calls left out, read-only runs folded. */
export function useDisplayRows(chatId: string | undefined): DisplayRow[] {
  return useChatStore((s) => {
    const feed = feedOf(s.feeds, chatId)
    if (!feed) return NO_ROWS
    let rows = drawn.get(feed.order)
    if (!rows) {
      rows = displayRows(feed.order, feed.items)
      drawn.set(feed.order, rows)
    }
    return rows
  })
}

export function useLiveTail(chatId: string | undefined): string[] {
  return useChatStore((s) => feedOf(s.feeds, chatId)?.liveTail ?? EMPTY_IDS)
}

export function useChatItems(
  chatId: string | undefined,
): Record<string, TranscriptItem> | undefined {
  return useChatStore((s) => feedOf(s.feeds, chatId)?.items)
}

export function useChatItem<T extends TranscriptItem = TranscriptItem>(
  chatId: string | undefined,
  id: string,
): T | undefined {
  return useChatStore((s) => feedOf(s.feeds, chatId)?.items[id] as T | undefined)
}

/** The live text of one delta stream: `answer`/`reasoning` by message id,
 *  `tool_output`/`diff` by tool id. */
export function useLiveText(
  chatId: string | undefined,
  id: string,
  stream: 'answer' | 'reasoning' | 'tool_output' | 'diff',
): string | undefined {
  return useChatStore((s) => feedOf(s.feeds, chatId)?.live[liveKey(stream, id)])
}

export function useLiveReasoning(chatId: string | undefined): string | undefined {
  return useChatStore((s) => {
    const feed = feedOf(s.feeds, chatId)
    return feed?.liveReasoning ? feed.live[feed.liveReasoning] : undefined
  })
}

export function useTurnFacts(chatId: string | undefined): TurnFacts | undefined {
  return useChatStore((s) => feedOf(s.feeds, chatId)?.facts)
}

export function useFeedStatus(chatId: string | undefined): ChatFeed['status'] | undefined {
  return useChatStore((s) => feedOf(s.feeds, chatId)?.status)
}

/** Why the daemon refused the chat's feed, when it said. */
export function useFeedError(chatId: string | undefined): string | undefined {
  return useChatStore((s) => feedOf(s.feeds, chatId)?.error)
}
