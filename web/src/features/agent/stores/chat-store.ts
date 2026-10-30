import { createStore, useStore } from 'zustand'
import type { AgentChat, AgentTelemetry } from '@/features/agent/api/agent-chat-reads'
import type { FeedDelta, FeedFrame } from '@/features/agent/api/chat-rows'
import { applyDelta, applyReady } from '@/features/agent/stores/lib/fold-live'
import { applyRow, emptyFeed, FeedFold } from '@/features/agent/stores/lib/fold-rows'
import {
  applySnapshot,
  reduceChatFrame,
  removeChat,
  type ChatEntities,
  type ChatFrame,
  type ChatFrameOutcome,
} from '@/features/agent/stores/lib/reduce-chat-frame'
import type { OutboxItem } from '@/features/agent/stores/lib/outbox'
import type { ChatFeed } from '@/features/agent/stores/lib/transcript-items'

/** Whether a workspace's chat list has landed; set by its lifecycle scope. */
export type ChatScopeStatus = 'seeding' | 'ready' | 'error'

export interface ChatStoreState extends ChatEntities {
  feeds: Record<string, ChatFeed>
  scopes: Record<string, ChatScopeStatus>
  telemetry: Record<string, AgentTelemetry | null>
  /** Prompts with no row yet, by chat; written only by chat-outbox. */
  outbox: Record<string, OutboxItem[]>
}

export const chatStore = createStore<ChatStoreState>()(() => ({
  feeds: {},
  chats: {},
  byWorkspace: {},
  byRunner: {},
  scopes: {},
  telemetry: {},
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

function commit(result: { next: ChatEntities; outcome: ChatFrameOutcome }): ChatFrameOutcome {
  const { next } = result
  if (next.chats !== chatStore.getState().chats) {
    chatStore.setState({
      chats: next.chats,
      byWorkspace: next.byWorkspace,
      byRunner: next.byRunner,
    })
  }
  return result.outcome
}

/** Applies a chat snapshot from a GET under the version rule. */
export const applyChat = (chat: AgentChat) => commit(applySnapshot(chatStore.getState(), chat))

/** Applies a lifecycle frame: the one writer of chat entities. */
export const applyChatFrame = (frame: ChatFrame) =>
  commit(reduceChatFrame(chatStore.getState(), frame))

export function removeChats(chatIds: readonly string[]) {
  const state = chatStore.getState()
  let entities: ChatEntities = state
  const telemetry = { ...state.telemetry }
  for (const id of chatIds) {
    entities = removeChat(entities, id)
    delete telemetry[id]
  }
  if (entities === state) return
  const { chats, byWorkspace, byRunner } = entities
  chatStore.setState({ chats, byWorkspace, byRunner, telemetry })
}

/** The selection endpoint answers 202 with no body and no frame, so the
 *  accepted pair is written here or the picker paints the old one. */
export function setChatSelection(chatId: string, model: string, effort: string) {
  const chat = chatStore.getState().chats[chatId]
  if (!chat) return
  chatStore.setState((s) => ({ chats: { ...s.chats, [chatId]: { ...chat, model, effort } } }))
}

export function setChatScope(wsId: string, status: ChatScopeStatus | null) {
  const { scopes } = chatStore.getState()
  if ((scopes[wsId] ?? null) === status) return
  const { [wsId]: _was, ...rest } = scopes
  chatStore.setState({ scopes: status ? { ...rest, [wsId]: status } : rest })
}

const observed = (report: AgentTelemetry | null | undefined) =>
  report ? Date.parse(report.observedAt) : -Infinity

/** A null report clears the gauge; a report older than the held one is dropped. */
export function setChatTelemetry(chatId: string, report: AgentTelemetry | null) {
  const held = chatStore.getState().telemetry[chatId]
  if (report && observed(report) < observed(held)) return
  chatStore.setState((s) => ({ telemetry: { ...s.telemetry, [chatId]: report } }))
}
