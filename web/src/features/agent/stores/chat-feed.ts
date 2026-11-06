import { applyEntityFrame } from '@/core/entities/entity-store'
import {
  holdScope,
  registerScopeKind,
  resubscribe,
  streamOpened,
  watchScope,
} from '@/core/stream/stream'
import type { Frame } from '@/core/stream/frames'
import type { FeedDelta, FeedFrame, LedgerRow } from '@/features/agent/api/chat-rows'
import {
  applyFeedDeltas,
  applyFeedFrame,
  chatStore,
  dropFeed,
  failFeed,
  openFeed,
  refoldFeed,
} from '@/features/agent/stores/chat-store'
import { streamFailureWords } from '@/features/hosts/lib/host-failure-notice'

interface Attachment {
  hostId: string
  refs: number
  release: () => void
  pending: FeedDelta[]
  frame: number | null
  /** A synced disagreed with the last applied row and the scope was asked for again. */
  reasked: boolean
  /** How many of the newest turns a fresh replay opens on. */
  turns: number
  /** A wider window is replaying: the next sub asks for it, not for rows after the head. */
  widening: boolean
}

/** A long chat opens on its newest turns; older ones replay as the reader reaches them. */
const OPEN_TURNS = 30
const OLDER_TURNS = 60

const attachments = new Map<string, Attachment>()
/** host → the daemon run its stream's ready named: a feed synced under another drops its live tail. */
const epochs = new Map<string, string>()

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
function feedFrameOf(hostId: string, f: Frame): FeedFrame | undefined {
  switch (f.t) {
    case 'row':
      return { type: 'row', row: f.row as unknown as LedgerRow, from: f.from }
    case 'synced':
      return { type: 'ready', epoch: epochs.get(hostId) ?? '' }
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
  const frame = feedFrameOf(attachment.hostId, f)
  if (frame) applyFeedFrame(chatId, frame)
  if (f.t !== 'synced') return
  attachment.widening = false
  checkHead(chatId, attachment, f.seq)
}

/** A synced names the ledger's head: one past the last applied row means rows
 *  were dropped, so the scope is asked for again once, then the feed fails. */
function checkHead(chatId: string, attachment: Attachment, head: number | undefined) {
  const seq = chatStore.getState().feeds[chatId]?.seq
  if (head === undefined || seq === undefined || head === seq) {
    attachment.reasked = false
    return
  }
  if (attachment.reasked) return failFeed(chatId, 'This chat’s history did not line up')
  attachment.reasked = true
  resubscribe(attachment.hostId, scopeOf(chatId))
}

registerScopeKind('chat', {
  // Rows after the head the feed holds, or (a negative since) a window of the newest turns.
  since: (hostId, chatId) => {
    const attachment = attachments.get(chatId)
    const held = attachment?.hostId === hostId && !attachment.widening
    return (held && chatStore.getState().feeds[chatId]?.seq) || -(attachment?.turns ?? OPEN_TURNS)
  },
  epoch: (hostId, next) => {
    epochs.set(hostId, next)
  },
  forget: (hostId) => {
    epochs.delete(hostId)
  },
  frame(hostId, f) {
    if (!('scope' in f)) return
    const chatId = f.scope.slice('chat:'.length)
    const attachment = attachments.get(chatId)
    if (attachment?.hostId !== hostId) return
    // The chat's usage report rides its scope as an entity the entity store holds.
    if (f.t === 'entity') return applyEntityFrame(hostId, f)
    onFrame(chatId, attachment, f)
  },
})

/** Opens the chat on more of its older turns: a wider window replays behind the
 *  shown feed and replaces it at ready. No-op while one replays or none is older. */
export function loadOlder(chatId: string) {
  const attachment = attachments.get(chatId)
  if (!attachment || attachment.widening || !chatStore.getState().feeds[chatId]?.from) return
  attachment.turns += OLDER_TURNS
  attachment.widening = true
  refoldFeed(chatId)
  resubscribe(attachment.hostId, scopeOf(chatId))
}

/** Holds the chat's ledger scope on `hostId` while anything attached to it is mounted; the
 *  last detach drops its state, and the next attach replays its newest turns afresh. */
export function attachFeed(hostId: string, chatId: string): () => void {
  let attachment = attachments.get(chatId)
  if (!attachment) {
    const scope = scopeOf(chatId)
    const fresh: Attachment = {
      hostId,
      refs: 0,
      release: () => {},
      pending: [],
      frame: null,
      reasked: false,
      turns: OPEN_TURNS,
      widening: false,
    }
    attachment = fresh
    attachments.set(chatId, fresh)
    openFeed(chatId)
    // A deleted chat's scope is closed as gone, an unknown one's as refused.
    const stopWatch = watchScope(hostId, scope, {
      refused(problem) {
        if (attachments.get(chatId) !== fresh) return
        if (problem) return failFeed(chatId, problem.message)
        release(chatId, fresh)
        dropFeed(chatId)
      },
    })
    const letGo = holdScope(hostId, scope)
    fresh.release = () => {
      stopWatch()
      letGo()
    }
    // An unreachable stream keeps retrying; the feed says why when its host is the reason.
    streamOpened(hostId).catch((err: unknown) => {
      if (attachments.get(chatId) === fresh) failFeed(chatId, streamFailureWords(hostId, err))
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
