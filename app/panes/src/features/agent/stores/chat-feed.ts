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
  applyFeedBatch,
  applyFeedFrame,
  chatStore,
  dropFeed,
  failFeed,
  foldsLive,
  openFeed,
  refoldFeed,
  showStoredFeed,
  type LiveFrame,
} from '@/features/agent/stores/chat-store'
import {
  forgetChatFeeds,
  forgetFeed,
  forgetHostFeeds,
  loadFeed,
  saveFeed,
} from '@/features/agent/stores/lib/feed-cache'
import type { ChatFeed } from '@/features/agent/stores/lib/transcript-items'
import { forgetWithChat } from '@/features/agent/stores/chat-lifecycle'
import { streamFailureWords } from '@/features/hosts/lib/host-failure-notice'

interface Attachment {
  hostId: string
  refs: number
  release: () => void
  /** Live rows and deltas since the last animation frame, applied there in one write. */
  pending: LiveFrame[]
  frame: number | null
  /** A synced disagreed with the last applied row and the scope was asked for again. */
  reasked: boolean
  /** About how many of the newest rows a fresh replay opens on, in whole turns. */
  rows: number
  /** A fresh window is replaying: the next sub asks for it, not for rows after the head. */
  widening: boolean
  /** The shown feed came from the stored copy and the daemon has not confirmed it yet. */
  cached: boolean
}

/** A long chat opens on its newest rows; older ones replay as the reader reaches them.
 *  Rows, not turns: one turn of a heavy chat can hold a thousand of them. */
const OPEN_ROWS = 1500
const OLDER_ROWS = 3000

/** How many chats keep their folded feed after their last view detaches. */
const KEPT_FEEDS = 8

const attachments = new Map<string, Attachment>()
/** chat → the host of a feed kept with no view, oldest first. */
const kept = new Map<string, string>()
/** host → the daemon run its stream's ready named: a feed synced under another drops its live tail. */
const epochs = new Map<string, string>()

const scopeOf = (chatId: string) => `chat:${chatId}`

function flush(chatId: string, attachment: Attachment) {
  if (attachment.frame !== null) cancelAnimationFrame(attachment.frame)
  attachment.frame = null
  if (attachment.pending.length === 0) return
  const frames = attachment.pending
  attachment.pending = []
  applyFeedBatch(chatId, frames)
}

/** Writes the shown feed to the cache, when it is a settled copy of the daemon's. */
function persist(chatId: string, attachment: Attachment) {
  const feed = chatStore.getState().feeds[chatId]
  if (feed?.status === 'live' && feed.seq > 0 && !attachment.cached) {
    saveFeed(attachment.hostId, chatId, feed)
  }
}

// No frame callback runs in a hidden tab: what is queued applies as the tab hides, and is saved there.
if (typeof document !== 'undefined') {
  const flushAll = () => {
    for (const [chatId, attachment] of attachments) {
      flush(chatId, attachment)
      persist(chatId, attachment)
    }
  }
  document.addEventListener('visibilitychange', () => document.hidden && flushAll())
  window.addEventListener('pagehide', flushAll)
}

// Each save copies the feed, so a running turn's rows wait for its end: a replay's ready, the
// turn's end, a hide and the last detach write it, and a crash costs the daemon a short resume.
chatStore.subscribe((state, prev) => {
  for (const [chatId, attachment] of attachments) {
    const next = state.feeds[chatId]
    const before = prev.feeds[chatId]
    if (!next || next === before || next.facts.running) continue
    if (next.seq !== before?.seq || next.items !== before.items) persist(chatId, attachment)
  }
})

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

/** A live row or delta, queued for the next animation frame; anything else, or any
 *  frame of a replay, applies at once behind what is queued. */
function liveFrameOf(chatId: string, f: Frame): LiveFrame | undefined {
  if (f.t === 'delta') return { type: 'delta', delta: f.delta as unknown as FeedDelta }
  if (f.t !== 'row' || f.from !== undefined || !foldsLive(chatId)) return undefined
  return { type: 'row', row: f.row as unknown as LedgerRow }
}

function onFrame(chatId: string, attachment: Attachment, f: Frame) {
  if (attachment.cached && f.t === 'closed' && f.reason === 'resync') {
    return reconcile(chatId, attachment, false)
  }
  const live = liveFrameOf(chatId, f)
  if (live && !document.hidden) {
    attachment.pending.push(live)
    attachment.frame ??= requestAnimationFrame(() => {
      attachment.frame = null
      flush(chatId, attachment)
    })
    return
  }
  flush(chatId, attachment)
  const frame = live ?? feedFrameOf(attachment.hostId, f)
  if (frame) applyFeedFrame(chatId, frame)
  if (f.t !== 'synced') return
  attachment.widening = false
  checkHead(chatId, attachment, f.seq)
  persist(chatId, attachment)
}

/** A synced names the ledger's head: one past the last applied row means rows
 *  were dropped, so the scope is asked for again once, then the feed fails. */
