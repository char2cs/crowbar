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
  fillRow,
  foldsLive,
  openFeed,
  prependRows,
  refoldFeed,
  showStoredFeed,
  type LiveFrame,
} from '@/features/agent/stores/chat-store'
import { PAGE_BYTES, openRows } from '@/features/agent/lib/feed-window'
import { readOlderRows, readRow } from '@/features/agent/api/chat-rows-reads'
import { report } from '@/lib/errors/report'
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
  pending: LiveFrame[]
  frame: number | null
  reasked: boolean
  widening: boolean
  paging: boolean
  cached: boolean
}

const openWindow = () => openRows(typeof window === 'undefined' ? 0 : window.innerHeight)

const KEPT_FEEDS = 8

const attachments = new Map<string, Attachment>()
const kept = new Map<string, string>()
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

function persist(chatId: string, attachment: Attachment) {
  const feed = chatStore.getState().feeds[chatId]
  if (feed?.status === 'live' && feed.seq > 0 && !attachment.cached) {
    saveFeed(attachment.hostId, chatId, feed)
  }
}

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

function reconcile(chatId: string, attachment: Attachment, ask: boolean) {
  attachment.cached = false
  attachment.widening = true
  void forgetFeed(attachment.hostId, chatId)
  refoldFeed(chatId)
  if (ask) resubscribe(attachment.hostId, scopeOf(chatId))
}

function showStored(chatId: string, attachment: Attachment, stored: ChatFeed | undefined) {
  if (!stored || !showStoredFeed(chatId, stored)) return
  attachment.cached = true
  const run = epochs.get(attachment.hostId)
  if (run !== undefined && run !== stored.epoch) reconcile(chatId, attachment, false)
}

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
  since: (hostId, chatId) => {
    const attachment = attachments.get(chatId)
    const held = attachment?.hostId === hostId && !attachment.widening
    return (held && chatStore.getState().feeds[chatId]?.seq) || -openWindow()
  },
  bytes: () => PAGE_BYTES,
  epoch: (hostId, next) => {
    const was = epochs.get(hostId)
    epochs.set(hostId, next)
    if (was !== undefined && was !== next) {
      dropKeptOn(hostId)
      void forgetHostFeeds(hostId)
    }
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
    if (f.t === 'entity') return applyEntityFrame(hostId, f)
    onFrame(chatId, attachment, f)
  },
})

export async function loadOlder(chatId: string) {
  const attachment = attachments.get(chatId)
  const from = chatStore.getState().feeds[chatId]?.from
  if (!attachment || attachment.paging || !from || !foldsLive(chatId)) return
  attachment.paging = true
  try {
    const page = await readOlderRows(attachment.hostId, chatId, from)
    if (attachments.get(chatId) === attachment) {
      prependRows(chatId, from, page.rows, page.more ? page.from : 0)
    }
  } catch (err) {
    report(err, { action: 'Load older messages' })
  } finally {
    attachment.paging = false
  }
}

export async function showFull(chatId: string, itemId: string) {
  const attachment = attachments.get(chatId)
  const item = chatStore.getState().feeds[chatId]?.items.get(itemId)
  const seq = item && 'fullSeq' in item ? item.fullSeq : undefined
  if (!attachment || seq === undefined) return
  const row = await readRow(attachment.hostId, chatId, seq)
  const now = chatStore.getState().feeds[chatId]?.items.get(itemId)
  if (attachments.get(chatId) === attachment && now && 'fullSeq' in now && now.fullSeq === seq) {
    fillRow(chatId, itemId, row)
  }
}

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
      widening: false,
      paging: false,
      cached: false,
    }
    attachment = fresh
    attachments.set(chatId, fresh)
    const keptOn = kept.get(chatId)
    if (keptOn && keptOn !== hostId) dropFeed(chatId)
    kept.delete(chatId)
    const unseen = !chatStore.getState().feeds[chatId]
    openFeed(chatId)
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
    if (unseen)
      void loadFeed(hostId, chatId).then((stored) => (showStored(chatId, fresh, stored), hold()))
    else hold()
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
