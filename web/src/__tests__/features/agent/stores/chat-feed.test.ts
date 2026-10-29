import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { FeedFrame } from '@/features/agent/api/chat-rows'
import { attachFeed as attachReal, feedEndpoint } from '@/features/agent/stores/chat-feed'
import { chatStore } from '@/features/agent/stores/chat-store'
import { delta, ledger } from '@/__tests__/__fixtures__/ledger-rows'

interface Sub {
  endpoint: string
  cb: (data: unknown) => void
  url?: () => string
  closed: boolean
}
const subs: Sub[] = []

vi.mock('@/lib/ws/manager', () => ({
  wsManager: {
    subscribe: (endpoint: string, cb: (data: unknown) => void, url?: () => string) => {
      const sub: Sub = { endpoint, cb, url, closed: false }
      subs.push(sub)
      return () => {
        sub.closed = true
      }
    },
    opened: () => Promise.resolve(),
  },
}))

const open = () => subs.filter((s) => !s.closed)
const push = (frame: FeedFrame | { reconnected: true }) => open().forEach((s) => s.cb(frame))
const feed = () => chatStore.getState().feeds['chat-1']
const held: (() => void)[] = []
const attachFeed = (chatId: string) => {
  const detach = attachReal(chatId)
  held.push(detach)
  return detach
}

beforeEach(() => {
  subs.length = 0
  chatStore.setState({ feeds: {} })
  vi.useFakeTimers({ toFake: ['requestAnimationFrame', 'cancelAnimationFrame'] })
})
afterEach(() => {
  held.splice(0).forEach((detach) => detach())
  vi.useRealTimers()
})

describe('attachFeed', () => {
  it('attaches once for two views of one chat and detaches with the last', () => {
    const a = attachFeed('chat-1')
    const b = attachFeed('chat-1')
    expect(subs).toHaveLength(1)
    expect(subs[0]?.endpoint).toBe(feedEndpoint('chat-1'))
    a()
    a()
    expect(open()).toHaveLength(1)
    b()
    expect(open()).toHaveLength(0)
    expect(feed()).toBeUndefined()
  })

  it('publishes replayed rows once, at ready', () => {
    attachFeed('chat-1')
    const l = ledger()
    const writes = vi.fn()
    const stop = chatStore.subscribe(writes)
    push({ type: 'row', row: l.prompt('r1', 'hi') })
    push({ type: 'row', row: l.start('turn-1', 'r1') })
    expect(writes).not.toHaveBeenCalled()
    push({ type: 'ready', epoch: 'e1', seq: 2 })
    expect(writes).toHaveBeenCalledTimes(1)
    expect(feed()).toMatchObject({ status: 'live', seq: 2, order: ['p:r1'] })
    stop()
  })

  it('resumes with since=seq after a resync frame naming the feed seq', () => {
    attachFeed('chat-1')
    const l = ledger()
    push({ type: 'row', row: l.prompt('r1', 'hi') })
    push({ type: 'ready', epoch: 'e1', seq: 1 })
    push({ type: 'resync', seq: 1 })
    expect(subs[0]?.url?.()).toBe(`${feedEndpoint('chat-1')}?since=1`)
    expect(feed()?.order).toEqual(['p:r1'])
  })

  it('rebuilds from since=0 when resync names a seq behind the feed', () => {
    attachFeed('chat-1')
    const l = ledger()
    push({ type: 'row', row: l.prompt('r1', 'hi') })
    push({ type: 'row', row: l.prompt('r2', 'again') })
    push({ type: 'ready', epoch: 'e1', seq: 2 })
    push({ type: 'resync', seq: 1 })
    expect(subs[0]?.url?.()).toBe(`${feedEndpoint('chat-1')}?since=0`)
    expect(feed()?.order).toEqual([])
  })

  it('discards an unpublished replay on a resync before ready', () => {
    attachFeed('chat-1')
    const l = ledger()
    push({ type: 'row', row: l.prompt('r1', 'hi') })
    push({ type: 'resync', seq: 1 })
    expect(subs[0]?.url?.()).toBe(`${feedEndpoint('chat-1')}?since=0`)
    push({ type: 'row', row: ledger().prompt('r9', 'replayed') })
    push({ type: 'ready', epoch: 'e1', seq: 1 })
    expect(feed()?.order).toEqual(['p:r9'])
  })

  it('drops the feed and unsubscribes on a deleted frame', () => {
    attachFeed('chat-1')
    push({ type: 'ready', epoch: 'e1' })
    push({ type: 'deleted' })
    expect(open()).toHaveLength(0)
    expect(feed()).toBeUndefined()
  })

  it('ignores the manager reconnected callback', () => {
    attachFeed('chat-1')
    push({ type: 'ready', epoch: 'e1' })
    const before = feed()
    push({ reconnected: true })
    expect(feed()).toBe(before)
  })

  it('symptom 4: a missed prompt_settled arrives on resume since=seq', () => {
    attachFeed('chat-1')
    const l = ledger()
    push({ type: 'row', row: l.prompt('r1', 'hi') })
    push({ type: 'row', row: l.start('turn-1', 'r1') })
    push({ type: 'ready', epoch: 'e1', seq: 2 })
    push({ type: 'resync', seq: 2 })
    expect(subs[0]?.url?.()).toContain('since=2')
    push({ type: 'row', row: l.settle('r1') })
    push({ type: 'ready', epoch: 'e1', seq: 3 })
    expect(feed()?.facts.prompts.r1).toEqual({ state: 'settled' })
  })

  it('coalesces deltas into one write per frame and flushes them before a row', () => {
    attachFeed('chat-1')
    push({ type: 'ready', epoch: 'e1' })
    const writes = vi.fn()
    const stop = chatStore.subscribe(writes)
    push({ type: 'delta', delta: delta('m1', 1, 'a') })
    push({ type: 'delta', delta: delta('m1', 2, 'b') })
    expect(writes).not.toHaveBeenCalled()
    vi.advanceTimersToNextFrame()
    expect(writes).toHaveBeenCalledTimes(1)
    expect(feed()?.live['answer:m1']).toBe('ab')
    push({ type: 'delta', delta: delta('m1', 3, 'c') })
    push({ type: 'row', row: ledger('chat-1').text('turn-1', 'm1', 0, 'abc') })
    expect(feed()?.live['answer:m1']).toBe('abc')
    expect(feed()?.order).toEqual(['m:turn-1:m1'])
    stop()
  })
})
