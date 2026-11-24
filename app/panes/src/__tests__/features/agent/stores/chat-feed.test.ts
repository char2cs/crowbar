import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { attachFeed as attachReal, loadOlder } from '@/features/agent/stores/chat-feed'
import { forgetEntities } from '@/core/entities/entity-store'
import { followChatLifecycle } from '@/features/agent/stores/chat-lifecycle'
import { chatStore } from '@/features/agent/stores/chat-store'
import { heldScopes, streamDaemon } from '@/__tests__/__fixtures__/stream-daemon'
import { writeChat } from '@/__tests__/__fixtures__/agent-chat'
import { delta, ledger } from '@/__tests__/__fixtures__/ledger-rows'
import { IDBFactory, IDBObjectStore } from 'fake-indexeddb'
import { getDB } from '@/lib/persistence/idb'
import { forgetFeed, loadFeed, saveFeed } from '@/features/agent/stores/lib/feed-cache'
import { applyReady } from '@/features/agent/stores/lib/fold-live'
import { applyRows } from '@/__tests__/__fixtures__/ledger-rows'
import { emptyFeed } from '@/features/agent/stores/lib/fold-rows'

const SCOPE = 'chat:chat-1'
const feed = (chatId = 'chat-1') => chatStore.getState().feeds[chatId]
const push = (frame: object, scope = SCOPE) => streamDaemon.deliver({ scope, ...frame })
const subs = (scope = SCOPE) => streamDaemon.sent.filter((r) => r.op === 'sub' && r.scope === scope)
const held: (() => void)[] = []
// The scope is held once the cache read settles, so a view's first sub is awaited here.
const attachFeed = async (chatId: string) => {
  const detach = attachReal('local', chatId)
  held.push(detach)
  await vi.waitFor(() => expect(subs(`chat:${chatId}`).length).toBeGreaterThan(0))
  return detach
}

beforeEach(async () => {
  globalThis.indexedDB = new IDBFactory()
  chatStore.setState({ feeds: {} })
  streamDaemon.answer = () => [] // each test writes what the daemon sends
  streamDaemon.deliver({ t: 'ready', epoch: 'e1' })
  await settled() // a new run's forget of the previous test's host must land before this one stores
  streamDaemon.sent.length = 0
  vi.useFakeTimers({ toFake: ['requestAnimationFrame', 'cancelAnimationFrame'] })
})
afterEach(async () => {
  held.splice(0).forEach((detach) => detach())
  forgetEntities('chat', ['chat-1', 'chat-2']) // a kept feed would answer the next test's attach
  await settled()
  vi.useRealTimers()
})

const settled = async () => {
  await Promise.resolve()
  await forgetFeed('~', '~')
}

