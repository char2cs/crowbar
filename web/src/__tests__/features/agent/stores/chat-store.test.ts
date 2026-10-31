import { beforeEach, describe, expect, it } from 'vitest'
import type { AgentTelemetry } from '@/features/agent/api/agent-chat-reads'
import {
  applyChat,
  applyFeedDeltas,
  applyFeedFrame,
  chatStore,
  dropFeed,
  failFeed,
  openFeed,
  removeChats,
  setChatTelemetry,
} from '@/features/agent/stores/chat-store'
import { chatSnapshot, resetChatStore } from '@/__tests__/__fixtures__/agent-chat'
import { delta, ledger } from '@/__tests__/__fixtures__/ledger-rows'

const feed = (chatId = 'chat-1') => chatStore.getState().feeds[chatId]

beforeEach(resetChatStore)

describe('chat entities', () => {
  it('indexes chats by workspace without building arrays in selectors', () => {
    applyChat(chatSnapshot({ id: 'a', workspaceId: 'w1' }))
    applyChat(chatSnapshot({ id: 'b', workspaceId: 'w2' }))
    applyChat(chatSnapshot({ id: 'c', workspaceId: 'w1' }))
    const w2 = chatStore.getState().byWorkspace.w2
    applyChat(chatSnapshot({ id: 'a', workspaceId: 'w1', title: 'renamed' }))
    expect(chatStore.getState().byWorkspace).toEqual({ w1: ['a', 'c'], w2: ['b'] })
    expect(chatStore.getState().byWorkspace.w2).toBe(w2)
    removeChats(['a', 'c'])
    expect(chatStore.getState().byWorkspace).toEqual({ w2: ['b'] })
  })

  it('finds the chat a runner is on, and leaves the index alone on a feed delta', () => {
    applyChat(chatSnapshot({ id: 'a', liveRunnerId: 'r1' }))
    applyChat(chatSnapshot({ id: 'b', liveRunnerId: 'r2' }))
    expect(chatStore.getState().byRunner).toEqual({ r1: 'a', r2: 'b' })
    const held = chatStore.getState()
    openFeed('a')
    applyFeedFrame('a', { type: 'ready', epoch: 'e' })
    applyFeedDeltas('a', [delta('m1', 1, 'hi')])
    expect(chatStore.getState().byRunner).toBe(held.byRunner)
    expect(chatStore.getState().chats).toBe(held.chats)
    applyChat(chatSnapshot({ id: 'a', liveRunnerId: '' }))
    applyChat(chatSnapshot({ id: 'b', liveRunnerId: 'r1' }))
    expect(chatStore.getState().byRunner).toEqual({ r1: 'b' })
    removeChats(['b'])
    expect(chatStore.getState().byRunner).toEqual({})
  })

  it('keeps the newer telemetry when an older observedAt arrives', () => {
    const report = (observedAt: string, usedPercent: number): AgentTelemetry => ({
      observedAt,
      source: 'callback',
      context: { usedPercent },
    })
    setChatTelemetry('a', report('2026-08-24T12:00:05Z', 40))
    setChatTelemetry('a', report('2026-08-24T12:00:01Z', 10))
    expect(chatStore.getState().telemetry.a?.context?.usedPercent).toBe(40)
    setChatTelemetry('a', report('2026-08-24T12:00:09Z', 55))
    expect(chatStore.getState().telemetry.a?.context?.usedPercent).toBe(55)
    setChatTelemetry('a', null)
    expect(chatStore.getState().telemetry.a).toBeNull()
  })
})

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
