import { createStore, useStore } from 'zustand'
import type { FeedDelta, FeedFrame } from '@/features/agent/api/chat-rows'
import { applyReady } from '@/features/agent/stores/lib/fold-live'
import { applyLive, applyRow, emptyFeed, FeedFold } from '@/features/agent/stores/lib/fold-rows'
import type { OutboxItem } from '@/features/agent/stores/lib/outbox'
import type { ChatFeed } from '@/features/agent/stores/lib/transcript-items'

/** What the client keeps per chat beyond its entity: the feed it folds and its unsent
 *  prompts. The chat itself lives in the entity store. */
export interface ChatStoreState {
  feeds: Record<string, ChatFeed>
  /** Prompts with no row yet, by chat; written only by chat-outbox. */
  outbox: Record<string, OutboxItem[]>
}

export const chatStore = createStore<ChatStoreState>()(() => ({
  feeds: {},
  outbox: {},
}))

export function useChatStore<T>(selector: (state: ChatStoreState) => T): T {
  return useStore(chatStore, selector)
}

// Rows replayed before `ready` fold into one draft and publish once, at ready.
const drafts = new Map<string, FeedFold>()

function publish(chatId: string, next: ChatFeed) {
  const { feeds } = chatStore.getState()
  if (feeds[chatId] !== next) chatStore.setState({ feeds: { ...feeds, [chatId]: next } })
}

/** Stop was pressed: the running turn reads as stopping until its end lands. */
export function setStopping(chatId: string, stopping: boolean) {
  const feed = chatStore.getState().feeds[chatId]
  const turn = stopping ? (feed?.facts.running?.turnId ?? null) : null
  if (!feed || feed.facts.stopping === turn) return
  publish(chatId, { ...feed, facts: { ...feed.facts, stopping: turn } })
}

/** Starts a chat's feed state, keeping one that is already there. */
export function openFeed(chatId: string) {
  if (!chatStore.getState().feeds[chatId]) publish(chatId, emptyFeed())
}

/** Marks the feed failed; `error` is the daemon's word for why, when it gave one. */
export function failFeed(chatId: string, error?: string) {
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
  if (feed) publish(chatId, deltas.reduce(applyLive, feed))
}

/** Replays the chat afresh behind what is shown: the feed it publishes at ready
 *  replaces this one, so a wider window lands in one write. */
export function refoldFeed(chatId: string) {
  draftOf(chatId, emptyFeed())
}

function draftOf(chatId: string, feed: ChatFeed): FeedFold {
  const draft = new FeedFold(feed)
  drafts.set(chatId, draft)
  return draft
}

/** Applies one frame of the chat's ledger scope. */
export function applyFeedFrame(chatId: string, frame: FeedFrame) {
  const feed = chatStore.getState().feeds[chatId] ?? emptyFeed()
  switch (frame.type) {
    case 'row': {
      let draft = drafts.get(chatId)
      if (!draft && (feed.status !== 'live' || frame.from)) draft = draftOf(chatId, feed)
      if (!draft) return publish(chatId, applyRow(feed, frame.row))
      if (frame.from) draft.startAt(frame.from)
      draft.apply(frame.row)
      return
    }
    case 'ready': {
      const draft = drafts.get(chatId)?.finish() ?? feed
      drafts.delete(chatId)
      publish(chatId, applyReady(draft, frame.epoch))
      return
    }
    case 'delta':
      if (!drafts.has(chatId) && chatStore.getState().feeds[chatId]) {
        publish(chatId, applyLive(feed, frame.delta))
      }
      return
    case 'resync': {
      // Behind our seq: the ledger was reset, so rebuild from nothing. Either
      // way the replay that follows folds into a fresh draft of what is published.
      const next = (frame.seq ?? 0) < feed.seq ? emptyFeed() : feed
      publish(chatId, next)
      draftOf(chatId, next)
    }
  }
}