describe('attachFeed', () => {
  it('attaching a feed holds chat:<id> and detaching releases it', async () => {
    const a = await attachFeed('chat-1')
    const b = await attachFeed('chat-1')
    expect(subs()).toEqual([{ op: 'sub', scope: SCOPE, since: -1500 }])
    a()
    a()
    expect(heldScopes('chat:')).toEqual([SCOPE])
    b()
    expect(heldScopes('chat:')).toEqual([])
  })

  it('keeps a live feed after its last view detaches and resumes it from its seq', async () => {
    const detach = await attachFeed('chat-1')
    push({ t: 'row', row: ledger().prompt('r1', 'hi') })
    push({ t: 'synced', seq: 1 })
    detach()
    expect(feed()).toMatchObject({ status: 'live', order: ['p:r1'] })
    expect(heldScopes('chat:')).toEqual([])

    streamDaemon.sent.length = 0
    await attachFeed('chat-1')
    expect(feed()).toMatchObject({ status: 'live', order: ['p:r1'] })
    expect(subs()).toEqual([{ op: 'sub', scope: SCOPE, since: 1 }])
  })

  it('drops a feed that detaches before its replay lands', async () => {
    ;(await attachFeed('chat-1'))()
    expect(feed()).toBeUndefined()
  })

  it('keeps only the newest detached feeds', async () => {
    for (let i = 0; i < 9; i++) {
      await attachFeed(`c${i}`)
      push({ t: 'synced' }, `chat:c${i}`)
      held.pop()!()
    }
    expect(feed('c0')).toBeUndefined()
    expect(feed('c1')).toBeDefined()
    expect(feed('c8')).toBeDefined()
  })

  it('a deleted chat leaves its kept slot to the others', async () => {
    writeChat({ id: 'c3' })
    const stopLifecycle = followChatLifecycle()
    for (let i = 0; i < 8; i++) {
      await attachFeed(`c${i}`)
      push({ t: 'synced' }, `chat:c${i}`)
      held.pop()!()
    }
    forgetEntities('chat', ['c3'])
    await attachFeed('c8')
    push({ t: 'synced' }, 'chat:c8')
    held.pop()!()
    stopLifecycle()
    expect(feed('c3')).toBeUndefined()
    expect(feed('c0')).toBeDefined()
  })

  it('a feed kept under an older ledger epoch is dropped, so its return replays from the newest rows', async () => {
    const detach = await attachFeed('chat-1')
    push({ t: 'row', row: ledger().prompt('r1', 'hi') })
    push({ t: 'synced', seq: 1 })
    detach()

    streamDaemon.drop()
    streamDaemon.deliver({ t: 'ready', epoch: 'e2' })
    streamDaemon.sent.length = 0
    await attachFeed('chat-1')
    expect(feed()).toMatchObject({ status: 'connecting', order: [] })
    expect(subs()).toEqual([{ op: 'sub', scope: SCOPE, since: -1500 }])
  })

  it('does not resume a kept feed from another host’s seq', async () => {
    const detach = await attachFeed('chat-1')
    push({ t: 'row', row: ledger().prompt('r1', 'hi') })
    push({ t: 'synced', seq: 1 })
    detach()
    held.push(attachReal('host-9', 'chat-1'))
    expect(feed()).toMatchObject({ status: 'connecting', order: [] })
  })

  it('publishes replayed rows once, at synced', async () => {
    await attachFeed('chat-1')
    const l = ledger()
    const writes = vi.fn()
    const stop = chatStore.subscribe(writes)
    push({ t: 'row', row: l.prompt('r1', 'hi') })
    push({ t: 'row', row: l.start('turn-1', 'r1') })
    expect(writes).not.toHaveBeenCalled()
    push({ t: 'synced', seq: 2 })
    expect(writes).toHaveBeenCalledTimes(1)
    expect(feed()).toMatchObject({ status: 'live', seq: 2, order: ['p:r1'] })
    stop()
  })

  it('a resubscribe sends since = the last applied seq', async () => {
    await attachFeed('chat-1')
    const l = ledger()
    // A turn's start adds no item, so the seq is not the item count.
    push({ t: 'row', row: l.prompt('r1', 'hi') })
    push({ t: 'row', row: l.start('turn-1', 'r1') })
    push({ t: 'synced', seq: 2 })
    expect(feed()?.order).toEqual(['p:r1'])
    push({ t: 'closed', reason: 'resync', seq: 2 })
    expect(subs().at(-1)).toEqual({ op: 'sub', scope: SCOPE, since: 2 })

    streamDaemon.drop()
    streamDaemon.deliver({ t: 'ready', epoch: 'e1' })
    expect(subs().at(-1)).toEqual({ op: 'sub', scope: SCOPE, since: 2 })
  })

  it('rebuilds from the newest rows when a resync names a seq behind the feed', async () => {
    await attachFeed('chat-1')
    const l = ledger()
    push({ t: 'row', row: l.prompt('r1', 'hi') })
    push({ t: 'row', row: l.prompt('r2', 'again') })
    push({ t: 'synced', seq: 2 })
    push({ t: 'closed', reason: 'resync', seq: 1 })
    expect(subs().at(-1)).toEqual({
      op: 'sub',
      scope: SCOPE,
      since: -1500,
    })
    expect(feed()?.order).toEqual([])
  })

  it('opens on a window of the newest rows, and loads older ones behind the shown feed', async () => {
    const l = ledger('chat-1', 39)
    const [p39, s39, p41, s41] = [
      l.prompt('r39', 'older'),
      l.start('turn-39', 'r39'),
      l.prompt('r41', 'newest'),
      l.start('turn-41', 'r41'),
    ]
    await attachFeed('chat-1')
    expect(subs()).toEqual([{ op: 'sub', scope: SCOPE, since: -1500 }])
    push({ t: 'row', row: p41, from: 40 })
    push({ t: 'row', row: s41 })
    push({ t: 'synced', seq: 42 })
    expect(feed()).toMatchObject({ status: 'live', from: 40, seq: 42, order: ['p:r41'] })

    loadOlder('chat-1')
    expect(subs().at(-1)).toEqual({ op: 'sub', scope: SCOPE, since: -4500 })
    // The old sub's live row lands before the wider replay: it is not the window's start.
    push({ t: 'row', row: l.prompt('r43', 'live') })
    push({ t: 'row', row: p39, from: 38 })
    for (const row of [s39, p41, s41]) push({ t: 'row', row })
    expect(feed()?.order).toEqual(['p:r41'])
    push({ t: 'synced', seq: 42 })
    expect(feed()).toMatchObject({ from: 38, seq: 42, order: ['p:r39', 'p:r41'] })
  })

  it('asks for no older turns when the feed holds the chat from its start', async () => {
    await attachFeed('chat-1')
    push({ t: 'row', row: ledger().prompt('r1', 'hi') })
    push({ t: 'synced', seq: 1 })
    loadOlder('chat-1')
    expect(subs()).toHaveLength(1)
  })

  it('drops a row that does not follow the last applied one, so the replay fills the hole', async () => {
    await attachFeed('chat-1')
    const l = ledger()
    for (const id of ['r1', 'r2', 'r3']) push({ t: 'row', row: l.prompt(id, id) })
    push({ t: 'synced', seq: 3 })
    push({ t: 'closed', reason: 'resync', seq: 3 })
    expect(subs().at(-1)).toEqual({ op: 'sub', scope: SCOPE, since: 3 })
    const [r4, r5, r6] = [l.prompt('r4', 'r4'), l.prompt('r5', 'r5'), l.prompt('r6', 'r6')]

    // The old sub's row 5 was still in flight when the new sub went out.
    push({ t: 'row', row: r5 })
    for (const row of [r4, r5, r6]) push({ t: 'row', row })
    push({ t: 'synced', seq: 6 })

    expect(feed()).toMatchObject({ seq: 6, status: 'live' })
    expect(feed()?.order).toEqual(['p:r1', 'p:r2', 'p:r3', 'p:r4', 'p:r5', 'p:r6'])
  })

  it('drops a live row past the next one, so the order never skips', async () => {
    await attachFeed('chat-1')
    const l = ledger()
    push({ t: 'row', row: l.prompt('r1', 'r1') })
    push({ t: 'synced', seq: 1 })
    const [r2, r3] = [l.prompt('r2', 'r2'), l.prompt('r3', 'r3')]

    push({ t: 'row', row: r3 })
    vi.advanceTimersToNextFrame()
    expect(feed()).toMatchObject({ seq: 1, order: ['p:r1'] })
    push({ t: 'row', row: r2 })
    push({ t: 'row', row: r3 })
    vi.advanceTimersToNextFrame()
    expect(feed()).toMatchObject({ seq: 3, order: ['p:r1', 'p:r2', 'p:r3'] })
  })

  it('a synced past the last applied row resubscribes from it once, then fails', async () => {
    await attachFeed('chat-1')
    const l = ledger()
    push({ t: 'row', row: l.prompt('r1', 'r1') })
    push({ t: 'row', row: l.prompt('r2', 'r2') })
    const sent = streamDaemon.sent.length

    push({ t: 'synced', seq: 5 })
    expect(streamDaemon.sent.slice(sent)).toEqual([
      { op: 'unsub', scope: SCOPE },
      { op: 'sub', scope: SCOPE, since: 2 },
    ])

    push({ t: 'synced', seq: 5 })
    expect(feed()).toMatchObject({ status: 'error', seq: 2 })
    expect(streamDaemon.sent.slice(sent)).toHaveLength(2)
  })

  it('a synced that agrees again clears the one resubscribe it allows', async () => {
    await attachFeed('chat-1')
    const l = ledger()
    push({ t: 'row', row: l.prompt('r1', 'r1') })
    push({ t: 'synced', seq: 2 })
    push({ t: 'row', row: l.prompt('r2', 'r2') })
    push({ t: 'synced', seq: 2 })
    push({ t: 'row', row: l.prompt('r3', 'r3') })
    push({ t: 'synced', seq: 4 })

    expect(feed()?.status).toBe('live')
    expect(subs().at(-1)).toEqual({ op: 'sub', scope: SCOPE, since: 3 })
  })

  it('discards an unpublished replay on a resync before synced', async () => {
    await attachFeed('chat-1')
    push({ t: 'row', row: ledger().prompt('r1', 'hi') })
    push({ t: 'closed', reason: 'resync', seq: 1 })
    expect(subs().at(-1)).toEqual({
      op: 'sub',
      scope: SCOPE,
      since: -1500,
    })
    push({ t: 'row', row: ledger().prompt('r9', 'replayed') })
    push({ t: 'synced', seq: 1 })
    expect(feed()?.order).toEqual(['p:r9'])
  })

  it('a deleted close drops the feed and leaves the others', async () => {
    await attachFeed('chat-1')
    await attachFeed('chat-2')
    push({ t: 'synced' })
    push({ t: 'synced' }, 'chat:chat-2')
    push({ t: 'closed', reason: 'deleted' })
    expect(feed()).toBeUndefined()
    expect(feed('chat-2')?.status).toBe('live')
    expect(heldScopes('chat:')).toEqual(['chat:chat-2'])
  })

  it('a refused chat says why and is not asked for again, even after a reconnect', async () => {
    await attachFeed('chat-1')
    const unreadable = "This chat's history could not be read"
    push({ t: 'closed', reason: 'refused', problem: { code: 'unavailable', message: unreadable } })
    expect(feed()).toMatchObject({ status: 'error', error: unreadable })
    expect(heldScopes('chat:')).toEqual([])

    streamDaemon.drop()
    streamDaemon.deliver({ t: 'ready', epoch: 'e1' })
    expect(subs()).toHaveLength(1)
  })

  it('a new epoch resets every feed', async () => {
    await attachFeed('chat-1')
    await attachFeed('chat-2')
    push({ t: 'synced' })
    push({ t: 'synced' }, 'chat:chat-2')
    push({ t: 'delta', delta: delta('m1', 1, 'a') })
    push({ t: 'delta', delta: delta('m2', 1, 'b') }, 'chat:chat-2')
    vi.advanceTimersToNextFrame()
    expect(feed()?.live).not.toEqual({})

    streamDaemon.drop()
    streamDaemon.deliver({ t: 'ready', epoch: 'e2' })
    push({ t: 'synced' })
    push({ t: 'synced' }, 'chat:chat-2')
    expect(feed()).toMatchObject({ epoch: 'e2', liveTail: [], live: {} })
    expect(feed('chat-2')).toMatchObject({ epoch: 'e2', liveTail: [], live: {} })
  })

  it('applies live rows and deltas once per frame, in the order they came', async () => {
    await attachFeed('chat-1')
    push({ t: 'synced' })
    const writes = vi.fn()
    const stop = chatStore.subscribe(writes)
    const l = ledger('chat-1')
    push({ t: 'delta', delta: delta('m1', 1, 'a') })
    push({ t: 'delta', delta: delta('m1', 2, 'b') })
    push({ t: 'row', row: l.prompt('r1', 'hi') })
    push({ t: 'delta', delta: delta('m1', 3, 'c') })
    push({ t: 'row', row: l.text('turn-1', 'm1', 0, 'abc') })
    expect(writes).not.toHaveBeenCalled()
    vi.advanceTimersToNextFrame()
    expect(writes).toHaveBeenCalledTimes(1)
    expect(feed()?.order).toEqual(['p:r1', 'm:turn-1:m1'])
    expect(feed()?.seq).toBe(2)
    stop()
  })

  it('applies what is queued before a synced checks the head', async () => {
    await attachFeed('chat-1')
    push({ t: 'synced' })
    push({ t: 'row', row: ledger('chat-1').prompt('r1', 'hi') })
    push({ t: 'synced', seq: 1 })
    expect(feed()).toMatchObject({ seq: 1, order: ['p:r1'] })
    expect(subs()).toHaveLength(1)
  })

  it('applies what is queued as the tab hides, and frames as they come while hidden', async () => {
    await attachFeed('chat-1')
    push({ t: 'synced' })
    const l = ledger('chat-1')
    push({ t: 'row', row: l.prompt('r1', 'hi') })
    const hidden = vi.spyOn(document, 'hidden', 'get').mockReturnValue(true)
    document.dispatchEvent(new Event('visibilitychange'))
    expect(feed()?.order).toEqual(['p:r1'])
    push({ t: 'row', row: l.prompt('r2', 'again') })
    expect(feed()?.order).toEqual(['p:r1', 'p:r2'])
    hidden.mockRestore()
  })

  it('applies what is queued as the last view detaches, so the kept feed resumes after it', async () => {
    const detach = await attachFeed('chat-1')
    push({ t: 'synced' })
    push({ t: 'row', row: ledger('chat-1').prompt('r1', 'hi') })
    detach()
    expect(feed()?.order).toEqual(['p:r1'])
  })
})

