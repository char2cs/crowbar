import { beforeEach, describe, expect, it, vi } from 'vitest'
import { IDBFactory } from 'fake-indexeddb'
import { getDB } from '@/lib/persistence/idb'
import { applyRows, ledger } from '@/__tests__/__fixtures__/ledger-rows'
import { applyReady } from '@/features/agent/stores/lib/fold-live'
import { emptyFeed } from '@/features/agent/stores/lib/fold-rows'
import {
  forgetChatFeeds,
  forgetFeed,
  forgetHostFeeds,
  loadFeed,
  saveFeed,
} from '@/features/agent/stores/lib/feed-cache'

const MAX_ITEMS = 500
const MAX_CHATS = 100
// A delete queues behind every write already queued, so its settling is theirs.
const settled = async () => {
  await Promise.resolve()
  await forgetFeed('~', '~')
}
const prompts = (n: number, chatId = 'chat-1') => {
  const l = ledger(chatId)
  return Array.from({ length: n }, (_, i) => l.prompt(`r${i}`, `text ${i}`))
}
const feedOf = (n: number, epoch = 'e1') => applyReady(applyRows(emptyFeed(), prompts(n)), epoch)
const entries = async () => (await getDB()).getAll('chat-feeds')

beforeEach(() => {
  globalThis.indexedDB = new IDBFactory()
})

