import { createStore, useStore } from 'zustand'
import type { FeedDelta, FeedFrame } from '@/features/agent/api/chat-rows'
import { applyDelta, applyReady } from '@/features/agent/stores/lib/fold-live'
import { applyRow, emptyFeed, FeedFold } from '@/features/agent/stores/lib/fold-rows'
import type { ChatFeed } from '@/features/agent/stores/lib/transcript-items'

interface ChatStoreState {
  feeds: Record<string, ChatFeed>
}

export const chatStore = createStore<ChatStoreState>()(() => ({ feeds: {} }))

export function useChatStore<T>(selector: (state: ChatStoreState) => T): T {
  return useStore(chatStore, selector)
}

// Rows replayed before `ready` fold into one draft and publish once, at ready.
const drafts = new Map<string, FeedFold>()

function publish(chatId: string, next: ChatFeed) {
  const { feeds } = chatStore.getState()
  if (feeds[chatId] !== next) chatStore.setState({ feeds: { ...feeds, [chatId]: next } })
}

/** Starts a chat's feed state, keeping one that is already there. */
export function openFeed(chatId: string) {
  if (!chatStore.getState().feeds[chatId]) publish(chatId, emptyFeed())
}

export function failFeed(chatId: string, error: string) {
  const feed = chatStore.getState().feeds[chatId]
  if (feed) publish(chatId, { ...feed, status: 'error', error })
}

export function dropFeed(chatId: string) {
  drafts.delete(chatId)
  const { feeds } = chatStore.getState()
  if (!(chatId in feeds)) return
  const { [chatId]: _dropped, ...rest } = feeds
  chatStore.setState({ feeds: rest })
}

/** Applies coalesced deltas in one store write. */
export function applyFeedDeltas(chatId: string, deltas: FeedDelta[]) {
  const feed = chatStore.getState().feeds[chatId]
  if (feed) publish(chatId, deltas.reduce(applyDelta, feed))
}

function draftOf(chatId: string, feed: ChatFeed): FeedFold {
  const draft = new FeedFold(feed)
  drafts.set(chatId, draft)
  return draft
}

/** Applies one frame; false when the chat is gone and its feed must close. */
export function applyFeedFrame(chatId: string, frame: FeedFrame): boolean {
  const feed = chatStore.getState().feeds[chatId] ?? emptyFeed()
  switch (frame.type) {
    case 'row': {
      const draft = drafts.get(chatId)
      if (draft) draft.apply(frame.row)
      else if (feed.status !== 'live') draftOf(chatId, feed).apply(frame.row)
      else publish(chatId, applyRow(feed, frame.row))
      return true
    }
    case 'ready': {
      const draft = drafts.get(chatId)?.finish() ?? feed
      drafts.delete(chatId)
      publish(chatId, applyReady(draft, frame.epoch))
      return true
    }
    case 'delta':
      if (!drafts.has(chatId) && chatStore.getState().feeds[chatId]) {
        publish(chatId, applyDelta(feed, frame.delta))
      }
      return true
    case 'resync': {
      // Behind our seq: the ledger was reset, so rebuild from nothing. Either
      // way the replay that follows folds into a fresh draft of what is published.
      const next = (frame.seq ?? 0) < feed.seq ? emptyFeed() : feed
      publish(chatId, next)
      draftOf(chatId, next)
      return true
    }
    case 'deleted':
      dropFeed(chatId)
      return false
  }
}