describe('the stored feed', () => {
  const rowsOf = (n: number) => {
    const l = ledger()
    return Array.from({ length: n }, (_, i) => l.prompt(`r${i + 1}`, `text ${i + 1}`))
  }
  const store = async (n: number, epoch = 'e1', chatId = 'chat-1') => {
    saveFeed('local', chatId, applyReady(applyRows(emptyFeed(), rowsOf(n)), epoch))
    await settled()
  }
  const shown = (n: number) => Array.from({ length: n }, (_, i) => `p:r${i + 1}`)
  const entry = () => loadFeed('local', 'chat-1')
  const window = () => -1500

  it('shows a cold start’s stored rows before any daemon frame, then resumes from its cursor', async () => {
    await store(2)
    await attachFeed('chat-1')
    await vi.waitFor(() => expect(feed()).toMatchObject({ status: 'live', order: shown(2) }))
    expect(subs().at(-1)).toEqual({ op: 'sub', scope: SCOPE, since: 2 })

    push({ t: 'row', row: ledger('chat-1', 3).prompt('r3', 'text 3') })
    push({ t: 'synced', seq: 3 })
    vi.advanceTimersToNextFrame()
    expect(feed()).toMatchObject({ seq: 3, order: shown(3) })
  })

  it('replays the newest window when nothing is stored', async () => {
    await attachFeed('chat-1')
    await settled()
    expect(feed()).toMatchObject({ status: 'connecting', order: [] })
    expect(subs()).toEqual([{ op: 'sub', scope: SCOPE, since: window() }])
  })

  it('sends one sub on a hit, at the cursor, with no window replay', async () => {
    await store(2)
    await attachFeed('chat-1')
    await vi.waitFor(() => expect(feed()?.order).toEqual(shown(2)))
    expect(subs()).toEqual([{ op: 'sub', scope: SCOPE, since: 2 }])
  })

  it('holds the scope until the cache read settles, so a slow read delays the one sub, not doubles it', async () => {
    await store(2)
    let land!: () => void
    const blocked = new Promise<void>((resolve) => (land = resolve))
    const db = await getDB()
    const get = db.get.bind(db)
    vi.spyOn(db, 'get').mockImplementation(async (...args: Parameters<typeof get>) => {
      await blocked
      return get(...args)
    })
    held.push(attachReal('local', 'chat-1'))
    expect(subs()).toEqual([])
    land()
    await vi.waitFor(() => expect(subs()).toHaveLength(1))
    expect(subs()[0]).toMatchObject({ since: 2 })
  })

  it('asks for the window when the cache read fails', async () => {
    await store(2)
    const db = await getDB()
    vi.spyOn(db, 'get').mockRejectedValue(new Error('unreadable'))
    await attachFeed('chat-1')
    expect(subs()).toEqual([{ op: 'sub', scope: SCOPE, since: window() }])
  })

  it('replaces a copy of another run in one write, with the window of the new run', async () => {
    await store(2, 'old-run')
    await attachFeed('chat-1')
    await vi.waitFor(() => expect(feed()?.order).toEqual(shown(2)))
    expect(subs().at(-1)).toEqual({ op: 'sub', scope: SCOPE, since: window() })
    const writes = vi.fn()
    const stop = chatStore.subscribe(writes)
    const [r1, r2] = rowsOf(2)
    push({ t: 'row', row: r1, from: 0 })
    push({ t: 'row', row: r2 })
    expect(writes).not.toHaveBeenCalled()
    push({ t: 'synced', seq: 2 })
    stop()
    expect(writes).toHaveBeenCalledTimes(1)
    expect(feed()).toMatchObject({ epoch: 'e1', seq: 2, order: shown(2) })
    await settled()
    expect((await entry())?.epoch).toBe('e1')
  })

  it('shows what the daemon has when it holds more rows than the copy', async () => {
    await store(1)
    await attachFeed('chat-1')
    await vi.waitFor(() => expect(feed()?.order).toEqual(shown(1)))
    const [, r2, r3] = rowsOf(3)
    push({ t: 'row', row: r2 })
    push({ t: 'row', row: r3 })
    push({ t: 'synced', seq: 3 })
    vi.advanceTimersToNextFrame()
    expect(feed()).toMatchObject({ seq: 3, order: shown(3) })
  })

  it('replays the window when the daemon holds fewer rows than the copy', async () => {
    await store(3)
    await attachFeed('chat-1')
    await vi.waitFor(() => expect(feed()?.order).toEqual(shown(3)))
    push({ t: 'synced', seq: 1 })
    expect(feed()?.order).toEqual(shown(3))
    expect(subs().at(-1)).toEqual({ op: 'sub', scope: SCOPE, since: window() })
    push({ t: 'row', row: rowsOf(1)[0], from: 0 })
    push({ t: 'synced', seq: 1 })
    expect(feed()).toMatchObject({ seq: 1, order: shown(1) })
    await settled()
    expect((await entry())?.seq).toBe(1)
  })

  it('replays the window when the daemon answers the cursor with a resync', async () => {
    await store(3)
    await attachFeed('chat-1')
    await vi.waitFor(() => expect(feed()?.order).toEqual(shown(3)))
    push({ t: 'closed', reason: 'resync', seq: 1 })
    expect(feed()?.order).toEqual(shown(3))
    expect(subs().at(-1)).toEqual({ op: 'sub', scope: SCOPE, since: window() })
    push({ t: 'row', row: rowsOf(1)[0], from: 0 })
    push({ t: 'synced', seq: 1 })
    expect(feed()).toMatchObject({ seq: 1, order: shown(1) })
  })

  it('writes the feed through as its replay settles and as live rows land', async () => {
    await attachFeed('chat-1')
    const [r1, r2] = rowsOf(2)
    push({ t: 'row', row: r1 })
    push({ t: 'synced', seq: 1 })
    await settled()
    expect((await entry())?.seq).toBe(1)
    push({ t: 'row', row: r2 })
    vi.advanceTimersToNextFrame()
    await settled()
    expect((await entry())?.order).toEqual(shown(2))
  })

  it('writes a running turn through when it settles, not on each live row and delta', async () => {
    await attachFeed('chat-1')
    const l = ledger()
    push({ t: 'row', row: l.prompt('r1', 'hi') })
    push({ t: 'synced', seq: 1 })
    await settled()
    const puts = vi.spyOn(IDBObjectStore.prototype, 'put')

    push({ t: 'row', row: l.start('turn-1', 'r1') })
    for (let n = 1; n <= 1000; n++) {
      push({ t: 'delta', delta: delta('m1', n, 'x', { turnId: 'turn-1' }) })
      if (n % 10 === 0) {
        push({ t: 'row', row: l.tool('turn-1', `tool-${n}`, 'post') })
        vi.advanceTimersToNextFrame()
        await Promise.resolve()
      }
    }
    await settled()
    expect(puts).not.toHaveBeenCalled()

    push({ t: 'row', row: l.end('turn-1') })
    vi.advanceTimersToNextFrame()
    await settled()
    expect(puts).toHaveBeenCalledTimes(1)
    puts.mockRestore()
    expect((await entry())?.seq).toBe(l.seq)
  })

  it('writes a running turn through as the last view detaches and as the tab hides', async () => {
    const detach = await attachFeed('chat-1')
    const l = ledger()
    push({ t: 'row', row: l.prompt('r1', 'hi') })
    push({ t: 'row', row: l.start('turn-1', 'r1') })
    push({ t: 'synced', seq: 2 })
    push({ t: 'row', row: l.tool('turn-1', 'a', 'post') })
    vi.advanceTimersToNextFrame()
    await settled()
    expect((await entry())?.seq).toBe(2)
    detach()
    await settled()
    expect((await entry())?.seq).toBe(3)
  })

  it('is forgotten with its deleted chat and with a restarted daemon', async () => {
    writeChat({ id: 'chat-1' })
    const stopLifecycle = followChatLifecycle()
    await store(1)
    await store(1, 'e1', 'chat-2')
    forgetEntities('chat', ['chat-1'])
    await settled()
    expect(await entry()).toBeUndefined()
    expect(await loadFeed('local', 'chat-2')).toBeDefined()
    stopLifecycle()

    streamDaemon.drop()
    streamDaemon.deliver({ t: 'ready', epoch: 'e2' })
    await settled()
    expect(await loadFeed('local', 'chat-2')).toBeUndefined()
  })

  it('works as before when the database cannot open', async () => {
    const broken = new IDBFactory()
    vi.spyOn(broken, 'open').mockImplementation(() => {
      throw new DOMException('denied', 'SecurityError')
    })
    globalThis.indexedDB = broken
    await attachFeed('chat-1')
    push({ t: 'row', row: rowsOf(1)[0] })
    push({ t: 'synced', seq: 1 })
    await settled()
    expect(feed()).toMatchObject({ status: 'live', order: shown(1) })
  })
})
