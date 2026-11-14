import { describe, expect, it } from 'vitest'
import { applyDelta, applyReady, fuller } from '@/features/agent/stores/lib/fold-live'
import { emptyFeed } from '@/features/agent/stores/lib/fold-rows'
import { delta, ledger, applyRows } from '@/__tests__/__fixtures__/ledger-rows'

const live = () => applyReady(emptyFeed(), 'epoch-1')

describe('applyDelta', () => {
  it('appends a delta when n follows the last and ignores a repeated n', () => {
    let feed = applyDelta(live(), delta('m1', 1, 'Hel'))
    feed = applyDelta(feed, delta('m1', 2, 'lo'))
    const repeated = applyDelta(feed, delta('m1', 2, 'lo'))
    const gap = applyDelta(feed, delta('m1', 4, '??'))
    expect(feed.live['answer:m1']).toBe('Hello')
    expect(repeated).toBe(feed)
    expect(gap).toBe(feed)
  })

  it('replaces the live text on a replace frame', () => {
    let feed = applyDelta(live(), delta('m1', 1, 'Hel'))
    feed = applyDelta(feed, delta('m1', 7, 'Rewritten', { replace: true }))
    expect(feed.live['answer:m1']).toBe('Rewritten')
    expect(feed.liveN['answer:m1']).toBe(7)
  })

  it('drops live text when ready carries a new epoch', () => {
    const feed = applyDelta(live(), delta('m1', 1, 'Hel'))
    expect(applyReady(feed, 'epoch-1').live).toBe(feed.live)
    const restarted = applyReady(feed, 'epoch-2')
    expect(restarted.live).toEqual({})
    expect(restarted.liveN).toEqual({})
    expect(restarted.liveTail).toEqual([])
    expect(restarted.items.get('m:turn-1:m1')).toBeUndefined()
  })

  it('symptom 7: a delta changes only its own item live text', () => {
    const l = ledger()
    const feed = applyRows(live(), [
      l.prompt('r1', 'hi'),
      l.start('turn-1', 'r1'),
      l.text('turn-1', 'm1', 0, 'Hel'),
    ])
    const next = applyDelta(feed, delta('m1', 1, 'Hello'))
    expect(next.live['answer:m1']).toBe('Hello')
    expect(next.order).toBe(feed.order)
    expect(next.items).toBe(feed.items)
    expect(next.facts).toBe(feed.facts)
  })

  it('numbers a tool output stream and its diff stream separately', () => {
    let feed = applyDelta(live(), delta('tool-1', 1, 'out', { stream: 'tool_output' }))
    feed = applyDelta(feed, delta('tool-1', 1, '+a', { stream: 'diff' }))
    feed = applyDelta(feed, delta('tool-1', 2, 'put', { stream: 'tool_output' }))
    expect(feed.live).toEqual({ 'tool_output:tool-1': 'output', 'diff:tool-1': '+a' })
    expect(feed.liveTail).toEqual([])
  })

  it('holds a reply known only from deltas in the live tail until its row lands', () => {
    const l = ledger()
    const feed = applyDelta(live(), delta('m1', 1, 'Hel'))
    expect(feed.liveTail).toEqual(['m:turn-1:m1'])
    const landed = applyRows(feed, [l.text('turn-1', 'm1', 0, 'Hel')])
    expect(landed.liveTail).toEqual([])
    expect(landed.order).toEqual(['m:turn-1:m1'])
    expect(landed.items.get('m:turn-1:m1')?.seq).toBe(1)
  })

  it('tracks the newest main reasoning stream', () => {
    const feed = applyDelta(live(), delta('m1', 1, 'thinking', { stream: 'reasoning' }))
    expect(feed.liveReasoning).toBe('reasoning:m1')
  })
})

describe('fuller', () => {
  it('takes the text that extends the other', () => {
    expect(fuller('Hel', 'Hello')).toBe('Hello')
    expect(fuller('Hello', 'Hel')).toBe('Hello')
    expect(fuller('', 'x')).toBe('x')
    expect(fuller('abc', 'xyz1')).toBe('abc')
  })
})
