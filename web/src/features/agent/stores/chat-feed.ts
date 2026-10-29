import { wsManager } from '@/lib/ws/manager'
import type { FeedDelta, FeedFrame } from '@/features/agent/api/chat-rows'
import {
  applyFeedDeltas,
  applyFeedFrame,
  chatStore,
  dropFeed,
  failFeed,
  openFeed,
} from '@/features/agent/stores/chat-store'

interface Attachment {
  refs: number
  unsubscribe: () => void
  pending: FeedDelta[]
  frame: number | null
}

const attachments = new Map<string, Attachment>()

const feedEndpoint = (chatId: string) => `/v0/chats/${encodeURIComponent(chatId)}/feed`

function isFrame(data: unknown): data is FeedFrame {
  return typeof data === 'object' && data !== null && 'type' in data
}

function flush(chatId: string, attachment: Attachment) {
  if (attachment.frame !== null) cancelAnimationFrame(attachment.frame)
  attachment.frame = null
  if (attachment.pending.length === 0) return
  const deltas = attachment.pending
  attachment.pending = []
  applyFeedDeltas(chatId, deltas)
}

function release(chatId: string, attachment: Attachment) {
  if (attachment.frame !== null) cancelAnimationFrame(attachment.frame)
  attachment.unsubscribe()
  if (attachments.get(chatId) === attachment) attachments.delete(chatId)
}

/** Holds the chat's feed open while anything attached to it is mounted; the
 *  last detach drops its state, and the next attach replays from since=0. */
export function attachFeed(chatId: string): () => void {
  let attachment = attachments.get(chatId)
  if (!attachment) {
    const endpoint = feedEndpoint(chatId)
    const fresh: Attachment = { refs: 0, unsubscribe: () => {}, pending: [], frame: null }
    attachment = fresh
    attachments.set(chatId, fresh)
    openFeed(chatId)
    const onData = (data: unknown) => {
      if (!isFrame(data)) return // the manager's { reconnected: true }: our replay is `since`
      if (data.type === 'delta') {
        fresh.pending.push(data.delta)
        fresh.frame ??= requestAnimationFrame(() => {
          fresh.frame = null
          flush(chatId, fresh)
        })
        return
      }
      flush(chatId, fresh)
      if (!applyFeedFrame(chatId, data)) release(chatId, fresh)
    }
    const url = () => `${endpoint}?since=${chatStore.getState().feeds[chatId]?.seq ?? 0}`
    fresh.unsubscribe = wsManager.subscribe(endpoint, onData, url)
    wsManager.opened(endpoint).catch((error: unknown) => {
      if (attachments.get(chatId) === fresh) failFeed(chatId, String(error))
    })
  }
  const held = attachment
  held.refs += 1
  let detached = false
  return () => {
    if (detached) return
    detached = true
    held.refs -= 1
    if (held.refs > 0 || attachments.get(chatId) !== held) return
    release(chatId, held)
    dropFeed(chatId)
  }
}
