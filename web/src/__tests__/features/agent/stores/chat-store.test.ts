import { beforeEach, describe, expect, it } from 'vitest'
import {
  applyFeedDeltas,
  applyFeedFrame,
  chatStore,
  dropFeed,
  failFeed,
  openFeed,
} from '@/features/agent/stores/chat-store'
import { delta, ledger } from '@/__tests__/__fixtures__/ledger-rows'

const feed = (chatId = 'chat-1') => chatStore.getState().feeds[chatId]

beforeEach(() => chatStore.setState({ feeds: {} }))

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

  it('records a failure and drops on deleted', () => {
    openFeed('chat-1')
    failFeed('chat-1', 'refused')
    expect(feed()).toMatchObject({ status: 'error', error: 'refused' })
    expect(applyFeedFrame('chat-1', { type: 'deleted' })).toBe(false)
    expect(feed()).toBeUndefined()
    dropFeed('chat-1')
    expect(chatStore.getState().feeds).toEqual({})
  })
})
