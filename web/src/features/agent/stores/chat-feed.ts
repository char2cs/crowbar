import { holdScope, registerScopeKind, streamOpened, watchScope } from '@/core/stream/stream'
import type { Frame } from '@/core/stream/frames'
import type { FeedDelta, FeedFrame, LedgerRow } from '@/features/agent/api/chat-rows'
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
  release: () => void
  pending: FeedDelta[]
  frame: number | null
}

const attachments = new Map<string, Attachment>()
/** The daemon run the stream's ready named: a feed synced under another drops its live tail. */
let epoch = ''

const scopeOf = (chatId: string) => `chat:${chatId}`

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
  attachment.release()
  if (attachments.get(chatId) === attachment) attachments.delete(chatId)
}

/** The stream's frame as the chat store folds it; a delta or a refusal is not one. */
function feedFrameOf(f: Frame): FeedFrame | undefined {
  switch (f.t) {
    case 'row':
      return { type: 'row', row: f.row as unknown as LedgerRow }
    case 'synced':
      return { type: 'ready', epoch }
    case 'closed':
      return f.reason === 'resync' ? { type: 'resync', seq: f.seq } : undefined
  }
  return undefined
}

function onFrame(chatId: string, attachment: Attachment, f: Frame) {
  if (f.t === 'delta') {
    attachment.pending.push(f.delta as unknown as FeedDelta)
    attachment.frame ??= requestAnimationFrame(() => {
      attachment.frame = null
      flush(chatId, attachment)
    })
    return
  }
  flush(chatId, attachment)
  const frame = feedFrameOf(f)
  if (frame) applyFeedFrame(chatId, frame)
}

registerScopeKind('chat', {
  since: (chatId) => chatStore.getState().feeds[chatId]?.seq ?? 0,
  epoch: (next) => {
    epoch = next
  },
  frame(f) {
    if (!('scope' in f)) return
    const chatId = f.scope.slice('chat:'.length)
    const attachment = attachments.get(chatId)
    if (attachment) onFrame(chatId, attachment, f)
  },
})

/** Holds the chat's ledger scope while anything attached to it is mounted; the
 *  last detach drops its state, and the next attach replays from since=0. */
export function attachFeed(chatId: string): () => void {
  let attachment = attachments.get(chatId)
  if (!attachment) {
    const scope = scopeOf(chatId)
    const fresh: Attachment = { refs: 0, release: () => {}, pending: [], frame: null }
    attachment = fresh
    attachments.set(chatId, fresh)
    openFeed(chatId)
    // A deleted chat's scope is closed as gone, an unknown one's as refused.
    const stopWatch = watchScope(scope, {
      refused(problem) {
        if (attachments.get(chatId) !== fresh) return
        if (problem) return failFeed(chatId, problem.message)
        release(chatId, fresh)
        dropFeed(chatId)
      },
    })
    const letGo = holdScope(scope)
    fresh.release = () => {
      stopWatch()
      letGo()
    }
    streamOpened().catch((error: unknown) => {
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