describe('feed cache', () => {
  it('returns a saved feed with its rows, items and resume cursor', async () => {
    const feed = feedOf(3)
    saveFeed('h', 'chat-1', feed)
    await settled()
    const back = await loadFeed('h', 'chat-1')
    expect(back).toMatchObject({ status: 'live', epoch: 'e1', seq: 3, from: 0, order: feed.order })
    for (const id of feed.order) expect(back?.items.get(id)).toEqual(feed.items.get(id))
    expect(back?.facts).toEqual(feed.facts)
  })

  it('misses on another host or chat', async () => {
    saveFeed('h', 'chat-1', feedOf(1))
    await settled()
    expect(await loadFeed('other', 'chat-1')).toBeUndefined()
    expect(await loadFeed('h', 'chat-2')).toBeUndefined()
  })

  it('writes what saves queued in one tick as one transaction, newest feed winning', async () => {
    const db = await getDB()
    const tx = vi.spyOn(db, 'transaction')
    saveFeed('h', 'chat-1', feedOf(1))
    saveFeed('h', 'chat-1', feedOf(2))
    saveFeed('h', 'chat-2', feedOf(1))
    await vi.waitFor(async () => expect(await entries()).toHaveLength(2))
    expect(tx.mock.calls.filter(([, mode]) => mode === 'readwrite')).toHaveLength(1)
    expect((await loadFeed('h', 'chat-1'))?.seq).toBe(2)
    expect(await entries()).toHaveLength(2)
  })

  it('drops an entry of another schema when it is read', async () => {
    saveFeed('h', 'chat-1', feedOf(1))
    await settled()
    const db = await getDB()
    const old = (await db.get('chat-feeds', ['h', 'chat-1']))!
    await db.put('chat-feeds', { ...old, schema: 99 })
    expect(await loadFeed('h', 'chat-1')).toBeUndefined()
    expect(await entries()).toEqual([])
  })

  it('keeps the newest items of a long chat and moves its first held row up', async () => {
    saveFeed('h', 'chat-1', feedOf(MAX_ITEMS + 20))
    await settled()
    const back = await loadFeed('h', 'chat-1')
    expect(back?.order).toHaveLength(MAX_ITEMS)
    expect(back?.order.at(-1)).toBe(`p:r${MAX_ITEMS + 19}`)
    expect(back).toMatchObject({ seq: MAX_ITEMS + 20, from: 20 })
    expect(back?.items.get('p:r0')).toBeUndefined()
  })

  it('cuts a long chat between turns, so every kept row follows its cursor', async () => {
    const l = ledger()
    const rows = Array.from({ length: 300 }, (_, i) => [
      l.prompt(`r${i}`, `q${i}`),
      l.start(`t${i}`, `r${i}`),
      l.tool(`t${i}`, `a${i}`, 'post'),
      l.tool(`t${i}`, `b${i}`, 'post'),
      l.end(`t${i}`),
    ]).flat()
    saveFeed('h', 'chat-1', applyReady(applyRows(emptyFeed(), rows), 'e1'))
    await settled()
    const back = (await loadFeed('h', 'chat-1'))!
    expect(back.order.length).toBeLessThanOrEqual(MAX_ITEMS)
    expect(back.items.get(back.order[0]!)?.kind).toBe('user')
    const kept = back.order.map((id) => back.items.get(id)!.seq)
    expect(Math.min(...kept)).toBe(back.from + 1)
  })

  it('evicts the least recently saved chats beyond the cap', async () => {
    for (let i = 0; i < MAX_CHATS; i++) saveFeed('h', `c${i}`, feedOf(1))
    await settled()
    saveFeed('h', 'newest', feedOf(1))
    await settled()
    expect(await entries()).toHaveLength(MAX_CHATS)
    expect(await loadFeed('h', 'newest')).toBeDefined()
  })

  it('forgets a chat on every host, a host, and one entry, and cancels a save still queued', async () => {
    for (const [h, c] of [
      ['a', 'x'],
      ['b', 'x'],
      ['a', 'y'],
      ['b', 'y'],
    ] as const) {
      saveFeed(h, c, feedOf(1))
    }
    await settled()
    await forgetChatFeeds('x')
    expect((await entries()).map((e) => `${e.hostId}/${e.chatId}`).sort()).toEqual(['a/y', 'b/y'])
    await forgetHostFeeds('a')
    expect((await entries()).map((e) => e.hostId)).toEqual(['b'])
    saveFeed('b', 'z', feedOf(1))
    await forgetFeed('b', 'z')
    await forgetFeed('b', 'y')
    expect(await entries()).toEqual([])
  })

  it('shares its database between windows: an entry is one window’s whole copy, and a forget is seen by both', async () => {
    vi.resetModules()
    const other = await import('@/features/agent/stores/lib/feed-cache')
    saveFeed('h', 'chat-1', feedOf(3))
    other.saveFeed('h', 'chat-1', feedOf(5))
    other.saveFeed('h', 'chat-2', feedOf(1))
    await Promise.all([settled(), other.forgetFeed('~', '~')])
    const back = await loadFeed('h', 'chat-1')
    expect(back?.seq).toBe(back?.order.length)
    expect(await other.loadFeed('h', 'chat-2')).toBeDefined()
    await other.forgetFeed('h', 'chat-1')
    expect(await loadFeed('h', 'chat-1')).toBeUndefined()
  })

  it('does nothing and throws nothing when the database cannot open', async () => {
    const broken = new IDBFactory()
    vi.spyOn(broken, 'open').mockImplementation(() => {
      throw new DOMException('denied', 'SecurityError')
    })
    globalThis.indexedDB = broken
    saveFeed('h', 'chat-1', feedOf(1))
    await expect(settled()).resolves.toBeUndefined()
    await expect(loadFeed('h', 'chat-1')).resolves.toBeUndefined()
    await expect(forgetChatFeeds('chat-1')).resolves.toBeUndefined()
  })

  it('survives a write that aborts', async () => {
    const db = await getDB()
    vi.spyOn(db, 'transaction').mockImplementationOnce(() => {
      throw new DOMException('quota', 'QuotaExceededError')
    })
    saveFeed('h', 'chat-1', feedOf(1))
    await expect(settled()).resolves.toBeUndefined()
    saveFeed('h', 'chat-1', feedOf(2))
    await settled()
    expect((await loadFeed('h', 'chat-1'))?.seq).toBe(2)
  })
})