function checkHead(chatId: string, attachment: Attachment, head: number | undefined) {
  const seq = chatStore.getState().feeds[chatId]?.seq
  if (head === undefined || seq === undefined || head === seq) {
    attachment.reasked = false
    attachment.cached = false
    return
  }
  if (attachment.cached) return reconcile(chatId, attachment, true)
  if (attachment.reasked) return failFeed(chatId, 'This chat’s history did not line up')
  attachment.reasked = true
  resubscribe(attachment.hostId, scopeOf(chatId))
}

/** The stored copy does not line up with the daemon's ledger: it is forgotten, and a fresh
 *  window replays behind it and replaces it at ready in one write, so nothing blinks. */
function reconcile(chatId: string, attachment: Attachment, ask: boolean) {
  attachment.cached = false
  attachment.widening = true
  void forgetFeed(attachment.hostId, chatId)
  refoldFeed(chatId)
  if (ask) resubscribe(attachment.hostId, scopeOf(chatId))
}

/** Shows what the cache holds for a chat nothing has filled yet; the scope is held after, so its
 *  one sub resumes from the cursor (or asks for a window when the stored run is not the daemon's). */
function showStored(chatId: string, attachment: Attachment, stored: ChatFeed | undefined) {
  if (!stored || !showStoredFeed(chatId, stored)) return
  attachment.cached = true
  const run = epochs.get(attachment.hostId)
  if (run !== undefined && run !== stored.epoch) reconcile(chatId, attachment, false)
}

/** Drops the kept feeds of `hostId`: their rows belong to a ledger this client no longer follows. */
function dropKeptOn(hostId: string) {
  for (const [chatId, on] of kept) {
    if (on !== hostId) continue
    kept.delete(chatId)
    dropFeed(chatId)
  }
}

forgetWithChat((chatId) => {
  kept.delete(chatId)
  void forgetChatFeeds(chatId)
})

registerScopeKind('chat', {
  // Rows after the head the feed holds, or (a negative since) a window of the newest rows.
  since: (hostId, chatId) => {
    const attachment = attachments.get(chatId)
    const held = attachment?.hostId === hostId && !attachment.widening
    return (held && chatStore.getState().feeds[chatId]?.seq) || -(attachment?.rows ?? OPEN_ROWS)
  },
  // A kept feed's seq means nothing in another run's ledger: it replays from the newest rows.
  epoch: (hostId, next) => {
    const was = epochs.get(hostId)
    epochs.set(hostId, next)
    if (was !== undefined && was !== next) {
      dropKeptOn(hostId)
      void forgetHostFeeds(hostId)
    }
    // A stored copy of another run answers to that run's ledger: it shows until the window replaces it.
    for (const [chatId, attachment] of attachments) {
      const stored = chatStore.getState().feeds[chatId]
      if (attachment.hostId === hostId && attachment.cached && stored?.epoch !== next) {
        reconcile(chatId, attachment, false)
      }
    }
  },
  forget: (hostId) => {
    epochs.delete(hostId)
    dropKeptOn(hostId)
    void forgetHostFeeds(hostId)
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
  attachment.rows += OLDER_ROWS
  attachment.widening = true
  flush(chatId, attachment)
  refoldFeed(chatId)
  resubscribe(attachment.hostId, scopeOf(chatId))
}

/** Keeps a detached chat's live feed so its next attach shows it at once and resumes from its
 *  seq; a feed still replaying, or past the newest few, is dropped. */
function keepOrDrop(chatId: string, hostId: string) {
  kept.delete(chatId)
  if (!foldsLive(chatId)) return dropFeed(chatId)
  kept.set(chatId, hostId)
  for (const [oldest] of kept) {
    if (kept.size <= KEPT_FEEDS) break
    kept.delete(oldest)
    dropFeed(oldest)
  }
}

/** Holds the chat's ledger scope on `hostId` while anything attached to it is mounted. The
 *  last detach keeps the feed, and the next attach resumes it from its seq. */
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
      rows: OPEN_ROWS,
      widening: false,
      cached: false,
    }
    attachment = fresh
    attachments.set(chatId, fresh)
    const keptOn = kept.get(chatId)
    if (keptOn && keptOn !== hostId) dropFeed(chatId)
    kept.delete(chatId)
    const unseen = !chatStore.getState().feeds[chatId]
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
    let letGo = () => {}
    fresh.release = () => {
      stopWatch()
      letGo()
    }
    const hold = () => {
      if (attachments.get(chatId) === fresh) letGo = holdScope(hostId, scope)
    }
    // The cache is read before the scope is held, so a hit's first sub resumes at its cursor
    // instead of streaming a window the screen already shows. A failed read is a miss.
    if (unseen)
      void loadFeed(hostId, chatId).then((stored) => (showStored(chatId, fresh, stored), hold()))
    else hold()
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
    flush(chatId, held)
    persist(chatId, held)
    release(chatId, held)
    keepOrDrop(chatId, hostId)
  }
}
