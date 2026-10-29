import { useEffect } from 'react'
import { attachFeed } from '@/features/agent/stores/chat-feed'
import { useChatStore } from '@/features/agent/stores/chat-store'
import {
  liveKey,
  type ChatFeed,
  type TranscriptItem,
  type TurnFacts,
} from '@/features/agent/stores/lib/transcript-items'

const EMPTY_IDS: string[] = []

/** Holds the chat's feed open for as long as the calling view is mounted. */
export function useChatFeed(chatId: string | undefined) {
  useEffect(() => (chatId ? attachFeed(chatId) : undefined), [chatId])
}

const feedOf = (feeds: Record<string, ChatFeed>, chatId: string | undefined) =>
  chatId ? feeds[chatId] : undefined

export function useChatOrder(chatId: string | undefined): string[] {
  return useChatStore((s) => feedOf(s.feeds, chatId)?.order ?? EMPTY_IDS)
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
