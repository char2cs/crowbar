import { commit, getDB } from '@/lib/persistence/idb'
import { WriteQueue } from '@/lib/persistence/write-queue'
import { ItemMap } from '@/features/agent/stores/lib/item-map'
import type { CachedFeed } from '@/features/agent/stores/lib/cached-feed'
import type { ChatFeed } from '@/features/agent/stores/lib/transcript-items'

const FEED_SCHEMA = 2
const MAX_ITEMS = 500
const MAX_CHATS = 100
const STORE = 'chat-feeds'
const LAST_KEY = '￿'

type Rows = CachedFeed['rows']
type FeedCursor = CachedFeed['cursor']

interface Save {
  hostId: string
  chatId: string
  feed: ChatFeed
}

function cutAt(feed: ChatFeed): number {
  const start = Math.max(0, feed.order.length - MAX_ITEMS)
  if (start === 0) return 0
  const at = feed.order.findIndex((id, i) => i >= start && feed.items.get(id)?.kind === 'user')
  return at < 0 ? start : at
}

function keptScopes(feed: ChatFeed, keep: Set<string>): Record<string, string[]> {
  const scopes = Object.entries(feed.facts.subagentScopes)
    .filter(([, card]) => keep.has(card))
    .map(([scope]) => scope)
  return Object.fromEntries(scopes.map((scope) => [scope, feed.scoped[scope] ?? []]))
}

function snapshot(feed: ChatFeed): { rows: Rows; cursor: FeedCursor } {
  const order = feed.order.slice(cutAt(feed))
  const listedByFacts = [...feed.facts.shells, ...feed.facts.liveSubagents]
  const keep = new Set([...order, ...listedByFacts])
  const scoped = keptScopes(feed, keep)
  Object.values(scoped).forEach((ids) => ids.forEach((id) => keep.add(id)))
  const items = feed.items.all().filter((item) => keep.has(item.id))
  const trimmed = order.length < feed.order.length
  const first = trimmed ? Math.min(...order.map((id) => feed.items.get(id)?.seq || Infinity)) : 0
  const { liveTail: _t, live: _l, liveN: _n, items: _i, status: _s, error: _e, ...rest } = feed
  return {
    rows: { ...rest, order, scoped, items, liveReasoning: null },
    cursor: {
      seq: feed.seq,
      from: trimmed && first !== Infinity ? Math.max(feed.from, first - 1) : feed.from,
      epoch: feed.epoch,
    },
  }
}

function restore(entry: CachedFeed): ChatFeed {
  const { epoch, seq, from } = { ...entry.cursor }
  return {
    ...entry.rows,
    status: 'live',
    epoch,
    seq,
    from,
    items: ItemMap.of(entry.rows.items),
    liveTail: [],
    live: {},
    liveN: {},
  }
}

const isEntry = (v: unknown): v is CachedFeed => {
  const e = v as Partial<CachedFeed> | undefined
  return (
    e?.schema === FEED_SCHEMA &&
    Array.isArray(e.rows?.order) &&
    Array.isArray(e.rows.items) &&
    typeof e.cursor?.seq === 'number' &&
    typeof e.cursor.epoch === 'string'
  )
}

const key = (hostId: string, chatId: string) => `${hostId}\u0000${chatId}`

const entryOf = ({ hostId, chatId, feed }: Save, savedAt: number): CachedFeed => ({
  hostId,
  chatId,
  schema: FEED_SCHEMA,
  ...snapshot(feed),
  savedAt,
})

const writeTx = async () => (await getDB()).transaction(STORE, 'readwrite')
type FeedTx = Awaited<ReturnType<typeof writeTx>>

async function evictOldest(tx: FeedTx): Promise<void> {
  let excess = (await tx.store.count()) - MAX_CHATS
  for (let at = await tx.store.index('savedAt').openCursor(); at && excess > 0; excess--) {
    await at.delete()
    at = await at.continue()
  }
}

async function writeEntries(entries: CachedFeed[]): Promise<void> {
  const tx = await writeTx()
  const write = async () => {
    await Promise.all(entries.map((entry) => tx.store.put(entry)))
    await evictOldest(tx)
  }
  await commit(tx, write)
}

async function deleteChatEntries(chatId: string): Promise<void> {
  const tx = await writeTx()
  const write = async () => {
    const keys = await tx.store.index('chatId').getAllKeys(chatId)
    await Promise.all(keys.map((k) => tx.store.delete(k)))
  }
  await commit(tx, write)
}

class FeedWrites {
  private readonly pending = new Map<string, Save>()
  private queued = false
  private readonly queue = new WriteQueue()

  save(save: Save): void {
    this.pending.set(key(save.hostId, save.chatId), save)
    if (this.queued) return
    this.queued = true
    queueMicrotask(() => this.flush())
  }

  forget(match: (save: Save) => boolean, job: () => Promise<unknown>): Promise<void> {
    for (const [k, save] of this.pending) if (match(save)) this.pending.delete(k)
    return this.queue.push(job)
  }

  private flush(): void {
    this.queued = false
    void this.queue.push(() => this.drain())
  }

  private async drain(): Promise<void> {
    const batch = [...this.pending.values()]
    this.pending.clear()
    if (batch.length === 0) return
    const savedAt = Date.now()
    await writeEntries(batch.map((save) => entryOf(save, savedAt)))
  }
}

const writes = new FeedWrites()

/** Queues the feed to be written behind the next microtask; saves meanwhile coalesce to the latest.
 *  An entry keeps the chat's newest 500 top-level items, cut at a prompt so its cursor resumes them,
 *  plus the shells and live subagents its facts list; past 100 chats the least recently saved go. */
export function saveFeed(hostId: string, chatId: string, feed: ChatFeed): void {
  writes.save({ hostId, chatId, feed })
}

/** The cached feed of `(hostId, chatId)`, or undefined: absent, of another schema, or unreadable
 *  (the cache only speeds an open up, so an unreadable one is a miss). */
export async function loadFeed(hostId: string, chatId: string): Promise<ChatFeed | undefined> {
  try {
    const db = await getDB()
    const entry = await db.get(STORE, [hostId, chatId])
    if (!entry) return undefined
    if (isEntry(entry)) return restore(entry)
    await db.delete(STORE, [hostId, chatId])
  } catch {
    return undefined
  }
  return undefined
}

/** Deletes a feed's entry, and any save still queued for it. */
export function forgetFeed(hostId: string, chatId: string): Promise<void> {
  return writes.forget(
    (v) => v.hostId === hostId && v.chatId === chatId,
    async () => (await getDB()).delete(STORE, [hostId, chatId]),
  )
}

/** Deletes a chat's entries on every host: the chat was deleted. */
export function forgetChatFeeds(chatId: string): Promise<void> {
  return writes.forget(
    (v) => v.chatId === chatId,
    () => deleteChatEntries(chatId),
  )
}

/** Deletes every entry of a host: its ledger is no longer one this client follows. */
export function forgetHostFeeds(hostId: string): Promise<void> {
  return writes.forget(
    (v) => v.hostId === hostId,
    async () => (await getDB()).delete(STORE, IDBKeyRange.bound([hostId, ''], [hostId, LAST_KEY])),
  )
}
