import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { attachFeed as attachReal } from '@/features/agent/stores/chat-feed'
import { chatStore } from '@/features/agent/stores/chat-store'
import { heldScopes, streamDaemon, streamDaemonFor } from '@/__tests__/__fixtures__/stream-daemon'
import { pairRemoteHost, resetHostWorld } from '@/__tests__/__fixtures__/remote-host'
import { delta, ledger } from '@/__tests__/__fixtures__/ledger-rows'

const SCOPE = 'chat:chat-1'
const feed = (chatId = 'chat-1') => chatStore.getState().feeds[chatId]
const push = (frame: object, scope = SCOPE) => streamDaemon.deliver({ scope, ...frame })
const subs = (scope = SCOPE) => streamDaemon.sent.filter((r) => r.op === 'sub' && r.scope === scope)
const held: (() => void)[] = []
const attachFeed = (chatId: string) => {
  const detach = attachReal('local', chatId)
  held.push(detach)
  return detach
}

beforeEach(() => {
  chatStore.setState({ feeds: {} })
  streamDaemon.answer = () => [] // each test writes what the daemon sends
  streamDaemon.deliver({ t: 'ready', epoch: 'e1' })
  streamDaemon.sent.length = 0
  vi.useFakeTimers({ toFake: ['requestAnimationFrame', 'cancelAnimationFrame'] })
})
afterEach(() => {
  held.splice(0).forEach((detach) => detach())
  vi.useRealTimers()
})

describe('attachFeed', () => {
  it('attaching a feed holds chat:<id> and detaching releases it', () => {
    const a = attachFeed('chat-1')
    const b = attachFeed('chat-1')
    expect(subs()).toEqual([{ op: 'sub', scope: SCOPE }])
    a()
    a()
    expect(heldScopes('chat:')).toEqual([SCOPE])
    b()
    expect(heldScopes('chat:')).toEqual([])
    expect(feed()).toBeUndefined()
  })

  it('publishes replayed rows once, at synced', () => {
    attachFeed('chat-1')
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

  it('a resubscribe sends since = the last applied seq', () => {
    attachFeed('chat-1')
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

  it('rebuilds from since=0 when a resync names a seq behind the feed', () => {
    attachFeed('chat-1')
    const l = ledger()
    push({ t: 'row', row: l.prompt('r1', 'hi') })
    push({ t: 'row', row: l.prompt('r2', 'again') })
    push({ t: 'synced', seq: 2 })
    push({ t: 'closed', reason: 'resync', seq: 1 })
    expect(subs().at(-1)).toEqual({ op: 'sub', scope: SCOPE })
    expect(feed()?.order).toEqual([])
  })

  it('drops a row that does not follow the last applied one, so the replay fills the hole', () => {
    attachFeed('chat-1')
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

  it('drops a live row past the next one, so the order never skips', () => {
    attachFeed('chat-1')
    const l = ledger()
    push({ t: 'row', row: l.prompt('r1', 'r1') })
    push({ t: 'synced', seq: 1 })
    const [r2, r3] = [l.prompt('r2', 'r2'), l.prompt('r3', 'r3')]

    push({ t: 'row', row: r3 })
    expect(feed()).toMatchObject({ seq: 1, order: ['p:r1'] })
    push({ t: 'row', row: r2 })
    push({ t: 'row', row: r3 })
    expect(feed()).toMatchObject({ seq: 3, order: ['p:r1', 'p:r2', 'p:r3'] })
  })

  it('a synced past the last applied row resubscribes from it once, then fails', () => {
    attachFeed('chat-1')
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

  it('a synced that agrees again clears the one resubscribe it allows', () => {
    attachFeed('chat-1')
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

  it('discards an unpublished replay on a resync before synced', () => {
    attachFeed('chat-1')
    push({ t: 'row', row: ledger().prompt('r1', 'hi') })
    push({ t: 'closed', reason: 'resync', seq: 1 })
    expect(subs().at(-1)).toEqual({ op: 'sub', scope: SCOPE })
    push({ t: 'row', row: ledger().prompt('r9', 'replayed') })
    push({ t: 'synced', seq: 1 })
    expect(feed()?.order).toEqual(['p:r9'])
  })

  it('a deleted close drops the feed and leaves the others', () => {
    attachFeed('chat-1')
    attachFeed('chat-2')
    push({ t: 'synced' })
    push({ t: 'synced' }, 'chat:chat-2')
    push({ t: 'closed', reason: 'deleted' })
    expect(feed()).toBeUndefined()
    expect(feed('chat-2')?.status).toBe('live')
    expect(heldScopes('chat:')).toEqual(['chat:chat-2'])
  })

  it('a refused chat says why and is not asked for again, even after a reconnect', () => {
    attachFeed('chat-1')
    const unreadable = "This chat's history could not be read"
    push({ t: 'closed', reason: 'refused', problem: { code: 'unavailable', message: unreadable } })
    expect(feed()).toMatchObject({ status: 'error', error: unreadable })
    expect(heldScopes('chat:')).toEqual([])

    streamDaemon.drop()
    streamDaemon.deliver({ t: 'ready', epoch: 'e1' })
    expect(subs()).toHaveLength(1)
  })

  it('a new epoch resets every feed', () => {
    attachFeed('chat-1')
    attachFeed('chat-2')
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

  it('coalesces deltas into one write per frame and flushes them before a row', () => {
    attachFeed('chat-1')
    push({ t: 'synced' })
    const writes = vi.fn()
    const stop = chatStore.subscribe(writes)
    push({ t: 'delta', delta: delta('m1', 1, 'a') })
    push({ t: 'delta', delta: delta('m1', 2, 'b') })
    expect(writes).not.toHaveBeenCalled()
    vi.advanceTimersToNextFrame()
    expect(writes).toHaveBeenCalledTimes(1)
    expect(feed()?.live['answer:m1']).toBe('ab')
    push({ t: 'delta', delta: delta('m1', 3, 'c') })
    push({ t: 'row', row: ledger('chat-1').text('turn-1', 'm1', 0, 'abc') })
    expect(feed()?.live['answer:m1']).toBe('abc')
    expect(feed()?.order).toEqual(['m:turn-1:m1'])
    stop()
  })
})

describe("a paired host's chat", () => {
  const host1 = streamDaemonFor('host-1')
  const subsOn = () => host1.sent.filter((r) => r.op === 'sub' && r.scope === SCOPE)
  afterEach(() => resetHostWorld())

  it('subs on its host and resumes from its seq when that host reconnects, not when local does', async () => {
    pairRemoteHost('host-1')
    await Promise.resolve()
    host1.answer = () => []
    held.push(attachReal('host-1', 'chat-1'))
    expect(subsOn()).toEqual([{ op: 'sub', scope: SCOPE }])
    expect(subs()).toEqual([])
    const l = ledger()
    host1.deliver({ t: 'row', scope: SCOPE, row: l.prompt('r1', 'hi') })
    host1.deliver({ t: 'synced', scope: SCOPE, seq: 1 })

    streamDaemon.drop()
    streamDaemon.deliver({ t: 'ready', epoch: 'e1' })
    expect(subs()).toEqual([])
    host1.drop()
    host1.deliver({ t: 'ready', epoch: 'h1' })
    expect(subsOn().at(-1)).toEqual({ op: 'sub', scope: SCOPE, since: 1 })
  })
})
