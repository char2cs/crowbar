import { beforeEach, describe, expect, it } from 'vitest'
import {
  applyFeedDeltas,
  applyFeedFrame,
  chatStore,
  dropFeed,
  failFeed,
  openFeed,
} from '@/features/agent/stores/chat-store'
import { resetChatStore } from '@/__tests__/__fixtures__/agent-chat'
import { delta, ledger } from '@/__tests__/__fixtures__/ledger-rows'

const feed = (chatId = 'chat-1') => chatStore.getState().feeds[chatId]

beforeEach(resetChatStore)

describe('chatStore', () => {
  it('opens a connecting feed once and keeps it on a second open', () => {
    openFeed('chat-1')
    const first = feed()
    openFeed('chat-1')
    expect(feed()).toBe(first)
    expect(first?.status).toBe('connecting')
  })

  it('applies rows live once ready, writing only when the feed changes', () => {
    openFeed('chat-1')
    const l = ledger()
    const row = l.prompt('r1', 'hi')
    applyFeedFrame('chat-1', { type: 'ready', epoch: 'e1' })
    applyFeedFrame('chat-1', { type: 'row', row })
    const after = chatStore.getState().feeds
    applyFeedFrame('chat-1', { type: 'row', row })
    expect(chatStore.getState().feeds).toBe(after)
    expect(feed()?.order).toEqual(['p:r1'])
  })

  it('keeps other chats untouched by a frame', () => {
    openFeed('chat-1')
    openFeed('chat-2')
    const other = feed('chat-2')
    applyFeedFrame('chat-1', { type: 'ready', epoch: 'e1' })
    expect(feed('chat-2')).toBe(other)
  })

  it('applies coalesced deltas in one write', () => {
    openFeed('chat-1')
    applyFeedFrame('chat-1', { type: 'ready', epoch: 'e1' })
    applyFeedDeltas('chat-1', [delta('m1', 1, 'a'), delta('m1', 2, 'b')])
    expect(feed()?.live['answer:m1']).toBe('ab')
  })

  it('records a failure and drops the feed', () => {
    openFeed('chat-1')
    failFeed('chat-1', 'refused')
    expect(feed()).toMatchObject({ status: 'error', error: 'refused' })
    dropFeed('chat-1')
    expect(chatStore.getState().feeds).toEqual({})
  })

  // A reattach replays from since=0, so a long chat's whole record folds before
  // ready. The structural guard is fold-rows.test's one-copy-per-replay check;
  // this generous clock only backstops it (row-by-row copying took ~17s here).
  it('folds a 30000-row replay into one publish, well inside a backstop', () => {
    openFeed('chat-1')
    const rows = replay(30_000)
    let publishes = 0
    const unsubscribe = chatStore.subscribe(() => void (publishes += 1))
    const started = performance.now()
    for (const row of rows) applyFeedFrame('chat-1', { type: 'row', row })
    applyFeedFrame('chat-1', { type: 'ready', epoch: 'e1' })
    const took = performance.now() - started
    unsubscribe()

    expect(publishes).toBe(1)
    expect(feed()?.order).toHaveLength(15_000)
    expect(feed()?.unsettled).toEqual({})
    expect(took).toBeLessThan(5_000)
  })
})

/** Turns of six rows each: prompt, start, a tool, a final text, end. */
function replay(count: number) {
  const l = ledger()
  return Array.from({ length: Math.floor(count / 6) }, (_, i) => [
    l.prompt(`r${i}`, `prompt ${i}`),
    l.start(`turn-${i}`, `r${i}`),
    l.tool(`turn-${i}`, `tool-${i}`, 'pre'),
    l.tool(`turn-${i}`, `tool-${i}`, 'post'),
    l.text(`turn-${i}`, `m${i}`, 0, `reply ${i}`, true),
    l.end(`turn-${i}`),
  ]).flat()
}
