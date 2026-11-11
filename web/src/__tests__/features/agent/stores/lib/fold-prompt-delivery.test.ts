import { describe, expect, it } from 'vitest'
import type { LedgerRow } from '@/features/agent/api/chat-rows'
import { applyLive, applyRow, emptyFeed } from '@/features/agent/stores/lib/fold-rows'
import type { ChatFeed, TextItem } from '@/features/agent/stores/lib/transcript-items'
import { applyRows, delta, ledger, recordedRows } from '@/__tests__/__fixtures__/ledger-rows'

/** A recording cut mid-ledger: the feed already holds every row before it. */
const feedAt = (seq: number): ChatFeed => ({ ...emptyFeed(), seq })
/** What the transcript draws: the folded order, then what is still streaming. */
const shown = (f: ChatFeed) => [...f.order, ...f.liveTail]

describe('prompt delivery', () => {
  it('reads a prompt settled unconsumed as refused, or retracted when it says so', () => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [
      l.prompt('r1', 'a'),
      l.settle('r1', false, 'runner_exited'),
      l.prompt('r2', 'b'),
      l.settle('r2', false, 'retracted'),
    ])
    expect(feed.facts.prompts.r1).toEqual({ state: 'refused', reason: 'runner_exited' })
    expect(feed.facts.prompts.r2).toEqual({ state: 'retracted' })
  })

  it('drops a prompt retracted before delivery from the order', () => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [l.prompt('r1', 'a'), l.settle('r1', false, 'retracted')])
    expect(feed.facts.prompts.r1).toEqual({ state: 'retracted' })
    expect(feed.order).toEqual([])
  })

  it('reads a turn retracted before any output as its prompt retracted, leaving no row', () => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [
      l.prompt('r1', 'a'),
      l.start('turn-1', 'r1'),
      l.retract('turn-1'),
    ])
    expect(feed.facts.prompts.r1).toEqual({ state: 'retracted' })
    expect(feed.facts.running).toBeNull()
    expect(feed.order).toEqual([])
  })

  it('keeps a retracted turn that had output, under its divider', () => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [
      l.prompt('r1', 'a'),
      l.start('turn-1', 'r1'),
      l.text('turn-1', 'm1', 0, 'partial'),
      l.retract('turn-1'),
    ])
    expect(feed.facts.prompts.r1).toEqual({ state: 'retracted', turnId: 'turn-1' })
    expect(feed.order).toEqual(['p:r1', 'm:turn-1:m1', 'k:4'])
  })

  // A recorded Stop mid-turn; the partial reply's row and a second turn_ended land after.
  it('draws a stopped turn once, under the reply its provider flushed after the stop', () => {
    const rows = recordedRows('feed-codex-pre-unix.txt', 52, 58)
    const prompt = rows[0] as LedgerRow<'user_message'>
    const turn = prompt.turnId!
    const feed = applyRows(feedAt(prompt.seq - 1), rows)

    const reply = feed.order.find((id) => feed.items.get(id)?.kind === 'text')
    const thought = feed.order.find((id) => feed.items.get(id)?.kind === 'reasoning')
    expect(feed.order).toEqual([`p:${prompt.body.requestId}`, thought, reply, 'k:54'])
    expect(feed.items.get(reply!)?.turnId).toBe(turn)
  })

  it('folds a reply that streamed before its rows into the item a replay folds', () => {
    const l = ledger()
    const head = [l.prompt('r1', 'a'), l.start('turn-1', 'r1')]
    const rows = [l.text('turn-1', 'm1', 0, 'one\ntwo', true), l.end('turn-1')]
    const replayed = applyRows(emptyFeed(), [...head, ...rows])
    const streamed = applyRows(applyLive(applyRows(emptyFeed(), head), delta('m1', 1, 'one')), rows)
    expect(streamed.items.get('m:turn-1:m1')).toEqual(replayed.items.get('m:turn-1:m1'))
  })

  it('streams a stopped turn above its marker at every moment (recorded Stop)', () => {
    const rows = recordedRows('feed-codex-pre-unix.txt', 52, 58)
    const prompt = rows[0] as LedgerRow<'user_message'>
    const reply = rows[4] as LedgerRow<'assistant_text'>
    const text = `m:${prompt.turnId}:${reply.body.messageId}`
    let feed = applyRows(feedAt(prompt.seq - 1), rows.slice(0, 2))
    feed = applyLive(feed, delta(reply.body.messageId, 1, '1. Apple', { turnId: prompt.turnId! }))
    const thought = `r:${prompt.turnId}:${reply.body.messageId}`
    for (const row of rows.slice(2)) {
      feed = applyRow(feed, row)
      // The reasoning row lands at the turn's end and reads above the reply.
      const reasoned = row.seq >= 57 ? [thought] : []
      expect(shown(feed)).toEqual([`p:${prompt.body.requestId}`, ...reasoned, text, 'k:54'])
      // A settled text stops following its stream, so it must hold the words itself.
      const item = feed.items.get(text) as TextItem
      if (item.settled) expect(item.text).toMatch(/^1\. Apple/)
    }
  })

  // The provider ran a prompt whose turn shows output, even if Esc handed the text back.
  it('brings a retracted prompt back above whatever output follows its retract', () => {
    for (const late of ['text', 'tool'] as const) {
      const l = ledger()
      const feed = applyRows(emptyFeed(), [
        l.prompt('r1', 'a'),
        l.start('turn-1', 'r1'),
        l.retract('turn-1'),
        late === 'text' ? l.text('turn-1', 'm1', -1, 'partial') : l.tool('turn-1', 'tool-1', 'pre'),
        l.end('turn-1', 'interrupted'),
      ])
      expect(feed.order).toEqual(['p:r1', late === 'text' ? 'm:turn-1:m1' : 't:tool-1', 'k:3'])
      expect(feed.facts.prompts.r1).toEqual({ state: 'delivered', turnId: 'turn-1' })
    }
  })

  it('brings a retracted prompt back when its turn streams after the retract', () => {
    const l = ledger()
    const traceless = applyRows(emptyFeed(), [
      l.prompt('r1', 'a'),
      l.start('turn-1', 'r1'),
      l.retract('turn-1'),
    ])
    expect(traceless.order).toEqual([])
    const feed = applyLive(traceless, delta('m1', 1, 'late'))
    expect(shown(feed)).toEqual(['p:r1', 'm:turn-1:m1', 'k:3'])
  })

  it('never folds a turn that streamed text traceless', () => {
    const l = ledger()
    const streaming = applyLive(
      applyRows(emptyFeed(), [l.prompt('r1', 'a'), l.start('turn-1', 'r1')]),
      delta('m1', 1, 'Work'),
    )
    const ended = applyRow(streaming, l.retract('turn-1'))
    expect(shown(ended)).toEqual(['p:r1', 'm:turn-1:m1', 'k:3'])
    const flushed = applyRow(ended, l.text('turn-1', 'm1', -1, 'Work'))
    expect(shown(flushed)).toEqual(['p:r1', 'm:turn-1:m1', 'k:3'])
  })

  it('delivers a prompt steered into the running turn by its own row', () => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [
      l.prompt('r1', 'a'),
      l.start('turn-1', 'r1'),
      l.steer('r2', 'b', 'turn-1'),
    ])
    expect(feed.facts.prompts.r2).toEqual({ state: 'delivered', turnId: 'turn-1' })
  })

  it('reads a prompt sent during a turn below what that turn still says, as it was delivered', () => {
    const l = ledger()
    const steer = { requestId: 'r2', text: 'and?', origin: 'crowbar' as const }
    const feed = applyRows(emptyFeed(), [
      l.prompt('r1', 'look at this'),
      l.start('turn-1', 'r1'),
      l.row('user_message', { ...steer, delivery: 'steer' }, { turnId: 'turn-2' }),
      l.tool('turn-1', 'read', 'pre'),
      l.tool('turn-1', 'read', 'post'),
      l.text('turn-1', 'm1', 0, 'You asked nothing yet.', true),
      l.end('turn-1'),
      l.start('turn-2', 'r2'),
      l.text('turn-2', 'm2', 0, 'A circle.', true),
    ])
    expect(feed.order).toEqual(['p:r1', 't:read', 'm:turn-1:m1', 'p:r2', 'm:turn-2:m2'])
  })

  it('keeps a steer parked on the turn end below that turn, and its reply under it', () => {
    const l = ledger()
    const waiting = applyRows(emptyFeed(), [
      l.prompt('r1', 'a'),
      l.start('turn-1', 'r1'),
      l.tool('turn-1', 'tool-1', 'pre'),
      l.steer('r2', 'b', 'turn-2'),
      l.tool('turn-1', 'tool-1', 'post'),
      l.tool('turn-1', 'tool-2', 'pre'),
      l.text('turn-1', 'm1', 0, 'DONE-A', true),
    ])
    expect(waiting.facts.prompts.r2).toMatchObject({ state: 'pending' })
    const parked = ['p:r1', 't:tool-1', 'm:turn-1:m1', 't:tool-2', 'p:r2']
    expect(waiting.order).toEqual(parked)
    const taken = applyRows(waiting, [
      l.end('turn-1'),
      l.start('turn-2', 'r2'),
      l.text('turn-2', 'm2', 0, 'BANANA', true),
    ])
    expect(taken.order).toEqual([...parked, 'm:turn-2:m2'])
  })
})
