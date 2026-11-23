import { commit, getDB } from '@/lib/persistence/idb'
import { ItemMap } from '@/features/agent/stores/lib/item-map'
import type { ChatFeed, TranscriptItem } from '@/features/agent/stores/lib/transcript-items'

/** Bump when ChatFeed's folded shape changes: an entry of another schema is dropped on read. */
const FEED_SCHEMA = 2
/** Newest top-level items kept per chat, and chats kept in all (least recently saved go first). */
const MAX_ITEMS = 500
const MAX_CHATS = 100

/** Where a cached feed resumes: the ledger run, the last folded row, and the first one held. */
interface FeedCursor {
  seq: number
  from: number
  epoch: string
}

/** The folded feed minus what a reload cannot trust: live text, status and cursor. */
type Rows = Omit<
  ChatFeed,
  'status' | 'error' | 'epoch' | 'seq' | 'from' | 'items' | 'liveTail' | 'live' | 'liveN'
> & { items: TranscriptItem[] }

export interface CachedFeed {
  hostId: string
  chatId: string
  schema: number
  rows: Rows
  cursor: FeedCursor
  savedAt: number
}

/** Where the newest `MAX_ITEMS` items begin: at a prompt, so no turn is kept in part and every
 *  kept row follows the cursor. A run with no prompt in it is cut where it falls. */
function cutAt(feed: ChatFeed): number {
  const start = Math.max(0, feed.order.length - MAX_ITEMS)
  if (start === 0) return 0
  const at = feed.order.findIndex((id, i) => i >= start && feed.items.get(id)?.kind === 'user')
  return at < 0 ? start : at
}

/** The newest top-level items with what hangs off them, and the cursor to resume at. */
function snapshot(feed: ChatFeed): { rows: Rows; cursor: FeedCursor } {
  const order = feed.order.slice(cutAt(feed))
  // The facts list shells and live subagents by id wherever they sit, so they are kept by id.
  const keep = new Set([...order, ...feed.facts.shells, ...feed.facts.liveSubagents])
  const cards = new Set<string>()
  for (const [scope, card] of Object.entries(feed.facts.subagentScopes)) {
    if (keep.has(card)) cards.add(scope)
  }
  const scoped: Record<string, string[]> = {}
  for (const scope of cards) {
    const ids = feed.scoped[scope] ?? []
    scoped[scope] = ids
    for (const id of ids) keep.add(id)
  }
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

const pending = new Map<string, { hostId: string; chatId: string; feed: ChatFeed }>()
let queued = false
// Every write runs behind the last, so a delete is never overtaken by the save it follows.
let tail: Promise<void> = Promise.resolve()

/** Runs `job` after the earlier ones; a failure is the cache's loss, never the caller's. */
function enqueue(job: () => Promise<unknown>): Promise<void> {
  tail = tail.then(job).then(
    () => undefined,
    () => undefined,
  )
  return tail
}

async function drain() {
  const batch = [...pending.values()]
  pending.clear()
  if (batch.length === 0) return
  const db = await getDB()
  const tx = db.transaction('chat-feeds', 'readwrite')
  const savedAt = Date.now()
  await commit(tx, async () => {
    await Promise.all(
      batch.map(({ hostId, chatId, feed }) => {
        const { rows, cursor } = snapshot(feed)
        return tx.store.put({ hostId, chatId, schema: FEED_SCHEMA, rows, cursor, savedAt })
      }),
    )
    let excess = (await tx.store.count()) - MAX_CHATS
    for (let at = await tx.store.index('savedAt').openCursor(); at && excess > 0; excess--) {
      await at.delete()
      at = await at.continue()
    }
  })
}

/** Queues the feed to be written behind the next microtask; saves meanwhile coalesce to the latest. */
export function saveFeed(hostId: string, chatId: string, feed: ChatFeed): void {
  pending.set(key(hostId, chatId), { hostId, chatId, feed })
  if (queued) return
  queued = true
  queueMicrotask(() => {
    queued = false
    void enqueue(drain)
  })
}

/** The cached feed of `(hostId, chatId)`, or undefined: absent, of another schema, or unreadable. */
export async function loadFeed(hostId: string, chatId: string): Promise<ChatFeed | undefined> {
  try {
    const db = await getDB()
    const entry = await db.get('chat-feeds', [hostId, chatId])
    if (!entry) return undefined
    if (isEntry(entry)) return restore(entry)
    await db.delete('chat-feeds', [hostId, chatId])
  } catch {
    // The cache only speeds an open up: an unreadable one is a miss.
    return undefined
  }
  return undefined
}

function dropPending(match: (v: { hostId: string; chatId: string }) => boolean) {
  for (const [k, v] of pending) if (match(v)) pending.delete(k)
}

/** Deletes a feed's entry, and any save still queued for it. */
export function forgetFeed(hostId: string, chatId: string): Promise<void> {
  dropPending((v) => v.hostId === hostId && v.chatId === chatId)
  return enqueue(async () => (await getDB()).delete('chat-feeds', [hostId, chatId]))
}

/** Deletes a chat's entries on every host: the chat was deleted. */
export function forgetChatFeeds(chatId: string): Promise<void> {
  dropPending((v) => v.chatId === chatId)
  return enqueue(async () => {
    const db = await getDB()
    const tx = db.transaction('chat-feeds', 'readwrite')
    await commit(tx, async () => {
      const keys = await tx.store.index('chatId').getAllKeys(chatId)
      await Promise.all(keys.map((k) => tx.store.delete(k)))
    })
  })
}

/** Deletes every entry of a host: its ledger is no longer one this client follows. */
export function forgetHostFeeds(hostId: string): Promise<void> {
  dropPending((v) => v.hostId === hostId)
  return enqueue(async () =>
    (await getDB()).delete('chat-feeds', IDBKeyRange.bound([hostId, ''], [hostId, '￿'])),
  )
}
