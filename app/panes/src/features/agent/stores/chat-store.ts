import { createStore, useStore } from 'zustand'
import { forgetWithChat } from '@/features/agent/stores/chat-lifecycle'
import type { ChatPresentation } from '@/features/settings/lib/chat-presentation'
import type { FeedFrame, LedgerRow } from '@/features/agent/api/chat-rows'
import { applyReady } from '@/features/agent/stores/lib/fold-live'
import { applyLive, applyRow, emptyFeed, FeedFold } from '@/features/agent/stores/lib/fold-rows'
import { fillFull, prependOlder } from '@/features/agent/stores/lib/older-rows'
import type { OutboxItem } from '@/features/agent/stores/lib/outbox'
import type { ChatFeed } from '@/features/agent/stores/lib/transcript-items'

export interface ChatStoreState {
  feeds: Record<string, ChatFeed>
  outbox: Record<string, OutboxItem[]>
  presentations: Record<string, ChatPresentation>
}

export const chatStore = createStore<ChatStoreState>()(() => ({
  feeds: {},
  outbox: {},
  presentations: {},
}))

export function useChatStore<T>(selector: (state: ChatStoreState) => T): T {
  return useStore(chatStore, selector)
}

const drafts = new Map<string, FeedFold>()

function publish(chatId: string, next: ChatFeed) {
  const { feeds } = chatStore.getState()
  if (feeds[chatId] !== next) chatStore.setState({ feeds: { ...feeds, [chatId]: next } })
}

export function setStopping(chatId: string, stopping: boolean) {
  const feed = chatStore.getState().feeds[chatId]
  const turn = stopping ? (feed?.facts.running?.turnId ?? null) : null
  if (!feed || feed.facts.stopping === turn) return
  publish(chatId, { ...feed, facts: { ...feed.facts, stopping: turn } })
}

export function openFeed(chatId: string) {
  if (!chatStore.getState().feeds[chatId]) publish(chatId, emptyFeed())
}

export function showStoredFeed(chatId: string, stored: ChatFeed): boolean {
  const feed = chatStore.getState().feeds[chatId]
  if (!feed || feed.seq !== 0 || feed.status !== 'connecting' || drafts.has(chatId)) return false
  publish(chatId, stored)
  return true
}

export function prependRows(chatId: string, heldFrom: number, rows: LedgerRow[], from: number) {
  const feed = chatStore.getState().feeds[chatId]
  if (!feed || drafts.has(chatId) || feed.from !== heldFrom || from >= heldFrom) return false
  publish(chatId, prependOlder(feed, rows, from))
  return true
}

export function fillRow(chatId: string, id: string, row: LedgerRow) {
  const feed = chatStore.getState().feeds[chatId]
  if (feed) publish(chatId, fillFull(feed, id, row))
}

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
forgetWithChat(dropFeed)

export type LiveFrame = Extract<FeedFrame, { type: 'row' | 'delta' }>

export function foldsLive(chatId: string): boolean {
  return chatStore.getState().feeds[chatId]?.status === 'live' && !drafts.has(chatId)
}

export function applyFeedBatch(chatId: string, frames: LiveFrame[]) {
  const feed = chatStore.getState().feeds[chatId]
  if (!feed) return
  let next = feed
  for (const frame of frames) {
    next = frame.type === 'row' ? applyRow(next, frame.row) : applyLive(next, frame.delta)
  }
  publish(chatId, next)
}

export function refoldFeed(chatId: string) {
  draftOf(chatId, emptyFeed())
}

function draftOf(chatId: string, feed: ChatFeed): FeedFold {
  const draft = new FeedFold(feed)
  drafts.set(chatId, draft)
  return draft
}

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
      const next = (frame.seq ?? 0) < feed.seq ? emptyFeed() : feed
      publish(chatId, next)
      draftOf(chatId, next)
    }
  }
}
