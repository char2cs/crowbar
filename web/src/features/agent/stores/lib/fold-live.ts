import type { FeedDelta } from '@/features/agent/api/chat-rows'
import {
  liveKey,
  textItemId,
  type ChatFeed,
  type TextItem,
} from '@/features/agent/stores/lib/transcript-items'

/** `next` when it extends `text`, else `text`; `next` fills an empty text. */
export function fuller(text: string, next: string): string {
  if (text === '' || (next.length > text.length && next.startsWith(text))) return next
  return text
}

/** S1's Messages rule: the chunks 0..k when k is final, else the complete
 *  text, else the chunks present in index order. */
export function textOf(item: Pick<TextItem, 'chunks' | 'finalIndex' | 'complete'>): {
  text: string
  settled: boolean
} {
  let stream = ''
  let final = false
  for (let i = 0; i in item.chunks; i++) {
    stream += item.chunks[i]
    if (i === item.finalIndex) {
      final = true
      break
    }
  }
  const complete = item.complete ?? ''
  if (final) return { text: fuller(stream, complete), settled: true }
  if (item.complete !== undefined) return { text: fuller(complete, stream), settled: true }
  const present = Object.keys(item.chunks)
    .map(Number)
    .sort((a, b) => a - b)
    .map((i) => item.chunks[i])
    .join('')
  return { text: present, settled: false }
}

function liveItem(feed: ChatFeed, delta: FeedDelta): ChatFeed {
  if (delta.stream !== 'answer' || delta.scope || feed.facts.traceless[delta.turnId]) return feed
  const id = textItemId(delta.turnId, delta.id)
  if (feed.items[id]) return feed
  const item: TextItem = {
    kind: 'text',
    id,
    seq: 0,
    turnId: delta.turnId,
    at: '',
    messageId: delta.id,
    chunks: {},
    text: '',
    settled: false,
    phase: '',
  }
  return { ...feed, items: { ...feed.items, [id]: item }, liveTail: [...feed.liveTail, id] }
}

/** Appends a delta that follows its stream's last, replaces on `replace`, and
 *  ignores a repeat or a gap (the next replace or the item's row repairs it). */
export function applyDelta(feed: ChatFeed, delta: FeedDelta): ChatFeed {
  const key = liveKey(delta.stream, delta.id)
  const last = feed.liveN[key] ?? 0
  if (!delta.replace && delta.n !== last + 1) return feed
  const text = delta.replace ? delta.text : (feed.live[key] ?? '') + delta.text
  const next: ChatFeed = {
    ...feed,
    live: { ...feed.live, [key]: text },
    liveN: { ...feed.liveN, [key]: delta.n },
  }
  if (delta.stream === 'reasoning' && !delta.scope && feed.liveReasoning !== key) {
    next.liveReasoning = key
  }
  return liveItem(next, delta)
}

/** A new epoch is a restarted daemon: live text of the old one never resumes. */
export function applyReady(feed: ChatFeed, epoch: string): ChatFeed {
  if (feed.epoch === epoch) return feed.status === 'live' ? feed : { ...feed, status: 'live' }
  if (feed.epoch === '') return { ...feed, epoch, status: 'live' }
  const items = { ...feed.items }
  for (const id of feed.liveTail) delete items[id]
  return {
    ...feed,
    epoch,
    status: 'live',
    items,
    liveTail: [],
    live: {},
    liveN: {},
    liveReasoning: null,
  }
}
