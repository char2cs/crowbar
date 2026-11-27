import { useEffect } from 'react'
import { attachFeed } from '@/features/agent/stores/chat-feed'
import { useFeedHost } from '@/features/hosts/lib/host-resolve-cached'
import { useChatStore } from '@/features/agent/stores/chat-store'
import {
  liveKey,
  type ChatFeed,
  type TranscriptItem,
} from '@/features/agent/stores/lib/transcript-items'
import type { ItemMap } from '@/features/agent/stores/lib/item-map'
import { displayRows, type DisplayRow } from '@/features/agent/transcript/lib/tool-runs'

const EMPTY_IDS: string[] = []

export function useChatFeed(chatId: string | undefined) {
  const hostId = useFeedHost(chatId ?? '')
  useEffect(() => (chatId && hostId ? attachFeed(hostId, chatId) : undefined), [chatId, hostId])
}

const feedOf = (feeds: Record<string, ChatFeed>, chatId: string | undefined) =>
  chatId ? feeds[chatId] : undefined

const NO_ROWS: DisplayRow[] = []
const drawn = new WeakMap<string[], DisplayRow[]>()
const lastDrawn = new Map<string, DisplayRow[]>()

export function useDisplayRows(chatId: string | undefined): DisplayRow[] {
  return useChatStore((s) => {
    const feed = feedOf(s.feeds, chatId)
    if (!feed) return NO_ROWS
    let rows = drawn.get(feed.order)
    if (!rows) {
      rows = displayRows(feed.order, feed.items, lastDrawn.get(chatId!))
      drawn.set(feed.order, rows)
      lastDrawn.set(chatId!, rows)
    }
    return rows
  })
}

export function useLiveTail(chatId: string | undefined): string[] {
  return useChatStore((s) => feedOf(s.feeds, chatId)?.liveTail ?? EMPTY_IDS)
}

export function useChatItems(chatId: string | undefined): ItemMap | undefined {
  return useChatStore((s) => feedOf(s.feeds, chatId)?.items)
}

export function useChatItem<T extends TranscriptItem = TranscriptItem>(
  chatId: string | undefined,
  id: string,
): T | undefined {
  return useChatStore((s) => feedOf(s.feeds, chatId)?.items.get(id) as T | undefined)
}

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

export function useFeedStatus(chatId: string | undefined): ChatFeed['status'] | undefined {
  return useChatStore((s) => feedOf(s.feeds, chatId)?.status)
}

export function useFeedError(chatId: string | undefined): string | undefined {
  return useChatStore((s) => feedOf(s.feeds, chatId)?.error)
}
